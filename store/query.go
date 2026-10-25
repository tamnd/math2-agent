package store

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/status"
)

// Closure is the best alternative of a node with its trust profile and its axiom profile (query Q3 of spec 05.5 and 05.6).
type Closure struct {
	graph.Alternative
	Evidence   status.Level `json:"evidence"`   // the evidence level of the node
	Weakest    status.Level `json:"weakest"`    // the weakest evidence level of a node of the alternative that has a level
	Below      []string     `json:"below"`      // the nodes of the alternative with a lower evidence level than the node, in byte order
	Axioms     []string     `json:"axioms"`     // the Lean axioms of the bindings of the nodes of the alternative, in byte order
	Hypotheses []string     `json:"hypotheses"` // the conjectures and the problems that the alternative uses, in byte order
}

// Closure returns the best alternative for the node with the tag, by the measure m.
func (s *Store) Closure(tag string, m graph.Measure) (Closure, error) {
	if _, err := s.node(tag); err != nil {
		return Closure{}, err
	}
	alt, ok := s.graph.Closure(tag, m)
	if !ok {
		return Closure{}, fmt.Errorf("%s has no alternative: each alternative goes around a cycle", tag)
	}
	c := Closure{Alternative: alt, Evidence: s.evidence(tag), Below: []string{}, Axioms: []string{}, Hypotheses: []string{}}
	// Many nodes have the same axioms, so the loop marks each set of axioms once.
	sets := make([]bool, len(s.sets))
	for k, i := range alt.Index {
		if i < 0 {
			continue
		}
		t, ev := alt.Nodes[k], s.level[i]
		if ev != status.None && (c.Weakest == status.None || ev < c.Weakest) {
			c.Weakest = ev
		}
		if t != tag && ev != status.None && ev < c.Evidence {
			c.Below = append(c.Below, t)
		}
		sets[s.axioms[i]] = true
		if t != tag && s.hyp[i] {
			c.Hypotheses = append(c.Hypotheses, t)
		}
	}
	for k, ok := range sets {
		if ok {
			c.Axioms = append(c.Axioms, s.sets[k]...)
		}
	}
	slices.Sort(c.Axioms)
	c.Axioms = slices.Compact(c.Axioms)
	return c, nil
}

// Dependents are the reverse dependencies of a node (query Q4).
type Dependents struct {
	Tag   string   `json:"tag"`
	Needs []string `json:"needs"` // the nodes that lose each alternative when the node is false, in byte order
	Uses  []string `json:"uses"`  // the nodes that use the node in at least one alternative, in byte order
}

// Dependents returns the reverse dependencies of the node with the tag.
func (s *Store) Dependents(tag string) (Dependents, error) {
	if _, err := s.node(tag); err != nil {
		return Dependents{}, err
	}
	d := Dependents{Tag: tag}
	var wg sync.WaitGroup
	wg.Go(func() { d.Needs = s.graph.Needs(tag) })
	wg.Go(func() { d.Uses = s.graph.Dependents(tag) })
	wg.Wait()
	if d.Needs == nil {
		d.Needs = []string{}
	}
	if d.Uses == nil {
		d.Uses = []string{}
	}
	return d, nil
}

// Path returns the shortest learning path from the node from to the node to (query Q5). See [graph.Graph.Path].
func (s *Store) Path(from, to string) ([]string, error) {
	for _, t := range []string{from, to} {
		if _, err := s.node(t); err != nil {
			return nil, err
		}
	}
	p, ok := s.graph.Path(from, to)
	if !ok {
		return nil, fmt.Errorf("no learning path goes from %s to %s through the uses edges into definitions and the defines edges", from, to)
	}
	return p, nil
}

// Filter selects the nodes of a frontier.
type Filter struct {
	Open     bool         // the open-problem frontier (query Q7). Without Open, the frontier of formal statements with no proof (query Q6).
	MSC      string       // an MSC code or the start of one, for example 11 or 11A41. Empty means each code.
	Topic    string       // the tag of a topic: only the nodes with a part_of edge to it. Empty means each topic.
	Evidence status.Level // only the nodes at this evidence level. None means each level.
}

