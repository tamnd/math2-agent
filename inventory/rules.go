package inventory

import (
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/node"
)

// Rule is a decision that a curator approved once and that applies to many rows (corpus spec K4.4). Each outcome that a rule gives names the rule. The file schema/outcomes.v1.yaml of the math2 tree lists the rules, and rule I04 checks that each rule that a row names is in it.
type Rule struct {
	ID      string
	Outcome string
	Reason  string
	match   func(ms []*Mention, sig Signals) bool
}

// Rules returns the rules of Cluster, in order. Cluster applies the rules before the formal rules to each new row, and the first rule that matches gives the outcome. Then it applies the formal rules of corpus K4.5 to each row of a formal declaration that is open or formal_only.
func Rules() []Rule { return slices.Concat(rules, formalRules) }

var rules = []Rule{
	{ID: "person", Outcome: OutcomeOutOfScope, Reason: "a person, not an item of mathematics", match: each(proofWiki("Mathematician:"), wikidataClass("Q5"))},
	{ID: "book", Outcome: OutcomeOutOfScope, Reason: "a book or a journal, not an item of mathematics", match: each(proofWiki("Book:"), wikidataClass("Q47461344", "Q7725634", "Q5633421"))},
	{ID: "symbol", Outcome: OutcomeOutOfScope, Reason: "a notation: the notation register holds it (corpus spec K10.4)", match: each(proofWiki("Symbols:"))},
	{ID: "place", Outcome: OutcomeOutOfScope, Reason: "a country, not an item of mathematics", match: each(wikidataClass("Q6256", "Q3624078"))},
	{ID: "research-exercise", Outcome: OutcomeRecord, Reason: "an exercise of a paper, which becomes a node only when a node, a project or a curriculum asks for it (corpus arxiv AX8.6)", match: each(arxivKind("exercise"))},
	{ID: "research-context", Outcome: OutcomeOutOfScope, Reason: "a context item of a paper, such as a standing hypothesis, which is not an item of mathematics by itself (corpus arxiv AX8.6)", match: each(arxivKind("context"))},
	{ID: "research-default", Outcome: OutcomeRecord, Reason: "an item of a paper that no node asks for yet. The record keeps its edges in the record plane (corpus arxiv AX8.6)", match: each(arxivKind("statement", "definition", "example", "counterexample", "conjecture", "problem", "exercise"))},
	{ID: ResearchResults, Outcome: OutcomeRecord, Reason: "a result of the research layer that no node needs yet. It stays in the record plane until the graph needs it (spec 9.8, corpus spec K4.5)", match: each(func(m *Mention) bool { return researchResults[m.Source] }, arxivKind("statement", "definition", "example", "counterexample", "conjecture", "problem", "exercise"))},
	{ID: ResearchPaper, Outcome: OutcomeRecord, Reason: "a paper or a book of the record plane: the source of items, not an item (spec 9.8)", match: each(paper)},
}

// The identifiers of the rules of the research layer that this package names.
const (
	ResearchResults = "research-results"
	ResearchPaper   = "research-paper"
	ResearchAligned = "research-aligned"
)

// researchResults are the research sources other than arXiv whose records are results: the results that the arXiv adapter takes from the content plane, the registered results of Palomar and the statements of TheoremSearch.
var researchResults = map[string]bool{"arxiv_results": true, "palomar": true, "theoremsearch": true}

// paper reports whether a mention is a paper or a book: a document of zbMATH Open, or a paper of the metadata plane of arXiv, whose identifier is a record identifier and not an ax:// reference to an item.
func paper(m *Mention) bool {
	return m.Source == "zbmath" || m.Source == "arxiv" && strings.HasPrefix(m.ID, "m2://rec/arxiv/")
}

// researchRules are the rules whose outcome the rule research-aligned can replace: a record of the research layer becomes aligned when a node states it.
var researchRules = []string{"research-default", "research-exercise", ResearchResults, ResearchPaper, ResearchAligned}

