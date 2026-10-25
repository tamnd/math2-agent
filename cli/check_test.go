package cli

import (
	"encoding/json"
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
