package cli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
)

// agentLean reads each line "def NAME : TYPE := VALUE" or "theorem NAME : TYPE := PROOF" of lean/Math2/T.lean of the tree as a declaration.
type agentLean struct{ tree string }

var agentDeclRe = regexp.MustCompile(`^(def|theorem) (\S+) : (.+?) := (.+)$`)

func (f agentLean) Check(context.Context, string) (*lean.Elaboration, error) {
	return &lean.Elaboration{Messages: []lean.Message{}}, nil
}

func (f agentLean) Extract(_ context.Context, decls []string, _ bool) ([]lean.Record, error) {
	data, err := os.ReadFile(filepath.Join(f.tree, "lean", "Math2", "T.lean"))
	if err != nil {
		return nil, err
	}
	all := map[string]lean.Record{}
	for _, line := range strings.Split(string(data), "\n") {
		if m := agentDeclRe.FindStringSubmatch(line); m != nil {
			r := lean.Record{Decl: m[2], Kind: m[1], Module: "Math2.T", Type: "c " + m[3] + "\n", Axioms: []string{}}
			if m[3] == "Prop" {
				r.Type, r.ValueHash = "s 0\n", lean.Hash(m[4])
			}
			all[r.Decl] = r
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

func agentNode(decl string) string {
	return "---\ntag: 7K3QX2\nkind: statement\nname: Reflexivity\nformal:\n  - system: lean4\n    library: math2\n    decl: " + decl + "\n    match: exact\n---\n\n## Statement\n\nEach natural number is equal to itself.\n"
}

const agentLeanFile = "module\n\n@[m2 \"7K3QX2\"]\ndef Math2.T.s : Prop := ∀ n : Nat, n = n\n"

func TestAgentRun(t *testing.T) {
	out := t.TempDir()
	if _, err := build.Run(context.Background(), build.Options{Root: valid, Out: out}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for name, text := range map[string]string{
		"nodes/7K/7K3QX2.md": agentNode("Math2.T.s"),
		"lean/Math2.lean":    "module\n\npublic import Math2.T\n",
		"lean/Math2/T.lean":  agentLeanFile,
	} {
		p := filepath.Join(root, filepath.FromSlash(name))
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
		if b, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, b)
		}
	}

	u := llm.Usage{Input: 1000, Output: 200}
	write := func(path, text string) *llm.Response {
		return llmtest.Call("write_file", map[string]string{"path": path, "text": text}, u)
	}
	proved := "module\n\ndef Math2.T.s : Prop := ∀ n : Nat, n = n\n\n@[m2 \"7K3QX2\"]\ntheorem Math2.T.t : Math2.T.s := fun _ => rfl\n"
	oldModel, oldLean := newModel, newLean
	defer func() { newModel, newLean = oldModel, oldLean }()
	newModel = func() (llm.Model, error) {
		return llmtest.New(write("lean/Math2/T.lean", proved), write("nodes/7K/7K3QX2.md", agentNode("Math2.T.t"))), nil
	}
	newLean = func(_ context.Context, tree string) (tools.Lean, error) { return agentLean{tree}, nil }

	work := filepath.Join(t.TempDir(), "w")
	store := t.TempDir()
	code, stdout, stderr := run("agent", "run", "-root", root, "-out", out, "-store", store, "-work", work, "-target", "7K3QX2", "-on-behalf-of", "github:octocat", "-json", "prove")
	if code != ExitOK {
		t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var sum struct {
		Run      string   `json:"run"`
		Outcome  string   `json:"outcome"`
		WorkTree string   `json:"work_tree"`
		Changes  []string `json:"changes"`
		Turns    int      `json:"turns"`
	}
	if err := json.Unmarshal([]byte(stdout), &sum); err != nil {
		t.Fatal(err)
	}
	if sum.Outcome != agent.Success || sum.Turns != 2 || sum.WorkTree != work || strings.Join(sum.Changes, " ") != "lean/Math2/T.lean nodes/7K/7K3QX2.md" {
		t.Errorf("the summary is %+v", sum)
	}
	if _, err := os.Stat(filepath.Join(store, agent.RecordsDir, sum.Run+".json")); err != nil {
		t.Errorf("the run record: %v", err)
	}

	for _, c := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"agent"}, ExitUsage, "Usage: m2 agent run"},
		{[]string{"agent", "run", "explore"}, ExitUsage, "the role explore is not written yet"},
		{[]string{"agent", "run", "-root", root, "-out", out, "-target", "7K3QX2", "-on-behalf-of", "", "prove"}, ExitFailure, "not a person, a project or a schedule"},
		{[]string{"agent", "run", "-root", root, "-out", t.TempDir(), "-target", "7K3QX2", "-on-behalf-of", "github:octocat", "prove"}, ExitFailure, "Run m2 build first"},
	} {
		code, _, stderr := run(c.args...)
		if code != c.code || !strings.Contains(stderr, c.want) {
			t.Errorf("m2 %s: exit code %d, stderr %q", strings.Join(c.args, " "), code, stderr)
		}
	}
}
