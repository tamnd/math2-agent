package graph

import (
	"slices"
	"strings"
)

// New makes a graph from nodes and edges, for example from the tables of a store. It sorts both and removes each edge that occurs two times, as Build does. The same nodes and edges give the same Hash as Build.
func New(nodes []Node, edges []Edge) *Graph {
	g := &Graph{Nodes: slices.Clone(nodes), Edges: slices.Clone(edges)}
	slices.SortFunc(g.Nodes, func(a, b Node) int { return strings.Compare(a.Tag, b.Tag) })
	slices.SortFunc(g.Edges, Edge.Compare)
	g.Edges = slices.Compact(g.Edges)
	g.init()
	return g
}

// Needs returns the nodes that have an alternative and that lose each alternative when the node with the tag is false (query Q4 of spec 05.6). These are the nodes whose every proof uses the node, directly or through other nodes. The node itself is not in the result. The result is in byte order, and it is empty when the tag is not in the graph.
func (g *Graph) Needs(tag string) []string {
	s := g.solve(MinDepth)
	x, ok := s.uses.Index(tag)
	if !ok || s.pos[x] < 0 {
		return nil
	}
	// Only the dependents of x can lose an alternative. Another node keeps its value, and it has an alternative when its value is finite. A node with an infinite value has no alternative, and it does not get one when x is false, so it is never ready.
	w := s.scratch()
	defer s.release(w)
	deps := s.dependents(x, w, true)
	in, waiting, state, facts := w.in, w.waiting, w.state, s.facts
	queue, head := w.queue[:0], 0
	ready := func(i int32) {
		if state[i]&done == 0 && waiting[i] == 0 && facts[i]&finite != 0 && (state[i]&proved != 0 || facts[i]&hasProofs == 0) {
			state[i] |= done
			queue = append(queue, i)
		}
	}
	for _, i := range deps {
		if facts[i]&hasProofs != 0 {
			for _, p := range s.proofs.Out(int(i)) {
				if !in.has(int(p)) && facts[p]&finite != 0 {
					state[i] |= proved
				}
			}
		}
		ready(i)
	}
	// Each node that uses or that a proof proves a dependent is a dependent too, or x.
	for ; head < len(queue); head++ {
		i := queue[head]
		for _, p := range s.usedBy.Out(int(i)) {
			if int(p) != x {
				waiting[p]--
				ready(p)
			}
		}
		for _, st := range s.proves.Out(int(i)) {
			if int(st) != x {
				state[st] |= proved
				ready(st)
			}
		}
	}
	w.queue = queue
	out := newSet(len(s.uses.IDs))
	for _, i := range deps {
		if state[i]&done == 0 && facts[i]&(finite|isNode) == finite|isNode {
			out.set(int(i))
		}
	}
	return s.names(out)
}

// Dependents returns the nodes that use the node with the tag in at least one alternative: each node from which a chain of uses and proves edges goes to the node, against the direction of the proves edges. A proof uses what it uses, and a statement uses each of its proofs. The node itself is not in the result. The result is in byte order.
func (g *Graph) Dependents(tag string) []string {
	s := g.solve(MinDepth)
	x, ok := s.uses.Index(tag)
	if !ok || s.pos[x] < 0 {
		return nil
	}
	w := s.scratch()
	defer s.release(w)
	out := newSet(len(s.uses.IDs))
	for _, i := range s.dependents(x, w, false) {
		if s.pos[i] >= 0 {
			out.set(int(i))
		}
	}
	return s.names(out)
}

// dependents returns the ids that use the id x in at least one alternative. It adds these ids and x to w.in, and it keeps them in w for release. With count, it sets w.waiting of each id to the number of its uses in w.in: the search sees each uses edge into w.in once.
func (a *adjacency) dependents(x int, w *scratch, count bool) []int32 {
	in := w.in
	in.set(x)
	// The list of the ids is the queue of the search too.
	out := append(w.deps[:0], int32(x)) // #nosec G115 -- there are fewer than 2^31 ids.
	for k := 0; k < len(out); k++ {
		i := out[k]
		for _, p := range a.usedBy.Out(int(i)) {
			if count {
				w.waiting[p]++
			}
			if !in.has(int(p)) {
				in.set(int(p))
				out = append(out, p)
			}
		}
		for _, p := range a.proves.Out(int(i)) {
			if !in.has(int(p)) {
				in.set(int(p))
				out = append(out, p)
			}
		}
	}
	w.deps = out
	return out[1:]
}

// The bits of scratch.state.
const (
	proved = 1 << iota // a proof of the id that is not a dependent has a value, or one that is a dependent keeps its value
	done               // the id keeps its value
)

// scratch holds the arrays of one call of Needs or Dependents, with an entry for each id. The arrays are large, so a call takes them from a pool, and it clears only the entries that it set before it gives them back.
type scratch struct {
	in      idset   // x and its dependents
	waiting []int32 // the uses of each dependent that can lose their value and did not keep it yet
	state   []uint8 // the bits proved and done of each dependent
	deps    []int32 // x and its dependents, in the order of the search
	queue   []int32 // the ids that keep their value, for Needs
}

func (a *adjacency) scratch() *scratch {
	if w, ok := a.pool.Get().(*scratch); ok {
		return w
	}
	n := len(a.uses.IDs)
	return &scratch{in: newSet(n), waiting: make([]int32, n), state: make([]uint8, n)}
}

func (a *adjacency) release(w *scratch) {
	for _, i := range w.deps {
		w.in[i/64] = 0
		w.waiting[i], w.state[i] = 0, 0
	}
	w.deps, w.queue = w.deps[:0], w.queue[:0]
	a.pool.Put(w)
}

// Path returns the shortest learning path from the node from to the node to (query Q5 of spec 05.6). The path uses only the uses edges that end at a definition and the defines edges, and it goes against their direction: to uses a definition, which uses another definition, which defines from. The path starts with from and ends with to. Of two paths with the same length, Path gives the first in byte order. Path returns false when no such path exists.
func (g *Graph) Path(from, to string) ([]string, bool) {
	if _, ok := g.index(from); !ok {
		return nil, false
	}
	if _, ok := g.index(to); !ok {
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
			if _, ok := g.index(e.To); ok {
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
