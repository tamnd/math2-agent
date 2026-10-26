package formalize_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/formalize"
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

// fakeLean reads each line "def NAME : TYPE := VALUE" or "theorem NAME : TYPE := PROOF" of the Lean files of the tree as a declaration. With checks, a statement with the value False is refuted.
type fakeLean struct {
	ws     *workspace.Workspace
	checks []bool
}

var declRe = regexp.MustCompile(`^(def|theorem) (\S+) : (.+?) := (.+)$`)

func (f *fakeLean) Check(context.Context, string) (*lean.Elaboration, error) {
	return &lean.Elaboration{Messages: []lean.Message{}}, nil
}

func (f *fakeLean) Extract(_ context.Context, decls []string, checks bool) ([]lean.Record, error) {
	f.checks = append(f.checks, checks)
	all := map[string]lean.Record{}
	err := filepath.WalkDir(filepath.Join(f.ws.Dir, "lean", "Math2"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path) // #nosec G304 -- the path is in the temporary tree of the test.
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(data), "\n") {
			m := declRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			r := lean.Record{Decl: m[2], Kind: m[1], Type: "c " + m[3] + "\n", Axioms: []string{}}
			if m[3] == "Prop" {
				r.Type = "s 0\n"
			}
			if strings.Contains(m[4], "sorry") {
				r.Axioms = append(r.Axioms, "sorryAx")
			}
			if checks && r.Statement() {
				r.Checks = &lean.Checks{}
				if m[4] == "False" {
					r.Checks.Refuted = "decide"
				}
			}
			all[r.Decl] = r
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var out []lean.Record
	for _, d := range decls {
		r, ok := all[d]
		if !ok {
			r = lean.Record{Decl: d, Error: "unknown declaration"}
		}
		out = append(out, r)
	}
	return out, nil
}

const nodePath = "nodes/7K3/7K3QX2.md"

func node(kind, formal string) string {
	return "---\ntag: 7K3QX2\nkind: " + kind + "\nname: Reflexivity\n" + formal + "---\n\n## Statement\n\nEach natural number is equal to itself.\n"
}

const binding = "formal:\n  - system: lean4\n    library: math2\n    decl: Math2.R.refl_statement\n    match: exact\n"

func job() *agent.Job {
	return &agent.Job{ID: "j-2026-10-26-2", Role: "formalize", Target: "7K3QX2", Goal: "the node has a Lean binding", OnBehalfOf: "github:octocat"}
}

func change(before, after string) []workspace.Change {
	return []workspace.Change{{Path: nodePath, Old: []byte(before), New: []byte(after)}}
}

func TestAdded(t *testing.T) {
	bare := node("statement", "")
	got, kind, err := formalize.Added("7K3QX2", change(bare, node("statement", binding)))
	if err != nil || kind != "statement" || len(got) != 1 || got[0] != (formalize.Binding{Decl: "Math2.R.refl_statement", Library: "math2", Match: "exact"}) {
		t.Errorf("Added gives %+v, %q, %v", got, kind, err)
	}
	for _, c := range []struct {
		name    string
		changes []workspace.Change
		want    string
	}{
		{"no change", nil, "has not appended"},
		{"no new binding", change(bare, bare), "has not appended"},
		{"another system", change(bare, node("statement", strings.Replace(binding, "lean4", "coq", 1))), "the system coq"},
		{"no decl", change(bare, node("statement", "formal:\n  - system: lean4\n    match: exact\n")), "formal[0] has no decl"},
		{"a bad match", change(bare, node("statement", strings.Replace(binding, "exact", "close", 1))), `the match "close"`},
	} {
		if _, _, err := formalize.Added("7K3QX2", c.changes); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: Added gives %v, and it must give %q", c.name, err, c.want)
		}
	}
}

