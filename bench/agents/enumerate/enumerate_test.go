// Package enumerate_test is the suite of the enumerate role (corpus K7.4, spec 17.3.4).
//
// Each cell of items.yaml is a cell of the inventory, the names that the inventory has for it, and reference items that a model of the cell must know, each with a source that resolves. The statements are in our own words. These are the public items. The held-out items are in a private repository, and -items reads them from a file of the same form.
//
// The test runs the role on each cell in a new work tree. With no flags, the model is scripted, and no test calls a model service. For each cell, the script first lists the names that the inventory has, and the tool must refuse each one. Then it lists the reference items, and the goal must hold with each item in the mentions of the tree. Thus the suite checks the tool and the goal of the role, and that each item is valid.
//
// With -live, the test runs the role on each cell with the Claude model of the rung -rung of the ladder. The report gives, for each cell, the items that the model listed, the share of the reference items among them by the key of a name or an alias, and the share of the listed items with at least one source. It needs ANTHROPIC_API_KEY, and it spends money. Give -report FILE to write the report.
package enumerate_test

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
	"github.com/tamnd/math2-agent/agent/enumerate"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/claude"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/models"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/workspace"
)

var (
	itemsFile = flag.String("items", "items.yaml", "the file of the cells")
	live      = flag.Bool("live", false, "run the role with the Claude models. This needs ANTHROPIC_API_KEY and spends money.")
	rung      = flag.Int("rung", 0, "the rung of the ladder of the role, with -live")
	report    = flag.String("report", "", "write the report of the run to this file")
)

// item is one reference item, in the form of the input of list_items.
type item struct {
	Kind      string           `yaml:"kind" json:"kind"`
	Name      string           `yaml:"name" json:"name"`
	Aliases   []string         `yaml:"aliases" json:"aliases,omitempty"`
	Statement string           `yaml:"statement" json:"statement"`
	Uses      []string         `yaml:"uses" json:"uses,omitempty"`
	Sources   []inventory.Cite `yaml:"sources" json:"sources,omitempty"`
}

// cell is one cell of items.yaml.
type cell struct {
	Cell  string   `yaml:"cell"`
	Have  []string `yaml:"have"`
	Items []item   `yaml:"items"`
}

func readCells(t *testing.T) []cell {
	t.Helper()
	data, err := os.ReadFile(*itemsFile)
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Cells []cell `yaml:"cells"`
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		t.Fatal(err)
	}
	if len(f.Cells) == 0 {
		t.Fatal("the file has no cells")
	}
	return f.Cells
}

// rows returns the rows of the names that the inventory has for the cell.
func (c cell) rows() []inventory.Row {
	code, _, _ := strings.Cut(c.Cell, "/")
	var out []inventory.Row
	for i, n := range c.Have {
		out = append(out, inventory.Row{ID: fmt.Sprintf("B%05d", i), Name: n, MSC: []string{code}})
	}
	return out
}

