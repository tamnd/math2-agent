package explain_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/explain"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/prov"
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

const (
	nodePath  = "nodes/3C/3C53TD.md"
	groupPath = "nodes/G5/G5S1M9.md"
	target    = "---\ntag: 3C53TD\nkind: statement\nrole: theorem\nname: Lagrange's theorem\nuses: [G5S1M9]\n---\n\n## Statement\n\nLet $G$ be a finite [[group|G5S1M9]] and let $H$ be a subgroup of $G$. Then the order of $H$ divides the order of $G$.\n"
	group     = "---\ntag: G5S1M9\nkind: definition\nname: Group\n---\n\n## Definition\n\nA group is a set with an associative operation, an identity and inverses.\n"
	other     = "---\ntag: 7K3QX2\nkind: definition\nname: Ring\n---\n\n## Definition\n\nA ring is a set with two operations.\n"
	curricula = "curriculum: seed\ncourses:\n  - course: algebra\n    year: 1\n    items:\n      - {name: group, kind: definition, node: G5S1M9}\n  - course: rings\n    year: 3\n    items:\n      - {name: ring, kind: definition, node: 7K3QX2}\n"
)

func explained(level, text string) string {
	return target + "\n## Explanations\n\n### " + level + " agent\n\n" + text + "\n"
}

func job(goal string) *agent.Job {
	return &agent.Job{ID: "j-2026-10-26-3", Role: "explain", Target: "3C53TD", Goal: goal, OnBehalfOf: "github:octocat"}
}

