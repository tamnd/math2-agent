// Package bump_test is the suite of the bump role (spec 17.3.4).
//
// items.yaml has the pinned Mathlib after a bump, the declarations of the Math2 library, and the items. Each item is a replay of a bump: the Lean bindings of the target before the bump, the rebinds that repair them, and whether the repair can merge with no review. A wrong attempt has the reason that the run must give. The public items are made up, and the held-out items are replays of real bumps in a private repository. -items reads them from a file of the same form.
//
// For each item, the test makes a work tree of math2 with the nodes of tree/, adds the bindings of the item to the target, and runs the role with a Lean environment that the file gives. No test runs Lean.
//
// With no flags, the model is scripted, and no test calls a model service. The script first makes the rebinds of the reference, and the goal must hold. bump.AutoMerge must give the value of the item. An item with type_change needs the constraint, thus the reference must fail without it. Then the script makes wrong attempts, and the run must fail with the reason of the attempt. A run with no rebind fails, and so does a write of the node, because the role has no tool write_file.
//
// With -live, the test runs the role once on each item with the Claude model of the rung -rung of the ladder. The report gives the share of the breakages that the role repaired, the share of its repairs that can merge with no review, and its repairs that change a type hash. The last number must be zero on the items with no type_change. It needs ANTHROPIC_API_KEY, and it spends money. Give -report FILE to write the report.
package bump_test

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
	"github.com/tamnd/math2-agent/agent/bump"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/claude"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/workspace"
)

var (
	itemsFile = flag.String("items", "items.yaml", "the file of the items")
	live      = flag.Bool("live", false, "run the role with the Claude models. This needs ANTHROPIC_API_KEY and spends money.")
	rung      = flag.Int("rung", 0, "the rung of the ladder of the role, with -live")
	report    = flag.String("report", "", "write the report of the run to this file")
)

// rebind is a change of the decl of one Lean binding.
type rebind struct {
	Decl string `yaml:"decl"`
	New  string `yaml:"new"`
}

// wrong is a wrong attempt at an item.
type wrong struct {
	Name   string   `yaml:"name"`
	Rebind []rebind `yaml:"rebind"`
	Why    string   `yaml:"why"`
}

// item is one item of items.yaml.
type item struct {
	Target     string   `yaml:"target"`
	Bindings   []string `yaml:"bindings"`
	Rebind     []rebind `yaml:"rebind"`
	TypeChange bool     `yaml:"type_change"`
	Auto       bool     `yaml:"auto"`
	Wrong      []wrong  `yaml:"wrong"`

	n int // the number of the item in the file, from 1
}

// file is the file of the items.
type file struct {
	Mathlib []struct {
		Decl        string `yaml:"decl"`
		Kind        string `yaml:"kind"`
		TypeHash    string `yaml:"type_hash"`
		Deprecated  bool   `yaml:"deprecated"`
		Replacement string `yaml:"replacement"`
	} `yaml:"mathlib"`
	Math2 []string `yaml:"math2"`
	Items []item   `yaml:"items"`
}

func readItems(t *testing.T) (*file, fakeLean) {
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
	env := fakeLean{}
	for _, d := range f.Mathlib {
		r := lean.Record{Decl: d.Decl, Kind: d.Kind, TypeHash: d.TypeHash}
		if d.Deprecated {
			r.Deprecated = &lean.Deprecation{NewName: d.Replacement, Since: "2026-09-01"}
		}
		env[d.Decl] = r
	}
	for _, d := range f.Math2 {
		env[d] = lean.Record{Decl: d, Kind: "theorem", TypeHash: "math2:" + d}
	}
	for i := range f.Items {
		it := &f.Items[i]
		it.n = i + 1
		if len(it.Bindings) == 0 || len(it.Rebind) == 0 {
			t.Fatalf("%s: the item %d needs bindings and the rebinds of the reference", *itemsFile, it.n)
		}
		for _, w := range it.Wrong {
			if len(w.Rebind) == 0 || w.Why == "" {
				t.Fatalf("%s: the wrong attempt %q of the item %d needs rebinds and a reason", *itemsFile, w.Name, it.n)
			}
		}
	}
	return &f, env
}

