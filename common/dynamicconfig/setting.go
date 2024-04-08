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

//go:generate go run ../../cmd/tools/gendynamicconfig

package dynamicconfig

type (
	Type int

	Precedence int

	Setting[T any] struct {
		// string value of key. case-insensitive.
		Key Key
		// precedence
		Precedence Precedence
		// default value. ConstrainedDefault is used in preference to Default if non-nil.
		Default            T
		ConstrainedDefault []TypedConstrainedValue[T]
		// documentation
		Description string
	}

	GenericSetting interface {
		GetKey() Key
		GetType() Type
		GetPrecedence() Precedence
		GetDefault() any
		GetDescription() string
	}
)

const (
	PrecedenceGlobal Precedence = iota
	PrecedenceNamespace
	PrecedenceNamespaceId
	PrecedenceTaskQueue
	PrecedenceShardId
	PrecedenceTaskType
)
