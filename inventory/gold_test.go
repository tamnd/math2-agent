package inventory

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
)

// lemma gives a mention of a Mathlib declaration of the kind with the type hash.
func lemma(decl, kind, hash string) Mention {
	src := &ingest.Source{ID: "mathlib", Licence: "Apache-2.0", Class: ingest.Open}
	return FromRecords(src, "blake3:00", "mathlib/1", []ingest.Record{{ID: decl, Kind: kind, Class: ingest.Open, Meta: map[string]string{"type_hash": hash}}})[0]
}

func TestFromRecordsTypeHash(t *testing.T) {
	if m := lemma("Nat.add_comm", "theorem", "blake3:aa"); m.TypeHash != "blake3:aa" {
		t.Errorf("a theorem has the type hash %q", m.TypeHash)
	}
	if m := lemma("Nat.add", "def", "blake3:aa"); m.TypeHash != "" {
		t.Errorf("a definition keeps the type hash %q", m.TypeHash)
	}
}

func TestClusterJoinsEqualStatements(t *testing.T) {
	ms := []Mention{
		lemma("Nat.exists_infinite_primes", "theorem", "blake3:primes"),
		lemma("Nat.exists_prime_gt", "theorem", "blake3:primes"),
		lemma("Nat.succ", "def", "blake3:nat-nat"),
		lemma("Nat.pred", "def", "blake3:nat-nat"),
		lemma("Nat.prime_two", "theorem", "blake3:two"),
	}
	// A wikidata item names one of the two lemmas, so pass 1 joins it and pass 2 joins the other lemma.
	ms = append(ms, wikidata("Q290", "Euclid's theorem", map[string]string{"mathlib": "Nat.exists_infinite_primes"}))
	res, err := Cluster(nil, ms, Options{Day: day, By: "test", Mint: counter()})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 4 || res.Equivalent != 1 {
		t.Fatalf("got %d rows and %d joins of pass 2, want 4 and 1", len(res.Rows), res.Equivalent)
	}
	for _, r := range res.Rows {
		if r.Name == "Euclid's theorem" && len(r.Mentions) != 3 {
			t.Errorf("the row of Euclid's theorem has the mentions %v", r.Mentions)
		}
	}
	again, err := Cluster(nil, ms, Options{Day: day, By: "test", Mint: counter()})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(res.Rows, again.Rows, func(a, b Row) bool { return a.ID == b.ID && slices.Equal(a.Mentions, b.Mentions) }) {
		t.Error("two runs on the same mentions give different rows")
	}
}

func TestClusterMergeNamesThePass(t *testing.T) {
	a := lemma("Nat.exists_infinite_primes", "theorem", "blake3:primes")
	b := lemma("Nat.exists_prime_gt", "theorem", "")
	res, err := Cluster(nil, []Mention{a, b}, Options{Day: day, By: "test", Mint: counter()})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(res.Rows))
	}
	// A new snapshot of Mathlib gives the hash of the second lemma, so the two rows are one item.
	b.TypeHash = "blake3:primes"
	again, err := Cluster(res.Rows, []Mention{a, b}, Options{Day: day, By: "test", Mint: counter()})
	if err != nil {
		t.Fatal(err)
	}
	if again.Merged != 1 {
		t.Fatalf("merged %d rows, want 1", again.Merged)
	}
	for _, r := range again.Rows {
		if r.Outcome != nil && (r.Outcome.Rule != "equivalence" || !strings.Contains(r.History[len(r.History)-1].Note, "an equal formal statement")) {
			t.Errorf("the merged row has the outcome %+v and the history %+v", r.Outcome, r.History)
		}
	}
}

