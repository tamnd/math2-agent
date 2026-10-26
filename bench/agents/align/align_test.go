// Package align_test is the suite of the align role (spec 17.3.3 and 17.3.4).
//
// items.yaml has the records of the sources wikidata, arxiv and mathlib, the type hashes of the Math2 bindings in the build, the edges that a person rejected, and the items. Each item is a target node of tree/ and its reference change: the proposal of an edge to a record, or a Mathlib binding with the type hash of the Math2 binding. A wrong attempt has the verdict that the judge must give it, or the reason that the role must give with no judge. The records are made up, and the texts are in our own words. These are the public items. The held-out items are in a private repository, and -items reads them from a file of the same form.
//
// The test makes a work tree of math2 with the nodes of tree/, a catalogue with the three sources, their records in build/records/ and the register of the decisions. Then it runs the role on each item, and puts the tree back after each run.
//
// With no flags, the model and the judge are scripted, and no test calls a model service. For each item, the script first makes the reference change, the judge gives the verdict of the item, and the goal must hold. Then the script makes wrong attempts, and the run must fail with the reason that the attempt must give. A run with no proposal, an aligned edge to a record that is not a paper, a same_as to a paper and a same_as to Mathlib fail with no judge, and so do the wrong attempts of the items with a reason. The other wrong attempts fail with the verdict of the judge. Thus the suite checks the goal and the tool of the role, and each item has a solution.
//
// With -live, the test runs the role once on each target with the Claude model of the rung -rung of the ladder, and the same model judges. The report gives the precision of the changes of the role, which is the share of its proposals and bindings that are the reference change of an item, and the recall, which is the share of the items whose reference change the role made. Then the judge alone grades the reference and each wrong attempt with a verdict, and the report gives the share of the verdicts that are right. It needs ANTHROPIC_API_KEY, and it spends money. Give -report FILE to write the report.
package align_test

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
	"github.com/tamnd/math2-agent/agent/align"
	"github.com/tamnd/math2-agent/agent/curate"
	"github.com/tamnd/math2-agent/agent/tools"
	aligner "github.com/tamnd/math2-agent/align"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/claude"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/workspace"
)

var (
	itemsFile = flag.String("items", "items.yaml", "the file of the items")
	live      = flag.Bool("live", false, "run the role and the judge with the Claude models. This needs ANTHROPIC_API_KEY and spends money.")
	rung      = flag.Int("rung", 0, "the rung of the ladder of the role, with -live")
	report    = flag.String("report", "", "write the report of the run to this file")
)

// mathlib is the edge of an item whose change is a Mathlib binding.
const mathlib = "mathlib"

// change is a change of the role: an edge to a record, or a Mathlib binding.
type change struct {
	Name    string `yaml:"name"`
	Edge    string `yaml:"edge"`    // same_as, close_to, aligned or mathlib
	Ref     string `yaml:"ref"`     // the source and the id of the record
	Match   string `yaml:"match"`   // the match of a Mathlib binding
	Verdict string `yaml:"verdict"` // the verdict of the judge
	Why     string `yaml:"why"`     // the reason of a wrong attempt that fails with no judge
}

// item is one item of items.yaml.
type item struct {
	Target string `yaml:"target"`
	change `yaml:",inline"`
	Wrong  []change `yaml:"wrong"`

	n int // the number of the item in the file, from 1
}

// file is the file of the items.
type file struct {
	Records map[string][]struct {
		ID       string `yaml:"id"`
		Title    string `yaml:"title"`
		Text     string `yaml:"text"`
		Locator  string `yaml:"locator"`
		TypeHash string `yaml:"type_hash"`
	} `yaml:"records"`
	Math2    map[string]string `yaml:"math2"` // the type hash of each Math2 declaration
	Rejected []struct {
		Target string `yaml:"target"`
		Edge   string `yaml:"edge"`
		Ref    string `yaml:"ref"`
	} `yaml:"rejected"`
	Items []item `yaml:"items"`
}

var verdicts = []string{curate.Same, curate.Generalizes, curate.Specializes, curate.Related, curate.Different}

