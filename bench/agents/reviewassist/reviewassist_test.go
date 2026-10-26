// Package reviewassist_test is the suite of the review-assist role (spec 17.3.4).
//
// Each item of items.yaml is a node with one Lean binding, the Lean record of the binding, and the verdict that a review gave it. A not-faithful item has the class of spec 10.5 that the critic must name and a reference concern. A faithful item has no concern. The items are in our own words, and the records are written by hand. These are the public items. The held-out items are in a private repository, and -items reads them from a file of the same form.
//
// The test makes a work tree of math2 with a node for each item, and the review input of a build from the nodes and the records. Then it runs the role on each item, and puts the tree back after each run.
//
// With no flags, the model is scripted, and no test calls a model service. For each item, the script first writes the packet with the reference concern, and the goal must hold. Then the script makes wrong attempts, and the run must fail with the reason that the attempt must give: no packet, a concern of a binding that the packet does not have, a class that spec 10.5 does not have, too many concerns, and a write of the node. Thus the suite checks the goal and the tool of the role, and each item has a solution.
//
// With -live, the test runs the role on each item with the Claude model of the rung -rung of the ladder. The report gives the recall of the critic, which is the share of the not-faithful items whose packet has a concern of the class of the item, the concerns of the faithful items, and the length of the packets. It needs ANTHROPIC_API_KEY, and it spends money. Give -report FILE to write the report.
package reviewassist_test

import (
	"context"
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
	"github.com/tamnd/math2-agent/agent/reviewassist"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/battery"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/claude"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/review"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/tag"
	"github.com/tamnd/math2-agent/workspace"
)

var (
	itemsFile = flag.String("items", "items.yaml", "the file of the items")
	live      = flag.Bool("live", false, "run the role with the Claude models. This needs ANTHROPIC_API_KEY and spends money.")
	rung      = flag.Int("rung", 0, "the rung of the ladder of the role, with -live")
	report    = flag.String("report", "", "write the report of the run to this file")
)

// item is one item of items.yaml.
type item struct {
	Target    string   `yaml:"target"`
	Name      string   `yaml:"name"`
	Statement string   `yaml:"statement"`
	Decl      string   `yaml:"decl"`
	Lean      string   `yaml:"lean"`
	LeanAll   string   `yaml:"lean_all"`
	Constants []string `yaml:"constants"`
	Verdict   string   `yaml:"verdict"` // faithful or not-faithful
	Class     string   `yaml:"class"`   // the class of a not-faithful item
	Concern   string   `yaml:"concern"` // the reference concern of a not-faithful item

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
		switch {
		case !tag.IsTag(it.Target) || tag.Excluded(it.Target):
			t.Fatalf("%s: %q is not a tag", *itemsFile, it.Target)
		case it.Verdict == "faithful" && (it.Class != "" || it.Concern != ""):
			t.Fatalf("%s: the faithful item %s has a class or a concern", *itemsFile, it.Target)
		case it.Verdict == "not-faithful" && (!slices.Contains(battery.Classes(), it.Class) || it.Concern == ""):
			t.Fatalf("%s: the not-faithful item %s has the class %q and no concern", *itemsFile, it.Target, it.Class)
		case it.Verdict != "faithful" && it.Verdict != "not-faithful":
			t.Fatalf("%s: the item %s has the verdict %q", *itemsFile, it.Target, it.Verdict)
		}
	}
	return f.Items
}

// text is the node of the item.
func (it item) text() string {
	return fmt.Sprintf("---\ntag: %s\nkind: statement\nrole: theorem\nname: %s\nformal:\n  - system: lean4\n    library: math2\n    decl: %s\n    match: exact\n---\n\n## Statement\n\n%s\n", it.Target, it.Name, it.Decl, it.Statement)
}

// reference is the concerns of the reference packet of the item, in the form of write_packet.
func (it item) reference() string {
	if it.Verdict == "faithful" {
		return ""
	}
	return it.Decl + " " + it.Class + ": " + it.Concern
}

