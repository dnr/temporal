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

package authorization

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"sync"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/protoc-gen-gogo/descriptor"

	annotationspb "go.temporal.io/api/annotations/v1"
)

type (
	serviceMethodAnnotations map[string]annotationspb.Method

	methodAnnotations struct {
		workflow serviceMethodAnnotations
		operator serviceMethodAnnotations
		admin    serviceMethodAnnotations
	}
)

var (
	initMethodAnnotations   sync.Once
	cachedMethodAnnotations *methodAnnotations
)

func loadAnnotations(protoFile, serviceName string) (serviceMethodAnnotations, error) {
	r, err := gzip.NewReader(bytes.NewReader(proto.FileDescriptor(protoFile)))
	if err != nil {
		return nil, err
	}
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if err := r.Close(); err != nil {
		return nil, err
	}
	var d descriptor.FileDescriptorProto
	if err := proto.Unmarshal(buf, &d); err != nil {
		return nil, err
	}

	out := make(serviceMethodAnnotations)

	for _, service := range d.Service {
		if *service.Name != serviceName {
			continue
		}
		for _, method := range service.Method {
			v, err := proto.GetExtension(method.Options, annotationspb.E_Method)
			if err != nil {
				return nil, fmt.Errorf("annotation for %s missing", *method.Name)
			}
			annotations := *v.(*annotationspb.Method)
			if annotations.Scope == annotationspb.SCOPE_UNSPECIFIED {
				return nil, fmt.Errorf("scope annotation for %s missing", *method.Name)
			}
			if annotations.Role == annotationspb.ROLE_UNSPECIFIED {
				return nil, fmt.Errorf("role annotation for %s missing", *method.Name)
			}
			out[*method.Name] = annotations
		}
	}

	return out, nil
}

func getMethodAnnotations() *methodAnnotations {
	initMethodAnnotations.Do(func() {
		var m methodAnnotations
		var err error
		m.workflow, err = loadAnnotations("temporal/api/workflowservice/v1/service.proto", "WorkflowService")
		if err != nil {
			panic(fmt.Errorf("couldn't load method annotations for WorkflowService: %s", err))
		}
		m.operator, err = loadAnnotations("temporal/api/operatorservice/v1/service.proto", "OperatorService")
		if err != nil {
			panic(fmt.Errorf("couldn't load method annotations for OperatorService: %s", err))
		}
		m.admin, err = loadAnnotations("temporal/server/api/adminservice/v1/service.proto", "AdminService")
		if err != nil {
			panic(fmt.Errorf("couldn't load method annotations for AdminService: %s", err))
		}
		cachedMethodAnnotations = &m
	})
	return cachedMethodAnnotations
}

func init() {
	_ = getMethodAnnotations()
	panic("ok")
}
