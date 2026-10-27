package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusSample(t *testing.T) {
	root := t.TempDir()
	if code, _, _ := run("corpus", "sample", "-root", root, "-month", "2026-13", "-source", "arxiv"); code != ExitUsage {
		t.Errorf("a bad month gave %d", code)
	}
	if code, _, stderr := run("corpus", "sample", "-root", root, "-month", "2026-10", "-source", "stacks"); code != ExitUsage || !strings.Contains(stderr, "no strata") {
		t.Errorf("another source gave %d, %q", code, stderr)
	}
	code, stdout, stderr := run("corpus", "sample", "-root", root, "-month", "2026-10", "-source", "arxiv")
	if code != ExitOK || !strings.Contains(stdout, "extraction source") || !strings.Contains(stdout, "Give -write") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	path := filepath.Join(root, SamplesDir, "2026-10-arxiv.jsonl")
	if _, err := os.Stat(path); err == nil {
		t.Error("a sample with no -write wrote the packets")
	}
	if code, _, stderr := run("corpus", "sample", "-root", root, "-month", "2026-10", "-source", "arxiv", "-write"); code != ExitOK {
		t.Fatalf("-write gave %d, %q", code, stderr)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := run("corpus", "sample", "-root", root, "-month", "2026-10", "-source", "arxiv", "-write"); code != ExitFailure || !strings.Contains(stderr, "drawn once") {
		t.Errorf("a second sample of the month gave %d, %q", code, stderr)
	}
}
