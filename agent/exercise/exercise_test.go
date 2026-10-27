package exercise_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/exercise"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/schema"
	"github.com/tamnd/math2-agent/workspace"
)

const (
	groupPath    = "nodes/G5S/G5S1M9.md"
	targetPath   = "nodes/3C5/3C53TD.md"
	exercisePath = "nodes/~/~1a2b3c4d.md"
	group        = "---\ntag: G5S1M9\nkind: definition\nname: Group\n---\n\n## Definition\n\nA **group** is a set with an associative operation, an identity and inverses.\n"
	lagrange     = "---\ntag: 3C53TD\nkind: statement\nrole: theorem\nname: Lagrange's theorem\nuses: [G5S1M9]\n---\n\n## Statement\n\nLet $G$ be a finite [[group|G5S1M9]] and let $H$ be a subgroup of $G$. Then the order of $H$ divides the order of $G$.\n"
	task         = "A group $G$ has order 12, and $H$ is a subgroup of $G$ whose order is greater than 6. What is the order of $H$?"
	good         = "---\ntag: ~1a2b3c4d\nkind: example\nname: A large subgroup of a group of order 12\nuses: [3C53TD]\n---\n\n## Example\n\n" + task + "\n\n## Remarks\n\nRole: exercise. About: 3C53TD. Difficulty: 2. Group: B. Check: exact.\n\n### Hint\n\nUse [[Lagrange's theorem|3C53TD]].\n\n### Answer\n\n12\n\n### Solution\n\n1. The order of $H$ divides 12, by [[Lagrange's theorem|3C53TD]].\n2. The only divisor of 12 that is greater than 6 is 12. Thus the order of $H$ is 12.\n"
)

// goodV2 is good in the form of node.v2.
const goodV2 = "---\ntag: ~1a2b3c4d\nkind: exercise\nname: A large subgroup of a group of order 12\nuses: [3C53TD]\nabout: [3C53TD]\ndifficulty: 2\nmath_group: B\ncheck: exact\n---\n\n## Exercise\n\n" + task + "\n\n## Hint\n\nUse [[Lagrange's theorem|3C53TD]].\n\n## Answer\n\n12\n\n## Solution\n\n1. The order of $H$ divides 12, by [[Lagrange's theorem|3C53TD]].\n2. The only divisor of 12 that is greater than 6 is 12. Thus the order of $H$ is 12.\n"

// v1 is the file that puts a tree at node.v1. Its text is the schema of the toolchain, because only the name of the file tells the form.
var v1 = map[string]string{schema.Previous: string(schema.Raw())}

