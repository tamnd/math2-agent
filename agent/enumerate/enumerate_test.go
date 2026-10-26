package enumerate_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/enumerate"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/workspace"
)

// tree makes a git repository with the files in one commit and opens it as a workspace.
func tree(t *testing.T, files map[string]string) *workspace.Workspace {
	t.Helper()
	dir := t.TempDir()
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
	w, err := workspace.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

const by = "agent:m2-agent/enumerate@0.1.0"

var u = llm.Usage{Input: 1000, Output: 200}

// rows and mentions are the inventory of the tests: a row of 54D30, a row of another code, and a mention of a model list of 54D30 from a round before.
var (
	rows = []inventory.Row{
		{ID: "AAAAAA", Name: "Compact space", MSC: []string{"54D30"}},
		{ID: "AAAAAB", Name: "Prime number", MSC: []string{"11A41"}},
	}
	mentions = []inventory.Mention{
		{ID: inventory.MentionID("model-claude", "54D30.tube-lemma", 1), Source: "model-claude", Name: "Tube lemma", MSC: []string{"54D30"}, By: by},
		{ID: "m2://rec/wikidata/Q1#1", Source: "wikidata", Name: "Lindelöf space", MSC: []string{"54D30"}, By: "m2/0.2.0"},
	}
)

func TestNewCell(t *testing.T) {
	c, err := enumerate.NewCell("54D30/undergraduate", rows, mentions)
	if err != nil {
		t.Fatal(err)
	}
	if c.Code != "54D30" || c.Level != "undergraduate" || !slices.Equal(c.Names, []string{"Compact space", "Tube lemma"}) {
		t.Errorf("the cell is %+v", c)
	}
	for _, target := range []string{"54D30", "54D/undergraduate", "54D30/college", "3C53TD"} {
		if _, err := enumerate.NewCell(target, nil, nil); err == nil || !strings.Contains(err.Error(), "is not a cell") {
			t.Errorf("the target %q gives %v", target, err)
		}
	}
}

func TestBrief(t *testing.T) {
	c, err := enumerate.NewCell("54D30/undergraduate", rows, mentions)
	if err != nil {
		t.Fatal(err)
	}
	b := enumerate.Brief(&agent.Job{ID: "j-1", Goal: "list the items."}, c)
	for _, want := range []string{"MSC2020 code 54D30 at the undergraduate level", "The goal of the job: list the items.", "these 2 items", "Compact space\nTube lemma", "leave it out"} {
		if !strings.Contains(b, want) {
			t.Errorf("the brief has no %q:\n%s", want, b)
		}
	}
	if b := enumerate.Brief(&agent.Job{ID: "j-1", Goal: "g"}, &enumerate.Cell{Code: "54D30", Level: "school"}); !strings.Contains(b, "has no item for 54D30 yet") {
		t.Errorf("the brief of an empty cell is %s", b)
	}
}

// heineBorel is an item with two good sources and two sources that are not valid.
var heineBorel = map[string]any{
	"kind":      "statement",
	"name":      "Heine–Borel  theorem",
	"aliases":   []string{"Borel-Lebesgue theorem", "Heine-Borel Theorem", " "},
	"statement": "A subset of $\\mathbb{R}^n$ is compact if and only if it is closed and bounded.",
	"uses":      []string{"compact space", "closed set", "bounded set"},
	"sources": []map[string]string{
		{"system": "ISBN", "id": "978-0-07-054235-8", "locator": "Theorem 2.41"},
		{"system": "wikipedia", "id": "Heine–Borel theorem"},
		{"system": "isbn", "id": "978-0-07-054235-9"},
		{"system": "doi", "id": "not a doi"},
	},
}

func list(t *testing.T, ws *workspace.Workspace, items ...map[string]any) (agent.Result, error) {
	t.Helper()
	c, err := enumerate.NewCell("54D30/undergraduate", rows, mentions)
	if err != nil {
		t.Fatal(err)
	}
	in, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		t.Fatal(err)
	}
	tool := enumerate.List(ws, c, "claude", "claude-haiku-5-5", by)
	return tool.Call(context.Background(), &agent.Job{ID: "j-1", Role: "enumerate", Target: "54D30/undergraduate"}, in)
}

