package agent_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/workspace"
)

func TestOnChanges(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
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
	ws, err := workspace.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()

	calls := 0
	var fail error
	goal := agent.OnChanges(ws, func(_ context.Context, changes []workspace.Change) (bool, string, error) {
		calls++
		if fail != nil {
			return false, "", fail
		}
		return len(changes) > 0, "no change", nil
	})
	step := func(text string, done bool, want int) {
		t.Helper()
		if text != "" {
			if err := ws.Write("a.txt", []byte(text)); err != nil {
				t.Fatal(err)
			}
		}
		d, why, err := goal(ctx)
		if err != nil || d != done || calls != want {
			t.Fatalf("the goal gives %v, %q, %v after %d checks; want %v after %d checks", d, why, err, calls, done, want)
		}
	}
	step("", false, 1)
	step("", false, 1)
	step("b\n", true, 2)
	step("b\n", true, 2)
	fail = errors.New("the build stopped")
	if err := ws.Write("a.txt", []byte("c\n")); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, _, err := goal(ctx); !errors.Is(err, fail) {
			t.Fatalf("the goal gives the error %v", err)
		}
	}
	// The goal does not keep a check that gave an error, thus it checks again.
	if calls != 4 {
		t.Errorf("the goal checked %d times", calls)
	}
}
