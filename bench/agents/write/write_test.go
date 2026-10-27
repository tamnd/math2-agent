// Package write_test is the suite of the write role (corpus K7.3, K7.5, spec 17.3.4).
//
// Each item of items.yaml is a work tree with a target node, the goal of the job, and the reference writes that take the target to the level of the goal. The nodes are in our own words. These are the public items. The held-out items are in a private repository, and -items reads them from a file of the same form.
//
// The test runs the role on each item in a new work tree. With no flags, the model is scripted, and no test calls a model service. For each item, the script makes each reference write but the last. Then it makes the last write with the wrong form of the item, such as a proof that is not in steps or a definition with no term in bold, and the goal must refuse it with the reason of the item. Then it makes the last write as it is, and the goal must hold. Thus the suite checks the gates of the goal, and that the reference writes take each target to its level.
//
// With -live, the test runs the role on each item with the Claude model of the rung -rung of the ladder. The report gives, for each item, the level of the job, the outcome, the turns, the files that the run wrote and the spend. It needs ANTHROPIC_API_KEY, and it spends money. Give -report FILE to write the report.
package write_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/agent/write"
	"github.com/tamnd/math2-agent/card"
	"github.com/tamnd/math2-agent/graph"
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
	Tag    string            `yaml:"tag"`
	Goal   string            `yaml:"goal"`
	Files  map[string]string `yaml:"files"`
	Writes []file            `yaml:"writes"`
	Wrong  struct {
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
			t.Fatalf("the item %s has no writes, no wrong form or no reason", it.Tag)
		}
	}
	return f.Items
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

// level returns the writing level of the job of the item, from the tree before the run.
func level(t *testing.T, it item) card.Level {
	t.Helper()
	var nodes []*node.Node
	var target *node.Node
	for p, text := range it.Files {
		n, err := node.Parse(p, []byte(text))
		if err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, n)
		if n.ID == it.Tag {
			target = n
		}
	}
	if target == nil {
		t.Fatalf("the files of the item have no node %s", it.Tag)
	}
	return write.Level(card.Embedded(), target, graph.Build(nodes), it.Goal)
}

// run runs the role on the item in a new work tree with the model on the rung, and returns the record and the paths that the run changed.
func run(t *testing.T, it item, m llm.Model, r int) (*agent.Record, []string) {
	t.Helper()
	ws := tree(t, it)
	role, err := agent.LoadRole(prompts.FS, "write")
	if err != nil {
		t.Fatal(err)
	}
	j := &agent.Job{ID: "j-2026-10-27-1", Role: "write", Target: it.Tag, Goal: it.Goal, OnBehalfOf: "github:octocat"}
	brief, err := write.Brief(ws, j, card.Embedded())
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tools.Profile(role, nil, ws, nil)
	if err != nil {
		t.Fatal(err)
	}
	rn := agent.Runner{Role: role, Model: m, Tools: ts, Goal: write.Goal(ws, j, card.Embedded(), nil)}
	rec, err := rn.Run(context.Background(), j, r, brief)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := ws.Changes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, c := range changes {
		paths = append(paths, c.Path)
	}
	return rec, paths
}

var u = llm.Usage{Input: 1000, Output: 500}

func call(f file) *llm.Response {
	return llmtest.Call("write_file", map[string]string{"path": f.Path, "text": f.Text}, u)
}

func TestSuite(t *testing.T) {
	items := readItems(t)
	if *live {
		liveRun(t, items)
		return
	}
	for _, it := range items {
		t.Run(it.Tag, func(t *testing.T) {
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
			rec, _ := run(t, it, llmtest.New(script...), 0)
			if rec.Outcome != agent.Success || len(rec.Turns) != len(script) {
				t.Fatalf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
			}
			if got := rec.Turns[len(script)-2].Why; !strings.Contains(got, it.Refuse) {
				t.Errorf("the wrong form gives %q, and it must say %q", got, it.Refuse)
			}
		})
	}
}

// liveRun runs the role with the Claude model of the rung on each item.
func liveRun(t *testing.T, items []item) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Fatal("-live needs ANTHROPIC_API_KEY")
	}
	m := claude.New()
	var rows []string
	usd := 0.0
	for _, it := range items {
		rec, paths := run(t, it, m, *rung)
		usd += rec.Spend.USD
		rows = append(rows, fmt.Sprintf("| %s | %s | %s | %d | %s | %.4f |", it.Tag, level(t, it), rec.Outcome, len(rec.Turns), strings.Join(paths, ", "), rec.Spend.USD))
	}
	text := fmt.Sprintf("# The suite of the write role\n\nThe role ran on %d items for $%.4f, at the rung %d of the ladder.\n\n| Target | Level | Outcome | Turns | Files | USD |\n|---|---|---|---|---|---|\n%s\n",
		len(items), usd, *rung, strings.Join(rows, "\n"))
	if *report == "" {
		t.Log(text)
		return
	}
	if err := os.WriteFile(*report, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}
