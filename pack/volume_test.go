package pack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseErrors(t *testing.T) {
	for _, c := range []struct{ body, want string }{
		{"id: V\ntitle: T\neditors: [github:a]\nkind: book\ntargets: [Q529ES]\n", ":1: the id \"V\" is not a slug"},
		{"id: w\ntitle: T\neditors: [github:a]\nkind: book\ntargets: [Q529ES]\n", "the id w is not the name of the file"},
		{header + "kind: book\n", "no targets"},
		{header + "kind: poem\ntargets: [Q529ES]\n", ":4: the kind \"poem\""},
		{"id: v\ntitle: T\neditors: [ada]\nkind: book\ntargets: [Q529ES]\n", "is not a person"},
		{header + "kind: book\ntargets: [Q529ES]\nproofs: some\n", "proofs is \"some\""},
		{header + "kind: book\ntargets: [Q529ES]\nexercises: {difficulty: [4, 2]}\n", "difficulty is [4 2]"},
		{header + "kind: book\ntargets: [Q529ES]\nformats: [pdf]\n", "the format \"pdf\""},
		{header + "kind: book\ntargets: [Q529ES]\nglue: [{at: preface, writer: github:a}]\n", "exactly one of text and file"},
		{header + "kind: book\ntargets: [Q529ES]\noutline: [{title: A, sections: [{title: B}]}, {title: B, sections: []}]\n", "two parts of the outline have the id b"},
		{header + "kind: book\ntargets: [Q529ES]\ncolour: red\n", "field colour not found"},
		{header + "kind: book\ntargets: [Q529ES]\nconventions: [{convention: c}]\n", "no convention or no option"},
	} {
		_, err := Parse("volumes/v.yaml", []byte(c.body))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Parse(%q) = %v, want %q", c.body, err, c.want)
		}
	}
}

func TestParseDefaults(t *testing.T) {
	for kind, want := range map[string]string{"book": AllProofs, "article": AllProofs, "notes": MainProofs, "course": MainProofs, "survey": NoProofs, "reference": NoProofs, "problems": NoProofs} {
		v := volume(t, "kind: "+kind+"\ntargets: [Q529ES]\n")
		if got := v.proofs(); got != want {
			t.Errorf("a %s shows %s proofs, want %s", kind, got, want)
		}
	}
	v := volume(t, "kind: book\ntargets: [Q529ES]\n")
	if f := v.formats(); len(f) != 1 || f[0] != HTML {
		t.Errorf("the formats are %v", f)
	}
	if v.Line["kind"] != 4 {
		t.Errorf("kind is at line %d", v.Line["kind"])
	}
}

func TestLoadGlueFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, Dir, "v"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := header + "kind: book\ntargets: [Q529ES]\nglue: [{at: preface, file: preface.md, writer: github:ada}]\n"
	if err := os.WriteFile(filepath.Join(root, Dir, "v.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, Dir, "v", "preface.md"), []byte("Why this volume.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v, err := Load(root, "v")
	if err != nil {
		t.Fatal(err)
	}
	if v.Glue[0].Text != "Why this volume.\n" || v.File != "volumes/v.yaml" {
		t.Errorf("the volume is %+v", v)
	}
	if err := os.WriteFile(filepath.Join(root, Dir, "v.yaml"), []byte(strings.Replace(body, "preface.md", "../secret.md", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, "v"); err == nil || !strings.Contains(err.Error(), "is not in volumes/v/") {
		t.Errorf("Load of a glue file outside the volume = %v", err)
	}
}
