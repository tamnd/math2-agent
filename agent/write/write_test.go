package write_test

import (
	"cmp"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/agent/write"
	"github.com/tamnd/math2-agent/card"
	"github.com/tamnd/math2-agent/copycheck"
	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/node"
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
	nodePath  = "nodes/3C5/3C53TD.md"
	groupPath = "nodes/G5S/G5S1M9.md"
	proofPath = "nodes/~/~1a2b3c4d.md"
	examPath  = "nodes/~/~2b3c4d5e.md"
	target    = "---\ntag: 3C53TD\nkind: statement\nrole: theorem\nname: Lagrange's theorem\nuses: [G5S1M9]\n---\n\n## Statement\n\nLet $G$ be a finite [[group|G5S1M9]] and let $H$ be a subgroup of $G$. Then the order of $H$ divides the order of $G$.\n"
	group     = "---\ntag: G5S1M9\nkind: definition\nname: Group\n---\n\n## Definition\n\nA **group** is a set with an associative operation, an identity and inverses.\n"
	steps     = "---\ntag: ~1a2b3c4d\nkind: proof\nname: Proof of Lagrange's theorem by cosets\nproves: 3C53TD\nuses: [G5S1M9]\n---\n\n## Proof\n\n1. The left cosets of $H$ partition $G$, by the definition of a [[group|G5S1M9]].\n2. Each coset has as many elements as $H$, by step 1.\n3. Thus the order of $H$ divides the order of $G$, by step 2.\n"
	example   = "---\ntag: ~2b3c4d5e\nkind: example\nname: The subgroups of the cyclic group of order 6\nillustrates: [3C53TD]\n---\n\n## Example\n\nThe cyclic group of order 6 has subgroups of the orders 1, 2, 3 and 6, and each one divides 6.\n"
)

func job(goal string) *agent.Job {
	return &agent.Job{ID: "j-2026-10-27-1", Role: "write", Target: "3C53TD", Goal: goal, OnBehalfOf: "github:octocat"}
}

