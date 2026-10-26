package inventory

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/tag"
)

var day = time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)

// counter mints the row identifiers 7K3QX2M9R2, then the next valid tags, so that the tests do not depend on crypto/rand.
func counter() func(func(string) bool) (string, error) {
	n := 0
	return func(used func(string) bool) (string, error) {
		for {
			n++
			p := fmt.Sprintf("AB%07d", n)
			t := p + string(tag.Check(p))
			if !used(t) {
				return t, nil
			}
		}
	}
}

func wikidata(qid, label string, meta map[string]string) Mention {
	src := &ingest.Source{ID: "wikidata", Licence: "CC0-1.0", Class: ingest.Open}
	return FromRecords(src, "blake3:00", "wikidata/1", []ingest.Record{{ID: qid, Title: label, URL: "https://www.wikidata.org/wiki/" + qid, Class: ingest.Open, Meta: meta}})[0]
}

func TestClusterJoinsByIdentifier(t *testing.T) {
	ms := []Mention{
		wikidata("Q11518", "Pythagorean theorem", map[string]string{"proofwiki": "Pythagoras's_Theorem", "mathworld": "PythagoreanTheorem"}),
		wikidata("Q177646", "group", map[string]string{"nlab": "group", "proofwiki": "Definition:Group"}),
		{ID: "m2://rec/proofwiki/Pythagoras's Theorem#1", Source: "proofwiki", NativeID: "Pythagoras's Theorem", Kind: "statement", Class: "open", By: "proofwiki/1"},
		{ID: "m2://rec/enumerate/x#1", Source: "enumerate", NativeID: "x", Kind: "statement", Class: "open", By: "agent:enumerate"},
	}
	res, err := Cluster(nil, ms, Options{Day: day, By: "test", Mint: counter()})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 2 || len(res.Added) != 2 || res.Waiting != 1 {
		t.Fatalf("got %d rows, %d added, %d waiting; want 2, 2, 1", len(res.Rows), len(res.Added), res.Waiting)
	}
	var pyth *Row
	for i := range res.Rows {
		if res.Rows[i].Name == "Pythagorean theorem" {
			pyth = &res.Rows[i]
		}
	}
	if pyth == nil || len(pyth.Mentions) != 2 || pyth.Spread != 2 || pyth.Kind != "statement" || pyth.Layer != Textbook {
		t.Fatalf("the Pythagorean row is %+v", pyth)
	}
	for _, r := range res.Rows {
		if !tag.IsTag(r.ID) || len(r.ID) != tag.Len {
			t.Errorf("the row id %s is not a tag of 10 characters", r.ID)
		}
	}

	// A second run with a new mention that links the two rows merges them, and keeps the row with more mentions.
	ms = append(ms, Mention{ID: "m2://rec/nlab/group#1", Source: "nlab", NativeID: "group", Kind: "definition", Class: "record", By: "nlab/1", Same: map[string][]string{"wikipedia": {"Pythagorean theorem"}, "wikidata": {"Q11518"}}})
	again, err := Cluster(res.Rows, ms, Options{Day: day, By: "test", Mint: counter(), Used: func(s string) bool { return slices.Contains(res.Added, s) }})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Added) != 0 || again.Merged != 1 || len(again.Rows) != 2 {
		t.Fatalf("the second run added %d and merged %d of %d rows; want 0, 1, 2", len(again.Added), again.Merged, len(again.Rows))
	}
	if n := OpenCount(again.Rows, day); n != 1 {
		t.Fatalf("the open count is %d, want 1", n)
	}
	s := Summarize(again.Rows, ms, day)
	if s.Rows != 1 || s.Outcomes[OutcomeMerged] != 1 || s.Sources["wikidata"] != 2 {
		t.Fatalf("the summary is %+v", s)
	}
}

