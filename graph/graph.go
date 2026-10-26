// Package graph derives the knowledge graph from node files (spec 05).
//
// The graph is never edited. Build reads the edges from the front matter of the nodes, for each of the 25 edge types of spec 05.2 and corpus K8.9. The edges from the formal plane and from the record plane come in later milestones. The same nodes give the same graph and the same Hash, also on a second machine.
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

// The 25 edge types: the 23 of spec 05.2, and not_instance_of and illustrates of corpus K8.9.
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
	NotInstance = "not_instance_of"
	Illustrates = "illustrates"
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
	Defines: Conceptual, InstanceOf: Conceptual, NotInstance: Conceptual, Illustrates: Conceptual, About: Conceptual, Specializes: Conceptual, Method: Conceptual, AnalogousTo: Conceptual, PartOf: Conceptual,
	Formalizes: Record, Source: Record, Aligned: Record, SameAs: Record, CloseTo: Record, Cites: Record, ResolvedBy: Record, Successor: Record,
}

// Types returns the 25 edge types in the order of the schema. The two edges of corpus K8.9 come after instance_of.
func Types() []string {
	return []string{Proves, Uses, Implies, Equivalent, Refutes, Resolves, Generalizes, Bounds, Defines, InstanceOf, NotInstance, Illustrates, About, Specializes, Method, AnalogousTo, PartOf, Formalizes, Source, Aligned, SameAs, CloseTo, Cites, ResolvedBy, Successor}
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
	Type   string `parquet:"type" json:"type"`
	From   string `parquet:"from" json:"from"`
	To     string `parquet:"to" json:"to"`
	Via    string `parquet:"via" json:"via,omitempty"`       // the proof or the computation that supports the edge, or ""
	System string `parquet:"system" json:"system,omitempty"` // the external system, the kind of record or the formal system, or ""
	Label  string `parquet:"label" json:"label,omitempty"`   // the match, the outcome, the bound, the specialising map or the locator, or ""
}

// Compare compares two edges by type, then by From, To, Via, System and Label. It returns -1, 0 or +1. This is the order of Graph.Edges.
func (e Edge) Compare(f Edge) int {
	return cmp.Or(strings.Compare(e.Type, f.Type), strings.Compare(e.From, f.From), strings.Compare(e.To, f.To),
		strings.Compare(e.Via, f.Via), strings.Compare(e.System, f.System), strings.Compare(e.Label, f.Label))
}

// Graph is the graph of a set of nodes. A Graph does not change after Build, and its methods are safe for concurrent use. Nodes is sorted by tag. Edges is sorted by type, then by From, To, Via, System and Label, and it has no edge two times.
type Graph struct {
	Nodes []Node
	Edges []Edge

	in []int32 // the positions in Edges, sorted by type, then by To

	adjOnce sync.Once // for adj
	adj     *adjacency
	once    [2]sync.Once // one for each Measure
	solved  [2]*solution
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
	slices.SortFunc(g.Edges, Edge.Compare)
	g.Edges = slices.Compact(g.Edges)
	g.init()
	return g
}

func (g *Graph) init() {
	g.in = make([]int32, len(g.Edges))
	for i := range g.in {
		g.in[i] = int32(i) // #nosec G115 -- there are fewer than 2^31 edges.
	}
	// Edges is sorted by type, so a stable sort by type and To keeps the order of Edges within each end.
	slices.SortStableFunc(g.in, func(a, b int32) int {
		return cmp.Or(strings.Compare(g.Edges[a].Type, g.Edges[b].Type), strings.Compare(g.Edges[a].To, g.Edges[b].To))
	})
}

// Node returns the node with the tag.
func (g *Graph) Node(tag string) (Node, bool) {
	if i, ok := g.index(tag); ok {
		return g.Nodes[i], true
	}
	return Node{}, false
}

func (g *Graph) index(tag string) (int, bool) {
	return slices.BinarySearchFunc(g.Nodes, tag, func(n Node, t string) int { return strings.Compare(n.Tag, t) })
}

// Out returns the edges of type t that start at from, in the order of Edges.
func (g *Graph) Out(t, from string) []Edge {
	lo, _ := slices.BinarySearchFunc(g.Edges, Edge{Type: t, From: from}, func(e, k Edge) int {
		return cmp.Or(strings.Compare(e.Type, k.Type), strings.Compare(e.From, k.From))
	})
	hi := lo
	for hi < len(g.Edges) && g.Edges[hi].Type == t && g.Edges[hi].From == from {
		hi++
	}
	return slices.Clone(g.Edges[lo:hi])
}

// In returns the edges of type t that end at to, in the order of Edges.
func (g *Graph) In(t, to string) []Edge {
	lo, _ := slices.BinarySearchFunc(g.in, Edge{Type: t, To: to}, func(p int32, k Edge) int {
		return cmp.Or(strings.Compare(g.Edges[p].Type, k.Type), strings.Compare(g.Edges[p].To, k.To))
	})
	var out []Edge
	for _, p := range g.in[lo:] {
		if e := g.Edges[p]; e.Type == t && e.To == to {
			out = append(out, e)
			continue
		}
		break
	}
	return out
}

// typed returns the edges of type t.
func (g *Graph) typed(t string) []Edge {
	lo, _ := slices.BinarySearchFunc(g.Edges, t, func(e Edge, t string) int { return strings.Compare(e.Type, t) })
	hi := lo
	for hi < len(g.Edges) && g.Edges[hi].Type == t {
		hi++
	}
	return g.Edges[lo:hi:hi]
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