// fakeLean is the pinned Mathlib after the bump and the Math2 library. A declaration that it does not have does not resolve.
type fakeLean map[string]lean.Record

func (f fakeLean) Check(context.Context, string) (*lean.Elaboration, error) {
	return &lean.Elaboration{Messages: []lean.Message{}}, nil
}

func (f fakeLean) Extract(_ context.Context, decls []string, _ bool) ([]lean.Record, error) {
	var out []lean.Record
	for _, d := range decls {
		r, ok := f[d]
		if !ok {
			r = lean.Record{Decl: d, Error: "unknown declaration"}
		}
		out = append(out, r)
	}
	return out, nil
}

// setup makes the work tree of the item: the nodes of tree/, with the Lean bindings of the item before the bindings of the target.
func setup(t *testing.T, it item) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("tree")); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, filepath.FromSlash(tools.NodePath(it.Target)))
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("formal:\n")
	for _, d := range it.Bindings {
		fmt.Fprintf(&b, "  - system: lean4\n    library: mathlib\n    decl: %s\n    match: exact\n", d)
	}
	text := strings.Replace(string(data), "formal:\n", b.String(), 1)
	if text == string(data) {
		t.Fatalf("%s has no formal field", tools.NodePath(it.Target))
	}
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
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
	return dir
}

