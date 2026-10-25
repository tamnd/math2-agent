package graph

import (
	"container/heap"
	"math"
	"slices"
)

// Measure says which alternative Closure gives.
type Measure int

const (
	// MinDepth gives the alternative with the smallest longest chain of dependencies.
	MinDepth Measure = iota
	// MinSize gives an alternative with few distinct nodes. The problem is NP-hard, so this is an approximation: for each statement, Closure takes the proof whose tree of dependencies, with each shared node counted again, is the smallest.
	MinSize
)

// Alternative is one way to establish a node: one proof for each statement that needs one, and each node that the node and these proofs use (spec 05.5).
type Alternative struct {
	Tag    string   // the node
	Nodes  []string // each node of the closure, the node too, in byte order
	Proofs []string // the proofs that the alternative takes, in byte order
	Depth  int      // the length of the longest chain of dependencies, in edges
}

const infinite = math.MaxInt

// Closure returns the best alternative for the node with the tag, by the measure m. The closure is an AND-OR graph: a node needs each node in its uses, and a statement with proofs needs one of its proofs. A node with neither is a leaf. Closure returns false when the tag is not in the graph, or when each alternative goes around a cycle. An edge to a tag that is not in the graph ends at a leaf.
func (g *Graph) Closure(tag string, m Measure) (Alternative, bool) {
	s := g.solve(m)
	i, ok := s.uses.Index(tag)
	if _, isNode := g.index[tag]; !ok || !isNode || s.value[i] == infinite {
		return Alternative{}, false
	}
	alt := Alternative{Tag: tag}
	seen := make([]bool, len(s.uses.IDs))
	var visit func(i int) int
	visit = func(i int) int {
		if seen[i] {
			return s.depth[i]
		}
		seen[i] = true
		alt.Nodes = append(alt.Nodes, s.uses.IDs[i])
		for _, j := range s.uses.Out(i) {
			visit(int(j))
		}
		if p := s.choice[i]; p >= 0 {
			alt.Proofs = append(alt.Proofs, s.uses.IDs[p])
			visit(int(p))
		}
		return s.depth[i]
	}
	alt.Depth = visit(i)
	slices.Sort(alt.Nodes)
	slices.Sort(alt.Proofs)
	return alt, true
}

// solution is the best alternative of each id for one measure.
type solution struct {
	uses   *CSR    // the uses edges
	proofs *CSR    // from each statement to its proofs, over the same IDs
	value  []int   // the measure of the best alternative, or infinite
	depth  []int   // the depth of the best alternative, or infinite
	choice []int32 // the proof that the best alternative takes, or -1
}

func (g *Graph) solve(m Measure) *solution {
	g.once[m].Do(func() { g.solved[m] = g.compute(m) })
	return g.solved[m]
}

// compute uses the algorithm of Knuth (1977) for superior functions, a form of the algorithm of Dijkstra for AND-OR graphs. An id gets its value when each of its uses has a value and one of its proofs has a value. Ids leave the queue in order of value, so the first proof to get a value is the best proof.
func (g *Graph) compute(m Measure) *solution {
	var us, ps []Edge
	for _, e := range g.Edges {
		switch e.Type {
		case Uses:
			us = append(us, e)
		case Proves:
			ps = append(ps, e)
		}
	}
	// Both adjacencies must use the same ids.
	all := append(slices.Clip(us), ps...)
	ids := g.csr(all, false).IDs
	sub := &Graph{Nodes: make([]Node, len(ids))}
	for i, id := range ids {
		sub.Nodes[i].Tag = id
	}
	s := &solution{uses: sub.csr(us, false), proofs: sub.csr(ps, true)}
	n := len(ids)
	usedBy, proves := sub.csr(us, true), sub.csr(ps, false)

	s.value, s.depth, s.choice = make([]int, n), make([]int, n), make([]int32, n)
	waiting := make([]int, n) // the uses that have no value yet
	acc, deep := make([]int, n), make([]int, n)
	proved := make([]bool, n)
	q := &queue{}
	for i := range n {
		s.value[i], s.depth[i], s.choice[i] = infinite, infinite, -1
		waiting[i] = len(s.uses.Out(i))
		if waiting[i] == 0 && len(s.proofs.Out(i)) == 0 {
			heap.Push(q, item{i, leaf(m), 0})
		}
	}
	ready := func(i int) {
		if waiting[i] == 0 && (proved[i] || len(s.proofs.Out(i)) == 0) {
			v := acc[i]
			if m == MinDepth {
				v = deep[i]
			}
			heap.Push(q, item{i, add(v, 1), add(deep[i], 1)})
		}
	}
	for q.Len() > 0 {
		it := heap.Pop(q).(item)
		if s.value[it.id] != infinite {
			continue
		}
		s.value[it.id], s.depth[it.id] = it.value, it.depth
		for _, p := range usedBy.Out(it.id) {
			waiting[p]--
			acc[p] = add(acc[p], it.value)
			deep[p] = max(deep[p], it.depth)
			ready(int(p))
		}
		for _, st := range proves.Out(it.id) {
			if !proved[st] {
				proved[st], s.choice[st] = true, int32(it.id) // #nosec G115 -- there are fewer than 2^31 ids.
				acc[st] = add(acc[st], it.value)
				deep[st] = max(deep[st], it.depth)
				ready(int(st))
			}
		}
	}
	return s
}

// leaf returns the value of a leaf by the measure m.
func leaf(m Measure) int {
	if m == MinSize {
		return 1
	}
	return 0
}

// add adds two values and stops at infinite.
func add(a, b int) int {
	if a > infinite-b {
		return infinite
	}
	return a + b
}

type item struct{ id, value, depth int }

// queue is a heap of items in order of value, then of id.
type queue []item

func (q queue) Len() int { return len(q) }
func (q queue) Less(i, j int) bool {
	if q[i].value != q[j].value {
		return q[i].value < q[j].value
	}
	return q[i].id < q[j].id
}
func (q queue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *queue) Push(x any)   { *q = append(*q, x.(item)) }
func (q *queue) Pop() any {
	old := *q
	it := old[len(old)-1]
	*q = old[:len(old)-1]
	return it
}
