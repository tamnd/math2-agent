package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var validRoot = filepath.Join("..", "testdata", "nodes", "valid")

func TestCheckValidTree(t *testing.T) {
	code, stdout, stderr := run("check", "-root", validRoot)
	if code != ExitOK || stdout != "" {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "nothing failed") {
		t.Errorf("stderr has no summary:\n%s", stderr)
	}
}

func TestCheckProjects(t *testing.T) {
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(validRoot)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, "projects", "groups.yaml"), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const valid = "name: Groups\ngoal: Prove the members of the topic.\ntopic: ZZZJE6\nbudget: 1\nmembers:\n  - {id: github:octocat, role: owner}\n"
	write(valid)
	if code, _, stderr := run("check", "-root", root, "-q"); code != ExitOK {
		t.Fatalf("a valid project gives the exit code %d: %s", code, stderr)
	}
	for _, c := range []struct{ text, want string }{
		{strings.Replace(valid, "owner", "member", 1), "has no owner"},
		{strings.Replace(valid, "ZZZJE6", "Q529ES", 1), "the topic Q529ES is not a topic node"},
		{valid + "color: red\n", "field color not found"},
	} {
		write(c.text)
		if code, _, stderr := run("check", "-root", root, "-q"); code != ExitFailure || !strings.Contains(stderr, c.want) {
			t.Errorf("the exit code is %d and stderr is %q, want %q", code, stderr, c.want)
		}
	}
}

func TestCheckBrokenFile(t *testing.T) {
	path := filepath.Join("..", "testdata", "nodes", "broken", "nodes", "3N", "3N63EV.md")
	code, stdout, _ := run("check", "-root", validRoot, "-q", path)
	if code != ExitFailure {
		t.Fatalf("exit code is %d, want %d", code, ExitFailure)
	}
	if lines := strings.Split(strings.TrimSpace(stdout), "\n"); len(lines) != 1 || !strings.Contains(lines[0], ": S04: ") {
		t.Errorf("stdout must have one S04 line:\n%s", stdout)
	}
}

func TestCheckJSON(t *testing.T) {
	code, stdout, _ := run("check", "-root", validRoot, "-json", "-rule", "b01,G01")
	if code != ExitOK {
		t.Fatalf("exit code is %d", code)
	}
	var rep struct {
		Files   int
		Results []struct{ Rule, State string }
	}
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatal(err)
	}
	for _, r := range rep.Results {
		want := "skip"
		if r.Rule == "B01" || r.Rule == "G01" {
			want = "pass"
		}
		if r.State != want {
			t.Errorf("%s: the state is %s, want %s", r.Rule, r.State, want)
		}
	}
}

func TestCheckUnknownRule(t *testing.T) {
	if code, _, _ := run("check", "-rule", "X99"); code != ExitUsage {
		t.Errorf("exit code is %d, want %d", code, ExitUsage)
	}
}
