package graph

import "slices"

// Cycle is a cycle of dependencies that rule G03 does not allow. Each node in Nodes has an edge of the type to the next node, and the last node has an edge to the first. The first node is the smallest in byte order.
type Cycle struct {
	Type  string   // Uses or Specializes
	Nodes []string // the nodes of the cycle, in order
}

// Cycles returns the cycles of dependencies that rule G03 does not allow (spec 05.8). The specializes edges must have no cycle. In the uses edges, a statement goes to each of its proofs. A cycle through two different proofs is allowed, because an equivalence of two statements makes such a cycle. A cycle through one proof or through no proof is not allowed. Cycles gives one cycle for each group of nodes that are on such cycles, in byte order of the first node.
func (g *Graph) Cycles() []Cycle {
	var out []Cycle
	spec := g.CSR(Specializes)
	for _, comp := range components(spec, nil) {
		out = append(out, Cycle{Type: Specializes, Nodes: names(spec, cycle(spec, comp))})
	}

	s := g.solve(MinDepth)
	ids := s.uses.IDs
	isProof := make([]bool, len(ids))
	for i := range ids {
		for _, p := range s.proofs.Out(i) {
			isProof[p] = true
		}
	}
	// deps goes from a node to its uses, and from a statement to its proofs.
	deps := &CSR{IDs: ids, Offsets: make([]int32, len(ids)+1)}
	for i := range ids {
		out := append(slices.Clone(s.uses.Out(i)), s.proofs.Out(i)...)
		slices.Sort(out)
		deps.Targets = append(deps.Targets, slices.Compact(out)...)
		deps.Offsets[i+1] = int32(len(deps.Targets)) // #nosec G115 -- there are fewer than 2^31 edges.
	}
	for _, comp := range components(deps, nil) {
		var proofs []int
		for _, i := range comp {
			if isProof[i] {
				proofs = append(proofs, i)
			}
		}
		// Look for a cycle through no proof, then for a cycle through each proof and no other proof.
		in := make([]bool, len(ids))
		for _, i := range comp {
			in[i] = !isProof[i]
		}
		c := badCycle(deps, in)
		for k := 0; c == nil && k < len(proofs); k++ {
			in[proofs[k]] = true
			c = badCycle(deps, in)
			in[proofs[k]] = false
		}
		if c != nil {
			out = append(out, Cycle{Type: Uses, Nodes: names(deps, c)})
		}
	}
	slices.SortFunc(out, func(a, b Cycle) int { return slices.Compare(a.Nodes, b.Nodes) })
	return out
}

// badCycle returns a cycle in the ids that keep marks, or nil. Keep marks only ids of one component.
func badCycle(c *CSR, keep []bool) []int {
	if subs := components(c, func(i int) bool { return keep[i] }); len(subs) > 0 {
		return cycle(c, subs[0])
	}
	return nil
}

func names(c *CSR, path []int) []string {
	out := make([]string, len(path))
	for i, p := range path {
		out[i] = c.IDs[p]
	}
	return out
}

// components returns the strongly connected components of c that have a cycle, with the algorithm of Tarjan. When keep is not nil, it uses only the ids that keep accepts. Each component is in byte order, and the components are in order of their first id.
func components(c *CSR, keep func(int) bool) [][]int {
	n := len(c.IDs)
	index, low := make([]int, n), make([]int, n)
	onStack := make([]bool, n)
	var stack []int
	var out [][]int
	next := 1
	var visit func(v int)
	visit = func(v int) {
		index[v], low[v] = next, next
		next++
		stack = append(stack, v)
		onStack[v] = true
		loop := false
		for _, w := range c.Out(v) {
			w := int(w)
			if keep != nil && !keep(w) {
				continue
			}
			loop = loop || w == v
			if index[w] == 0 {
				visit(w)
				low[v] = min(low[v], low[w])
			} else if onStack[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] != index[v] {
			return
		}
		var comp []int
		for {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[w] = false
			comp = append(comp, w)
			if w == v {
				break
			}
		}
		if len(comp) > 1 || loop {
			slices.Sort(comp)
			out = append(out, comp)
		}
	}
	for v := range n {
		if index[v] == 0 && (keep == nil || keep(v)) {
			visit(v)
		}
	}
	slices.SortFunc(out, func(a, b []int) int { return a[0] - b[0] })
	return out
}

// cycle returns a shortest cycle through the first id of the component comp. The cycle uses only the ids of comp.
func cycle(c *CSR, comp []int) []int {
	start := comp[0]
	prev := map[int]int{}
	frontier := []int{start}
	for len(frontier) > 0 {
		var next []int
		for _, v := range frontier {
			for _, w := range c.Out(v) {
				w := int(w)
				if _, ok := slices.BinarySearch(comp, w); !ok {
					continue
				}
				if w == start {
					path := []int{v}
					for path[0] != start {
						path = append([]int{prev[path[0]]}, path...)
					}
					return path
				}
				if _, seen := prev[w]; !seen {
					prev[w] = v
					next = append(next, w)
				}
			}
		}
		frontier = next
	}
	return []int{start}
}