func readItems(t *testing.T) *file {
	t.Helper()
	data, err := os.ReadFile(*itemsFile)
	if err != nil {
		t.Fatal(err)
	}
	var f file
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		t.Fatalf("%s: %v", *itemsFile, err)
	}
	for i := range f.Items {
		it := &f.Items[i]
		it.n = i + 1
		if it.Edge == mathlib && it.Match == "" || it.Edge != mathlib && (it.Verdict == "" || it.Verdict == curate.Different) {
			t.Fatalf("%s: the reference of the item %d has the edge %s, the match %q and the verdict %q", *itemsFile, it.n, it.Edge, it.Match, it.Verdict)
		}
		for _, w := range it.Wrong {
			if w.Why == "" && !slices.Contains(verdicts, w.Verdict) || w.Why != "" && w.Verdict != "" {
				t.Fatalf("%s: the wrong attempt %q of the item %d needs a verdict or a reason", *itemsFile, w.Name, it.n)
			}
		}
	}
	return &f
}

// suite is the work tree of the items and the status of its build.
type suite struct {
	dir string
	st  []status.Status
}

func setup(t *testing.T, f *file) *suite {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("tree")); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		".gitignore":         "/build/\n",
		ingest.CatalogueFile: "sources:\n  - id: wikidata\n    name: Wikidata\n    licence: CC0-1.0\n    class: open\n  - id: arxiv\n    name: arXiv\n    licence: CC0-1.0\n    class: record\n  - id: mathlib\n    name: Mathlib\n    licence: Apache-2.0\n    class: open\n",
	}
	for source, recs := range f.Records {
		var b strings.Builder
		for _, r := range recs {
			rec := ingest.Record{ID: r.ID, Title: r.Title, Text: r.Text}
			switch source {
			case "wikidata":
				rec.Kind, rec.URL = "item", "https://www.wikidata.org/wiki/"+r.ID
			case "arxiv":
				rec.Kind, rec.URL = "paper", "https://arxiv.org/abs/"+r.ID
			case mathlib:
				rec.Kind, rec.Locator, rec.Meta = "theorem", r.Locator, map[string]string{"type_hash": r.TypeHash}
			}
			data, err := json.Marshal(rec)
			if err != nil {
				t.Fatal(err)
			}
			b.Write(data)
			b.WriteByte('\n')
		}
		files["build/records/"+source+".jsonl"] = b.String()
	}
	var reg strings.Builder
	for _, r := range f.Rejected {
		source, id, _ := strings.Cut(r.Ref, ":")
		system, _ := aligner.System(source)
		fmt.Fprintf(&reg, "%s %s %s %s rejected github:bench 2026-10-01 -\n", r.Target, r.Edge, system, id)
	}
	files[aligner.Register] = reg.String()
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
	nodes, err := node.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	records := map[string]lean.Record{}
	for decl, hash := range f.Math2 {
		records[decl] = lean.Record{Decl: decl, Kind: "theorem", TypeHash: hash}
	}
	return &suite{dir: dir, st: status.Compute(status.Input{Nodes: nodes, Records: records})}
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

