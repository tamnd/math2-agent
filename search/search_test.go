package search_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tamnd/math2-agent/align"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/search"
)

// lines joins the lines of a canonical form, as M2Extract writes it.
func lines(ls ...string) string { return strings.Join(ls, "\n") + "\n" }

// records are the extraction records of the tests. The canonical forms are short versions of the forms of Mathlib, with the same heads.
var records = []lean.Record{
	{
		Decl: "Subgroup.card_subgroup_dvd_card", Kind: "theorem", Module: "Mathlib.GroupTheory.Coset.Card",
		StatementPP: "∀ {α : Type u_1} [inst : Group α] (s : Subgroup α), Nat.card ↥s ∣ Nat.card α",
		Type: lines("s s(u0)", "c Group u0", "b 0", "a 1 2", "c Subgroup u0", "b 1", "a 4 5", "c Dvd.dvd 0", "c Nat", "a 7 8", "c Nat.card u0", "b 0", "a 10 11",
			"a 9 12", "b 2", "a 10 14", "a 13 15", "p d 6 16", "p c 3 17", "p i 0 18"),
		UsesConstantsInStatement: []string{"Dvd.dvd", "Group", "Nat", "Nat.card", "Subgroup"},
		UsesDecls:                []string{"Subgroup.card_eq_card_quotient_mul_card_subgroup", "Dvd.intro"},
	},
	{
		Decl: "isCyclic_of_prime_card", Kind: "theorem", Module: "Mathlib.GroupTheory.SpecificGroups.Cyclic",
		StatementPP: "∀ {α : Type u} [inst : Group α] [inst_1 : Finite α] {p : ℕ} [hp : Fact (Nat.Prime p)], Nat.card α = p → IsCyclic α",
		Type: lines("s s(u0)", "c Group u0", "b 0", "a 1 2", "c Finite u0", "a 4 2", "c Nat", "c Fact", "c Nat.Prime", "b 0", "a 8 9", "a 7 10", "c Eq 1", "a 12 6",
			"c Nat.card u0", "a 14 2", "a 13 15", "a 16 9", "c IsCyclic u0", "a 18 2", "p d 17 19", "p c 11 20", "p i 6 21", "p c 5 22", "p c 3 23", "p i 0 24"),
		UsesConstantsInStatement: []string{"Eq", "Fact", "Finite", "Group", "IsCyclic", "Nat", "Nat.Prime", "Nat.card"},
		UsesDecls:                []string{"Subgroup.card_subgroup_dvd_card", "isCyclic_of_orderOf_eq_card"},
	},
	{
		Decl: "IsCyclic.exists_generator", Kind: "theorem", Module: "Mathlib.GroupTheory.SpecificGroups.Cyclic",
		StatementPP:              "∀ {α : Type u} [inst : Group α] [IsCyclic α], ∃ g, ∀ (x : α), x ∈ Subgroup.zpowers g",
		Type:                     lines("s s(u0)", "c Group u0", "b 0", "a 1 2", "c IsCyclic u0", "a 4 2", "c Exists u0", "a 6 2", "p c 5 7", "p c 3 8", "p i 0 9"),
		UsesConstantsInStatement: []string{"Exists", "Group", "IsCyclic", "Membership.mem", "Subgroup.zpowers"},
	},
}

var (
	once    sync.Once
	ixPath  string
	onceErr error
)

func loadNodes(t testing.TB) []*node.Node {
	t.Helper()
	nodes, err := node.Load(filepath.Join("..", "testdata", "nodes", "valid"))
	if err != nil {
		t.Fatal(err)
	}
	return nodes
}

func input(t testing.TB) search.Input {
	return search.Input{Nodes: loadNodes(t), Records: records, Aliases: map[string]string{"lagrange-theorem-groups": "Q529ES"}}
}

