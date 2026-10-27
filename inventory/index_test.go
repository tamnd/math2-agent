package inventory

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTermKeys(t *testing.T) {
	for _, tc := range []struct {
		term string
		want []string
	}{
		{"abelian group", []string{"abelian group"}},
		{"group, cyclic", []string{"cyclic group", "group cyclic"}},
		{"subgroup!normal", []string{"normal subgroup", "subgroup normal"}},
		{"order (of an element)", []string{"order element", "order"}},
		{"Lagrange's theorem", []string{"lagrange theorem"}},
		{"()", nil},
	} {
		if got := termKeys(tc.term); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("termKeys(%q) = %q, want %q", tc.term, got, tc.want)
		}
	}
}

// The fixture index is the index of a small book of algebra, in testdata/records/index/. Its terms are in our own words.
func TestMatchIndexesFixture(t *testing.T) {
	idx, err := ReadIndexes("testdata")
	if err != nil || len(idx) != 1 || idx[0].Book != "fixture_algebra" || len(idx[0].Terms) != 8 {
		t.Fatalf("ReadIndexes gave %+v, %v", idx, err)
	}
	ms := []Mention{
		{ID: "m1", Source: "wikidata", Name: "Abelian group"},
		{ID: "m2", Source: "wikidata", Name: "cyclic groups"},
		{ID: "m3", Source: "nlab", Name: "order of a group element", Aliases: []string{"order"}},
		{ID: "m4", Source: "wikidata", Name: "Lagrange theorem"},
		{ID: "m5", Source: "wikidata", Name: "quotient group"},
		{ID: "m6", Source: "wikidata", Name: "Sylow theorems"},
	}
	rows := []Row{
		{ID: "r1", Mentions: []string{"m1"}},
		{ID: "r2", Mentions: []string{"m2"}},
		{ID: "r3", Mentions: []string{"m3"}},
		{ID: "r4", Mentions: []string{"m4"}},
		{ID: "r5", Name: "normal subgroup"},
		// A merged row is not an item, so its name matches no term.
		{ID: "r6", Mentions: []string{"m5"}, Outcome: &Outcome{Value: OutcomeMerged, Target: []string{"r1"}}},
		{ID: "r7", Mentions: []string{"m6"}, Outcome: &Outcome{Value: OutcomeOutOfScope, Reason: "test"}},
	}
	got := MatchIndexes(rows, ms, idx)
	// The two terms of the normal subgroup count as one term.
	want := []IndexMatch{{Book: "fixture_algebra", Terms: 7, Matched: 5, Rate: 5.0 / 7, Missing: []string{"quotient group", "Sylow theorems"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MatchIndexes gave %+v, want %+v", got, want)
	}
}

func TestReadIndexes(t *testing.T) {
	if idx, err := ReadIndexes(t.TempDir()); err != nil || idx != nil {
		t.Errorf("a tree with no index gave %v, %v", idx, err)
	}
	for _, tc := range []struct{ file, data, want string }{
		{"Bad-Name.jsonl", `{"term":"group"}`, "the name of the book"},
		{"book.jsonl", `{"term":"group","page":3}`, `line 1: json: unknown field "page"`},
		{"book.jsonl", "{\"term\":\"group\"}\n{\"term\":\" \"}", "line 2: the term is empty"},
		{"book.jsonl", `{"term":`, "line 1:"},
	} {
		root := t.TempDir()
		dir := filepath.Join(root, IndexDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, tc.file), []byte(tc.data), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadIndexes(root); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s with %q gave %v, want %q", tc.file, tc.data, err, tc.want)
		}
	}
	// A book with no terms has the rate 0.
	if got := MatchIndexes(nil, nil, []Index{{Book: "empty"}}); len(got) != 1 || got[0].Terms != 0 || got[0].Rate != 0 {
		t.Errorf("an empty index gave %+v", got)
	}
}
