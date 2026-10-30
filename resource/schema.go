package resource

import (
	"bytes"
	"embed"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// The JSON Schemas of the catalogue. m2 writes them to schema/ in a math2 tree, beside the node schema (RS12.1).
const (
	ResourceSchema  = "resource.v1.json"
	CiteSchema      = "cite.v1.json"
	StructureSchema = "structure.v1.json"
)

//go:embed schema/*.json
var schemas embed.FS

// RawSchema returns the bytes of one of the embedded schemas.
func RawSchema(name string) ([]byte, error) { return schemas.ReadFile("schema/" + name) }

var (
	compileOnce sync.Once
	compiled    map[string]*jsonschema.Schema
	compileErr  error
)

func compile() {
	compiled = map[string]*jsonschema.Schema{}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	for _, name := range []string{ResourceSchema, CiteSchema, StructureSchema} {
		data, err := RawSchema(name)
		if err != nil {
			compileErr = err
			return
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			compileErr = fmt.Errorf("resource: %s: %w", name, err)
			return
		}
		if err := c.AddResource(name, doc); err != nil {
			compileErr = err
			return
		}
	}
	for _, name := range []string{ResourceSchema, CiteSchema, StructureSchema} {
		s, err := c.Compile(name)
		if err != nil {
			compileErr = err
			return
		}
		compiled[name] = s
	}
}

// Validate checks one JSON value against one of the embedded schemas. It returns nil when the value is valid.
func Validate(name string, data []byte) error {
	compileOnce.Do(compile)
	if compileErr != nil {
		return compileErr
	}
	s := compiled[name]
	if s == nil {
		return fmt.Errorf("resource: no schema %q", name)
	}
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return err
	}
	return s.Validate(v)
}
