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

package dynamicconfig

import (
	enumspb "go.temporal.io/api/enums/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
)

type (
	// Client is a source of dynamic configuration. The default Client, fileBasedClient, reads
	// from a file in the filesystem, and refreshes it periodically. You can extend the server
	// with an alternate Client using ServerOptions.
	Client interface {
		// GetValue returns a set of values and associated constraints for a key. Not all
		// constraints are valid for all keys.
		//
		// Usually, the order of ConstrainedValues doesn't matter, since Collection looks for
		// an exact match on the Constraints. That is, when retriving a value for a namespace,
		// a value constrained to that namespace will always be used over a value with no
		// constraints, regardless of what order they appear in this slice. See the comment on
		// Constraints below.
		//
		// However, if two ConstrainedValues have the exact same Constraints, then the first
		// one will be used.
		//
		// If none of the ConstrainedValues match the constraints being for lookup, then the
		// server default value will be used.
		//
		// Note that GetValue is called very often and expected to be fast! You should not
		// synchronously call out to an external system. Instead you should keep a set of all
		// configured values, refresh it periodically or when notified, and only do in-memory
		// lookups inside of GetValue.
		GetValue(key Key) []ConstrainedValue
	}

	// NotifyingClient is an optional interface that a Client can also implement, that adds
	// support for faster notifications of dynamic config changes.
	NotifyingClient interface {
		// Adds a subscription to all updates from this Client. `update` will be called on any
		// change to the current value set. The caller should call `cancel` to cancel the
		// subscription. Calls to `update` will not be made concurrently.
		Subscribe(update ClientUpdateFunc) (cancel func())
	}

	// Called with modified keys on any change to the current value set.
	// Deleted keys/constraints will get a nil value.
	ClientUpdateFunc func(map[Key][]ConstrainedValue)

	// Key is a key/property stored in dynamic config. For convenience, it is recommended that
	// you treat keys as case-insensitive.
	Key string

	// ConstrainedValue is a value plus associated constraints. This is the main value type
	// used in the interface between Client and Collection.
	//
	// The type of the Value field depends on the key. Acceptable types will usually be one of:
	//   int, float64, bool, string, map[string]any, time.Duration
	// but settings for keys may define their own conversion functions that understand other
	// types.
	//
	// If time.Duration is expected, a string is also accepted, which will be converted using
	// timestamp.ParseDurationDefaultDays. If float64 is expected, int is also accepted. In
	// other cases, the exact type must be used. If a Value is returned with an unexpected
	// type, it will be ignored.
	ConstrainedValue struct {
		Constraints Constraints
		Value       any
	}
	// TypedConstrainedValue is used instead of ConstrainedValue for complex default values,
	// where the type is available at compile time.
	TypedConstrainedValue[T any] struct {
		Constraints Constraints
		Value       T
	}

	// Constraints describe under what conditions a ConstrainedValue should be used.
	// There are few standard "constraint precedence orders" that the server uses:
	//   global precedence:
	//     no constraints
	//   namespace precedence:
	//     Namespace
	//     no constraints
	//   task queue precedence
	//     Namespace+TaskQueueName+TaskQueueType
	//     Namespace+TaskQueueName
	//     TaskQueueName
	//     Namespace
	//     no constraints
	//   shard id precedence:
	//     ShardID
	//     no constraints
	// In each case, the constraints that the server is checking and the constraints that apply
	// to the value must match exactly, including the fields that are not set (zero values).
	// That is, for keys that use namespace precedence, you must either return a
	// ConstrainedValue with only Namespace set, or with no fields set. (Or return one of
	// each.) If you return a ConstrainedValue with Namespace and ShardID set, for example,
	// that value will never be used, even if the Namespace matches.
	Constraints struct {
		Namespace     string
		NamespaceID   string
		TaskQueueName string
		TaskQueueType enumspb.TaskQueueType
		ShardID       int32
		TaskType      enumsspb.TaskType
		Destination   string
	}
)

func (k Key) String() string {
	return string(k)
}