func TestList(t *testing.T) {
	ws := tree(t, map[string]string{"README.md": "a tree\n"})
	res, err := list(t, ws,
		heineBorel,
		map[string]any{"kind": "statement", "name": "Tube Lemma", "statement": "A lemma."},
		map[string]any{"kind": "theorem", "name": "A", "statement": "A."},
		map[string]any{"kind": "concept", "name": "Sequentially compact space", "statement": ""},
		map[string]any{"kind": "concept", "name": "heine borel theorem", "statement": "Again."},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Added 1 of 5 items to model-claude.", "no source isbn: \"978-0-07-054235-9\" is not an ISBN", "and no source doi:", "item 2: the inventory has \"Tube Lemma\" already", "item 3: the kind \"theorem\"", "item 4: the item \"Sequentially compact space\" has no statement", "item 5: the inventory has \"heine borel theorem\" already"} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("the result has no %q: %s", want, res.Text)
		}
	}
	ms, err := inventory.ReadMentions(ws.Dir)
	if err != nil || len(ms) != 1 {
		t.Fatalf("the tree has the mentions %+v, %v", ms, err)
	}
	m := ms[0]
	want := inventory.Mention{
		ID:       inventory.MentionID("model-claude", "54D30.heine-borel-theorem", 1),
		Source:   "model-claude",
		Snapshot: "claude-haiku-5-5",
		Locator:  "MSC 54D30, undergraduate",
		Kind:     "statement",
		Name:     "Heine–Borel theorem",
		Aliases:  []string{"Borel-Lebesgue theorem"},
		Uses:     []string{"compact space", "closed set", "bounded set"},
		Cites:    []inventory.Cite{{System: "isbn", ID: "978-0-07-054235-8", Locator: "Theorem 2.41"}, {System: "wikipedia", ID: "Heine–Borel theorem"}},
		Text:     "A subset of $\\mathbb{R}^n$ is compact if and only if it is closed and bounded.",
		MSC:      []string{"54D30"},
		Class:    "open",
		By:       by,
	}
	a, _ := json.Marshal(m)
	b, _ := json.Marshal(want)
	if string(a) != string(b) {
		t.Errorf("the mention is\n%s\nwant\n%s", a, b)
	}
	// A second tool lists the item again in a new round. Its id is in the file, thus it gets the next fragment.
	c := &enumerate.Cell{Code: "54D30", Level: "graduate"}
	in, _ := json.Marshal(map[string]any{"items": []any{heineBorel}})
	res, err = enumerate.List(ws, c, "claude", "claude-sonnet-5-5", by).Call(context.Background(), &agent.Job{Role: "enumerate"}, in)
	if err != nil {
		t.Fatal(err)
	}
	if ms, _ = inventory.ReadMentions(ws.Dir); len(ms) != 2 || !slices.ContainsFunc(ms, func(m inventory.Mention) bool {
		return m.ID == inventory.MentionID("model-claude", "54D30.heine-borel-theorem", 2) && m.Snapshot == "claude-sonnet-5-5"
	}) {
		t.Errorf("after the second round the mentions are %+v", ms)
	}
	if _, err := list(t, ws, map[string]any{"kind": "statement", "name": "Compact space", "statement": "x"}); err == nil || !strings.Contains(err.Error(), "added no item of the 1") {
		t.Errorf("a call with no new item gives %v", err)
	}
}

func TestISBN(t *testing.T) {
	for s, want := range map[string]bool{
		"978-0-07-054235-8": true,
		"0-07-054235-X":     true,
		"978 0 07 054235 8": true,
		"0070542358":        false,
		"978-0-07-054235-7": false,
		"12345":             false,
		"97800705423X8":     false,
	} {
		if got := enumerate.ISBN(s); got != want {
			t.Errorf("ISBN(%q) is %v, want %v", s, got, want)
		}
	}
}

// run runs an enumerate job on a new tree with the scripted model.
func run(t *testing.T, model []*llm.Response) *agent.Record {
	t.Helper()
	ws := tree(t, map[string]string{"README.md": "a tree\n"})
	role, err := agent.LoadRole(prompts.FS, "enumerate")
	if err != nil {
		t.Fatal(err)
	}
	j := &agent.Job{ID: "j-2026-10-27-1", Role: "enumerate", Target: "54D30/undergraduate", Goal: "list the items", OnBehalfOf: "github:octocat"}
	c, err := enumerate.NewCell(j.Target, rows, mentions)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tools.Profile(role, nil, ws, nil, enumerate.List(ws, c, "claude", role.Ladder[0].Model, "agent:m2-agent/enumerate@"+role.Version))
	if err != nil {
		t.Fatal(err)
	}
	rn := agent.Runner{Role: role, Model: llmtest.New(model...), Tools: ts, Goal: enumerate.Goal(ws, j)}
	rec, err := rn.Run(context.Background(), j, 0, enumerate.Brief(j, c))
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestRun(t *testing.T) {
	rec := run(t, []*llm.Response{
		llmtest.Call("list_items", map[string]any{"items": []any{map[string]any{"kind": "statement", "name": "Compact space", "statement": "x"}}}, u),
		llmtest.Call("list_items", map[string]any{"items": []any{heineBorel}}, u),
	})
	if rec.Outcome != agent.Success || len(rec.Turns) != 2 {
		t.Fatalf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
	}
	if r := rec.Turns[0].Results[0]; !r.IsError || !strings.Contains(r.Content, "the inventory has \"Compact space\" already") {
		t.Errorf("the first call gives %+v", r)
	}
	if got := rec.Turns[0].Why; !strings.Contains(got, "has not added a mention") {
		t.Errorf("after the first turn the goal says %q", got)
	}
}
