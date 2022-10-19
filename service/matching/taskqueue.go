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
	"bytes"
	"fmt"
	"strconv"
	"strings"

	enumspb "go.temporal.io/api/enums/v1"

	"go.temporal.io/server/common/namespace"
)

// TODO: This file ought to be moved into common. Frontend is duplicating this name mangling scheme
//   and if it ever changes, it's likely to break.

type (
	// taskQueueID is the key that uniquely identifies a task queue
	taskQueueID struct {
		QualifiedTaskQueueName
		namespaceID namespace.ID
		taskType    enumspb.TaskQueueType
	}
	// QualifiedTaskQueueName refers to the fully qualified task queue name
	QualifiedTaskQueueName struct {
		name       string // internal name of the tasks list
		baseName   string // original name of the task queue as specified by user
		partition  int    // partitionID of task queue
		versionSet string // version set id
	}
)

const (
	// taskQueuePartitionPrefix is the required naming prefix for any task queue partition other than partition 0
	taskQueuePartitionPrefix = "/_sys/"
)

// newTaskQueueName returns a fully qualified task queue name.
// Fully qualified names contain additional metadata about task queue
// derived from their given name. The additional metadata only makes sense
// when a task queue has more than one partition. When there is more than
// one partition for a user specified task queue, each of the
// individual partitions have an internal name of the form
//
//     /_sys/[original-name]/[partitionID]
//
// The name of the root partition is always the same as the user specified name. Rest of
// the partitions follow the naming convention above. In addition, the task queues partitions
// logically form a N-ary tree where N is configurable dynamically. The tree formation is an
// optimization to allow for partitioned task queues to dispatch tasks with low latency when
// throughput is low - See https://github.com/uber/cadence/issues/2098
//
// Returns error if the given name is non-compliant with the required format
// for task queue names
func newTaskQueueName(name string) (QualifiedTaskQueueName, error) {
	tn := QualifiedTaskQueueName{
		name:     name,
		baseName: name,
	}
	if err := tn.init(); err != nil {
		return QualifiedTaskQueueName{}, err
	}
	return tn, nil
}

// NewTaskQueueNameWithPartition can be used to create root and non-root taskqueue names easily without needing to
// manually craft the correct string. See newTaskQueueName for more details.
// Note that name can be a base name or an prefixed name.
func NewTaskQueueNameWithPartition(name string, partition int) (QualifiedTaskQueueName, error) {
	tqName, err := newTaskQueueName(name)
	tqName.partition = partition
	tqName.name = mkName(tqName.baseName, partition, "")
	return tqName, err
}

func newTaskQueueNameWithPartitionAndVersion(name string, partition int, versionSetId string) (QualifiedTaskQueueName, error) {
	tqName, err := newTaskQueueName(name)
	tqName.partition = partition
	tqName.versionSet = versionSetId
	tqName.name = mkName(tqName.baseName, partition, versionSetId)
	return tqName, err
}

// IsRoot returns true if this task queue is a root partition
func (tn *QualifiedTaskQueueName) IsRoot() bool {
	return tn.partition == 0
}

// GetRoot returns the root name for a task queue
func (tn *QualifiedTaskQueueName) GetRoot() string {
	return tn.baseName
}

// Parent returns the name of the parent task queue
// input:
//   degree: Number of children at each level of the tree
// Returns empty string if this task queue is the root
func (tn *QualifiedTaskQueueName) Parent(degree int) string {
	if tn.IsRoot() || degree == 0 {
		return ""
	}
	pid := (tn.partition+degree-1)/degree - 1
	return mkName(tn.baseName, pid, tn.versionSet)
}

func mkName(baseName string, partition int, versionSetId string) string {
	if len(versionSetId) == 0 {
		if partition == 0 {
			return baseName
		}
		return fmt.Sprintf("%s%s/%s", taskQueuePartitionPrefix, baseName, partition)
	}
	// versioned always use prefix for simplicity
	return fmt.Sprintf("%s%s/%s:%s", taskQueuePartitionPrefix, baseName, versionSetId, partition)
}

func (tn *QualifiedTaskQueueName) init() error {
	// FIXME: move into constructor

	if !strings.HasPrefix(tn.name, taskQueuePartitionPrefix) {
		return nil
	}

	suffixOff := strings.LastIndex(tn.name, "/")
	if suffixOff <= len(taskQueuePartitionPrefix) {
		return fmt.Errorf("invalid partitioned task queue name %v", tn.name)
	}

	suffix := tn.name[suffixOff+1:]
	if partitionOff := strings.LastIndex(suffix, ":"); partitionOff >= 0 {
		// we have a version set id id and partition
		tn.versionSet = suffix[:partitionOff]
		if len(tn.versionSet) == 0 {
			return fmt.Errorf("invalid version set id in name %v", tn.name)
		}
		suffix = suffix[partitionOff+1:]
	}

	p, err := strconv.Atoi(suffix)
	if err != nil || p <= 0 {
		return fmt.Errorf("invalid partitioned task queue name %v", tn.name)
	}

	tn.partition = p
	tn.baseName = tn.name[len(taskQueuePartitionPrefix):suffixOff]
	return nil
}

func (tn *QualifiedTaskQueueName) String() string {
	return mkName(tn.baseName, tn.partition, tn.versionSet)
}

// newTaskQueueID returns taskQueueID which uniquely identfies as task queue
func newTaskQueueID(namespaceID namespace.ID, taskQueueName string, taskType enumspb.TaskQueueType) (*taskQueueID, error) {
	return newTaskQueueIDWithPartition(namespaceID, taskQueueName, taskType, 0)
}

func newTaskQueueIDWithPartition(
	namespaceID namespace.ID, taskQueueName string, taskType enumspb.TaskQueueType, partition int,
) (*taskQueueID, error) {
	name, err := NewTaskQueueNameWithPartition(taskQueueName, partition)
	if err != nil {
		return nil, err
	}
	return &taskQueueID{
		QualifiedTaskQueueName: name,
		namespaceID:            namespaceID,
		taskType:               taskType,
	}, nil
}

func newTaskQueueIDWithVersion(id *taskQueueID, versionSetID string) (*taskQueueID, error) {
	name, err := newTaskQueueNameWithPartitionAndVersion(id.baseName, id.partition, versionSetID)
	if err != nil {
		return nil, err
	}
	return &taskQueueID{
		QualifiedTaskQueueName: name,
		namespaceID:            id.namespaceID,
		taskType:               id.taskType,
	}, nil
}

func (tid *taskQueueID) String() string {
	var b bytes.Buffer
	b.WriteString("[")
	b.WriteString("name=")
	b.WriteString(tid.name)
	b.WriteString("type=")
	if tid.taskType == enumspb.TASK_QUEUE_TYPE_ACTIVITY {
		b.WriteString("activity")
	} else {
		b.WriteString("workflow")
	}
	b.WriteString("]")
	return b.String()
}
