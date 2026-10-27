// Package agents_test is the suite of the formalize and prove roles (spec 17.3.4).
//
// Each line of items.tsv is an item: a target node and the role that works on it. A prove item has a pinned statement, a definition of a proposition, and a reference proof. A formalize item has a node with no binding and a reference formal statement. The statements use only the Lean core library and are in our own words. These are the public items. The held-out items are in a private repository, and -items reads them from a file of the same form.
//
// The test makes one work tree of math2 with all the items: a node for each item, and a Lean module for each prove item. tree/ has the Lake project, and tree/lean/Math2/Meta/Attr.lean is a copy of the m2 attribute of tamnd/math2. The test builds M2Extract and the tree with Lake, then runs each item in the tree, and puts the tree back after each run.
//
// With no flags, the model is a scripted model, and no test calls a model service. For each item, the script first writes the reference, and the goal of the role must hold. Then for each item it writes some wrong attempts, and the goal must not hold, with the reason that the attempt must give. A prove attempt that uses sorry, that changes the statement, or that proves another statement fails. A formalize attempt that states a false statement fails. Thus the suite checks the goal predicates and the items on real Lean, and each item has a solution.
//
// With -live, the test runs the role on each item with the Claude model of the rung -rung of the ladder of the role. It needs ANTHROPIC_API_KEY, and it spends money. With M2_LLM_COMMAND, for example codex,claude, it runs the command-line clients of the subscriptions instead (package llm/command), and the dollars are the prices of the Claude API for the same tokens. The report gives the outcome, the turns and the dollars of each item, and the share of the items that the role solved within the budget. Give -report FILE to write it. A live run is not a release gate yet, because the baseline needs the runs of the first versions of the roles.
//
// The test skips with -short, and when lake is not on the PATH.
package agents_test

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/formalize"
	"github.com/tamnd/math2-agent/agent/prove"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/claude"
	"github.com/tamnd/math2-agent/llm/command"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/workspace"
)

var (
	itemsFile = flag.String("items", "items.tsv", "the file of the items")
	live      = flag.Bool("live", false, "run the roles with the Claude models. This needs ANTHROPIC_API_KEY and spends money, or M2_LLM_COMMAND for the clients of the subscriptions.")
	rung      = flag.Int("rung", 0, "the rung of the ladder of each role, with -live")
	report    = flag.String("report", "", "write the report of the run to this file")
	cache     = flag.String("cache", "", "the directory where M2Extract is built. Empty means m2 in the user cache directory.")
)

// item is one line of items.tsv.
type item struct {
	ID, Role, Kind, Decl, Reference, Statement string
}