// tree makes an empty git repository and opens it as a workspace.
func tree(t *testing.T) *workspace.Workspace {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("a tree\n"), 0o600); err != nil {
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

// run runs the role on the cell in a new work tree with the model on the rung, and returns the record and the mentions of the tree.
func run(t *testing.T, c cell, m llm.Model, r int) (*agent.Record, []inventory.Mention) {
	t.Helper()
	ws := tree(t)
	role, err := agent.LoadRole(prompts.FS, "enumerate")
	if err != nil {
		t.Fatal(err)
	}
	j := &agent.Job{ID: "j-2026-10-27-1", Role: "enumerate", Target: c.Cell, Goal: "list the items of the cell", OnBehalfOf: "github:octocat"}
	cl, err := enumerate.NewCell(c.Cell, c.rows(), nil)
	if err != nil {
		t.Fatal(err)
	}
	model := role.Ladder[r].Model
	ts, err := tools.Profile(role, nil, ws, nil, enumerate.List(ws, cl, models.Default().Family(model), model, "agent:m2-agent/enumerate@"+role.Version))
	if err != nil {
		t.Fatal(err)
	}
	rn := agent.Runner{Role: role, Model: m, Tools: ts, Goal: enumerate.Goal(ws, j)}
	rec, err := rn.Run(context.Background(), j, r, enumerate.Brief(j, cl))
	if err != nil {
		t.Fatal(err)
	}
	ms, err := inventory.ReadMentions(ws.Dir)
	if err != nil {
		t.Fatal(err)
	}
	return rec, ms
}

// found returns the reference items whose name or alias has the key of a name or an alias of a mention.
func found(items []item, ms []inventory.Mention) []string {
	keys := map[string]bool{}
	for _, m := range ms {
		for _, n := range append([]string{m.Name}, m.Aliases...) {
			keys[enumerate.Key(n)] = true
		}
	}
	var out []string
	for _, it := range items {
		for _, n := range append([]string{it.Name}, it.Aliases...) {
			if keys[enumerate.Key(n)] {
				out = append(out, it.Name)
				break
			}
		}
	}
	return out
}

var u = llm.Usage{Input: 1000, Output: 500}

func TestSuite(t *testing.T) {
	cells := readCells(t)
	if *live {
		liveRun(t, cells)
		return
	}
	for _, c := range cells {
		t.Run(c.Cell, func(t *testing.T) {
			var have []item
			for _, n := range c.Have {
				have = append(have, item{Kind: "concept", Name: n, Statement: "A name that the inventory has."})
			}
			rec, ms := run(t, c, llmtest.New(
				llmtest.Call("list_items", map[string]any{"items": have}, u),
				llmtest.Call("list_items", map[string]any{"items": c.Items}, u),
			), 0)
			if rec.Outcome != agent.Success || len(rec.Turns) != 2 {
				t.Fatalf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
			}
			if r := rec.Turns[0].Results[0]; !r.IsError || !strings.Contains(r.Content, "already") {
				t.Errorf("the names that the inventory has give %+v", r)
			}
			if r := rec.Turns[1].Results[0]; r.IsError || r.Content != fmt.Sprintf("Added %d of %d items to model-claude.", len(c.Items), len(c.Items)) {
				t.Errorf("the reference items give %+v", r)
			}
			if got := found(c.Items, ms); len(got) != len(c.Items) || len(ms) != len(c.Items) {
				t.Errorf("the tree has %d mentions, with %d of the %d reference items", len(ms), len(got), len(c.Items))
			}
			for _, m := range ms {
				if len(m.Cites) == 0 {
					t.Errorf("the reference item %s has no source", m.Name)
				}
			}
		})
	}
}

// liveRun runs the role with the Claude model of the rung on each cell.
func liveRun(t *testing.T, cells []cell) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Fatal("-live needs ANTHROPIC_API_KEY")
	}
	m := claude.New()
	var rows, lists []string
	usd := 0.0
	for _, c := range cells {
		rec, ms := run(t, c, m, *rung)
		usd += rec.Spend.USD
		got := found(c.Items, ms)
		cited := 0
		for _, x := range ms {
			if len(x.Cites) > 0 {
				cited++
			}
			lists = append(lists, fmt.Sprintf("| %s | %s | %s | %d |", c.Cell, x.Kind, md(x.Name), len(x.Cites)))
		}
		rows = append(rows, fmt.Sprintf("| %s | %s | %d | %d of %d | %d of %d | %.4f |", c.Cell, rec.Outcome, len(ms), len(got), len(c.Items), cited, len(ms), rec.Spend.USD))
	}
	text := fmt.Sprintf("# The suite of the enumerate role\n\nThe role ran on %d cells for $%.4f, at the rung %d of the ladder.\n\n| Cell | Outcome | Listed | Reference items found | Items with a source | USD |\n|---|---|---|---|---|---|\n%s\n\n| Cell | Kind | Name | Sources |\n|---|---|---|---|\n%s\n",
		len(cells), usd, *rung, strings.Join(rows, "\n"), strings.Join(lists, "\n"))
	if *report == "" {
		t.Log(text)
		return
	}
	if err := os.WriteFile(*report, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func md(s string) string { return strings.ReplaceAll(s, "|", `\|`) }
