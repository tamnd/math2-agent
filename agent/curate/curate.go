// Package curate has the goal, the brief and the tools of the curate role (spec 12.1).
//
// The target of a curate job is a node or a record of the record plane. A record target is the source and the id of the record, for example arxiv_results:2312.00752#3KH2, and the run turns the result of the record into a new statement or concept node in its own words. A node target is a tag, and the run fixes the fields uses, about, msc, name and aliases of the node and of its neighbours, or proposes that two nodes are the same node. Curate writes no Lean and runs no computation.
//
// [Goal] holds when the changes are inside the field permission set of curate, the m2 check rules find nothing in the changed nodes, and the run did the work of its target. For a record, a new node of the matching kind must name the locator of the record in its sources, its body must not copy the text of the record, and the judge must find that the node states the same result as the record. For a node, the run must change the node or propose a duplicate that names it. [Judge] is the judge: a second model call that compares the node with the record (spec 05.4). [Brief] gives the first message of a run.
//
// The text of a record is untrusted (spec 12.7). It reaches the model only in the result of the tool record, and the judge in a data block.
package curate

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/align"
	"github.com/tamnd/math2-agent/api"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/schema"
	"github.com/tamnd/math2-agent/workspace"
)

// The verdicts of the judge (spec 05.4). Each one says what the node is to the record.
const (
	Same        = "same"        // the node states the result of the record
	Generalizes = "generalizes" // the node states more than the record
	Specializes = "specializes" // the node states less than the record
	Related     = "related"     // the node and the record are about the same thing, and neither implies the other
	Different   = "different"   // the node and the record are about different things
)

// kinds maps the kind of a record to the kind of the node that curate makes from it.
var kinds = map[string]string{"statement": "statement", "definition": "concept"}

// A Record is a record of the record plane and the source that it comes from.
type Record struct {
	ingest.Record
	Ref     string         // the source and the id, for example arxiv_results:2312.00752#3KH2
	Source  *ingest.Source // the entry of the source in the catalogue
	Records string         // the hash of the records file of the source
}

// Licence returns the SPDX id of the licence of the record: its own licence, or the licence of its source.
func (r *Record) Licence() string { return cmp.Or(r.Record.Licence, r.Source.Licence) }

// Records reads the record plane of the working tree at Root: the catalogue in manifests/sources.yaml and the records that m2 ingest wrote to build/records. It reads the records of each source once. Records is not safe for concurrent use.
type Records struct {
	Root string

	cat     *ingest.Catalogue
	sources map[string]*source
}

// source is the records of one source.
type source struct {
	entry *ingest.Source
	byID  map[string]ingest.Record
	ids   []string // in the order of the file
	hash  string
}

// IsRecord reports whether the target of a job is a record, in the form source:id.
func IsRecord(target string) bool {
	source, id, ok := strings.Cut(target, ":")
	return ok && source != "" && id != ""
}

// Get returns the record with the reference source:id.
func (rs *Records) Get(ref string) (*Record, error) {
	name, id, ok := strings.Cut(ref, ":")
	if !ok || name == "" || id == "" {
		return nil, fmt.Errorf("curate: %q is not a record reference of the form source:id", ref)
	}
	src, err := rs.source(name)
	if err != nil {
		return nil, err
	}
	r, ok := src.byID[id]
	if !ok {
		return nil, fmt.Errorf("curate: the records of %s have no record %s", name, id)
	}
	return &Record{Record: r, Ref: ref, Source: src.entry, Records: src.hash}, nil
}

// Locate returns the first record of the source whose locator or URL is the URI, as the target of an aligned edge names its record.
func (rs *Records) Locate(name, uri string) (*Record, error) {
	src, err := rs.source(name)
	if err != nil {
		return nil, err
	}
	for _, id := range src.ids {
		if r := src.byID[id]; r.Locator == uri || r.URL == uri {
			return &Record{Record: r, Ref: name + ":" + id, Source: src.entry, Records: src.hash}, nil
		}
	}
	return nil, fmt.Errorf("curate: the records of %s have no record with the locator or the URL %s", name, uri)
}

