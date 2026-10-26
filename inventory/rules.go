package inventory

import "strings"

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
	{ID: "person", Outcome: OutcomeOutOfScope, Reason: "a person, not an item of mathematics", match: allProofWiki("Mathematician:")},
	{ID: "book", Outcome: OutcomeOutOfScope, Reason: "a book, not an item of mathematics", match: allProofWiki("Book:")},
	{ID: "symbol", Outcome: OutcomeOutOfScope, Reason: "a notation: the notation register holds it (corpus spec K10.4)", match: allProofWiki("Symbols:")},
}

// allProofWiki matches a row when each of its mentions names a ProofWiki page in the namespace, and no mention is of another kind of page.
func allProofWiki(ns string) func(ms []*Mention) bool {
	return func(ms []*Mention) bool {
		for _, m := range ms {
			ok := false
			for _, p := range m.Same["proofwiki"] {
				if strings.HasPrefix(p, ns) {
					ok = true
				}
			}
			if !ok {
				return false
			}
		}
		return len(ms) > 0
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
