// The MIT License
//
// Copyright (c) 2020 Temporal Technologies Inc.  All rights reserved.
//
// Copyright (c) 2020 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package matching

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/dgryski/go-farm"

	"github.com/gogo/protobuf/proto"
	"go.temporal.io/api/serviceerror"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	persistencespb "go.temporal.io/server/api/persistence/v1"
	"go.temporal.io/server/common"
)

type (
	// versioningData is a thin wrapper around persistencespb.VersioningData that maintains an
	// index and wraps a mutation operation. versioningData is immutable once constructed.
	versioningData struct {
		data  persistencespb.VersioningData
		index map[taskqueuepb.VersionId]taskqueuepb.VersionId
	}
)

var (
	errNoVersioningData = errors.New("no versioning data but got versioned task")
	errUnknownBuildID   = errors.New("unknown build id")
)

func newVersioningData(data *persistencespb.VersioningData) *versioningData {
	if data == nil {
		// nil *versioningData means "this task queue is not versioned"
		return nil
	}
	return &versioningData{
		data:  *data,
		index: makeIndex(data),
	}
}

func makeIndex(data *persistencespb.VersioningData) map[taskqueuepb.VersionId]taskqueuepb.VersionId {
	index := make(map[taskqueuepb.VersionId]taskqueuepb.VersionId)
	// Empty value means we don't have a version yet. Map it to the current default.
	if id := data.CurrentDefault.GetVersion(); id != nil {
		index[taskqueuepb.VersionId{}] = *data.CurrentDefault.GetVersion()
	}
	for _, leaf := range data.CompatibleLeaves {
		if target := leaf.GetVersion(); target != nil {
			for _ = 0; leaf != nil; leaf = leaf.PreviousCompatible {
				if leaf.GetVersion() != nil {
					index[*leaf.GetVersion()] = *target
				}
			}
		}
	}
	return index
}

func (v *versioningData) GetData() *persistencespb.VersioningData {
	if v == nil {
		return nil
	}
	return &v.data
}

func (v *versioningData) GetTarget(versionID *taskqueuepb.VersionId) (taskqueuepb.VersionId, error) {
	var emptyVersionID taskqueuepb.VersionId

	if v == nil {
		// If the task queue is unversioned (v == nil) and a task comes in with no previous
		// version id (versionID == nil), that's not an error, we just keep everything
		// unversioned and use the empty string as the "build id" to locate a channel.
		if versionID == nil {
			return emptyVersionID, nil
		}
		// But if the task does have a version, that's an error: we should have versioning data.
		return emptyVersionID, errNoVersioningData
	}

	if versionID == nil {
		// We have versioning data but task came in without it. Use the empty value for "default"
		versionID = &emptyVersionID
	}
	if target, ok := v.index[*versionID]; ok {
		return target, nil
	}
	// We have versioning data but it doesn't mention this version ID, so we can't figure out
	// what it's compatible with.
	return emptyVersionID, errUnknownBuildID
}

func (v *versioningData) CloneAndApplyMutation(mutator func(*persistencespb.VersioningData) error) (*versioningData, error) {
	data := &persistencespb.VersioningData{}
	proto.Merge(data, v.GetData())
	err := mutator(data)
	if err != nil {
		return nil, err
	}
	return newVersioningData(data), nil
}

func (v *versioningData) ToBuildIdOrderingResponse(maxDepth int) *workflowservice.GetWorkerBuildIdOrderingResponse {
	return depthLimiter(v.GetData(), maxDepth, true)
}

// Hash returns a farm.Fingerprint64 Hash of the versioning data as bytes. If the data is nonexistent or
// invalid, returns nil.
func (v *versioningData) Hash() []byte {
	if v == nil || v.data.GetCurrentDefault() == nil {
		return nil
	}
	asBytes, err := v.data.Marshal()
	if err != nil {
		return nil
	}
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, farm.Fingerprint64(asBytes))
	return b
}

