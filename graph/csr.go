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
	var es [][]Edge
	if len(types) == 0 {
		es = append(es, g.Edges)
	}
	for _, t := range types {
		es = append(es, g.typed(t))
	}
	ids, pairs := g.ends(es...)
	return fromPairs(ids, slices.Concat(pairs...), false)
}

// ends gives the node tags and the ends of the edges in es, in byte order, and the edges of each slice of es as pairs of positions in the ids.
func (g *Graph) ends(es ...[]Edge) ([]string, [][][2]int32) {
	pos := make(map[string]int32, len(g.Nodes))
	ids := make([]string, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		pos[n.Tag] = int32(len(ids)) // #nosec G115 -- there are fewer than 2^31 ids.
		ids = append(ids, n.Tag)
	}
	nodes := len(ids)
	at := func(id string) int32 {
		p, ok := pos[id]
		if !ok {
			p = int32(len(ids)) // #nosec G115 -- there are fewer than 2^31 ids.
			pos[id] = p
			ids = append(ids, id)
		}
		return p
	}
	pairs := make([][][2]int32, len(es))
	for k, list := range es {
		pairs[k] = make([][2]int32, len(list))
		for i, e := range list {
			pairs[k][i] = [2]int32{at(e.From), at(e.To)}
		}
	}
	if len(ids) == nodes {
		return ids, pairs
	}
	// Merge the ends that are not nodes into the node tags, and move the pairs to the new positions.
	extra := make([]int32, len(ids)-nodes)
	for i := range extra {
		extra[i] = int32(nodes + i) // #nosec G115 -- there are fewer than 2^31 ids.
	}
	slices.SortFunc(extra, func(a, b int32) int { return strings.Compare(ids[a], ids[b]) })
	moved := make([]int32, len(ids))
	merged := make([]string, 0, len(ids))
	i, j := 0, 0
	for i < nodes || j < len(extra) {
		var p int32
		if j == len(extra) || i < nodes && ids[i] < ids[extra[j]] {
			p = int32(i) // #nosec G115 -- there are fewer than 2^31 ids.
			i++
		} else {
			p = extra[j]
			j++
		}
		moved[p] = int32(len(merged)) // #nosec G115 -- there are fewer than 2^31 ids.
		merged = append(merged, ids[p])
	}
	for _, list := range pairs {
		for i := range list {
			list[i] = [2]int32{moved[list[i][0]], moved[list[i][1]]}
		}
	}
	return merged, pairs
}

// fromPairs makes the adjacency of the pairs over the ids. With reverse, each pair goes from its second id to its first. The pairs do not change.
func fromPairs(ids []string, pairs [][2]int32, reverse bool) *CSR {
	from, to := 0, 1
	if reverse {
		from, to = 1, 0
	}
	c := &CSR{IDs: ids, Offsets: make([]int32, len(ids)+1), Targets: make([]int32, len(pairs))}
	for _, p := range pairs {
		c.Offsets[p[from]+1]++
	}
	for i := range ids {
		c.Offsets[i+1] += c.Offsets[i]
	}
	fill := slices.Clone(c.Offsets[:len(ids)])
	for _, p := range pairs {
		c.Targets[fill[p[from]]] = p[to]
		fill[p[from]]++
	}
	// Sort each row and remove the pairs that occur two times.
	w := int32(0)
	for i := range ids {
		row := c.Targets[c.Offsets[i]:c.Offsets[i+1]]
		slices.Sort(row)
		c.Offsets[i] = w
		for k, t := range row {
			if k == 0 || t != row[k-1] {
				c.Targets[w] = t
				w++
			}
		}
	}
	c.Offsets[len(ids)] = w
	c.Targets = slices.Clip(c.Targets[:w])
	return c
}

// Index returns the position of id in IDs.
func (c *CSR) Index(id string) (int, bool) {
	return slices.BinarySearchFunc(c.IDs, id, strings.Compare)
}

// Out returns the positions of the ends of the edges from the id at position i.
func (c *CSR) Out(i int) []int32 { return c.Targets[c.Offsets[i]:c.Offsets[i+1]] }
