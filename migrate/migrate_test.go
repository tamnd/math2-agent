package migrate_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/migrate"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/schema"
)

const (
	old = "---\ntag: EX6H3W\nkind: example\nname: A large subgroup\nuses: [3C53TD]\n---\n\n## Example\n\nFind the order of $H$.\n\n## Remarks\n\nRole: exercise. About: 3C53TD. Difficulty: 2. Group: B. Check: exact.\n\n### Hint\n\nUse Lagrange's theorem.\n\n### Answer\n\n12\n\n### Solution\n\n1. The order of $H$ divides 12.\n2. Thus it is 12.\n"
	cur = "---\ntag: EX6H3W\nkind: exercise\nname: A large subgroup\nabout: [3C53TD]\ndifficulty: 2\nmath_group: B\ncheck: exact\nuses: [3C53TD]\n---\n\n## Exercise\n\nFind the order of $H$.\n\n## Hint\n\nUse Lagrange's theorem.\n\n## Answer\n\n12\n\n## Solution\n\n1. The order of $H$ divides 12.\n2. Thus it is 12.\n"
	thm = "---\ntag: 3C53TD\nkind: statement\nrole: theorem\nname: Lagrange's theorem\n---\n\n## Statement\n\nThe order of a subgroup divides the order of the group.\n"
	at  = "2026-10-27T10:00:00Z"
)

func parse(t *testing.T, text string) *node.Node {
	t.Helper()
	n, err := node.Parse("nodes/EX6/EX6H3W.md", []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestWindow(t *testing.T) {
	m := migrate.Window()
	if m == nil || m.From != "node.v1" || m.To != schema.Version {
		t.Fatalf("the window is %+v", m)
	}
}

func TestNode(t *testing.T) {
	got, path, err := migrate.Node(parse(t, old), "node.v1")
	if err != nil || got == nil || got.Kind() != "exercise" || !slices.Equal(path, []string{"node.v2"}) {
		t.Fatalf("got %v, path %v, error %v", got, path, err)
	}
	if got, path, err := migrate.Node(parse(t, cur), "node.v1"); got != nil || len(path) != 1 || err != nil {
		t.Errorf("a node.v2 node gives %v, %v, %v", got, path, err)
	}
	if got, path, err := migrate.Node(parse(t, old), schema.Version); got != nil || path != nil || err != nil {
		t.Errorf("a node at the current version gives %v, %v, %v", got, path, err)
	}
	if _, _, err := migrate.Node(parse(t, old), "node.v0"); err == nil || !strings.Contains(err.Error(), "no migration goes from schema node.v0") {
		t.Errorf("an unknown version gives %v", err)
	}
}

func write(t *testing.T, root, p, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(p)), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, p), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTree(t *testing.T) {
	root := t.TempDir()
	write(t, root, schema.Previous, "{}")
	write(t, root, "nodes/EX6/EX6H3W.md", old)
	write(t, root, "nodes/3C5/3C53TD.md", thm)
	agent := prov.Line{Field: prov.Whole, Op: prov.Add, Agent: "m2-agent/exercise@0.1.0", Model: "claude-opus-5-5", Run: "r-2026-10-01-1", Human: "github:tamnd", At: "2026-10-01T09:00:00Z"}
	person := prov.Line{Field: "body.remarks", Op: prov.Change, Human: "github:tamnd", At: "2026-10-02T09:00:00Z"}
	if err := prov.Append(root, "EX6H3W", agent, person); err != nil {
		t.Fatal(err)
	}
	changes, err := migrate.Tree(root, at)
	if err != nil {
		t.Fatal(err)
	}
	if want := []migrate.Change{{Path: "nodes/EX6/EX6H3W.md", Lines: 11}}; !slices.Equal(changes, want) {
		t.Fatalf("the changes are %v, want %v", changes, want)
	}
	data, err := os.ReadFile(filepath.Join(root, "nodes/EX6/EX6H3W.md"))
	if err != nil || string(data) != cur {
		t.Fatalf("the node is\n%s\nwant\n%s", data, cur)
	}
	if data, err := os.ReadFile(filepath.Join(root, "nodes/3C5/3C53TD.md")); err != nil || string(data) != thm {
		t.Errorf("the statement changed:\n%s", data)
	}
	log, err := prov.Read(root, "EX6H3W")
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]prov.Line{}
	for _, l := range log[2:] {
		if l.At != at || l.Commit != "" {
			t.Errorf("%s: the time is %s and the commit is %q", l.Field, l.At, l.Commit)
		}
		by[l.Field] = l
	}
	for f, want := range map[string]prov.Line{
		"kind":          {Op: prov.Change, Agent: agent.Agent, Run: agent.Run, Human: agent.Human},
		"body.example":  {Op: prov.Remove, Agent: agent.Agent, Run: agent.Run, Human: agent.Human},
		"body.exercise": {Op: prov.Add, Agent: agent.Agent, Run: agent.Run, Human: agent.Human},
		"body.remarks":  {Op: prov.Remove, Human: person.Human},
		"body.hint":     {Op: prov.Add, Human: person.Human},
		"difficulty":    {Op: prov.Add, Human: person.Human},
	} {
		l, ok := by[f]
		if !ok || l.Op != want.Op || l.Agent != want.Agent || l.Run != want.Run || l.Human != want.Human {
			t.Errorf("%s: the line is %+v, want %+v", f, l, want)
		}
	}
	if _, err := os.Stat(filepath.Join(root, schema.Previous)); err == nil {
		t.Errorf("%s is still there", schema.Previous)
	}
	if data, err := os.ReadFile(filepath.Join(root, schema.File)); err != nil || string(data) != string(schema.Raw()) {
		t.Errorf("%s is not the embedded schema: %v", schema.File, err)
	}
	if changes, err := migrate.Tree(root, at); len(changes) != 0 || err != nil {
		t.Errorf("a second run gives %v, %v", changes, err)
	}
}
