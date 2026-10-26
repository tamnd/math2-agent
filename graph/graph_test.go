package graph

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/tamnd/math2-agent/node"
)

// parse makes nodes from front matter. Each item is "ID kind" and then the other lines of the front matter.
func parse(t *testing.T, items ...string) []*node.Node {
	t.Helper()
	var out []*node.Node
	for _, it := range items {
		head, rest, _ := strings.Cut(it, "\n")
		id, kind, _ := strings.Cut(head, " ")
		src := "---\ntag: " + id + "\nkind: " + kind + "\nname: " + id + "\n" + rest + "\n---\n\nText.\n"
		n, err := node.Parse("nodes/"+id[:3]+"/"+id+".md", []byte(src))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	return out
}

func loadSeed(t *testing.T) []*node.Node {
	t.Helper()
	nodes, err := node.Load("testdata/seed")
	if err != nil {
		t.Fatal(err)
	}
	return nodes
}

func TestSeed(t *testing.T) {
	nodes := loadSeed(t)
	g := Build(nodes)
	var got []string
	for _, e := range g.Edges {
		got = append(got, e.Type+" "+e.From+" "+e.To+" "+e.System+" "+e.Label)
	}
	wantEdges := []string{
		"source 3C53TD isbn:9780471433347 book Section 3.2, Theorem 8",
		"source G5S1M9 isbn:9780471433347 book Section 1.1",
		"uses 3C53TD G5S1M9  ",
	}
	if !slices.Equal(got, wantEdges) {
		t.Errorf("edges:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(wantEdges, "\n"))
	}

	// The same nodes in a different order give the same hash.
	slices.Reverse(nodes)
	if h := Build(nodes).Hash(); h != g.Hash() {
		t.Errorf("hash after a reorder = %s, want %s", h, g.Hash())
	}
	golden := filepath.Join("testdata", "seed.hash")
	if *update {
		if err := os.WriteFile(golden, []byte(g.Hash()+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if h := strings.TrimSpace(string(b)); h != g.Hash() {
		t.Errorf("hash = %s, want %s from %s", g.Hash(), h, golden)
	}
}

func TestHashChanges(t *testing.T) {
	a := Build(parse(t, "1A2B3C concept", "2A2B3C statement\nuses: [1A2B3C]"))
	b := Build(parse(t, "1A2B3C concept", "2A2B3C statement\nuses: [3A2B3C]"))
	if a.Hash() == b.Hash() {
		t.Error("two graphs with different edges have the same hash")
	}
	if !strings.HasPrefix(a.Hash(), node.HashPrefix) || len(a.Hash()) != len(node.HashPrefix)+64 {
		t.Errorf("hash %q does not have the form blake3:<64 hex>", a.Hash())
	}
}

func TestEdges(t *testing.T) {
	g := Build(parse(t,
		`1A0000 statement
role: theorem
uses: [1B0000]
proofs: [1C0000]
implies: [{target: 1D0000, via: 1C0000}]
resolves: [{target: 1R0000, outcome: proved}]
generalizes: [{target: 1F0000, map: "n = 2"}]
bounds: [{target: 1G0000, side: lower, value: "n^2", via: 1C0000}]
about: [1H0000]
topics: [1J0000]
formal: [{system: lean4, library: mathlib, decl: Foo.bar, match: exact}]
aligned: [{record: "ax://paper/2106.09685#AB12", match: weaker}]
same_as: {wikidata: Q1, oeis: null}
close_to: [{system: wikipedia, id: Foo, note: wider}]
analogous_to: [{target: 1K0000, source: {doi: "10.1/x", locator: p. 3}}]`,
		`1C0000 proof
proves: 1A0000
methods: [1M0000]`,
		`1N0000 concept
specializes: [1P0000]
retired: {date: 2026-01-01, reason: merged, successor: [1Q0000]}`,
	))
	var got []string
	for _, e := range g.Edges {
		got = append(got, strings.Join([]string{e.Type, e.From, e.To, e.Via, e.System, e.Label}, "|"))
	}
	want := []string{
		"about|1A0000|1H0000|||",
		"aligned|ax://paper/2106.09685#AB12|1A0000|||weaker",
		"analogous_to|1A0000|1K0000||doi|10.1/x",
		"bounds|1A0000|1G0000|1C0000||lower n^2",
		"close_to|1A0000|Foo||wikipedia|",
		"formalizes|Foo.bar|1A0000||lean4/mathlib|exact",
		"generalizes|1A0000|1F0000|||n = 2",
		"implies|1A0000|1D0000|1C0000||",
		"method|1C0000|1M0000|||",
		"part_of|1A0000|1J0000|||",
		"proves|1C0000|1A0000|||",
		"resolves|1A0000|1R0000|||proved",
		"same_as|1A0000|Q1||wikidata|",
		"specializes|1N0000|1P0000|||",
		"successor|1N0000|1Q0000|||",
		"uses|1A0000|1B0000|||",
	}
	if !slices.Equal(got, want) {
		t.Errorf("edges:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if n, _ := g.Node("1N0000"); !n.Retired {
		t.Error("1N0000 is not retired")
	}
	if es := g.In(Proves, "1A0000"); len(es) != 1 || es[0].From != "1C0000" {
		t.Errorf("In(proves, 1A0000) = %v", es)
	}
	if es := g.Out(Uses, "1A0000"); len(es) != 1 || es[0].To != "1B0000" {
		t.Errorf("Out(uses, 1A0000) = %v", es)
	}
	for _, typ := range Types() {
		if Family(typ) == "" {
			t.Errorf("edge type %s has no family", typ)
		}
	}
	if len(Types()) != 23 {
		t.Errorf("%d edge types, want 23", len(Types()))
	}
}

// alternatives is a statement S with two proofs. Proof P1 is shallow and uses three leaves. Proof P2 is deep and uses one chain of two nodes.
var alternatives = []string{
	"5S0000 statement\nproofs: [5P1000, 5P2000]",
	"5P1000 proof\nproves: 5S0000\nuses: [5A0000, 5B0000, 5C0000]",
	"5P2000 proof\nproves: 5S0000\nuses: [5D0000]",
	"5D0000 definition\nuses: [5F0000]",
	"5A0000 concept", "5B0000 concept", "5C0000 concept", "5F0000 concept",
}

func TestClosure(t *testing.T) {
	g := Build(parse(t, alternatives...))
	deep, ok := g.Closure("5S0000", MinDepth)
	if !ok {
		t.Fatal("no closure")
	}
	if !slices.Equal(deep.Proofs, []string{"5P1000"}) || deep.Depth != 2 || len(deep.Nodes) != 5 {
		t.Errorf("min-depth closure = %+v", deep)
	}
	small, _ := g.Closure("5S0000", MinSize)
	if !slices.Equal(small.Proofs, []string{"5P2000"}) || small.Depth != 3 || !slices.Equal(small.Nodes, []string{"5D0000", "5F0000", "5P2000", "5S0000"}) {
		t.Errorf("min-size closure = %+v", small)
	}
	if leaf, _ := g.Closure("5A0000", MinDepth); leaf.Depth != 0 || len(leaf.Nodes) != 1 {
		t.Errorf("closure of a leaf = %+v", leaf)
	}
	if _, ok := g.Closure("ZZZZZZ", MinDepth); ok {
		t.Error("closure of a tag that is not in the graph")
	}

	// A proof that goes around a cycle is not an alternative.
	g = Build(parse(t,
		"6S0000 statement\nproofs: [6P1000, 6P2000]",
		"6P1000 proof\nproves: 6S0000\nuses: [6S0000]",
		"6P2000 proof\nproves: 6S0000\nuses: [6A0000, 6B0000]",
		"6A0000 concept\nuses: [6B0000]", "6B0000 concept",
	))
	if alt, _ := g.Closure("6S0000", MinDepth); !slices.Equal(alt.Proofs, []string{"6P2000"}) || alt.Depth != 3 {
		t.Errorf("closure with a circular proof = %+v", alt)
	}
	g = Build(parse(t, "7A0000 concept\nuses: [7B0000]", "7B0000 concept\nuses: [7A0000]"))
	if _, ok := g.Closure("7A0000", MinDepth); ok {
		t.Error("a closure for a node on a cycle")
	}
}

func TestCycles(t *testing.T) {
	for _, tc := range []struct {
		name  string
		nodes []string
		want  []string
	}{
		{"none", alternatives, nil},
		{"uses", []string{"8A0000 concept\nuses: [8B0000]", "8B0000 definition\nuses: [8C0000]", "8C0000 concept\nuses: [8A0000]"}, []string{"uses 8A0000 8B0000 8C0000"}},
		{"self", []string{"8A0000 concept\nuses: [8A0000]"}, []string{"uses 8A0000"}},
		{"one proof", []string{"9S0000 statement\nproofs: [9P0000]", "9P0000 proof\nproves: 9S0000\nuses: [9T0000]", "9T0000 statement\nuses: [9S0000]"}, []string{"uses 9P0000 9T0000 9S0000"}},
		{"two proofs", []string{
			"9A0000 statement\nproofs: [9P1000]", "9P1000 proof\nproves: 9A0000\nuses: [9B0000]",
			"9B0000 statement\nproofs: [9P2000]", "9P2000 proof\nproves: 9B0000\nuses: [9A0000]",
		}, nil},
		{"specializes", []string{"8A0000 concept\nspecializes: [8B0000]", "8B0000 concept\nspecializes: [8A0000]"}, []string{"specializes 8A0000 8B0000"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, c := range Build(parse(t, tc.nodes...)).Cycles() {
				got = append(got, c.Type+" "+strings.Join(c.Nodes, " "))
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("cycles = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCSR(t *testing.T) {
	c := Build(parse(t, alternatives...)).CSR(Uses)
	i, ok := c.Index("5P1000")
	if !ok {
		t.Fatal("5P1000 is not in the CSR")
	}
	var got []string
	for _, j := range c.Out(i) {
		got = append(got, c.IDs[j])
	}
	if !slices.Equal(got, []string{"5A0000", "5B0000", "5C0000"}) {
		t.Errorf("out(5P1000) = %v", got)
	}
	if len(c.Offsets) != len(c.IDs)+1 || int(c.Offsets[len(c.IDs)]) != len(c.Targets) || len(c.Targets) != 5 {
		t.Errorf("csr has %d ids, %d offsets and %d targets", len(c.IDs), len(c.Offsets), len(c.Targets))
	}
}

func TestParquet(t *testing.T) {
	g := Build(loadSeed(t))
	a, b := t.TempDir(), t.TempDir()
	files, err := g.WriteParquet(a)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"edges/conceptual.parquet", "edges/logical.parquet", "edges/record.parquet", "nodes/concept.parquet", "nodes/statement.parquet"}
	if !slices.Equal(files, want) {
		t.Errorf("files = %v, want %v", files, want)
	}
	if _, err := Build(loadSeed(t)).WriteParquet(b); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		x, _ := os.ReadFile(filepath.Join(a, f))
		y, _ := os.ReadFile(filepath.Join(b, f))
		if !bytes.Equal(x, y) {
			t.Errorf("%s is different in a second write", f)
		}
	}
	rows, err := parquet.ReadFile[Edge](filepath.Join(a, "edges", "record.parquet"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].From != "3C53TD" || rows[0].Label != "Section 3.2, Theorem 8" {
		t.Errorf("record edges = %+v", rows)
	}
}

func TestNew(t *testing.T) {
	g := Build(loadSeed(t))
	rev := slices.Clone(g.Nodes)
	slices.Reverse(rev)
	h := New(rev, append(slices.Clone(g.Edges), g.Edges...))
	if h.Hash() != g.Hash() {
		t.Errorf("New gives the hash %s, and Build gives %s", h.Hash(), g.Hash())
	}
	if _, ok := h.Node("3C53TD"); !ok {
		t.Error("the graph of New has no node 3C53TD")
	}
}

func TestQueriesUnknown(t *testing.T) {
	g := Build(parse(t, "5S0000 statement\nuses: [5T0000]"))
	if got := g.Needs("5T0000"); got != nil {
		t.Errorf("Needs of a tag that is not a node gives %v", got)
	}
	if got := g.Dependents("5Z0000"); got != nil {
		t.Errorf("Dependents of a tag that is not in the graph gives %v", got)
	}
	if _, ok := g.Path("5S0000", "5Z0000"); ok {
		t.Error("Path finds a path to a tag that is not in the graph")
	}
	if p, ok := g.Path("5S0000", "5S0000"); !ok || len(p) != 1 {
		t.Errorf("Path from a node to itself gives %v, %v", p, ok)
	}
}