// source returns the records of the source with the name, and reads them the first time.
func (rs *Records) source(name string) (*source, error) {
	if rs.cat == nil {
		cat, err := ingest.ReadCatalogue(rs.Root)
		if err != nil {
			return nil, fmt.Errorf("curate: %w", err)
		}
		rs.cat, rs.sources = cat, map[string]*source{}
	}
	if src, ok := rs.sources[name]; ok {
		return src, nil
	}
	entry := rs.cat.Source(name)
	if entry == nil {
		return nil, fmt.Errorf("curate: the catalogue has no source %s", name)
	}
	recs, hash, err := ingest.ReadRecords(rs.Root, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("curate: the tree has no records of %s. Run m2 ingest first", name)
	}
	if err != nil {
		return nil, fmt.Errorf("curate: %w", err)
	}
	src := &source{entry: entry, byID: map[string]ingest.Record{}, hash: hash}
	for _, r := range recs {
		if _, ok := src.byID[r.ID]; !ok {
			src.ids = append(src.ids, r.ID)
		}
		src.byID[r.ID] = r
	}
	rs.sources[name] = src
	return src, nil
}

// Text returns the record in a data block marked with its reference and its licence: the kind, the title, the locator, the meta and the text. Each of them comes from the source, thus all of them are in the block.
func (r *Record) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "kind: %s\ntitle: %s\nlocator: %s\nurl: %s\n", r.Kind, r.Title, r.Locator, r.URL)
	for _, k := range slices.Sorted(maps.Keys(r.Meta)) {
		fmt.Fprintf(&b, "%s: %s\n", k, r.Meta[k])
	}
	text := r.Record.Text
	if text == "" {
		text = "The licence of the record does not let math2 store its text."
	}
	fmt.Fprintf(&b, "\n%s", text)
	return tools.Data(r.Ref, r.Licence(), b.String())
}

var refParam = api.Param{Name: "ref", Type: api.String, Required: true, Summary: "the source and the id of the record, for example arxiv_results:2312.00752#3KH2"}

// Read returns the tool record, which reads a record of the record plane.
func Read(rs *Records) agent.Tool {
	return agent.Func{
		Def: llm.Tool{Name: "record", Description: "Read a record of the record plane: a result of a paper or an item of another source, with its locator and, when the licence lets math2 store it, its text.", Schema: tools.Schema([]api.Param{refParam})},
		Fn: func(_ context.Context, _ *agent.Job, input json.RawMessage) (agent.Result, error) {
			var in struct{ Ref string }
			if err := json.Unmarshal(input, &in); err != nil {
				return agent.Result{}, err
			}
			r, err := rs.Get(in.Ref)
			if err != nil {
				return agent.Result{}, err
			}
			return agent.Result{Text: r.Text()}, nil
		},
	}
}