func TestClusterKeepsRows(t *testing.T) {
	ms := []Mention{wikidata("Q1", "a", nil), wikidata("Q2", "b", nil)}
	res, err := Cluster(nil, ms, Options{Day: day, Mint: counter()})
	if err != nil {
		t.Fatal(err)
	}
	again, err := Cluster(res.Rows, ms, Options{Day: day, Mint: counter(), Used: func(s string) bool { return slices.Contains(res.Added, s) }})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(res.Rows, again.Rows, func(a, b Row) bool { return a.ID == b.ID && slices.Equal(a.Mentions, b.Mentions) }) || len(again.Added) != 0 || again.Touched != 0 {
		t.Fatalf("a second run of the same mentions changed the rows")
	}
	// A mention that the source removed stays in its row, so the row keeps its history.
	gone, err := Cluster(res.Rows, ms[:1], Options{Day: day, Mint: counter()})
	if err != nil || len(gone.Rows) != 2 || len(gone.Added) != 0 {
		t.Fatalf("a removed mention gave %v, %v", gone, err)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	ms := []Mention{wikidata("Q2", "b", map[string]string{"msc": "20-XX"}), wikidata("Q1", "a", map[string]string{"oeis": "A000045"})}
	if err := WriteMentions(root, "wikidata", ms); err != nil {
		t.Fatal(err)
	}
	if err := WriteMentions(root, "oeis", ms); err == nil {
		t.Fatal("WriteMentions accepted the mentions of another source")
	}
	got, err := ReadMentions(root)
	if err != nil || len(got) != 2 || got[0].NativeID != "Q1" || got[1].Kind != "topic" || got[0].Kind != "object" {
		t.Fatalf("ReadMentions = %+v, %v", got, err)
	}
	res, err := Cluster(nil, got, Options{Day: day, Mint: counter()})
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteRows(root, res.Rows); err != nil {
		t.Fatal(err)
	}
	if err := AppendRegister(root, res.Added, "abc1234"); err != nil {
		t.Fatal(err)
	}
	if err := AppendRegister(root, res.Added[:1], "abc1234"); err == nil {
		t.Fatal("AppendRegister accepted an identifier that the register has")
	}
	rows, err := ReadRows(root)
	if err != nil || len(rows) != 2 || rows[0].ID != res.Rows[0].ID {
		t.Fatalf("ReadRows = %+v, %v", rows, err)
	}
	reg, err := ReadRegister(root)
	if err != nil || len(reg) != 2 {
		t.Fatalf("ReadRegister = %v, %v", reg, err)
	}
	data, err := os.ReadFile(filepath.Join(root, RegisterDir, RowShard(res.Added[0])+".reg"))
	if err != nil || !strings.HasPrefix(string(data), "# id commit\n") {
		t.Fatalf("the register file is %q, %v", data, err)
	}
	// Writing fewer mentions removes the shards that are empty now.
	if err := WriteMentions(root, "wikidata", ms[:1]); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(root, MentionsDir, "wikidata", "*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("got %d shard files, want 1", len(files))
	}
}

func TestFromRecordsDropsShareAlikeText(t *testing.T) {
	src := &ingest.Source{ID: "planetmath", Licence: "CC-BY-SA-4.0", Class: ingest.Open}
	ms := FromRecords(src, "blake3:00", "planetmath/1", []ingest.Record{{ID: "x", Title: "  A   title ", Text: "Some text.", Class: ingest.Open}})
	if ms[0].Text != "" || ms[0].Name != "A title" {
		t.Fatalf("the mention is %+v", ms[0])
	}
	src = &ingest.Source{ID: "mathlib", Licence: "Apache-2.0", Class: ingest.Open}
	ms = FromRecords(src, "blake3:00", "mathlib/1", []ingest.Record{{ID: "Nat.add_comm", Text: "The sum is commutative.", Class: ingest.Open}})
	if ms[0].Text == "" || ms[0].ID != "m2://rec/mathlib/Nat.add_comm#1" {
		t.Fatalf("the mention is %+v", ms[0])
	}
}

func TestIsOpen(t *testing.T) {
	cases := []struct {
		o    *Outcome
		open bool
	}{
		{nil, true},
		{&Outcome{Value: OutcomeNode}, false},
		{&Outcome{Value: OutcomeDeferred, Until: "2026-10-26"}, false},
		{&Outcome{Value: OutcomeDeferred, Until: "2026-10-25"}, true},
		{&Outcome{Value: OutcomeDeferred}, true},
	}
	for _, c := range cases {
		r := Row{Outcome: c.o}
		if r.IsOpen(day) != c.open {
			t.Errorf("%+v: IsOpen = %v", c.o, !c.open)
		}
	}
}

func TestShards(t *testing.T) {
	if RowShard("7K3QX2M9R2") != "7K3" {
		t.Fatal("RowShard is not the first three characters")
	}
	s := MentionShard("m2://rec/wikidata/Q1#1")
	if len(s) != 3 || strings.Trim(s, tag.Alphabet) != "" || s != MentionShard("m2://rec/wikidata/Q1#1") {
		t.Fatalf("MentionShard = %q", s)
	}
	if got := MentionID("oeis", "A000045", 12); got != "m2://rec/oeis/A000045#12" {
		t.Fatalf("MentionID = %q", got)
	}
}

func TestRules(t *testing.T) {
	ms := []Mention{
		wikidata("Q8487", "Euclid", map[string]string{"proofwiki": "Mathematician:Euclid", "mathworld": "Euclid"}),
		wikidata("Q9", "Elements", map[string]string{"proofwiki": "Book:Euclid/The Elements"}),
		wikidata("Q10", "group", map[string]string{"proofwiki": "Definition:Group"}),
	}
	res, err := Cluster(nil, ms, Options{Day: day, Mint: counter()})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range res.Rows {
		if r.Outcome != nil {
			got[r.Name] = r.Outcome.Rule
		}
	}
	if len(got) != 2 || got["Euclid"] != "person" || got["Elements"] != "book" {
		t.Fatalf("the rules gave %v", got)
	}
}
