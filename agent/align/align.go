// Package align has the goal, the brief and the tools of the align role (spec 12.1).
//
// The target of an align job is a node. The run finds the records of the other systems that are the node, that are near it or that state it, and proposes the crosswalk edges same_as, close_to and aligned with the tool propose_edge. The tool writes each proposal into the queue file of its system in queues/align, as the aligner of the align package does, and a person accepts or rejects it. The run does not write an edge into a node.
//
// A node meets a Mathlib declaration through its formal binding and not through same_as. Thus the same_as to Mathlib of spec 12.1 is a Mathlib binding that the run appends to the formal field of the target. The run can append one only when the type hash of the Mathlib declaration is the type hash of a Math2 binding of the node in the build, thus the two declarations have the same type. Such a change is the only change of align that can merge with no review, and [AutoMerge] reports it.
//
// [Goal] holds when the changes are inside the field permission set of align, the m2 check rules find nothing in the changed nodes, and each new proposal and each new binding passes its check. A same_as proposal needs the verdict same of the judge of the curate package, and a close_to or an aligned proposal needs a verdict that is not different. [Brief] gives the first message of a run.
package align

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/curate"
	"github.com/tamnd/math2-agent/agent/tools"
	aligner "github.com/tamnd/math2-agent/align"
	"github.com/tamnd/math2-agent/api"
	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/workspace"
)

// edges are the edges that the role proposes.
var edges = []string{aligner.SameAs, aligner.CloseTo, aligner.Aligned}

// Propose returns the tool propose_edge, which adds to the queue of the work tree the proposal of an edge from the target of the job to a record. The proposal is by the agent id, for example m2-agent/align@0.1.0. The node has at most one edge to a record, thus a proposal replaces the proposal of the role to the same record, and the edge none removes it.
func Propose(ws *workspace.Workspace, rs *curate.Records, by string) agent.Tool {
	write := tools.Write(ws)
	return agent.Func{
		Def: llm.Tool{
			Name:        "propose_edge",
			Description: "Propose a crosswalk edge from the target node to a record, for a person to accept: same_as when the record is the node, close_to when it is near the node and not the same, and aligned when a paper states the node. A new proposal to a record replaces your proposal to it, and the edge none removes your proposal.",
			Schema: tools.Schema([]api.Param{
				{Name: "edge", Type: api.String, Required: true, Summary: "same_as, close_to, aligned, or none to remove your proposal to the record"},
				{Name: "ref", Type: api.String, Required: true, Summary: "the source and the id of the record, for example wikidata:Q11518"},
				{Name: "why", Type: api.String, Required: true, Summary: "why the edge holds, with the map between the node and the record, or for close_to how they differ"},
			}),
		},
		Fn: func(ctx context.Context, job *agent.Job, input json.RawMessage) (agent.Result, error) {
			var in struct{ Edge, Ref, Why string }
			if err := json.Unmarshal(input, &in); err != nil {
				return agent.Result{}, err
			}
			none := in.Edge == "none"
			if strings.TrimSpace(in.Why) == "" && !none {
				return agent.Result{}, errors.New("give the reason, with the map between the node and the record")
			}
			n, err := target(ws, job.Target)
			if err != nil {
				return agent.Result{}, err
			}
			r, err := rs.Get(in.Ref)
			if err != nil {
				return agent.Result{}, err
			}
			edge := in.Edge
			if none {
				edge = aligner.Edge(r.Source.ID)
			}
			p, err := proposal(n, edge, r)
			if err != nil {
				return agent.Result{}, err
			}
			if err := decided(ws, p); err != nil && !none {
				return agent.Result{}, err
			}
			p.By, p.Proposed, p.Why = by, time.Now().UTC().Format(time.DateOnly), strings.TrimSpace(in.Why)
			file := aligner.QueueFile(p.System)
			old, err := ws.Read(file)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return agent.Result{}, err
			}
			ps, err := aligner.ParseQueue(old)
			if err != nil {
				return agent.Result{}, fmt.Errorf("%s:%w", file, err)
			}
			if i := slices.IndexFunc(ps, func(q aligner.Proposal) bool { return q.ID == p.ID }); i >= 0 && !mine(ps[i]) && !none {
				return agent.Result{Text: fmt.Sprintf("The queue %s has the proposal of this edge already, by %s.", file, ps[i].By)}, nil
			}
			before := len(ps)
			ps = slices.DeleteFunc(ps, func(q aligner.Proposal) bool {
				if mine(q) && q.Node == p.Node && q.Target == p.Target {
					if q.ID == p.ID {
						p.Proposed = q.Proposed
					}
					return true
				}
				return false
			})
			msg := fmt.Sprintf("Proposed the edge %s from %s to %s %s, in %s.", p.Edge, p.Node, p.System, p.Target, file)
			if none {
				if len(ps) == before {
					return agent.Result{Text: fmt.Sprintf("You have no proposal from %s to %s %s.", p.Node, p.System, p.Target)}, nil
				}
				msg = fmt.Sprintf("Removed your proposal from %s to %s %s, in %s.", p.Node, p.System, p.Target, file)
			} else {
				ps = append(ps, p)
			}
			text, err := aligner.FormatQueue(ps)
			if err != nil {
				return agent.Result{}, err
			}
			args, err := json.Marshal(map[string]string{"path": file, "text": string(text)})
			if err != nil {
				return agent.Result{}, err
			}
			if _, err := write.Call(ctx, job, args); err != nil {
				return agent.Result{}, err
			}
			return agent.Result{Text: msg}, nil
		},
	}
}

