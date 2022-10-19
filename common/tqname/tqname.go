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

package tqname

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// taskQueuePartitionPrefix is the required naming prefix for any task queue partition
	// other than partition 0 or any versioned task queue.
	taskQueuePartitionPrefix = "/_sys/"
)

type (
	// Name refers to the fully qualified task queue name
	Name struct {
		fullName   string // internal name of the task queue
		baseName   string // original name of the task queue as specified by user
		partition  int    // partition of task queue
		versionSet string // version set id
	}
)

// Parse returns a fully qualified task queue name.
// Fully qualified names contain additional metadata about task queue
// derived from their given name. The additional metadata only makes sense
// when a task queue has more than one partition. When there is more than
// one partition for a user specified task queue, or the task queue is
// versioned, each of the individual partitions have an internal name of
// the form
//
//     /_sys/<original name>/[<version set>:]<partition id>
//
// The name of the root partition is always the same as the user specified name. Rest of
// the partitions follow the naming convention above. In addition, the task queues partitions
// logically form a N-ary tree where N is configurable dynamically. The tree formation is an
// optimization to allow for partitioned task queues to dispatch tasks with low latency when
// throughput is low - See https://github.com/uber/cadence/issues/2098
//
// Returns error if the given name is non-compliant with the required format
// for task queue names
func Parse(name string) (Name, error) {
	baseName := name
	partition := 0
	versionSet := ""

	if strings.HasPrefix(name, taskQueuePartitionPrefix) {
		suffixOff := strings.LastIndex(name, "/")
		if suffixOff <= len(taskQueuePartitionPrefix) {
			return Name{}, fmt.Errorf("invalid partitioned task queue name %q", name)
		}
		baseName = name[len(taskQueuePartitionPrefix):suffixOff]

		suffix := name[suffixOff+1:]
		if partitionOff := strings.LastIndex(suffix, ":"); partitionOff == 0 {
			return Name{}, fmt.Errorf("invalid partitioned task queue name %q", name)
		} else if partitionOff > 0 {
			// pull out version set
			versionSet, suffix = suffix[:partitionOff], suffix[partitionOff+1:]
		}

		var err error
		partition, err = strconv.Atoi(suffix)
		if err != nil || partition < 0 || partition == 0 && len(versionSet) > 0 {
			return Name{}, fmt.Errorf("invalid partitioned task queue name %q", name)
		}
	}

	return Name{
		baseName:   baseName,
		partition:  partition,
		versionSet: versionSet,
	}, nil
}

func ParseAndSetPartition(name string, partition int) (Name, error) {
	tn, err := Parse(name)
	if err != nil {
		return Name{}, err
	}
	tn = tn.WithPartition(partition)
	return tn, nil
}

func ParseAndSetPartitionAndVersion(name string, partition int, versionSet string) (Name, error) {
	tn, err := Parse(name)
	if err != nil {
		return Name{}, err
	}
	tn = tn.WithPartition(partition)
	tn = tn.WithVersionSet(versionSet)
	return tn, nil
}

func (tn Name) WithPartition(partition int) Name {
	nn := tn
	nn.partition = partition
	return nn
}

func (tn Name) WithVersionSet(versionSet string) Name {
	nn := tn
	nn.versionSet = versionSet
	return nn
}

// IsRoot returns true if this task queue is a root partition
func (tn Name) IsRoot() bool {
	return tn.partition == 0
}

// GetRoot returns the root name for a task queue
// FIXME: rename to BaseName
func (tn Name) GetRoot() string {
	return tn.baseName
}

// Parent returns the name of the parent task queue
// input:
//   degree: Number of children at each level of the tree
// Returns empty string if this task queue is the root
func (tn Name) Parent(degree int) Name {
	if tn.IsRoot() || degree == 0 {
		return tn
	}
	parent := (tn.partition+degree-1)/degree - 1
	return tn.WithPartition(parent)
}

// FullName returns the internal name of the task queue
func (tn Name) FullName() string {
	if len(tn.versionSet) == 0 {
		if tn.partition == 0 {
			return tn.baseName
		}
		return fmt.Sprintf("%s%s/%s", taskQueuePartitionPrefix, tn.baseName, tn.partition)
	}
	// versioned always use prefix for simplicity
	return fmt.Sprintf("%s%s/%s:%s", taskQueuePartitionPrefix, tn.baseName, tn.versionSet, tn.partition)
}
