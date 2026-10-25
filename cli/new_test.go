package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/schema"
	"github.com/tamnd/math2-agent/tag"
)

func TestNewEachKind(t *testing.T) {
	s := schema.Embedded()
	for _, k := range s.Kinds() {
		root := t.TempDir()
		code, stdout, stderr := run("new", "-root", root, k)
		if code != ExitOK || stderr != "" {
			t.Fatalf("%s: exit code %d, stderr %q", k, code, stderr)
		}
		path := strings.TrimSuffix(stdout, "\n")
		id := strings.TrimSuffix(filepath.Base(path), ".md")
		if filepath.Dir(path) != filepath.Join(root, "nodes", "~") || !tag.IsProvisional(id) {
			t.Fatalf("%s: the path is %q", k, path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}

		// m2 check reports each placeholder, and nothing else.
		code, stdout, _ = run("check", "-root", root, "-q")
		if code != ExitFailure {
			t.Errorf("%s: m2 check gave exit code %d, want %d", k, code, ExitFailure)
		}
		req, _ := s.Required(k)
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) != len(req)-2 {
			t.Errorf("%s: m2 check gave %d findings, want %d:\n%s", k, len(lines), len(req)-2, stdout)
		}
		for _, l := range lines {
			if !strings.Contains(l, ": S01: the field ") || !strings.HasSuffix(l, " has no value") {
				t.Errorf("%s: a finding is not a placeholder: %s", k, l)
			}
		}
		if code, stdout, _ := run("fmt", "-check", "-root", root); code != ExitOK {
			t.Errorf("%s: the new file is not canonical: %s", k, stdout)
		}
	}
}

func TestNewUsage(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"new", "-root", root}, {"new", "-root", root, "lemma"}, {"new", "-root", root, "proof", "statement"}} {
		if code, stdout, _ := run(args...); code != ExitUsage || stdout != "" {
			t.Errorf("%v: exit code %d, stdout %q", args, code, stdout)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "nodes")); err == nil {
		t.Error("m2 new made a directory after a usage error")
	}
}
