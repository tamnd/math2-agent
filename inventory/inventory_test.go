package inventory

import (
	"fmt"
	"maps"
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
	for _, c := range []ingest.Class{ingest.RecordOnly, ingest.GFDL} {
		m := Mention{ID: MentionID("stacks", "01H8", 1), Source: "stacks", NativeID: "01H8", Class: string(c), Text: "Some text.", By: "stacks/1"}
		if err := WriteMentions(root, "stacks", []Mention{m}); err == nil {
			t.Errorf("WriteMentions accepted a mention of the class %s with text", c)
		}
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

func TestFromRecordsTextbookResource(t *testing.T) {
	src := &ingest.Source{ID: "apex", Kind: "textbook", Licence: "CC-BY-NC-4.0", Class: ingest.Open, Resource: "8F2K1QW7ZJ/e1"}
	ms := FromRecords(src, "blake3:00", "textbook/1", []ingest.Record{
		{ID: "a.tex#thm-1", Kind: "statement", Class: ingest.Open, Meta: map[string]string{"environment": "theorem", "section": "3.14 The mean value theorem"}},
		{ID: "a.tex#def-1", Kind: "definition", Class: ingest.Open, Meta: map[string]string{"section": "Limits"}},
	})
	if ms[1].Resource != "8F2K1QW7ZJ/e1" || ms[1].At == nil || *ms[1].At != (At{Section: "3.14", Kind: "theorem"}) {
		t.Errorf("the theorem is %+v, at %+v", ms[1], ms[1].At)
	}
	if ms[0].Resource != "8F2K1QW7ZJ/e1" || ms[0].At != nil {
		t.Errorf("the definition is %+v, at %+v", ms[0], ms[0].At)
	}
	src.Resource = ""
	if ms := FromRecords(src, "blake3:00", "textbook/1", []ingest.Record{{ID: "x", Class: ingest.Open}}); ms[0].Resource != "" {
		t.Errorf("a source with no resource gives the resource %q", ms[0].Resource)
	}
}

func TestFromRecordsLibrary(t *testing.T) {
	src := &ingest.Source{ID: "afp", Kind: ingest.Library, Licence: "BSD-3-Clause", Class: ingest.Open, Intake: "crosswalk", Authored: "mixed",
		Snapshots: []ingest.Snapshot{{Hash: "blake3:00", Commit: "c0"}, {Hash: "blake3:01", Commit: "c1"}}}
	ms := FromRecords(src, "blake3:01", "afp/1", []ingest.Record{{ID: "Prime_Cyclic", Kind: "statement", Licence: "LGPL-2.1-only", Class: ingest.RecordOnly}, {ID: "Prime_Factors", Kind: "entry", Class: ingest.Open}})
	got := []string{ms[0].Pin, ms[0].Intake, ms[0].Authored, ms[0].Licence, ms[1].Licence, ms[0].Kind, ms[1].Kind}
	if want := []string{"c1", "crosswalk", "mixed", "LGPL-2.1-only", "BSD-3-Clause", "statement", Unknown}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	src.Kind = "wiki"
	if m := FromRecords(src, "blake3:01", "afp/1", []ingest.Record{{ID: "x", Class: ingest.Open}})[0]; m.Pin != "" || m.Intake != "" || m.Licence != "" {
		t.Fatalf("a source that is not a library gives %+v", m)
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

// TestWriteIsStable checks that two writes of the same mentions and rows, in any order, give the same bytes (corpus spec K4.8).
func TestWriteIsStable(t *testing.T) {
	ms := []Mention{wikidata("Q2", "b", map[string]string{"msc": "20-XX"}), wikidata("Q1", "a", map[string]string{"oeis": "A000045"}), wikidata("Q3", "c", nil)}
	res, err := Cluster(nil, ms, Options{Day: day, Mint: counter()})
	if err != nil {
		t.Fatal(err)
	}
	write := func(ms []Mention, rows []Row) map[string]string {
		root := t.TempDir()
		if err := WriteMentions(root, "wikidata", ms); err != nil {
			t.Fatal(err)
		}
		if err := WriteRows(root, rows); err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for _, dir := range []string{MentionsDir, RowsDir} {
			err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				data, err := os.ReadFile(path)
				rel, _ := filepath.Rel(root, path)
				out[rel] = string(data)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		return out
	}
	first := write(ms, res.Rows)
	rev := slices.Clone(ms)
	slices.Reverse(rev)
	rows := slices.Clone(res.Rows)
	slices.Reverse(rows)
	second := write(rev, rows)
	if len(first) == 0 || !maps.Equal(first, second) {
		t.Errorf("the two writes differ:\n%v\n%v", first, second)
	}
}

func TestFromRecordsKeepsOnlyAllowedText(t *testing.T) {
	src := &ingest.Source{ID: "mixed", Licence: "CC-BY-4.0", Class: ingest.Open}
	var recs []ingest.Record
	for _, c := range ingest.Classes() {
		recs = append(recs, ingest.Record{ID: string(c), Title: "A lemma", Text: "Some text.", Class: c})
	}
	for _, m := range FromRecords(src, "blake3:00", "mixed/1", recs) {
		if allowed := ingest.Class(m.Class).StoresText(); (m.Text != "") != allowed {
			t.Errorf("the mention %s of the class %s has the text %q", m.ID, m.Class, m.Text)
		}
	}
}

func TestFromRecordsLicenceOfEachItem(t *testing.T) {
	src := &ingest.Source{ID: "zenodo", Licence: "CC-BY-4.0", Class: ingest.Open, PerItemLicence: true}
	ms := FromRecords(src, "blake3:00", "zenodo/1", []ingest.Record{
		{ID: "a", Class: ingest.Open, Licence: "CC0-1.0"},
		{ID: "b", Class: ingest.Open},
	})
	if ms[0].Licence != "CC0-1.0" || ms[1].Licence != "CC-BY-4.0" {
		t.Fatalf("the licences are %q and %q", ms[0].Licence, ms[1].Licence)
	}
	src.PerItemLicence = false
	if ms = FromRecords(src, "blake3:00", "zenodo/1", []ingest.Record{{ID: "a", Class: ingest.Open}}); ms[0].Licence != "" {
		t.Fatalf("a source with one licence gives the mention the licence %q", ms[0].Licence)
	}
}

func TestMathlibKinds(t *testing.T) {
	src := &ingest.Source{ID: "mathlib", Licence: "Apache-2.0", Class: ingest.Open}
	want := map[string]string{"theorem": "statement", "axiom": "statement", "def": "definition", "inductive": "definition", "opaque": "definition", "instance": Unknown}
	for kind, k := range want {
		ms := FromRecords(src, "blake3:00", "mathlib/1", []ingest.Record{{ID: "X." + kind, Kind: kind, Class: ingest.Open}})
		if ms[0].Kind != k {
			t.Errorf("the Mathlib kind %s gives %s, want %s", kind, ms[0].Kind, k)
		}
	}
}

func TestClusterJoinsNLab(t *testing.T) {
	src := &ingest.Source{ID: "nlab", Licence: "LicenseRef-unknown", Class: ingest.RecordOnly}
	ms := []Mention{
		wikidata("Q177646", "group", map[string]string{"nlab": "group"}),
		FromRecords(src, "blake3:00", "nlab/1", []ingest.Record{{ID: "group", Title: "group", Class: ingest.RecordOnly, Text: "Some text."}})[0],
	}
	if ms[1].Text != "" {
		t.Fatalf("the nLab mention holds the text %q", ms[1].Text)
	}
	res, err := Cluster(nil, ms, Options{Day: day, By: "test", Mint: counter()})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || len(res.Rows[0].Mentions) != 2 {
		t.Fatalf("the rows are %+v, want one row with the two mentions", res.Rows)
	}
}

func TestClusterJoinsByHash(t *testing.T) {
	ax := func(id, kind, hash string, msc ...string) Mention {
		return Mention{ID: "ax://paper/" + id, Source: "arxiv", NativeID: id, Kind: kind, Hash: hash, MSC: msc, Class: "record", By: "m2/arxiv"}
	}
	ms := []Mention{
		ax("1701.00517#R1", "statement", "sha256:aa", "54E35"),
		ax("1701.00518#R1", "statement", "sha256:aa", "54E99", "47H10"),
		ax("1701.00519#R1", "statement", "sha256:aa", "11A41"),  // another block
		ax("1701.00520#R1", "definition", "sha256:aa", "54E35"), // another kind
		ax("1701.00521#R1", "exercise", "", "54E35"),            // no hash
		ax("1701.00522#R1", "unknown", "sha256:bb"),             // no code and no class yet
		ax("1701.00523#R1", "context", "sha256:cc", "54E35"),    // a context item
		ax("1701.00524#P1", "proof", "sha256:aa", "54E35"),      // attached, below
	}
	ms[7].AttachedTo = ms[0].ID
	res, err := Cluster(nil, ms, Options{Day: day, By: "test", Mint: counter()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Hashed != 1 || len(res.Rows) != 6 {
		t.Fatalf("got %d joins by hash and %d rows, want 1 and 6", res.Hashed, len(res.Rows))
	}
	rules := map[string]string{}
	for _, r := range res.Rows {
		rule := "open"
		if r.Outcome != nil {
			rule = r.Outcome.Rule
		}
		rules[r.Mentions[0]] = rule
		if r.Mentions[0] == ms[0].ID && (len(r.Mentions) != 2 || r.Mentions[1] != ms[1].ID || len(r.Proofs) != 1) {
			t.Errorf("the row of the copied statement is %+v", r)
		}
	}
	want := map[string]string{ms[0].ID: "research-default", ms[2].ID: "research-default", ms[3].ID: "research-default", ms[4].ID: "research-exercise", ms[5].ID: "open", ms[6].ID: "research-context"}
	if !maps.Equal(rules, want) {
		t.Errorf("the rules are %v, want %v", rules, want)
	}
	// Two rows of the same copied item that a run before kept apart are merged by the rule hash.
	split := []Row{{ID: "AB00000017", Mentions: []string{ms[0].ID}}, {ID: "AB00000025", Mentions: []string{ms[1].ID}}}
	again, err := Cluster(split, ms[:2], Options{Day: day, By: "test", Mint: counter()})
	if err != nil {
		t.Fatal(err)
	}
	if again.Merged != 1 || !slices.ContainsFunc(again.Rows, func(r Row) bool {
		return r.Outcome != nil && r.Outcome.Value == OutcomeMerged && r.Outcome.Rule == "hash"
	}) {
		t.Errorf("the second run gives %+v", again.Rows)
	}
}

func TestClusterJoinsByCitedResult(t *testing.T) {
	ax := func(id, kind string, cites ...CitedResult) Mention {
		return Mention{ID: "ax://paper/" + id, Source: "arxiv", NativeID: id, Kind: kind, MSC: []string{"14J60"}, Class: "record", By: "m2/arxiv", CitesResult: cites}
	}
	ms := []Mention{
		ax("1701.00517#R1", "statement"),
		ax("1701.00518#R1", "statement", CitedResult{To: "ax://paper/1701.00517#R1", Locator: "Theorem 1", Confidence: "high"}),
		ax("1701.00519#R1", "conjecture", CitedResult{To: "ax://paper/1701.00517#R1", Locator: "Theorem 1", Confidence: "high"}),
		ax("1701.00520#R1", "statement", CitedResult{To: "ax://paper/1701.00517#R1", Locator: "1", Confidence: "medium"}),        // not high
		ax("1701.00521#R1", "definition", CitedResult{To: "ax://paper/1701.00517#R1", Locator: "Theorem 1", Confidence: "high"}), // another kind
		ax("1701.00522#R1", "statement", CitedResult{To: "doi:10.1/x", Locator: "Theorem 2", Confidence: "high"}),                // a work, not a mention
	}
	res, err := Cluster(nil, ms, Options{Day: day, By: "test", Mint: counter()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Cited != 2 || len(res.Rows) != 4 {
		t.Fatalf("got %d joins by cited result and %d rows, want 2 and 4", res.Cited, len(res.Rows))
	}
	for _, r := range res.Rows {
		if r.Mentions[0] == ms[0].ID && !slices.Equal(r.Mentions, []string{ms[0].ID, ms[1].ID, ms[2].ID}) {
			t.Errorf("the row of the cited theorem has %v", r.Mentions)
		}
	}
}

func TestClusterJoinsByName(t *testing.T) {
	ax := func(id, kind, name string, msc ...string) Mention {
		return Mention{ID: "ax://paper/" + id, Source: "arxiv", NativeID: id, Kind: kind, Name: name, MSC: msc, Class: "record", By: "m2/arxiv"}
	}
	ms := []Mention{
		wikidata("Q1", "Perron–Frobenius theorem", map[string]string{"proofwiki": "Perron-Frobenius_Theorem"}),
		wikidata("Q2", "stopping time", map[string]string{"proofwiki": "Definition:Stopping_Time"}),
		wikidata("Q3", "chain rule", map[string]string{"proofwiki": "Chain_Rule"}),
		wikidata("Q4", "lower bound", map[string]string{"proofwiki": "Lower_Bound"}),
		wikidata("Q5", "Hahn–Banach theorem", map[string]string{"proofwiki": "Hahn-Banach_Theorem"}),
		wikidata("Q6", "Hahn–Banach theorem", map[string]string{"proofwiki": "Hahn-Banach_Theorem_(Real)"}),
		ax("1701.00517#R1", "statement", "The Perron-Frobenius Theorem", "15B48"),
		ax("1701.00518#R1", "definition", "Stopping time", "60G40"),
		ax("1701.00519#R1", "statement", "Chain rule"),                         // no result word
		ax("1701.00520#R1", "statement", "Lower bound"),                        // no result word
		ax("1701.00521#R1", "statement", "Perron-Frobenius theorem for cones"), // a variant
		ax("1701.00522#R1", "definition", "Perron–Frobenius theorem"),          // another kind
		ax("1701.00523#R1", "statement", "Hahn-Banach theorem"),                // two groups
	}
	res, err := Cluster(nil, ms, Options{Day: day, By: "test", Mint: counter()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Named != 2 {
		t.Fatalf("got %d joins by name, want 2", res.Named)
	}
	for _, r := range res.Rows {
		if (slices.Contains(r.Mentions, ms[0].ID) || slices.Contains(r.Mentions, ms[1].ID)) && (len(r.Mentions) != 2 || r.Layer != Textbook || r.Spread != 2) {
			t.Errorf("the row of %s is %+v", r.Name, r)
		}
	}
	// A run on rows that kept the paper apart merges its row into the row of the other source, by the rule name.
	again, err := Cluster([]Row{{ID: "AB00000017", Mentions: []string{ms[0].ID}, Layer: Textbook}, {ID: "AB00000025", Mentions: []string{ms[6].ID}, Layer: Research, Outcome: &Outcome{Value: OutcomeRecord}}}, ms[:7], Options{Day: day, By: "test", Mint: counter()})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(again.Rows, func(r Row) bool {
		return r.ID == "AB00000025" && r.Outcome.Value == OutcomeMerged && r.Outcome.Rule == "name" && r.Outcome.Target[0] == "AB00000017"
	}) {
		t.Errorf("the second run gives %+v", again.Rows)
	}
}
