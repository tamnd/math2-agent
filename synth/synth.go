// Package synth makes synthetic knowledge graphs for benchmarks and load tests.
//
// The performance targets of spec 02.4 hold at reference scale: 5 million nodes in the statement plane. The real graph is smaller for a long time, so the benchmarks of the graph queries (spec 05.6) fill the gap with synthetic nodes. Make gives a graph with the shape of the real one: concepts with their definitions, statements with one or two proofs, open conjectures with bounds, topics, MSC codes, Lean bindings and Wikidata ids. The uses edges go mostly to nodes of the same MSC class and of the levels just below, so a closure has a realistic size and a depth of about 30.
//
// Make is deterministic: the same Options give the same graph.
package synth

import (
	"fmt"
	"math/rand/v2"

	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/tag"
)

// Options sets the size and the seed of a synthetic graph.
type Options struct {
	Nodes int    // the number of nodes, about. Make gives at least this many.
	Seed  uint64 // the seed of the random numbers
}

// Data is a synthetic graph with the status, the MSC codes and the aliases of its nodes.
type Data struct {
	Graph   *graph.Graph
	Status  []status.Status
	MSC     map[string][]string // the MSC codes of each node
	Aliases map[string]string   // the tag of each alias
}

// Levels is the number of levels of the graph. Each statement uses nodes of lower levels, and the uses edges of a level go down one level or two, so a closure from the top level has a depth of about 2 × Levels.
const Levels = 15

// Classes is the number of MSC classes of the graph.
const Classes = 60

// Tag returns the tag of the synthetic node with the index i. The tags start with 1, so they do not collide with the tags of the seed, which start with 0.
func Tag(i int) string {
	v := uint64(i) + 1<<25 // #nosec G115 -- i is not negative.
	b := make([]byte, 6)
	for k := 5; k >= 0; k-- {
		b[k] = tag.Alphabet[v&31]
		v >>= 5
	}
	return string(b)
}

// maker holds the state of Make.
type maker struct {
	r       *rand.Rand
	nodes   []graph.Node
	edges   []graph.Edge
	sts     []status.Status
	msc     map[string][]string
	aliases map[string]string
	// The nodes that a statement or a proof can use, for each class and each level.
	usable [Classes][Levels][]int
	defs   [Classes][Levels][]int
	open   [Classes][]int
	topics [Classes]int
}

// Make makes a synthetic graph.
func Make(o Options) *Data {
	m := &maker{r: rand.New(rand.NewPCG(o.Seed, 0x6d32)), msc: map[string][]string{}, aliases: map[string]string{}} // #nosec G404 -- the graph is synthetic, and it must repeat.
	for c := range Classes {
		m.topics[c] = m.add("topic", fmt.Sprintf("Topic %d", c), c, status.None, status.Unbound)
		m.aliases[fmt.Sprintf("topic-%d", c)] = m.nodes[m.topics[c]].Tag
	}
	per := max(1, (o.Nodes-Classes+Classes*Levels-1)/(Classes*Levels))
	for l := range Levels {
		for c := range Classes {
			m.level(c, l, per)
		}
	}
	return &Data{Graph: graph.New(m.nodes, m.edges), Status: m.sts, MSC: m.msc, Aliases: m.aliases}
}

// level adds at least n nodes to the class c at the level l.
func (m *maker) level(c, l, n int) {
	for start := len(m.nodes); len(m.nodes)-start < n || len(m.usable[c][l]) == 0; {
		switch k := m.r.IntN(100); {
		case k < 12:
			// A concept and its definition.
			cn := m.add("concept", "", c, status.None, status.Unbound)
			d := m.add("definition", "", c, status.None, status.Unbound)
			m.edge(graph.Defines, d, cn)
			for range 1 + m.r.IntN(3) {
				if t, ok := m.pick(m.defs[:], c, l); ok {
					m.edge(graph.Uses, d, t)
				}
			}
			if m.r.IntN(3) == 0 {
				m.edges = append(m.edges, graph.Edge{Type: graph.SameAs, From: m.nodes[cn].Tag, To: fmt.Sprintf("Q%d", 1000+cn), System: "wikidata"})
			}
			m.aliases[fmt.Sprintf("concept-%d", cn)] = m.nodes[cn].Tag
			m.defs[c][l] = append(m.defs[c][l], d)
			m.usable[c][l] = append(m.usable[c][l], d)
		case k < 14 && l > 0:
			// An open conjecture, part of the topic of the class, with a bound.
			conj := m.add("conjecture", "", c, status.Open, status.Unbound)
			m.edge(graph.PartOf, conj, m.topics[c])
			b := m.statement(c, l)
			m.edges = append(m.edges, graph.Edge{Type: graph.Bounds, From: m.nodes[b].Tag, To: m.nodes[conj].Tag, Label: fmt.Sprintf("lower %d", 1+m.r.IntN(1000))})
			m.open[c] = append(m.open[c], conj)
		default:
			m.statement(c, l)
		}
	}
}

