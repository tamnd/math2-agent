package inventory

import (
	"maps"
	"slices"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
)

// list gives a mention of an item of a list of Mathlib, such as the 100 theorems, that names the declaration.
func list(source, item, decl string) Mention {
	src := &ingest.Source{ID: source, Licence: "Apache-2.0", Class: ingest.Open}
	return FromRecords(src, "blake3:00", source+"/1", []ingest.Record{{ID: item, Class: ingest.Open, Meta: map[string]string{"mathlib": decl}}})[0]
}

// TestFormalRules runs the formal rules on a fixture library. Each declaration gets a test of corpus K4.5 or the outcome formal_only.
func TestFormalRules(t *testing.T) {
	ms := []Mention{
		lemma("Nat.Prime", "def", ""),
		wikidata("Q49008", "prime number", map[string]string{"mathlib": "Nat.Prime"}),
		lemma("Nat.exists_infinite_primes", "theorem", "blake3:1"),
		list("mathlib_100", "11", "Nat.exists_infinite_primes"),
		list("mathlib_100", "1", "Nat.add_comm"),
		lemma("Nat.add_comm", "theorem", "blake3:2"),
		wikidata("Q90000001", "a fixture item", map[string]string{"mathlib": "Nat.add_comm"}),
		lemma("Nat.Prime.two_le", "theorem", "blake3:3"),
		lemma("Nat.Coprime.gcd_eq_one", "theorem", "blake3:4"),
		lemma("Nat.primeFactorsList_unique", "theorem", "blake3:5"),
		lemma("Nat.succ_ne_zero_aux", "theorem", "blake3:6"),
		lemma("Nat.instDecidablePrime", "def", ""),
		{ID: "m2://rec/setmm/id#1", Source: "setmm", NativeID: "id", Kind: "statement", Class: "open", By: "setmm/1"},
		{ID: "m2://rec/formal_conjectures/Erdos1#1", Source: "formal_conjectures", NativeID: "Erdos1", Kind: "conjecture", Class: "open", By: "formal_conjectures/1"},
	}
	id := func(native string) string { return MentionID("mathlib", native, 1) }
	sig := Signals{
		Main:  map[string]bool{id("Nat.Prime.two_le"): true},
		Used:  map[string]bool{id("Nat.Coprime.gcd_eq_one"): true},
		Asked: map[string]bool{id("Nat.primeFactorsList_unique"): true},
	}
	res, err := Cluster(nil, ms, Options{Day: day, By: "test", Mint: counter(), Signals: sig})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Nat.Prime":                   FormalHuman,
		"Nat.exists_infinite_primes":  FormalMain,
		"Nat.add_comm":                FormalHuman, // the human mention comes first in the order of corpus K6.4
		"Nat.Prime.two_le":            FormalMain,
		"Nat.Coprime.gcd_eq_one":      FormalUsed,
		"Nat.primeFactorsList_unique": FormalAsked,
		"Nat.succ_ne_zero_aux":        FormalOnly,
		"Nat.instDecidablePrime":      FormalOnly,
		"id":                          FormalOnly,
		"Erdos1":                      FormalMain,
	}
	got := map[string]string{}
	for i := range res.Rows {
		r := &res.Rows[i]
		var decl string
		for _, m := range ms {
			if slices.Contains(r.Mentions, m.ID) && IsFormal(m.Source) {
				decl = m.NativeID
			}
		}
		switch rule := Selected(r, ms, sig); {
		case r.Outcome == nil && rule != "":
			got[decl] = rule
		case r.Outcome != nil && r.Outcome.Value == OutcomeFormalOnly && rule == "":
			got[decl] = r.Outcome.Rule
		default:
			t.Errorf("the row of %s has the outcome %+v and the test %q", decl, r.Outcome, rule)
		}
	}
	if !maps.Equal(got, want) {
		t.Errorf("the rules are %v, want %v", got, want)
	}
	if res.Formal != 3 {
		t.Errorf("the formal rules changed %d rows, want 3", res.Formal)
	}

	// A person now asks for an auxiliary lemma, so its row opens again. The other rows keep their outcomes.
	sig.Asked[id("Nat.succ_ne_zero_aux")] = true
	again, err := Cluster(res.Rows, ms, Options{Day: day, By: "test", Mint: counter(), Signals: sig})
	if err != nil {
		t.Fatal(err)
	}
	if again.Formal != 1 || len(again.Added) != 0 {
		t.Fatalf("the second run changed %d rows and added %d, want 1 and 0", again.Formal, len(again.Added))
	}
	for _, r := range again.Rows {
		if !slices.Contains(r.Mentions, id("Nat.succ_ne_zero_aux")) {
			continue
		}
		if r.Outcome != nil || Selected(&r, ms, sig) != FormalAsked || r.History[len(r.History)-1].Op != "outcome" {
			t.Errorf("the row of the lemma has the outcome %+v and the history %+v", r.Outcome, r.History)
		}
	}
}

// TestFormalRulesKeepDecisions checks that the formal rules do not change an outcome that a person or another rule gave.
func TestFormalRulesKeepDecisions(t *testing.T) {
	ms := []Mention{lemma("Nat.foo", "theorem", "blake3:1"), lemma("Nat.bar", "theorem", "blake3:2")}
	rows := []Row{
		{ID: "AB0000001", Mentions: []string{ms[0].ID}, Outcome: &Outcome{Value: OutcomeNode, Target: []string{"3C53TD"}, By: "tamnd"}},
		{ID: "AB0000002", Mentions: []string{ms[1].ID}, Outcome: &Outcome{Value: OutcomeOutOfScope, Reason: "a duplicate", By: "tamnd"}},
	}
	res, err := Cluster(rows, ms, Options{Day: day, By: "test", Mint: counter()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Formal != 0 || res.Rows[0].Outcome.Value != OutcomeNode || res.Rows[1].Outcome.Value != OutcomeOutOfScope {
		t.Errorf("the formal rules changed %d rows: %+v, %+v", res.Formal, res.Rows[0].Outcome, res.Rows[1].Outcome)
	}
}

// TestRulesHaveOutcomes checks that each rule has a unique identifier and an outcome of corpus spec K4.4.
func TestRulesHaveOutcomes(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range Rules() {
		if seen[r.ID] || !slices.Contains(Outcomes(), r.Outcome) || r.Reason == "" {
			t.Errorf("the rule %+v is repeated or has no outcome or reason", r)
		}
		seen[r.ID] = true
	}
}
