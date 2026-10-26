// Package explain_test is the suite of the explain role (spec 17.3.4 and 17.3.5).
//
// Each item of items.yaml is a target node of tree/, a level and a reference explanation, with wrong explanations and the label that the grounding check must give each of their sentences. The explanations are in our own words. These are the public items. The held-out items are in a private repository, and -items reads them from a file of the same form.
//
// The test makes a work tree of math2 from tree/, with the nodes and a curriculum in topics/curricula/. Then it runs the role on each item, and puts the tree back after each run.
//
// With no flags, the model and the grounding check are scripted, and no test calls a model service. For each item, the script first writes the reference, the check labels each sentence supported, and the goal must hold. Then the script writes wrong attempts, and the goal must not hold, with the reason that the attempt must give. An explanation with no citation, an explanation that cites a node outside the curricula of its level, and an explanation at the wrong level fail with no check. A wrong explanation of the items fails with the labels of the check. Thus the suite checks the goal of the role and that each item has a solution.
//
// With -live, the test runs the role on each item with the Claude model of the rung -rung of the ladder, and the same model runs the check. Then the check alone labels the reference and each wrong explanation of each item, and the report gives the share of the labels that are right. It needs ANTHROPIC_API_KEY, and it spends money. Give -report FILE to write the report.
package explain_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/explain"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/claude"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/workspace"
)

var (
	itemsFile = flag.String("items", "items.yaml", "the file of the items")
	live      = flag.Bool("live", false, "run the role and the check with the Claude models. This needs ANTHROPIC_API_KEY and spends money.")
	rung      = flag.Int("rung", 0, "the rung of the ladder of the role, with -live")
	report    = flag.String("report", "", "write the report of the run to this file")
)

// beyond is a node of the tree in a course of year 3, thus outside the curricula of L1 and L2.
const beyond = "7K3QX2"

// item is one item of items.yaml.
type item struct {
	Target string `yaml:"target"`
	Level  string `yaml:"level"` // L1 or L2
	Text   string `yaml:"text"`  // the reference explanation
	Wrong  []struct {
		Name   string   `yaml:"name"`
		Text   string   `yaml:"text"`
		Labels []string `yaml:"labels"`
	} `yaml:"wrong"`

	n int // the number of the item in the file, from 1
}

func readItems(t *testing.T) []item {
	t.Helper()
	data, err := os.ReadFile(*itemsFile)
	if err != nil {
		t.Fatal(err)
	}
	var f struct{ Items []item }
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		t.Fatalf("%s: %v", *itemsFile, err)
	}
	for i := range f.Items {
		it := &f.Items[i]
		it.n = i + 1
		if it.Level != "L1" && it.Level != "L2" || len(explain.Cited(it.Text)) == 0 || len(it.Wrong) == 0 {
			t.Fatalf("%s: the item %s has the level %q, %d citations and %d wrong explanations", *itemsFile, it.Target, it.Level, len(explain.Cited(it.Text)), len(it.Wrong))
		}
		for _, w := range it.Wrong {
			if len(w.Labels) != len(explain.Sentences(w.Text)) {
				t.Fatalf("%s: the wrong explanation %q of %s has %d labels and %d sentences", *itemsFile, w.Name, it.Target, len(w.Labels), len(explain.Sentences(w.Text)))
			}
		}
	}
	return f.Items
}

func (it item) path() string { return tools.NodePath(it.Target) }

// explained is the target node with the explanation at the level.
func (it item) explained(base, level, text string) string {
	return base + "\n## Explanations\n\n### " + level + " agent\n\n" + text + "\n"
}

// suite is the work tree of the items.
type suite struct{ dir string }

func setup(t *testing.T) *suite {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("tree")); err != nil {
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
	return &suite{dir: dir}
}

