package coverage_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/tamnd/math2-agent/coverage"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/msc"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
)

var update = flag.Bool("update", false, "write the golden files again")

// fixture returns a report of the small topic map in testdata, with four nodes, four records, a count of two classes and a skeleton of one class.
func fixture(t *testing.T) *coverage.Report {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "msc2020.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := msc.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var nodes []*node.Node
	for tag, front := range map[string]string{
		"S00001": "kind: statement\nname: Euclid's theorem\nmsc: [11A41, 11N05]\n",
		"C00001": "kind: conjecture\nname: The twin prime conjecture\nmsc: [11N05]\n",
		"R00001": "kind: statement\nname: A retired copy\nmsc: [11A41]\nretired: {date: 2027-03-20, reason: duplicate, successor: [S00001]}\n",
		"G00001": "kind: concept\nname: Finite group\nmsc: [20D99]\nsources:\n  - {book: 'isbn:9780471433347', locator: Section 1.1}\n",
	} {
		n, err := node.Parse("nodes/"+tag[:2]+"/"+tag+".md", []byte("---\ntag: "+tag+"\n"+front+"---\n\n## Statement\n\nText.\n"))
		if err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, n)
	}
	rec := func(codes string) ingest.Record {
		return ingest.Record{ID: codes, Meta: map[string]string{"msc": codes}}
	}
	in := coverage.Input{
		Map:   m,
		Nodes: nodes,
		Status: []status.Status{
			{Tag: "S00001", Evidence: status.Kernel, Fidelity: status.F2},
			{Tag: "C00001", Evidence: status.Open},
		},
		Records: map[string][]ingest.Record{
			"zbmath": {rec("11A41 11A05"), rec("11N05 20D60"), rec("20D60")},
			"arxiv":  {rec("11A41")},
		},
		Expected: &coverage.Expected{Source: "zbmath", Date: "2026-10-26", Counts: map[string]int{"11-XX": 4, "20-XX": 1}},
		Skeletons: []coverage.Skeleton{{Code: "20-XX", Title: "Groups", Items: []coverage.Item{
			{Name: "finite group", Kind: "concept", Node: "G00001"},
			{Name: "Sylow theorems", Kind: "statement"},
			{Name: "retired copy", Kind: "statement", Node: "R00001"},
			{Name: "Euclid's theorem", Kind: "statement", Node: "S00001"},
		}}},
	}
	return &coverage.Report{Name: "next", Rows: coverage.Compute(in), Expected: in.Expected}
}

func TestCompute(t *testing.T) {
	r := fixture(t)
	f := func(v float64) *float64 { return &v }
	want := map[string]coverage.Row{
		"11-XX": {Code: "11-XX", Label: "number theory", Level: 1, Records: 3, Recorded: 2, Expected: 4, Nodes: 2, Statements: 1, Conjectures: 1, E2: 1, E5: 1, C0: f(0.5), C1: f(0)},
		"11Axx": {Code: "11Axx", Label: "elementary number theory", Level: 2, Records: 2, Recorded: 1, Nodes: 1, Statements: 1, E2: 1, E5: 1},
		"11A41": {Code: "11A41", Label: "prime numbers", Level: 3, Records: 2, Recorded: 1, Nodes: 1, Statements: 1, E2: 1, E5: 1},
		"11N05": {Code: "11N05", Label: "distribution of prime numbers", Level: 3, Records: 1, Recorded: 1, Nodes: 2, Statements: 1, Conjectures: 1, E2: 1, E5: 1},
		"20-XX": {Code: "20-XX", Label: "groups and their generalizations", Level: 1, Records: 2, Recorded: 2, Expected: 1, Nodes: 1, Skeleton: 4, Mapped: 1, C0: f(1), C1: f(0), C2: f(0.25)},
		"20Dxx": {Code: "20Dxx", Label: "finite groups", Level: 2, Records: 2, Recorded: 2, Nodes: 1},
		"11-01": {Code: "11-01", Label: "textbooks on number theory", Level: 2},
	}
	if len(r.Rows) != 10 || r.Rows[0].Code != "11-XX" {
		t.Fatalf("Compute gives %d rows, from %s", len(r.Rows), r.Rows[0].Code)
	}
	for _, row := range r.Rows {
		if w, ok := want[row.Code]; ok && !reflect.DeepEqual(row, w) {
			t.Errorf("the row of %s is\n%+v\nand it must be\n%+v", row.Code, row, w)
		}
	}
}

