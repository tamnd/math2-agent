package lean_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/tamnd/math2-agent/lean"
)

// fakeLake puts a program lake on the PATH that prints the output and exits with the code.
func fakeLake(t *testing.T, output string, code int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake lake is a shell script")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "out"), []byte(output), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncat '" + filepath.Join(dir, "out") + "'\nexit " + strconv.Itoa(code) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "lake"), []byte(script), 0o700); err != nil { // #nosec G306 -- the test runs the script.
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

const messages = `{"caption":"","data":"declaration uses ` + "`sorry`" + `","endPos":{"column":11,"line":1},"fileName":"T.lean","isSilent":false,"keepFullRange":false,"kind":"hasSorry","pos":{"column":8,"line":1},"severity":"warning"}
{"caption":"","data":"Type mismatch","endPos":null,"fileName":"T.lean","isSilent":false,"keepFullRange":false,"kind":"[anonymous]","pos":{"column":23,"line":4},"severity":"error"}
`

func TestElaborate(t *testing.T) {
	fakeLake(t, messages, 1)
	e, err := lean.Elaborate(context.Background(), t.TempDir(), "T.lean")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Messages) != 2 || !e.Sorry() {
		t.Fatalf("messages %+v", e.Messages)
	}
	want := lean.Message{Severity: "error", Line: 4, Column: 23, Text: "Type mismatch"}
	if errs := e.Errors(); len(errs) != 1 || errs[0] != want {
		t.Errorf("errors %+v, want %+v", errs, want)
	}
	if w := e.Messages[0]; w.Kind != "hasSorry" || w.EndLine != 1 || w.EndColumn != 11 {
		t.Errorf("warning %+v", w)
	}
}

func TestElaborateFails(t *testing.T) {
	for name, tc := range map[string]struct {
		out  string
		code int
	}{
		"no message": {"", 1},
		"not JSON":   {"error: unknown package\n", 1},
		"no errors":  {messages[:len(messages)/2], 0},
	} {
		t.Run(name, func(t *testing.T) {
			fakeLake(t, tc.out, tc.code)
			if _, err := lean.Elaborate(context.Background(), t.TempDir(), "T.lean"); err == nil {
				t.Error("no error")
			}
		})
	}
}

func TestElaborateLean(t *testing.T) {
	if testing.Short() {
		t.Skip("the test runs Lean")
	}
	if _, err := exec.LookPath("lake"); err != nil {
		t.Skip("lake is not on the PATH")
	}
	dir := t.TempDir()
	toolchain, err := os.ReadFile("lean-toolchain")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"lean-toolchain": string(toolchain),
		"lakefile.toml":  "name = \"t\"\n",
		"T.lean":         "theorem t1 : 1 + 1 = 2 := by\n  sorry\n\ntheorem t2 : 2 = 3 := rfl\n\ntheorem t3 : 2 + 2 = 4 := rfl\n",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	e, err := lean.Elaborate(context.Background(), dir, "T.lean")
	if err != nil {
		t.Fatal(err)
	}
	if !e.Sorry() || len(e.Errors()) == 0 || e.Errors()[0].Line != 4 {
		t.Errorf("messages %+v", e.Messages)
	}
}