func TestMeasure(t *testing.T) {
	ms := []Mention{
		lemma("A", "theorem", "blake3:1"),
		lemma("B", "theorem", "blake3:1"),
		lemma("C", "theorem", "blake3:2"),
		wikidata("Q1", "x", map[string]string{"mathlib": "A"}),
		wikidata("Q2", "y", map[string]string{"mathlib": "C"}),
	}
	id := func(s string) string {
		return ms[slices.IndexFunc(ms, func(m Mention) bool { return m.NativeID == s })].ID
	}
	gold := []Pair{
		{A: id("A"), B: id("Q1"), Same: true, Layer: Formal},     // pass 1 joins it
		{A: id("A"), B: id("B"), Same: true, Layer: Formal},      // pass 2 joins it
		{A: id("B"), B: id("C"), Same: true, Layer: Formal},      // a false split
		{A: id("Q1"), B: id("Q2"), Same: false, Layer: Textbook}, // a true negative
		{A: id("C"), B: id("Q2"), Same: false, Layer: Textbook},  // a false merge by pass 1
		{A: id("A"), B: "m2://rec/gone/x#1", Same: true, Layer: Formal},
	}
	got := map[string]Score{}
	for _, s := range Measure(ms, gold) {
		got[s.Pass+" "+s.Layer] = s
	}
	if len(got) != 9 {
		t.Fatalf("got the scores %v", got)
	}
	for k, want := range map[string][5]int{
		"identifiers formal":   {1, 0, 2, 0, 1},
		"hash formal":          {1, 0, 2, 0, 1},
		"equivalence formal":   {2, 0, 1, 0, 1},
		"identifiers textbook": {0, 1, 0, 1, 0},
		"equivalence all":      {2, 1, 1, 1, 1},
	} {
		s := got[k]
		if have := [5]int{s.TruePos, s.FalsePos, s.FalseNeg, s.TrueNeg, s.Missing}; have != want {
			t.Errorf("%s: got %v, want %v", k, have, want)
		}
	}
	if s := got["equivalence all"]; *s.Precision != 2.0/3 || *s.Recall != 2.0/3 {
		t.Errorf("the precision and recall are %v, %v", *s.Precision, *s.Recall)
	}
	if s := got["identifiers textbook"]; s.Recall != nil || *s.Precision != 0 {
		t.Errorf("the textbook score is %+v", s)
	}
}

func TestReadGold(t *testing.T) {
	root := t.TempDir()
	if g, err := ReadGold(root); err != nil || g != nil {
		t.Fatalf("a tree with no gold set gives %v, %v", g, err)
	}
	write := func(s string) {
		path := filepath.Join(root, filepath.FromSlash(GoldFile))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	good := `{"a":"m2://rec/x/1#1","b":"m2://rec/y/1#1","same":true,"layer":"formal","by":"tamnd","on":"2026-10-27"}`
	write(good + "\n\n")
	if g, err := ReadGold(root); err != nil || len(g) != 1 || !g[0].Same {
		t.Fatalf("got %v, %v", g, err)
	}
	for _, c := range []struct{ file, want string }{
		{good + "\n" + `{"a":"m2://rec/y/1#1","b":"m2://rec/x/1#1","same":false,"layer":"formal","by":"tamnd","on":"2026-10-27"}`, "also on line 1"},
		{`{"a":"m2://rec/x/1#1","b":"m2://rec/x/1#1","same":true,"layer":"formal","by":"tamnd","on":"2026-10-27"}`, "twice"},
		{`{"a":"m2://rec/x/1#1","b":"m2://rec/y/1#1","same":true,"layer":"model","by":"tamnd","on":"2026-10-27"}`, "layer"},
		{`{"a":"m2://rec/x/1#1","b":"m2://rec/y/1#1","same":true,"layer":"formal","on":"2026-10-27"}`, "curator"},
		{`{"a":"m2://rec/x/1#1","b":"m2://rec/y/1#1","same":true,"layer":"formal","by":"tamnd","on":"27/10/2026"}`, "YYYY-MM-DD"},
		{`{"a":"m2://rec/x/1#1","same":true,"layer":"formal","by":"tamnd","on":"2026-10-27"}`, "needs the mentions"},
		{`{"a":"m2://rec/x/1#1","b":"m2://rec/y/1#1","same":true,"layer":"formal","by":"tamnd","on":"2026-10-27","why":"x"}`, "unknown field"},
	} {
		write(c.file)
		if _, err := ReadGold(root); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want an error with %q", c.file, err, c.want)
		}
	}
}