// mine reports whether the proposal is by the align role.
func mine(p aligner.Proposal) bool { return strings.HasPrefix(p.By, "m2-agent/align@") }

// proposal returns the proposal of the edge from the node to the record, with no date, author or reason. It refuses an edge that the source does not give and an edge that the node has already.
func proposal(n *node.Node, edge string, r *curate.Record) (aligner.Proposal, error) {
	source := r.Source.ID
	if source == "mathlib" {
		return aligner.Proposal{}, errors.New("a node meets a Mathlib declaration through its formal binding and not through same_as. Append a Mathlib binding to the target when the type hash of the declaration is the type hash of a Math2 binding of the node")
	}
	system, ok := aligner.System(source)
	if !ok {
		return aligner.Proposal{}, fmt.Errorf("the crosswalk has no system for the records of %s", source)
	}
	switch want := aligner.Edge(source); {
	case !slices.Contains(edges, edge):
		return aligner.Proposal{}, fmt.Errorf("the edge %q is not one of %s", edge, strings.Join(edges, ", "))
	case want == aligner.Aligned && edge != aligner.Aligned:
		return aligner.Proposal{}, fmt.Errorf("a record of %s is a paper, and a paper states a node. Thus the edge is aligned", source)
	case want != aligner.Aligned && edge == aligner.Aligned:
		return aligner.Proposal{}, fmt.Errorf("only a paper gets an aligned edge, and a record of %s is not a paper", source)
	}
	to := aligner.Target(edge, r.Record)
	if to == "" {
		return aligner.Proposal{}, fmt.Errorf("the record %s has no URI for an aligned edge", r.Ref)
	}
	if edge != aligner.Aligned && crosswalk.Known(system) {
		if err := crosswalk.Check(system, to); err != nil {
			return aligner.Proposal{}, err
		}
	}
	if has(n, edge, system, to) {
		return aligner.Proposal{}, fmt.Errorf("the node %s has the edge %s to %s %s already", n.ID, edge, system, to)
	}
	p := aligner.NewProposal(n.ID, edge, system, to)
	p.Source, p.Records = source, r.Records
	return p, nil
}

// has reports whether the node has the edge to the target in the system.
func has(n *node.Node, edge, system, to string) bool {
	if edge == aligner.SameAs {
		v, ok := aligner.SameAsOf(n)[system]
		return ok && v == to
	}
	field := map[string]string{aligner.CloseTo: "id", aligner.Aligned: "record"}[edge]
	list, _ := n.Meta[edge].([]any)
	for _, e := range list {
		if m, _ := e.(map[string]any); m != nil && m[field] == to {
			return true
		}
	}
	return false
}

// decided returns an error when a person accepted or rejected the edge of the proposal already. A rejected edge is not proposed again.
func decided(ws *workspace.Workspace, p aligner.Proposal) error {
	ds, err := aligner.ReadDecisions(ws.Dir)
	if err != nil {
		return err
	}
	for _, d := range ds {
		if d.Tag == p.Node && d.System == p.System && d.ID == p.Target {
			return fmt.Errorf("%s %s the edge from %s to %s %s on %s. Do not propose it again", d.Person, strings.ToLower(d.Decision), p.Node, p.System, p.Target, d.Date)
		}
	}
	return nil
}