// base is the target node of the item at the base commit.
func (s *suite) base(t *testing.T, it item) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.dir, filepath.FromSlash(it.path())))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// reset puts the tree back at the base commit.
func (s *suite) reset(t *testing.T) {
	t.Helper()
	for _, args := range [][]string{{"checkout", "-q", "--", "."}, {"clean", "-q", "-f", "-d"}} {
		if out, err := exec.Command("git", append([]string{"-C", s.dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// run runs the role on the item with the model and the model of the check, and puts the tree back.
func (s *suite) run(t *testing.T, it item, model, check llm.Model, r int) *agent.Record {
	t.Helper()
	defer s.reset(t)
	ctx := context.Background()
	ws, err := workspace.Open(ctx, s.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	role, err := agent.LoadRole(prompts.FS, "explain")
	if err != nil {
		t.Fatal(err)
	}
	job := &agent.Job{ID: fmt.Sprintf("j-2026-10-26-%d", it.n), Role: "explain", Target: it.Target, Goal: "an " + it.Level + " explanation", OnBehalfOf: "schedule:agent-suite"}
	brief, err := explain.Brief(ws, job)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tools.Profile(role, nil, ws, nil)
	if err != nil {
		t.Fatal(err)
	}
	meter := &agent.Meter{Model: check}
	rn := agent.Runner{Role: role, Model: model, Tools: ts, Meter: meter, Goal: explain.Goal(ws, job, meter, role.Ladder[r].Model, role.Checks["ground"])}
	rec, err := rn.Run(ctx, job, r, brief)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

var u = llm.Usage{Input: 1000, Output: 200}

func stuck() *llm.Response {
	return llmtest.Call(agent.StuckTool, map[string]string{"reason": "the attempt does not work"}, u)
}

func labels(l ...string) *llm.Response {
	var in []map[string]any
	for i, x := range l {
		in = append(in, map[string]any{"sentence": i + 1, "label": x, "why": "the scripted label."})
	}
	return llmtest.Call("label", map[string]any{"labels": in}, u)
}

// supported labels each sentence of the text supported.
func supported(text string) []string {
	return slices.Repeat([]string{explain.Supported}, len(explain.Sentences(text)))
}

// link matches a link [[text|TAG]].
var link = regexp.MustCompile(`\[\[([^|\]]*)\|[^\]]*\]\]`)

// attempt is a scripted attempt at an item, the scripted labels of the check, and the reason that the goal must give after it. An empty reason means that the goal must hold.
type attempt struct {
	name   string
	steps  []*llm.Response
	labels []*llm.Response
	why    string
}

// attempts are the reference and the wrong attempts at the item, whose target node is base at the base commit.
func attempts(it item, base string) []attempt {
	write := func(level, text string, last ...*llm.Response) []*llm.Response {
		return append([]*llm.Response{llmtest.Call("write_file", map[string]string{"path": it.path(), "text": it.explained(base, level, text)}, u)}, last...)
	}
	other := map[string]string{"L1": "L3", "L2": "L3"}[it.Level]
	out := []attempt{
		{"reference", write(it.Level, it.Text), []*llm.Response{labels(supported(it.Text)...)}, ""},
		{"no citation", write(it.Level, link.ReplaceAllString(it.Text, "$1"), stuck()), nil, "explanation cites no node"},
		{"beyond the level", write(it.Level, it.Text+" A [[ring|"+beyond+"]] is a group with more structure.", stuck()), nil, "links to " + beyond + ", which is not in the curricula of " + it.Level},
		{"wrong level", write(other, it.Text, stuck()), []*llm.Response{labels(supported(it.Text)...)}, "has not written the " + it.Level + " explanation"},
	}
	for _, w := range it.Wrong {
		why := "the cited nodes do not support sentence"
		if slices.Contains(w.Labels, explain.Contradicted) {
			why = "of the " + it.Level + " explanation is contradicted by the cited nodes"
		}
		out = append(out, attempt{w.Name, write(it.Level, w.Text, stuck()), []*llm.Response{labels(w.Labels...)}, why})
	}
	return out
}

func TestSuite(t *testing.T) {
	items := readItems(t)
	s := setup(t)
	if *live {
		liveRun(t, s, items)
		return
	}
	solved := 0
	for _, it := range items {
		for _, a := range attempts(it, s.base(t, it)) {
			t.Run(it.Target+"/"+a.name, func(t *testing.T) {
				rec := s.run(t, it, llmtest.New(a.steps...), llmtest.New(a.labels...), 0)
				whys := []string{rec.Reason}
				for _, turn := range rec.Turns {
					whys = append(whys, turn.Why)
				}
				if a.why == "" {
					if rec.Outcome != agent.Success {
						t.Fatalf("the reference gives %s, %q; the goal said %q", rec.Outcome, rec.Reason, whys)
					}
					solved++
					return
				}
				if rec.Outcome == agent.Success || !slices.ContainsFunc(whys, func(w string) bool { return strings.Contains(w, a.why) }) {
					t.Errorf("the attempt gives %s, %q, and the goal said %q; it must say %q", rec.Outcome, rec.Reason, whys, a.why)
				}
			})
		}
	}
	writeReport(t, fmt.Sprintf("# The suite of the explain role\n\nThe references solved %d of %d items with the scripted model, and the goal refused each wrong attempt.\n", solved, len(items)))
}

// liveRun runs the role and the check with the Claude model of the rung.
func liveRun(t *testing.T, s *suite, items []item) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Fatal("-live needs ANTHROPIC_API_KEY")
	}
	role, err := agent.LoadRole(prompts.FS, "explain")
	if err != nil {
		t.Fatal(err)
	}
	m := claude.New()
	var runs, checked []string
	solved, right, total, usd := 0, 0, 0, 0.0
	for _, it := range items {
		rec := s.run(t, it, m, m, *rung)
		if rec.Outcome == agent.Success {
			solved++
		}
		usd += rec.Spend.USD
		runs = append(runs, fmt.Sprintf("| %s | %s | %s | %d | %.4f | %s |", it.Target, it.Level, rec.Outcome, len(rec.Turns), rec.Spend.USD, cell(rec.Reason)))

		cited := map[string]string{}
		for _, tg := range explain.Cited(it.Text) {
			data, err := os.ReadFile(filepath.Join(s.dir, filepath.FromSlash(tools.NodePath(tg))))
			if err != nil {
				t.Fatal(err)
			}
			cited[tg] = string(data)
		}
		cited[it.Target] = s.base(t, it)
		cases := []struct {
			name, text string
			want       []string
		}{{"reference", it.Text, supported(it.Text)}}
		for _, w := range it.Wrong {
			cases = append(cases, struct {
				name, text string
				want       []string
			}{w.Name, w.Text, w.Labels})
		}
		for _, c := range cases {
			got, err := explain.Ground(context.Background(), m, role.Ladder[*rung].Model, role.Checks["ground"], explain.Explanation{Level: it.Level, Text: c.text}, cited)
			if err != nil {
				t.Fatal(err)
			}
			var labels []string
			for i, l := range got {
				total++
				if i < len(c.want) && l.Label == c.want[i] {
					right++
				}
				labels = append(labels, l.Label)
			}
			checked = append(checked, fmt.Sprintf("| %s | %s | %s | %s |", it.Target, c.name, strings.Join(c.want, ", "), strings.Join(labels, ", ")))
		}
	}
	writeReport(t, fmt.Sprintf("# The suite of the explain role\n\nThe role solved %d of %d items within the budget, for $%.4f, at the rung %d of the ladder.\n\n| Item | Level | Outcome | Turns | USD | Reason |\n|---|---|---|---|---|---|\n%s\n\nThe check gave the right label to %d of %d sentences.\n\n| Item | Explanation | Want | Labels |\n|---|---|---|---|\n%s\n",
		solved, len(items), usd, *rung, strings.Join(runs, "\n"), right, total, strings.Join(checked, "\n")))
}

func cell(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "|", `\|`), "\n", " ") }

func writeReport(t *testing.T, text string) {
	t.Helper()
	if *report == "" {
		return
	}
	if err := os.WriteFile(*report, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}