// Entry is one node of a frontier.
type Entry struct {
	Tag      string          `json:"tag"`
	Kind     string          `json:"kind"`
	Name     string          `json:"name"`
	Evidence status.Level    `json:"evidence"`
	Fidelity status.Fidelity `json:"fidelity"`
	MSC      []string        `json:"msc"`
	Bounds   []Bound         `json:"bounds,omitempty"` // for an open problem, the statements that bound it
}

// Bound is a statement that bounds an open problem.
type Bound struct {
	Tag      string       `json:"tag"`      // the statement
	Side     string       `json:"side"`     // lower or upper
	Value    string       `json:"value"`    // the bound
	Via      string       `json:"via"`      // the proof of the bound, or ""
	Evidence status.Level `json:"evidence"` // the evidence level of the statement
}

// Frontier returns the nodes of a frontier, in tag order (queries Q6 and Q7).
//
// The frontier of formal statements has each statement that is not retired, that has a formal statement at fidelity F1 or above, and that has an evidence level below E5. Its proof is open to formalise. The open-problem frontier has each conjecture and each problem at E0, with the statements that bound it. The dates that the bounds moved come from the provenance of the nodes, in a later milestone.
func (s *Store) Frontier(f Filter) ([]Entry, error) {
	// The conditions are on the columns of a table that the letter X stands for.
	var where []string
	var args []any
	if f.Open {
		where = append(where, `X.kind IN ('conjecture', 'problem')`, `X.evidence = ?`)
		args = append(args, status.Open.String())
	} else {
		where = append(where, `X.kind = 'statement'`, `X.fidelity IN ('F1', 'F2', 'F3')`, `X.evidence <> ?`)
		args = append(args, status.Kernel.String())
	}
	where = append(where, `X.retired = 0`)
	if f.Evidence != status.None {
		where = append(where, `X.evidence = ?`)
		args = append(args, f.Evidence.String())
	}
	cond := strings.Join(where, " AND ")
	// The query starts from the MSC codes or from the topic, when the filter has one, so it reads only the nodes of the slice. The MSC table has the columns of the conditions.
	from, fromArgs := `nodes n`, []any{}
	switch {
	case f.MSC != "":
		from = `(SELECT DISTINCT tag FROM msc WHERE code >= ? AND code < ? AND ` + strings.ReplaceAll(cond, "X.", "") + `) m CROSS JOIN nodes n ON n.tag = m.tag`
		fromArgs = append(fromArgs, f.MSC, f.MSC+"\x7f")
		fromArgs = append(fromArgs, args...)
	case f.Topic != "":
		from = `(SELECT DISTINCT src AS tag FROM edges WHERE dst = ? AND type = ?) m CROSS JOIN nodes n ON n.tag = m.tag`
		fromArgs = append(fromArgs, f.Topic, graph.PartOf)
	}
	cond = strings.ReplaceAll(cond, "X.", "n.")
	args = append(fromArgs, args...)
	if f.MSC != "" && f.Topic != "" {
		cond += ` AND EXISTS (SELECT 1 FROM edges e WHERE e.src = n.tag AND e.type = ? AND e.dst = ?)`
		args = append(args, graph.PartOf, f.Topic)
	}
	query := `SELECT n.tag, n.kind, n.name, n.evidence, n.fidelity, (SELECT group_concat(code, ' ') FROM (SELECT code FROM msc c WHERE c.tag = n.tag ORDER BY code)) FROM ` + from + ` WHERE ` + cond + ` ORDER BY n.tag`
	out := []Entry{}
	err := rows(s.db, query, func(r *sql.Rows) error {
		var e Entry
		var evidence, fidelity []byte
		var codes sql.NullString
		if err := r.Scan(&e.Tag, &e.Kind, &e.Name, &evidence, &fidelity, &codes); err != nil {
			return err
		}
		if err := errors.Join(e.Evidence.UnmarshalText(evidence), e.Fidelity.UnmarshalText(fidelity)); err != nil {
			return err
		}
		e.MSC = strings.Fields(codes.String)
		if e.MSC == nil {
			e.MSC = []string{}
		}
		out = append(out, e)
		return nil
	}, args...)
	if err != nil || !f.Open {
		return out, err
	}
	// The bounds of many problems come in one query.
	at := make(map[string]int, len(out))
	for i, e := range out {
		at[e.Tag] = i
	}
	for chunk := range slices.Chunk(out, 500) {
		args := []any{graph.Bounds}
		for _, e := range chunk {
			args = append(args, e.Tag)
		}
		query := `SELECT dst, src, via, label FROM edges WHERE type = ? AND dst IN (` + strings.Repeat(", ?", len(chunk))[2:] + `) ORDER BY dst, src, via, system, label`
		err := rows(s.db, query, func(r *sql.Rows) error {
			var dst, label string
			var b Bound
			if err := r.Scan(&dst, &b.Tag, &b.Via, &label); err != nil {
				return err
			}
			b.Side, b.Value, _ = strings.Cut(label, " ")
			b.Evidence = s.evidence(b.Tag)
			out[at[dst]].Bounds = append(out[at[dst]].Bounds, b)
			return nil
		}, args...)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Neighbourhood is the part of the graph near a node (query Q2).
type Neighbourhood struct {
	Tag   string       `json:"tag"`
	Hops  int          `json:"hops"`
	Nodes []string     `json:"nodes"` // the nodes at most Hops edges from the node, the node too, in byte order
	Edges []graph.Edge `json:"edges"` // each edge of each type with an end less than Hops edges from the node, in the order of [graph.Edge.Compare]
}

// Neighbourhood returns the nodes and the edges near the node with the tag (query Q2). An edge goes either way. The ends of the edges that are not nodes, such as Lean declarations and external ids, are in the edges, but a path does not go through them.
func (s *Store) Neighbourhood(tag string, hops int) (Neighbourhood, error) {
	if _, err := s.node(tag); err != nil {
		return Neighbourhood{}, err
	}
	nb := Neighbourhood{Tag: tag, Hops: hops, Nodes: []string{tag}, Edges: []graph.Edge{}}
	seen := map[string]bool{tag: true}
	found := map[graph.Edge]bool{}
	near := []string{tag}
	for range hops {
		var next []string
		for chunk := range slices.Chunk(near, 500) {
			marks := strings.Repeat(", ?", len(chunk))[2:]
			args := make([]any, 0, 2*len(chunk))
			for _, t := range chunk {
				args = append(args, t)
			}
			args = append(args, args...)
			query := `SELECT type, src, dst, via, system, label FROM edges WHERE src IN (` + marks + `) UNION ALL SELECT type, src, dst, via, system, label FROM edges WHERE dst IN (` + marks + `)`
			err := rows(s.db, query, func(r *sql.Rows) error {
				var e graph.Edge
				if err := r.Scan(&e.Type, &e.From, &e.To, &e.Via, &e.System, &e.Label); err != nil {
					return err
				}
				if found[e] {
					return nil
				}
				found[e] = true
				nb.Edges = append(nb.Edges, e)
				for _, t := range []string{e.From, e.To} {
					if _, isNode := s.at(t); isNode && !seen[t] {
						seen[t] = true
						next = append(next, t)
						nb.Nodes = append(nb.Nodes, t)
					}
				}
				return nil
			}, args...)
			if err != nil {
				return Neighbourhood{}, err
			}
		}
		near = next
	}
	slices.Sort(nb.Nodes)
	slices.SortFunc(nb.Edges, graph.Edge.Compare)
	return nb, nil
}
