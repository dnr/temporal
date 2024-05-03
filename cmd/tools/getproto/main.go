package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/exp/maps"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

type protoWriter struct {
	baseDir string
	out     *os.File
	level   int
}

var (
	pw *protoWriter

	importMap map[string]protoreflect.FileDescriptor
)

func fatalIfErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func checkImports(files map[string]protoreflect.FileDescriptor) {
	missing := make(map[string]struct{})
	for _, fd := range files {
		imports := fd.Imports()
		num := imports.Len()
		for i := 0; i < num; i++ {
			imp := imports.Get(i).Path()
			if strings.HasPrefix(imp, "temporal/api/") || strings.HasPrefix(imp, "google/") {
				if _, ok := files[imp]; !ok {
					missing[imp] = struct{}{}
				}
			}
		}
	}
	if len(missing) > 0 {
		addImports(maps.Keys(missing)) // doesn't return
	}
}

func (w *protoWriter) writeLine(format string, args ...any) {
	w.level -= strings.Count(format, "}")
	indent := strings.Repeat("  ", w.level)
	fmt.Fprintf(w.out, indent+format, args...)
	w.level += strings.Count(format, "{")
}

func (w *protoWriter) writeFile(relPath string, fd protoreflect.FileDescriptor) {
	fullPath := filepath.Join(w.baseDir, relPath)
	fatalIfErr(os.MkdirAll(filepath.Dir(fullPath), 0755))
	out, err := os.Create(fullPath)
	fatalIfErr(err)

	defer func() {
		out.Close()
		w.out = nil
	}()

	w.out = out
	w.level = 0

	w.writeLine("syntax = \"%s\";\n", fd.Syntax())
	w.writeLine("package %s;\n", fd.Package())

	opts, _ := fd.Options().(*descriptorpb.FileOptions)
	if pkg := opts.GetGoPackage(); pkg != "" {
		w.writeLine("option go_package = \"%s\";\n", pkg)
	}

	w.writeImports(fd.Imports())
	w.writeEnums(fd.Enums())
	w.writeMessages(fd.Messages())
	w.writeExtensions(fd.Extensions())
	w.writeServices(fd.Services())
}

func (w *protoWriter) writeImports(imports protoreflect.FileImports) {
	num := imports.Len()
	for i := 0; i < num; i++ {
		imp := imports.Get(i)
		// FIXME
		// if imp == "google/api/annotations.proto" {
		// 	continue
		// }
		w.writeLine("import \"%s\";\n", imp.Path())
	}
}

func (w *protoWriter) writeEnums(enums protoreflect.EnumDescriptors) {
	for i := 0; i < enums.Len(); i++ {
		e := enums.Get(i)

		w.writeLine("enum %s {\n", e.Name())

		vals := e.Values()
		for j := 0; j < vals.Len(); j++ {
			val := vals.Get(j)
			w.writeLine("%s = %d;\n", val.Name(), val.Number())
		}

		reservedNames := e.ReservedNames()
		for j := 0; j < reservedNames.Len(); j++ {
			reservedName := reservedNames.Get(j)
			w.writeLine("reserved \"%s\";\n", reservedName)
		}

		reservedRanges := e.ReservedRanges()
		for j := 0; j < reservedRanges.Len(); j++ {
			reservedRange := reservedRanges.Get(j)
			for r := reservedRange[0]; r < reservedRange[1]; r++ {
				w.writeLine("reserved %d;\n", r)
			}
		}

		w.writeLine("}\n\n")
	}
}

func (w *protoWriter) writeMessages(messages protoreflect.MessageDescriptors) {
	for i := 0; i < messages.Len(); i++ {
		m := messages.Get(i)

		w.writeLine("message %s {\n", m.Name())

		w.writeEnums(m.Enums())
		w.writeMessages(m.Messages())
		w.writeExtensions(m.Extensions())

		w.writeFields(m.Fields(), true)

		oneofs := m.Oneofs()
		for j := 0; j < oneofs.Len(); j++ {
			oneof := oneofs.Get(j)
			w.writeLine("oneof %s {\n", oneof.Name())
			w.writeFields(oneof.Fields(), false)
			w.writeLine("}\n")
		}

		reservedNames := m.ReservedNames()
		for j := 0; j < reservedNames.Len(); j++ {
			reservedName := reservedNames.Get(j)
			w.writeLine("reserved \"%s\";\n", reservedName)
		}

		reservedRanges := m.ReservedRanges()
		for j := 0; j < reservedRanges.Len(); j++ {
			reservedRange := reservedRanges.Get(j)
			for r := reservedRange[0]; r < reservedRange[1]; r++ {
				w.writeLine("reserved %d;\n", r)
			}
		}

		w.writeLine("}\n\n")
	}
}

func (w *protoWriter) writeFields(fields protoreflect.FieldDescriptors, skipOneOf bool) {
	for j := 0; j < fields.Len(); j++ {
		f := fields.Get(j)
		if f.ContainingOneof() != nil && skipOneOf {
			continue
		}
		if f.IsMap() {
			key := getKind(f.MapKey())
			val := getKind(f.MapValue())
			w.writeLine("map<%s, %s> %s = %d;\n", key, val, f.Name(), f.Number())
		} else {
			kind := getKind(f)
			switch f.Cardinality() {
			case protoreflect.Optional:
				if f.HasOptionalKeyword() {
					kind = "optional " + kind
				}
			case protoreflect.Required:
				log.Fatal("invalid with proto3")
			case protoreflect.Repeated:
				kind = "repeated " + kind
			}
			w.writeLine("%s %s = %d;\n", kind, f.Name(), f.Number())
		}
	}
}

func getKind(f protoreflect.FieldDescriptor) string {
	kind := f.Kind().String()
	switch kind {
	case "enum":
		return string(f.Enum().FullName())
	case "message":
		return string(f.Message().FullName())
	case "group":
		log.Fatal("we don't handle groups")
	}
	return kind
}

func (w *protoWriter) writeServices(services protoreflect.ServiceDescriptors) {
	// This function is not used, just here for completeness. We don't import or depend on
	// service definitions.
	for i := 0; i < services.Len(); i++ {
		s := services.Get(i)
		w.writeLine("service %s {\n", s.Name())
		methods := s.Methods()
		for j := 0; j < methods.Len(); j++ {
			m := methods.Get(j)
			w.writeLine("rpc %s (%s) returns (%s) {\n", m.Name(), m.Input().FullName(), m.Output().FullName())
			// options would go here
			w.writeLine("}\n")
		}
		w.writeLine("}\n\n")
	}
}

func (w *protoWriter) writeExtensions(exts protoreflect.ExtensionDescriptors) {
	if exts.Len() > 0 {
		log.Fatal("we don't support custom extensions yet")
	}
}

func main() {
	if len(importMap) == 0 {
		initSeeds() // doesn't return
	}

	checkImports(importMap)

	baseDir, err := os.MkdirTemp("", "protofiles")
	fatalIfErr(err)
	pw = &protoWriter{baseDir: baseDir}
	for relPath, fd := range importMap {
		pw.writeFile(relPath, fd)
	}
	fmt.Println(baseDir)
}
