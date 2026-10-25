package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFmtCheckValidTree(t *testing.T) {
	code, stdout, stderr := run("fmt", "-check", "-root", validRoot)
	if code != ExitOK || stdout != "" || stderr != "" {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

// copyTree copies the files below src into dst.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
}

func TestFmtAliasTree(t *testing.T) {
	root := t.TempDir()
	copyTree(t, validRoot, root)
	aliases := filepath.Join("..", "testdata", "nodes", "aliases")
	for _, dir := range []string{"nodes", "registers"} {
		copyTree(t, filepath.Join(aliases, dir), filepath.Join(root, dir))
	}
	want := "nodes/A1/A1A5K1.md\nnodes/A2/A2A5K2.md\nnodes/A3/A3A5K3.md\n"
	path := filepath.Join(root, "nodes", "A2", "A2A5K2.md")
	before, _ := os.ReadFile(path)

	code, stdout, _ := run("fmt", "-check", "-root", root)
	if code != ExitFailure || stdout != want {
		t.Fatalf("fmt -check: exit code %d, stdout:\n%s", code, stdout)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("fmt -check changed a file")
	}
	if code, stdout, _ := run("fmt", "-root", root); code != ExitOK || stdout != want {
		t.Fatalf("fmt: exit code %d, stdout:\n%s", code, stdout)
	}
	if code, stdout, _ := run("fmt", "-root", root); code != ExitOK || stdout != "" {
		t.Fatalf("a second fmt: exit code %d, stdout:\n%s", code, stdout)
	}
	if code, stdout, _ := run("check", "-root", root, "-q"); code != ExitOK {
		t.Fatalf("check after fmt: exit code %d, stdout:\n%s", code, stdout)
	}
}

func TestFmtBadFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nodes", "7K", "7K3QX2.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("no front matter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("fmt", "-root", root, path)
	if code != ExitFailure || stdout != "" || !strings.Contains(stderr, "nodes/7K/7K3QX2.md:1:1:") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if code, _, _ := run("fmt", "-frobnicate"); code != ExitUsage {
		t.Fatalf("an unknown flag gave exit code %d, want %d", code, ExitUsage)
	}
}