// open returns the index of the valid nodes of testdata. The tests share one file.
func open(t *testing.T) *search.Index {
	t.Helper()
	once.Do(func() {
		dir, err := os.MkdirTemp("", "m2-search")
		if err != nil {
			onceErr = err
			return
		}
		ixPath = filepath.Join(dir, "search.db")
		onceErr = search.Write(ixPath, input(t))
	})
	if onceErr != nil {
		t.Fatal(onceErr)
	}
	ix, err := search.Open(ixPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	return ix
}

func TestMain(m *testing.M) {
	code := m.Run()
	if ixPath != "" {
		_ = os.RemoveAll(filepath.Dir(ixPath))
	}
	os.Exit(code)
}

func run(t *testing.T, ix *search.Index, q search.Query) []search.Result {
	t.Helper()
	rs, err := ix.Search(q)
	if err != nil {
		t.Fatalf("Search(%q): %v", q.Text, err)
	}
	return rs
}

func keys(rs []search.Result) []string {
	var out []string
	for _, r := range rs {
		if r.Tag != "" {
			out = append(out, r.Tag)
		} else {
			out = append(out, "lean:"+r.Decl)
		}
	}
	return out
}

func hit(r search.Result, channel string) (search.Hit, bool) {
	i := slices.IndexFunc(r.Channels, func(h search.Hit) bool { return h.Channel == channel })
	if i < 0 {
		return search.Hit{}, false
	}
	return r.Channels[i], true
}

func TestIdentifier(t *testing.T) {
	ix := open(t)
	for q, want := range map[string]string{
		"Q529ES":                    "Q529ES",
		"lagrange-theorem-groups":   "Q529ES",
		"Q180967":                   "Q529ES", // the Wikidata item
		"m2://ext/wikidata/Q180967": "Q529ES",
		"m2://lean/Subgroup.card_subgroup_dvd_card": "Q529ES",
		"Subgroup.card_subgroup_dvd_card":           "Q529ES",
		"IsCyclic.exists_generator":                 "lean:IsCyclic.exists_generator",
	} {
		rs := run(t, ix, search.Query{Text: q})
		if len(rs) == 0 || keys(rs)[0] != want {
			t.Errorf("%s gives %v first, and it must give %s", q, keys(rs), want)
			continue
		}
		if _, ok := hit(rs[0], search.Identifier); !ok {
			t.Errorf("%s: the identifier resolver did not find %s: %+v", q, want, rs[0].Channels)
		}
	}
	// A book is the source of more than one node. The resolver gives each of them.
	rs := run(t, ix, search.Query{Text: "isbn:9780471433347", Channels: []string{search.Identifier}, K: 50})
	if len(rs) < 2 || !slices.Contains(keys(rs), "Q529ES") {
		t.Errorf("the book gives %v", keys(rs))
	}
	if _, err := ix.Search(search.Query{Text: "m2://"}); err == nil {
		t.Error("Search accepts a bad m2:// URI")
	}
}

func TestLexical(t *testing.T) {
	ix := open(t)
	rs := run(t, ix, search.Query{Text: "Lagrange's theorem", Channels: []string{search.Lexical}})
	if len(rs) == 0 || rs[0].Tag != "Q529ES" {
		t.Fatalf("the lexical channel gives %v", keys(rs))
	}
	if h, _ := hit(rs[0], search.Lexical); h.Rank == 0 || len(rs[0].Channels) != 1 {
		t.Errorf("the channels of the first result are %+v", rs[0].Channels)
	}
	// The statement ranks above the proofs of the same name.
	i := slices.Index(keys(rs), "ENS73X")
	if i < 1 {
		t.Errorf("the proof by cosets is at %d in %v", i, keys(rs))
	}
}

func TestFold(t *testing.T) {
	ix := open(t)
	rs := run(t, ix, search.Query{Text: "Proof of Lagrange's theorem by orbits", Channels: []string{search.Lexical}})
	if slices.Contains(keys(rs), "DTT8HP") {
		t.Fatalf("the retired duplicate is a result: %v", keys(rs))
	}
	i := slices.Index(keys(rs), "ENS73X")
	if i < 0 || !slices.Equal(rs[i].Folded, []string{"DTT8HP"}) {
		t.Errorf("the successor does not stand for the duplicate: %v, %+v", keys(rs), rs)
	}
	// A node that was retired because it is false, with no successor, is not a result, unless its identifier names it.
	if rs := run(t, ix, search.Query{Text: "Each group of even order is abelian", Channels: []string{search.Lexical}}); slices.Contains(keys(rs), "5FK5AA") {
		t.Errorf("the false node is a result: %v", keys(rs))
	}
	if rs := run(t, ix, search.Query{Text: "5FK5AA"}); len(rs) == 0 || rs[0].Tag != "5FK5AA" {
		t.Errorf("the tag of the false node gives %v", keys(rs))
	}
}

func TestFormula(t *testing.T) {
	ix := open(t)
	// Fermat's little theorem with other names for the variables.
	rs := run(t, ix, search.Query{Text: `$x^{q-1} \equiv 1 \pmod{q}$`, Channels: []string{search.Formula}})
	if len(rs) == 0 || rs[0].Tag != "X5X1EQ" {
		t.Fatalf("the formula channel gives %v", keys(rs))
	}
	if h, _ := hit(rs[0], search.Formula); h.Score < 1 {
		t.Errorf("the whole query must match, and the score is %v", h.Score)
	}
	if rs := run(t, ix, search.Query{Text: "a theorem about primes", Channels: []string{search.Formula}}); len(rs) != 0 {
		t.Errorf("a query in words gives formulas: %v", keys(rs))
	}
}

func TestFormal(t *testing.T) {
	ix := open(t)
	for q, want := range map[string]string{
		"Subgroup _ → Finite _ → IsCyclic _":  "BP30YM",
		"IsCyclic, Group, Fact":               "BP30YM",
		"Nat.card H ∣ Nat.card G":             "Q529ES",
		"[Group α] [IsCyclic α] ⊢ ∃ g, g = g": "lean:IsCyclic.exists_generator",
	} {
		rs := run(t, ix, search.Query{Text: q, Channels: []string{search.Formal}})
		if len(rs) == 0 || keys(rs)[0] != want {
			t.Errorf("%s gives %v, and it must give %s first", q, keys(rs), want)
			continue
		}
		if rs[0].Decl == "" {
			t.Errorf("%s: the result has no declaration", q)
		}
	}
	rs := run(t, ix, search.Query{Text: "IsCyclic.exists_generator", Channels: []string{search.Identifier}})
	if len(rs) != 1 || rs[0].Tag != "" || rs[0].Name != "IsCyclic.exists_generator" {
		t.Errorf("a declaration with no node gives %+v", rs)
	}
}

func TestEmbedding(t *testing.T) {
	ix := open(t)
	rs := run(t, ix, search.Query{Text: "if H is a subgroup of a finite group G, then |H| divides |G|", Channels: []string{search.Embedding}, K: 3})
	if !slices.Contains(keys(rs), "Q529ES") {
		t.Errorf("the embedding channel gives %v", keys(rs))
	}
}

func TestFusion(t *testing.T) {
	ix := open(t)
	rs := run(t, ix, search.Query{Text: "Lagrange subgroup divides"})
	if len(rs) == 0 || rs[0].Tag != "Q529ES" {
		t.Fatalf("the fusion gives %v", keys(rs))
	}
	if len(rs[0].Channels) < 2 {
		t.Errorf("only %+v found the first result", rs[0].Channels)
	}
	for i := 1; i < len(rs); i++ {
		if rs[i].Score > rs[i-1].Score {
			t.Errorf("the results are not in order of score: %v", keys(rs))
		}
	}
	if rs := run(t, ix, search.Query{Text: "Lagrange", Kinds: []string{"proof"}}); len(rs) == 0 || slices.ContainsFunc(rs, func(r search.Result) bool { return r.Kind != "proof" }) {
		t.Errorf("the kind filter gives %+v", rs)
	}
	plain := run(t, ix, search.Query{Text: "conjecture primes", Channels: []string{search.Lexical}, K: 50})
	open := run(t, ix, search.Query{Text: "conjecture primes", Channels: []string{search.Lexical}, K: 50, Open: true})
	at := func(rs []search.Result, tag string) int { return slices.Index(keys(rs), tag) }
	if at(open, "CXXMNB") > at(plain, "CXXMNB") {
		t.Errorf("the open facet does not raise the twin prime conjecture: %d and %d", at(plain, "CXXMNB"), at(open, "CXXMNB"))
	}
	for _, q := range []search.Query{{Text: " "}, {Text: "x", Channels: []string{"vectors"}}} {
		if _, err := ix.Search(q); err == nil {
			t.Errorf("Search(%+v) gives no error", q)
		}
	}
}

func TestPremises(t *testing.T) {
	ix := open(t)
	ps, err := ix.Premises(search.Goal{Goal: "G : Type\n[Group G]\nH : Subgroup G\n⊢ Nat.card H ∣ Nat.card G"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) == 0 || ps[0].Decl != "Subgroup.card_subgroup_dvd_card" || !slices.Equal(ps[0].Tags, []string{"Q529ES"}) {
		t.Fatalf("the premises are %+v", ps)
	}
	// The node of the goal uses N98ZA2 and E18G47, and Lean binds them to Subgroup and Nat.card. Those are not in the records, thus the dependency priors come from the near declarations.
	ps, err = ix.Premises(search.Goal{Goal: "⊢ IsCyclic G", Tag: "BP30YM"})
	if err != nil || len(ps) == 0 {
		t.Fatalf("Premises gives %+v, %v", ps, err)
	}
	if !slices.ContainsFunc(ps, func(p search.Premise) bool { return p.Decl == "isCyclic_of_orderOf_eq_card" }) {
		t.Errorf("the proof of a near declaration gives no premise: %+v", ps)
	}
	if _, err := ix.Premises(search.Goal{}); err == nil {
		t.Error("Premises accepts an empty goal")
	}
}

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	in := input(t)
	a, b := filepath.Join(dir, "a.db"), filepath.Join(dir, "b.db")
	slices.Reverse(in.Nodes)
	if err := search.Write(a, in); err != nil {
		t.Fatal(err)
	}
	slices.Reverse(in.Nodes)
	if err := search.Write(b, in); err != nil {
		t.Fatal(err)
	}
	da, _ := os.ReadFile(a)
	db, _ := os.ReadFile(b)
	if !bytes.Equal(da, db) {
		t.Error("two writes of the same input give different files")
	}
	if ix, err := search.Open(a, align.Hash{Dim: 1024}); err != nil {
		t.Errorf("the default embedder does not open the index: %v", err)
	} else {
		_ = ix.Close()
	}
	if _, err := search.Open(a, align.Hash{Dim: 64}); err == nil || !strings.Contains(err.Error(), "length 1024") {
		t.Errorf("Open with vectors of another length gives %v", err)
	}
	if _, err := search.Open(a, other{}); err == nil || !strings.Contains(err.Error(), "hash/1") {
		t.Errorf("Open with another embedder gives %v", err)
	}
	if _, err := search.Open(filepath.Join(dir, "none.db"), nil); err == nil {
		t.Error("Open reads a file that does not exist")
	}
}

type other struct{}

func (other) Name() string           { return "other/1" }
func (other) Embed(string) []float32 { return []float32{1} }