func depthLimiter(g *persistencespb.VersioningData, maxDepth int, noMutate bool) *workflowservice.GetWorkerBuildIdOrderingResponse {
	// note: g can be nil here
	curDefault := g.GetCurrentDefault()
	compatLeaves := g.GetCompatibleLeaves()
	if maxDepth > 0 {
		if noMutate {
			curDefault = common.CloneProto(g.GetCurrentDefault())
		}
		curNode := curDefault
		curDepth := 1
		for curDepth < maxDepth {
			if curNode.GetPreviousIncompatible() == nil {
				break
			}
			curNode = curNode.GetPreviousIncompatible()
			curDepth++
		}
		if curNode != nil {
			curNode.PreviousIncompatible = nil
		}
		// Apply to compatible leaves as well
		newCompatLeaves := make([]*taskqueuepb.VersionIdNode, len(g.GetCompatibleLeaves()))
		for ix := range compatLeaves {
			compatLeaf := compatLeaves[ix]
			if noMutate {
				compatLeaf = proto.Clone(compatLeaves[ix]).(*taskqueuepb.VersionIdNode)
			}
			curNode = compatLeaf
			curDepth = 1
			for curDepth < maxDepth {
				if curNode.GetPreviousCompatible() == nil {
					break
				}
				curNode = curNode.GetPreviousCompatible()
				curDepth++
			}
			if curNode != nil {
				curNode.PreviousCompatible = nil
			}
			newCompatLeaves[ix] = compatLeaf
		}
		compatLeaves = newCompatLeaves
	}
	return &workflowservice.GetWorkerBuildIdOrderingResponse{
		CurrentDefault:   curDefault,
		CompatibleLeaves: compatLeaves,
	}
}

// Given an existing graph and an update request, update the graph appropriately.
//
// See the API docs for more detail. In short, the graph looks like one long line of default versions, each of which
// is incompatible with the previous, optionally with branching compatibility branches. Like so:
//
// ─┬─1.0───2.0─┬─3.0───4.0
//  │           ├─3.1
//  │           └─3.2
//  ├─1.1
//  ├─1.2
//  └─1.3
//
// In the above graph, 4.0 is the current default, and [1.3, 3.2] is the set of current compatible leaves. Links
// going left are incompatible relationships, and links going up are compatible relationships.
//
// A request may:
// 1. Add a new version to the graph, as a default version
// 2. Add a new version to the graph, compatible with some existing version.
// 3. Add a new version to the graph, compatible with some existing version and as the new default.
// 4. Unset a version as a default. It will be dropped and its previous incompatible version becomes default.
// 5. Unset a version as a compatible. It will be dropped and its previous compatible version will become the new
//    compatible leaf for that branch.
func UpdateVersionsGraph(existingData *persistencespb.VersioningData, req *workflowservice.UpdateWorkerBuildIdOrderingRequest, maxSize int) error {
	if req.GetVersionId().GetWorkerBuildId() == "" {
		return serviceerror.NewInvalidArgument(
			"request to update worker build id ordering is missing a valid version identifier")
	}
	err := updateImpl(existingData, req)
	if err != nil {
		return err
	}
	// Limit graph size if it's grown too large
	depthLimiter(existingData, maxSize, false)
	return nil
}