// statement adds a statement with its proofs, and returns its index.
func (m *maker) statement(c, l int) int {
	ev, fid := status.Claimed, status.Unbound
	formal := m.r.IntN(5) == 0
	if formal {
		fid = status.Fidelity(1 + m.r.IntN(2)) // #nosec G115 -- the value is 1 or 2.
		if m.r.IntN(2) == 0 {
			ev = status.Kernel
		}
	}
	s := m.add("statement", "", c, ev, fid)
	if formal {
		decl := fmt.Sprintf("Synth.decl_%d", s)
		m.edges = append(m.edges, graph.Edge{Type: graph.Formalizes, From: decl, To: m.nodes[s].Tag, System: "lean4/mathlib", Label: "exact"})
		m.sts[s].Bindings = []status.Binding{{Decl: decl, Match: "exact", Axioms: axioms[m.r.IntN(len(axioms))], Fidelity: fid, Level: ev}}
	}
	for range 1 + m.r.IntN(2) {
		if t, ok := m.pick(m.defs[:], c, l); ok {
			m.edge(graph.Uses, s, t)
		}
	}
	if len(m.open[c]) > 0 && m.r.IntN(200) == 0 {
		m.edge(graph.Uses, s, m.open[c][m.r.IntN(len(m.open[c]))])
	}
	if l > 0 && m.r.IntN(20) == 0 {
		return s // a statement with no proof yet
	}
	for range 1 + m.r.IntN(2) {
		p := m.add("proof", "", c, status.Claimed, status.Unbound)
		m.edge(graph.Proves, p, s)
		for range 2 + m.r.IntN(5) {
			if t, ok := m.pick(m.usable[:], c, l); ok {
				m.edge(graph.Uses, p, t)
			}
		}
	}
	m.usable[c][l] = append(m.usable[c][l], s)
	return s
}

var axioms = [][]string{{}, {"propext"}, {"Classical.choice", "Quot.sound", "propext"}}

// pick returns a node of a lower level to use: mostly of the same class and of the level just below.
func (m *maker) pick(from [][Levels][]int, c, l int) (int, bool) {
	if l == 0 {
		return 0, false
	}
	if m.r.IntN(10) == 0 {
		c = m.r.IntN(Classes)
	}
	low := l - 1
	if m.r.IntN(4) == 0 {
		low = m.r.IntN(l)
	}
	pool := from[c][low]
	if len(pool) == 0 {
		return 0, false
	}
	return pool[m.r.IntN(len(pool))], true
}

// add adds a node and its status, and returns its index.
func (m *maker) add(kind, name string, c int, ev status.Level, fid status.Fidelity) int {
	i := len(m.nodes)
	t := Tag(i)
	if name == "" {
		name = fmt.Sprintf("Synthetic %s %d", kind, i)
	}
	m.nodes = append(m.nodes, graph.Node{Tag: t, Kind: kind, Name: name})
	m.sts = append(m.sts, status.Status{Tag: t, Kind: kind, Name: name, Evidence: ev, Fidelity: fid})
	code := fmt.Sprintf("%02dA%02d", 3+c, m.r.IntN(100))
	if kind == "topic" {
		code = fmt.Sprintf("%02dAxx", 3+c)
	}
	m.msc[t] = []string{code}
	return i
}

func (m *maker) edge(t string, from, to int) {
	m.edges = append(m.edges, graph.Edge{Type: t, From: m.nodes[from].Tag, To: m.nodes[to].Tag})
}
