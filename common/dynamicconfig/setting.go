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

import "time"

type (
	Type int

	Precedence int

	Setting[T any] struct {
		// string value of key. case-insensitive.
		Key Key
		// type, for validation
		Type Type
		// precedence
		Precedence Precedence
		// default value. ConstrainedDefault is used in preference to Default if non-nil.
		Default            T
		ConstrainedDefault []TypedConstrainedValue[T]
		// documentation
		Description string
	}

	BoolSetting     Setting[bool]
	IntSetting      Setting[int]
	FloatSetting    Setting[float64]
	StringSetting   Setting[string]
	DurationSetting Setting[time.Duration]
	MapSetting      Setting[map[string]any]

	GenericSetting interface {
		GetKey() Key
		GetType() Type
		GetPrecedence() Precedence
		GetDefault() any
		GetDescription() string
	}
)

const (
	TypeBool     Type = iota // go type: bool
	TypeInt                  // go type: int
	TypeFloat                // go type: float64
	TypeString               // go type: string
	TypeDuration             // go type: time.Duration
	TypeMap                  // go type: map[string]any
)

const (
	PrecedenceGlobal Precedence = iota
	PrecedenceNamespace
	PrecedenceNamespaceId
	PrecedenceTaskQueue
	PrecedenceShardId
	PrecedenceTaskType
)

func (s *BoolSetting) GetKey() Key                   { return s.Key }
func (s *BoolSetting) GetType() Type                 { return s.Type }
func (s *BoolSetting) GetPrecedence() Precedence     { return s.Precedence }
func (s *BoolSetting) GetDefault() any               { return s.Default }
func (s *BoolSetting) GetDescription() string        { return s.Description }
func (s *IntSetting) GetKey() Key                    { return s.Key }
func (s *IntSetting) GetType() Type                  { return s.Type }
func (s *IntSetting) GetPrecedence() Precedence      { return s.Precedence }
func (s *IntSetting) GetDefault() any                { return s.Default }
func (s *IntSetting) GetDescription() string         { return s.Description }
func (s *FloatSetting) GetKey() Key                  { return s.Key }
func (s *FloatSetting) GetType() Type                { return s.Type }
func (s *FloatSetting) GetPrecedence() Precedence    { return s.Precedence }
func (s *FloatSetting) GetDefault() any              { return s.Default }
func (s *FloatSetting) GetDescription() string       { return s.Description }
func (s *StringSetting) GetKey() Key                 { return s.Key }
func (s *StringSetting) GetType() Type               { return s.Type }
func (s *StringSetting) GetPrecedence() Precedence   { return s.Precedence }
func (s *StringSetting) GetDefault() any             { return s.Default }
func (s *StringSetting) GetDescription() string      { return s.Description }
func (s *DurationSetting) GetKey() Key               { return s.Key }
func (s *DurationSetting) GetType() Type             { return s.Type }
func (s *DurationSetting) GetPrecedence() Precedence { return s.Precedence }
func (s *DurationSetting) GetDefault() any           { return s.Default }
func (s *DurationSetting) GetDescription() string    { return s.Description }
func (s *MapSetting) GetKey() Key                    { return s.Key }
func (s *MapSetting) GetType() Type                  { return s.Type }
func (s *MapSetting) GetPrecedence() Precedence      { return s.Precedence }
func (s *MapSetting) GetDefault() any                { return s.Default }
func (s *MapSetting) GetDescription() string         { return s.Description }
