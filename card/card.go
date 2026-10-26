// Package card reads the node cards of corpus spec K08 and computes the writing level of each node (corpus K2.4).
//
// A card lists the sections, the edges and the fields of a good node of one kind, in reading order, and the writing level that needs each one. The cards are data, in schema/cards.v1.yaml in tamnd/math2. Load reads them from a math2 working tree, and Embedded gives the copy in this package.
//
// Assess gives the writing level of a node and its work list: the items that the node lacks for the level above its level. The levels are W0 (stub), W1 (stated), W2 (usable) and W3 (complete), and each level needs each item of the levels below it. A node has at least W0. When it lacks a part of W0, its work list names that part too. W does not measure truth, and it does not depend on the evidence level.
package card

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/node"
)

// File is the path of the cards in a math2 working tree.
const File = "schema/cards.v1.yaml"

// Version is the version of the cards that this toolchain reads.
const Version = "cards.v1"

//go:embed cards.v1.yaml
var embedded []byte

// Level is a writing level.
type Level int

// The writing levels of corpus K2.4.
const (
	W0 Level = iota // stub: a tag, a kind, a name, an inventory row and a same_as or a source
	W1              // stated: the main section of the kind and a source with a locator
	W2              // usable: the items of the card for W2
	W3              // complete: the items of the card for W3, and a person confirmed each agent-written section
)

// String returns the name of the level, W0 to W3.
func (l Level) String() string { return "W" + strconv.Itoa(int(l)) }

// Word returns the short name of the level: stub, stated, usable or complete.
func (l Level) Word() string {
	if l < W0 || l > W3 {
		return ""
	}
	return [...]string{"stub", "stated", "usable", "complete"}[l]
}

// MarshalText gives the name of the level.
func (l Level) MarshalText() ([]byte, error) { return []byte(l.String()), nil }

// UnmarshalText reads the name of a level.
func (l *Level) UnmarshalText(b []byte) error {
	s, ok := strings.CutPrefix(string(b), "W")
	n, err := strconv.Atoi(s)
	if !ok || err != nil || n < int(W0) || n > int(W3) {
		return fmt.Errorf("card: %q is not a writing level", b)
	}
	*l = Level(n)
	return nil
}

// Item is one entry of a card. It has exactly one of Section, Edge, Field, Explain and Other.
type Item struct {
	Section string   `yaml:"section,omitempty"` // a level 2 section, or a level 3 heading of the same name in Remarks
	Edge    []string `yaml:"edge,omitempty"`    // edge types of the graph; the item counts the edges of each type
	Dir     string   `yaml:"dir,omitempty"`     // for an edge: in, out or both
	Field   string   `yaml:"field,omitempty"`   // a front matter field
	Explain string   `yaml:"explain,omitempty"` // an explanation level, L1 to L4, or audience for each level that the audience of the node asks for
	Other   string   `yaml:"other,omitempty"`   // work that the build does not see yet. It counts only through its condition
	Level   string   `yaml:"level"`             // R for W1, W2, W3, or o for optional
	Min     int      `yaml:"min,omitempty"`     // the smallest count; 0 means 1
	When    string   `yaml:"when,omitempty"`    // the condition under which the item counts, or ""
	Note    string   `yaml:"note,omitempty"`    // what the item holds, for the writer
}

// String names the item, for example "section Summary" or "edge instance_of in".
func (it Item) String() string {
	switch {
	case it.Section != "":
		return "section " + it.Section
	case len(it.Edge) > 0:
		return "edge " + strings.Join(it.Edge, " or ") + " " + it.Dir
	case it.Field != "":
		return "field " + it.Field
	case it.Explain == Audience:
		return "explanations for the audience"
	case it.Explain != "":
		return "explanation " + it.Explain
	}
	return it.Other
}

// Audience is the value of Item.Explain for the explanation levels that the audience of a node asks for (corpus K8.6).
const Audience = "audience"

// Definitions is the condition that holds when two or more definitions define the concept.
const Definitions = "definitions"

// Conditions are the values of Item.When. Only Definitions can hold now. The others need data that the build does not have yet: the formal libraries (library), the curricula (curriculum), the computational checks (computational) and the known counterexamples (known).
var Conditions = []string{Definitions, "library", "curriculum", "computational", "known"}

// levelNames maps the level of an item to the writing level that needs it.
var levelNames = map[string]Level{"R": W1, "W2": W2, "W3": W3}

// audienceLevels are the explanation levels that each audience asks for at W3. This is our reading of corpus K8.6: each level from the audience up to L3, L4 for grad and research, and L1 and L2 for school. A node with no audience asks for L1 to L3.
var audienceLevels = map[string][]string{
	"":         {"L1", "L2", "L3"},
	"school":   {"L1", "L2"},
	"ug1":      {"L1", "L2", "L3"},
	"ug2":      {"L1", "L2", "L3"},
	"ug3":      {"L2", "L3"},
	"ug4":      {"L2", "L3"},
	"grad":     {"L3", "L4"},
	"research": {"L3", "L4"},
}

