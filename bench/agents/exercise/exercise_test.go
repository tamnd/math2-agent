// Package exercise_test is the suite of the exercise role (corpus K7.3, K8.7, spec 17.3.4).
//
// Each item of items.yaml is a target node and reference exercises for it, each with the answer that a solver must reach. The nodes and the exercises are in our own words. These are the public items. The held-out items are in a private repository, and -items reads them from a file of the same form.
//
// The test runs the role on each item in a new work tree. With no flags, the writer and the solver are scripted, and no test calls a model service. For each item, the script first writes an exercise with no solution in steps, and the goal must refuse it. Then it writes the reference exercises, and the goal must hold. The solver gives the reference answer of the first exercise and a wrong answer of the others, thus only the first one does not go to a person. A reference exercise that a person checks always goes to a person. Thus the suite checks the goal of the role, the solver and the queue, and that each reference exercise is valid.
//
// With -live, the test runs the role on each item with the Claude model of the rung -rung of the ladder. The model register has no model of another family, thus each exercise goes to a person. The report gives, for each item, the outcome, the exercises with their groups and checks, and the spend. It needs ANTHROPIC_API_KEY, and it spends money. Give -report FILE to write the report.
package exercise_test

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/exercise"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/claude"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/workspace"
)

var (
	itemsFile = flag.String("items", "items.yaml", "the file of the items")
	live      = flag.Bool("live", false, "run the role with the Claude models. This needs ANTHROPIC_API_KEY and spends money.")
	rung      = flag.Int("rung", 0, "the rung of the ladder of the role, with -live")
	report    = flag.String("report", "", "write the report of the run to this file")
)

// reference is one reference exercise of an item.
type reference struct {
	Name       string `yaml:"name"`
	Task       string `yaml:"task"`
	Hint       string `yaml:"hint"`
	Answer     string `yaml:"answer"`
	Check      string `yaml:"check"`
	Group      string `yaml:"group"`
	Difficulty int    `yaml:"difficulty"`
	Solution   string `yaml:"solution"`
}

// item is one item of items.yaml.
type item struct {
	Tag       string      `yaml:"tag"`
	Kind      string      `yaml:"kind"`
	Text      string      `yaml:"text"`
	Exercises []reference `yaml:"exercises"`
}

func readItems(t *testing.T) []item {
	t.Helper()
	data, err := os.ReadFile(*itemsFile)
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Items []item `yaml:"items"`
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		t.Fatal(err)
	}
	if len(f.Items) == 0 {
		t.Fatal("the file has no items")
	}
	return f.Items
}

// file returns the path and the text of the exercise node of the reference, with the provisional id of its number.
func (r reference) file(target string, i int) (string, string) {
	id := fmt.Sprintf("~%08x", 0x1a2b3c00+i)
	var b strings.Builder
	fmt.Fprintf(&b, "---\ntag: %s\nkind: example\nname: %s\nuses: [%s]\n---\n\n## Example\n\n%s\n\n## Remarks\n\n%s About: %s. Difficulty: %d. Group: %s. Check: %s.\n", id, r.Name, target, strings.TrimSpace(r.Task), exercise.Marker, target, r.Difficulty, r.Group, r.Check)
	for _, s := range [][2]string{{"Hint", r.Hint}, {"Answer", r.Answer}, {"Solution", r.Solution}} {
		if s[1] != "" {
			fmt.Fprintf(&b, "\n### %s\n\n%s\n", s[0], strings.TrimSpace(s[1]))
		}
	}
	return "nodes/~/" + id + ".md", b.String()
}

