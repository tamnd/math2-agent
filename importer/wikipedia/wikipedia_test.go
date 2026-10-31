package wikipedia_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/importer/wikipedia"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/wpcorpus"
)

// corpus writes a small corpus repository: an article with a theorem, a proof and a glossary, an article that is not extracted, and an article whose edge file is of an older revision.
func corpus(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel string, data []byte) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var scope bytes.Buffer
	cs := []*wpcorpus.Candidate{
		{PageID: 100, Title: "Widget theorem (group theory)", InScope: true, Verdict: "math", Rule: "strong"},
		{PageID: 200, Title: "Gadget", InScope: true, Verdict: "math", Rule: "strong"},
		{PageID: 300, Title: "Sprocket", InScope: true, Verdict: "math", Rule: "strong"},
	}
	if err := wpcorpus.WriteCandidates(&scope, cs, "20261001", true); err != nil {
		t.Fatal(err)
	}
	write(wpcorpus.ScopePath, scope.Bytes())
	for _, md := range []*wpcorpus.Metadata{
		{Page: 100, Title: "Widget theorem (group theory)", Revision: 7, Type: wpcorpus.TypeStatement, Class: "B", Importance: "High", Wikidata: "Q1", Redirects: []string{"Widget lemma"}, MSC: []string{"20D20"}, Unstable: true},
		{Page: 200, Title: "Gadget", Revision: 8, Type: wpcorpus.TypeConcept},
		{Page: 300, Title: "Sprocket", Revision: 9, Type: wpcorpus.TypeConcept},
	} {
		if err := wpcorpus.WriteMetadata(root, md); err != nil {
			t.Fatal(err)
		}
	}
	objs := []wpcorpus.Object{
		{Ref: "o1", Tag: "SUBJ", Kind: "subject", Cue: "lead", Strength: "normal", Hash: "sha256:s", Extractor: "m2/t"},
		{Ref: "o2", Tag: "SECT", Kind: "section", Cue: "section", Name: "Statement", Section: "Statement", SectionAnchor: "Statement"},
		{Ref: "o3", Tag: "THM1", Kind: "statement/theorem", Cue: "math-theorem", Strength: "strong", Name: "Widget theorem", Section: "Statement", SectionAnchor: "Statement", Anchor: "widget", Span: [2]int{10, 20}, Hash: "sha256:t", Flags: []string{"informal"}, Extractor: "m2/t"},
		{Ref: "o4", Tag: "PRF1", Kind: "proof", Cue: "heading", Strength: "strong", Name: "Proof", Section: "Proof", SectionAnchor: "Proof", AttachedTo: "o3"},
		{Ref: "o5", Tag: "PRF2", Kind: "proof", Cue: "heading", Strength: "strong", Name: "Proof of the lead", AttachedTo: "o1"},
		{Ref: "o6", Tag: "GLO1", Kind: "entry", Cue: "term", Name: "gadget", Section: "Glossary"},
		{Ref: "o7", Tag: "LST1", Kind: "entry", Cue: "list-entry", Name: "cog"},
		{Ref: "o8", Kind: "example", Cue: "heading", Name: "Example"},
		{Ref: "o9", Tag: "CIT1", Kind: "citation", Cue: "ref"},
	}
	data, err := wpcorpus.MarshalObjects(100, 7, "Widget theorem (group theory)", objs)
	if err != nil {
		t.Fatal(err)
	}
	write(wpcorpus.ObjectsPath(100, 7), data)
	data, err = wpcorpus.MarshalObjects(300, 9, "Sprocket", []wpcorpus.Object{{Ref: "o1", Tag: "SUB3", Kind: "subject", Cue: "lead"}})
	if err != nil {
		t.Fatal(err)
	}
	write(wpcorpus.ObjectsPath(300, 9), data)
	art := wpcorpus.MentionID(100, "")
	thm := wpcorpus.MentionID(100, "THM1")
	edges := []wpcorpus.Edge{
		{From: wpcorpus.MentionID(100, "PRF1"), To: thm, Predicate: wpcorpus.PredUses, Method: wpcorpus.MethodAnchor, Confidence: wpcorpus.ConfCertain, Span: [2]int{30, 40}, Revision: 7},
		{From: thm, To: art, Predicate: wpcorpus.PredUses, Method: wpcorpus.MethodAnchor, Confidence: wpcorpus.ConfCertain, Span: [2]int{12, 14}, Revision: 7},
		{From: thm, To: wpcorpus.MentionID(200, ""), Predicate: wpcorpus.PredUses, Method: wpcorpus.MethodLinkInItem, Confidence: wpcorpus.ConfHigh, Span: [2]int{15, 18}, Revision: 7},
		{From: wpcorpus.MentionID(100, "GLO1"), To: wpcorpus.MentionID(200, ""), Predicate: wpcorpus.PredGlossaryOf, Method: wpcorpus.MethodGlossary, Confidence: wpcorpus.ConfCertain, Span: [2]int{50, 60}, Revision: 7},
		{From: art, Page: 55, Predicate: wpcorpus.PredRelated, Method: wpcorpus.MethodLinkInText, Confidence: wpcorpus.ConfLow, Span: [2]int{70, 80}, Revision: 7},
		{From: art, Target: "Missing", Predicate: wpcorpus.PredUses, Method: wpcorpus.MethodLinkInLead, Confidence: wpcorpus.ConfMedium, Span: [2]int{1, 2}, Revision: 7, Finding: wpcorpus.FindingRedLink},
		{From: art, Target: "Unresolved", Predicate: wpcorpus.PredUses, Method: wpcorpus.MethodLinkInLead, Confidence: wpcorpus.ConfMedium, Span: [2]int{3, 4}, Revision: 7},
		{From: art, Target: "Category:Theorems in group theory", Predicate: wpcorpus.PredKindOf, Method: wpcorpus.MethodCategory, Confidence: wpcorpus.ConfMedium, Span: [2]int{90, 99}, Revision: 7},
	}
	if err := wpcorpus.WriteEdges(root, &wpcorpus.EdgeFile{Page: 100, Revision: 7, Title: "Widget theorem (group theory)", Edges: edges}); err != nil {
		t.Fatal(err)
	}
	if err := wpcorpus.WriteEdges(root, &wpcorpus.EdgeFile{Page: 300, Revision: 5, Title: "Sprocket"}); err != nil {
		t.Fatal(err)
	}
	write(wpcorpus.TagsPath(100), []byte("---\npage: 100\ntags:\n  - {tag: SUBJ, ref: o1}\n  - {tag: THM1, ref: o3}\n  - {tag: PRF1, ref: o4}\n  - {tag: GONE, ref: o10, gone: r6}\n---\n"))
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "c"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return root
}

