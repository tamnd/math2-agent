package release_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/release"
)

func TestNames(t *testing.T) {
	for s, want := range map[string]bool{"2027.04": true, "2027.04.1": true, "2027.12.10": true, "2027.13": false, "2027.4": false, "2027.04.0": false, "next": false} {
		if release.IsName(s) != want {
			t.Errorf("IsName(%q) = %v", s, !want)
		}
	}
	if release.CompareNames("2027.04.10", "2027.04.9") <= 0 || release.CompareNames("2027.04", "2027.04.1") >= 0 || release.CompareNames("2027.05", "2027.04.3") <= 0 {
		t.Error("CompareNames does not give the order of time")
	}
	april := time.Date(2027, 4, 5, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		names []string
		want  string
	}{
		{nil, "2027.04"},
		{[]string{"2027.03"}, "2027.04"},
		{[]string{"2027.03", "2027.04"}, "2027.04.1"},
		{[]string{"2027.04", "2027.04.1", "2027.04.2"}, "2027.04.3"},
	} {
		if got := release.NextName(tc.names, april); got != tc.want {
			t.Errorf("NextName(%v) = %s, and it must be %s", tc.names, got, tc.want)
		}
	}

	root := t.TempDir()
	dir := filepath.Join(root, filepath.FromSlash(release.Dir))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"2027.04.10.yaml", "2027.04.9.yaml", "2027.03.yaml", "next.yaml", "2027.04.md"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := release.List(root)
	if err != nil || strings.Join(got, " ") != "2027.03 2027.04.9 2027.04.10" {
		t.Errorf("List gives %v, %v", got, err)
	}
}

