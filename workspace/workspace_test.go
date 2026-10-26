package workspace_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/workspace"
)

// repo makes a git repository with the files in one commit, and returns its directory.
func repo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "-A"},
		{"-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestChanges(t *testing.T) {
	ctx := context.Background()
	dir := repo(t, map[string]string{
		".gitignore":          "build/\n",
		"nodes/3C/3C53TD.md":  "old\n",
		"nodes/AA/AA0001.md":  "keep\n",
		"lean/Math2/Old.lean": "gone\n",
	})
	w, err := workspace.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()
	for p, text := range map[string]string{
		"nodes/3C/3C53TD.md":   "new\n",
		"nodes/AA/AA0001.md":   "keep\n",
		"lean/Math2/New.lean":  "theorem t : True := trivial\n",
		"build/out.olean":      "ignored",
		"nodes/3C/../3C/x.txt": "x",
	} {
		if err := w.Write(p, []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Delete("lean/Math2/Old.lean"); err != nil {
		t.Fatal(err)
	}
	got, err := w.Changes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, c := range got {
		lines = append(lines, c.Path+" "+strings.TrimSpace(string(c.Old))+">"+strings.TrimSpace(string(c.New)))
		if c.Path == "lean/Math2/New.lean" && c.Old != nil || c.Path == "lean/Math2/Old.lean" && c.New != nil {
			t.Errorf("%s: old %q, new %q", c.Path, c.Old, c.New)
		}
	}
	want := "lean/Math2/New.lean >theorem t : True := trivial\nlean/Math2/Old.lean gone>\nnodes/3C/3C53TD.md old>new\nnodes/3C/x.txt >x"
	if s := strings.Join(lines, "\n"); s != want {
		t.Errorf("changes\n%s\nwant\n%s", s, want)
	}
	if data, err := w.Read("nodes/3C/3C53TD.md"); err != nil || string(data) != "new\n" {
		t.Errorf("read %q, %v", data, err)
	}
}

func TestEscape(t *testing.T) {
	ctx := context.Background()
	outside := t.TempDir()
	dir := repo(t, map[string]string{"a.md": "a\n"})
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	w, err := workspace.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()
	for _, p := range []string{"", ".", "../x", "/etc/passwd", ".git/config", "a/../../x", `a\b`, "link/x", "link/../../x"} {
		if err := w.Write(p, []byte("x")); err == nil {
			t.Errorf("Write(%q) gave no error", p)
		}
		if _, err := w.Read(p); err == nil {
			t.Errorf("Read(%q) gave no error", p)
		}
	}
	if entries, _ := os.ReadDir(outside); len(entries) > 0 {
		t.Errorf("a write reached the directory outside: %v", entries)
	}
	if err := w.Write("big", make([]byte, workspace.MaxFile+1)); err == nil {
		t.Error("a file over the limit gave no error")
	}
}

func TestAdd(t *testing.T) {
	ctx := context.Background()
	src := repo(t, map[string]string{"a.md": "a\n"})
	dir := filepath.Join(t.TempDir(), "run")
	w, err := workspace.Add(ctx, src, dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Write("b.md", []byte("b\n")); err != nil {
		t.Fatal(err)
	}
	if cs, err := w.Changes(ctx); err != nil || len(cs) != 1 || cs[0].Path != "b.md" {
		t.Errorf("changes %+v, %v", cs, err)
	}
	if err := w.Remove(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the work tree is still there: %v", err)
	}
	if _, err := workspace.Open(ctx, t.TempDir()); err == nil {
		t.Error("Open of a directory that is not a work tree gave no error")
	}
}