// target reads and parses the node with the id from the work tree.
func target(ws *workspace.Workspace, id string) (*node.Node, error) {
	p := tools.NodePath(id)
	if p == "" {
		return nil, fmt.Errorf("align: the target %q is not a tag or a provisional id", id)
	}
	text, err := ws.Read(p)
	if err != nil {
		return nil, fmt.Errorf("align: %w", err)
	}
	return node.Parse(p, text)
}

// Bindings returns the Lean bindings to Mathlib that the change of the node file appends to its formal field, in order.
func Bindings(c workspace.Change) ([]map[string]any, error) {
	if c.Old == nil || c.New == nil {
		return nil, nil
	}
	o, err := node.Parse(c.Path, c.Old)
	if err != nil {
		return nil, err
	}
	n, err := node.Parse(c.Path, c.New)
	if err != nil {
		return nil, err
	}
	old, _ := o.Meta["formal"].([]any)
	cur, _ := n.Meta["formal"].([]any)
	if len(cur) < len(old) || !reflect.DeepEqual(old, cur[:len(old)]) {
		return nil, nil
	}
	var out []map[string]any
	for _, f := range cur[len(old):] {
		if m, _ := f.(map[string]any); m != nil && m["system"] == "lean4" && m["library"] == "mathlib" {
			out = append(out, m)
		}
	}
	return out, nil
}

// SameType checks a Mathlib binding of the node: the records of mathlib must have the declaration, and its type hash must be the type hash of a Math2 binding of the node in the build status st, with the same match. It returns "" when the binding passes, and else the reason.
func SameType(rs *curate.Records, st []status.Status, tag string, binding map[string]any) (string, error) {
	decl, _ := binding["decl"].(string)
	r, err := rs.Get("mathlib:" + decl)
	if err != nil {
		return fmt.Sprintf("the records of mathlib have no declaration %s. Bind a declaration that m2 ingest mathlib read", decl), nil
	}
	hash := r.Meta["type_hash"]
	i := slices.IndexFunc(st, func(s status.Status) bool { return s.Tag == tag })
	if i < 0 {
		return fmt.Sprintf("the build has no status of %s. Run m2 build", tag), nil
	}
	for _, b := range st[i].Bindings {
		if !strings.HasPrefix(b.Decl, "Math2.") || b.Adapter != "" || b.TypeHash == "" || b.TypeHash != hash {
			continue
		}
		if binding["match"] != b.Match {
			return fmt.Sprintf("%s has the type of %s, thus its match is %s, the match of the Math2 binding", decl, b.Decl, b.Match), nil
		}
		return "", nil
	}
	return fmt.Sprintf("the type hash of %s is %s, and no Math2 binding of %s has it. Align binds Mathlib only when the type hash is identical. Remove the binding", decl, hash, tag), nil
}

// AutoMerge reports whether the changes of an align run on the job can merge with no review: each change appends only Mathlib bindings to the target, and each binding passes SameType. Each other change of align needs a person.
func AutoMerge(rs *curate.Records, st []status.Status, job *agent.Job, changes []workspace.Change) (bool, error) {
	if len(changes) == 0 || agent.Permit(job.Role, job.Target, changes) != nil {
		return false, nil
	}
	for _, c := range changes {
		if c.Path != tools.NodePath(job.Target) {
			return false, nil
		}
		bs, err := Bindings(c)
		if err != nil || len(bs) == 0 {
			return false, err
		}
		for _, b := range bs {
			why, err := SameType(rs, st, job.Target, b)
			if err != nil || why != "" {
				return false, err
			}
		}
	}
	return true, nil
}

