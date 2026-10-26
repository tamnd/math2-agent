package bump_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/bump"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/workspace"
)

// tree makes a git repository with the files in one commit and opens it as a workspace.
func tree(t *testing.T, files map[string]string) *workspace.Workspace {
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
	w, err := workspace.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

// fakeLean is the pinned Mathlib of the tests. A declaration that it does not have does not resolve.
type fakeLean map[string]lean.Record

func (f fakeLean) Check(context.Context, string) (*lean.Elaboration, error) {
	return &lean.Elaboration{Messages: []lean.Message{}}, nil
}

func (f fakeLean) Extract(_ context.Context, decls []string, _ bool) ([]lean.Record, error) {
	var out []lean.Record
	for _, d := range decls {
		r, ok := f[d]
		if !ok {
			r = lean.Record{Decl: d, Error: "unknown declaration"}
		}
		out = append(out, r)
	}
	return out, nil
}

var mathlib = fakeLean{
	"Subgroup.card_subgroup_dvd_card": {Decl: "Subgroup.card_subgroup_dvd_card", Kind: "theorem", Deprecated: &lean.Deprecation{NewName: "Subgroup.card_dvd_card", Since: "2026-09-01"}},
	"Subgroup.card_dvd_card":          {Decl: "Subgroup.card_dvd_card", Kind: "theorem"},
	"Subgroup.index_def":              {Decl: "Subgroup.index_def", Kind: "def"},
	"Math2.Algebra.lagrange":          {Decl: "Math2.Algebra.lagrange", Kind: "theorem"},
}

const lagrangePath = "nodes/3C/3C53TD.md"

func lagrange(decl string) string {
	return "---\ntag: 3C53TD\nkind: statement\nrole: theorem\nname: Lagrange's theorem\nformal:\n  - system: lean4\n    library: mathlib\n    decl: " + decl + "\n    match: exact\n  - system: lean4\n    library: math2\n    decl: Math2.Algebra.lagrange\n    match: exact\n---\n\n## Statement\n\nThe order of a subgroup of a finite group divides the order of the group.\n"
}

func job() *agent.Job {
	return &agent.Job{ID: "j-2026-10-26-0001", Role: "bump", Target: "3C53TD", Goal: "each Lean binding of the node resolves and is not deprecated", OnBehalfOf: "github:octocat"}
}

var u = llm.Usage{Input: 1000, Output: 200}

func rebind(decl, to string) *llm.Response {
	return llmtest.Call("rebind", map[string]string{"decl": decl, "new": to, "why": "the deprecation names the new declaration."}, u)
}

func change(old, cur string) []workspace.Change {
	return []workspace.Change{{Path: lagrangePath, Old: []byte(lagrange(old)), New: []byte(lagrange(cur))}}
}

func TestRebinds(t *testing.T) {
	rbs, err := bump.Rebinds("3C53TD", change("Subgroup.card_subgroup_dvd_card", "Subgroup.card_dvd_card"))
	if err != nil || len(rbs) != 1 || rbs[0] != (bump.Rebinding{Index: 0, Old: "Subgroup.card_subgroup_dvd_card", New: "Subgroup.card_dvd_card"}) {
		t.Errorf("Rebinds gives %+v, %v", rbs, err)
	}
	isabelle := strings.Replace(lagrange("Subgroup.card_subgroup_dvd_card"), "system: lean4\n    library: mathlib", "system: isabelle\n    library: hol", 1)
	c := []workspace.Change{{Path: lagrangePath, Old: []byte(isabelle), New: []byte(strings.Replace(isabelle, "card_subgroup_dvd_card", "card_dvd_card", 1))}}
	if _, err := bump.Rebinds("3C53TD", c); err == nil || !strings.Contains(err.Error(), "not a Lean 4 binding") {
		t.Errorf("an Isabelle binding gives %v", err)
	}
	if rbs, err := bump.Rebinds("AA0001", change("a", "b")); err != nil || rbs != nil {
		t.Errorf("another target gives %+v, %v", rbs, err)
	}
}

func TestAutoMerge(t *testing.T) {
	recs, err := mathlib.Extract(context.Background(), []string{"Subgroup.card_subgroup_dvd_card"}, false)
	if err != nil {
		t.Fatal(err)
	}
	other := workspace.Change{Path: "nodes/AA/AA0001.md", Old: []byte("a"), New: []byte("b")}
	for _, c := range []struct {
		name    string
		changes []workspace.Change
		want    bool
	}{
		{"the deprecation", change("Subgroup.card_subgroup_dvd_card", "Subgroup.card_dvd_card"), true},
		{"another declaration", change("Subgroup.card_subgroup_dvd_card", "Subgroup.index_def"), false},
		{"another file", append(change("Subgroup.card_subgroup_dvd_card", "Subgroup.card_dvd_card"), other), false},
		{"no rebind", change("Subgroup.card_subgroup_dvd_card", "Subgroup.card_subgroup_dvd_card"), false},
		{"no change", nil, false},
	} {
		if got, err := bump.AutoMerge("3C53TD", c.changes, recs); err != nil || got != c.want {
			t.Errorf("%s: AutoMerge gives %v, %v", c.name, got, err)
		}
	}
}

func TestBrief(t *testing.T) {
	ws := tree(t, map[string]string{lagrangePath: lagrange("Subgroup.card_subgroup_dvd_card")})
	brief, err := bump.Brief(context.Background(), ws, mathlib, job())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Job j-2026-10-26-0001: rebind the target 3C53TD, a statement node, after the bump of Mathlib.",
		"- formal[0] Subgroup.card_subgroup_dvd_card: it is deprecated, and the deprecation names Subgroup.card_dvd_card.\n",
		"- formal[1] Math2.Algebra.lagrange: it resolves, and it is a theorem.\n",
		`<data source="` + lagrangePath + `"`,
	} {
		if !strings.Contains(brief, want) {
			t.Errorf("the brief has no %q:\n%s", want, brief)
		}
	}
	ws = tree(t, map[string]string{lagrangePath: "---\ntag: 3C53TD\nkind: statement\nname: Lagrange's theorem\n---\n\n## Statement\n\nThe order.\n"})
	if _, err := bump.Brief(context.Background(), ws, mathlib, job()); err == nil || !strings.Contains(err.Error(), "has no Lean binding") {
		t.Errorf("a node with no binding gives %v", err)
	}
}

