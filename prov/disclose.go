package prov

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/node"
)

// Facts is what [Disclose] and [Formalization] read about one node.
type Facts struct {
	Node    *node.Node         // the node
	Log     []Line             // the provenance log of the node
	Runs    map[string]*Run    // the run summaries by id, or nil
	Build   map[string]Binding // what the last build knows of each formal binding, by the declaration of the binding, or nil when there is no build
	Mathlib string             // the Mathlib commit of the tree, or ""
	Repo    string             // the URL of the content repository, for example https://github.com/tamnd/math2, or ""
	Commit  string             // the content commit, or ""
	License string             // the SPDX licence of the formal code, or "" for Apache-2.0
}

// Binding is what the build knows of one formal binding of a node.
type Binding struct {
	Level     string   // the evidence level of the binding: E5, E4 or None
	Axioms    []string // the axioms of the declaration that the kernel checked
	Fidelity  string   // the fidelity level, F0 to F3
	Reviewers []string // the reviewers whose reviews count for the fidelity level
}

// The kinds of agent contribution that a disclosure names (spec 16.5 item 8). They are adapted from the AI contributions wiki of erdosproblems.com.
const (
	NoAgent      = "no agent took part"
	AgentSupport = "literature search, formalization, rewriting or computation only"
	Together     = "people and agents worked together"
	AgentResult  = "an agent found the result"
)

// formalItem is one item of the formal list of a node.
type formalItem struct {
	system, library, decl, match, adapter string
}

// sourceItem is one item of the sources list of a node.
type sourceItem struct {
	key, value, locator, note string
}

// sourceKeys are the identifier keys of a source item (schema/node.v1.json).
var sourceKeys = []string{"zbmath", "arxiv", "doi", "book", "mr", "stacks", "kerodon", "mathoverflow", "mathse", "erdos", "formal_conjectures", "palomar", "hexagon", "url"}

func formalItems(n *node.Node) []formalItem {
	var out []formalItem
	list, _ := n.Meta["formal"].([]any)
	for _, v := range list {
		m, _ := v.(map[string]any)
		f := formalItem{system: str(m["system"]), library: str(m["library"]), decl: str(m["decl"]), match: str(m["match"]), adapter: str(m["adapter"])}
		if f.decl != "" {
			out = append(out, f)
		}
	}
	return out
}