func readItems(t *testing.T) []item {
	t.Helper()
	f, err := os.Open(*itemsFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var out []item
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		c := strings.Split(line, "\t")
		if len(c) != 6 {
			t.Fatalf("%s:%d: %d columns, want 6", *itemsFile, n, len(c))
		}
		it := item{c[0], c[1], c[2], c[3], c[4], c[5]}
		if (it.Role != "prove" && it.Role != "formalize") || (it.Role == "prove") == (it.Reference == "-") {
			t.Fatalf("%s:%d: the item %s has the role %q and the reference %q", *itemsFile, n, it.ID, it.Role, it.Reference)
		}
		out = append(out, it)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (it item) node() string { return tools.NodePath(it.ID) }
func (it item) module() string {
	return "lean/Math2/Bench/" + it.ID + ".lean"
}
func (it item) statement() string { return "Math2.Bench." + it.ID + ".statement" }

// nodeText is the node of the item, with a binding to decl when decl is not empty.
func (it item) nodeText(decl string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\ntag: %s\nkind: %s\nname: Item %s of the agent suite\n", it.ID, it.Kind, it.ID)
	if decl != "" {
		fmt.Fprintf(&b, "formal:\n  - system: lean4\n    library: math2\n    decl: %s\n    match: exact\n", decl)
	}
	section := "Statement"
	if it.Kind == "definition" {
		section = "Definition"
	}
	fmt.Fprintf(&b, "---\n\n## %s\n\n%s\n", section, it.Statement)
	return b.String()
}

// leanFile is a module of Math2 with the declarations.
func leanFile(decls ...string) string {
	return "module\n\npublic import Math2.Meta.Attr\n\n@[expose] public section\n\n" + strings.Join(decls, "\n\n") + "\n\nend\n"
}

// pinned is the module of a prove item at the base commit: the statement with the attribute.
func (it item) pinned() string {
	return leanFile(fmt.Sprintf("@[m2 %q]\ndef %s %s", it.ID, it.statement(), it.Decl))
}

// proved is the module of a prove item with a theorem of the statement.
func (it item) proved(statement, theorem, proof string) string {
	return leanFile("def "+it.statement()+" "+statement, fmt.Sprintf("@[m2 %q]\ntheorem Math2.Bench.%s.proof : %s := %s", it.ID, it.ID, theorem, proof))
}

// suite is the work tree of the items, with the Lean environment.
type suite struct {
	dir, exe string
	root     string // the root module at the base commit
}

func setup(t *testing.T, items []item) *suite {
	t.Helper()
	dir := t.TempDir()
	err := os.CopyFS(dir, os.DirFS("tree"))
	if err != nil {
		t.Fatal(err)
	}
	toolchain, err := os.ReadFile("../../lean/lean-toolchain")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{".gitignore": "/lean/.lake/\n", "lean/lean-toolchain": string(toolchain)}
	root := "module\n\npublic import Math2.Meta.Attr\n"
	for _, it := range items {
		if it.Role == "prove" {
			files[it.node()] = it.nodeText(it.statement())
			files[it.module()] = it.pinned()
			root += "public import Math2.Bench." + it.ID + "\n"
		} else {
			files[it.node()] = it.nodeText("")
		}
	}
	files["lean/Math2.lean"] = root
	for name, text := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The build writes lake-manifest.json, thus the build comes before the base commit.
	cmd := exec.Command("lake", "build", "-q")
	cmd.Dir = filepath.Join(dir, "lean")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("lake build of the suite: %v\n%s", err, out)
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
	exe, err := lean.Install(context.Background(), filepath.Join(dir, "lean"), *cache)
	if err != nil {
		t.Fatal(err)
	}
	return &suite{dir: dir, exe: exe, root: root}
}

// reset puts the tree back at the base commit, and keeps the build of Lake.
func (s *suite) reset(t *testing.T) {
	t.Helper()
	for _, args := range [][]string{{"checkout", "-q", "--", "."}, {"clean", "-q", "-f", "-d"}} {
		if out, err := exec.Command("git", append([]string{"-C", s.dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// run runs the role of the item with the model in the tree, and puts the tree back.
func (s *suite) run(t *testing.T, it item, model llm.Model, r int) *agent.Record {
	t.Helper()
	defer s.reset(t)
	ctx := context.Background()
	ws, err := workspace.Open(ctx, s.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	env := &tools.Lake{Tree: s.dir, Exe: s.exe}
	role, err := agent.LoadRole(prompts.FS, it.Role)
	if err != nil {
		t.Fatal(err)
	}
	job := &agent.Job{ID: "j-2026-10-26-" + strings.ToLower(it.ID), Role: it.Role, Target: it.ID, Goal: "the item of the suite has a solution", OnBehalfOf: "schedule:agent-suite"}
	var (
		brief string
		goal  agent.Goal
	)
	switch it.Role {
	case "prove":
		pin, err := prove.NewPin(ctx, ws, env, job)
		if err != nil {
			t.Fatal(err)
		}
		if brief, err = prove.Brief(ws, job, pin); err != nil {
			t.Fatal(err)
		}
		goal = prove.Goal(ws, env, job, pin)
	case "formalize":
		if brief, err = formalize.Brief(ws, job); err != nil {
			t.Fatal(err)
		}
		goal = formalize.Goal(ws, env, job)
	}
	ts, err := tools.Profile(role, nil, ws, env)
	if err != nil {
		t.Fatal(err)
	}
	rn := agent.Runner{Role: role, Model: model, Tools: ts, Goal: goal}
	rec, err := rn.Run(ctx, job, r, brief)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

var u = llm.Usage{Input: 1000, Output: 200}

func write(path, text string) *llm.Response {
	return llmtest.Call("write_file", map[string]string{"path": path, "text": text}, u)
}

func stuck() *llm.Response {
	return llmtest.Call(agent.StuckTool, map[string]string{"reason": "the attempt does not work"}, u)
}

// attempt is a scripted attempt at an item, and the reason that the goal must give after it. An empty reason means that the goal must hold.
type attempt struct {
	name  string
	steps []*llm.Response
	why   string
}

// attempts are the reference and the wrong attempts at the item. The root is the text of the root module at the base commit.
func attempts(it item, root string) []attempt {
	proof := "Math2.Bench." + it.ID + ".proof"
	bound := write(it.node(), it.nodeText(proof))
	if it.Role == "prove" {
		return []attempt{
			{"reference", []*llm.Response{write(it.module(), it.proved(it.Decl, it.statement(), it.Reference)), bound}, ""},
			{"sorry", []*llm.Response{write(it.module(), it.proved(it.Decl, it.statement(), "by sorry")), bound, stuck()}, "uses sorry"},
			{"changed statement", []*llm.Response{write(it.module(), it.proved(": Prop := True", it.statement(), "trivial")), bound, stuck()}, "the run changed " + it.statement()},
			{"other statement", []*llm.Response{write(it.module(), it.proved(it.Decl, "True", "trivial")), bound, stuck()}, "the type of " + proof + " is True"},
		}
	}
	name := "Math2.Bench." + it.ID + ".formal"
	add := func(decl string) []*llm.Response {
		return []*llm.Response{
			write(it.module(), leanFile(fmt.Sprintf("@[m2 %q]\ndef %s %s", it.ID, name, decl))),
			write("lean/Math2.lean", root+"public import Math2.Bench."+it.ID+"\n"),
			write(it.node(), it.nodeText(name)),
		}
	}
	return []attempt{
		{"reference", add(it.Decl), ""},
		{"false statement", append(add(": Prop := ∀ n : Nat, n < n"), stuck()), "proves its negation"},
	}
}

// liveModel returns the model of a live run: the clients in M2_LLM_COMMAND when it is set, and the Claude API otherwise.
func liveModel(t *testing.T) llm.Model {
	t.Helper()
	if s := os.Getenv(command.Env); s != "" {
		backends, err := command.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return command.New(backends...)
	}
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Fatal("-live needs ANTHROPIC_API_KEY or " + command.Env)
	}
	return claude.New()
}

func TestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("the suite runs Lean")
	}
	if _, err := exec.LookPath("lake"); err != nil {
		t.Skip("lake is not on the PATH")
	}
	items := readItems(t)
	s := setup(t, items)
	var rows []string
	solved, usd := 0, 0.0
	var model llm.Model // one model for all items, thus a client that hit its usage limit does not run again
	if *live {
		model = liveModel(t)
	}
	for _, it := range items {
		if *live {
			rec := s.run(t, it, model, *rung)
			if rec.Outcome == agent.Success {
				solved++
			}
			usd += rec.Spend.USD
			rows = append(rows, fmt.Sprintf("| %s | %s | %s | %d | %.4f | %s |", it.ID, it.Role, rec.Outcome, len(rec.Turns), rec.Spend.USD, strings.ReplaceAll(rec.Reason, "|", `\|`)))
			continue
		}
		for _, a := range attempts(it, s.root) {
			t.Run(it.ID+"/"+a.name, func(t *testing.T) {
				rec := s.run(t, it, llmtest.New(a.steps...), 0)
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
	if *report == "" {
		return
	}
	var b strings.Builder
	if *live {
		fmt.Fprintf(&b, "# The agent suite\n\nThe roles solved %d of %d items within the budget, for $%.4f, at the rung %d of each ladder.\n\n| Item | Role | Outcome | Turns | USD | Reason |\n|---|---|---|---|---|---|\n%s\n", solved, len(items), usd, *rung, strings.Join(rows, "\n"))
	} else {
		fmt.Fprintf(&b, "# The agent suite\n\nThe references solved %d of %d items with the scripted model, and the goals refused each wrong attempt.\n", solved, len(items))
	}
	if err := os.WriteFile(*report, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}
