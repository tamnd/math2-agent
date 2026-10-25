package replay

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
)

var (
	buildOnce sync.Once
	buildErr  error
)

const fixture = "testdata/fixture"

// checker builds the fixture project and returns a Checker for it. The test skips when lake is not on the PATH. On Linux the checkers run in the sandbox, thus bwrap must be on the PATH.
func checker(t *testing.T, paranoid bool) *Checker {
	t.Helper()
	if _, err := exec.LookPath("lake"); err != nil {
		t.Skip("lake is not on the PATH")
	}
	buildOnce.Do(func() {
		cmd := exec.Command("lake", "build")
		cmd.Dir = fixture
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("lake build: %w\n%s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	c, err := New(context.Background(), fixture, Options{Paranoid: paranoid, NoSandbox: runtime.GOOS != "linux", Work: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("the check runs lake comparator")
	}
	c := checker(t, true)
	tests := []struct {
		name   string
		target Target
		passed bool
		reason string
	}{
		{"complete", Target{Decl: "Fixture.add_comm", Statement: "∀ (a b : Nat), a + b = b + a"}, true, ""},
		{"universe", Target{Decl: "Fixture.refl", LevelParams: []string{"u"}, Statement: "∀ (α : Sort u) (a : α), a = a"}, true, ""},
		{"standard axioms", Target{Decl: "Fixture.em", Statement: "∀ (p : Prop), p ∨ ¬p"}, true, ""},
		{"sorry", Target{Decl: "Fixture.incomplete", Statement: "∀ (a b : Nat), a * b = b * a"}, false, "sorryAx"},
		{"changed statement", Target{Decl: "Fixture.add_comm", Statement: "∀ (a b : Nat), a + b = a + b"}, false, "lake comparator rejects"},
		{"statement that does not elaborate", Target{Decl: "Fixture.add_comm", Statement: "∀ (a b : Nat), a + b = c"}, false, "does not elaborate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.target.Module = "Fixture.Basic"
			res, err := c.Check(context.Background(), tt.target)
			if err != nil {
				t.Fatal(err)
			}
			if res.Passed != tt.passed || !strings.Contains(res.Reason, tt.reason) {
				t.Fatalf("Check gives passed %v and the reason %q, and it must give passed %v and a reason with %q", res.Passed, res.Reason, tt.passed, tt.reason)
			}
			if !tt.passed {
				return
			}
			for _, k := range []string{"leanchecker", "leanchecker-paranoid", "nanoda_bin"} {
				if !slices.Contains(res.Kernels, k) {
					t.Errorf("the kernels %v do not have %s", res.Kernels, k)
				}
			}
		})
	}
}

func TestCheckWithoutParanoid(t *testing.T) {
	if testing.Short() {
		t.Skip("the check runs lake comparator")
	}
	c := checker(t, false)
	res, err := c.Check(context.Background(), Target{Decl: "Fixture.add_comm", Module: "Fixture.Basic", Statement: "∀ (a b : Nat), a + b = b + a"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed || !slices.Equal(res.Kernels, []string{"leanchecker"}) {
		t.Errorf("Check gives passed %v and the kernels %v. It must pass with the Lean kernel only", res.Passed, res.Kernels)
	}
}

func TestCheckErrors(t *testing.T) {
	c := checker(t, false)
	for _, target := range []Target{
		{Decl: "Fixture.add_comm", Module: "Fixture.Basic"},
		{Decl: "Fixture.add_comm", Module: "Fixture.Absent", Statement: "True"},
	} {
		if _, err := c.Check(context.Background(), target); err == nil {
			t.Errorf("Check(%+v) gives no error", target)
		}
	}
}

func TestHeader(t *testing.T) {
	src := `/-
Copyright. This is a comment.
import NotAnImport
-/
module -- a comment

prelude
public import A.B
meta import C
public meta import all D
import E

/-! The module doc. -/
import F
`
	got, err := header(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"A.B", "C", "D", "E"}; !slices.Equal(got, want) {
		t.Errorf("header gives %v, and it must give %v", got, want)
	}
}

func TestSource(t *testing.T) {
	got := source(Target{Decl: "X.y", LevelParams: []string{"u", "v"}, Statement: "True"}, []string{"A", "B"})
	for _, want := range []string{"import A\nimport B\n", "theorem X.y.{u, v} :\nTrue :=\n  sorry\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("the challenge\n%s\ndoes not have\n%s", got, want)
		}
	}
}