func sourceItems(n *node.Node) []sourceItem {
	var out []sourceItem
	list, _ := n.Meta["sources"].([]any)
	for _, v := range list {
		m, _ := v.(map[string]any)
		for _, k := range sourceKeys {
			if s := str(m[k]); s != "" {
				out = append(out, sourceItem{key: k, value: s, locator: str(m["locator"]), note: str(m["note"])})
				break
			}
		}
	}
	return out
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// agentRole returns the role of the agent, for example formalize for m2-agent/formalize@0.9.1.
func agentRole(agent string) string {
	_, r, _ := strings.Cut(agent, "/")
	r, _, _ = strings.Cut(r, "@")
	return r
}

// Kind returns the kind of agent contribution to the node in the log (spec 16.5 item 8): NoAgent, AgentSupport, Together or AgentResult. An agent of the role prove or explore finds a result. When a person also changed the node, the kind is Together. The kind of the other roles is AgentSupport. The kinds of the wiki that tell if the literature had the result need the literature search of spec 16.5 item 5, and the log does not have it, so Kind does not give them.
func Kind(log []Line) string {
	var agent, result, person bool
	for _, l := range log {
		switch {
		case l.Agent != "":
			agent = true
			if r := agentRole(l.Agent); r == "prove" || r == "explore" {
				result = true
			}
		case l.Human != "" && l.Op != Confirm && l.Op != Quarantine:
			person = true
		}
	}
	switch {
	case !agent:
		return NoAgent
	case result && person:
		return Together
	case result:
		return AgentResult
	}
	return AgentSupport
}

// runsOf returns the ids of the runs in the log, and of the runs whose summaries name the node as a target, in order.
func runsOf(f Facts) []string {
	seen := map[string]bool{}
	for _, l := range f.Log {
		if l.Run != "" {
			seen[l.Run] = true
		}
	}
	for id, r := range f.Runs {
		if slices.Contains(r.Targets, f.Node.ID) {
			seen[id] = true
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

// people returns the persons who changed a field in the log, and the persons on whose behalf an agent changed one, in order. A confirm or a quarantine is a check and not a change, so it does not count. With formal true, only the formal fields count.
func people(f Facts, formal bool) []string {
	seen := map[string]bool{}
	for _, l := range f.Log {
		if formal && !within(l.Field, "formal") && l.Field != Whole || l.Op == Confirm || l.Op == Quarantine {
			continue
		}
		switch {
		case l.Human != "" && IsPerson(l.Human):
			seen[l.Human] = true
		case l.Agent != "" && f.Runs[l.Run] != nil && IsPerson(f.Runs[l.Run].OnBehalfOf):
			seen[f.Runs[l.Run].OnBehalfOf] = true
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

// uniq returns the items in order, with no item twice.
func uniq(items []string) []string {
	slices.Sort(items)
	return slices.Compact(items)
}

// join joins the items with commas and an "and" before the last item.
func join(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// plural returns the count and the noun, with an s when the count is not 1.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// minutes says a time in seconds for a reader.
func minutes(s float64) string {
	switch m := s / 60; {
	case m < 1:
		return fmt.Sprintf("%.0f seconds", s)
	case m < 120:
		return fmt.Sprintf("%.0f minutes", m)
	default:
		return fmt.Sprintf("%.1f hours", m/60)
	}
}

func dollars(usd float64) string { return fmt.Sprintf("%.2f US dollars", usd) }

// Disclose returns the disclosure paragraph of the node (spec 16.5 item 7). It says who changed the node and how, with the model, the prompt, the time and the spend of each agent run. It says which fields no person checked and which are in quarantine. For each formal binding it gives the evidence level, the fidelity level and the reviewers that count. It says what the record does not have, so that a reader can judge the disclosure. The text is plain, with no Markdown.
func Disclose(f Facts) string {
	t := f.Node.ID
	var s []string
	at := ""
	if f.Commit != "" {
		at = " at the commit " + f.Commit
	}
	s = append(s, fmt.Sprintf("This disclosure is about the math2 node m2://%s, %s%s.", t, f.Node.Name(), at))
	if len(f.Log) == 0 {
		s = append(s, fmt.Sprintf("The node has no provenance log in %s, so the record does not say who made it.", Path(t)))
	}
	var direct, checked []string
	for _, l := range f.Log {
		switch {
		case l.Human == "":
		case l.Op == Confirm || l.Op == Quarantine:
			checked = append(checked, l.Human)
		default:
			direct = append(direct, l.Human)
		}
	}
	if len(direct) > 0 {
		s = append(s, fmt.Sprintf("These people changed it themselves: %s.", join(uniq(direct))))
	}
	if len(checked) > 0 {
		s = append(s, fmt.Sprintf("These people checked the text of agents: %s.", join(uniq(checked))))
	}
	ids := runsOf(f)
	var failed []string
	var secs, usd float64
	var noCost int
	for _, id := range ids {
		r := f.Runs[id]
		var lines []Line
		for _, l := range f.Log {
			if l.Run == id {
				lines = append(lines, l)
			}
		}
		if r == nil {
			if len(lines) > 0 {
				l := lines[0]
				s = append(s, fmt.Sprintf("The agent %s changed %s in the run %s with the model %s and the prompt %s. The summary of the run is not in the run directory.", l.Agent, plural(len(lines), "field"), id, cmp.Or(l.Model, "that the line does not name"), cmp.Or(l.Prompt, "that the line does not name")))
			}
			noCost++
			continue
		}
		if len(lines) == 0 && r.Outcome != "success" {
			failed = append(failed, id)
		}
		if len(lines) > 0 {
			s = append(s, fmt.Sprintf("The agent %s changed %s in the run %s, on behalf of %s. It used the models %s and the prompt %s, and the outcome was %s.", r.Agent, plural(len(lines), "field"), id, r.OnBehalfOf, join(r.Models), r.Prompt, r.Outcome))
		}
		if r.Seconds == 0 && r.SpendUSD == 0 {
			noCost++
		}
		secs += r.Seconds
		usd += r.SpendUSD
	}
	if len(failed) > 0 {
		s = append(s, fmt.Sprintf("The run summaries also name %s on this node that did not succeed, and math2 did not merge their output: %s.", plural(len(failed), "run"), join(failed)))
	}
	if len(ids) > 0 {
		switch {
		case noCost == len(ids):
			s = append(s, "The run summaries do not record the time or the spend of the runs.")
		case noCost > 0:
			what := "the summaries of " + plural(noCost, "run") + " do not"
			if noCost == 1 {
				what = "the summary of 1 run does not"
			}
			s = append(s, fmt.Sprintf("The runs took %s and spent %s, but %s record the time or the spend.", minutes(secs), dollars(usd), what))
		default:
			s = append(s, fmt.Sprintf("The runs took %s and spent %s in all.", minutes(secs), dollars(usd)))
		}
	}
	s = append(s, "The kind of agent contribution is: "+Kind(f.Log)+".")
	q := Quarantined(f.Log)
	var open []string
	for field := range Unconfirmed(f.Log) {
		if _, ok := q[field]; !ok {
			open = append(open, field)
		}
	}
	slices.Sort(open)
	if len(open) > 0 {
		s = append(s, fmt.Sprintf("An agent wrote these fields and no person checked them: %s.", join(open)))
	}
	if len(q) > 0 {
		s = append(s, fmt.Sprintf("These fields are in quarantine: %s.", join(slices.Sorted(maps.Keys(q)))))
	}
	items := formalItems(f.Node)
	if len(items) == 0 {
		s = append(s, "The node has no formal binding.")
	}
	for _, it := range items {
		b := fmt.Sprintf("The node is bound to %s in %s %s, and the match is %s", it.decl, it.system, it.library, it.match)
		if it.adapter != "" {
			b += ", with the adapter " + it.adapter
		}
		bf, ok := f.Build[it.decl]
		switch {
		case f.Build == nil:
			b += ". There is no build, so this disclosure does not say if the kernel checked it."
		case !ok:
			b += ". The last build does not have this binding."
		default:
			b += fmt.Sprintf(". The binding is at the evidence level %s and the fidelity level %s", bf.Level, bf.Fidelity)
			if len(bf.Reviewers) > 0 {
				b += ", with reviews by " + join(bf.Reviewers)
			}
			b += "."
			if len(bf.Axioms) > 0 {
				b += fmt.Sprintf(" It uses the axioms %s.", join(bf.Axioms))
			}
		}
		s = append(s, b)
	}
	if len(ids) > 0 {
		s = append(s, "The full records of the runs, with the transcripts, are in the artefact store under the run ids.")
	}
	return strings.Join(s, " ") + "\n"
}

// The keys of formalization.yaml v0.4 (https://github.com/mathlib-initiative/formalization.yaml) that Formalization writes. The order of the fields is the order of the keys in the file.
type (
	fyFile struct {
		Version         string        `yaml:"version"`
		Project         fyProject     `yaml:"project"`
		Repository      *fyRepository `yaml:"repository,omitempty"`
		Sources         []fySource    `yaml:"sources"`
		Related         []fyRelated   `yaml:"related_formalizations,omitempty"`
		Classification  *fyClass      `yaml:"classification,omitempty"`
		Automation      fyAutomation  `yaml:"automation"`
		Status          fyStatus      `yaml:"status"`
		Fidelity        *fyFidelity   `yaml:"fidelity,omitempty"`
		Review          fyReview      `yaml:"review"`
		Alignment       []fyAlignment `yaml:"alignment,omitempty"`
		Acknowledgement string        `yaml:"acknowledgements,omitempty"`
	}
	fyProject struct {
		Name        string   `yaml:"name"`
		Description string   `yaml:"description,omitempty"`
		Authors     []string `yaml:"authors"`
		License     string   `yaml:"license"`
	}
	fyRepository struct {
		Role        string `yaml:"role"`
		Substantive struct {
			ID       string `yaml:"id"`
			Revision string `yaml:"revision,omitempty"`
		} `yaml:"substantive_formalization"`
	}
	fySource struct {
		Title        string `yaml:"title"`
		ID           string `yaml:"id,omitempty"`
		Type         string `yaml:"type,omitempty"`
		Location     string `yaml:"location,omitempty"`
		Relationship string `yaml:"relationship,omitempty"`
		Note         string `yaml:"note,omitempty"`
		License      string `yaml:"license,omitempty"`
	}
	fyRelated struct {
		ID           string `yaml:"id"`
		Relationship string `yaml:"relationship,omitempty"`
		Note         string `yaml:"note,omitempty"`
	}
	fyClass struct {
		MSC []string `yaml:"msc2020"`
	}
	fyAutomation struct {
		Methods  []fyMethod `yaml:"methods"`
		SpendUSD float64    `yaml:"spend_usd,omitempty"`
		Notes    string     `yaml:"notes,omitempty"`
	}
	fyMethod struct {
		Method    string   `yaml:"method"`
		Models    []string `yaml:"models,omitempty"`
		Framework string   `yaml:"framework,omitempty"`
		Tools     string   `yaml:"tool_setup,omitempty"`
		Cost      *fyCost  `yaml:"cost,omitempty"`
		Notes     string   `yaml:"prompting_notes,omitempty"`
	}
	fyCost struct {
		WallTime string  `yaml:"wall_time,omitempty"`
		SpendUSD float64 `yaml:"spend_usd,omitempty"`
	}
	fyStatus struct {
		Scope      string     `yaml:"scope"`
		SorryCount *int       `yaml:"sorry_count,omitempty"`
		Axioms     []string   `yaml:"axioms,omitempty"`
		Results    []fyResult `yaml:"main_results"`
	}
	fyResult struct {
		Declaration string   `yaml:"declaration"`
		SorryCount  *int     `yaml:"sorry_count,omitempty"`
		Axioms      []string `yaml:"axioms,omitempty"`
	}
	fyFidelity struct {
		Divergences string `yaml:"divergences"`
	}
	fyReview struct {
		Status    string   `yaml:"status"`
		Reviewers []string `yaml:"reviewers,omitempty"`
		Notes     string   `yaml:"notes,omitempty"`
	}
	fyAlignment struct {
		Informal string `yaml:"informal"`
		Formal   string `yaml:"formal"`
		System   string `yaml:"system"`
		Library  string `yaml:"library,omitempty"`
		Match    string `yaml:"match"`
		Adapter  string `yaml:"adapter,omitempty"`
	}
)

// mathlibRepo is the repository of Mathlib.
const mathlibRepo = "https://github.com/leanprover-community/mathlib4"

// ErrNoBinding is the error of [Formalization] for a node with no formal binding.
var ErrNoBinding = errors.New("the node has no formal binding")

var wikiLink = regexp.MustCompile(`\[\[([^\]|]*)(?:\|[^\]]*)?\]\]`)

// statement returns the text of the first section of the node that states the mathematics, on one line, with the text of each link in place of the link.
func statement(n *node.Node) string {
	names := check.StatementSections()
	for _, sec := range check.Sections(n) {
		if slices.Contains(names, sec.Name) {
			return strings.Join(strings.Fields(wikiLink.ReplaceAllString(sec.Text, "$1")), " ")
		}
	}
	return ""
}

// Formalization returns the formalization.yaml of the node, in the form v0.4 of the Mathlib Initiative (spec 16.5 item 7). With it a person can register a formal result of math2 in the Palomar registry. The project is the binding of the node: the formal declarations are in their library, and math2 adds the statement, the match and the adapter. A node with no formal binding gives ErrNoBinding.
func Formalization(f Facts) ([]byte, error) {
	items := formalItems(f.Node)
	if len(items) == 0 {
		return nil, fmt.Errorf("prov: m2://%s: %w", f.Node.ID, ErrNoBinding)
	}
	t := f.Node.ID
	y := fyFile{Version: "v0.4"}
	y.Project = fyProject{Name: f.Node.Name(), License: cmp.Or(f.License, "Apache-2.0"), Authors: people(f, true)}
	if len(y.Project.Authors) == 0 {
		y.Project.Authors = people(f, false)
	}
	if len(y.Project.Authors) == 0 {
		y.Project.Authors = []string{"the math2 contributors"}
	}
	desc := fmt.Sprintf("The math2 node m2://%s, a %s", t, cmp.Or(f.Node.Kind(), "node"))
	if st := statement(f.Node); st != "" {
		desc += ": " + st
	} else {
		desc += "."
	}
	y.Project.Description = desc

	if !slices.ContainsFunc(items, func(it formalItem) bool { return it.library != "mathlib" }) {
		r := &fyRepository{Role: "thin-wrapper"}
		r.Substantive.ID = mathlibRepo
		r.Substantive.Revision = f.Mathlib
		y.Repository = r
	}

	rel := "formalizes"
	if slices.ContainsFunc(items, func(it formalItem) bool { return it.match != "exact" }) {
		rel = "adapts"
	}
	self := fySource{Title: f.Node.Name(), ID: "m2://" + t, Type: "knowledge graph node", Location: "the statement of the node", Relationship: rel, License: "CC-BY-SA-4.0"}
	if f.Repo != "" && f.Commit != "" && f.Node.Path != "" {
		self.Note = "The file of the node is " + f.Repo + "/blob/" + f.Commit + "/" + f.Node.Path + "."
	}
	y.Sources = append(y.Sources, self)
	for _, src := range sourceItems(f.Node) {
		switch src.key {
		case "formal_conjectures", "palomar", "hexagon":
			y.Related = append(y.Related, fyRelated{ID: src.key + ":" + src.value, Relationship: "other", Note: "math2 lists it as a source of the node."})
			continue
		}
		s := fySource{Title: sourceID(src), ID: sourceID(src), Type: sourceType(src.key), Location: src.locator, Note: src.note}
		y.Sources = append(y.Sources, s)
	}
	if msc := f.Node.List("msc"); len(msc) > 0 {
		y.Classification = &fyClass{MSC: msc}
	}

	y.Automation = automation(f)

	var axioms []string
	sorry := 0
	for _, it := range items {
		res := fyResult{Declaration: cmp.Or(it.adapter, it.decl)}
		if b, ok := f.Build[it.decl]; ok {
			n := 0
			if slices.Contains(b.Axioms, "sorryAx") {
				n = 1
			}
			res.SorryCount = &n
			res.Axioms = b.Axioms
			sorry += n
			for _, a := range b.Axioms {
				if !slices.Contains(axioms, a) {
					axioms = append(axioms, a)
				}
			}
		}
		y.Status.Results = append(y.Status.Results, res)
	}
	y.Status.Scope = fmt.Sprintf("The binding of one math2 node to %s.", plural(len(items), "formal declaration"))
	if f.Build != nil {
		slices.Sort(axioms)
		y.Status.Axioms = axioms
		y.Status.SorryCount = &sorry
	} else {
		y.Status.Scope += " There is no build, so the file does not give the axioms."
	}

	var div []string
	for _, it := range items {
		div = append(div, divergence(it))
	}
	y.Fidelity = &fyFidelity{Divergences: strings.Join(div, " ")}

	y.Review = review(f, items)
	for _, it := range items {
		y.Alignment = append(y.Alignment, fyAlignment{Informal: "m2://" + t, Formal: it.decl, System: it.system, Library: it.library, Match: it.match, Adapter: it.adapter})
	}
	ack := "m2 prov disclose made this file from the provenance and the build of math2"
	if f.Commit != "" {
		ack += " at the commit " + f.Commit
	}
	ack += "."
	if f.Mathlib != "" {
		ack += " The declarations of Mathlib are at the commit " + f.Mathlib + "."
	}
	y.Acknowledgement = ack
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&y); err != nil {
		return nil, err
	}
	return buf.Bytes(), enc.Close()
}

// sourceID returns the identifier of a source item in a form that formalization.yaml accepts.
func sourceID(s sourceItem) string {
	switch s.key {
	case "arxiv":
		return "arXiv:" + s.value
	case "doi":
		return "https://doi.org/" + s.value
	case "book", "url":
		return s.value
	}
	return s.key + ":" + s.value
}

// sourceType returns the kind of source of an identifier key.
func sourceType(key string) string {
	switch key {
	case "zbmath", "arxiv", "doi", "mr":
		return "article"
	case "book":
		return "book"
	case "stacks", "kerodon":
		return "online book"
	case "mathoverflow", "mathse":
		return "web post"
	case "erdos":
		return "problem list"
	}
	return "web page"
}

// divergence says how the declaration of a binding differs from the statement of the node.
func divergence(it formalItem) string {
	var s string
	switch it.match {
	case "exact":
		s = it.decl + " states the node exactly."
	case "stronger":
		s = it.decl + " is stronger than the statement of the node."
	case "weaker":
		s = it.decl + " is weaker than the statement of the node."
	case "variant":
		s = it.decl + " states a variant of the node."
	case "partial":
		s = it.decl + " states a part of the node."
	default:
		s = fmt.Sprintf("The match of %s is %q.", it.decl, it.match)
	}
	if it.adapter != "" {
		s += " The adapter " + it.adapter + " states the node from it."
	}
	return s
}

// automation returns the methods: one for the people, and one for each agent run that changed the node.
func automation(f Facts) fyAutomation {
	var a fyAutomation
	if slices.ContainsFunc(f.Log, func(l Line) bool { return l.Human != "" && l.Op != Confirm && l.Op != Quarantine }) {
		a.Methods = append(a.Methods, fyMethod{Method: "manual"})
	}
	for _, id := range runsOf(f) {
		r := f.Runs[id]
		i := slices.IndexFunc(f.Log, func(l Line) bool { return l.Run == id })
		if i < 0 {
			continue
		}
		l := f.Log[i]
		m := fyMethod{Method: "agent", Framework: "math2-agent, the role " + agentRole(l.Agent)}
		if r == nil {
			m.Models = slices.DeleteFunc([]string{l.Model}, func(s string) bool { return s == "" })
			m.Notes = fmt.Sprintf("The run %s used the prompt %s. Its summary is not in the run directory.", id, cmp.Or(l.Prompt, "that the log does not name"))
			a.Methods = append(a.Methods, m)
			continue
		}
		if !IsPerson(r.OnBehalfOf) {
			m.Method = "autonomous"
		}
		m.Models = r.Models
		var tools []string
		for _, k := range slices.Sorted(maps.Keys(r.Tools)) {
			tools = append(tools, k+" "+r.Tools[k])
		}
		tools = append(tools, r.Images...)
		if len(tools) > 0 {
			m.Tools = strings.Join(tools, ", ")
		}
		if r.Seconds > 0 || r.SpendUSD > 0 {
			m.Cost = &fyCost{SpendUSD: r.SpendUSD}
			if r.Seconds > 0 {
				m.Cost.WallTime = minutes(r.Seconds)
			}
		}
		m.Notes = fmt.Sprintf("The run %s used the prompt %s on behalf of %s. The outcome was %s.", id, r.Prompt, r.OnBehalfOf, r.Outcome)
		a.SpendUSD += r.SpendUSD
		a.Methods = append(a.Methods, m)
	}
	if len(a.Methods) == 0 {
		a.Methods = []fyMethod{{Method: "other"}}
		a.Notes = "The node has no provenance log, so the record does not say how the binding was made."
		return a
	}
	a.Notes = "Each method is the work of the people or one agent run, from the provenance log of the node and the run summaries. The full records of the runs are in the artefact store of math2 under the run ids."
	return a
}

// review returns the review status from the fidelity levels of the bindings (spec 04.7). F2 and F3 need reviews by people, thus they are peer-reviewed. F1 is the agreement of the machine checks, thus it is agent-reviewed.
func review(f Facts, items []formalItem) fyReview {
	r := fyReview{Status: "unchecked"}
	var notes []string
	best := ""
	for _, it := range items {
		b, ok := f.Build[it.decl]
		if !ok {
			continue
		}
		best = max(best, b.Fidelity)
		for _, p := range b.Reviewers {
			if !slices.Contains(r.Reviewers, p) {
				r.Reviewers = append(r.Reviewers, p)
			}
		}
		notes = append(notes, fmt.Sprintf("%s is at the fidelity level %s.", it.decl, b.Fidelity))
	}
	switch best {
	case "F2", "F3":
		r.Status = "peer-reviewed"
	case "F1":
		r.Status = "agent-reviewed"
	}
	if f.Build == nil {
		notes = append(notes, "There is no build, so the file does not give the fidelity levels.")
	}
	notes = append(notes, "The fidelity levels are those of the math2 specification, document 04.7.")
	r.Notes = strings.Join(notes, " ")
	return r
}
