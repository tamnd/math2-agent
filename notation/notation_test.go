package notation

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const tree = "testdata/tree"

func TestLoad(t *testing.T) {
	r, err := Load(tree)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"notation/global.yaml", "notation/geometry.yaml", "notation/number-theory.yaml"}
	if !slices.Equal(r.Files, want) {
		t.Errorf("files %v, want %v", r.Files, want)
	}
	var ids []string
	for _, e := range r.Entries {
		ids = append(ids, e.ID())
	}
	if want := []string{"global.R", "global.ip", "geometry.goldenratio", "number-theory.totient"}; !slices.Equal(ids, want) {
		t.Errorf("ids %v, want %v", ids, want)
	}
	tot := r.Lookup(`\totient`)
	if len(tot) != 1 || tot[0].Line != 1 || tot[0].Args != 1 || tot[0].Variants[0].LaTeX != `\phi(#1)` {
		t.Fatalf("totient: %+v", tot)
	}
	if got := tot[0].Scopes(); !slices.Equal(got, []string{"number-theory"}) {
		t.Errorf("totient scopes %v", got)
	}
	if got := r.Lookup(`\goldenratio`)[0].Scopes(); !slices.Equal(got, []string{"geometry", "number-theory"}) {
		t.Errorf("goldenratio scopes %v", got)
	}
	if e := r.Lookup(`\ip`)[0]; !e.IsGlobal() || e.Line != 8 {
		t.Errorf("ip: global %v, line %d", e.IsGlobal(), e.Line)
	}
}

func TestLoadNone(t *testing.T) {
	if _, err := Load(t.TempDir()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("got %v, want fs.ErrNotExist", err)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"not a list":    "macro: \\R\n",
		"unknown field": "- macro: \\R\n  latex: \\mathbb{R}\n  colour: red\n",
		"bad macro":     "- macro: R\n  latex: \\mathbb{R}\n",
		"no expansion":  "- macro: \\R\n",
		"wrong args":    "- macro: \\ip\n  args: 1\n  latex: \\langle #1, #2\\rangle\n",
	}
	for name, data := range cases {
		if _, err := Parse("notation/x.yaml", []byte(data)); err == nil || !strings.HasPrefix(err.Error(), "notation/x.yaml:") {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestOutputs(t *testing.T) {
	r, err := Load(tree)
	if err != nil {
		t.Fatal(err)
	}
	sty := string(r.Sty())
	for _, want := range []string{
		"% notation/global.yaml\n\\newcommand{\\R}{\\mathbb{R}}\n\\newcommand{\\ip}[2]{\\left\\langle #1, #2\\right\\rangle}\n% notation/geometry.yaml\n",
		"\\newcommand{\\totient}[1]{\\varphi(#1)}\n",
	} {
		if !strings.Contains(sty, want) {
			t.Errorf("m2.sty has no %q:\n%s", want, sty)
		}
	}
	want := "{\n  \"\\\\R\": \"\\\\mathbb{R}\",\n  \"\\\\ip\": \"\\\\left\\\\langle #1, #2\\\\right\\\\rangle\",\n  \"\\\\goldenratio\": \"\\\\varphi\",\n  \"\\\\totient\": \"\\\\varphi(#1)\"\n}\n"
	if got := string(r.KaTeX()); got != want {
		t.Errorf("m2.katex.json:\n%s\nwant\n%s", got, want)
	}
}

// TestSeed checks that the register that Seed makes from a macro file gives the same macros back.
func TestSeed(t *testing.T) {
	sty := "% A comment.\n\\newcommand{\\N}{\\mathbb{N}}\n\\newcommand{\\abs}[1]{\\left|#1\\right|}\n"
	entries, err := Seed([]byte(sty))
	if err != nil {
		t.Fatal(err)
	}
	data, err := Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, Dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, Dir, Global+".yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(r.Sty()); !strings.HasSuffix(got, "% notation/global.yaml\n"+sty[13:]) {
		t.Errorf("the seed gives\n%s", got)
	}
	for _, bad := range []string{"\\newcommand{\\N}\n  {\\mathbb{N}}\n", "% none\n", "\\newcommand{\\ip}[1]{#1 #2}\n"} {
		if _, err := Seed([]byte(bad)); err == nil {
			t.Errorf("Seed(%q) gave no error", bad)
		}
	}
}

func TestConventions(t *testing.T) {
	cs, err := LoadConventions(tree)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].ID != "natural-numbers" || cs[0].Line != 1 || cs[0].Option("without-zero").Sources[0] != "https://example.org/book" {
		t.Fatalf("got %+v", cs)
	}
	if cs[0].Option("none") != nil {
		t.Error("an option that is not in the entry")
	}
	root := t.TempDir()
	if _, err := LoadConventions(root); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("no file: got %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, Dir), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		"- id: Ring\n  default: unital\n  options: [{option: unital}]\n",
		"- id: ring\n  default: commutative\n  options: [{option: unital}]\n",
		"- id: ring\n  default: unital\n  options: [{option: unital}, {option: unital}]\n",
		"- id: ring\n  default: unital\n  options: [{option: unital}]\n- id: ring\n  default: unital\n  options: [{option: unital}]\n",
	} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(ConventionsFile)), []byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConventions(root); err == nil {
			t.Errorf("%q gave no error", bad)
		}
	}
}