func TestImport(t *testing.T) {
	root := corpus(t)
	old := []inventory.Mention{
		{ID: wpcorpus.MentionID(100, "GONE"), Source: "wikipedia", Kind: "example", By: wikipedia.Version, Page: 100},
		{ID: wpcorpus.MentionID(400, ""), Source: "wikipedia", Kind: "definition", By: wikipedia.Version, Page: 400},
		{ID: "m2://rec/wikipedia/Widget#1", Source: "wikipedia", Kind: "statement", By: "wikipedia/1"},
	}
	res, err := wikipedia.Import(root, old)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]inventory.Mention{}
	var ids []string
	for _, m := range res.Mentions {
		got[strings.TrimPrefix(m.ID, "wikipedia://en/")] = m
		ids = append(ids, strings.TrimPrefix(m.ID, "wikipedia://en/"))
	}
	want := []string{"100", "100#GLO1", "100#GONE", "100#LST1", "100#PRF1", "100#PRF2", "100#THM1", "300", "400"}
	if !slices.Equal(ids, want) {
		t.Fatalf("mentions %v, want %v", ids, want)
	}
	if res.Articles != 2 || res.Objects != 6 || len(res.Untagged) != 1 || res.Untagged[0] != "100#o8" || !slices.Equal(res.NotExtracted, []int{200}) || !slices.Equal(res.StaleEdges, []int{300}) {
		t.Errorf("articles %d, objects %d, untagged %v, not extracted %v, stale %v", res.Articles, res.Objects, res.Untagged, res.NotExtracted, res.StaleEdges)
	}
	if res.Attached != 2 || res.Members != 1 || res.Retired != 1 || res.Stopped != 1 || res.RedLinks != 1 {
		t.Errorf("attached %d, members %d, retired %d, stopped %d, red links %d", res.Attached, res.Members, res.Retired, res.Stopped, res.RedLinks)
	}

	a := got["100"]
	if a.Kind != "statement" || a.Name != "Widget theorem" || a.Assessment != "B/High" || a.ArticleType != "statement" || !slices.Equal(a.Same["wikidata"], []string{"Q1"}) || !slices.Equal(a.Aliases, []string{"Widget lemma"}) || !slices.Equal(a.AliasesFrom, []string{"redirect"}) {
		t.Errorf("article mention %+v", a)
	}
	if a.Licence != wikipedia.Licence || a.Class != "open" || a.Text != "" || a.Revision != 7 || a.Hash != "sha256:s" || !slices.Contains(a.Flags, "unstable") || a.Snapshot == "" {
		t.Errorf("article mention %+v", a)
	}
	if a.Structure == nil || !slices.Equal(a.Structure.ProvedBy, []string{"wikipedia://en/100#PRF2"}) {
		t.Errorf("article structure %+v", a.Structure)
	}
	th := got["100#THM1"]
	if th.Kind != "statement" || th.Role != "theorem" || th.Anchor != "widget" || th.NativeID != "widget" || th.TextRef != "content/wikitext/0000/100.wiki@r7#10-20" || th.Cue != "math-theorem" || th.Strength != "strong" {
		t.Errorf("theorem %+v", th)
	}
	if th.Locator != "Widget theorem (group theory), revision 7, section Statement, https://en.wikipedia.org/w/index.php?oldid=7#widget" {
		t.Errorf("locator %q", th.Locator)
	}
	if th.Structure == nil || !slices.Equal(th.Structure.Uses, []string{"wikipedia://en/100"}) || !slices.Equal(th.Structure.ProvedBy, []string{"wikipedia://en/100#PRF1"}) || th.Structure.In != "wikipedia://en/100" {
		t.Errorf("theorem structure %+v", th.Structure)
	}
	if p := got["100#PRF1"]; p.Kind != "proof" || p.AttachedTo != "wikipedia://en/100#THM1" || !slices.Equal(p.Structure.Uses, []string{"wikipedia://en/100#THM1"}) {
		t.Errorf("proof %+v", p)
	}
	if p := got["100#PRF2"]; p.AttachedTo != "wikipedia://en/100" {
		t.Errorf("proof of the lead %+v", p)
	}
	if g := got["100#GLO1"]; g.Kind != "definition" || g.MemberOf != "wikipedia://en/200" {
		t.Errorf("glossary entry %+v", g)
	}
	if l := got["100#LST1"]; l.Kind != inventory.Unknown || !slices.Equal(l.Hints, []string{"list-entry"}) || l.MemberOf != "" {
		t.Errorf("list entry %+v", l)
	}
	if g := got["100#GONE"]; g.Retired != 6 {
		t.Errorf("gone mention %+v", g)
	}
	if s := got["400"]; !slices.Equal(s.Flags, []string{"stopped"}) {
		t.Errorf("stopped mention %+v", s)
	}
	if s := got["300"]; s.Kind != "definition" || s.Structure != nil {
		t.Errorf("article with a stale edge file %+v", s)
	}

	var es []string
	for _, e := range res.Edges {
		es = append(es, strings.TrimPrefix(e.From, "wikipedia://en/")+" "+strings.TrimPrefix(e.To, "wikipedia://en/")+" "+e.Predicate)
		if e.By != wikipedia.Version || e.Revision != 7 {
			t.Errorf("edge %+v", e)
		}
	}
	wantEdges := []string{"100#PRF1 100#THM1 uses", "100#THM1 100 uses", "100#THM1 200 uses", "100#GLO1 200 glossary-of", "100 55 related", "100 Category:Theorems in group theory kind-of"}
	if !slices.Equal(es, wantEdges) {
		t.Errorf("edges\n%s\nwant\n%s", strings.Join(es, "\n"), strings.Join(wantEdges, "\n"))
	}

	tree := t.TempDir()
	if err := inventory.WriteMentions(tree, wikipedia.Source, res.Mentions); err != nil {
		t.Fatal(err)
	}
	if err := inventory.WriteEdges(tree, wikipedia.Source, res.Edges); err != nil {
		t.Fatal(err)
	}
	back, err := inventory.ReadEdges(tree, wikipedia.Source)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != len(res.Edges) {
		t.Errorf("read %d edges, want %d", len(back), len(res.Edges))
	}
	ms, err := inventory.ReadMentions(tree)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != len(res.Mentions) || ms[0].Page != 100 {
		t.Errorf("read %d mentions", len(ms))
	}
}

