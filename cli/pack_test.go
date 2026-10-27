package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/pack"
)

// packTree copies the valid fixtures and adds a volume file.
func packTree(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(valid)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, pack.Dir), 0o755); err != nil {
		t.Fatal(err)
	}
	head := "id: lagrange\ntitle: Lagrange's theorem\neditors: [github:ada]\nkind: book\ntargets: [Q529ES]\n"
	if err := os.WriteFile(filepath.Join(root, pack.Dir, "lagrange.yaml"), []byte(head+body), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestPackCheck(t *testing.T) {
	root := packTree(t, "choose: {DSG7KN: GY1XDB}\n")
	code, stdout, stderr := run("pack", "-root", root, "-check", "-json", "lagrange")
	if code != ExitOK {
		t.Fatalf("m2 pack -check exits with code %d: %s%s", code, stdout, stderr)
	}
	var r check.Report
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Results) != len(pack.Checks) {
		t.Errorf("the report has %d results", len(r.Results))
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(pack.LockFile("lagrange")))); !os.IsNotExist(err) {
		t.Errorf("m2 pack -check wrote the lock file: %v", err)
	}
}

func TestPackWrite(t *testing.T) {
	root := packTree(t, "choose: {DSG7KN: GY1XDB}\n")
	out := t.TempDir()
	code, _, stderr := run("pack", "-root", root, "-out", out, "lagrange")
	if code != ExitOK {
		t.Fatalf("m2 pack exits with code %d: %s", code, stderr)
	}
	page, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil || !strings.Contains(string(page), `id="Q529ES"`) {
		t.Errorf("index.html: %v", err)
	}
	for _, f := range []string{pack.Style, "katex/katex.min.css"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(f))); err != nil {
			t.Error(err)
		}
	}
	lock, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(pack.LockFile("lagrange"))))
	if err != nil || !strings.Contains(string(lock), "node: Q529ES@draft") {
		t.Errorf("the lock file is %s, %v", lock, err)
	}
}

func TestPackWriteFormats(t *testing.T) {
	root := packTree(t, "choose: {DSG7KN: GY1XDB}\nformats: [html, latex, pretext]\n")
	out := t.TempDir()
	if code, _, stderr := run("pack", "-root", root, "-out", out, "lagrange"); code != ExitOK {
		t.Fatalf("m2 pack exits with code %d: %s", code, stderr)
	}
	for _, f := range []string{"index.html", "latex/" + pack.MainTeX, "latex/" + pack.MacroSty, "pretext/" + pack.MainPTX} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(f))); err != nil {
			t.Error(err)
		}
	}
}

func TestPackFails(t *testing.T) {
	root := packTree(t, "")
	code, stdout, stderr := run("pack", "-root", root, "-check", "lagrange")
	if code != ExitFailure || !strings.Contains(stdout, "PC3") || !strings.Contains(stderr, "PC3") {
		t.Errorf("m2 pack -check of a volume with no choice exits with code %d:\n%s%s", code, stdout, stderr)
	}
	if code, _, _ := run("pack", "-root", root); code != ExitUsage {
		t.Errorf("m2 pack with no volume exits with code %d", code)
	}
	if code, _, stderr := run("pack", "-root", root, "nosuch"); code != ExitFailure || !strings.Contains(stderr, "nosuch.yaml") {
		t.Errorf("m2 pack nosuch exits with code %d: %s", code, stderr)
	}
}
