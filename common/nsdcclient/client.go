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

package nsdcclient

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/util"
)

type (
	NSDynamicConfigClient struct {
		r namespace.Registry

		valueLock sync.Mutex
		values    map[dynamicconfig.Key][]dynamicconfig.ConstrainedValue

		subscriptionLock sync.Mutex
		subscriptionIdx  int
		subscriptions    map[int]dynamicconfig.ClientUpdateFunc
	}
)

var _ dynamicconfig.NotifyingClient = (*NSDynamicConfigClient)(nil)

func NewNSDynamicConfigClient() *NSDynamicConfigClient {
	return &NSDynamicConfigClient{
		subscriptions: make(map[int]dynamicconfig.ClientUpdateFunc),
	}
}

// SetRegistry must be called before Start/Stop. If SetRegistry or Start is never called,
// GetValue always returns nil.
func (n *NSDynamicConfigClient) SetRegistry(r namespace.Registry) {
	n.r = r
}

func (n *NSDynamicConfigClient) Start() {
	n.r.RegisterStateChangeCallback(fmt.Sprintf("%p", n), n.callback)
}

func (n *NSDynamicConfigClient) Stop() {
	n.r.UnregisterStateChangeCallback(fmt.Sprintf("%p", n))
}

func (n *NSDynamicConfigClient) callback(ns *namespace.Namespace, deleted bool) {
	newValues := make(map[dynamicconfig.Key][]dynamicconfig.ConstrainedValue)
	changedValues := make(map[dynamicconfig.Key][]dynamicconfig.ConstrainedValue)
	nsValues, err := loadDynamicConfigFromNs(ns, deleted)
	if err != nil {
		// TODO: log error
		return
	}

	n.valueLock.Lock()

	for k, prevCvs := range n.values {
		// optimization: if exactly the same before and after, skip copying slice
		prevNsCvs := util.FilterSlice(prevCvs, func(cv dynamicconfig.ConstrainedValue) bool { return cv.Constraints.Namespace == ns.Name().String() })
		if slices.Equal(prevNsCvs, nsValues[k]) {
			newValues[k] = prevCvs
			continue
		}
		// otherwise, filter out old values and add new
		newCvs := util.FilterSlice(prevCvs, func(cv dynamicconfig.ConstrainedValue) bool { return cv.Constraints.Namespace != ns.Name().String() })
		newCvs = append(newCvs, nsValues[k]...)
		newValues[k] = newCvs
		changedValues[k] = newCvs
	}
	for k, newCvs := range nsValues {
		// all new
		if _, present := n.values[k]; !present {
			newValues[k] = newCvs
			changedValues[k] = newCvs
		}
	}

	n.values = newValues
	n.valueLock.Unlock()

	n.subscriptionLock.Lock()
	for _, update := range n.subscriptions {
		update(changedValues)
	}
	n.subscriptionLock.Unlock()
}

func (n *NSDynamicConfigClient) GetValue(key dynamicconfig.Key) []dynamicconfig.ConstrainedValue {
	n.valueLock.Lock()
	defer n.valueLock.Unlock()
	return n.values[key]
}

func (n *NSDynamicConfigClient) Subscribe(update dynamicconfig.ClientUpdateFunc) (cancel func()) {
	n.subscriptionLock.Lock()
	defer n.subscriptionLock.Unlock()

	n.subscriptionIdx++
	id := n.subscriptionIdx
	n.subscriptions[id] = update

	return func() {
		n.subscriptionLock.Lock()
		defer n.subscriptionLock.Unlock()
		delete(n.subscriptions, id)
	}
}

func loadDynamicConfigFromNs(ns *namespace.Namespace, deleted bool) (map[dynamicconfig.Key][]dynamicconfig.ConstrainedValue, error) {
	if deleted {
		return nil, nil
	}
	data := ns.DynamicConfigJson()
	if len(data) == 0 {
		return nil, nil
	}
	m := make(map[dynamicconfig.Key]any)
	err := json.Unmarshal([]byte(data), &m)
	if err != nil {
		return nil, err
	}

	out := make(map[dynamicconfig.Key][]dynamicconfig.ConstrainedValue)
	for k, v := range m {
		cvs := convertOneCvs(ns, v)
		if cvs == nil {
			// shorthand: single value
			cvs = []dynamicconfig.ConstrainedValue{{
				Value: v,
				Constraints: dynamicconfig.Constraints{
					Namespace: ns.Name().String(),
				},
			}}
		}
		out[k] = cvs
	}
	return out, nil
}

func convertOneCvs(ns *namespace.Namespace, v any) []dynamicconfig.ConstrainedValue {
	// check if it looks like a []ConstrainedValue
	// must be list
	vs, ok := v.([]any)
	if !ok {
		return nil
	}
	// each list item must be map and must have at least a "value" key
	for _, v := range vs {
		cvmap, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		_, ok = cvmap["value"]
		if !ok {
			return nil
		}
		// if it has "constraints", that must be a map too
		if constraints, ok := cvmap["constraints"]; ok {
			if _, ok := constraints.(map[string]any); !ok {
				return nil
			}
		}
	}
	var out []dynamicconfig.ConstrainedValue
	for _, v := range vs {
		// interpret as []ConstrainedValue
		cvmap := v.(map[string]any)

		cv := dynamicconfig.ConstrainedValue{
			Value: cvmap["value"],
			Constraints: dynamicconfig.Constraints{
				Namespace: ns.Name().String(),
			},
		}

		if cvconstraints := cvmap["constraints"]; cvconstraints != nil {
			for ckey, cval := range cvconstraints.(map[string]any) {
				switch strings.ToLower(ckey) {
				case "taskqueuename":
					if strval, ok := cval.(string); ok {
						cv.Constraints.TaskQueueName = strval
					} else {
						// log error here
						return nil
					}
				case "taskqueuetype":
					if ival, ok := cval.(int); ok {
						cv.Constraints.TaskQueueType = enumspb.TaskQueueType(ival)
					} else {
						// log error here
						return nil
					}
				default:
					// other constraints not allowed, log error here
					return nil
				}
			}
		}

		out = append(out, cv)
	}
	return out
}
