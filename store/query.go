package store

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"

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
	c := Closure{Alternative: alt, Evidence: s.status[tag].Evidence, Below: []string{}, Axioms: []string{}, Hypotheses: []string{}}
	for _, t := range alt.Nodes {
		st := s.status[t]
		if st.Evidence != status.None && (c.Weakest == status.None || st.Evidence < c.Weakest) {
			c.Weakest = st.Evidence
		}
		if t != tag && st.Evidence != status.None && st.Evidence < c.Evidence {
			c.Below = append(c.Below, t)
		}
		for _, b := range st.Bindings {
			c.Axioms = append(c.Axioms, b.Axioms...)
		}
		if n, _ := s.graph.Node(t); t != tag && (n.Kind == "conjecture" || n.Kind == "problem") {
			c.Hypotheses = append(c.Hypotheses, t)
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
	d := Dependents{Tag: tag, Needs: s.graph.Needs(tag), Uses: s.graph.Dependents(tag)}
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
	var where []string
	var args []any
	if f.Open {
		where = append(where, `n.kind IN ('conjecture', 'problem')`, `n.evidence = ?`)
		args = append(args, status.Open.String())
	} else {
		where = append(where, `n.kind = 'statement'`, `n.fidelity IN ('F1', 'F2', 'F3')`, `n.evidence <> ?`)
		args = append(args, status.Kernel.String())
	}
	where = append(where, `n.retired = 0`)
	if f.MSC != "" {
		where = append(where, `EXISTS (SELECT 1 FROM msc m WHERE m.tag = n.tag AND substr(m.code, 1, length(?)) = ?)`)
		args = append(args, f.MSC, f.MSC)
	}
	if f.Topic != "" {
		where = append(where, `EXISTS (SELECT 1 FROM edges e WHERE e.type = ? AND e.src = n.tag AND e.dst = ?)`)
		args = append(args, graph.PartOf, f.Topic)
	}
	if f.Evidence != status.None {
		where = append(where, `n.evidence = ?`)
		args = append(args, f.Evidence.String())
	}
	query := `SELECT n.tag, n.kind, n.name, (SELECT group_concat(code, ' ') FROM (SELECT code FROM msc m WHERE m.tag = n.tag ORDER BY code)) FROM nodes n WHERE ` + strings.Join(where, " AND ") + ` ORDER BY n.tag`
	out := []Entry{}
	err := rows(s.db, query, func(r *sql.Rows) error {
		var e Entry
		var codes sql.NullString
		if err := r.Scan(&e.Tag, &e.Kind, &e.Name, &codes); err != nil {
			return err
		}
		st := s.status[e.Tag]
		e.Evidence, e.Fidelity, e.MSC = st.Evidence, st.Fidelity, strings.Fields(codes.String)
		if e.MSC == nil {
			e.MSC = []string{}
		}
		out = append(out, e)
		return nil
	}, args...)
	if err != nil {
		return nil, err
	}
	if f.Open {
		for i := range out {
			for _, b := range s.graph.In(graph.Bounds, out[i].Tag) {
				side, value, _ := strings.Cut(b.Label, " ")
				out[i].Bounds = append(out[i].Bounds, Bound{Tag: b.From, Side: side, Value: value, Via: b.Via, Evidence: s.status[b.From].Evidence})
			}
		}
	}
	return out, nil
}