func TestVerify(t *testing.T) {
	stmt := lean.Record{Decl: "Math2.R.s", Kind: "def", Type: "s 0\n", Axioms: []string{"propext"}, Checks: &lean.Checks{}}
	with := func(edit func(*lean.Record)) lean.Record {
		r := stmt
		r.Checks = &lean.Checks{}
		edit(&r)
		return r
	}
	for _, c := range []struct {
		name string
		kind string
		rec  lean.Record
		want string
	}{
		{"a statement", "statement", stmt, ""},
		{"a theorem", "statement", with(func(r *lean.Record) { r.Kind, r.Type = "theorem", "c True\n" }), ""},
		{"a definition", "definition", with(func(r *lean.Record) { r.Type = "c Nat\n" }), ""},
		{"an unknown name", "statement", lean.Record{Decl: "Math2.R.s", Error: "unknown declaration"}, "import its module"},
		{"a def of Nat", "statement", with(func(r *lean.Record) { r.Type = "c Nat\n" }), "binds to a statement"},
		{"a theorem for a definition", "definition", with(func(r *lean.Record) { r.Kind = "theorem" }), "binds to a def"},
		{"sorry", "statement", with(func(r *lean.Record) { r.Axioms = []string{"sorryAx"} }), "the axioms sorryAx"},
		{"native_decide", "statement", with(func(r *lean.Record) { r.NativeDecideAxioms = []string{"x"} }), "uses native_decide"},
		{"vacuous", "statement", with(func(r *lean.Record) { r.Checks.Vacuous = "simp" }), "contradictory"},
		{"refuted", "statement", with(func(r *lean.Record) { r.Checks.Refuted = "decide" }), "decide proves its negation"},
	} {
		err := formalize.Verify(c.kind, []lean.Record{c.rec})
		if (c.want == "") != (err == nil) || err != nil && !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: Verify gives %v, and it must give %q", c.name, err, c.want)
		}
	}
}

var u = llm.Usage{Input: 1000, Output: 200}

func write(path, text string) *llm.Response {
	return llmtest.Call("write_file", map[string]string{"path": path, "text": text}, u)
}

func TestRun(t *testing.T) {
	ctx := context.Background()
	ws := tree(t, map[string]string{
		nodePath:            node("statement", ""),
		"lean/Math2.lean":   "module\n\npublic import Math2.T\n",
		"lean/Math2/T.lean": "module\n",
	})
	env := &fakeLean{ws: ws}
	role, err := agent.LoadRole(prompts.FS, "formalize")
	if err != nil {
		t.Fatal(err)
	}
	j := job()
	brief, err := formalize.Brief(ws, j)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"a statement node", `@[m2 "7K3QX2"]`, `<data source="nodes/7K3/7K3QX2.md" licence="CC-BY-SA-4.0">`} {
		if !strings.Contains(brief, want) {
			t.Errorf("the brief has no %q:\n%s", want, brief)
		}
	}
	ts, err := tools.Profile(role, nil, ws, env)
	if err != nil {
		t.Fatal(err)
	}
	rn := agent.Runner{Role: role, Model: llmtest.New(
		write("lean/Math2/R.lean", "module\n\ndef Math2.R.refl_statement : Prop := False\n"),
		write("lean/Math2.lean", "module\n\npublic import Math2.T\npublic import Math2.R\n"),
		write(nodePath, node("statement", binding)),
		write("lean/Math2/R.lean", "module\n\ndef Math2.R.refl_statement : Prop := ∀ n : Nat, n = n\n"),
	), Tools: ts, Goal: formalize.Goal(ws, env, j)}
	rec, err := rn.Run(ctx, j, 0, brief)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Outcome != agent.Success || len(rec.Turns) != 4 {
		t.Fatalf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
	}
	for i, want := range []string{"has not appended", "has not appended", "decide proves its negation"} {
		if !strings.Contains(rec.Turns[i].Why, want) {
			t.Errorf("after turn %d the goal says %q, and it must say %q", i+1, rec.Turns[i].Why, want)
		}
	}
	if len(env.checks) != 2 || !env.checks[0] || !env.checks[1] {
		t.Errorf("the goal extracted with the checks %v", env.checks)
	}
}
