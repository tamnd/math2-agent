package understand_test

import (
	"bytes"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/tamnd/math2-agent/schema/understand"
)

func TestSchemas(t *testing.T) {
	files, err := fs.Glob(os.DirFS("."), "*.v1.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("the schema files are %v, %v", files, err)
	}
	for _, f := range files {
		pass := strings.TrimSuffix(f, ".v1.json")
		raw := understand.Of(pass)
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource(f, doc); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Compile(f); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	if understand.Of("explain") != nil {
		t.Error("a pass with no schema file has a schema")
	}
}
