package align_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/align"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/node"
)

// write writes the file at the path in the tree at root.
func write(t *testing.T, root, path, data string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// records writes the records of a source in the tree at root.
func records(t *testing.T, root, source string, recs ...ingest.Record) {
	t.Helper()
	var b strings.Builder
	for _, r := range recs {
		if r.Class == "" {
			r.Class = ingest.Open
		}
		j, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(j)
		b.WriteByte('\n')
	}
	write(t, root, "build/records/"+source+".jsonl", b.String())
}

// nodeFile returns a node file with the front matter lines and a statement.
func nodeFile(tag, front, statement string) string {
	return "---\ntag: " + tag + "\nkind: statement\nrole: theorem\n" + front + "---\n\n## Statement\n\n" + statement + "\n"
}

// index makes the index of a few records of a source, in the words of the test.
func index(t *testing.T) *align.Index {
	t.Helper()
	x, err := align.NewIndex("wikidata", []ingest.Record{
		{ID: "Q1", Title: "theorem of the two sets"},
		{ID: "Q2", Title: "group", Meta: map[string]string{"nlab": "group", "mathlib": "Group"}},
		{ID: "Q3", Title: "A-group"},
		{ID: "Q4", Title: "ring", Meta: map[string]string{"oeis": "A000001|A000002"}},
		{ID: "Q5", Title: "lemma of the cycle"},
		{ID: "Q6", Title: "lemma of the cycle"},
		{ID: "Q7", Title: "a sum of two squares", Text: "Each prime $p \\equiv 1 \\pmod 4$ is a sum of two squares."},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func TestMatch(t *testing.T) {
	x := index(t)
	for _, tc := range []struct {
		name string
		it   align.Item
		top  string
		id   bool
	}{
		{"the same name", align.Item{Names: []string{"Group"}}, "Q2", false},
		{"an id in a third system", align.Item{Names: []string{"a name with no match"}, SameAs: map[string]string{"nlab": "group"}}, "Q2", true},
		{"one of two ids", align.Item{Names: []string{"x"}, SameAs: map[string]string{"oeis": "A000002"}}, "Q4", true},
		{"a Mathlib declaration", align.Item{Names: []string{"x"}, Decls: []string{"Group"}}, "Q2", true},
		{"its own id", align.Item{Names: []string{"x"}, SameAs: map[string]string{"wikidata": "Q5"}}, "Q5", true},
		{"a formula", align.Item{Names: []string{"two squares"}, Text: "A prime $p\\equiv 1 \\pmod{4}$ is a sum of two squares."}, "Q7", false},
	} {
		got := x.Match(tc.it, 3)
		if len(got) == 0 || got[0].Record.ID != tc.top {
			t.Errorf("%s: the candidates are %+v, want %s first", tc.name, got, tc.top)
			continue
		}
		if tc.id && (got[0].Score != 1 || got[0].Channels[align.ChannelID] != 1) {
			t.Errorf("%s: %s has the score %v and the channels %v, want 1 by the id", tc.name, tc.top, got[0].Score, got[0].Channels)
		}
	}
	if got := x.Match(align.Item{Names: []string{"two squares"}, Text: "A prime $p\\equiv 1 \\pmod{4}$ is a sum of two squares."}, 1); got[0].Channels[align.ChannelFormula] != 1 {
		t.Errorf("the formula channel is %v, want 1", got[0].Channels)
	}
	got := x.Match(align.Item{Names: []string{"Lemma of the cycle"}}, 5)
	if len(got) < 2 || got[0].Score != got[1].Score {
		t.Errorf("the two records with one name have the candidates %+v, want two with the same score", got)
	}
	if got := x.Match(align.Item{Names: []string{"zzz"}}, 5); len(got) != 0 {
		t.Errorf("a name with no match has the candidates %+v", got)
	}
}

func TestNewIndex(t *testing.T) {
	if _, err := align.NewIndex("mathlib", []ingest.Record{{ID: "Group"}}, nil); err == nil {
		t.Error("NewIndex of mathlib gives no error")
	}
	if _, err := align.NewIndex("oeis", nil, nil); err == nil {
		t.Error("NewIndex of no records gives no error")
	}
	if s, ok := align.System("erdosproblems"); !ok || s != "erdos" || align.SourceOf("erdos") != "erdosproblems" {
		t.Errorf("the system of erdosproblems is %s, %v", s, ok)
	}
	if align.Edge("arxiv") != align.Aligned || align.Edge("oeis") != align.SameAs {
		t.Error("the edges of arxiv and oeis are wrong")
	}
}

// tree makes a working tree with nodes and records, in the words of the test.
func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "nodes/3C5/3C53TD.md", nodeFile("3C53TD", "name: Theorem of the two sets\nuses: [G5S1M9]\n", "A statement on two sets."))
	write(t, root, "nodes/G5S/G5S1M9.md", nodeFile("G5S1M9", "name: Group\nsame_as:\n  nlab: group\n", "A set with an operation."))
	write(t, root, "nodes/H7A/H7AAAA.md", nodeFile("H7AAAA", "name: Lemma of the cycle\n", "A statement on a cycle."))
	write(t, root, "nodes/J8A/J8AAAA.md", nodeFile("J8AAAA", "name: Ring\nsame_as:\n  wikidata: null\n", "A set with two operations."))
	write(t, root, "nodes/K9A/K9AAAA.md", nodeFile("K9AAAA", "name: Ring of two sets\naliases: [Theorem of the two sets]\n", "A statement on two sets."))
	write(t, root, "nodes/M1A/M1AAAA.md", nodeFile("M1AAAA", "name: Old group\nretired:\n  date: \"2026-01-01\"\n  reason: duplicate\n", "A set."))
	records(t, root, "wikidata",
		ingest.Record{ID: "Q1", Title: "theorem of the two sets"},
		ingest.Record{ID: "Q2", Title: "group of the test", Meta: map[string]string{"nlab": "group", "oeis": "A000001"}},
		ingest.Record{ID: "Q4", Title: "ring"},
		ingest.Record{ID: "Q5", Title: "lemma of the cycle"},
		ingest.Record{ID: "Q6", Title: "lemma of the cycle"},
	)
	records(t, root, "oeis", ingest.Record{ID: "A000001", Title: "a sequence of the test"}, ingest.Record{ID: "A000002", Title: "lemma of the cycle"})
	records(t, root, "arxiv", ingest.Record{ID: "2601.00001", Title: "Theorem of the two sets", Locator: "ax://paper/2601.00001", Class: ingest.RecordOnly})
	return root
}

// edges returns the proposals as node edge target lines, in order.
func edges(ps []align.Proposal) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Node+" "+p.Edge+" "+p.Target)
	}
	slices.Sort(out)
	return out
}

