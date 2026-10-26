// Package explore_test is the suite of the explore role (spec 17.3.4).
//
// The items are planted problems. Each target node of tree/ is a plausible statement that is false, and each item of items.yaml has the program that finds the counterexample, the standard output of the program, and the reference counterexample. The texts are in our own words. These are the public items. The held-out items are in a private repository, and -items reads them from a file of the same form.
//
// The test makes a work tree of math2 from tree/, and runs the role on each item on behalf of the project agent-suite. It puts the tree back after each run. The novelty check searches the graph of a build of tree/. The searches of the web are not in the suite, because a run sends each query to the service.
//
// With no flags, the model and the judge of the novelty check are scripted, and the sandbox is a fake that gives the output of the item for the program of the item. No test calls a model service or runs a container. For each item, the script first writes the program and the computation node, runs the computation, looks up the claim and files the counterexample, and the goal must hold. Then the script writes wrong attempts, and the goal or the tool must refuse it with the reason that the attempt must give: no output, an output that says that it is new, a conjecture that no computation checks, a counterexample with no computation, a wrong output hash, and a change of the target. Thus the suite checks the goal of the role and that each item has a solution.
//
// With -live, the test runs the role on each item with the Claude model of the rung -rung of the ladder, and the same model judges the matches of the novelty check. The computations run in docker, thus -live also needs the manifest of the images with -images, for example manifests/images.yaml of math2, and the images. The report gives the refutations found, the conjectures that the runs filed, and the share of the runs that looked up each claim before they filed it. Each target is false, thus a person reads each conjecture of the report for a false conjecture that the falsify step did not catch. It needs ANTHROPIC_API_KEY, and it spends money. Give -report FILE to write the report.
package explore_test

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
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/explore"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/compute"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/claude"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/novelty"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/sandbox"
	"github.com/tamnd/math2-agent/search"
	"github.com/tamnd/math2-agent/tag"
	"github.com/tamnd/math2-agent/workspace"
)

var (
	itemsFile  = flag.String("items", "items.yaml", "the file of the items")
	live       = flag.Bool("live", false, "run the role and the judge with the Claude models and the computations in docker. This needs ANTHROPIC_API_KEY and spends money.")
	rung       = flag.Int("rung", 0, "the rung of the ladder of the role, with -live")
	imagesFile = flag.String("images", "", "the manifest of the images of the sandbox, with -live")
	report     = flag.String("report", "", "write the report of the run to this file")
)

// item is one item of items.yaml.
type item struct {
	Target string `yaml:"target"`
	Claim  string `yaml:"claim"`  // the claim of the computation
	Code   string `yaml:"code"`   // the program, in Python
	Output string `yaml:"output"` // the standard output of the program
	Name   string `yaml:"name"`   // the name of the counterexample
	Text   string `yaml:"text"`   // the text of the counterexample

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
		if tools.NodePath(it.Target) == "" || it.Code == "" || it.Output == "" || it.Name == "" || it.Text == "" {
			t.Fatalf("%s: the item %d has no target, program, output, name or text", *itemsFile, it.n)
		}
	}
	return f.Items
}

// The provisional ids of the computation and the output of the item.
func (it item) comp() string { return fmt.Sprintf("~c%07x", it.n) }
func (it item) out() string  { return fmt.Sprintf("~e%07x", it.n) }

func (it item) codePath() string { return tag.CodeDir(it.comp()) + "/main.py" }

// computation is the computation node of the item with the image and the output hash.
func (it item) computation(image, hash string) string {
	return "---\ntag: " + it.comp() + "\nkind: computation\nname: A search for a counterexample to " + it.Target + "\nclaim: " + yamlString(it.Claim) + "\nengine:\n  image: " + image + "\n  entry: " + it.codePath() + "\n  resources: {cpu: 1, mem: 1G, wall: 1m}\noutput:\n  hash: " + hash + "\n  summary: " + yamlString(strings.TrimSpace(it.Output)) + "\n---\n\n## Description\n\nThe program tests the statement of " + it.Target + " on the first cases, and prints the first case where it fails.\n"
}

