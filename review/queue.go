package review

import (
	"cmp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
)

// Task is a Lean binding in the fidelity queue of spec 16.3: a binding that waits for a review.
type Task struct {
	Tag        string          `json:"tag"`
	Kind       string          `json:"kind"`
	Name       string          `json:"name"`
	Decl       string          `json:"decl"` // the declaration that the kernel checks and that the review reads: the adapter, or the declaration
	Fidelity   status.Fidelity `json:"fidelity"`
	MSC        []string        `json:"msc"`        // the MSC2020 codes of the node
	Dependents int             `json:"dependents"` // the number of nodes that use the node, directly or through other nodes
}

// Filter selects the tasks of Queue.
type Filter struct {
	// Reviewer leaves out the bindings that the reviewer wrote, and the bindings with a review by the reviewer that is not stale. Empty means each binding.
	Reviewer string

	// Area keeps the nodes with an MSC2020 code that starts with the area, for example 20 or 20D. A suffix of x, X or - is not part of the area, thus 20-XX is the same as 20. Empty means each node.
	Area string

	// Second selects the bindings at F2, which wait for a second review for F3. False selects the bindings at F1, which wait for a first review.
	Second bool
}

// Queue returns the bindings of the build that wait for a review and that pass the filter. A binding waits for a review when it is at F1, or at F2 for a second review, and it has no not-faithful review. The node must not be retired, and the build must have the type hash of the declaration.
//
// The first tasks are the bindings of the nodes with the most dependents, because a review of a node that many nodes use unlocks more of the graph (spec 11.4). Tasks with the same number of dependents are in the order of their tags, then of their declarations.
func Queue(in Input, f Filter) []Task {
	want := status.F1
	if f.Second {
		want = status.F2
	}
	area := strings.TrimRight(f.Area, "xX-")
	nodes := map[string]*node.Node{}
	for _, n := range in.Nodes {
		nodes[n.ID] = n
	}
	var out []Task
	for _, s := range in.Status {
		n := nodes[s.Tag]
		if s.Retired || n == nil {
			continue
		}
		msc := n.List("msc")
		if area != "" && !slices.ContainsFunc(msc, func(c string) bool { return strings.HasPrefix(c, area) }) {
			continue
		}
		for _, b := range s.Bindings {
			if b.Fidelity != want || b.TypeHash == "" || !waits(in, s.Tag, b, f.Reviewer) {
				continue
			}
			out = append(out, Task{Tag: s.Tag, Kind: s.Kind, Name: s.Name, Decl: b.Checked, Fidelity: b.Fidelity, MSC: slices.Clone(msc)})
		}
	}
	if len(out) == 0 {
		return nil
	}
	g := graph.Build(in.Nodes)
	deps := map[string]int{}
	for i, t := range out {
		d, ok := deps[t.Tag]
		if !ok {
			d = len(g.Dependents(t.Tag))
			deps[t.Tag] = d
		}
		out[i].Dependents = d
	}
	slices.SortFunc(out, func(a, b Task) int {
		return cmp.Or(cmp.Compare(b.Dependents, a.Dependents), strings.Compare(a.Tag, b.Tag), strings.Compare(a.Decl, b.Decl))
	})
	return out
}

// waits tells if the binding b of the node with the tag waits for a review by the reviewer.
func waits(in Input, tag string, b status.Binding, reviewer string) bool {
	for _, r := range b.Reviews {
		if r.State == status.NotFaithfulRv || (reviewer != "" && r.Reviewer == reviewer && r.State != status.Stale) {
			return false
		}
	}
	return reviewer == "" || in.Authors[status.Key{Tag: tag, Decl: b.Decl}] != reviewer
}
