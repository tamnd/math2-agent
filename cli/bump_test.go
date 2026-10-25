package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bumpTree writes a tree with a Lake project that pins Mathlib v4.35.0.
func bumpTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"lean/lean-toolchain": "leanprover/lean4:v4.35.0\n",
		"lean/lakefile.toml":  "name = \"Math2\"\n\n[[require]]\nname = \"mathlib\"\ngit = \"https://github.com/leanprover-community/mathlib4\"\nrev = \"v4.35.0\"\n",
	}
	for p, data := range files {
		path := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestBump(t *testing.T) {
	root := bumpTree(t)
	for _, tc := range []struct {
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{[]string{"-to", "v4.35.0"}, ExitOK, "math2 pins v4.35.0 already", ""},
		{[]string{"-to", "v4.36.0", "-n"}, ExitOK, "m2 bump moves Mathlib from v4.35.0 to v4.36.0", ""},
		{[]string{"-to", "v4.36.0-rc1", "-n"}, ExitFailure, "", "is not a release tag of Mathlib"},
		{[]string{"-to", "v4.36.0"}, ExitFailure, "", "must be a git repository with no changes"},
		{[]string{"-to", "v4.36.0", "-master"}, ExitUsage, "", "usage: m2 bump"},
		{[]string{"extra"}, ExitUsage, "", "usage: m2 bump"},
	} {
		code, stdout, stderr := run(append([]string{"bump", "-root", root}, tc.args...)...)
		if code != tc.code || !strings.Contains(stdout, tc.stdout) || !strings.Contains(stderr, tc.stderr) {
			t.Errorf("m2 bump %v: exit code %d, stdout %q, stderr %q", tc.args, code, stdout, stderr)
		}
	}
	if code, _, stderr := run("bump", "-root", t.TempDir(), "-to", "v4.36.0"); code != ExitFailure || !strings.Contains(stderr, "lean-toolchain") {
		t.Errorf("a tree with no Lake project: exit code %d, stderr %q", code, stderr)
	}
}