// Propose returns the tool propose_same, which adds to the dedup queue of the work tree the proposal that two nodes are the same node. The proposal is by the agent id, for example m2-agent/curate@0.1.0, and a person merges the nodes or keeps them apart.
func Propose(ws *workspace.Workspace, by string) agent.Tool {
	write := tools.Write(ws)
	return agent.Func{
		Def: llm.Tool{
			Name:        "propose_same",
			Description: "Propose that two nodes of the graph are the same node, for a person to merge. Give the map between the two statements in the reason.",
			Schema: tools.Schema([]api.Param{
				{Name: "node", Type: api.String, Required: true, Summary: "the tag of the first node"},
				{Name: "other", Type: api.String, Required: true, Summary: "the tag of the second node"},
				{Name: "why", Type: api.String, Required: true, Summary: "why the two nodes are the same, with the map between their statements"},
			}),
		},
		Fn: func(ctx context.Context, job *agent.Job, input json.RawMessage) (agent.Result, error) {
			var in struct{ Node, Other, Why string }
			if err := json.Unmarshal(input, &in); err != nil {
				return agent.Result{}, err
			}
			for _, id := range []string{in.Node, in.Other} {
				if _, err := ws.Read(tools.NodePath(id)); err != nil {
					return agent.Result{}, fmt.Errorf("the graph has no node %q", id)
				}
			}
			if in.Node == in.Other {
				return agent.Result{}, errors.New("a node is the same as itself. Give two nodes")
			}
			if strings.TrimSpace(in.Why) == "" {
				return agent.Result{}, errors.New("give the reason, with the map between the two statements")
			}
			old, err := ws.Read(align.DedupQueue)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return agent.Result{}, err
			}
			ps, err := align.ParseQueue(old)
			if err != nil {
				return agent.Result{}, fmt.Errorf("%s:%w", align.DedupQueue, err)
			}
			p := align.NewDuplicate(in.Node, in.Other)
			if slices.ContainsFunc(ps, func(q align.Proposal) bool { return q.ID == p.ID }) {
				return agent.Result{Text: fmt.Sprintf("The dedup queue has the proposal of %s and %s already.", p.Node, p.Target)}, nil
			}
			p.By, p.Proposed, p.Why = by, time.Now().UTC().Format(time.DateOnly), strings.TrimSpace(in.Why)
			text, err := align.FormatQueue(append(ps, p))
			if err != nil {
				return agent.Result{}, err
			}
			args, err := json.Marshal(map[string]string{"path": align.DedupQueue, "text": string(text)})
			if err != nil {
				return agent.Result{}, err
			}
			if _, err := write.Call(ctx, job, args); err != nil {
				return agent.Result{}, err
			}
			return agent.Result{Text: fmt.Sprintf("Proposed that %s and %s are the same node, in %s.", p.Node, p.Target, align.DedupQueue)}, nil
		},
	}
}

// Verdict is what the judge finds.
type Verdict struct {
	Verdict string // Same, Generalizes, Specializes, Related or Different
	Why     string // the reason, with the map between the statements
}

// judgeTool is the tool with which the model of the judge answers.
var judgeTool = llm.Tool{
	Name:        "judge",
	Description: "Give what the node is to the record: same, generalizes, specializes, related or different, with the reason.",
	Schema: json.RawMessage(`{"type":"object","properties":{` +
		`"verdict":{"type":"string","enum":["same","generalizes","specializes","related","different"]},` +
		`"why":{"type":"string","description":"the reason, with the map between the statements"}},"required":["verdict","why"],"additionalProperties":false}`),
}

// Judge compares the node with the record with the model (spec 05.4): one call of the model with the system prompt, the record and the node file, which gives the verdict. An answer with no verdict is Different. The record and the node go to the model in data blocks, thus an instruction in them is data.
func Judge(ctx context.Context, m llm.Model, model, system string, r *Record, path string, text []byte) (Verdict, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Compare the node with the record, and give the verdict with the tool %s.\n\nThe record:\n\n%s\n\nThe node:\n\n%s\n", judgeTool.Name, r.Text(), tools.Data(path, tools.Licence(path), string(text)))
	resp, err := m.Complete(ctx, &llm.Request{
		Model:     model,
		System:    system,
		Messages:  []llm.Message{{Role: llm.User, Content: []llm.Block{llm.TextBlock(b.String())}}},
		Tools:     []llm.Tool{judgeTool},
		MaxTokens: 2000,
	})
	if err != nil {
		return Verdict{}, fmt.Errorf("curate: the judge: %w", err)
	}
	for _, c := range resp.Calls() {
		if c.Name != judgeTool.Name {
			continue
		}
		var v Verdict
		if err := json.Unmarshal(c.Input, &v); err != nil {
			return Verdict{}, fmt.Errorf("curate: the judge gave a verdict that is not valid: %w", err)
		}
		if slices.Contains([]string{Same, Generalizes, Specializes, Related, Different}, v.Verdict) {
			return v, nil
		}
	}
	return Verdict{Verdict: Different, Why: "the judge gave no verdict"}, nil
}

