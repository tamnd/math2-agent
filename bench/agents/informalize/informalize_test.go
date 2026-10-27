// Package informalize_test is the suite of the informalize role (corpus K7.3, K8.3, spec 17.3.4).
//
// Each item of items.yaml is a Mathlib declaration with the record that a fake Lean environment gives for it, a work tree, and the reference writes that give the declaration its node. The types of the records are short forms that we wrote for the suite. The nodes are in our own words. These are the public items. The held-out items are in a private repository, and -items reads them from a file of the same form.
//
// The test runs the role on each item in a new work tree, with the fake Lean environment, thus no test needs Lean or Mathlib. With no flags, the model is scripted, and no test calls a model service. For each item, the script makes each reference write but the last. Then it makes the last write with the wrong form of the item, such as a Lean name as the name of the node or a definition with no term in bold, and the goal must refuse it with the reason of the item. Then it makes the last write as it is, and the goal must hold. Thus the suite checks the gates of the goal, and that the reference writes give each declaration a valid node.
//
// With -live, the test runs the role on each item with the Claude model of the rung -rung of the ladder. The report gives, for each item, the outcome, the turns, the nodes that the run added with their kinds and names, and the spend. It needs ANTHROPIC_API_KEY, and it spends money. Give -report FILE to write the report.
package informalize_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/informalize"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/lean"
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

// file is one reference write of an item.
type file struct {
	Path string `yaml:"path"`
	Text string `yaml:"text"`
}

// item is one item of items.yaml.
type item struct {
	Decl      string            `yaml:"decl"`
	Kind      string            `yaml:"kind"`
	Module    string            `yaml:"module"`
	Type      string            `yaml:"type"`
	Constants []string          `yaml:"constants"`
	Files     map[string]string `yaml:"files"`
	Writes    []file            `yaml:"writes"`
	Wrong     struct {
		From string `yaml:"from"`
		To   string `yaml:"to"`
	} `yaml:"wrong"`
	Refuse string `yaml:"refuse"`
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
	for _, it := range f.Items {
		if len(it.Writes) == 0 || it.Wrong.From == "" || it.Refuse == "" {
			t.Fatalf("the item %s has no writes, no wrong form or no reason", it.Decl)
		}
	}
	return f.Items
}

// fakeLean gives the record of the declaration of each item, and an error record for each other declaration.
type fakeLean map[string]lean.Record

func (fakeLean) Check(context.Context, string) (*lean.Elaboration, error) {
	return nil, errors.New("informalize writes no Lean")
}

func (f fakeLean) Extract(_ context.Context, decls []string, _ bool) ([]lean.Record, error) {
	out := make([]lean.Record, len(decls))
	for i, d := range decls {
		r, ok := f[d]
		if !ok {
			r = lean.Record{Decl: d, Error: "unknown declaration"}
		}
		out[i] = r
	}
	return out, nil
}

func env(items []item) fakeLean {
	f := fakeLean{}
	for _, it := range items {
		f[it.Decl] = lean.Record{Decl: it.Decl, Kind: it.Kind, Module: it.Module, StatementPP: it.Type, UsesConstantsInStatement: it.Constants}
	}
	return f
}

// tree makes a git repository with the files of the item and opens it as a workspace.
func tree(t *testing.T, it item) *workspace.Workspace {
	t.Helper()
	dir := t.TempDir()
	for name, text := range it.Files {
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

// run runs the role on the item in a new work tree with the Lean environment and the model on the rung, and returns the record and the nodes that the run added.
func run(t *testing.T, it item, lenv fakeLean, m llm.Model, r int) (*agent.Record, []*node.Node) {
	t.Helper()
	ws := tree(t, it)
	role, err := agent.LoadRole(prompts.FS, "informalize")
	if err != nil {
		t.Fatal(err)
	}
	target := "mathlib:" + it.Decl
	j := &agent.Job{ID: "j-2026-10-27-1", Role: "informalize", Target: target, Goal: "the statement of the declaration in words", OnBehalfOf: "github:octocat"}
	d, err := informalize.Load(context.Background(), ws, lenv, nil, target)
	if err != nil {
		t.Fatal(err)
	}
	brief, err := informalize.Brief(ws, j, d)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tools.Profile(role, nil, ws, nil)
	if err != nil {
		t.Fatal(err)
	}
	rn := agent.Runner{Role: role, Model: m, Tools: ts, Goal: informalize.Goal(ws, j, d)}
	rec, err := rn.Run(context.Background(), j, r, brief)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := ws.Changes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var added []*node.Node
	for _, c := range changes {
		if n, err := node.Parse(c.Path, c.New); err == nil && c.Old == nil {
			added = append(added, n)
		}
	}
	return rec, added
}

var u = llm.Usage{Input: 1000, Output: 500}

func call(f file) *llm.Response {
	return llmtest.Call("write_file", map[string]string{"path": f.Path, "text": f.Text}, u)
}

func TestSuite(t *testing.T) {
	items := readItems(t)
	lenv := env(items)
	if *live {
		liveRun(t, items, lenv)
		return
	}
	for _, it := range items {
		t.Run(it.Decl, func(t *testing.T) {
			last := it.Writes[len(it.Writes)-1]
			wrong := file{Path: last.Path, Text: strings.Replace(last.Text, it.Wrong.From, it.Wrong.To, 1)}
			if wrong.Text == last.Text {
				t.Fatalf("the wrong form %q is not in the last write", it.Wrong.From)
			}
			var script []*llm.Response
			for _, f := range it.Writes[:len(it.Writes)-1] {
				script = append(script, call(f))
			}
			script = append(script, call(wrong), call(last))
			rec, added := run(t, it, lenv, llmtest.New(script...), 0)
			if rec.Outcome != agent.Success || len(rec.Turns) != len(script) {
				t.Fatalf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
			}
			if got := rec.Turns[len(script)-2].Why; !strings.Contains(got, it.Refuse) {
				t.Errorf("the wrong form gives %q, and it must say %q", got, it.Refuse)
			}
			if len(added) != len(it.Writes) {
				t.Errorf("the run adds %d nodes, and the item has %d writes", len(added), len(it.Writes))
			}
		})
	}
}

// liveRun runs the role with the Claude model of the rung on each item.
func liveRun(t *testing.T, items []item, lenv fakeLean) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Fatal("-live needs ANTHROPIC_API_KEY")
	}
	m := claude.New()
	var rows []string
	usd := 0.0
	for _, it := range items {
		rec, added := run(t, it, lenv, m, *rung)
		usd += rec.Spend.USD
		var nodes []string
		for _, n := range added {
			nodes = append(nodes, fmt.Sprintf("%s (%s)", n.Name(), n.Kind()))
		}
		rows = append(rows, fmt.Sprintf("| %s | %s | %d | %s | %.4f |", it.Decl, rec.Outcome, len(rec.Turns), strings.Join(nodes, ", "), rec.Spend.USD))
	}
	text := fmt.Sprintf("# The suite of the informalize role\n\nThe role ran on %d items for $%.4f, at the rung %d of the ladder.\n\n| Declaration | Outcome | Turns | Nodes | USD |\n|---|---|---|---|---|\n%s\n",
		len(items), usd, *rung, strings.Join(rows, "\n"))
	if *report == "" {
		t.Log(text)
		return
	}
	if err := os.WriteFile(*report, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}