// counterexample is the counterexample node of the item with the refutes field and the text.
func (it item) counterexample(refutes, text string) string {
	return "---\ntag: " + it.out() + "\nkind: counterexample\nname: " + yamlString(it.Name) + "\n" + refutes + "---\n\n## Counterexample\n\n" + text + "\n"
}

func (it item) refutes() string {
	return "refutes:\n  - {target: " + it.Target + ", via: " + it.comp() + "}\n"
}

func yamlString(s string) string {
	data, err := yaml.Marshal(s)
	if err != nil {
		panic(err)
	}
	return strings.TrimSpace(string(data))
}

// fake is a sandbox that runs no program. It gives the output of an item for the program of the item, and exit code 1 for each other program.
type fake map[string]string

func (f fake) Run(_ context.Context, s sandbox.Spec) (*sandbox.Result, error) {
	entry := s.Command[len(s.Command)-1]
	data, err := os.ReadFile(filepath.Join(s.Dir, strings.TrimPrefix(entry, sandbox.WorkDir+"/")))
	if err != nil {
		return &sandbox.Result{ExitCode: 2, Stderr: []byte(err.Error())}, nil
	}
	res := &sandbox.Result{Runtime: "fake", Network: "none", Arch: "arm64", Duration: time.Second}
	out, ok := f[string(data)]
	if !ok {
		res.ExitCode = 1
		res.Stderr = []byte("the fake sandbox runs only the programs of the items")
		return res, nil
	}
	res.Stdout = []byte(out)
	return res, nil
}

// suite is the work tree of the items and the graph search of tree/.
type suite struct {
	dir    string
	graph  novelty.Searcher
	images *compute.Images
}

func setup(t *testing.T) *suite {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("tree")); err != nil {
		t.Fatal(err)
	}
	if *imagesFile != "" {
		data, err := os.ReadFile(*imagesFile)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(compute.ImagesFile)), data, 0o600); err != nil {
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
	im, err := compute.ReadImages(dir)
	if err != nil {
		t.Fatal(err)
	}

	out := t.TempDir()
	if _, err := build.Run(context.Background(), build.Options{Root: "tree", Out: out}); err != nil {
		t.Fatal(err)
	}
	ix, err := search.Open(filepath.Join(out, build.SearchFile), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	files, err := check.ReadTree("tree")
	if err != nil {
		t.Fatal(err)
	}
	g := &novelty.Graph{Index: ix}
	for _, f := range files {
		n, err := node.Parse(f.Path, f.Data)
		if err != nil {
			t.Fatal(err)
		}
		g.Nodes = append(g.Nodes, n)
	}
	return &suite{dir: dir, graph: g, images: im}
}

// image is the reference of the image of the suite, m2-python, with its digest.
func (s *suite) image(t *testing.T) string {
	t.Helper()
	for _, img := range s.images.Images {
		if img.Name == "m2-python" {
			for _, d := range img.Digests {
				return img.Name + "@" + d
			}
		}
	}
	t.Fatalf("%s has no image m2-python", compute.ImagesFile)
	return ""
}

// base is the target node of the item at the base commit.
func (s *suite) base(t *testing.T, it item) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.dir, filepath.FromSlash(tools.NodePath(it.Target))))
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