// run runs the role on the item with the model, with the constraint type_change or not. It returns the record and the changes of the run.
func run(t *testing.T, it item, env fakeLean, typeChange bool, model llm.Model, r int) (*agent.Record, []workspace.Change) {
	t.Helper()
	ctx := context.Background()
	ws, err := workspace.Open(ctx, setup(t, it))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	role, err := agent.LoadRole(prompts.FS, "bump")
	if err != nil {
		t.Fatal(err)
	}
	job := &agent.Job{ID: fmt.Sprintf("j-2026-10-26-%04d", it.n), Role: "bump", Target: it.Target, Goal: "each Lean binding of the node resolves and is not deprecated", OnBehalfOf: "schedule:agent-suite"}
	job.Constraints.TypeChange = typeChange
	brief, err := bump.Brief(ctx, ws, env, job)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tools.Profile(role, nil, ws, env, bump.Rebind(ws))
	if err != nil {
		t.Fatal(err)
	}
	rn := agent.Runner{Role: role, Model: model, Tools: ts, Goal: bump.Goal(ws, env, job)}
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

func calls(rbs []rebind) []*llm.Response {
	var out []*llm.Response
	for _, rb := range rbs {
		out = append(out, llmtest.Call("rebind", map[string]string{"decl": rb.Decl, "new": rb.New, "why": "the scripted reason."}, u))
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

// mustFail checks that the run failed and gave the reason.
func mustFail(t *testing.T, rec *agent.Record, why string) {
	t.Helper()
	whys := reasons(rec)
	if rec.Outcome == agent.Success || !slices.ContainsFunc(whys, func(w string) bool { return strings.Contains(w, why) }) {
		t.Errorf("the attempt gives %s, %q, and the run said %q; it must say %q", rec.Outcome, rec.Reason, whys, why)
	}
}

// olds are the records of the old and the new declarations of the rebinds of the changes.
func olds(t *testing.T, it item, env fakeLean, changes []workspace.Change) ([]bump.Rebinding, []lean.Record) {
	t.Helper()
	rbs, err := bump.Rebinds(it.Target, changes)
	if err != nil {
		t.Fatal(err)
	}
	var decls []string
	for _, rb := range rbs {
		decls = append(decls, rb.Old, rb.New)
	}
	recs, err := env.Extract(context.Background(), decls, false)
	if err != nil {
		t.Fatal(err)
	}
	return rbs, recs
}

func TestSuite(t *testing.T) {
	f, env := readItems(t)
	if *live {
		liveRun(t, f.Items, env)
		return
	}
	solved := 0
	for _, it := range f.Items {
		name := fmt.Sprintf("%d-%s", it.n, it.Target)
		t.Run(name+"/reference", func(t *testing.T) {
			rec, changes := run(t, it, env, it.TypeChange, llmtest.New(calls(it.Rebind)...), 0)
			if rec.Outcome != agent.Success {
				t.Fatalf("the reference gives %s, %q; the run said %q", rec.Outcome, rec.Reason, reasons(rec))
			}
			_, recs := olds(t, it, env, changes)
			if auto, err := bump.AutoMerge(it.Target, changes, recs); err != nil || auto != it.Auto {
				t.Errorf("AutoMerge gives %v, %v; the item says %v", auto, err, it.Auto)
			}
			solved++
		})
		if it.TypeChange {
			t.Run(name+"/no type_change", func(t *testing.T) {
				rec, _ := run(t, it, env, false, llmtest.New(append(calls(it.Rebind), stuck())...), 0)
				mustFail(t, rec, "the job must permit type_change")
			})
		}
		t.Run(name+"/no rebind", func(t *testing.T) {
			read := llmtest.Call("read_file", map[string]string{"path": tools.NodePath(it.Target)}, u)
			rec, _ := run(t, it, env, it.TypeChange, llmtest.New(read, stuck()), 0)
			mustFail(t, rec, "the run changed no binding")
		})
		t.Run(name+"/a write of the node", func(t *testing.T) {
			write := llmtest.Call("write_file", map[string]string{"path": tools.NodePath(it.Target), "text": "---\ntag: " + it.Target + "\n---\n"}, u)
			rec, _ := run(t, it, env, it.TypeChange, llmtest.New(write, stuck()), 0)
			mustFail(t, rec, "The role has no tool write_file")
		})
		for _, w := range it.Wrong {
			t.Run(name+"/"+w.Name, func(t *testing.T) {
				rec, _ := run(t, it, env, it.TypeChange, llmtest.New(append(calls(w.Rebind), stuck())...), 0)
				mustFail(t, rec, w.Why)
			})
		}
	}
	writeReport(t, fmt.Sprintf("# The suite of the bump role\n\nThe references solved %d of %d items with the scripted model, and the run refused each wrong attempt.\n", solved, len(f.Items)))
}

// liveRun runs the role once on each item with the Claude model of the rung.
func liveRun(t *testing.T, items []item, env fakeLean) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Fatal("-live needs ANTHROPIC_API_KEY")
	}
	m := claude.New()
	var rows []string
	repaired, auto, changed, usd := 0, 0, 0, 0.0
	for _, it := range items {
		rec, changes := run(t, it, env, it.TypeChange, m, *rung)
		usd += rec.Spend.USD
		ok, merge, typed := rec.Outcome == agent.Success, false, 0
		if ok {
			repaired++
			rbs, recs := olds(t, it, env, changes)
			var err error
			if merge, err = bump.AutoMerge(it.Target, changes, recs); err != nil {
				t.Fatal(err)
			}
			if merge {
				auto++
			}
			for _, rb := range rbs {
				if o, cur := env[rb.Old], env[rb.New]; o.TypeHash != "" && o.TypeHash != cur.TypeHash && !it.TypeChange {
					typed++
				}
			}
			changed += typed
		}
		rows = append(rows, fmt.Sprintf("| %d | %s | %s | %v | %d | %d | %.4f | %s |", it.n, it.Target, rec.Outcome, merge, typed, len(rec.Turns), rec.Spend.USD, cell(rec.Reason)))
	}
	writeReport(t, fmt.Sprintf("# The suite of the bump role\n\nThe role ran once on each of %d items, for $%.4f, at the rung %d of the ladder. It repaired %d items, and %d of these repairs can merge with no review. %d repairs change a type hash with no type_change, and this must be zero.\n\n| Item | Target | Outcome | Auto-merge | Type changes | Turns | USD | Reason |\n|---|---|---|---|---|---|---|---|\n%s\n",
		len(items), usd, *rung, repaired, auto, changed, strings.Join(rows, "\n")))
	if changed > 0 {
		t.Errorf("%d repairs change a type hash with no type_change", changed)
	}
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
