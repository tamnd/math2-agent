package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNotation(t *testing.T) {
	root := t.TempDir()
	sty := "\\newcommand{\\R}{\\mathbb{R}}\n\\newcommand{\\abs}[1]{\\left|#1\\right|}\n"
	if err := os.MkdirAll(filepath.Join(root, "macros"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "macros", "m2.sty"), []byte(sty), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := run("notation", "seed", "-root", root); code != ExitOK || stdout != "notation/global.yaml\n" {
		t.Fatalf("seed: exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if code, _, stderr := run("notation", "seed", "-root", root); code != ExitFailure || stderr == "" {
		t.Fatalf("a second seed: exit code %d, stderr %q", code, stderr)
	}
	want := "macros/m2.katex.json\nmacros/m2.sty\n"
	if code, stdout, _ := run("notation", "build", "-check", "-root", root); code != ExitFailure || stdout != want {
		t.Fatalf("build -check: exit code %d, stdout %q", code, stdout)
	}
	if code, stdout, stderr := run("notation", "build", "-root", root); code != ExitOK || stdout != want {
		t.Fatalf("build: exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if code, stdout, _ := run("notation", "build", "-check", "-root", root); code != ExitOK || stdout != "" {
		t.Fatalf("build -check after build: exit code %d, stdout %q", code, stdout)
	}
	data, err := os.ReadFile(filepath.Join(root, "macros", "m2.katex.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "{\n  \"\\\\R\": \"\\\\mathbb{R}\",\n  \"\\\\abs\": \"\\\\left|#1\\\\right|\"\n}\n" {
		t.Errorf("m2.katex.json:\n%s", got)
	}
}

func TestNotationUsage(t *testing.T) {
	for _, args := range [][]string{{"notation"}, {"notation", "list"}, {"notation", "build", "extra"}} {
		if code, _, _ := run(args...); code != ExitUsage {
			t.Errorf("%v: exit code %d", args, code)
		}
	}
	if code, _, _ := run("notation", "build", "-root", t.TempDir()); code != ExitFailure {
		t.Errorf("build with no register: exit code %d", code)
	}
}