func TestPropose(t *testing.T) {
	root := tree(t)
	res, err := align.Propose(root, "wikidata", align.Options{Date: "2026-10-01", By: "m2/test align"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"G5S1M9 same_as Q2",  // by the nlab id
		"H7AAAA close_to Q5", // two records with the name
		"H7AAAA close_to Q6", //
		"J8AAAA close_to Q4", // the node says that wikidata has no entry
		"K9AAAA close_to Q1", // two nodes match Q1
		"3C53TD close_to Q1", //
	}
	slices.Sort(want)
	if got := edges(res.Proposals); !slices.Equal(got, want) {
		t.Errorf("the proposals are\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := edges(res.Duplicates); !slices.Equal(got, []string{"3C53TD duplicate K9AAAA"}) {
		t.Errorf("the duplicates are %v", got)
	}
	for _, p := range res.Proposals {
		if p.Proposed != "2026-10-01" || p.By != "m2/test align" || p.Embedder != "hash/1" || p.Records == "" || len(p.ID) != 12 {
			t.Errorf("the proposal %+v has a missing field", p)
		}
		if p.Node == "3C53TD" && p.Centrality != 0 || p.Node == "G5S1M9" && p.Centrality != 1 {
			t.Errorf("%s has the centrality %d", p.Node, p.Centrality)
		}
	}

	// The oeis records meet the nodes through the Wikidata item of the group.
	res, err = align.Propose(root, "oeis", align.Options{Date: "2026-10-01"})
	if err != nil {
		t.Fatal(err)
	}
	if got := edges(res.Proposals); !slices.Contains(got, "G5S1M9 same_as A000001") || !slices.Contains(got, "H7AAAA same_as A000002") {
		t.Errorf("the oeis proposals are %v", got)
	}

	res, err = align.Propose(root, "arxiv", align.Options{Date: "2026-10-01"})
	if err != nil {
		t.Fatal(err)
	}
	if got := edges(res.Proposals); !slices.Equal(got, []string{"3C53TD aligned ax://paper/2601.00001", "K9AAAA aligned ax://paper/2601.00001"}) {
		t.Errorf("the arxiv proposals are %v", got)
	}

	if _, err := align.Propose(root, "stacks", align.Options{}); err == nil || !strings.Contains(err.Error(), "m2 ingest stacks") {
		t.Errorf("Propose with no records gives %v", err)
	}
}

func TestCurate(t *testing.T) {
	root := tree(t)
	res, err := align.Propose(root, "wikidata", align.Options{Date: "2026-10-01", By: "m2/test align"})
	if err != nil {
		t.Fatal(err)
	}
	if err := res.Write(root); err != nil {
		t.Fatal(err)
	}
	// A second run on a later day keeps the first date.
	res, err = align.Propose(root, "wikidata", align.Options{Date: "2026-10-09"})
	if err != nil {
		t.Fatal(err)
	}
	if err := res.Write(root); err != nil {
		t.Fatal(err)
	}
	queue, err := align.ReadQueue(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 7 {
		t.Fatalf("the queue has %d proposals, want 7", len(queue))
	}
	if queue[0].Node != "G5S1M9" {
		t.Errorf("the first proposal is %+v, and the node with the most dependents must be first", queue[0])
	}
	id := map[string]string{}
	for _, p := range queue {
		if p.Proposed != "2026-10-01" {
			t.Errorf("%s has the date %s, want the first date", p.ID, p.Proposed)
		}
		id[p.Node+" "+p.Edge+" "+p.Target] = p.ID
	}

	person := align.Review{Person: "github:curator", Date: "2026-10-10"}
	for _, r := range []align.Review{{Person: "agent:align", Date: "2026-10-10"}, {Person: "github:helper[bot]", Date: "2026-10-10"}, {Person: "github:curator", Date: "10 October"}} {
		if _, err := align.Accept(root, id["G5S1M9 same_as Q2"], r); err == nil {
			t.Errorf("Accept with %+v gives no error", r)
		}
	}
	if _, err := align.Accept(root, "000000000000", person); err == nil {
		t.Error("Accept of an id that is not in the queue gives no error")
	}
	if _, err := align.Accept(root, id["H7AAAA close_to Q5"], person); err == nil {
		t.Error("Accept of a close_to edge with no note gives no error")
	}
	if _, err := align.Accept(root, id["G5S1M9 same_as Q2"], person); err != nil {
		t.Fatal(err)
	}
	note := person
	note.Note = "The item is a cycle of a graph, and the node is a cycle of a permutation."
	if _, err := align.Accept(root, id["H7AAAA close_to Q5"], note); err != nil {
		t.Fatal(err)
	}
	if _, err := align.Reject(root, id["H7AAAA close_to Q6"], person); err != nil {
		t.Fatal(err)
	}
	if _, err := align.Accept(root, id["3C53TD duplicate K9AAAA"], person); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(filepath.Join(root, "nodes/G5S/G5S1M9.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "same_as:\n  wikidata: Q2\n  nlab: group\n") {
		t.Errorf("the node file is\n%s", b)
	}
	b, err = os.ReadFile(filepath.Join(root, "nodes/H7A/H7AAAA.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "close_to:\n  - system: wikidata\n    id: Q5\n    note: "+note.Note+"\n") {
		t.Errorf("the node file is\n%s", b)
	}

	ds, err := align.ReadDecisions(root)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, d := range ds {
		lines = append(lines, d.String())
	}
	want := []string{
		"3C53TD duplicate math2 K9AAAA accepted github:curator 2026-10-10 -",
		"G5S1M9 same_as wikidata Q2 accepted github:curator 2026-10-10 " + align.RecordHash(ingest.Record{ID: "Q2", Title: "group of the test"}),
		"H7AAAA close_to wikidata Q5 accepted github:curator 2026-10-10 " + align.RecordHash(ingest.Record{Title: "lemma of the cycle"}),
		"H7AAAA close_to wikidata Q6 rejected github:curator 2026-10-10 " + align.RecordHash(ingest.Record{Title: "lemma of the cycle"}),
	}
	if !slices.Equal(lines, want) {
		t.Errorf("the register is\n%s\nwant\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}

	// A decided edge is not proposed again, and the queue has the edges that wait.
	res, err = align.Propose(root, "wikidata", align.Options{Date: "2026-10-11"})
	if err != nil {
		t.Fatal(err)
	}
	if err := res.Write(root); err != nil {
		t.Fatal(err)
	}
	queue, err = align.ReadQueue(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := edges(queue); !slices.Equal(got, []string{"3C53TD close_to Q1", "J8AAAA close_to Q4", "K9AAAA close_to Q1"}) {
		t.Errorf("the queue is %v", got)
	}

	if err := align.Confirm(root, "G5S1M9", "nlab", person); err != nil {
		t.Fatal(err)
	}
	if err := align.Confirm(root, "J8AAAA", "wikidata", person); err == nil {
		t.Error("Confirm of a null value gives no error")
	}
	if err := align.Confirm(root, "ZZZZZZ", "wikidata", person); err == nil {
		t.Error("Confirm of a tag with no node gives no error")
	}
	ds, err = align.ReadDecisions(root)
	if err != nil {
		t.Fatal(err)
	}
	// The line of the confirmation goes after the other lines of its tag.
	if len(ds) != 5 || ds[2].String() != "G5S1M9 same_as nlab group confirmed github:curator 2026-10-10 -" {
		t.Errorf("the register is %v", ds)
	}
}

func TestAcceptAligned(t *testing.T) {
	root := tree(t)
	res, err := align.Propose(root, "arxiv", align.Options{Date: "2026-10-01"})
	if err != nil {
		t.Fatal(err)
	}
	if err := res.Write(root); err != nil {
		t.Fatal(err)
	}
	p := res.Proposals[0]
	if _, err := align.Accept(root, p.ID, align.Review{Person: "orcid:0000-0002-1825-0097", Date: "2026-10-02", Match: "close"}); err == nil {
		t.Error("Accept with the match close gives no error")
	}
	if _, err := align.Accept(root, p.ID, align.Review{Person: "orcid:0000-0002-1825-0097", Date: "2026-10-02", Match: "weaker"}); err != nil {
		t.Fatal(err)
	}
	nodes, err := node.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range nodes {
		if n.ID == p.Node {
			l, _ := n.Meta["aligned"].([]any)
			if len(l) != 1 || l[0].(map[string]any)["record"] != "ax://paper/2601.00001" || l[0].(map[string]any)["match"] != "weaker" {
				t.Errorf("the aligned edges of %s are %v", n.ID, l)
			}
		}
	}
}

func TestHash(t *testing.T) {
	var h align.Hash
	a, b := h.Embed("the order of a subgroup divides the order of the group"), h.Embed("the order of a subgroup divides the order of the group")
	if len(a) != 1024 || !slices.Equal(a, b) {
		t.Errorf("Embed gives %d numbers, or two vectors for one text", len(a))
	}
	if len(align.Hash{Dim: 16}.Embed("x")) != 16 || h.Name() != "hash/1" {
		t.Error("Dim or Name is wrong")
	}
	if v := h.Embed(""); slices.ContainsFunc(v, func(x float32) bool { return x != 0 }) {
		t.Error("the vector of an empty text is not zero")
	}
}

func TestPick(t *testing.T) {
	c := func(scores ...float64) []align.Candidate {
		var out []align.Candidate
		for i, s := range scores {
			out = append(out, align.Candidate{Record: ingest.Record{ID: string(rune('a' + i))}, Score: s})
		}
		return out
	}
	var o align.Options
	for _, tc := range []struct {
		cands []align.Candidate
		ok    bool
	}{
		{nil, false},
		{c(0.95), true},
		{c(0.89), false},
		{c(0.95, 0.9), true}, // the difference is 0.05 to three places
		{c(0.95, 0.91), false},
		{c(1, 1), false},
	} {
		if got, ok := o.Pick(tc.cands); ok != tc.ok || ok && got.Record.ID != "a" {
			t.Errorf("Pick of %v gives %v, %v", tc.cands, got.Record.ID, ok)
		}
	}
}

func TestMatchWords(t *testing.T) {
	x, err := align.NewIndex("oeis", []ingest.Record{
		{ID: "A1", Title: "Graphs of strength 3."},
		{ID: "A2", Title: "Factorials of double factorials."},
		{ID: "A3", Title: "Graphs of the plane."},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, id string
		max      float64
	}{
		{"graphs of strength 2", "A1", 0}, // other numbers
		{"double factorial", "A2", 0.99},  // a word two times in the record
		{"graphs of strength 3", "A1", 1},
	} {
		for _, c := range x.Match(align.Item{Names: []string{tc.name}}, 5) {
			if c.Record.ID == tc.id && c.Channels[align.ChannelName] > tc.max {
				t.Errorf("%s: the name channel of %s is %v, want %v or less", tc.name, tc.id, c.Channels[align.ChannelName], tc.max)
			}
		}
	}
}