// run does one bump run on the target with the scripted model.
func run(t *testing.T, decl string, model ...*llm.Response) *agent.Record {
	t.Helper()
	ws := tree(t, map[string]string{lagrangePath: lagrange(decl)})
	role, err := agent.LoadRole(prompts.FS, "bump")
	if err != nil {
		t.Fatal(err)
	}
	j := job()
	brief, err := bump.Brief(context.Background(), ws, mathlib, j)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tools.Profile(role, nil, ws, mathlib, bump.Rebind(ws))
	if err != nil {
		t.Fatal(err)
	}
	rn := agent.Runner{Role: role, Model: llmtest.New(model...), Tools: ts, Goal: bump.Goal(ws, mathlib, j)}
	rec, err := rn.Run(context.Background(), j, 0, brief)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestRunDeprecated(t *testing.T) {
	rec := run(t, "Subgroup.card_subgroup_dvd_card",
		rebind("Subgroup.card_subgroup_dvd_card", "Subgroup.card_subgroup_dvd_card"),
		rebind("Subgroup.card_subgroup_dvd_card", "Subgroup.index_def"),
		rebind("Subgroup.index_def", "Subgroup.card_dvd_card"),
	)
	if rec.Outcome != agent.Success || len(rec.Turns) != 3 {
		t.Fatalf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
	}
	if r := rec.Turns[0].Results[0]; !r.IsError || !strings.Contains(r.Content, "the new declaration is the declaration of the binding") {
		t.Errorf("a rebind to the same declaration gives %+v", r)
	}
	if got := rec.Turns[0].Why; !strings.Contains(got, "the run changed no binding") {
		t.Errorf("after no change the goal says %q", got)
	}
	if got := rec.Turns[1].Why; !strings.Contains(got, "Subgroup.card_subgroup_dvd_card is a theorem and Subgroup.index_def is a def") {
		t.Errorf("a def for a theorem gives %q", got)
	}
	if r := rec.Turns[2].Results[0]; r.IsError || r.Content != "Changed the binding of 3C53TD from Subgroup.index_def to Subgroup.card_dvd_card." {
		t.Errorf("the rebind gives %+v", r)
	}
}

func TestRunLost(t *testing.T) {
	rec := run(t, "Subgroup.card_dvd_of_le",
		rebind("Subgroup.card_dvd", "Subgroup.card_dvd_card"),
		rebind("Subgroup.card_dvd_of_le", "Subgroup.card_dvd_of_subgroup"),
		rebind("Math2.Algebra.lagrange", "Subgroup.card_dvd_card"),
		llmtest.Call("report_stuck", map[string]string{"reason": "No declaration of Mathlib states the theorem."}, u),
	)
	if rec.Outcome != agent.Failure || !strings.Contains(rec.Reason, "No declaration of Mathlib") {
		t.Fatalf("the run gives %s, %q", rec.Outcome, rec.Reason)
	}
	if r := rec.Turns[0].Results[0]; !r.IsError || !strings.Contains(r.Content, "the target has no Lean binding of Subgroup.card_dvd. Its Lean bindings are Subgroup.card_dvd_of_le, Math2.Algebra.lagrange") {
		t.Errorf("a rebind of another declaration gives %+v", r)
	}
	if got := rec.Turns[1].Why; !strings.Contains(got, "the binding of Subgroup.card_dvd_of_subgroup still fails: it does not resolve: unknown declaration") {
		t.Errorf("a new declaration that does not resolve gives %q", got)
	}
	if got := rec.Turns[2].Why; !strings.Contains(got, "Math2.Algebra.lagrange resolves and is not deprecated, thus bump does not change formal[1]") {
		t.Errorf("a rebind of a binding that resolves gives %q", got)
	}
}
