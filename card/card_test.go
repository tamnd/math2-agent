package card

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/node"
)

func parse(t *testing.T, id, text string) *node.Node {
	t.Helper()
	n, err := node.Parse("nodes/"+id[:3]+"/"+id+".md", []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func whats(r Result) []string {
	var out []string
	for _, n := range r.Next {
		out = append(out, n.What)
	}
	return out
}

func yes(string) bool { return true }

const src = "sources:\n  - url: https://en.wikipedia.org/wiki/Group_(mathematics)\n    locator: Definition\n"

func TestAssess(t *testing.T) {
	concept := parse(t, "G5S1M9", "---\ntag: G5S1M9\nkind: concept\nname: Group\nspecializes: [9ZZZZZ]\n"+src+"---\n\n## Summary\n\nA group is a set with an operation.\n\n## Remarks\n\n### History\n\nGalois used the word.\n\n## Explanations\n\n### L1 human\n\nThe symmetries of a square.\n\n### L2 human\n\nThe axioms.\n\n### L3 human\n\nSymmetry.\n")
	def := parse(t, "1D5F2A", "---\ntag: 1D5F2A\nkind: definition\nname: Group\ndefines: G5S1M9\n"+src+"---\n\n## Definition\n\nA **group** is a set $G$ with an operation.\n")
	ex1 := parse(t, "2E5X1A", "---\ntag: 2E5X1A\nkind: example\nname: The integers\ninstance_of: [G5S1M9]\n"+src+"---\n\n## Example\n\n$\\mathbb{Z}$ under addition.\n")
	ex2 := parse(t, "2E5X2B", "---\ntag: 2E5X2B\nkind: example\nname: A cyclic group\ninstance_of: [G5S1M9]\n"+src+"---\n\n## Example\n\n$\\mathbb{Z}/n$.\n")
	non := parse(t, "2E5X3C", "---\ntag: 2E5X3C\nkind: example\nname: The natural numbers\nnot_instance_of:\n  - target: G5S1M9\n    fails: [3]\n"+src+"---\n\n## Example\n\n$\\mathbb{N}$ has no inverses.\n")
	bare := parse(t, "9ZZZZZ", "---\ntag: 9ZZZZZ\nkind: concept\nname: Ring\n---\n\n## Summary\n\nA ring.\n")
	all := []*node.Node{concept, def, ex1, ex2, non, bare}
	g := graph.Build(all)
	c := Embedded()

	r := c.Assess(concept, Env{Graph: g, Row: yes})
	if r.Level != W2 || strings.Join(whats(r), "; ") != "section Motivation; a person confirmed each agent-written section" {
		t.Errorf("the concept is %s with %q", r.Level, whats(r))
	}
	r = c.Assess(concept, Env{Graph: g, Row: yes, Confirmed: yes})
	if r.Level != W2 || len(r.Next) != 1 {
		t.Errorf("the confirmed concept is %s with %q", r.Level, whats(r))
	}
	r = c.Assess(concept, Env{Graph: g})
	if r.Level != W0 || whats(r)[0] != "an inventory row" {
		t.Errorf("the concept with no row is %s with %q", r.Level, whats(r))
	}
	r = c.Assess(concept, Env{Graph: g, Row: yes, Clean: func(string) bool { return false }})
	if r.Level != W0 || strings.Join(whats(r), "; ") != "a body that passes B01 to B03" {
		t.Errorf("the concept with a broken body is %s with %q", r.Level, whats(r))
	}

	// The examples of the concept count for its definition.
	r = c.Assess(def, Env{Graph: g, Row: yes})
	if r.Level != W2 {
		t.Errorf("the definition is %s with %q", r.Level, whats(r))
	}
	r = c.Assess(bare, Env{Graph: g, Row: yes})
	if r.Level != W0 || strings.Join(whats(r), "; ") != "a same_as or a source; a source with a locator" {
		t.Errorf("the bare concept is %s with %q", r.Level, whats(r))
	}
	r = c.Assess(concept, Env{Row: yes})
	if r.Level != W1 || len(r.Next) != 4 {
		t.Errorf("the concept with no graph is %s with %q", r.Level, whats(r))
	}
}

func TestDefinitionsCondition(t *testing.T) {
	concept := parse(t, "G5S1M9", "---\ntag: G5S1M9\nkind: concept\nname: Group\n"+src+"---\n\n## Summary\n\nA group.\n")
	d1 := parse(t, "1D5F2A", "---\ntag: 1D5F2A\nkind: definition\nname: Group\ndefines: G5S1M9\n"+src+"---\n\n## Definition\n\nA **group**.\n")
	d2 := parse(t, "1D5F2B", "---\ntag: 1D5F2B\nkind: definition\nname: Group by division\ndefines: G5S1M9\n"+src+"---\n\n## Definition\n\nA **group**.\n")
	g := graph.Build([]*node.Node{concept, d1})
	if r := Embedded().Assess(concept, Env{Graph: g, Row: yes}); strings.Contains(strings.Join(whats(r), ";"), "equivalent") {
		t.Errorf("one definition asks for an equivalence: %q", whats(r))
	}
	g = graph.Build([]*node.Node{concept, d1, d2})
	if r := Embedded().Assess(concept, Env{Graph: g, Row: yes}); !strings.Contains(strings.Join(whats(r), ";"), "edge equivalent in") {
		t.Errorf("two definitions ask for no equivalence: %q", whats(r))
	}
}

func TestAudience(t *testing.T) {
	text := "---\ntag: G5S1M9\nkind: concept\nname: Group\naudience: grad\n" + src + "---\n\n## Summary\n\nA group.\n\n## Explanations\n\n### L3 agent\n\nSymmetry.\n"
	a := assessor{n: parse(t, "G5S1M9", text)}
	a.sections = check.Sections(a.n)
	if have, want := a.count(Item{Explain: Audience}); have != 1 || want != 2 {
		t.Errorf("the grad explanations count %d of %d, and want 1 of 2", have, want)
	}
}

func TestLevelText(t *testing.T) {
	data, err := json.Marshal(Result{Tag: "G5S1M9", Kind: "concept", Level: W2})
	if err != nil || string(data) != `{"tag":"G5S1M9","kind":"concept","level":"W2"}` {
		t.Fatalf("got %s, %v", data, err)
	}
	var r Result
	if err := json.Unmarshal(data, &r); err != nil || r.Level != W2 {
		t.Errorf("got %v, %v", r.Level, err)
	}
	if err := json.Unmarshal([]byte(`{"level":"W7"}`), &r); err == nil {
		t.Error("W7 is a level")
	}
}

func TestParse(t *testing.T) {
	if got := Embedded().Kinds(); len(got) != 11 {
		t.Errorf("the embedded cards have the kinds %v, and want 11", got)
	}
	for name, text := range map[string]string{
		"version":   "version: cards.v2\ncards: {}\n",
		"two":       "version: cards.v1\ncards:\n  concept:\n    - {section: Summary, field: name, level: R}\n",
		"none":      "version: cards.v1\ncards:\n  concept:\n    - {level: R}\n",
		"level":     "version: cards.v1\ncards:\n  concept:\n    - {section: Summary, level: W4}\n",
		"edge":      "version: cards.v1\ncards:\n  concept:\n    - {edge: [likes], dir: in, level: W2}\n",
		"dir":       "version: cards.v1\ncards:\n  concept:\n    - {edge: [uses], level: W2}\n",
		"explain":   "version: cards.v1\ncards:\n  concept:\n    - {explain: L5, level: W2}\n",
		"condition": "version: cards.v1\ncards:\n  concept:\n    - {section: Summary, level: R, when: sunny}\n",
	} {
		if _, err := Parse([]byte(text), "test"); err == nil {
			t.Errorf("%s: Parse gives no error", name)
		}
	}
}

func TestLoad(t *testing.T) {
	root := t.TempDir()
	c, err := Load(root)
	if err != nil || c.Source != "embedded" {
		t.Fatalf("got %v, %v", c, err)
	}
	if err := os.MkdirAll(filepath.Join(root, "schema"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, File), []byte("version: cards.v1\ncards:\n  topic:\n    - {section: Summary, level: R}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err = Load(root)
	if err != nil || c.Source != File || len(c.Kinds()) != 1 {
		t.Fatalf("got %v, %v", c, err)
	}
	if items, ok := c.Card("topic"); !ok || items[0].String() != "section Summary" {
		t.Errorf("got %v, %v", items, ok)
	}
}

func TestCount(t *testing.T) {
	concept := parse(t, "G5S1M9", "---\ntag: G5S1M9\nkind: concept\nname: Group\n"+src+"---\n\n## Summary\n\nA group.\n")
	ex := parse(t, "2E5X1A", "---\ntag: 2E5X1A\nkind: example\nname: The integers\ninstance_of: [G5S1M9]\n"+src+"---\n\n## Example\n\n$\\mathbb{Z}$ under addition.\n")
	env := Env{Graph: graph.Build([]*node.Node{concept, ex})}
	for _, tc := range []struct {
		it         Item
		have, want int
	}{
		{Item{Section: "Summary", Level: "R"}, 1, 1},
		{Item{Section: "Motivation", Level: "W3"}, 0, 1},
		{Item{Edge: []string{graph.InstanceOf}, Dir: "in", Min: 2, Level: "W2"}, 1, 2},
		{Item{Edge: []string{graph.Equivalent}, Dir: "in", When: Definitions, Level: "W2"}, 0, 0},
	} {
		if have, want := Count(concept, env, tc.it); have != tc.have || want != tc.want {
			t.Errorf("Count(%s) = %d, %d, want %d, %d", tc.it, have, want, tc.have, tc.want)
		}
	}
	if l, ok := (Item{Level: "W2"}).Needs(); !ok || l != W2 {
		t.Errorf("the level W2 needs %s, %v", l, ok)
	}
	if _, ok := (Item{Level: "o"}).Needs(); ok {
		t.Error("an optional item needs a level")
	}
}