func TestLevels(t *testing.T) {
	l := release.Levels{"3C53TD": {Evidence: "E1", Fidelity: "F0"}, "G5S1M9": {}}
	data := l.Marshal()
	if string(data) != "# tag evidence fidelity\n3C53TD E1 F0\nG5S1M9 - -\n" {
		t.Errorf("Marshal gives %q", data)
	}
	path := filepath.Join(t.TempDir(), "2027.03.levels")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	back, err := release.ReadLevels(path)
	if err != nil || !reflect.DeepEqual(back, l) {
		t.Errorf("ReadLevels gives %v, %v", back, err)
	}
	if err := os.WriteFile(path, []byte("3C53TD E1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := release.ReadLevels(path); err == nil {
		t.Error("ReadLevels accepts a line with two fields")
	}
}

func TestCheckPins(t *testing.T) {
	m := &release.Manifest{Toolchain: release.Toolchain{Lean: "leanprover/lean4:v4.35.0", Mathlib: strings.Repeat("ab", 20)}}
	if err := m.CheckPins(); err != nil {
		t.Error(err)
	}
	m.Toolchain = release.Toolchain{Lean: "leanprover/lean4:v4.36.0-rc1", Mathlib: "master"}
	if err := m.CheckPins(); err == nil || strings.Count(err.Error(), "\n") != 1 {
		t.Errorf("CheckPins of a release candidate and a branch gives %v", err)
	}
}

// parse returns the node files of the front matter, by tag.
func parse(t *testing.T, fronts map[string]string) []*node.Node {
	t.Helper()
	var out []*node.Node
	for tag, front := range fronts {
		n, err := node.Parse("nodes/"+tag[:3]+"/"+tag+".md", []byte("---\ntag: "+tag+"\n"+front+"---\n\n## Statement\n\nText.\n"))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	return out
}

func notesFixture(t *testing.T) (from, to *release.Snapshot) {
	from = &release.Snapshot{
		Manifest: &release.Manifest{Release: "2027.03", Counts: release.Counts{NodesLive: 5, ByKind: map[string]int{"topic": 1, "concept": 1, "statement": 2, "conjecture": 1}, ByEvidence: map[string]int{"E1": 2}}},
		Levels:   release.Levels{"TT0001": {}, "GG0001": {}, "SS0001": {Evidence: "E1"}, "CC0001": {}, "RR0001": {Evidence: "E1"}},
		Nodes: parse(t, map[string]string{
			"TT0001": "kind: topic\nname: Group theory\n",
			"GG0001": "kind: concept\nname: Group\ntopics: [TT0001]\n",
			"SS0001": "kind: statement\nname: Lagrange's theorem\ntopics: [TT0001]\n",
			"CC0001": "kind: conjecture\nname: A conjecture\n",
			"RR0001": "kind: statement\nname: A copy of Lagrange's theorem\n",
		}),
		Bindings: []string{"SS0001 lean4 A.b A.b bind 0123456789abcdef -"},
	}
	to = &release.Snapshot{
		Manifest: &release.Manifest{Release: "2027.04", Counts: release.Counts{NodesLive: 5, ByKind: map[string]int{"topic": 1, "concept": 2, "statement": 1, "conjecture": 1, "example": 1}, ByEvidence: map[string]int{"E5": 1}}},
		Levels:   release.Levels{"TT0001": {}, "GG0001": {}, "SS0001": {Evidence: "E5", Fidelity: "F2"}, "CC0001": {}, "NN0001": {}},
		Nodes: parse(t, map[string]string{
			"TT0001": "kind: topic\nname: Group theory\n",
			"GG0001": "kind: concept\nname: Group\ntopics: [TT0001]\n",
			"SS0001": "kind: statement\nname: Lagrange's theorem\ntopics: [TT0001]\n",
			"CC0001": "kind: conjecture\nname: A conjecture\nresolved_by: [SS0001]\n",
			"RR0001": "kind: statement\nname: A copy of Lagrange's theorem\nretired: {date: 2027-03-20, reason: duplicate, successor: [SS0001]}\n",
			"NN0001": "kind: example\nname: The cyclic group of order 6\ntopics: [TT0001]\n",
		}),
		Bindings: []string{"SS0001 lean4 A.b A.b bind 0123456789abcdef -", "SS0001 lean4 A.b A.c rename fedcba9876543210fedc -"},
	}
	return from, to
}

func TestDiff(t *testing.T) {
	from, to := notesFixture(t)
	n := release.Diff(from, to)
	want := []release.Change{
		{Count: "by_evidence.E1", Before: 2, After: 0, Cause: "1 node moved to another level, 1 retired node"},
		{Count: "by_evidence.E5", Before: 0, After: 1, Cause: "1 node moved to this level"},
		{Count: "by_kind.concept", Before: 1, After: 2}, // the manifest has a count that the node files do not give
		{Count: "by_kind.example", Before: 0, After: 1, Cause: "1 new node"},
		{Count: "by_kind.statement", Before: 2, After: 1, Cause: "1 retired node"},
	}
	if !reflect.DeepEqual(n.Counts, want) {
		t.Errorf("the counts are\n%+v\nand they must be\n%+v", n.Counts, want)
	}
	if !reflect.DeepEqual(n.Topics, []release.TopicCount{{Tag: "TT0001", Name: "Group theory", Before: 2, After: 3}}) {
		t.Errorf("the topics are %+v", n.Topics)
	}
	if !reflect.DeepEqual(n.Moves, []release.Move{{Tag: "SS0001", Name: "Lagrange's theorem", Before: "E1", After: "E5"}}) {
		t.Errorf("the moves are %+v", n.Moves)
	}
	if !reflect.DeepEqual(n.Resolved, []release.Resolve{{Tag: "CC0001", Name: "A conjecture", By: []string{"SS0001"}}}) {
		t.Errorf("the resolved nodes are %+v", n.Resolved)
	}
	if !reflect.DeepEqual(n.Retired, []release.Retire{{Tag: "RR0001", Name: "A copy of Lagrange's theorem", Reason: "duplicate", Successors: []string{"SS0001"}}}) {
		t.Errorf("the retired nodes are %+v", n.Retired)
	}
	if !reflect.DeepEqual(n.Rebinds, []release.Rebind{{Tag: "SS0001", Old: "A.b", New: "A.c", Cause: "rename", Mathlib: "fedcba9876543210fedc"}}) {
		t.Errorf("the rebinds are %+v", n.Rebinds)
	}

	first := release.Diff(nil, from)
	if first.From != "" || len(first.Counts) != 6 || first.Counts[0].Count != "nodes_live" || first.Counts[0].Cause != "5 new nodes" {
		t.Errorf("the notes of the first release have the counts %+v", first.Counts)
	}
}

func TestNotesMarkdown(t *testing.T) {
	from, to := notesFixture(t)
	n := release.Diff(from, to)
	n.Summary = "This release settles a conjecture.\n\nIt has a | in it."
	data := n.Markdown()
	golden := filepath.Join("testdata", "notes.md")
	if *update {
		if err := os.WriteFile(golden, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(want) {
		t.Errorf("Markdown gives\n%s\nand %s has\n%s", data, golden, want)
	}
	back, err := release.ParseNotes(data)
	if err != nil {
		t.Fatal(err)
	}
	if back.Summary != n.Summary || !reflect.DeepEqual(back.Counts, n.Counts) {
		t.Errorf("ParseNotes gives the summary %q and the counts %+v", back.Summary, back.Counts)
	}
	n.Summary = ""
	if back, err := release.ParseNotes(n.Markdown()); err != nil || back.Summary != "" {
		t.Errorf("ParseNotes of new notes gives the summary %q, %v", back.Summary, err)
	}
	if _, err := release.ParseNotes([]byte("## Counts\n\n| nodes_live | one | 2 | +1 | x |\n")); err == nil {
		t.Error("ParseNotes accepts a count that is not a number")
	}
}

func TestLoad(t *testing.T) {
	dir, commit := repo(t, filepath.Join("testdata", "seed"))
	m := &release.Manifest{Release: "2027.03", Commit: commit}
	data, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	for p, b := range map[string][]byte{release.ManifestFile("2027.03"): data, release.LevelsFile("2027.03"): release.Levels{"3C53TD": {Evidence: "E1"}}.Marshal()} {
		path := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := release.Load(context.Background(), dir, "2027.03")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Nodes) != 2 || s.Levels["3C53TD"].Evidence != "E1" || len(s.Bindings) != 0 {
		t.Errorf("Load gives %d nodes, the levels %v and the bindings %v", len(s.Nodes), s.Levels, s.Bindings)
	}
	if _, err := release.Load(context.Background(), dir, "2027.04"); err == nil {
		t.Error("Load reads a release that has no manifest")
	}
}