// applyAligned gives the outcome aligned to a row of the research layer that a node states: a node has an aligned edge to a mention of the row (corpus spec K4.4). The target is the tags of the nodes. The row must be open, or have its outcome from a rule of researchRules. When no node states a row that was aligned by the rule, the row gets the outcome of the first rule that matches it again. It reports whether the outcome of the row changed.
func applyAligned(r *Row, byID map[string]*Mention, sig Signals, day, by string) bool {
	if r.Layer != Research || r.Outcome != nil && !slices.Contains(researchRules, r.Outcome.Rule) {
		return false
	}
	var tags []string
	for _, m := range mentionsOf(r, byID) {
		tags = append(tags, sig.Aligned[m.ID]...)
		if m.Locator != m.ID {
			tags = append(tags, sig.Aligned[m.Locator]...)
		}
	}
	slices.Sort(tags)
	tags = slices.Compact(tags)
	was := r.Outcome
	switch {
	case len(tags) > 0:
		if was != nil && was.Rule == ResearchAligned && slices.Equal(was.Target, tags) {
			return false
		}
		r.Outcome = &Outcome{Value: OutcomeAligned, Target: tags, Reason: "a node states the result and has an aligned edge to it (corpus spec K4.4)", Rule: ResearchAligned, By: by}
		r.History = append(r.History, Event{Op: "outcome", At: day, By: by, Note: "aligned to " + strings.Join(tags, ", ")})
		return true
	case was != nil && was.Rule == ResearchAligned:
		r.Outcome = nil
		applyRules(r, byID, by)
		r.History = append(r.History, Event{Op: "outcome", At: day, By: by, Note: "no node has an aligned edge to the result now"})
		return true
	}
	return false
}

// The identifiers of the formal rules of corpus K4.5. A row that a test selects stays open: it waits for a person or a run to write its node. The node outcome of the row then names the test. Each other row of a formal declaration gets the outcome of the rule formal-only.
const (
	FormalHuman = "formal-human"
	FormalMain  = "formal-main"
	FormalUsed  = "formal-used"
	FormalAsked = "formal-asked"
	FormalOnly  = "formal-only"
)

var formalRules = []Rule{
	{ID: FormalHuman, Outcome: OutcomeNode, Reason: "a mention from a human source is in the same row as the declaration (corpus spec K4.5)", match: withFormal(func(m *Mention, _ Signals) bool { return !IsFormal(m.Source) && !mainLists[m.Source] })},
	{ID: FormalMain, Outcome: OutcomeNode, Reason: "the library documents the declaration as a main result, in its module documentation or in a list such as the 100 theorems (corpus spec K4.5)", match: withFormal(func(m *Mention, sig Signals) bool { return mainLists[m.Source] || sig.Main[m.ID] })},
	{ID: FormalUsed, Outcome: OutcomeNode, Reason: "a node uses the declaration in a uses edge that a person wrote (corpus spec K4.5)", match: withFormal(func(m *Mention, sig Signals) bool { return sig.Used[m.ID] })},
	{ID: FormalAsked, Outcome: OutcomeNode, Reason: "a person asks for the declaration (corpus spec K4.5)", match: withFormal(func(m *Mention, sig Signals) bool { return sig.Asked[m.ID] })},
	{ID: FormalOnly, Outcome: OutcomeFormalOnly, Reason: "a declaration of a formal library that no person cites. It stays in the crosswalk, so that a binding and search can find it (corpus spec K4.5)", match: withFormal(func(*Mention, Signals) bool { return true })},
}

// mainLists are the sources that list the main results of a formal library, from the files of the library or from a project that tracks them. Formal Conjectures is a library and a list: each of its statements is a conjecture that the literature names.
var mainLists = map[string]bool{"mathlib_100": true, "mathlib_1000": true, "mathlib_undergrad": true, "mathlib_overview": true, "thousand_plus": true, "formal_conjectures": true}

// Signals are the facts outside the mentions that select a formal declaration for a node (corpus spec K4.5), and the aligned edges of the nodes. Each set holds mention identifiers. The zero Signals selects only by the mentions of the row.
type Signals struct {
	Main  map[string]bool // the declarations that the module documentation of their library names as main results
	Used  map[string]bool // the declarations that a node uses in a uses edge that a person wrote
	Asked map[string]bool // the declarations that a person asks for

	// Aligned gives the tags of the nodes that have an aligned edge to a record, by the record of the edge: a mention identifier or a locator such as ax://paper/2312.00752#3KH2. It is for the rule research-aligned.
	Aligned map[string][]string
}

// withFormal matches a row that has a mention of a formal library and a mention that passes the test. The test sees each mention of the row, so a test of a declaration also sees the mentions of the other sources.
func withFormal(test func(m *Mention, sig Signals) bool) func(ms []*Mention, sig Signals) bool {
	return func(ms []*Mention, sig Signals) bool {
		return slices.ContainsFunc(ms, func(m *Mention) bool { return IsFormal(m.Source) }) &&
			slices.ContainsFunc(ms, func(m *Mention) bool { return test(m, sig) })
	}
}