// Goal returns the goal of an align run on the job. st is the status of the build of the tree. The judge is one call of the model m with the model name and the system prompt for each new proposal. Give m an [agent.Meter], thus the judge counts in the budget of the run.
func Goal(ws *workspace.Workspace, job *agent.Job, rs *curate.Records, st []status.Status, m llm.Model, model, system string) agent.Goal {
	verdicts := map[string]curate.Verdict{} // by the proposal and the hash of the node file
	return agent.OnChanges(ws, func(ctx context.Context, changes []workspace.Change) (bool, string, error) {
		if len(changes) == 0 {
			return false, "the run has not proposed an edge yet", nil
		}
		if err := agent.Permit(job.Role, job.Target, changes); err != nil {
			return false, err.Error(), nil
		}
		findings, err := curate.Check(ws, changes)
		if err != nil {
			return false, "", err
		}
		if len(findings) > 0 {
			return false, fmt.Sprintf("m2 check finds %d problems in the changed nodes: %s", len(findings), findings[0]), nil
		}
		file := tools.NodePath(job.Target)
		text, err := ws.Read(file)
		if err != nil {
			return false, "", err
		}
		n, err := node.Parse(file, text)
		if err != nil {
			return false, "", err
		}
		sum := sha256.Sum256(text)
		done := 0
		for _, c := range changes {
			if c.Path == file {
				bs, err := Bindings(c)
				if err != nil {
					return false, "", err
				}
				for _, b := range bs {
					why, err := SameType(rs, st, job.Target, b)
					if err != nil {
						return false, "", err
					}
					if why != "" {
						return false, why, nil
					}
					done++
				}
				continue
			}
			if path.Dir(c.Path) != aligner.QueueDir {
				continue
			}
			for _, p := range added(c) {
				r, err := record(rs, p)
				if err != nil {
					return false, err.Error(), nil
				}
				if want, err := proposal(n, p.Edge, r); err != nil || want.ID != p.ID || want.Source != p.Source {
					return false, fmt.Sprintf("the proposal %s is not the edge %s from %s to the record %s: %v", p.ID, p.Edge, p.Node, r.Ref, err), nil
				}
				if err := decided(ws, p); err != nil {
					return false, err.Error(), nil
				}
				key := p.ID + " " + hex.EncodeToString(sum[:])
				v, ok := verdicts[key]
				if !ok {
					if v, err = curate.Judge(ctx, m, model, system, r, file, text); err != nil {
						return false, "", err
					}
					verdicts[key] = v
				}
				switch {
				case p.Edge == aligner.SameAs && v.Verdict != curate.Same:
					return false, fmt.Sprintf("the judge finds that the node is %s to the record %s, not the same: %s. Propose close_to, or no edge", v.Verdict, r.Ref, strings.TrimSuffix(v.Why, ".")), nil
				case v.Verdict == curate.Different:
					return false, fmt.Sprintf("the judge finds that the node and the record %s are about different things: %s. Remove the proposal", r.Ref, strings.TrimSuffix(v.Why, ".")), nil
				}
				done++
			}
		}
		if done == 0 {
			return false, "the run has not proposed an edge or bound the target to Mathlib yet", nil
		}
		return true, "", nil
	})
}

// added returns the proposals of the align role that the change of the queue file adds or changes.
func added(c workspace.Change) []aligner.Proposal {
	old, _ := aligner.ParseQueue(c.Old)
	ps, _ := aligner.ParseQueue(c.New)
	var out []aligner.Proposal
	for _, p := range ps {
		if mine(p) && !slices.ContainsFunc(old, func(q aligner.Proposal) bool { return reflect.DeepEqual(p, q) }) {
			out = append(out, p)
		}
	}
	return out
}

// record returns the record of the target of the proposal.
func record(rs *curate.Records, p aligner.Proposal) (*curate.Record, error) {
	if p.Edge == aligner.Aligned {
		return rs.Locate(p.Source, p.Target)
	}
	return rs.Get(p.Source + ":" + p.Target)
}

// Brief returns the first message of an align run on the job.
func Brief(ws *workspace.Workspace, job *agent.Job) (string, error) {
	n, err := target(ws, job.Target)
	if err != nil {
		return "", err
	}
	text, err := ws.Read(n.Path)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Job %s: align the target %s, a %s node. The goal of the job: %s.\n\n", job.ID, job.Target, n.Kind(), strings.TrimSuffix(job.Goal, "."))
	fmt.Fprintf(&b, "Find the records of the other systems that are the node, that are near it, or that state it. Read each one with the tool record, and propose each edge with propose_edge. A person accepts or rejects each proposal.\n\n")
	fmt.Fprintf(&b, "Do not write an edge into the node. The only change of the node that you may make is a Mathlib binding appended to its formal field, and only when the type hash of the Mathlib declaration is the type hash of a Math2 binding of the node.\n\n")
	fmt.Fprintf(&b, "The target node:\n\n%s\n", tools.Data(n.Path, tools.Licence(n.Path), string(text)))
	return b.String(), nil
}