// suite is the work tree of the items and the review input of its build.
type suite struct {
	dir string
	in  review.Input
}

func setup(t *testing.T, items []item) *suite {
	t.Helper()
	dir := t.TempDir()
	records := map[string]lean.Record{}
	for _, it := range items {
		p := filepath.Join(dir, filepath.FromSlash(tools.NodePath(it.Target)))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(it.text()), 0o600); err != nil {
			t.Fatal(err)
		}
		records[it.Decl] = lean.Record{Decl: it.Decl, Kind: "theorem", TypeHash: "blake3:" + strings.ToLower(it.Target), StatementPP: it.Lean, StatementPPAll: it.LeanAll, UsesConstantsInStatement: it.Constants}
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
	nodes, err := node.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	st := status.Compute(status.Input{Nodes: nodes, Records: records})
	return &suite{dir: dir, in: review.Input{Nodes: nodes, Status: st, Records: records}}
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

// run runs the role on the item with the model, and puts the tree back. It returns the record and the packet that the run wrote, or nil.
func (s *suite) run(t *testing.T, it item, model llm.Model, r int) (*agent.Record, *reviewassist.File) {
	t.Helper()
	defer s.reset(t)
	ctx := context.Background()
	ws, err := workspace.Open(ctx, s.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	role, err := agent.LoadRole(prompts.FS, "review-assist")
	if err != nil {
		t.Fatal(err)
	}
	job := &agent.Job{ID: fmt.Sprintf("j-2026-10-26-%04d", it.n), Role: "review-assist", Target: it.Target, Goal: "the node has a review packet with the concerns of the critic", OnBehalfOf: "schedule:agent-suite"}
	brief, err := reviewassist.Brief(ws, s.in, job)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tools.Profile(role, nil, ws, nil, reviewassist.Write(ws, s.in, "m2-agent/review-assist@"+role.Version))
	if err != nil {
		t.Fatal(err)
	}
	rn := agent.Runner{Role: role, Model: model, Tools: ts, Goal: reviewassist.Goal(ws, s.in, job)}
	rec, err := rn.Run(ctx, job, r, brief)
	if err != nil {
		t.Fatal(err)
	}
	data, err := ws.Read(reviewassist.Path(it.Target))
	if err != nil {
		return rec, nil
	}
	f, err := reviewassist.ParseFile(data)
	if err != nil {
		return rec, nil
	}
	return rec, f
}

var u = llm.Usage{Input: 1000, Output: 200}

func stuck() *llm.Response {
	return llmtest.Call(agent.StuckTool, map[string]string{"reason": "the attempt does not work"}, u)
}

func write(concerns string) *llm.Response {
	return llmtest.Call("write_packet", map[string]string{"concerns": concerns}, u)
}

// attempt is a scripted attempt at an item and the reason that the run must give after it. An empty reason means that the goal must hold.
type attempt struct {
	name  string
	steps []*llm.Response
	why   string
}

func attempts(it item) []attempt {
	many := strings.Repeat(it.Decl+" M-conv: the person should check the convention.\n", reviewassist.MaxConcerns+1)
	return []attempt{
		{"reference", []*llm.Response{write(it.reference())}, ""},
		{"no packet", []*llm.Response{llmtest.Call("read_file", map[string]string{"path": tools.NodePath(it.Target)}, u), stuck()}, "the run has not written the packet " + reviewassist.Path(it.Target)},
		{"another binding", []*llm.Response{write("Bench.other M-junk: the statement uses a junk value."), stuck()}, "the packet has no binding of Bench.other"},
		{"no class", []*llm.Response{write(it.Decl + " M-wrong: the statement is wrong."), stuck()}, "M-wrong is not a class of spec 10.5"},
		{"too many concerns", []*llm.Response{write(many), stuck()}, fmt.Sprintf("the packet has %d concerns", reviewassist.MaxConcerns+1)},
		{"a write of the node", []*llm.Response{llmtest.Call("write_file", map[string]string{"path": tools.NodePath(it.Target), "text": it.text()}, u), stuck()}, "The role has no tool write_file."},
	}
}

// reasons are the reason of the run, the reasons of the goal after each turn, and the results of the tools that failed.
func reasons(rec *agent.Record) []string {
	out := []string{rec.Reason}
	for _, turn := range rec.Turns {
		out = append(out, turn.Why)
		for _, r := range turn.Results {
			if r.IsError {
				out = append(out, r.Content)
			}
		}
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
			t.Run(it.Target+"/"+a.name, func(t *testing.T) {
				rec, f := s.run(t, it, llmtest.New(a.steps...), 0)
				whys := reasons(rec)
				if a.why == "" {
					if rec.Outcome != agent.Success || f == nil {
						t.Fatalf("the reference gives %s, %q; the goal said %q", rec.Outcome, rec.Reason, whys)
					}
					if found(it, f) != (it.Verdict == "not-faithful") {
						t.Errorf("the reference packet has the concerns %+v", f.Concerns)
					}
					solved++
					return
				}
				if rec.Outcome == agent.Success || !slices.ContainsFunc(whys, func(w string) bool { return strings.Contains(w, a.why) }) {
					t.Errorf("the attempt gives %s, %q, and the run said %q; it must say %q", rec.Outcome, rec.Reason, whys, a.why)
				}
			})
		}
	}
	writeReport(t, fmt.Sprintf("# The suite of the review-assist role\n\nThe references solved %d of %d items with the scripted model, and the goal refused each wrong attempt.\n", solved, len(items)))
}

// found reports whether the packet has a concern of the binding of the item with the class of the item.
func found(it item, f *reviewassist.File) bool {
	return slices.ContainsFunc(f.Concerns, func(c reviewassist.Concern) bool { return c.Decl == it.Decl && c.Class == it.Class })
}

// liveRun runs the role with the Claude model of the rung, and measures the recall of the critic and the length of the packets.
func liveRun(t *testing.T, s *suite, items []item) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Fatal("-live needs ANTHROPIC_API_KEY")
	}
	m := claude.New()
	var rows []string
	solved, faults, caught, alarms, packets, concerns, chars, usd := 0, 0, 0, 0, 0, 0, 0, 0.0
	for _, it := range items {
		rec, f := s.run(t, it, m, *rung)
		if rec.Outcome == agent.Success {
			solved++
		}
		usd += rec.Spend.USD
		var classes []string
		if f != nil {
			packets++
			concerns += len(f.Concerns)
			for _, c := range f.Concerns {
				chars += len([]rune(c.Text))
				classes = append(classes, c.Class)
			}
		}
		switch {
		case it.Verdict == "not-faithful":
			faults++
			if f != nil && found(it, f) {
				caught++
			}
		case f != nil:
			alarms += len(f.Concerns)
		}
		rows = append(rows, fmt.Sprintf("| %s | %s | %s | %s | %s | %d | %.4f |", it.Target, it.Verdict, it.Class, strings.Join(classes, ", "), rec.Outcome, len(rec.Turns), rec.Spend.USD))
	}
	mean := 0.0
	if packets > 0 {
		mean = float64(concerns) / float64(packets)
	}
	writeReport(t, fmt.Sprintf("# The suite of the review-assist role\n\nThe role wrote %d packets for %d items, for $%.4f, at the rung %d of the ladder, and %d runs succeeded.\n\nThe recall of the critic is %d of %d: the packets of %d not-faithful items have a concern of the class of the review. The packets of the faithful items have %d concerns. A packet has %.1f concerns on average, with %d characters of reasons in all.\n\n| Item | Verdict | Class | Concerns | Outcome | Turns | USD |\n|---|---|---|---|---|---|---|\n%s\n",
		packets, len(items), usd, *rung, solved, caught, faults, caught, alarms, mean, chars, strings.Join(rows, "\n")))
}

func writeReport(t *testing.T, text string) {
	t.Helper()
	if *report == "" {
		return
	}
	if err := os.WriteFile(*report, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}
