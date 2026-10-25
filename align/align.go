// Package align proposes the crosswalk edges between the nodes of math2 and the records of the external sources, and records the decisions of people on them (spec 05.3, 05.4 and 16.3).
//
// The aligner reads the records that m2 ingest wrote to build/records. For each live node, it finds the records that can be the same thing by four channels: an id that the node and the record share through a third system, the names, the formulas, and the vectors of an Embedder. It gives each candidate a score from 0 to 1 and proposes an edge:
//
//   - same_as, when the best candidate has a score at the threshold or above, and the next candidate is lower by the margin or more;
//   - close_to, for the other candidates with a score at the close threshold or above;
//   - aligned, for each paper of arXiv or zbMATH Open with a score at the threshold or above;
//   - duplicate, for two nodes that match one record, or a node that matches a record that a second node has in same_as already.
//
// The aligner never writes an edge into a node. It writes the proposals into the queue, one file for each system under queues/align and the duplicates in queues/dedup.jsonl. A person accepts or rejects each proposal. Accept writes the edge into the node, and both Accept and Reject add a line to registers/crosswalk.reg. A rejected edge is not proposed again.
//
// The default Embedder, Hash, runs on the local machine. The aligner sends no text to a service.
package align

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/node"
)

// Options are the settings of Propose. The zero value of a number means its default.
type Options struct {
	Threshold float64  // the lowest score of a same_as or aligned proposal. The default is 0.9. The evaluation in bench/align sets it.
	Close     float64  // the lowest score of a close_to proposal. The default is 0.75.
	Margin    float64  // how much lower the second candidate must be for a same_as proposal. The default is 0.05.
	K         int      // the number of candidates for each node. The default is 5.
	Embedder  Embedder // nil means Hash
	Date      string   // the date of the run, YYYY-MM-DD
	By        string   // the program that proposes, for example m2/0.2.0 align
}

func (o Options) defaults() Options {
	o.Threshold = cmp.Or(o.Threshold, 0.9)
	o.Close = cmp.Or(o.Close, 0.75)
	o.Margin = cmp.Or(o.Margin, 0.05)
	o.K = cmp.Or(o.K, 5)
	if o.Embedder == nil {
		o.Embedder = Hash{}
	}
	return o
}

// A Result is the output of one run of Propose on one source.
type Result struct {
	Source     string
	System     string
	Records    string     // the hash of the records file
	Nodes      int        // the number of live nodes that the run read
	Proposals  []Proposal // the same_as, close_to and aligned proposals, for the queue of the system
	Duplicates []Proposal // the duplicate proposals, for the dedup queue
}

// Propose reads the nodes and the records of the source in the working tree at root, and returns the proposals of the edges between them. It does not write them. When the tree has the records of Wikidata, Propose uses them as links between the systems.
//
// Propose leaves out an edge that the node has already, an edge that a person decided on, and a same_as proposal in a system where the node has a same_as value or a null.
func Propose(root, source string, opt Options) (*Result, error) {
	opt = opt.defaults()
	recs, hash, err := ingest.ReadRecords(root, source)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("align: the tree has no records of %s. Run m2 ingest %s first", source, source)
	}
	if err != nil {
		return nil, err
	}
	x, err := NewIndex(source, recs, opt.Embedder)
	if err != nil {
		return nil, err
	}
	if source != "wikidata" {
		wd, _, err := ingest.ReadRecords(root, "wikidata")
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		x.Bridge(wd)
	}
	nodes, err := node.Load(root)
	if err != nil {
		return nil, err
	}
	decisions, err := ReadDecisions(root)
	if err != nil {
		return nil, err
	}
	decided := map[[3]string]bool{}
	for _, d := range decisions {
		decided[[3]string{d.Tag, d.System, d.ID}] = true
	}

	g := graph.Build(nodes)
	res := &Result{Source: source, System: x.system, Records: hash}
	edge := Edge(source)
	claimed := map[string]string{} // the node of each id in same_as
	var live []*node.Node
	for _, n := range nodes {
		if _, retired := n.Meta["retired"]; retired {
			continue
		}
		live = append(live, n)
		if v := SameAsOf(n)[x.system]; v != "" {
			claimed[v] = n.ID
		}
	}
	res.Nodes = len(live)

	propose := func(n *node.Node, edge, target string, c Candidate) {
		res.Proposals = append(res.Proposals, Proposal{
			ID: proposalID(n.ID, edge, x.system, target), Node: n.ID, Edge: edge, System: x.system, Target: target,
			Source: source, Score: c.Score, Channels: c.Channels, Centrality: len(g.Dependents(n.ID)),
			Proposed: opt.Date, By: opt.By, Embedder: opt.Embedder.Name(), Records: hash,
		})
	}
	duplicate := func(a, b string, score float64) {
		a, b = min(a, b), max(a, b)
		for _, d := range res.Duplicates {
			if d.Node == a && d.Target == b {
				return
			}
		}
		if decided[[3]string{a, "math2", b}] {
			return
		}
		res.Duplicates = append(res.Duplicates, Proposal{
			ID: proposalID(a, Duplicate, "math2", b), Node: a, Edge: Duplicate, System: "math2", Target: b,
			Source: source, Score: score, Centrality: len(g.Dependents(a)) + len(g.Dependents(b)),
			Proposed: opt.Date, By: opt.By, Embedder: opt.Embedder.Name(), Records: hash,
		})
	}

	for _, n := range live {
		it := ItemOf(n)
		have := map[string]bool{}
		for _, e := range edges(n, CloseTo, "id") {
			have[e] = true
		}
		for _, e := range edges(n, Aligned, "record") {
			have[e] = true
		}
		cur, set := SameAsOf(n)[x.system]
		var cands []Candidate
		for _, c := range x.Match(it, opt.K) {
			t := target(edge, c.Record)
			if t == "" || have[t] || t == cur || decided[[3]string{n.ID, x.system, t}] {
				continue
			}
			cands = append(cands, c)
		}
		if edge == Aligned {
			for _, c := range cands {
				if c.Score >= opt.Threshold {
					propose(n, Aligned, target(edge, c.Record), c)
				}
			}
			continue
		}
		for i, c := range cands {
			best := i == 0 && !set && c.Score >= opt.Threshold && (len(cands) == 1 || c.Score-cands[1].Score >= opt.Margin)
			if other, ok := claimed[c.Record.ID]; ok && other != n.ID && c.Score >= opt.Threshold {
				duplicate(n.ID, other, c.Score)
				best = false
			}
			switch {
			case best:
				propose(n, SameAs, c.Record.ID, c)
			case c.Score >= opt.Close:
				propose(n, CloseTo, c.Record.ID, c)
			}
		}
	}

	// same_as is functional in both directions (rule G02). When two nodes match one record, neither gets same_as, and the two nodes can be one.
	by := map[string][]int{}
	for i, p := range res.Proposals {
		if p.Edge == SameAs {
			by[p.Target] = append(by[p.Target], i)
		}
	}
	for _, is := range by {
		if len(is) < 2 {
			continue
		}
		for _, i := range is {
			p := &res.Proposals[i]
			p.Edge, p.ID = CloseTo, proposalID(p.Node, CloseTo, p.System, p.Target)
			for _, j := range is {
				if q := res.Proposals[j]; q.Node > p.Node {
					duplicate(p.Node, q.Node, min(p.Score, q.Score))
				}
			}
		}
	}
	return res, nil
}

