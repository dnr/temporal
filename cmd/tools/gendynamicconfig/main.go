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

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"
)

type (
	settingType struct {
		Name   string
		GoType string
		Index  int
	}
)

var (
	types = []*settingType{
		{
			Name:   "Bool",
			GoType: "bool",
		},
		{
			Name:   "Int",
			GoType: "int",
		},
		{
			Name:   "Float",
			GoType: "float64",
		},
		{
			Name:   "String",
			GoType: "string",
		},
		{
			Name:   "Duration",
			GoType: "time.Duration",
		},
		{
			Name:   "Map",
			GoType: "map[string]any",
		},
	}
)

func panicIfErr(err error) {
	if err != nil {
		panic(err)
	}
}

func writeTemplatedCode(w io.Writer, text string, data any) {
	panicIfErr(template.Must(template.New("code").Parse(text)).Execute(w, data))
}

func generateType(w io.Writer, idx int, tp *settingType) {
	tp.Index = idx
	writeTemplatedCode(w, `
const Type{{.Name}} Type = {{.Index}} // go type: {{.GoType}}

type {{.Name}}Setting Setting[{{.GoType}}]

func (s *{{.Name}}Setting) GetKey() Key               { return s.Key }
func (s *{{.Name}}Setting) GetType() Type             { return s.Type }
func (s *{{.Name}}Setting) GetPrecedence() Precedence { return s.Precedence }
func (s *{{.Name}}Setting) GetDefault() any           { return s.Default }
func (s *{{.Name}}Setting) GetDescription() string    { return s.Description }

type (
	{{.Name}}PropertyFn                         func() {{.GoType}}
	{{.Name}}PropertyFnWithNamespaceFilter      func(namespace string) {{.GoType}}
	{{.Name}}PropertyFnWithNamespaceIDFilter    func(namespaceID string) {{.GoType}}
	{{.Name}}PropertyFnWithShardIDFilter        func(shardID int32) {{.GoType}}
	{{.Name}}PropertyFnWithTaskQueueInfoFilters func(namespace string, taskQueue string, taskType enumspb.TaskQueueType) {{.GoType}}
	{{.Name}}PropertyFnWithTaskTypeFilter       func(task enumsspb.TaskType) {{.GoType}}
)

func (c *Collection) Get{{.Name}}Property(s *{{.Name}}Setting) {{.Name}}PropertyFn {
	return func() {{.GoType}} {
		return matchAndConvert(
			c,
			(*Setting[{{.GoType}}])(s),
			globalPrecedence(),
			convert{{.Name}},
		)
	}
}

func (c *Collection) Get{{.Name}}PropertyFnWithNamespaceFilter(s *{{.Name}}Setting) {{.Name}}PropertyFnWithNamespaceFilter {
	return func(namespace string) {{.GoType}} {
		return matchAndConvert(
			c,
			(*Setting[{{.GoType}}])(s),
			namespacePrecedence(namespace),
			convert{{.Name}},
		)
	}
}

func (c *Collection) Get{{.Name}}PropertyFnWithNamespaceIDFilter(s *{{.Name}}Setting) {{.Name}}PropertyFnWithNamespaceIDFilter {
	return func(namespaceID string) {{.GoType}} {
		return matchAndConvert(
			c,
			(*Setting[{{.GoType}}])(s),
			namespaceIDPrecedence(namespaceID),
			convert{{.Name}},
		)
	}
}

func (c *Collection) Get{{.Name}}PropertyFilteredByTaskQueueInfo(s *{{.Name}}Setting) {{.Name}}PropertyFnWithTaskQueueInfoFilters {
	return func(namespace string, taskQueue string, taskType enumspb.TaskQueueType) {{.GoType}} {
		return matchAndConvert(
			c,
			(*Setting[{{.GoType}}])(s),
			taskQueuePrecedence(namespace, taskQueue, taskType),
			convert{{.Name}},
		)
	}
}

func (c *Collection) Get{{.Name}}PropertyFilteredByShardID(s *{{.Name}}Setting) {{.Name}}PropertyFnWithShardIDFilter {
	return func(shardID int32) {{.GoType}} {
		return matchAndConvert(
			c,
			(*Setting[{{.GoType}}])(s),
			shardIDPrecedence(shardID),
			convert{{.Name}},
		)
	}
}
`, tp)
}

func generate(w io.Writer) {
	writeTemplatedCode(w, `
package dynamicconfig

import (
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
)
`, nil)
	for idx, tp := range types {
		generateType(w, idx, tp)
	}
}

func callWithFile(f func(io.Writer), filename string, licenseText string) {
	w, err := os.Create(filename + "_gen.go")
	if err != nil {
		panic(err)
	}
	defer func() {
		panicIfErr(w.Close())
	}()
	if _, err := fmt.Fprintf(w, "%s\n// Code generated by cmd/tools/gendynamicconfig. DO NOT EDIT.\n", licenseText); err != nil {
		panic(err)
	}
	f(w)
}

func readLicenseFile(path string) string {
	text, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var lines []string
	for _, line := range strings.Split(string(text), "\n") {
		lines = append(lines, strings.TrimRight("// "+line, " "))
	}
	return strings.Join(lines, "\n") + "\n"
}

func main() {
	licenseFlag := flag.String("licence_file", "../../LICENSE", "path to license to copy into header")
	flag.Parse()

	licenseText := readLicenseFile(*licenseFlag)

	callWithFile(generate, "setting", licenseText)
}