func updateImpl(existingData *persistencespb.VersioningData, req *workflowservice.UpdateWorkerBuildIdOrderingRequest) error {
	// If the version is to become the new default, add it to the list of current defaults, possibly replacing
	// the currently set one.
	if req.GetBecomeDefault() {
		curDefault := existingData.GetCurrentDefault()
		isCompatWithCurDefault :=
			req.GetPreviousCompatible().GetWorkerBuildId() == curDefault.GetVersion().GetWorkerBuildId()
		if req.GetPreviousCompatible() != nil && !isCompatWithCurDefault {
			// It does not make sense to introduce a version which is the new overall default, but is somehow also
			// supposed to be compatible with some existing version, as that would necessarily imply that the newly
			// added version is somehow both compatible and incompatible with the same target version.
			return serviceerror.NewInvalidArgument("adding a new default version which is compatible " +
				" with any version other than the existing default is not allowed.")
		}
		if curDefault != nil {
			// If the current default is going to be the previous compat version with the one we're adding,
			// then we need to skip over it when setting the previous *incompatible* version.
			if isCompatWithCurDefault {
				existingData.CurrentDefault = &taskqueuepb.VersionIdNode{
					Version:              req.VersionId,
					PreviousCompatible:   curDefault,
					PreviousIncompatible: curDefault.PreviousIncompatible,
				}
			} else {
				// Otherwise, set the previous incompatible version to the current default.
				existingData.CurrentDefault = &taskqueuepb.VersionIdNode{
					Version:              req.VersionId,
					PreviousCompatible:   nil,
					PreviousIncompatible: curDefault,
				}
			}
		} else {
			existingData.CurrentDefault = &taskqueuepb.VersionIdNode{
				Version:              req.VersionId,
				PreviousCompatible:   nil,
				PreviousIncompatible: nil,
			}
		}
	} else {
		if req.GetPreviousCompatible() != nil {
			prevCompat, indexInCompatLeaves := findCompatibleNode(existingData, req.GetPreviousCompatible())
			if prevCompat != nil {
				newNode := &taskqueuepb.VersionIdNode{
					Version:              req.VersionId,
					PreviousCompatible:   prevCompat,
					PreviousIncompatible: nil,
				}
				if indexInCompatLeaves >= 0 {
					existingData.CompatibleLeaves[indexInCompatLeaves] = newNode
				} else {
					existingData.CompatibleLeaves = append(existingData.CompatibleLeaves, newNode)
				}
			} else {
				return serviceerror.NewNotFound(
					fmt.Sprintf("previous compatible version %v not found", req.GetPreviousCompatible()))
			}
		} else {
			// Check if the version is already a default, and remove it from being one if it is.
			curDefault := existingData.GetCurrentDefault()
			if curDefault.GetVersion().Equal(req.GetVersionId()) {
				existingData.CurrentDefault = nil
				if curDefault.GetPreviousCompatible() != nil {
					existingData.CurrentDefault = curDefault.GetPreviousCompatible()
				} else if curDefault.GetPreviousIncompatible() != nil {
					existingData.CurrentDefault = curDefault.GetPreviousIncompatible()
				}
				return nil
			}
			// Check if it's a compatible leaf, and remove it from being one if it is.
			for i, def := range existingData.GetCompatibleLeaves() {
				if def.GetVersion().Equal(req.GetVersionId()) {
					existingData.CompatibleLeaves =
						append(existingData.CompatibleLeaves[:i], existingData.CompatibleLeaves[i+1:]...)
					if def.GetPreviousCompatible() != nil {
						existingData.CompatibleLeaves =
							append(existingData.CompatibleLeaves, def.GetPreviousCompatible())
					}
					return nil
				}
			}
			return serviceerror.NewInvalidArgument(
				"requests to update build id ordering cannot create a new non-default version with no links")
		}
	}
	return nil
}

// Finds the node that the provided version should point at, given that it says it's compatible with the provided
// version. Note that this does not necessary mean *that* node. If the version being targeted as compatible has nodes
// which already point at it as their previous compatible version, that chain will be followed out to the leaf, which
// will be returned.
func findCompatibleNode(
	existingData *persistencespb.VersioningData,
	versionId *taskqueuepb.VersionId,
) (*taskqueuepb.VersionIdNode, int) {
	// First search down from all existing compatible leaves, as if any of those chains point at the desired version,
	// we will need to return that leaf.
	for ix, node := range existingData.GetCompatibleLeaves() {
		if node.GetVersion().Equal(versionId) {
			return node, ix
		}
		if findInNode(node, versionId) != nil {
			return node, ix
		}
	}
	// Otherwise, this must be targeting some version in the default/incompatible chain, and it will become a new leaf
	curDefault := existingData.GetCurrentDefault()
	if curDefault.GetVersion().Equal(versionId) {
		return curDefault, -1
	}
	if nn := findInNode(curDefault, versionId); nn != nil {
		return nn, -1
	}

	return nil, -1
}

func findInNode(
	node *taskqueuepb.VersionIdNode,
	versionId *taskqueuepb.VersionId,
) *taskqueuepb.VersionIdNode {
	if node.GetVersion().Equal(versionId) {
		return node
	}
	if node.GetPreviousCompatible() != nil {
		return findInNode(node.GetPreviousCompatible(), versionId)
	}
	if node.GetPreviousIncompatible() != nil {
		return findInNode(node.GetPreviousIncompatible(), versionId)
	}
	return nil
}
