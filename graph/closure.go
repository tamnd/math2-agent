package graph

import (
	"container/heap"
	"math"
	"math/bits"
	"sync"
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
	// Index holds the position in the Nodes of the graph of each tag in Nodes, or -1 for a tag that is not a node of the graph. A caller that keeps data in a slice in the order of the Nodes of the graph reads it with no search.
	Index []int `json:"-"`
}

const infinite = math.MaxInt

// Closure returns the best alternative for the node with the tag, by the measure m. The closure is an AND-OR graph: a node needs each node in its uses, and a statement with proofs needs one of its proofs. A node with neither is a leaf. Closure returns false when the tag is not in the graph, or when each alternative goes around a cycle. An edge to a tag that is not in the graph ends at a leaf.
func (g *Graph) Closure(tag string, m Measure) (Alternative, bool) {
	s := g.solve(m)
	i, ok := s.uses.Index(tag)
	if !ok || s.pos[i] < 0 || s.value[i] == infinite {
		return Alternative{}, false
	}
	// The closure can hold a large part of the graph, so the sets are bit sets, and the ids come out of them in order with no sort.
	nodes, proofs := newSet(len(s.uses.IDs)), newSet(len(s.uses.IDs))
	stack := []int32{int32(i)} // #nosec G115 -- there are fewer than 2^31 ids.
	nodes.set(i)
	push := func(j int32) {
		if !nodes.has(int(j)) {
			nodes.set(int(j))
			stack = append(stack, j)
		}
	}
	for len(stack) > 0 {
		j := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, k := range s.uses.Out(int(j)) {
			push(k)
		}
		if p := s.choice[j]; p >= 0 {
			proofs.set(int(p))
			push(p)
		}
	}
	return Alternative{Tag: tag, Nodes: s.names(nodes), Proofs: s.names(proofs), Depth: s.depth[i], Index: s.positions(nodes)}, true
}

// idset is a set of ids.
type idset []uint64

func newSet(n int) idset { return make(idset, (n+63)/64) }

func (b idset) set(i int)      { b[i/64] |= 1 << (i % 64) }
func (b idset) has(i int) bool { return b[i/64]&(1<<(i%64)) != 0 }

func (b idset) count() int {
	n := 0
	for _, w := range b {
		n += bits.OnesCount64(w)
	}
	return n
}

// adjacency is the uses and the proves edges over one set of ids, in both directions.
type adjacency struct {
	pos    []int32   // the position in the Nodes of the graph of each id, or -1 when the id is not the tag of a node
	uses   *CSR      // the uses edges
	proofs *CSR      // from each statement to its proofs
	usedBy *CSR      // the uses edges against their direction
	proves *CSR      // from each proof to its statements
	pool   sync.Pool // of *scratch
}

func (g *Graph) adjacency() *adjacency {
	g.adjOnce.Do(func() {
		ids, pairs := g.ends(g.typed(Uses), g.typed(Proves))
		pos := make([]int32, len(ids))
		for i, k := 0, 0; i < len(ids); i++ {
			pos[i] = -1
			if k < len(g.Nodes) && ids[i] == g.Nodes[k].Tag {
				pos[i] = int32(k) // #nosec G115 -- there are fewer than 2^31 nodes.
				k++
			}
		}
		g.adj = &adjacency{
			pos:    pos,
			uses:   fromPairs(ids, pairs[0], false),
			usedBy: fromPairs(ids, pairs[0], true),
			proofs: fromPairs(ids, pairs[1], true),
			proves: fromPairs(ids, pairs[1], false),
		}
	})
	return g.adj
}

// names returns the names of the ids in the set, in byte order.
func (a *adjacency) names(set idset) []string {
	out := make([]string, 0, set.count())
	for k, w := range set {
		for w != 0 {
			out = append(out, a.uses.IDs[k*64+bits.TrailingZeros64(w)])
			w &= w - 1
		}
	}
	return out
}

// positions returns the position in the Nodes of the graph of each id in the set, or -1, in the order of the ids.
func (a *adjacency) positions(set idset) []int {
	out := make([]int, 0, set.count())
	for k, w := range set {
		for w != 0 {
			out = append(out, int(a.pos[k*64+bits.TrailingZeros64(w)]))
			w &= w - 1
		}
	}
	return out
}

// solution is the best alternative of each id for one measure.
type solution struct {
	*adjacency
	value  []int   // the measure of the best alternative, or infinite
	depth  []int   // the depth of the best alternative, or infinite
	choice []int32 // the proof that the best alternative takes, or -1
	facts  []uint8 // the bits isNode, hasProofs and finite of each id, in one byte for the queries that read them often
}

// The bits of solution.facts.
const (
	isNode    = 1 << iota // the id is the tag of a node
	hasProofs             // a proof proves the id
	finite                // the id has an alternative
)

func (g *Graph) solve(m Measure) *solution {
	g.once[m].Do(func() { g.solved[m] = g.compute(m) })
	return g.solved[m]
}

// compute uses the algorithm of Knuth (1977) for superior functions, a form of the algorithm of Dijkstra for AND-OR graphs. An id gets its value when each of its uses has a value and one of its proofs has a value. Ids leave the queue in order of value, so the first proof to get a value is the best proof.
func (g *Graph) compute(m Measure) *solution {
	s := &solution{adjacency: g.adjacency()}
	n := len(s.uses.IDs)
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
		for _, p := range s.usedBy.Out(it.id) {
			waiting[p]--
			acc[p] = add(acc[p], it.value)
			deep[p] = max(deep[p], it.depth)
			ready(int(p))
		}
		for _, st := range s.proves.Out(it.id) {
			if !proved[st] {
				proved[st], s.choice[st] = true, int32(it.id) // #nosec G115 -- there are fewer than 2^31 ids.
				acc[st] = add(acc[st], it.value)
				deep[st] = max(deep[st], it.depth)
				ready(int(st))
			}
		}
	}
	s.facts = make([]uint8, n)
	for i := range n {
		if s.pos[i] >= 0 {
			s.facts[i] |= isNode
		}
		if len(s.proofs.Out(i)) > 0 {
			s.facts[i] |= hasProofs
		}
		if s.value[i] != infinite {
			s.facts[i] |= finite
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
