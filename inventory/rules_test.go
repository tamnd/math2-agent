package inventory

import (
	"maps"
	"slices"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/node"
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

// TestResearchRules runs the research rules on a fixture of the research layer. A result becomes aligned when a node has an aligned edge to it, and a record again when the edge is gone.
func TestResearchRules(t *testing.T) {
	m := func(source, native, kind string) Mention {
		return Mention{ID: MentionID(source, native, 1), Source: source, NativeID: native, Kind: kind, Class: "open", By: source + "/1"}
	}
	ms := []Mention{
		m("arxiv_results", "2401.00001/thm1", "statement"),
		m("palomar", "p1", "statement"),
		m("theoremsearch", "t1", "statement"),
		m("zbmath", "1234.56789", Unknown),
	}
	ms[1].Locator = "arXiv:2401.00002"
	res, err := Cluster(nil, ms, Options{Day: day, By: "test", Mint: counter()})
	if err != nil {
		t.Fatal(err)
	}
	rule := func(rows []Row) map[string]string {
		out := map[string]string{}
		for _, r := range rows {
			if r.Layer != Research || r.Outcome == nil {
				t.Errorf("the row %+v is not a decided row of the research layer", r)
				continue
			}
			out[r.Mentions[0]] = r.Outcome.Rule
		}
		return out
	}
	want := map[string]string{ms[0].ID: ResearchResults, ms[1].ID: ResearchResults, ms[2].ID: ResearchResults, ms[3].ID: ResearchPaper}
	if got := rule(res.Rows); !maps.Equal(got, want) {
		t.Errorf("the rules are %v, want %v", got, want)
	}

	// Two nodes state the first result, and one node states the second through its locator.
	sig := Signals{Aligned: map[string][]string{ms[0].ID: {"PRIME0000B", "PRIME0000A"}, "arXiv:2401.00002": {"PNT000000A"}}}
	again, err := Cluster(res.Rows, ms, Options{Day: day, By: "test", Mint: counter(), Signals: sig})
	if err != nil {
		t.Fatal(err)
	}
	want[ms[0].ID], want[ms[1].ID] = ResearchAligned, ResearchAligned
	if got := rule(again.Rows); again.Aligned != 2 || !maps.Equal(got, want) {
		t.Errorf("the rules are %v and %d rows changed, want %v and 2", got, again.Aligned, want)
	}
	for _, r := range again.Rows {
		if r.Mentions[0] == ms[0].ID && (r.Outcome.Value != OutcomeAligned || !slices.Equal(r.Outcome.Target, []string{"PRIME0000A", "PRIME0000B"})) {
			t.Errorf("the aligned row has the outcome %+v", r.Outcome)
		}
	}
	same, err := Cluster(again.Rows, ms, Options{Day: day, By: "test", Mint: counter(), Signals: sig})
	if err != nil || same.Aligned != 0 {
		t.Errorf("a run with the same edges changed %d rows: %v", same.Aligned, err)
	}

	// The edges are gone, so the rows are records again.
	back, err := Cluster(again.Rows, ms, Options{Day: day, By: "test", Mint: counter()})
	if err != nil {
		t.Fatal(err)
	}
	want[ms[0].ID], want[ms[1].ID] = ResearchResults, ResearchResults
	if got := rule(back.Rows); back.Aligned != 2 || !maps.Equal(got, want) {
		t.Errorf("the rules are %v and %d rows changed, want %v and 2", got, back.Aligned, want)
	}
}

// TestResearchRulesKeepDecisions checks that an aligned edge does not change an outcome that a person gave.
func TestResearchRulesKeepDecisions(t *testing.T) {
	ms := []Mention{{ID: MentionID("palomar", "p1", 1), Source: "palomar", NativeID: "p1", Kind: "statement", Class: "open", By: "palomar/1"}}
	rows := []Row{{ID: "AB0000001", Mentions: []string{ms[0].ID}, Layer: Research, Outcome: &Outcome{Value: OutcomeOutOfScope, Reason: "not mathematics", By: "tamnd"}}}
	res, err := Cluster(rows, ms, Options{Day: day, By: "test", Mint: counter(), Signals: Signals{Aligned: map[string][]string{ms[0].ID: {"PRIME0000A"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Aligned != 0 || res.Rows[0].Outcome.Value != OutcomeOutOfScope {
		t.Errorf("the rule research-aligned changed the outcome %+v", res.Rows[0].Outcome)
	}
}

func TestAlignedEdges(t *testing.T) {
	edge := func(rec string) map[string]any { return map[string]any{"record": rec} }
	nodes := []*node.Node{
		{ID: "PRIME0000A", Meta: map[string]any{"aligned": []any{edge("arXiv:2401.00001"), edge("m2://rec/palomar/p1#1"), edge("arXiv:2401.00001")}}},
		{ID: "PNT000000A", Meta: map[string]any{"aligned": []any{edge("arXiv:2401.00001"), map[string]any{"note": "no record"}}}},
		{ID: "OLD000000A", Meta: map[string]any{"retired": "2026-01-01", "aligned": []any{edge("m2://rec/palomar/p2#1")}}},
		{ID: "NONE00000A", Meta: map[string]any{}},
	}
	want := map[string][]string{"arXiv:2401.00001": {"PRIME0000A", "PNT000000A"}, "m2://rec/palomar/p1#1": {"PRIME0000A"}}
	if got := AlignedEdges(nodes); !maps.EqualFunc(got, want, slices.Equal) {
		t.Errorf("the edges are %v, want %v", got, want)
	}
}

func TestResearchKinds(t *testing.T) {
	for _, c := range []struct{ source, kind, want string }{
		{"arxiv_results", "definition", "definition"},
		{"arxiv_results", "lemma", Unknown},
		{"palomar", "result", "statement"},
		{"palomar", "proof", Unknown},
	} {
		src := &ingest.Source{ID: c.source, Licence: "CC-BY-4.0", Class: ingest.Open}
		got := FromRecords(src, "blake3:00", c.source+"/1", []ingest.Record{{ID: "1", Kind: c.kind, Class: ingest.Open}})[0].Kind
		if got != c.want {
			t.Errorf("a %s record of the kind %s has the kind %s, want %s", c.source, c.kind, got, c.want)
		}
	}
	rows := []Row{{ID: "A", Layer: Research}, {ID: "B", Layer: Research, Outcome: &Outcome{Value: OutcomeRecord}}, {ID: "C", Layer: Textbook}}
	if s := Summarize(rows, nil, day); !maps.Equal(s.OpenBy, map[string]int{Research: 1, Textbook: 1}) {
		t.Errorf("the open counts are %v", s.OpenBy)
	}
}
