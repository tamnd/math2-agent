package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResNotesMeasure(t *testing.T) {
	code, stdout, stderr := run("res", "notes", "measure", filepath.Join("..", "ingest", "arxiv", "testdata", "notes-sample.md"))
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if !strings.HasPrefix(stdout, "arxiv-notes/1: 94 papers, precision ") || !strings.Contains(stdout, "wrong: ") {
		t.Errorf("stdout = %q", stdout)
	}
	root := t.TempDir()
	writeFile(t, root, "bad.md", "---\nrecords:\n    - id: \"1\"\n      title: T\n      colour: red\n---\n")
	if code, _, _ := run("res", "notes", "measure", filepath.Join(root, "bad.md")); code != ExitFailure {
		t.Errorf("a bad sample: exit code %d, want %d", code, ExitFailure)
	}
}

func TestResNotesUsage(t *testing.T) {
	for _, args := range [][]string{
		{"res", "notes"},
		{"res", "notes", "measure"},
		{"res", "notes", "scan", "extra"},
		{"res", "notes", "fetch"},
	} {
		if code, _, stderr := run(args...); code != ExitUsage || !strings.Contains(stderr, "usage") {
			t.Errorf("%v: exit code %d, stderr %q", args, code, stderr)
		}
	}
	code, _, stderr := run("res", "notes", "scan", "-gap", "2s")
	if code != ExitUsage || !strings.Contains(stderr, "10 requests a minute") {
		t.Errorf("a gap of 2s: exit code %d, stderr %q", code, stderr)
	}
}
