package cli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/autoform"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/prov"
)

// autoformLean reads each line "def NAME : TYPE := VALUE" or "theorem NAME : TYPE := PROOF" of each file in lean/Math2/ of the tree as a declaration, as agentLean does for one file.
type autoformLean struct{ tree string }

func (f autoformLean) Check(context.Context, string) (*lean.Elaboration, error) {
	return &lean.Elaboration{Messages: []lean.Message{}}, nil
}

func (f autoformLean) Extract(_ context.Context, decls []string, _ bool) ([]lean.Record, error) {
	files, err := filepath.Glob(filepath.Join(f.tree, "lean", "Math2", "*.lean"))
	if err != nil {
		return nil, err
	}
	all := map[string]lean.Record{}
	for _, file := range files {
		data, err := os.ReadFile(file) // #nosec G304 -- the file is in the test tree.
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			if m := agentDeclRe.FindStringSubmatch(line); m != nil {
				r := lean.Record{Decl: m[2], Kind: m[1], Type: "c " + m[3] + "\n", Axioms: []string{}}
				if m[3] == "Prop" {
					r.Type, r.Value, r.ValueHash = "s 0\n", m[4], lean.Hash(m[4])
				}
				r.TypeHash = lean.Hash(r.Type)
				all[r.Decl] = r
			}
		}
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

// candidateNode is the node 7K3QX2 of agentTree with a second binding to the declaration.
func candidateNode(decl string) string {
	return strings.Replace(agentNode("Math2.T.s"), "---\n\n", "  - system: lean4\n    library: math2\n    decl: "+decl+"\n    match: exact\n---\n\n", 1)
}

// autoformModel replaces the model and the Lean environment. The model writes one candidate for each statement, in order: a Lean file with the statement, then the binding in the node. A statement "" is a run that reports that it is stuck.
func autoformModel(t *testing.T, statements ...string) {
	t.Helper()
	u := llm.Usage{Input: 1000, Output: 200}
	var script []*llm.Response
	for i, s := range statements {
		if s == "" {
			script = append(script, llmtest.Call("report_stuck", map[string]string{"reason": "the statement is ambiguous."}, u))
			continue
		}
		name := "F" + string(rune('1'+i))
		decl := "Math2." + name + ".s"
		lean := "module\n\n@[m2 \"7K3QX2\"]\ndef " + decl + " : Prop := " + s + "\n"
		script = append(script,
			llmtest.Call("write_file", map[string]string{"path": "lean/Math2/" + name + ".lean", "text": lean}, u),
			llmtest.Call("write_file", map[string]string{"path": "nodes/7K3/7K3QX2.md", "text": candidateNode(decl)}, u))
	}
	oldModel, oldLean := newModel, newLean
	newModel = func() (llm.Model, error) { return llmtest.New(script...), nil }
	newLean = func(_ context.Context, tree string, _ *tools.Box) (tools.Lean, error) { return autoformLean{tree}, nil }
	t.Cleanup(func() { newModel, newLean = oldModel, oldLean })
}

// gitIdentity gives the commits of the test an author, without a change to HOME.
func gitIdentity(t *testing.T) {
	t.Helper()
	for _, k := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
		t.Setenv(k+"_NAME", "test")
		t.Setenv(k+"_EMAIL", "test@example.com")
	}
}

