// Package graph derives the knowledge graph from node files (spec 05).
//
// The graph is never edited. Build reads the edges from the front matter of the nodes, for each of the 23 edge types of spec 05.2. The edges from the formal plane and from the record plane come in later milestones. The same nodes give the same graph and the same Hash, also on a second machine.
//
// A Graph answers the questions of the build. Closure gives the best alternative of the dependencies of a node, as an AND-OR graph: a statement needs one of its proofs, and a node needs all of its uses (spec 05.5). Cycles gives the dependency cycles that rule G03 does not allow. CSR gives the adjacency in compressed sparse row form, and WriteParquet writes the build format of spec 05.7.
package graph

import (
	"cmp"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/zeebo/blake3"

	"github.com/tamnd/math2-agent/node"
)

// The three families of edge types.
const (
	Logical    = "logical"    // the edges that carry truth
	Conceptual = "conceptual" // the edges that carry meaning
	Record     = "record"     // the edges that carry provenance and identity
)

// The 23 edge types of spec 05.2.
const (
	Proves      = "proves"
	Uses        = "uses"
	Implies     = "implies"
	Equivalent  = "equivalent"
	Refutes     = "refutes"
	Resolves    = "resolves"
	Generalizes = "generalizes"
	Bounds      = "bounds"
	Defines     = "defines"
	InstanceOf  = "instance_of"
	About       = "about"
	Specializes = "specializes"
	Method      = "method"
	AnalogousTo = "analogous_to"
	PartOf      = "part_of"
	Formalizes  = "formalizes"
	Source      = "source"
	Aligned     = "aligned"
	SameAs      = "same_as"
	CloseTo     = "close_to"
	Cites       = "cites"
	ResolvedBy  = "resolved_by"
	Successor   = "successor"
)

var families = map[string]string{
	Proves: Logical, Uses: Logical, Implies: Logical, Equivalent: Logical, Refutes: Logical, Resolves: Logical, Generalizes: Logical, Bounds: Logical,
	Defines: Conceptual, InstanceOf: Conceptual, About: Conceptual, Specializes: Conceptual, Method: Conceptual, AnalogousTo: Conceptual, PartOf: Conceptual,
	Formalizes: Record, Source: Record, Aligned: Record, SameAs: Record, CloseTo: Record, Cites: Record, ResolvedBy: Record, Successor: Record,
}

// Types returns the 23 edge types in the order of spec 05.2.
func Types() []string {
	return []string{Proves, Uses, Implies, Equivalent, Refutes, Resolves, Generalizes, Bounds, Defines, InstanceOf, About, Specializes, Method, AnalogousTo, PartOf, Formalizes, Source, Aligned, SameAs, CloseTo, Cites, ResolvedBy, Successor}
}

// Family returns the family of the edge type t, or "" when t is not an edge type.
func Family(t string) string { return families[t] }

// Node is one node of the graph.
type Node struct {
	Tag     string `parquet:"tag"`
	Kind    string `parquet:"kind"`
	Role    string `parquet:"role"`
	Name    string `parquet:"name"`
	Retired bool   `parquet:"retired"`
	Hash    string `parquet:"hash"` // the content hash of the node file (spec 06.3)
}

// Edge is one typed and directed edge. Between two nodes, From and To are tags or provisional ids. For the other edges, one end is a node and the other end is an external id, a record or a formal declaration.
type Edge struct {
	Type   string `parquet:"type"`
	From   string `parquet:"from"`
	To     string `parquet:"to"`
	Via    string `parquet:"via"`    // the proof or the computation that supports the edge, or ""
	System string `parquet:"system"` // the external system, the kind of record or the formal system, or ""
	Label  string `parquet:"label"`  // the match, the outcome, the bound, the specialising map or the locator, or ""
}

func (e Edge) compare(f Edge) int {
	return cmp.Or(strings.Compare(e.Type, f.Type), strings.Compare(e.From, f.From), strings.Compare(e.To, f.To),
		strings.Compare(e.Via, f.Via), strings.Compare(e.System, f.System), strings.Compare(e.Label, f.Label))
}

// Graph is the graph of a set of nodes. A Graph does not change after Build, and its methods are safe for concurrent use. Nodes is sorted by tag. Edges is sorted by type, then by From, To, Via, System and Label, and it has no edge two times.
type Graph struct {
	Nodes []Node
	Edges []Edge

	index map[string]int   // the position of each tag in Nodes
	out   map[string][]int // the edges of each type that start at a node, as positions in Edges
	in    map[string][]int // the edges of each type that end at a node, as positions in Edges

	once   [2]sync.Once // one for each Measure
	solved [2]*solution
}

// Build derives the graph from the nodes. A value that is not a reference is not an edge to a node, and the schema rules report it. An edge can name a target that is not in nodes, and rule G01 reports it.
func Build(nodes []*node.Node) *Graph {
	g := &Graph{}
	for _, n := range nodes {
		_, retired := n.Meta["retired"]
		g.Nodes = append(g.Nodes, Node{Tag: n.ID, Kind: n.Kind(), Role: n.Get("role"), Name: n.Name(), Retired: retired, Hash: node.Hash(n)})
		g.Edges = append(g.Edges, edges(n)...)
	}
	slices.SortFunc(g.Nodes, func(a, b Node) int { return strings.Compare(a.Tag, b.Tag) })
	slices.SortFunc(g.Edges, Edge.compare)
	g.Edges = slices.Compact(g.Edges)
	g.init()
	return g
}

func (g *Graph) init() {
	g.index = make(map[string]int, len(g.Nodes))
	for i, n := range g.Nodes {
		g.index[n.Tag] = i
	}
	g.out, g.in = map[string][]int{}, map[string][]int{}
	for i, e := range g.Edges {
		g.out[e.Type+" "+e.From] = append(g.out[e.Type+" "+e.From], i)
		g.in[e.Type+" "+e.To] = append(g.in[e.Type+" "+e.To], i)
	}
}

// Node returns the node with the tag.
func (g *Graph) Node(tag string) (Node, bool) {
	i, ok := g.index[tag]
	if !ok {
		return Node{}, false
	}
	return g.Nodes[i], true
}

// Out returns the edges of type t that start at from, in the order of Edges.
func (g *Graph) Out(t, from string) []Edge { return g.pick(g.out[t+" "+from]) }

// In returns the edges of type t that end at to, in the order of Edges.
func (g *Graph) In(t, to string) []Edge { return g.pick(g.in[t+" "+to]) }

func (g *Graph) pick(pos []int) []Edge {
	out := make([]Edge, len(pos))
	for i, p := range pos {
		out[i] = g.Edges[p]
	}
	return out
}

// Hash returns the content hash of the graph: "blake3:" and 64 lowercase hex characters. The hash covers each node with its content hash and each edge. Two builds of the same nodes give the same hash (rule R01).
func (g *Graph) Hash() string {
	h := blake3.New()
	for _, n := range g.Nodes {
		fmt.Fprintf(h, "node\t%s\t%s\t%s\t%t\t%s\n", n.Tag, n.Kind, n.Role, n.Retired, n.Hash)
	}
	for _, e := range g.Edges {
		fmt.Fprintf(h, "edge\t%s\t%s\t%s\t%s\t%s\t%s\n", e.Type, e.From, e.To, e.Via, e.System, e.Label)
	}
	return node.HashPrefix + hex.EncodeToString(h.Sum(nil))
}