// Cards are the cards of each kind.
type Cards struct {
	Source string // the file that the cards came from, or "embedded"
	cards  map[string][]Item
}

type file struct {
	Version string            `yaml:"version"`
	Cards   map[string][]Item `yaml:"cards"`
}

// Parse reads the cards in data. The source names the file in errors.
func Parse(data []byte, source string) (*Cards, error) {
	var f file
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	if f.Version != Version {
		return nil, fmt.Errorf("%s: the version is %q, and this toolchain reads %s", source, f.Version, Version)
	}
	types := graph.Types()
	for kind, items := range f.Cards {
		for i, it := range items {
			at := fmt.Sprintf("%s: the card %s, item %d", source, kind, i+1)
			set := 0
			for _, s := range []string{it.Section, it.Field, it.Explain, it.Other} {
				if s != "" {
					set++
				}
			}
			if len(it.Edge) > 0 {
				set++
			}
			if set != 1 {
				return nil, fmt.Errorf("%s has %d of section, edge, field, explain and other, and it must have one", at, set)
			}
			if _, ok := levelNames[it.Level]; !ok && it.Level != "o" {
				return nil, fmt.Errorf("%s has the level %q, and it must be R, W2, W3 or o", at, it.Level)
			}
			for _, t := range it.Edge {
				if !slices.Contains(types, t) {
					return nil, fmt.Errorf("%s has the edge type %q, which the graph does not have", at, t)
				}
			}
			if len(it.Edge) > 0 && it.Dir != "in" && it.Dir != "out" && it.Dir != "both" {
				return nil, fmt.Errorf("%s has the direction %q, and it must be in, out or both", at, it.Dir)
			}
			if it.Explain != "" && it.Explain != Audience && !slices.Contains(audienceLevels[""], it.Explain) && it.Explain != "L4" {
				return nil, fmt.Errorf("%s has the explanation %q, and it must be L1 to L4 or audience", at, it.Explain)
			}
			if it.When != "" && !slices.Contains(Conditions, it.When) {
				return nil, fmt.Errorf("%s has the condition %q, which the toolchain does not know", at, it.When)
			}
			if it.Min < 0 {
				return nil, fmt.Errorf("%s has the count %d, which is negative", at, it.Min)
			}
		}
	}
	return &Cards{Source: source, cards: f.Cards}, nil
}

var (
	embeddedOnce  sync.Once
	embeddedCards *Cards
)

// Embedded returns the cards in this package.
func Embedded() *Cards {
	embeddedOnce.Do(func() {
		c, err := Parse(embedded, "embedded")
		if err != nil {
			panic("card: the embedded cards do not parse: " + err.Error())
		}
		embeddedCards = c
	})
	return embeddedCards
}

// Load reads the cards from the working tree at root. When the tree has no cards file, Load returns the embedded copy.
func Load(root string) (*Cards, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(File)))
	if errors.Is(err, fs.ErrNotExist) {
		return Embedded(), nil
	}
	if err != nil {
		return nil, err
	}
	return Parse(data, File)
}