func TestReport(t *testing.T) {
	r := fixture(t)
	for code, name := range map[string]string{"": "next.md", "11-XX": "next-11.md"} {
		data := r.Markdown(code)
		golden := filepath.Join("testdata", name)
		if *update {
			if err := os.WriteFile(golden, data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, want) {
			t.Errorf("Markdown(%q) gives\n%s\nand %s has\n%s", code, data, golden, want)
		}
	}

	root := t.TempDir()
	stale := filepath.Join(root, "reports", "coverage", "next", "99.md")
	if err := os.MkdirAll(filepath.Dir(stale), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	paths, err := r.Write(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(paths, " "); got != "reports/coverage/next.parquet reports/coverage/next.md reports/coverage/next/11.md reports/coverage/next/20.md" {
		t.Errorf("Write writes %s", got)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("Write keeps the old summary 99.md: %v", err)
	}
	back, err := parquet.ReadFile[coverage.Row](filepath.Join(root, "reports", "coverage", "next.parquet"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, r.Rows) {
		t.Errorf("the Parquet file gives\n%+v", back)
	}
	var a, b bytes.Buffer
	if err := r.WriteParquet(&a); err != nil {
		t.Fatal(err)
	}
	if err := fixture(t).WriteParquet(&b); err != nil || !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Errorf("two reports of the same input give other bytes, %v", err)
	}
}

func TestExpected(t *testing.T) {
	root := t.TempDir()
	e := &coverage.Expected{Source: "zbmath", URL: "https://api.zbmath.org/v1/document/_search?search_string=cc%3A11", Date: "2026-10-26", Counts: map[string]int{"20-XX": 1, "11-XX": 4}}
	if err := e.Write(root); err != nil {
		t.Fatal(err)
	}
	back, err := coverage.ReadExpected(root)
	if err != nil || !reflect.DeepEqual(back, e) {
		t.Errorf("ReadExpected gives %+v, %v", back, err)
	}
	path := filepath.Join(root, filepath.FromSlash(coverage.ExpectedFile))
	for _, data := range []string{"source: zbmath\ndate: 2026-10-26\ncounts: {11Axx: 4}\n", "source: zbmath\ncounts: {}\n", "source: zbmath\ndate: 2026-10-26\ntotal: 1\n"} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := coverage.ReadExpected(root); err == nil {
			t.Errorf("ReadExpected accepts %q", data)
		}
	}
}

func TestReadSkeletons(t *testing.T) {
	root := t.TempDir()
	if s, err := coverage.ReadSkeletons(root); err != nil || len(s) != 0 {
		t.Fatalf("a tree with no skeletons gives %v, %v", s, err)
	}
	dir := filepath.Join(root, "topics", "skeletons")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	write := func(name, data string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("20.yaml", `skeleton: 20-XX
title: Group theory
sources:
  wikipedia: {title: English Wikipedia, url: 'https://en.wikipedia.org/wiki/', retrieved: '2026-10-26'}
items:
  - {name: group, kind: concept, wikipedia: Group (mathematics), node: G5S1M9}
  - {name: Lagrange's theorem, kind: statement, decl: Subgroup.card_subgroup_dvd_card}
`)
	s, err := coverage.ReadSkeletons(root)
	if err != nil || len(s) != 1 || s[0].Code != "20-XX" || len(s[0].Items) != 2 || s[0].Items[0].Node != "G5S1M9" || s[0].Sources["wikipedia"].Retrieved != "2026-10-26" {
		t.Fatalf("ReadSkeletons gives %+v, %v", s, err)
	}
	for data, want := range map[string]string{
		"skeleton: 11-XX\ntitle: x\nitems: [{name: a, kind: concept}]\n":                           "is not the top-level code of the file 20.yaml",
		"skeleton: 20Axx\ntitle: x\nitems: [{name: a, kind: concept}]\n":                           "is not the top-level code",
		"skeleton: 20-XX\ntitle: x\n":                                                              "has no items",
		"skeleton: 20-XX\ntitle: x\nitems: [{name: a, kind: concept}, {name: A, kind: concept}]\n": "occurs two times",
		"skeleton: 20-XX\ntitle: x\nitems: [{name: a}]\n":                                          "has no name or no kind",
		"skeleton: 20-XX\ntitle: x\nitems: [{name: a, kind: concept, page: 3}]\n":                  "field page not found",
	} {
		write("20.yaml", data)
		if _, err := coverage.ReadSkeletons(root); err == nil || !strings.Contains(err.Error(), want) || !strings.HasPrefix(err.Error(), "topics/skeletons/20.yaml: ") {
			t.Errorf("the skeleton\n%s\ngives the error %v, and it must contain %q", data, err, want)
		}
	}
}