// run runs the role on the item with the model, the judge and the sandbox, and puts the tree back. It returns the record and the outputs that the run filed.
func (s *suite) run(t *testing.T, it item, model llm.Model, judge novelty.Judge, sb sandbox.Sandbox, r int) (*agent.Record, []*node.Node) {
	t.Helper()
	defer s.reset(t)
	ctx := context.Background()
	ws, err := workspace.Open(ctx, s.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	role, err := agent.LoadRole(prompts.FS, "explore")
	if err != nil {
		t.Fatal(err)
	}
	job := &agent.Job{ID: fmt.Sprintf("j-2026-10-26-%d", it.n), Role: "explore", Target: it.Target, Goal: "test the statement of the target on small cases, and file a counterexample if it fails", OnBehalfOf: "project:agent-suite"}
	runner := &compute.Runner{Images: s.images, Sandbox: sb, Out: t.TempDir()}
	brief, err := explore.Brief(ws, job, runner)
	if err != nil {
		t.Fatal(err)
	}
	nv := &explore.Novelty{Searchers: []novelty.Searcher{s.graph}, Judge: judge}
	ts, err := tools.Profile(role, nil, ws, nil, explore.Run(ws, runner), explore.Lookup(nv))
	if err != nil {
		t.Fatal(err)
	}
	rn := agent.Runner{Role: role, Model: model, Tools: ts, Goal: explore.Goal(ws, job, runner, nv)}
	rec, err := rn.Run(ctx, job, r, brief)
	if err != nil {
		t.Fatal(err)
	}
	rec.Novelty = nv.Results()
	changes, err := ws.Changes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	outputs, _, err := explore.Filed(changes)
	if err != nil {
		t.Fatal(err)
	}
	return rec, outputs
}

// lookedUp reports whether the run called lookup before each write of a conjecture or a counterexample node, in an earlier turn or earlier in the same turn.
func lookedUp(rec *agent.Record) bool {
	seen := false
	for _, turn := range rec.Turns {
		for _, b := range turn.Response.Content {
			switch {
			case b.Type != llm.ToolUse:
			case b.Name == "lookup":
				seen = true
			case b.Name == "write_file" && !seen:
				var in struct{ Path, Text string }
				if yaml.Unmarshal(b.Input, &in) != nil {
					continue
				}
				if n, err := node.Parse(in.Path, []byte(in.Text)); err == nil && slices.Contains(explore.Outputs, n.Kind()) {
					return false
				}
			}
		}
	}
	return true
}

// refuted reports whether an output is a counterexample that refutes the target of the item through a computation.
func refuted(it item, outputs []*node.Node) bool {
	for _, n := range outputs {
		list, _ := n.Meta["refutes"].([]any)
		for _, e := range list {
			if m, ok := e.(map[string]any); ok && n.Kind() == "counterexample" && m["target"] == it.Target && m["via"] != nil {
				return true
			}
		}
	}
	return false
}

var u = llm.Usage{Input: 1000, Output: 200}

func write(path, text string) *llm.Response {
	return llmtest.Call("write_file", map[string]string{"path": path, "text": text}, u)
}

// calls returns a response with the calls of the responses.
func calls(rs ...*llm.Response) *llm.Response {
	r := *rs[0]
	r.Content = nil
	for _, x := range rs {
		r.Content = append(r.Content, x.Content...)
	}
	return &r
}

func stuck() *llm.Response {
	return llmtest.Call(agent.StuckTool, map[string]string{"reason": "the attempt does not work"}, u)
}

// attempt is a scripted attempt at an item and the reason that the goal must give after it. An empty reason means that the goal must hold.
type attempt struct {
	name  string
	steps []*llm.Response
	why   string
}

// attempts are the reference and the wrong attempts at the item, whose target node is base at the base commit.
func attempts(it item, base, image string) []attempt {
	good := it.computation(image, compute.Hash([]byte(it.Output)))
	prog := calls(write(it.codePath(), it.Code), write("nodes/~/"+it.comp()+".md", good))
	file := func(text string) *llm.Response { return write("nodes/~/"+it.out()+".md", text) }
	run := llmtest.Call("run_computation", map[string]string{"computation": it.comp()}, u)
	lookup := llmtest.Call("lookup", map[string]string{"name": it.Name, "statement": it.Text}, u)
	conjecture := "---\ntag: " + it.out() + "\nkind: conjecture\nname: The statement of " + it.Target + " holds\n---\n\n## Statement\n\nThe statement of " + it.Target + " holds for each case, because it holds for the first cases.\n"
	return []attempt{
		{"reference", []*llm.Response{prog, run, lookup, file(it.counterexample(it.refutes(), it.Text))}, ""},
		{"no output", []*llm.Response{prog, run, stuck()}, "filed no conjecture or counterexample"},
		{"claims novelty", []*llm.Response{prog, run, lookup, file(it.counterexample(it.refutes(), "This is a new counterexample. "+it.Text)), stuck()}, "Only a person states that a result is new"},
		{"unchecked conjecture", []*llm.Response{lookup, file(conjecture), stuck()}, "links to no computation. Falsify before filing"},
		{"no computation", []*llm.Response{lookup, file(it.counterexample("", it.Text)), stuck()}, "refutes no statement through a computation"},
		{"wrong output hash", []*llm.Response{calls(write(it.codePath(), it.Code), write("nodes/~/"+it.comp()+".md", it.computation(image, compute.Hash([]byte(it.Code))))), lookup, file(it.counterexample(it.refutes(), it.Text)), stuck()}, "does not verify"},
		{"edits the target", []*llm.Response{prog, write(tools.NodePath(it.Target), base+"\nThe statement is false.\n"), stuck()}, "changes no node that exists"},
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

// different is a judge that finds each match about a different thing.
func different(context.Context, novelty.Claim, novelty.Match) (string, string, error) {
	return novelty.Different, "the scripted judge", nil
}

func TestSuite(t *testing.T) {
	items := readItems(t)
	s := setup(t)
	if *live {
		liveRun(t, s, items)
		return
	}
	sb := fake{}
	for _, it := range items {
		sb[it.Code] = it.Output
	}
	image := s.image(t)
	solved := 0
	for _, it := range items {
		for _, a := range attempts(it, s.base(t, it), image) {
			t.Run(it.Target+"/"+a.name, func(t *testing.T) {
				rec, outputs := s.run(t, it, llmtest.New(a.steps...), different, sb, 0)
				whys := reasons(rec)
				if a.why == "" {
					if rec.Outcome != agent.Success || !refuted(it, outputs) || !lookedUp(rec) {
						t.Fatalf("the reference gives %s, %q, with %d outputs; the goal said %q", rec.Outcome, rec.Reason, len(outputs), whys)
					}
					if res := rec.Novelty[it.out()]; res == nil || res.Verdict != novelty.NoMatch {
						t.Errorf("the novelty result of %s is %+v", it.out(), res)
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
	writeReport(t, fmt.Sprintf("# The suite of the explore role\n\nThe references refuted %d of %d targets with the scripted model, and the goal refused each wrong attempt.\n", solved, len(items)))
}

// liveRun runs the role and the judge with the Claude model of the rung, and the computations in docker.
func liveRun(t *testing.T, s *suite, items []item) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Fatal("-live needs ANTHROPIC_API_KEY")
	}
	if *imagesFile == "" {
		t.Fatal("-live needs the manifest of the images with -images")
	}
	role, err := agent.LoadRole(prompts.FS, "explore")
	if err != nil {
		t.Fatal(err)
	}
	if *rung < 0 || *rung >= len(role.Ladder) {
		t.Fatalf("the ladder of explore has no rung %d", *rung)
	}
	sb := &sandbox.Docker{}
	if err := sb.Detect(context.Background()); err != nil {
		t.Fatal(err)
	}
	m := claude.New()
	judge := explore.Judge(m, role.Ladder[*rung].Model, role.Checks["judge"])
	var runs, conjectures []string
	found, looked, usd := 0, 0, 0.0
	for _, it := range items {
		rec, outputs := s.run(t, it, m, judge, sb, *rung)
		ok := rec.Outcome == agent.Success && refuted(it, outputs)
		if ok {
			found++
		}
		if lookedUp(rec) {
			looked++
		}
		usd += rec.Spend.USD
		for _, n := range outputs {
			if n.Kind() == "conjecture" {
				conjectures = append(conjectures, fmt.Sprintf("| %s | %s | %s |", it.Target, n.ID, cell(n.Name())))
			}
		}
		runs = append(runs, fmt.Sprintf("| %s | %s | %v | %v | %d | %.4f | %s |", it.Target, rec.Outcome, ok, lookedUp(rec), len(rec.Turns), rec.Spend.USD, cell(rec.Reason)))
	}
	writeReport(t, fmt.Sprintf("# The suite of the explore role\n\nThe role refuted %d of %d targets within the budget, for $%.4f, at the rung %d of the ladder. %d of %d runs looked up each claim before they filed it. The runs filed %d conjectures, and a person reads each one, because each target is false.\n\n| Target | Outcome | Refuted | Looked up | Turns | USD | Reason |\n|---|---|---|---|---|---|---|\n%s\n\n| Target | Conjecture | Name |\n|---|---|---|\n%s\n",
		found, len(items), usd, *rung, looked, len(items), len(conjectures), strings.Join(runs, "\n"), strings.Join(conjectures, "\n")))
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
