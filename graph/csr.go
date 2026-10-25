package graph

import (
	"slices"
	"strings"
)

// CSR is the adjacency of some edge types in compressed sparse row form (spec 05.7). IDs holds each end of the edges and each node tag, in byte order. The edges from IDs[i] end at the IDs at the positions Targets[Offsets[i]:Offsets[i+1]], in byte order.
type CSR struct {
	IDs     []string
	Offsets []int32
	Targets []int32
}

// CSR returns the adjacency of the edges of the types. With no types, it uses each edge.
func (g *Graph) CSR(types ...string) *CSR {
	var es []Edge
	for _, e := range g.Edges {
		if len(types) == 0 || slices.Contains(types, e.Type) {
			es = append(es, e)
		}
	}
	return g.csr(es, false)
}

// csr makes the adjacency of es over the node tags and the ends of es. With reverse, each edge goes from To to From.
func (g *Graph) csr(es []Edge, reverse bool) *CSR {
	ids := make([]string, 0, len(g.Nodes)+len(es))
	for _, n := range g.Nodes {
		ids = append(ids, n.Tag)
	}
	for _, e := range es {
		ids = append(ids, e.From, e.To)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	c := &CSR{IDs: ids, Offsets: make([]int32, len(ids)+1)}
	pairs := make([][2]int32, 0, len(es))
	for _, e := range es {
		from, to := c.index(e.From), c.index(e.To)
		if reverse {
			from, to = to, from
		}
		pairs = append(pairs, [2]int32{from, to})
	}
	slices.SortFunc(pairs, func(a, b [2]int32) int {
		if a[0] != b[0] {
			return int(a[0] - b[0])
		}
		return int(a[1] - b[1])
	})
	pairs = slices.Compact(pairs)
	c.Targets = make([]int32, len(pairs))
	for i, p := range pairs {
		c.Offsets[p[0]+1]++
		c.Targets[i] = p[1]
	}
	for i := range ids {
		c.Offsets[i+1] += c.Offsets[i]
	}
	return c
}

// Index returns the position of id in IDs.
func (c *CSR) Index(id string) (int, bool) {
	return slices.BinarySearchFunc(c.IDs, id, strings.Compare)
}

func (c *CSR) index(id string) int32 {
	i, _ := c.Index(id)
	return int32(i) // #nosec G115 -- IDs has fewer than 2^31 items.
}

// Out returns the positions of the ends of the edges from the id at position i.
func (c *CSR) Out(i int) []int32 { return c.Targets[c.Offsets[i]:c.Offsets[i+1]] }
