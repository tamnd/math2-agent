package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResStatus(t *testing.T) {
	root := t.TempDir()
	if code, _, _ := run("res", "status", "-root", root, "-day", "31-10-2026"); code != ExitUsage {
		t.Errorf("a bad day gave exit %d, want %d", code, ExitUsage)
	}
	code, stdout, stderr := run("res", "status", "-root", root, "-day", "2026-10-31")
	if code != ExitOK || !strings.Contains(stdout, "0 live resources, 0 open decisions, 0 of 0 curriculum texts") {
		t.Fatalf("m2 res status = %d\n%s\n%s", code, stdout, stderr)
	}
	for _, f := range []string{"reports/resources.md", "reports/resources.json"} {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			t.Errorf("m2 res status did not write %s: %v", f, err)
		}
	}
}