// Kinds returns the kinds that have a card, in byte order.
func (c *Cards) Kinds() []string {
	var out []string
	for k := range c.cards {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// Card returns the items of the card of a kind, in reading order.
func (c *Cards) Card(kind string) ([]Item, bool) {
	items, ok := c.cards[kind]
	return slices.Clone(items), ok
}

// Env is what Assess needs to know about the tree besides the node. A nil function is a test that no node passes, except Clean, where nil means that each body passes.
type Env struct {
	Graph     *graph.Graph          // the graph of the tree; nil means a graph with no edges
	Row       func(tag string) bool // reports whether an inventory row accounts for the node
	Clean     func(tag string) bool // reports whether the body of the node passes the rules B01 to B03
	Confirmed func(tag string) bool // reports whether a person confirmed each agent-written section of the node (rule P04)
}

// Need is one entry of a work list.
type Need struct {
	What string `json:"what"`           // the item, for example "section Summary"
	Note string `json:"note,omitempty"` // what the item holds
	Have int    `json:"have"`           // the count that the node has
	Want int    `json:"want"`           // the count that the level needs
}

// Result is the writing level of a node and its work list.
type Result struct {
	Tag   string `json:"tag"`
	Kind  string `json:"kind"`
	Level Level  `json:"level"`
	Next  []Need `json:"next,omitempty"` // what the node lacks for the level above its level; empty at W3
}

// Assess computes the writing level of n and its work list, from the card of its kind.
func (c *Cards) Assess(n *node.Node, env Env) Result {
	kind := n.Kind()
	a := assessor{n: n, env: env, sections: check.Sections(n)}
	var needs [W3 + 1][]Need
	add := func(l Level, what, note string, have, want int) {
		if have < want {
			needs[l] = append(needs[l], Need{What: what, Note: note, Have: have, Want: want})
		}
	}
	add(W0, "field name", "", count(n.Name() != ""), 1)
	add(W0, "a same_as or a source", "", count(n.Meta["same_as"] != nil || len(a.sources()) > 0), 1)
	add(W0, "an inventory row", "the row of the inventory that the node accounts for", count(env.Row != nil && env.Row(n.ID)), 1)
	add(W1, "a source with a locator", "", count(slices.ContainsFunc(a.sources(), func(m map[string]any) bool { return m["locator"] != nil && m["locator"] != "" })), 1)
	add(W1, "a body that passes B01 to B03", "", count(env.Clean == nil || env.Clean(n.ID)), 1)
	for _, it := range c.cards[kind] {
		l, ok := levelNames[it.Level]
		if !ok || !a.holds(it.When) {
			continue
		}
		have, want := a.count(it)
		add(l, it.String(), it.Note, have, want)
	}
	add(W3, "a person confirmed each agent-written section", "rule P04", count(env.Confirmed != nil && env.Confirmed(n.ID)), 1)

	r := Result{Tag: n.ID, Kind: kind, Level: W3}
	for l := W0; l <= W3; l++ {
		if len(needs[l]) > 0 {
			r.Level = max(l-1, W0)
			r.Next = needs[l]
			if l == W0 {
				r.Next = append(r.Next, needs[W1]...)
			}
			break
		}
	}
	return r
}

func count(ok bool) int {
	if ok {
		return 1
	}
	return 0
}

type assessor struct {
	n        *node.Node
	env      Env
	sections []check.Section
}

func (a assessor) sources() []map[string]any {
	var out []map[string]any
	list, _ := a.n.Meta["sources"].([]any)
	for _, x := range list {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// holds reports whether the condition of an item holds.
func (a assessor) holds(when string) bool {
	switch when {
	case "":
		return true
	case Definitions:
		return len(a.edges([]string{graph.Defines}, "in")) >= 2
	}
	return false
}

// count gives the count of an item that the node has, and the count that the item needs.
func (a assessor) count(it Item) (have, want int) {
	want = max(it.Min, 1)
	switch {
	case it.Section != "":
		return count(a.hasSection(it.Section)), 1
	case len(it.Edge) > 0:
		return len(a.edges(it.Edge, it.Dir)), want
	case it.Field != "":
		return count(!empty(a.n.Meta[it.Field])), 1
	case it.Explain == Audience:
		levels := audienceLevels[a.n.Get("audience")]
		for _, l := range levels {
			if a.hasExplanation(l) {
				have++
			}
		}
		return have, len(levels)
	case it.Explain != "":
		return count(a.hasExplanation(it.Explain)), 1
	}
	return 0, want
}

// hasSection reports whether the body has a section with the name and some text, or a level 3 heading with the name in Remarks. The heading in Remarks is the place of a section that node.v1 does not allow yet (corpus K8.4).
func (a assessor) hasSection(name string) bool {
	for _, s := range a.sections {
		if s.Name == name && strings.TrimSpace(s.Text) != "" {
			return true
		}
		if s.Name == "Remarks" && slices.ContainsFunc(s.Headings, func(h check.Heading) bool { return h.Level == 3 && h.Name == name }) {
			return true
		}
	}
	return false
}

// hasExplanation reports whether the Explanations section has a level 3 heading for the level, such as "L2 human".
func (a assessor) hasExplanation(level string) bool {
	for _, s := range a.sections {
		if s.Name != "Explanations" {
			continue
		}
		for _, h := range s.Headings {
			if first, _, _ := strings.Cut(h.Name, " "); h.Level == 3 && first == level {
				return true
			}
		}
	}
	return false
}

// edges returns the other ends of the edges of the types in the direction, each one time. For a definition, the edges into the concept that it defines count as edges into the definition, so that an example of the concept is an example of its definition.
func (a assessor) edges(types []string, dir string) []string {
	g := a.env.Graph
	if g == nil {
		return nil
	}
	targets := []string{a.n.ID}
	if a.n.Kind() == "definition" {
		if c := a.n.Get("defines"); c != "" && dir != "out" {
			targets = append(targets, c)
		}
	}
	var out []string
	for _, t := range types {
		for _, to := range targets {
			if dir == "in" || dir == "both" {
				for _, e := range g.In(t, to) {
					out = append(out, e.From)
				}
			}
			if (dir == "out" || dir == "both") && to == a.n.ID {
				for _, e := range g.Out(t, to) {
					out = append(out, e.To)
				}
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func empty(v any) bool {
	switch v := v.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	}
	return false
}