// tree makes a git repository with the target node and opens it as a workspace.
func tree(t *testing.T, it item) *workspace.Workspace {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, filepath.FromSlash(tools.NodePath(it.Tag)))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(it.Text), 0o600); err != nil {
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
	w, err := workspace.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

// run runs the role on the item in a new work tree with the model on the rung and the solver, and returns the record, the exercises of the tree and the referrals.
func run(t *testing.T, it item, m llm.Model, r int, solve exercise.Solver) (*agent.Record, []*exercise.Exercise, []exercise.Referral) {
	t.Helper()
	ws := tree(t, it)
	role, err := agent.LoadRole(prompts.FS, "exercise")
	if err != nil {
		t.Fatal(err)
	}
	j := &agent.Job{ID: "j-2026-10-27-1", Role: "exercise", Target: it.Tag, Goal: fmt.Sprintf("%d exercises", len(it.Exercises)), OnBehalfOf: "github:octocat"}
	brief, err := exercise.Brief(ws, j)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tools.Profile(role, nil, ws, nil)
	if err != nil {
		t.Fatal(err)
	}
	rn := agent.Runner{Role: role, Model: m, Tools: ts, Goal: exercise.Goal(ws, j, solve, "the scripted solver", role.Agent())}
	rec, err := rn.Run(context.Background(), j, r, brief)
	if err != nil {
		t.Fatal(err)
	}
	files, err := check.ReadTree(ws.Dir)
	if err != nil {
		t.Fatal(err)
	}
	var nodes []*node.Node
	for _, f := range files {
		if n, err := node.Parse(f.Path, f.Data); err == nil {
			nodes = append(nodes, n)
		}
	}
	exs, bad := exercise.Of(nodes, it.Tag)
	if len(bad) > 0 {
		t.Errorf("the tree has exercises that do not parse: %q", bad)
	}
	data, err := ws.Read(exercise.Queue)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	var refs []exercise.Referral
	for l := range strings.Lines(string(data)) {
		var x exercise.Referral
		if err := json.Unmarshal([]byte(l), &x); err != nil {
			t.Fatal(err)
		}
		refs = append(refs, x)
	}
	return rec, exs, refs
}

var u = llm.Usage{Input: 1000, Output: 500}

func TestSuite(t *testing.T) {
	items := readItems(t)
	if *live {
		liveRun(t, items)
		return
	}
	for _, it := range items {
		t.Run(it.Tag, func(t *testing.T) {
			first, firstText := it.Exercises[0].file(it.Tag, 0)
			script := []*llm.Response{llmtest.Call("write_file", map[string]string{"path": first, "text": strings.Replace(firstText, "\n1. ", "\n", 1)}, u)}
			for i, r := range it.Exercises {
				p, text := r.file(it.Tag, i)
				script = append(script, llmtest.Call("write_file", map[string]string{"path": p, "text": text}, u))
			}
			answers := map[string]string{}
			for i, r := range it.Exercises {
				answers[strings.TrimSpace(r.Task)] = "a wrong answer"
				if i == 0 {
					answers[strings.TrimSpace(r.Task)] = r.Answer
				}
			}
			solve := func(_ context.Context, task string) (string, error) { return answers[task], nil }
			rec, exs, refs := run(t, it, llmtest.New(script...), 0, solve)
			if rec.Outcome != agent.Success || len(rec.Turns) != len(script) {
				t.Fatalf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
			}
			if got := rec.Turns[0].Why; !strings.Contains(got, "numbered steps") {
				t.Errorf("an exercise with no steps gives %q", got)
			}
			if len(exs) != len(it.Exercises) {
				t.Errorf("the tree has %d exercises of %s, and the item has %d", len(exs), it.Tag, len(it.Exercises))
			}
			if len(refs) != len(it.Exercises)-1 {
				t.Errorf("%d exercises go to a person, and %d must go: %+v", len(refs), len(it.Exercises)-1, refs)
			}
			for _, x := range refs {
				if x.By != "m2-agent/exercise@0.1.0" || x.Reason == "" {
					t.Errorf("the referral is %+v", x)
				}
			}
		})
	}
}

// liveRun runs the role with the Claude model of the rung on each item, with no solver.
func liveRun(t *testing.T, items []item) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Fatal("-live needs ANTHROPIC_API_KEY")
	}
	m := claude.New()
	var rows, lists []string
	usd := 0.0
	for _, it := range items {
		rec, exs, refs := run(t, it, m, *rung, nil)
		usd += rec.Spend.USD
		groups := map[string]int{}
		for _, e := range exs {
			groups[e.Group]++
			lists = append(lists, fmt.Sprintf("| %s | %s | %s | %d | %s |", it.Tag, e.ID, e.Group, e.Difficulty, e.Check))
		}
		rows = append(rows, fmt.Sprintf("| %s | %s | %d | %d | %d | %d | %d | %.4f |", it.Tag, rec.Outcome, len(exs), groups["A"], groups["B"], groups["C"], len(refs), rec.Spend.USD))
	}
	text := fmt.Sprintf("# The suite of the exercise role\n\nThe role ran on %d items for $%.4f, at the rung %d of the ladder.\n\n| Target | Outcome | Exercises | A | B | C | To a person | USD |\n|---|---|---|---|---|---|---|---|\n%s\n\n| Target | Exercise | Group | Difficulty | Check |\n|---|---|---|---|---|\n%s\n",
		len(items), usd, *rung, strings.Join(rows, "\n"), strings.Join(lists, "\n"))
	if *report == "" {
		t.Log(text)
		return
	}
	if err := os.WriteFile(*report, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}