// Copies reports whether the body shares a passage with the text of the record, with the thresholds of rule L04: at least 2 fingerprints, and at least 30% of the fingerprints of the body.
func Copies(body string, r *Record) bool {
	p := ingest.Fingerprints(body)
	k := ingest.Shared(p, r.Fingerprints)
	return k >= 2 && float64(k) >= 0.3*float64(len(p))
}

// Locates reports whether one of the sources of the node has the locator.
func Locates(n *node.Node, locator string) bool {
	list, _ := n.Meta["sources"].([]any)
	for _, s := range list {
		if m, _ := s.(map[string]any); m != nil && m["locator"] == locator {
			return true
		}
	}
	return false
}

// Check runs the m2 check rules on the node files that the changes add or change, with the other nodes of the work tree for the links. It returns the findings.
func Check(ws *workspace.Workspace, changes []workspace.Change) ([]check.Finding, error) {
	var files []check.File
	for _, c := range changes {
		if c.New != nil && strings.HasPrefix(c.Path, "nodes/") && strings.HasSuffix(c.Path, ".md") {
			files = append(files, check.File{Path: c.Path, Data: c.New})
		}
	}
	if len(files) == 0 {
		return nil, nil
	}
	s, err := schema.Load(ws.Dir)
	if err != nil {
		return nil, err
	}
	macros, err := check.LoadMacros(ws.Dir)
	if err != nil {
		return nil, err
	}
	all, err := check.ReadTree(ws.Dir)
	if err != nil {
		return nil, err
	}
	ix := check.Index{}
	for _, f := range all {
		if n, err := node.Parse(f.Path, f.Data); err == nil {
			ix.Add(n)
		}
	}
	c := check.Checker{Schema: s, Macros: macros}
	return c.Check(files, ix).Findings, nil
}

// Goal returns the goal of a curate run on the job. The judge of a record target is one call of the model m with the model name and the system prompt. Give m an [agent.Meter], thus the judge counts in the budget of the run.
func Goal(ws *workspace.Workspace, job *agent.Job, rs *Records, m llm.Model, model, system string) agent.Goal {
	return agent.OnChanges(ws, func(ctx context.Context, changes []workspace.Change) (bool, string, error) {
		if len(changes) == 0 {
			return false, "the run has not changed the tree yet", nil
		}
		if err := agent.Permit(job.Role, job.Target, changes); err != nil {
			return false, err.Error(), nil
		}
		findings, err := Check(ws, changes)
		if err != nil {
			return false, "", err
		}
		if len(findings) > 0 {
			var lines []string
			for _, f := range findings[:min(len(findings), 5)] {
				lines = append(lines, f.String())
			}
			return false, fmt.Sprintf("m2 check finds %d problems in the changed nodes: %s", len(findings), strings.Join(lines, "; ")), nil
		}
		if !IsRecord(job.Target) {
			p := tools.NodePath(job.Target)
			for _, c := range changes {
				if c.Path == p {
					return true, "", nil
				}
				if c.Path == align.DedupQueue && proposes(c, job.Target) {
					return true, "", nil
				}
			}
			return false, fmt.Sprintf("the run has not changed %s or proposed a duplicate of it yet", job.Target), nil
		}
		r, err := rs.Get(job.Target)
		if err != nil {
			return false, "", err
		}
		for _, c := range changes {
			if c.Old != nil || !strings.HasPrefix(c.Path, "nodes/") {
				continue
			}
			n, err := node.Parse(c.Path, c.New)
			if err != nil || !Locates(n, r.Locator) {
				continue
			}
			if want := kinds[r.Kind]; n.Kind() != want {
				return false, fmt.Sprintf("the record is a %s, thus the new node %s is a %s node, and it is a %s node", r.Kind, n.ID, want, n.Kind()), nil
			}
			if Copies(n.Body, r) {
				return false, fmt.Sprintf("the body of %s copies the text of the record. State the result in your own words", n.ID), nil
			}
			v, err := Judge(ctx, m, model, system, r, c.Path, c.New)
			if err != nil {
				return false, "", err
			}
			if v.Verdict != Same {
				return false, fmt.Sprintf("the judge finds that the node %s is %s to the record, not the same: %s. State the result of the record, no more and no less", n.ID, v.Verdict, strings.TrimSuffix(v.Why, ".")), nil
			}
			return true, "", nil
		}
		return false, fmt.Sprintf("the run has not added a node with a source whose locator is %s yet", r.Locator), nil
	})
}