func TestSentences(t *testing.T) {
	for text, want := range map[string][]string{
		"":                       nil,
		"One. Two!\n\nThree":     {"One.", "Two!", "Three"},
		"Let $x = 1.5$. Then x.": {"Let $x = 1.5$.", "Then x."},
		"See cf. the proof.":     {"See cf. the proof."},
		"A line\nthat goes on.":  {"A line that goes on."},
	} {
		if got := explain.Sentences(text); !slices.Equal(got, want) {
			t.Errorf("Sentences(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestLevelsAndCited(t *testing.T) {
	if got := explain.Levels("an L2 and an L1 explanation, and L2 again, not L5 or XL1"); !slices.Equal(got, []string{"L1", "L2"}) {
		t.Errorf("Levels gives %v", got)
	}
	if got := explain.Cited("A [[group|G5S1M9]], a [[ring | 7K3QX2]], the [[group|G5S1M9]] and `[[code|3C53TD]]`."); !slices.Equal(got, []string{"G5S1M9", "7K3QX2"}) {
		t.Errorf("Cited gives %v", got)
	}
}

func TestCurricula(t *testing.T) {
	got, err := explain.Curricula(fstest.MapFS{"topics/curricula/seed.yaml": {Data: []byte(curricula)}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got["L1"], []string{"G5S1M9"}) || !slices.Equal(got["L2"], []string{"G5S1M9"}) || got["L3"] != nil {
		t.Errorf("Curricula gives %v", got)
	}
	if _, err := explain.Curricula(fstest.MapFS{"topics/curricula/bad.yaml": {Data: []byte("courses: 1\n")}}); err == nil {
		t.Error("Curricula reads a file that is not a curriculum")
	}
}

func TestWritten(t *testing.T) {
	before := explained("L1", "Old text.")
	after := strings.Replace(before, "Old text.", "New text.", 1) + "\n### L2 agent\n\nMore text.\n"
	got, err := explain.Written("3C53TD", []workspace.Change{{Path: nodePath, Old: []byte(before), New: []byte(after)}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []explain.Explanation{{Level: "L1", Text: "New text."}, {Level: "L2", Text: "More text."}}) {
		t.Errorf("Written gives %+v", got)
	}
	if got, _ := explain.Written("3C53TD", []workspace.Change{{Path: nodePath, Old: []byte(before), New: []byte(target)}}); len(got) != 0 {
		t.Errorf("a removed explanation gives %+v", got)
	}
}

var u = llm.Usage{Input: 1000, Output: 200}

func labels(l ...string) *llm.Response {
	var in []map[string]any
	for i, x := range l {
		in = append(in, map[string]any{"sentence": i + 1, "label": x, "why": "the reason " + x + "."})
	}
	return llmtest.Call("label", map[string]any{"labels": in}, u)
}

func TestGround(t *testing.T) {
	judge := llmtest.New(labels(explain.Supported, explain.Unsupported), llmtest.Say("I will not label.", u))
	e := explain.Explanation{Level: "L2", Text: "A [[group|G5S1M9]] has inverses. Every group is finite."}
	got, err := explain.Ground(context.Background(), judge, "claude-haiku-5-5", "Label.", e, map[string]string{"G5S1M9": group})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Label != explain.Supported || got[1].Label != explain.Unsupported || got[1].Sentence != "Every group is finite." {
		t.Errorf("Ground gives %+v", got)
	}
	req := judge.Requests()[0]
	text := req.Messages[0].Content[0].Text
	if req.Model != "claude-haiku-5-5" || req.System != "Label." || len(req.Tools) != 1 || !strings.Contains(text, "2. Every group is finite.") || !strings.Contains(text, `<data source="nodes/G5/G5S1M9.md" licence="CC-BY-SA-4.0">`) {
		t.Errorf("the request is %+v\n%s", req, text)
	}
	if got, err = explain.Ground(context.Background(), judge, "claude-haiku-5-5", "Label.", e, nil); err != nil || got[0].Label != explain.Unsupported || got[1].Label != explain.Unsupported {
		t.Errorf("a check with no labels gives %+v, %v", got, err)
	}
}

func write(path, text string) *llm.Response {
	return llmtest.Call("write_file", map[string]string{"path": path, "text": text}, u)
}

// run runs an explain job on a new tree with the scripted model and judge.
func run(t *testing.T, files map[string]string, goal string, model, judge []*llm.Response) (*agent.Record, string) {
	t.Helper()
	all := map[string]string{nodePath: target, groupPath: group, "nodes/7K/7K3QX2.md": other, "topics/curricula/seed.yaml": curricula}
	for k, v := range files {
		all[k] = v
	}
	ws := tree(t, all)
	role, err := agent.LoadRole(prompts.FS, "explain")
	if err != nil {
		t.Fatal(err)
	}
	j := job(goal)
	brief, err := explain.Brief(ws, j)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tools.Profile(role, nil, ws, nil)
	if err != nil {
		t.Fatal(err)
	}
	meter := &agent.Meter{Model: llmtest.New(judge...)}
	rn := agent.Runner{Role: role, Model: llmtest.New(model...), Tools: ts, Meter: meter, Goal: explain.Goal(ws, j, meter, role.Ladder[0].Model, role.Checks["ground"])}
	rec, err := rn.Run(context.Background(), j, 0, brief)
	if err != nil {
		t.Fatal(err)
	}
	return rec, brief
}

func TestRun(t *testing.T) {
	good := explained("L1", "A [[group|G5S1M9]] has inverses. The theorem is about a finite group.")
	rec, brief := run(t, nil, "an L1 explanation", []*llm.Response{
		write(nodePath, strings.Replace(target, "## Statement", "## Statement\n\nNew text.", 1)),
		write(nodePath, explained("L1", "A group has inverses.")),
		write(nodePath, explained("L1", "A [[ring|7K3QX2]] is a set.")),
		write(nodePath, explained("L1", "A [[group|G5S1M9]] has inverses. Every group is finite.")),
		write(nodePath, good),
	}, []*llm.Response{labels(explain.Supported, explain.Unsupported), labels(explain.Supported, explain.Supported)})
	for _, want := range []string{"An L1 explanation may link only to the target", "these nodes of the curricula: G5S1M9.", `"### L2 agent"`, `<data source="nodes/3C/3C53TD.md"`} {
		if !strings.Contains(brief, want) {
			t.Errorf("the brief has no %q:\n%s", want, brief)
		}
	}
	if rec.Outcome != agent.Success || len(rec.Turns) != 5 {
		t.Fatalf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
	}
	for i, want := range []string{"has not written an explanation", "cites no node", "not in the curricula of L1", `the cited nodes do not support sentence 2 of the L1 explanation ("Every group is finite."): the reason unsupported`, ""} {
		if got := rec.Turns[i].Why; want == "" && got != "" || !strings.Contains(got, want) {
			t.Errorf("after turn %d the goal says %q, and it must say %q", i+1, got, want)
		}
	}
	if len(rec.Turns[3].Checks) != 1 || len(rec.Turns[4].Checks) != 1 || rec.Spend.Tokens["claude-sonnet-5-5"].Input != 7000 {
		t.Errorf("the checks are %d and %d, and the tokens are %v", len(rec.Turns[3].Checks), len(rec.Turns[4].Checks), rec.Spend.Tokens)
	}
}

func TestRunFails(t *testing.T) {
	text := explained("L2", "Every [[group|G5S1M9]] is finite.")
	rec, _ := run(t, nil, "an L2 explanation", []*llm.Response{write(nodePath, text)}, []*llm.Response{labels(explain.Contradicted)})
	if rec.Outcome != agent.Failure || rec.Reason != "the goal check failed: sentence 1 of the L2 explanation is contradicted by the cited nodes: the reason contradicted" {
		t.Errorf("a contradicted sentence gives %s, %q", rec.Outcome, rec.Reason)
	}

	log := `{"field":"body.explanations.L2","op":"confirm","agent":"m2-agent/explain@0.1.0","run":"r-2026-10-20-0001","human":"github:octocat","at":"2026-10-21T10:00:00Z"}` + "\n"
	old := explained("L2", "A [[group|G5S1M9]] has inverses.")
	rec, _ = run(t, map[string]string{nodePath: old, prov.Path("3C53TD"): log}, "an L2 explanation", []*llm.Response{
		write(nodePath, strings.Replace(old, "has inverses", "has an identity", 1)),
		llmtest.Call(agent.StuckTool, map[string]string{"reason": "a person confirmed it"}, u),
	}, nil)
	if !strings.Contains(rec.Turns[0].Why, "github:octocat wrote or confirmed the L2 explanation") {
		t.Errorf("a change of a confirmed explanation gives %q", rec.Turns[0].Why)
	}

	rec, _ = run(t, nil, "an L3 explanation", []*llm.Response{
		write(nodePath, explained("L2", "A [[group|G5S1M9]] has inverses.")),
		write("lean/Math2/X.lean", "module\n"),
		llmtest.Call(agent.StuckTool, map[string]string{"reason": "no"}, u),
	}, []*llm.Response{labels(explain.Supported)})
	if !strings.Contains(rec.Turns[0].Why, "has not written the L3 explanation") || !rec.Turns[1].Results[0].IsError || !strings.Contains(rec.Turns[1].Results[0].Content, "writes no Lean") {
		t.Errorf("the turns say %q and %+v", rec.Turns[0].Why, rec.Turns[1].Results)
	}
}
