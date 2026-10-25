package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSiteBuildCheckArchive(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "site")
	url := "https://example.org/math2"
	if code, _, stderr := run("site", "build", "-content", "../testdata/nodes/valid", "-out", out, "-url", url); code != ExitOK {
		t.Fatalf("m2 site build exited with code %d: %s", code, stderr)
	}
	code, stdout, stderr := run("site", "check", "-url", url, out)
	if code != ExitOK || !strings.Contains(stdout, "offline  pass") {
		t.Fatalf("m2 site check exited with code %d:\n%s%s", code, stdout, stderr)
	}
	zip := filepath.Join(dir, "math2-test.zip")
	code, stdout, stderr = run("site", "archive", "-url", url, "-o", zip, out)
	if code != ExitOK || !strings.Contains(stdout, "m2 site archive wrote "+zip) {
		t.Fatalf("m2 site archive exited with code %d:\n%s%s", code, stdout, stderr)
	}
	if st, err := os.Stat(zip); err != nil || st.Size() == 0 {
		t.Fatalf("the archive is not there: %v", err)
	}
	if code, _, _ := run("site", "archive", "-o", filepath.Join(dir, "none.zip"), filepath.Join(dir, "none")); code != ExitFailure {
		t.Errorf("an archive of no site exited with code %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "none.zip")); err == nil {
		t.Error("a failed archive left a file")
	}
	if code, _, _ := run("site", "frobnicate"); code != ExitUsage {
		t.Errorf("an unknown site command exited with code %d", code)
	}
}