// proposes reports whether the change of the dedup queue adds a proposal that names the node.
func proposes(c workspace.Change, id string) bool {
	old, _ := align.ParseQueue(c.Old)
	ps, _ := align.ParseQueue(c.New)
	for _, p := range ps {
		if (p.Node == id || p.Target == id) && !slices.ContainsFunc(old, func(q align.Proposal) bool { return q.ID == p.ID }) {
			return true
		}
	}
	return false
}

// Brief returns the first message of a curate run on the job.
func Brief(ws *workspace.Workspace, job *agent.Job, rs *Records) (string, error) {
	var b strings.Builder
	goal := strings.TrimSuffix(job.Goal, ".")
	if IsRecord(job.Target) {
		r, err := rs.Get(job.Target)
		if err != nil {
			return "", err
		}
		kind, ok := kinds[r.Kind]
		switch {
		case !ok:
			return "", fmt.Errorf("curate: the record %s is a %s, and curate makes nodes only of statements and definitions", r.Ref, cmp.Or(r.Kind, "record with no kind"))
		case r.Locator == "":
			return "", fmt.Errorf("curate: the record %s has no locator", r.Ref)
		case r.Record.Text == "":
			return "", fmt.Errorf("curate: the licence of the record %s does not let math2 store its text, thus curate cannot read the result", r.Ref)
		}
		fmt.Fprintf(&b, "Job %s: turn the record %s into a new %s node. The goal of the job: %s.\n\n", job.ID, r.Ref, kind, goal)
		fmt.Fprintf(&b, "Read the record with the tool record. Then search the graph. When a node states the same result already, do not add a node, and call report_stuck with the tag of the node.\n\n")
		fmt.Fprintf(&b, "Write the new node in nodes/~/ with a provisional id, for example nodes/~/~1a2b3c4d.md, and state the result in your own words. Give it a source with the locator %s", r.Locator)
		if paper := r.Meta["paper"]; paper != "" {
			fmt.Fprintf(&b, ", for example {arxiv: %s%s, locator: %s}", paper, r.Meta["version"], r.Locator)
		}
		fmt.Fprintf(&b, ". Link the nodes that the result uses, and give it uses, about and msc when the graph has them.\n")
		return b.String(), nil
	}
	p := tools.NodePath(job.Target)
	if p == "" {
		return "", fmt.Errorf("curate: the target %q is not a tag, a provisional id or a record of the form source:id", job.Target)
	}
	text, err := ws.Read(p)
	if err != nil {
		return "", fmt.Errorf("curate: %w", err)
	}
	n, err := node.Parse(p, text)
	if err != nil {
		return "", fmt.Errorf("curate: %w", err)
	}
	fmt.Fprintf(&b, "Job %s: curate the target %s, a %s node. The goal of the job: %s.\n\n", job.ID, job.Target, n.Kind(), goal)
	fmt.Fprintf(&b, "You may change the fields uses, about, msc, name and aliases of the target and of other nodes, and propose with propose_same that two nodes are the same node. Change no body.\n\n")
	fmt.Fprintf(&b, "The target node:\n\n%s\n", tools.Data(p, tools.Licence(p), string(text)))
	return b.String(), nil
}