func TestImportNoCheckout(t *testing.T) {
	if _, err := wikipedia.Import(t.TempDir(), nil); err == nil || !strings.Contains(err.Error(), "not a git checkout") {
		t.Errorf("error %v", err)
	}
}

func TestKinds(t *testing.T) {
	for _, c := range []struct{ typ, title, kind string }{
		{wpcorpus.TypeConcept, "Group", "definition"},
		{wpcorpus.TypeMethod, "Euclidean algorithm", "definition"},
		{wpcorpus.TypeProblem, "Goldbach's conjecture", "conjecture"},
		{wpcorpus.TypeProblem, "Basel problem", "problem"},
		{wpcorpus.TypeField, "Group theory", "topic"},
		{wpcorpus.TypeList, "List of groups", "topic"},
	} {
		if k, _ := wikipedia.ArticleKind(c.typ, c.title, nil); k != c.kind {
			t.Errorf("%s %s: kind %s, want %s", c.typ, c.title, k, c.kind)
		}
	}
	for _, c := range []struct{ kind, mention, role string }{
		{"statement/lemma", "statement", "lemma"},
		{"statement/generalization", "statement", ""},
		{"example/counterexample", "counterexample", ""},
		{"application", "example", ""},
		{"definition/notation", "definition", ""},
		{"pointer-item", inventory.Unknown, ""},
	} {
		if k, r := wikipedia.KindOf(c.kind); k != c.mention || r != c.role {
			t.Errorf("%s: %s %s, want %s %s", c.kind, k, r, c.mention, c.role)
		}
	}
	if wikipedia.Name("Lagrange's theorem (group theory)") != "Lagrange's theorem" || wikipedia.Name("Group") != "Group" {
		t.Error("name")
	}
}