// tree makes a git repository with the files in one commit and opens it as a workspace.
func tree(t *testing.T, files map[string]string) *workspace.Workspace {
	t.Helper()
	dir := t.TempDir()
	all := map[string]string{groupPath: group, targetPath: lagrange}
	for k, v := range files {
		all[k] = v
	}
	for name, text := range all {
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

func job(goal string) *agent.Job {
	return &agent.Job{ID: "j-2026-10-27-1", Role: "exercise", Target: "3C53TD", Goal: goal, OnBehalfOf: "github:octocat"}
}

func parse(t *testing.T, text string) *node.Node {
	t.Helper()
	n, err := node.Parse(exercisePath, []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestParse(t *testing.T) {
	e, err := exercise.Parse(parse(t, good))
	if err != nil || e == nil {
		t.Fatalf("Parse gives %+v, %v", e, err)
	}
	if e.ID != "~1a2b3c4d" || len(e.About) != 1 || e.About[0] != "3C53TD" || e.Difficulty != 2 || e.Group != "B" || e.Check != "exact" || e.Task != task || e.Answer != "12" || !strings.HasPrefix(e.Solution, "1. The order") || !strings.HasPrefix(e.Hint, "Use ") {
		t.Errorf("the exercise is %+v", e)
	}
	if p := e.Problems("3C53TD"); len(p) > 0 {
		t.Errorf("the exercise has the problems %q", p)
	}
	if e, err := exercise.Parse(parse(t, strings.Replace(good, "Role: exercise. ", "", 1))); e != nil || err != nil {
		t.Errorf("an example that is not an exercise gives %+v, %v", e, err)
	}
	for text, want := range map[string]string{
		strings.Replace(good, " Group: B.", "", 1):                              "has no Group",
		strings.Replace(good, " Check: exact.", " Check: exact. Level: L1.", 1): "has only About",
		strings.Replace(good, " Difficulty: 2.", " Difficulty: easy.", 1):       "must be a number",
		strings.Replace(good, "\n\n### Hint", "\n\nMore text.\n\n### Hint", 1):  "has text between",
		strings.Replace(good, "### Hint", "### Answer", 1):                      "the heading Answer two times",
	} {
		if _, err := exercise.Parse(parse(t, text)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse gives %v, and it must say %q", err, want)
		}
	}
}

func TestProblems(t *testing.T) {
	base, err := exercise.Parse(parse(t, good))
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		change func(e *exercise.Exercise)
		want   string
	}{
		"about":      {func(e *exercise.Exercise) { e.About = []string{"G5S1M9"} }, "is not about the target"},
		"difficulty": {func(e *exercise.Exercise) { e.Difficulty = 6 }, "from 1 to 5"},
		"group":      {func(e *exercise.Exercise) { e.Group = "D" }, "A, B or C"},
		"check":      {func(e *exercise.Exercise) { e.Check = "oracle" }, "one of exact"},
		"lean":       {func(e *exercise.Exercise) { e.Check = "lean" }, "writes no Lean yet"},
		"answer":     {func(e *exercise.Exercise) { e.Answer = "" }, "no Answer"},
		"numeric":    {func(e *exercise.Exercise) { e.Check = "numeric"; e.Answer = "twelve" }, "needs a number"},
		"task":       {func(e *exercise.Exercise) { e.Task = " " }, "has no task"},
		"steps":      {func(e *exercise.Exercise) { e.Solution = "The order divides 12." }, "numbered steps"},
	} {
		e := *base
		c.change(&e)
		if p := strings.Join(e.Problems("3C53TD"), "; "); !strings.Contains(p, c.want) {
			t.Errorf("%s: the problems are %q, and they must say %q", name, p, c.want)
		}
	}
	v2, err := exercise.Parse(parse(t, goodV2))
	if err != nil || v2 == nil || v2.Old {
		t.Fatalf("Parse of a node.v2 exercise gives %+v, %v", v2, err)
	}
	if p := v2.Problems("3C53TD"); len(p) > 0 {
		t.Errorf("the node.v2 exercise has the problems %q", p)
	}
	v2.About, v2.Answer, v2.Solution = nil, "", "No steps."
	if p := strings.Join(v2.Problems("3C53TD"), "; "); !strings.Contains(p, "give about: [3C53TD]") || !strings.Contains(p, "under ## Answer") || !strings.Contains(p, "under ## Solution") {
		t.Errorf("the problems of a node.v2 exercise are %q", p)
	}
	e := *base
	e.Check, e.Answer = "none", ""
	if p := e.Problems("3C53TD"); len(p) > 0 {
		t.Errorf("an exercise that a person checks needs no answer, and it has the problems %q", p)
	}
}

func TestCount(t *testing.T) {
	for goal, want := range map[string]int{"": 1, "exercises of group C": 1, "three exercises": 3, "write 4 exercises, two of group C": 4, "Two.": 2} {
		if got := exercise.Count(goal); got != want {
			t.Errorf("Count(%q) = %d, want %d", goal, got, want)
		}
	}
}

func TestAgree(t *testing.T) {
	for _, c := range []struct {
		check, want, got string
		ok               bool
	}{
		{"exact", "12", "$12$.", true},
		{"exact", "$x^2 + 1$", "x^2+1", true},
		{"exact", "12", "6", false},
		{"exact", "", "", false},
		{"numeric", "0.25", "0.2500000001", true},
		{"numeric", "0.25", "1/4", false},
		{"choice", "(b)", "B", true},
		{"choice", "(b)", "(c) 12", false},
		{"cas", "x+1", "x+1", false},
		{"none", "a proof", "a proof", false},
	} {
		if got := exercise.Agree(c.check, c.want, c.got); got != c.ok {
			t.Errorf("Agree(%q, %q, %q) = %v", c.check, c.want, c.got, got)
		}
	}
}

var u = llm.Usage{Input: 1000, Output: 200}

func TestSolve(t *testing.T) {
	m := llmtest.New(llmtest.Call("answer", map[string]string{"answer": "12"}, u), llmtest.Say("12", u))
	solve := exercise.Solve(m, "gpt-x", "solve")
	if got, err := solve(context.Background(), task); err != nil || got != "12" {
		t.Errorf("the solver gives %q, %v", got, err)
	}
	if _, err := solve(context.Background(), task); err == nil || !strings.Contains(err.Error(), "gave no answer") {
		t.Errorf("a solver with no tool call gives %v", err)
	}
}

func TestBrief(t *testing.T) {
	other := strings.NewReplacer("~1a2b3c4d", "~5e6f7a8b", "Group: B", "Group: A").Replace(good)
	ws := tree(t, map[string]string{"nodes/~/~5e6f7a8b.md": other, schema.Previous: v1[schema.Previous]})
	brief, err := exercise.Brief(ws, job("three exercises"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`Job j-2026-10-27-1: write 3 exercises for the target 3C53TD, a statement node with the name "Lagrange's theorem". The goal of the job: three exercises.`,
		"Then the order of $H$ divides the order of $G$.",
		"- ~5e6f7a8b: group A, difficulty 2, check exact\n",
		"at least one exercise of group C",
		"kind: example\nname: the name of the exercise in words\nuses: [3C53TD]\n---\n\n## Example\n",
		"Role: exercise. About: 3C53TD. Difficulty: 2. Group: B. Check: exact.\n\n### Hint\n",
	} {
		if !strings.Contains(brief, want) {
			t.Errorf("the brief has no %q:\n%s", want, brief)
		}
	}
	if _, err := exercise.Brief(ws, &agent.Job{Target: "nope"}); err == nil {
		t.Error("a target that is not a node gives no error")
	}
	brief, err = exercise.Brief(tree(t, nil), job("an exercise"))
	if err != nil {
		t.Fatal(err)
	}
	want := "kind: exercise\nname: the name of the exercise in words\nuses: [3C53TD]\nabout: [3C53TD]\ndifficulty: 2\nmath_group: B\ncheck: exact\n---\n\n## Exercise\n"
	if !strings.Contains(brief, want) || !strings.Contains(brief, "\n## Solution\n\n1. ") || strings.Contains(brief, exercise.Marker) {
		t.Errorf("the brief of a tree at node.v2 has not the form of node.v2:\n%s", brief)
	}
}

func TestV2(t *testing.T) {
	if !exercise.V2(tree(t, nil).Dir) {
		t.Error("a tree with no schema file is not at node.v2")
	}
	if !exercise.V2(tree(t, map[string]string{schema.File: "{}"}).Dir) {
		t.Errorf("a tree with %s is not at node.v2", schema.File)
	}
	if exercise.V2(tree(t, v1).Dir) {
		t.Errorf("a tree with %s is at node.v2", schema.Previous)
	}
}

func call(path, text string) *llm.Response {
	return llmtest.Call("write_file", map[string]string{"path": path, "text": text}, u)
}

func stuck() *llm.Response {
	return llmtest.Call(agent.StuckTool, map[string]string{"reason": "no"}, u)
}

// run runs an exercise job on a new tree with the scripted model, and returns the record and the referrals.
func run(t *testing.T, files map[string]string, goal string, solve exercise.Solver, model ...*llm.Response) (*agent.Record, []exercise.Referral) {
	t.Helper()
	ws := tree(t, files)
	role, err := agent.LoadRole(prompts.FS, "exercise")
	if err != nil {
		t.Fatal(err)
	}
	j := job(goal)
	brief, err := exercise.Brief(ws, j)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tools.Profile(role, nil, ws, nil)
	if err != nil {
		t.Fatal(err)
	}
	rn := agent.Runner{Role: role, Model: llmtest.New(model...), Tools: ts, Goal: exercise.Goal(ws, j, solve, "gpt-x", role.Agent())}
	rec, err := rn.Run(context.Background(), j, 0, brief)
	if err != nil {
		t.Fatal(err)
	}
	var out []exercise.Referral
	data, err := ws.Read(exercise.Queue)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	for l := range strings.Lines(string(data)) {
		var r exercise.Referral
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return rec, out
}

func TestRun(t *testing.T) {
	steps := []struct{ text, why string }{
		{strings.Replace(good, "## Example", "## Statement", 1), "m2 check finds"},
		{strings.Replace(good, "Check: exact.", "Check: lean.", 1), "writes no Lean yet"},
		{strings.Replace(good, "1. The order", "The order", 1), "numbered steps"},
		{strings.Replace(good, " Group: B.", "", 1), "has no Group"},
		{good, ""},
	}
	var model []*llm.Response
	for _, s := range steps {
		model = append(model, call(exercisePath, s.text))
	}
	rec, refs := run(t, nil, "one exercise", nil, model...)
	if rec.Outcome != agent.Success || len(rec.Turns) != len(steps) {
		t.Fatalf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
	}
	for i, s := range steps {
		if got := rec.Turns[i].Why; s.why == "" && got != "" || !strings.Contains(got, s.why) {
			t.Errorf("after turn %d the goal says %q, and it must say %q", i+1, got, s.why)
		}
	}
	if len(refs) != 1 || refs[0].Exercise != "~1a2b3c4d" || refs[0].Reason != "math2 has no solver of another family yet" || refs[0].By != "m2-agent/exercise@0.2.0" || refs[0].Answer != "12" {
		t.Errorf("the referrals are %+v", refs)
	}
}

func TestRunV2(t *testing.T) {
	steps := []struct{ text, why string }{
		{strings.Replace(goodV2, "math_group: B", "math_group: D", 1), "m2 check finds"},
		{strings.Replace(goodV2, "1. The order", "The order", 1), "under ## Solution"},
		{goodV2, ""},
	}
	var model []*llm.Response
	for _, s := range steps {
		model = append(model, call(exercisePath, s.text))
	}
	rec, refs := run(t, nil, "one exercise", nil, model...)
	if rec.Outcome != agent.Success || len(rec.Turns) != len(steps) {
		t.Fatalf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
	}
	for i, s := range steps {
		if got := rec.Turns[i].Why; s.why == "" && got != "" || !strings.Contains(got, s.why) {
			t.Errorf("after turn %d the goal says %q, and it must say %q", i+1, got, s.why)
		}
	}
	if len(refs) != 1 || refs[0].Exercise != "~1a2b3c4d" || refs[0].Answer != "12" {
		t.Errorf("the referrals are %+v", refs)
	}
	// In a tree at node.v1, the goal still accepts an exercise of node.v1.
	if rec, _ := run(t, v1, "one exercise", nil, call(exercisePath, good)); rec.Outcome != agent.Success {
		t.Errorf("a node.v1 exercise in a tree at node.v1 gives %s, %q", rec.Outcome, rec.Reason)
	}
}

func TestRunCount(t *testing.T) {
	rec, _ := run(t, nil, "two exercises", nil, call(exercisePath, good), stuck())
	if got := rec.Turns[0].Why; !strings.Contains(got, "the run adds 1 of the 2 exercises that the job asks for") {
		t.Errorf("one exercise of two gives %q", got)
	}
}

func TestRunSolver(t *testing.T) {
	answer := func(a string) exercise.Solver {
		return func(_ context.Context, got string) (string, error) {
			if got != task {
				t.Errorf("the solver gets %q, and it must get only the task", got)
			}
			return a, nil
		}
	}
	if rec, refs := run(t, nil, "an exercise", answer("$12$"), call(exercisePath, good)); rec.Outcome != agent.Success || len(refs) != 0 {
		t.Errorf("a solved exercise gives %s, %+v", rec.Outcome, refs)
	}
	rec, refs := run(t, nil, "an exercise", answer("6"), call(exercisePath, good))
	if rec.Outcome != agent.Success || len(refs) != 1 || refs[0].Got != "6" || refs[0].Solver != "gpt-x" || !strings.Contains(refs[0].Reason, "does not reach the answer") {
		t.Errorf("an exercise that the solver does not solve gives %s, %+v", rec.Outcome, refs)
	}
	none := strings.Replace(good, "Check: exact.", "Check: none.", 1)
	if _, refs := run(t, nil, "an exercise", answer("12"), call(exercisePath, none)); len(refs) != 1 || refs[0].Got != "" || !strings.Contains(refs[0].Reason, "a person checks") {
		t.Errorf("an exercise that a person checks gives %+v", refs)
	}
}

func TestRunOwn(t *testing.T) {
	const path = "nodes/4N9/4N9QX2.md"
	mine := strings.Replace(good, "~1a2b3c4d", "4N9QX2", 1)
	changed := strings.Replace(mine, "Use [[Lagrange's theorem|3C53TD]].", "Use [[Lagrange's theorem|3C53TD]] on $H$.", 1)
	files := map[string]string{path: mine, prov.Path("4N9QX2"): `{"field":"*","op":"add","agent":null,"human":"github:octocat","at":"2026-10-21T10:00:00Z"}` + "\n"}
	rec, _ := run(t, files, "an exercise", nil, call(path, changed), stuck())
	if got := rec.Turns[0].Why; !strings.Contains(got, "github:octocat wrote or confirmed body.remarks, and exercise does not change it") {
		t.Errorf("a change of the text of a person gives %q", got)
	}
	// The tool refuses a change outside the permissions, thus the tree does not change.
	rec, _ = run(t, nil, "an exercise", nil, call(targetPath, strings.Replace(lagrange, "divides", "is a divisor of", 1)), stuck())
	if got := rec.Turns[0].Why; got != "the run has not changed the tree yet" {
		t.Errorf("a change of the target gives %q", got)
	}
}