func TestAutoform(t *testing.T) {
	t.Setenv("M2_LEAN_IMAGE", "")
	gitIdentity(t)
	root, out := agentTree(t)
	autoformModel(t, "∀ n : Nat, n = n", "∀ n : Nat, n = n", "∀ n : Nat, n + 0 = n")
	var pulls []string
	oldPull := openPull
	openPull = func(_ context.Context, dir, branch, title, body string) (string, error) {
		pulls = append(pulls, branch+" "+title)
		return "https://example.com/pull/1", nil
	}
	t.Cleanup(func() { openPull = oldPull })

	store := t.TempDir()
	work := filepath.Join(t.TempDir(), "w")
	code, stdout, stderr := run("autoform", "-root", root, "-out", out, "-store", store, "-work", work, "-rungs", "0,0,1", "-on-behalf-of", "github:octocat", "-no-sandbox", "-pr", "7K3QX2")
	if code != ExitOK {
		t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	for _, want := range []string{"3 candidates, 3 pass the battery, 2 groups", "the winner is c1, Math2.F1.s at F0", "split into 2 groups", "the pull request is https://example.com/pull/1"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout has no %q:\n%s", want, stdout)
		}
	}
	if len(pulls) != 1 || pulls[0] != "autoform/7K3QX2 formal: bind 7K3QX2 to Math2.F1.s with m2 autoform" {
		t.Errorf("the pull requests are %q", pulls)
	}

	files, err := filepath.Glob(filepath.Join(store, autoformDir, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("the results are %v, %v", files, err)
	}
	data, err := os.ReadFile(files[0]) // #nosec G304 -- the file is in the test store.
	if err != nil {
		t.Fatal(err)
	}
	var res autoform.Result
	if err := json.Unmarshal(data, &res); err != nil {
		t.Fatal(err)
	}
	if res.Winner != 0 || len(res.Groups) != 2 || len(res.Groups[0].Candidates) != 2 || res.Candidates[2].Decl != "Math2.F3.s" || res.Candidates[0].Model != "claude-sonnet-5-5" || res.Candidates[2].Model != "claude-opus-5-5" {
		t.Errorf("the result is %+v", res)
	}
	md, err := os.ReadFile(strings.TrimSuffix(files[0], ".json") + ".md") // #nosec G304 -- as above.
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "def Math2.F3.s : Prop := ∀ n : Nat, n + 0 = n") || strings.Contains(string(md), "def Math2.F2.s") {
		t.Errorf("the dissenting candidates of the report are wrong:\n%s", md)
	}

	// The branch has the changes of the winner and its provenance lines, which name its run.
	show := func(args ...string) string {
		b, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return string(b)
	}
	if got := show("diff", "--name-only", "HEAD", "autoform/7K3QX2"); got != "lean/Math2/F1.lean\nnodes/7K3/7K3QX2.md\n"+prov.Path("7K3QX2")+"\n" {
		t.Errorf("the branch changes %q", got)
	}
	lines, err := prov.Parse(strings.NewReader(show("show", "autoform/7K3QX2:"+prov.Path("7K3QX2"))))
	if err != nil || len(lines) == 0 {
		t.Fatalf("the provenance lines are %v, %v", lines, err)
	}
	for _, l := range lines {
		if l.Run != res.Candidates[0].Run || l.Agent != "m2-agent/formalize@0.1.0" || l.Model != "claude-sonnet-5-5" {
			t.Errorf("the line %+v does not name the run of the winner", l)
		}
	}
	if _, err := os.Stat(filepath.Join(store, prov.RunsDir, res.Candidates[0].Run+".json")); err != nil {
		t.Errorf("the summary of the run of the winner: %v", err)
	}
}

func TestAutoformNoWinner(t *testing.T) {
	t.Setenv("M2_LEAN_IMAGE", "")
	root, out := agentTree(t)
	autoformModel(t, "")
	store := t.TempDir()
	code, stdout, stderr := run("autoform", "-root", root, "-out", out, "-store", store, "-work", filepath.Join(t.TempDir(), "w"), "-rungs", "0", "-on-behalf-of", "github:octocat", "-no-sandbox", "7K3QX2")
	if code != ExitFailure || !strings.Contains(stdout, "no candidate passes the battery") || !strings.Contains(stdout, "stuck: the statement is ambiguous.") {
		t.Errorf("exit code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if b, err := exec.Command("git", "-C", root, "branch", "--list", "autoform/*").Output(); err != nil || len(b) != 0 {
		t.Errorf("the branches are %q, %v", b, err)
	}

	for _, c := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"autoform"}, ExitUsage, "Usage: m2 autoform"},
		{[]string{"autoform", "-root", root, "nothing"}, ExitFailure, "is not a tag"},
		{[]string{"autoform", "-root", root, "7K3QX3"}, ExitFailure, "the tree has no node 7K3QX3"},
		{[]string{"autoform", "-root", root, "-rungs", "0,9", "7K3QX2"}, ExitFailure, "the rung \"9\" is not a rung"},
		{[]string{"autoform", "-root", root, "-out", out, "7K3QX2"}, ExitFailure, "give the image of the Lean sandbox"},
		{[]string{"autoform", "-root", root, "-out", t.TempDir(), "-no-sandbox", "7K3QX2"}, ExitFailure, "Run m2 build first"},
	} {
		code, _, stderr := run(c.args...)
		if code != c.code || !strings.Contains(stderr, c.want) {
			t.Errorf("m2 %s: exit code %d, stderr %q", strings.Join(c.args, " "), code, stderr)
		}
	}
}