// run runs the role on the target with the model and the judge, and puts the tree back. It returns the record and the changes of the run.
func (s *suite) run(t *testing.T, target string, n int, model, judge llm.Model, r int) (*agent.Record, []workspace.Change) {
	t.Helper()
	defer s.reset(t)
	ctx := context.Background()
	ws, err := workspace.Open(ctx, s.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	role, err := agent.LoadRole(prompts.FS, "align")
	if err != nil {
		t.Fatal(err)
	}
	job := &agent.Job{ID: fmt.Sprintf("j-2026-10-26-%04d", n), Role: "align", Target: target, Goal: "the node has a proposal of each crosswalk edge that the records give", OnBehalfOf: "schedule:agent-suite"}
	rs := &curate.Records{Root: s.dir}
	brief, err := align.Brief(ws, job)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tools.Profile(role, nil, ws, nil, curate.Read(rs), align.Propose(ws, rs, "m2-agent/align@"+role.Version))
	if err != nil {
		t.Fatal(err)
	}
	meter := &agent.Meter{Model: judge}
	rn := agent.Runner{Role: role, Model: model, Tools: ts, Meter: meter, Goal: align.Goal(ws, job, rs, s.st, meter, role.Ladder[r].Model, role.Checks["judge"])}
	rec, err := rn.Run(ctx, job, r, brief)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := ws.Changes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return rec, changes
}

var u = llm.Usage{Input: 1000, Output: 200}

func stuck() *llm.Response {
	return llmtest.Call(agent.StuckTool, map[string]string{"reason": "the attempt does not work"}, u)
}

func verdict(v string) *llm.Response {
	return llmtest.Call("judge", map[string]string{"verdict": v, "why": "the scripted verdict."}, u)
}

// bound is the node text with a Mathlib binding appended to its formal field.
func bound(text, decl, match string) string {
	return strings.Replace(text, "    match: exact\n---", "    match: exact\n  - system: lean4\n    library: mathlib\n    decl: "+decl+"\n    match: "+match+"\n---", 1)
}

// steps are the calls that make the change on the target, whose node file is text.
func steps(target, text string, c change) []*llm.Response {
	read := llmtest.Call("record", map[string]string{"ref": c.Ref}, u)
	if c.Edge == mathlib {
		_, decl, _ := strings.Cut(c.Ref, ":")
		return []*llm.Response{read, llmtest.Call("write_file", map[string]string{"path": tools.NodePath(target), "text": bound(text, decl, c.Match)}, u)}
	}
	return []*llm.Response{read, llmtest.Call("propose_edge", map[string]string{"edge": c.Edge, "ref": c.Ref, "why": "the scripted reason."}, u)}
}

// attempt is a scripted attempt at an item, the scripted verdicts of the judge, and the reason that the run must give after it. An empty reason means that the goal must hold.
type attempt struct {
	name     string
	steps    []*llm.Response
	verdicts []*llm.Response
	why      string
}

// attempts are the reference and the wrong attempts at the item, whose target node is text at the base commit.
func attempts(it item, text string) []attempt {
	ref := attempt{name: "reference", steps: steps(it.Target, text, it.change)}
	if it.Edge != mathlib {
		ref.verdicts = []*llm.Response{verdict(it.Verdict)}
	}
	out := []attempt{
		ref,
		{"no change", []*llm.Response{llmtest.Call("record", map[string]string{"ref": it.Ref}, u), stuck()}, nil, "the run has not proposed an edge yet"},
		{"same_as to Mathlib", append(steps(it.Target, text, change{Edge: aligner.SameAs, Ref: "mathlib:Subgroup.card_subgroup_dvd_card"}), stuck()), nil, "through its formal binding and not through same_as"},
	}
	switch source, _, _ := strings.Cut(it.Ref, ":"); source {
	case "arxiv":
		out = append(out, attempt{"same_as to a paper", append(steps(it.Target, text, change{Edge: aligner.SameAs, Ref: it.Ref}), stuck()), nil, "a record of arxiv is a paper"})
	case "wikidata":
		out = append(out, attempt{"aligned to an item", append(steps(it.Target, text, change{Edge: aligner.Aligned, Ref: it.Ref}), stuck()), nil, "only a paper gets an aligned edge"})
	}
	for _, w := range it.Wrong {
		a := attempt{name: w.Name, steps: append(steps(it.Target, text, w), stuck()), why: w.Why}
		switch {
		case w.Why != "":
		case w.Verdict == curate.Different:
			a.verdicts, a.why = []*llm.Response{verdict(w.Verdict)}, "are about different things"
		case w.Edge == aligner.SameAs && w.Verdict != curate.Same:
			a.verdicts, a.why = []*llm.Response{verdict(w.Verdict)}, "not the same"
		}
		out = append(out, a)
	}
	return out
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

func (s *suite) text(t *testing.T, target string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.dir, filepath.FromSlash(tools.NodePath(target))))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSuite(t *testing.T) {
	f := readItems(t)
	s := setup(t, f)
	if *live {
		liveRun(t, s, f.Items)
		return
	}
	solved := 0
	for _, it := range f.Items {
		for _, a := range attempts(it, s.text(t, it.Target)) {
			t.Run(fmt.Sprintf("%d-%s/%s", it.n, it.Target, a.name), func(t *testing.T) {
				rec, _ := s.run(t, it.Target, it.n, llmtest.New(a.steps...), llmtest.New(a.verdicts...), 0)
				whys := reasons(rec)
				if a.why == "" {
					if rec.Outcome != agent.Success {
						t.Fatalf("the reference gives %s, %q; the run said %q", rec.Outcome, rec.Reason, whys)
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
	writeReport(t, fmt.Sprintf("# The suite of the align role\n\nThe references solved %d of %d items with the scripted model, and the run refused each wrong attempt.\n", solved, len(f.Items)))
}

// key is the key of a change of the role on the target: the edge and the id or the URI of the record, or mathlib and the declaration.
func key(rs *curate.Records, target string, c change) (string, error) {
	if c.Edge == mathlib {
		_, decl, _ := strings.Cut(c.Ref, ":")
		return mathlib + " " + decl, nil
	}
	r, err := rs.Get(c.Ref)
	if err != nil {
		return "", err
	}
	system, _ := aligner.System(r.Source.ID)
	return aligner.NewProposal(target, c.Edge, system, aligner.Target(c.Edge, r.Record)).ID, nil
}

// made returns the keys of the changes of a run on the target: the IDs of the proposals of the role, and mathlib and the declaration of each binding.
func made(t *testing.T, target string, changes []workspace.Change) []string {
	t.Helper()
	var out []string
	for _, c := range changes {
		if c.Path == tools.NodePath(target) {
			bs, err := align.Bindings(c)
			if err != nil {
				t.Fatal(err)
			}
			for _, b := range bs {
				out = append(out, fmt.Sprint(mathlib, " ", b["decl"]))
			}
			continue
		}
		ps, _ := aligner.ParseQueue(c.New)
		for _, p := range ps {
			if strings.HasPrefix(p.By, "m2-agent/align@") {
				out = append(out, p.ID)
			}
		}
	}
	return out
}

// liveRun runs the role once on each target and the judge with the Claude model of the rung.
func liveRun(t *testing.T, s *suite, items []item) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Fatal("-live needs ANTHROPIC_API_KEY")
	}
	role, err := agent.LoadRole(prompts.FS, "align")
	if err != nil {
		t.Fatal(err)
	}
	rs := &curate.Records{Root: s.dir}
	m := claude.New()
	var targets, runs, judged []string
	for _, it := range items {
		if !slices.Contains(targets, it.Target) {
			targets = append(targets, it.Target)
		}
	}
	found, right, total, good, all, usd := 0, 0, 0, 0, 0, 0.0
	for i, target := range targets {
		rec, changes := s.run(t, target, i+1, m, m, *rung)
		usd += rec.Spend.USD
		keys := made(t, target, changes)
		var refs []string
		for _, it := range items {
			if it.Target != target {
				continue
			}
			k, err := key(rs, target, it.change)
			if err != nil {
				t.Fatal(err)
			}
			refs = append(refs, k)
			if slices.Contains(keys, k) {
				found++
			}
		}
		for _, k := range keys {
			all++
			if slices.Contains(refs, k) {
				good++
			}
		}
		runs = append(runs, fmt.Sprintf("| %s | %s | %d | %d | %.4f | %s |", target, rec.Outcome, len(keys), len(rec.Turns), rec.Spend.USD, cell(rec.Reason)))
	}
	for _, it := range items {
		text := s.text(t, it.Target)
		cases := []change{it.change}
		cases = append(cases, it.Wrong...)
		for _, c := range cases {
			if c.Edge == mathlib || c.Verdict == "" {
				continue
			}
			r, err := rs.Get(c.Ref)
			if err != nil {
				t.Fatal(err)
			}
			v, err := curate.Judge(context.Background(), m, role.Ladder[*rung].Model, role.Checks["judge"], r, tools.NodePath(it.Target), []byte(text))
			if err != nil {
				t.Fatal(err)
			}
			total++
			if v.Verdict == c.Verdict {
				right++
			}
			judged = append(judged, fmt.Sprintf("| %s | %s | %s | %s | %s |", it.Target, c.Ref, c.Verdict, v.Verdict, cell(v.Why)))
		}
	}
	writeReport(t, fmt.Sprintf("# The suite of the align role\n\nThe role ran once on each of %d targets, for $%.4f, at the rung %d of the ladder. %d of its %d proposals and bindings are reference changes, and it made the reference changes of %d of %d items.\n\n| Target | Outcome | Changes | Turns | USD | Reason |\n|---|---|---|---|---|---|\n%s\n\nThe judge gave the right verdict for %d of %d records.\n\n| Target | Record | Want | Verdict | Reason |\n|---|---|---|---|---|\n%s\n",
		len(targets), usd, *rung, good, all, found, len(items), strings.Join(runs, "\n"), right, total, strings.Join(judged, "\n")))
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
