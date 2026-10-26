// Package curate_test is the suite of the curate role (spec 17.3.4).
//
// Each item of items.yaml is a record of the source bench, the reference node of the record, and wrong nodes with the verdict that the judge must give each one. The records and the nodes are in our own words. These are the public items. The held-out items are in a private repository, and -items reads them from a file of the same form.
//
// The test makes a work tree of math2 with the nodes of tree/, a catalogue with the source bench, and the records of the items in build/records/bench.jsonl. Then it runs the role on each record, and puts the tree back after each run.
//
// With no flags, the model and the judge are scripted, and no test calls a model service. For each item, the script first reads the record and writes the reference, the judge says same, and the goal must hold. Then the script writes wrong attempts, and the goal must not hold, with the reason that the attempt must give. A node that copies the record, a node with no source at the locator of the record and a node of the wrong kind fail with no judge. A wrong node of the items fails with the verdict of the judge. Thus the suite checks the goal of the role and that each item has a solution.
//
// With -live, the test runs the role on each item with the Claude model of the rung -rung of the ladder, and the same model judges. Then the judge alone grades the reference and each wrong node of each item, and the report gives the share of the verdicts that are right. It needs ANTHROPIC_API_KEY, and it spends money. Give -report FILE to write the report.
package curate_test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/curate"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/claude"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/workspace"
)

var (
	itemsFile = flag.String("items", "items.yaml", "the file of the items")
	live      = flag.Bool("live", false, "run the role and the judge with the Claude models. This needs ANTHROPIC_API_KEY and spends money.")
	rung      = flag.Int("rung", 0, "the rung of the ladder of the role, with -live")
	report    = flag.String("report", "", "write the report of the run to this file")
)

// item is one item of items.yaml.
type item struct {
	ID    string   `yaml:"id"`
	Kind  string   `yaml:"kind"` // statement or definition
	Text  string   `yaml:"text"` // the text of the record
	Name  string   `yaml:"name"`
	MSC   string   `yaml:"msc"`
	Uses  []string `yaml:"uses"`
	Body  string   `yaml:"body"` // the reference statement or summary
	Wrong []struct {
		Name    string `yaml:"name"`
		Body    string `yaml:"body"`
		Verdict string `yaml:"verdict"`
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
		if it.Kind != "statement" && it.Kind != "definition" || it.Body == "" || len(it.Wrong) == 0 {
			t.Fatalf("%s: the item %s has the kind %q, a body of %d bytes and %d wrong nodes", *itemsFile, it.ID, it.Kind, len(it.Body), len(it.Wrong))
		}
		for _, w := range it.Wrong {
			if !slices.Contains([]string{curate.Generalizes, curate.Specializes, curate.Related, curate.Different}, w.Verdict) {
				t.Fatalf("%s: the wrong node %q of %s has the verdict %q", *itemsFile, w.Name, it.ID, w.Verdict)
			}
		}
	}
	return f.Items
}

func (it item) ref() string     { return "bench:" + it.ID }
func (it item) locator() string { return "bench://" + it.ID }

// path is the path of the new node of the run.
const path = "nodes/~/~0b0c0d0e.md"

// node is a node of the item with the body. A kind of concept or statement other than the kind of the item, and a locator that is empty, make wrong nodes.
func (it item) node(kind, locator, body string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\ntag: ~0b0c0d0e\nkind: %s\n", kind)
	if kind == "statement" {
		b.WriteString("role: theorem\n")
	}
	fmt.Fprintf(&b, "name: %s\nmsc: [%s]\n", it.Name, it.MSC)
	if len(it.Uses) > 0 {
		fmt.Fprintf(&b, "uses: [%s]\n", strings.Join(it.Uses, ", "))
	}
	if locator != "" {
		fmt.Fprintf(&b, "sources:\n  - url: https://github.com/tamnd/math2-agent/tree/main/bench/agents/curate\n    locator: %s\n", locator)
	}
	section := "Statement"
	if kind == "concept" {
		section = "Summary"
	}
	fmt.Fprintf(&b, "---\n\n## %s\n\n%s\n", section, body)
	return b.String()
}