func TestImportPart(t *testing.T) {
	root := corpus(t)
	old := []inventory.Mention{
		{ID: wpcorpus.MentionID(100, "GONE"), Source: "wikipedia", Kind: "example", By: wikipedia.Version, Page: 100},
		{ID: wpcorpus.MentionID(400, ""), Source: "wikipedia", Kind: "definition", By: wikipedia.Version, Page: 400},
	}
	res, err := wikipedia.ImportPart(root, old, wikipedia.Filter{Type: wpcorpus.TypeConcept})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range res.Mentions {
		ids = append(ids, strings.TrimPrefix(m.ID, "wikipedia://en/"))
	}
	// The article 100 is a statement, so its mention of the earlier import
	// stays as it is, with no retirement.
	if want := []string{"100#GONE", "300", "400"}; !slices.Equal(ids, want) {
		t.Fatalf("mentions %v, want %v", ids, want)
	}
	if res.Articles != 1 || res.Retired != 0 || res.Stopped != 1 || res.Mentions[0].Retired != 0 {
		t.Errorf("articles %d, retired %d, stopped %d", res.Articles, res.Retired, res.Stopped)
	}
	res.KeepEdges([]inventory.Edge{
		{From: wpcorpus.MentionID(100, "THM1"), To: wpcorpus.MentionID(100, "")},
		{From: wpcorpus.MentionID(300, ""), To: wpcorpus.MentionID(100, "")},
		{From: wpcorpus.MentionID(400, ""), To: wpcorpus.MentionID(100, "")},
	})
	if len(res.Edges) != 1 || res.Edges[0].From != wpcorpus.MentionID(100, "THM1") {
		t.Errorf("kept edges %+v, want the edge from 100#THM1 only", res.Edges)
	}

	for _, c := range []struct {
		f    wikipedia.Filter
		want int
	}{
		{wikipedia.Filter{MSC: "20"}, 1},
		{wikipedia.Filter{MSC: "05"}, 0},
		{wikipedia.Filter{Since: "2026-01-01"}, 0},
		{wikipedia.Filter{}, 2},
	} {
		res, err := wikipedia.ImportPart(root, nil, c.f)
		if err != nil {
			t.Fatal(err)
		}
		if res.Articles != c.want {
			t.Errorf("filter %+v: %d articles, want %d", c.f, res.Articles, c.want)
		}
	}
}