// uriScheme matches the start of a URI, as the record of an aligned edge has in schema node.v1.
var uriScheme = regexp.MustCompile(`^[a-z][a-z0-9+.-]*://`)

// target returns the target of an edge to the record: the id, or for an aligned edge the URI of the record.
func target(edge string, r ingest.Record) string {
	if edge != Aligned {
		return r.ID
	}
	if uriScheme.MatchString(r.Locator) {
		return r.Locator
	}
	if uriScheme.MatchString(r.URL) {
		return r.URL
	}
	return ""
}

// edges returns the values of the field in the mappings of a list field of the node, for example the id of each close_to edge.
func edges(n *node.Node, list, field string) []string {
	var out []string
	l, _ := n.Meta[list].([]any)
	for _, e := range l {
		if m, ok := e.(map[string]any); ok {
			if s, ok := m[field].(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// ItemOf returns the item of a node: its name and aliases, the prose of its body without the remarks and the proof, its same_as values and the Mathlib declarations in its formal bindings.
func ItemOf(n *node.Node) Item {
	it := Item{Ref: n.ID, Names: append([]string{n.Name()}, n.List("aliases")...), Text: prose(n.Body), SameAs: map[string]string{}}
	for k, v := range SameAsOf(n) {
		if v != "" {
			it.SameAs[k] = v
		}
	}
	formal, _ := n.Meta["formal"].([]any)
	for _, f := range formal {
		if m, ok := f.(map[string]any); ok && m["library"] == mathlib {
			if d, ok := m["decl"].(string); ok && d != "" {
				it.Decls = append(it.Decls, d)
			}
		}
	}
	return it
}

// Write writes the proposals of the result into the queue of its system and the duplicates into the dedup queue of the working tree at root. A proposal that the queue has already keeps the date when it was first proposed. The run replaces the proposals that an earlier run on the same source made, thus a proposal that no longer matches leaves the queue.
func (r *Result) Write(root string) error {
	path := filepath.Join(root, filepath.FromSlash(QueueFile(r.System)))
	old, err := readFile(path)
	if err != nil {
		return err
	}
	if err := writeFile(path, keep(old, r.Proposals, r.Source)); err != nil {
		return err
	}
	path = filepath.Join(root, filepath.FromSlash(DedupQueue))
	old, err = readFile(path)
	if err != nil {
		return err
	}
	return writeFile(path, keep(old, r.Duplicates, r.Source))
}

// keep returns the new proposals with the dates of the old ones, and the old proposals of other sources.
func keep(old, now []Proposal, source string) []Proposal {
	first := map[string]string{}
	var out []Proposal
	for _, p := range old {
		if p.Source == source {
			first[p.ID] = p.Proposed
		} else {
			out = append(out, p)
		}
	}
	for _, p := range now {
		if d, ok := first[p.ID]; ok && d != "" {
			p.Proposed = d
		}
		out = append(out, p)
	}
	return slices.Clip(out)
}