// arxivKind reports whether a mention is of the source arxiv and of one of the kinds. A mention of the kind unknown waits for the pass classify (corpus arxiv AX7.2).
func arxivKind(kinds ...string) func(m *Mention) bool {
	return func(m *Mention) bool { return m.Source == "arxiv" && slices.Contains(kinds, m.Kind) }
}

// each matches a row when each of its mentions passes one of the tests.
func each(tests ...func(m *Mention) bool) func(ms []*Mention, sig Signals) bool {
	return func(ms []*Mention, _ Signals) bool {
		for _, m := range ms {
			if !slices.ContainsFunc(tests, func(t func(*Mention) bool) bool { return t(m) }) {
				return false
			}
		}
		return len(ms) > 0
	}
}

// proofWiki reports whether a mention names a ProofWiki page in the namespace ns.
func proofWiki(ns string) func(m *Mention) bool {
	return func(m *Mention) bool {
		return slices.ContainsFunc(m.Same["proofwiki"], func(p string) bool { return strings.HasPrefix(p, ns) })
	}
}

// wikidataClass reports whether a mention is a Wikidata item of one of the classes, and of no class that gives a kind.
func wikidataClass(classes ...string) func(m *Mention) bool {
	return func(m *Mention) bool {
		if m.Source != "wikidata" || slices.ContainsFunc(m.Types, func(q string) bool { return classKinds[q] != "" }) {
			return false
		}
		return slices.ContainsFunc(m.Types, func(q string) bool { return slices.Contains(classes, q) })
	}
}

// applyRules gives the outcome of the first rule that matches the open row.
func applyRules(r *Row, byID map[string]*Mention, by string) {
	if r.Outcome != nil {
		return
	}
	ms := mentionsOf(r, byID)
	for _, rule := range rules {
		if rule.match(ms, Signals{}) {
			r.Outcome = &Outcome{Value: rule.Outcome, Reason: rule.Reason, Rule: rule.ID, By: by}
			return
		}
	}
}

// applyFormal applies the formal rules to a row that is open or formal_only. A row that a test selects stays open, or opens again when it was formal_only. A row of a formal declaration that no test selects gets the outcome formal_only. It reports whether the outcome of the row changed.
func applyFormal(r *Row, byID map[string]*Mention, sig Signals, day, by string) bool {
	only := r.Outcome != nil && r.Outcome.Rule == FormalOnly
	if r.Outcome != nil && !only {
		return false
	}
	ms := mentionsOf(r, byID)
	for _, rule := range formalRules {
		if !rule.match(ms, sig) {
			continue
		}
		switch {
		case rule.Outcome == OutcomeNode && only:
			r.Outcome = nil
			r.History = append(r.History, Event{Op: "outcome", At: day, By: by, Note: "open again, a node candidate by the rule " + rule.ID})
			return true
		case rule.Outcome == OutcomeFormalOnly && !only:
			r.Outcome = &Outcome{Value: rule.Outcome, Reason: rule.Reason, Rule: rule.ID, By: by}
			return true
		}
		return false
	}
	return false
}

// Selected returns the formal rule that selects the row of a formal declaration for a node, or "" when no rule selects it. The node outcome of the row names this rule.
func Selected(r *Row, ms []Mention, sig Signals) string {
	byID := map[string]*Mention{}
	for i := range ms {
		byID[ms[i].ID] = &ms[i]
	}
	in := mentionsOf(r, byID)
	for _, rule := range formalRules {
		if rule.Outcome == OutcomeNode && rule.match(in, sig) {
			return rule.ID
		}
	}
	return ""
}

// AlignedEdges gives the tags of the live nodes that have an aligned edge to each record, by the record of the edge, for Signals.Aligned.
func AlignedEdges(nodes []*node.Node) map[string][]string {
	out := map[string][]string{}
	for _, n := range nodes {
		if _, retired := n.Meta["retired"]; retired {
			continue
		}
		edges, _ := n.Meta["aligned"].([]any)
		for _, e := range edges {
			if m, ok := e.(map[string]any); ok {
				if rec, _ := m["record"].(string); rec != "" && !slices.Contains(out[rec], n.ID) {
					out[rec] = append(out[rec], n.ID)
				}
			}
		}
	}
	return out
}

// mentionsOf returns the mentions of the row that are in byID.
func mentionsOf(r *Row, byID map[string]*Mention) []*Mention {
	var ms []*Mention
	for _, id := range r.Mentions {
		if m := byID[id]; m != nil {
			ms = append(ms, m)
		}
	}
	return ms
}
