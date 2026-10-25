package graph

import (
	"slices"
	"strings"
)

// New makes a graph from nodes and edges, for example from the tables of a store. It sorts both and removes each edge that occurs two times, as Build does. The same nodes and edges give the same Hash as Build.
func New(nodes []Node, edges []Edge) *Graph {
	g := &Graph{Nodes: slices.Clone(nodes), Edges: slices.Clone(edges)}
	slices.SortFunc(g.Nodes, func(a, b Node) int { return strings.Compare(a.Tag, b.Tag) })
	slices.SortFunc(g.Edges, Edge.compare)
	g.Edges = slices.Compact(g.Edges)
	g.init()
	return g
}

// Needs returns the nodes that have an alternative and that lose each alternative when the node with the tag is false (query Q4 of spec 05.6). These are the nodes whose every proof uses the node, directly or through other nodes. The node itself is not in the result. The result is in byte order, and it is empty when the tag is not in the graph.
func (g *Graph) Needs(tag string) []string {
	s := g.solve(MinDepth)
	x, ok := s.uses.Index(tag)
	if _, isNode := g.index[tag]; !ok || !isNode {
		return nil
	}
	n := len(s.uses.IDs)
	usedBy, proves := s.usedBy(), s.proves()
	waiting := make([]int, n)
	proved, done := make([]bool, n), make([]bool, n)
	var queue []int
	for i := range n {
		waiting[i] = len(s.uses.Out(i))
		if i != x && waiting[i] == 0 && len(s.proofs.Out(i)) == 0 {
			queue = append(queue, i)
		}
	}
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		if done[i] {
			continue
		}
		done[i] = true
		next := func(p int) {
			if p != x && !done[p] && waiting[p] == 0 && (proved[p] || len(s.proofs.Out(p)) == 0) {
				queue = append(queue, p)
			}
		}
		for _, p := range usedBy.Out(i) {
			waiting[p]--
			next(int(p))
		}
		for _, st := range proves.Out(i) {
			proved[st] = true
			next(int(st))
		}
	}
	var out []string
	for i := range n {
		if i != x && !done[i] && s.value[i] != infinite {
			if _, isNode := g.index[s.uses.IDs[i]]; isNode {
				out = append(out, s.uses.IDs[i])
			}
		}
	}
	return out
}

// Dependents returns the nodes that use the node with the tag in at least one alternative: each node from which a chain of uses and proves edges goes to the node, against the direction of the proves edges. A proof uses what it uses, and a statement uses each of its proofs. The node itself is not in the result. The result is in byte order.
func (g *Graph) Dependents(tag string) []string {
	s := g.solve(MinDepth)
	x, ok := s.uses.Index(tag)
	if _, isNode := g.index[tag]; !ok || !isNode {
		return nil
	}
	usedBy, proves := s.usedBy(), s.proves()
	seen := map[int]bool{x: true}
	queue := []int{x}
	var out []string
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		for _, next := range [][]int32{usedBy.Out(i), proves.Out(i)} {
			for _, p := range next {
				if !seen[int(p)] {
					seen[int(p)] = true
					queue = append(queue, int(p))
					if _, isNode := g.index[s.uses.IDs[p]]; isNode {
						out = append(out, s.uses.IDs[p])
					}
				}
			}
		}
	}
	slices.Sort(out)
	return out
}

// Path returns the shortest learning path from the node from to the node to (query Q5 of spec 05.6). The path uses only the uses edges that end at a definition and the defines edges, and it goes against their direction: to uses a definition, which uses another definition, which defines from. The path starts with from and ends with to. Of two paths with the same length, Path gives the first in byte order. Path returns false when no such path exists.
func (g *Graph) Path(from, to string) ([]string, bool) {
	if _, ok := g.index[from]; !ok {
		return nil, false
	}
	if _, ok := g.index[to]; !ok {
		return nil, false
	}
	prev := map[string]string{to: ""}
	queue := []string{to}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == from {
			path := []string{from}
			for p := prev[from]; p != ""; p = prev[p] {
				path = append(path, p)
			}
			return path, true
		}
		var next []string
		for _, e := range g.Out(Uses, cur) {
			if n, ok := g.Node(e.To); ok && n.Kind == "definition" {
				next = append(next, e.To)
			}
		}
		for _, e := range g.Out(Defines, cur) {
			if _, ok := g.index[e.To]; ok {
				next = append(next, e.To)
			}
		}
		slices.Sort(next)
		for _, n := range next {
			if _, ok := prev[n]; !ok {
				prev[n] = cur
				queue = append(queue, n)
			}
		}
	}
	return nil, false
}

// usedBy returns the uses edges against their direction.
func (s *solution) usedBy() *CSR { return s.reverse(s.uses) }

// proves returns the edges from each proof to its statements.
func (s *solution) proves() *CSR { return s.reverse(s.proofs) }

func (s *solution) reverse(c *CSR) *CSR {
	r := &CSR{IDs: c.IDs, Offsets: make([]int32, len(c.IDs)+1), Targets: make([]int32, len(c.Targets))}
	for i := range c.IDs {
		for _, j := range c.Out(i) {
			r.Offsets[j+1]++
		}
	}
	for i := range c.IDs {
		r.Offsets[i+1] += r.Offsets[i]
	}
	fill := slices.Clone(r.Offsets[:len(c.IDs)])
	for i := range c.IDs {
		for _, j := range c.Out(i) {
			r.Targets[fill[j]] = int32(i) // #nosec G115 -- there are fewer than 2^31 ids.
			fill[j]++
		}
	}
	return r
}
