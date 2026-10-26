package inventory

import (
	"slices"
	"strings"
)

// Rule is a decision that a curator approved once and that applies to many rows (corpus spec K4.4). Each outcome that a rule gives names the rule. The file schema/outcomes.v1.yaml of the math2 tree lists the rules, and rule I04 checks that each rule that a row names is in it.
type Rule struct {
	ID      string
	Outcome string
	Reason  string
	match   func(ms []*Mention) bool
}

// Rules returns the rules that Cluster applies to each new row, in order. The first rule that matches gives the outcome.
func Rules() []Rule { return rules }

var rules = []Rule{
	{ID: "person", Outcome: OutcomeOutOfScope, Reason: "a person, not an item of mathematics", match: each(proofWiki("Mathematician:"), wikidataClass("Q5"))},
	{ID: "book", Outcome: OutcomeOutOfScope, Reason: "a book or a journal, not an item of mathematics", match: each(proofWiki("Book:"), wikidataClass("Q47461344", "Q7725634", "Q5633421"))},
	{ID: "symbol", Outcome: OutcomeOutOfScope, Reason: "a notation: the notation register holds it (corpus spec K10.4)", match: each(proofWiki("Symbols:"))},
	{ID: "place", Outcome: OutcomeOutOfScope, Reason: "a country, not an item of mathematics", match: each(wikidataClass("Q6256", "Q3624078"))},
}

// each matches a row when each of its mentions passes one of the tests.
func each(tests ...func(m *Mention) bool) func(ms []*Mention) bool {
	return func(ms []*Mention) bool {
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
	var ms []*Mention
	for _, id := range r.Mentions {
		if m := byID[id]; m != nil {
			ms = append(ms, m)
		}
	}
	for _, rule := range rules {
		if rule.match(ms) {
			r.Outcome = &Outcome{Value: rule.Outcome, Reason: rule.Reason, Rule: rule.ID, By: by}
			return
		}
	}
}