func parse(t *testing.T, path, text string) *node.Node {
	t.Helper()
	n, err := node.Parse(path, []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMine(t *testing.T) {
	for it, want := range map[*card.Item]bool{
		{Section: "Statement"}:                                    true,
		{Section: "Explanations"}:                                 false,
		{Section: "History"}:                                      false,
		{Edge: []string{graph.Proves}, Dir: "in"}:                 true,
		{Edge: []string{"instance_of", "illustrates"}, Dir: "in"}: true,
		{Edge: []string{"defines"}, Dir: "in"}:                    false,
		{Edge: []string{graph.Uses}, Dir: "out"}:                  true,
		{Edge: []string{"about"}, Dir: "out"}:                     false,
		{Edge: []string{"specializes"}, Dir: "both"}:              false,
		{Field: "formal"}:                                         false,
		{Explain: "L2"}:                                           false,
	} {
		if got := write.Mine(*it); got != want {
			t.Errorf("Mine(%s) = %v, want %v", it, got, want)
		}
	}
}

func TestLevelAndNeeds(t *testing.T) {
	cards := card.Embedded()
	n := parse(t, nodePath, target)
	g := graph.Build([]*node.Node{n, parse(t, groupPath, group)})
	if l := write.Level(cards, n, g, ""); l != card.W2 {
		t.Errorf("a stated statement is at the level %s, and the job takes it to W2", l)
	}
	if l := write.Level(cards, n, g, "the W3 text"); l != card.W3 {
		t.Errorf("a goal that names W3 gives %s", l)
	}
	var what []string
	for _, nd := range write.Needs(cards, n, g, card.W2) {
		what = append(what, write.How(nd, "3C53TD"))
	}
	if strings.Join(what, "; ") != "add a proof node in nodes/~/ with proves: 3C53TD and a Proof section in numbered steps; add an example node in nodes/~/ with instance_of: [3C53TD]" {
		t.Errorf("the needs at W2 are %q", what)
	}
	bare := parse(t, nodePath, strings.Replace(target, "uses: [G5S1M9]\n", "", 1))
	if l := write.Level(cards, bare, graph.Build([]*node.Node{bare}), ""); l != card.W1 {
		t.Errorf("a statement with no uses is at the level %s, and the job takes it to W1", l)
	}
	g = graph.Build([]*node.Node{n, parse(t, proofPath, steps), parse(t, examPath, example)})
	if needs := write.Needs(cards, n, g, card.W2); len(needs) != 0 {
		t.Errorf("a statement with a proof and an example needs %+v", needs)
	}
}

func TestStepsAndBold(t *testing.T) {
	for text, want := range map[string]bool{
		"\n1. A claim, by step 0.\n2. The end.\n": true,
		"The cosets partition $G$.\n":             false,
		"":                                        false,
		"10. A claim.\n":                          false,
	} {
		if got := write.Steps(text); got != want {
			t.Errorf("Steps(%q) = %v", text, got)
		}
	}
	n := parse(t, groupPath, strings.Replace(group, "name: Group\n", "name: Group\naliases: [abstract-group]\n", 1))
	for text, want := range map[string]bool{
		"A **group** is a set.":           true,
		"An **Abstract group** is a set.": true,
		"A group is a set.":               false,
		"A **set** with an operation.":    false,
	} {
		if got := write.Bold(n, text); got != want {
			t.Errorf("Bold(%q) = %v", text, got)
		}
	}
}

func TestBrief(t *testing.T) {
	ws := tree(t, map[string]string{nodePath: target, groupPath: group})
	brief, err := write.Brief(ws, job("the node has each section that its card asks for"), card.Embedded())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"at the writing level W2 (usable)", "- add a proof node in nodes/~/ with proves: 3C53TD and a Proof section in numbered steps: at least one proof node.", "with instance_of: [3C53TD]: one example that shows the statement at work.", `<data source="nodes/3C5/3C53TD.md"`} {
		if !strings.Contains(brief, want) {
			t.Errorf("the brief has no %q:\n%s", want, brief)
		}
	}
	if _, err := write.Brief(ws, &agent.Job{Role: "write", Target: "nothing"}, card.Embedded()); err == nil {
		t.Error("a target that is not a tag gives a brief")
	}
}

var u = llm.Usage{Input: 1000, Output: 200}

func call(path, text string) *llm.Response {
	return llmtest.Call("write_file", map[string]string{"path": path, "text": text}, u)
}

// run runs a write job on a new tree with the scripted model.
func run(t *testing.T, files map[string]string, goal string, corpus *copycheck.Corpus, model ...*llm.Response) *agent.Record {
	t.Helper()
	all := map[string]string{nodePath: target, groupPath: group}
	for k, v := range files {
		all[k] = v
	}
	ws := tree(t, all)
	role, err := agent.LoadRole(prompts.FS, "write")
	if err != nil {
		t.Fatal(err)
	}
	j := job(cmp.Or(goal, "the node has each section, example and proof that its card asks for at the next writing level"))
	brief, err := write.Brief(ws, j, card.Embedded())
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tools.Profile(role, nil, ws, nil)
	if err != nil {
		t.Fatal(err)
	}
	rn := agent.Runner{Role: role, Model: llmtest.New(model...), Tools: ts, Goal: write.Goal(ws, j, card.Embedded(), corpus)}
	rec, err := rn.Run(context.Background(), j, 0, brief)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func stuck() *llm.Response {
	return llmtest.Call(agent.StuckTool, map[string]string{"reason": "no"}, u)
}

func TestRun(t *testing.T) {
	prose := strings.Replace(steps, "1. The left", "The left", 1)
	rec := run(t, nil, "", nil,
		call(proofPath, prose),
		call(examPath, strings.Replace(example, "## Example", "## Statement", 1)),
		call(examPath, example),
		call(proofPath, steps),
	)
	if rec.Outcome != agent.Success || len(rec.Turns) != 4 {
		t.Fatalf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
	}
	for i, want := range []string{"still needs this work: add an example node in nodes/~/ with instance_of: [3C53TD]", "m2 check finds", "is not a list of numbered steps", ""} {
		if got := rec.Turns[i].Why; want == "" && got != "" || !strings.Contains(got, want) {
			t.Errorf("after turn %d the goal says %q, and it must say %q", i+1, got, want)
		}
	}
}

func TestRunOwnSections(t *testing.T) {
	remarks := target + "\n## Remarks\n\nThe converse is false.\n"
	files := map[string]string{nodePath: remarks, proofPath: steps, examPath: example}
	changed := strings.Replace(remarks, "The converse is false.", "The converse is false for the alternating group $A_4$.", 1)

	log := `{"field":"*","op":"add","agent":null,"human":"github:octocat","at":"2026-10-21T10:00:00Z"}` + "\n"
	files[prov.Path("3C53TD")] = log
	rec := run(t, files, "", nil, call(nodePath, changed), stuck())
	if !strings.Contains(rec.Turns[0].Why, "github:octocat wrote or confirmed body.remarks") {
		t.Errorf("a change of the text of a person gives %q", rec.Turns[0].Why)
	}

	files[prov.Path("3C53TD")] = log + `{"field":"body.remarks","op":"add","agent":"m2-agent/explain@0.1.0","human":null,"at":"2026-10-22T10:00:00Z"}` + "\n"
	rec = run(t, files, "", nil, call(nodePath, changed), stuck())
	if !strings.Contains(rec.Turns[0].Why, "m2-agent/explain@0.1.0 wrote body.remarks") {
		t.Errorf("a change of the text of another role gives %q", rec.Turns[0].Why)
	}

	files[prov.Path("3C53TD")] = log + `{"field":"body.remarks","op":"add","agent":"m2-agent/write@0.1.0","human":null,"at":"2026-10-22T10:00:00Z"}` + "\n"
	if rec = run(t, files, "", nil, call(nodePath, changed)); rec.Outcome != agent.Success {
		t.Errorf("a change of its own text gives %s, %q", rec.Outcome, rec.Turns[0].Why)
	}

	delete(files, prov.Path("3C53TD"))
	rec = run(t, files, "", nil, call(nodePath, changed), stuck())
	if !strings.Contains(rec.Turns[0].Why, "does not show who wrote body.remarks") {
		t.Errorf("a change of a section with no log gives %q", rec.Turns[0].Why)
	}
}

func TestRunGates(t *testing.T) {
	done := map[string]string{proofPath: steps, examPath: example}
	filler := target + "\n## Remarks\n\nThe converse is clearly false.\n"
	rec := run(t, done, "a W3 text", nil, call(nodePath, filler), stuck())
	if !strings.Contains(rec.Turns[0].Why, "the house style finds 1 problems") || !strings.Contains(rec.Turns[0].Why, "Y02") {
		t.Errorf("a W3 job with a filler word gives %q", rec.Turns[0].Why)
	}
	if rec = run(t, done, "a W2 text", nil, call(nodePath, filler)); rec.Outcome != agent.Success {
		t.Errorf("a W2 job with a filler word gives %s, %q", rec.Outcome, rec.Turns[0].Why)
	}

	remark := "The converse is false. The alternating group on four letters has order twelve, and it has no subgroup of order six, although six divides twelve."
	corpus := copycheck.New([]copycheck.Text{{Source: "a-book", Licence: "LicenseRef-closed", Class: "record", Text: remark}}, nil)
	rec = run(t, done, "", corpus, call(nodePath, target+"\n## Remarks\n\n"+remark+"\n"), stuck())
	if !strings.Contains(rec.Turns[0].Why, "the copy check finds text of 3C53TD that copies a source") || !strings.Contains(rec.Turns[0].Why, "body.remarks copies a-book") {
		t.Errorf("a copied remark gives %q", rec.Turns[0].Why)
	}

	rec = run(t, done, "", nil, call(nodePath, strings.Replace(target, "name: Lagrange's theorem", "name: Lagrange", 1)), stuck())
	if !rec.Turns[0].Results[0].IsError || !strings.Contains(rec.Turns[0].Results[0].Content, "the field name") {
		t.Errorf("a change of the name gives %+v", rec.Turns[0].Results)
	}
}