// kind is the kind of the node of the item, and other is the other kind.
func (it item) kind() (kind, other string) {
	if it.Kind == "definition" {
		return "concept", "statement"
	}
	return "statement", "concept"
}

// suite is the work tree of the items.
type suite struct{ dir string }

func setup(t *testing.T, items []item) *suite {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("tree")); err != nil {
		t.Fatal(err)
	}
	var recs strings.Builder
	for _, it := range items {
		data, err := json.Marshal(ingest.Record{ID: it.ID, Kind: it.Kind, Title: it.ID, Locator: it.locator(), Class: ingest.Open, Text: it.Text, Fingerprints: ingest.Fingerprints(it.Text)})
		if err != nil {
			t.Fatal(err)
		}
		recs.Write(data)
		recs.WriteByte('\n')
	}
	for name, text := range map[string]string{
		".gitignore":                "/build/\n",
		ingest.CatalogueFile:        "sources:\n  - id: bench\n    name: The records of the agent suite\n    licence: CC0-1.0\n    class: open\n",
		"build/records/bench.jsonl": recs.String(),
	} {
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
	return &suite{dir: dir}
}

// reset puts the tree back at the base commit, and keeps the records, which git ignores.
func (s *suite) reset(t *testing.T) {
	t.Helper()
	for _, args := range [][]string{{"checkout", "-q", "--", "."}, {"clean", "-q", "-f", "-d"}} {
		if out, err := exec.Command("git", append([]string{"-C", s.dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// run runs the role on the record of the item with the model and the judge, and puts the tree back.
func (s *suite) run(t *testing.T, it item, model, judge llm.Model, r int) *agent.Record {
	t.Helper()
	defer s.reset(t)
	ctx := context.Background()
	ws, err := workspace.Open(ctx, s.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	role, err := agent.LoadRole(prompts.FS, "curate")
	if err != nil {
		t.Fatal(err)
	}
	job := &agent.Job{ID: fmt.Sprintf("j-2026-10-26-%d", it.n), Role: "curate", Target: it.ref(), Goal: "the record has a node in the words of math2", OnBehalfOf: "schedule:agent-suite"}
	rs := &curate.Records{Root: s.dir}
	brief, err := curate.Brief(ws, job, rs)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tools.Profile(role, nil, ws, nil, curate.Read(rs), curate.Propose(ws, "m2-agent/curate@"+role.Version))
	if err != nil {
		t.Fatal(err)
	}
	meter := &agent.Meter{Model: judge}
	rn := agent.Runner{Role: role, Model: model, Tools: ts, Meter: meter, Goal: curate.Goal(ws, job, rs, meter, role.Ladder[r].Model, role.Checks["judge"])}
	rec, err := rn.Run(ctx, job, r, brief)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

var u = llm.Usage{Input: 1000, Output: 200}

func write(text string) *llm.Response {
	return llmtest.Call("write_file", map[string]string{"path": path, "text": text}, u)
}

func stuck() *llm.Response {
	return llmtest.Call(agent.StuckTool, map[string]string{"reason": "the attempt does not work"}, u)
}

func verdict(v string) *llm.Response {
	return llmtest.Call("judge", map[string]string{"verdict": v, "why": "the scripted verdict."}, u)
}

// attempt is a scripted attempt at an item, the scripted verdicts of the judge, and the reason that the goal must give after it. An empty reason means that the goal must hold.
type attempt struct {
	name     string
	steps    []*llm.Response
	verdicts []*llm.Response
	why      string
}

// attempts are the reference and the wrong attempts at the item.
func attempts(it item) []attempt {
	read := llmtest.Call("record", map[string]string{"ref": it.ref()}, u)
	kind, other := it.kind()
	out := []attempt{
		{"reference", []*llm.Response{read, write(it.node(kind, it.locator(), it.Body))}, []*llm.Response{verdict(curate.Same)}, ""},
		{"copy", []*llm.Response{read, write(it.node(kind, it.locator(), it.Text)), stuck()}, nil, "copies the text of the record"},
		{"no locator", []*llm.Response{read, write(it.node(kind, "", it.Body)), stuck()}, nil, "has not added a node with a source whose locator is " + it.locator()},
		{"wrong kind", []*llm.Response{read, write(it.node(other, it.locator(), it.Body)), stuck()}, nil, "is a " + kind + " node, and it is a " + other + " node"},
	}
	for _, w := range it.Wrong {
		out = append(out, attempt{w.Name, []*llm.Response{read, write(it.node(kind, it.locator(), w.Body)), stuck()}, []*llm.Response{verdict(w.Verdict)}, "is " + w.Verdict + " to the record, not the same"})
	}
	return out
}

func TestSuite(t *testing.T) {
	items := readItems(t)
	s := setup(t, items)
	if *live {
		liveRun(t, s, items)
		return
	}
	solved := 0
	for _, it := range items {
		for _, a := range attempts(it) {
			t.Run(it.ID+"/"+a.name, func(t *testing.T) {
				rec := s.run(t, it, llmtest.New(a.steps...), llmtest.New(a.verdicts...), 0)
				whys := make([]string, len(rec.Turns))
				for i, turn := range rec.Turns {
					whys[i] = turn.Why
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
	writeReport(t, fmt.Sprintf("# The suite of the curate role\n\nThe references solved %d of %d items with the scripted model, and the goal refused each wrong attempt.\n", solved, len(items)))
}

// liveRun runs the role and the judge with the Claude model of the rung.
func liveRun(t *testing.T, s *suite, items []item) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Fatal("-live needs ANTHROPIC_API_KEY")
	}
	role, err := agent.LoadRole(prompts.FS, "curate")
	if err != nil {
		t.Fatal(err)
	}
	m := claude.New()
	var runs, judged []string
	solved, right, total, usd := 0, 0, 0, 0.0
	for _, it := range items {
		rec := s.run(t, it, m, m, *rung)
		if rec.Outcome == agent.Success {
			solved++
		}
		usd += rec.Spend.USD
		runs = append(runs, fmt.Sprintf("| %s | %s | %d | %.4f | %s |", it.ID, rec.Outcome, len(rec.Turns), rec.Spend.USD, cell(rec.Reason)))

		r, err := (&curate.Records{Root: s.dir}).Get(it.ref())
		if err != nil {
			t.Fatal(err)
		}
		kind, _ := it.kind()
		cases := []struct{ name, body, want string }{{"reference", it.Body, curate.Same}}
		for _, w := range it.Wrong {
			cases = append(cases, struct{ name, body, want string }{w.Name, w.Body, w.Verdict})
		}
		for _, c := range cases {
			v, err := curate.Judge(context.Background(), m, role.Ladder[*rung].Model, role.Checks["judge"], r, path, []byte(it.node(kind, it.locator(), c.body)))
			if err != nil {
				t.Fatal(err)
			}
			total++
			if v.Verdict == c.want {
				right++
			}
			judged = append(judged, fmt.Sprintf("| %s | %s | %s | %s | %s |", it.ID, c.name, c.want, v.Verdict, cell(v.Why)))
		}
	}
	writeReport(t, fmt.Sprintf("# The suite of the curate role\n\nThe role solved %d of %d items within the budget, for $%.4f, at the rung %d of the ladder.\n\n| Item | Outcome | Turns | USD | Reason |\n|---|---|---|---|---|\n%s\n\nThe judge gave the right verdict for %d of %d nodes.\n\n| Item | Node | Want | Verdict | Reason |\n|---|---|---|---|---|\n%s\n",
		solved, len(items), usd, *rung, strings.Join(runs, "\n"), right, total, strings.Join(judged, "\n")))
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
