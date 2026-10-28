package wpcorpus

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const fixture = "testdata/corpus"

func extractFile(t *testing.T, name, title string) *Result {
	t.Helper()
	m, err := Load(fixture)
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join("testdata/pages", name))
	if err != nil {
		t.Fatal(err)
	}
	r, err := Extract(&Page{ID: 12345, Revision: 678, Title: title, Wikitext: string(src)}, m, "wpcorpus-test")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func find(r *Result, cue, name string) *Object {
	for i := range r.Objects {
		o := &r.Objects[i]
		if o.Cue == cue && o.Name == name {
			return o
		}
	}
	return nil
}

func TestExtractItems(t *testing.T) {
	r := extractFile(t, "widget.wiki", "Widget")
	tests := []struct {
		cue, name, kind, attached string
	}{
		{"lead", "Widget", "subject", ""},
		{"heading", "Definition", "definition", ""},
		{"inline-head", "Theorem 1", "statement/theorem", ""},
		{"inline-head", "Proof", "proof", "Theorem 1"},
		{"math-theorem", "Widget lemma", "statement/lemma", ""},
		{"collapse", "Proof of the widget lemma", "proof", "Widget lemma"},
		{"numblk", "1", "equation", ""},
		{"named-heading", "Gadget theorem", "statement/theorem", ""},
		{"dfn", "gadget", "definition", ""},
		{"table-row", "Tiny widget", "table-row", ""},
	}
	byRef := map[string]*Object{}
	for i := range r.Objects {
		byRef[r.Objects[i].Ref] = &r.Objects[i]
	}
	for _, tt := range tests {
		o := find(r, tt.cue, tt.name)
		if o == nil {
			t.Errorf("no object %s %q", tt.cue, tt.name)
			continue
		}
		if o.Kind != tt.kind {
			t.Errorf("%s %q: kind %s, want %s", tt.cue, tt.name, o.Kind, tt.kind)
		}
		if tt.attached != "" {
			if a := byRef[o.AttachedTo]; a == nil || a.Name != tt.attached {
				t.Errorf("%s %q: attached to %q, want %q", tt.cue, tt.name, o.AttachedTo, tt.attached)
			}
		}
		if o.Page != 12345 || o.Revision != 678 || o.Extractor != "wpcorpus-test" || !strings.HasPrefix(o.Hash, "sha256:") {
			t.Errorf("%s %q: bad record %+v", tt.cue, tt.name, o)
		}
	}

	// A proof heading under a statement heading, directly after a
	// math theorem, proves that theorem (WP3.7 rule 2).
	var proofHeading *Object
	for i := range r.Objects {
		o := &r.Objects[i]
		if o.Cue == "heading" && o.Kind == "proof" {
			proofHeading = o
		}
	}
	if proofHeading == nil || byRef[proofHeading.AttachedTo].Name != "Widget lemma" {
		t.Errorf("the proof heading is not attached to the lemma: %+v", proofHeading)
	}
	// A proof directly after another proof has no target.
	if o := find(r, "math-proof", "A second proof"); o == nil || o.AttachedTo != "" || !slices.Contains(o.Flags, "orphan") {
		t.Errorf("the second proof is not an orphan: %+v", o)
	}
}

func TestExtractFields(t *testing.T) {
	r := extractFile(t, "widget.wiki", "Widget")
	lead := find(r, "lead", "Widget")
	if lead.Text != "A widget is a set $W$ with a map $f$." {
		t.Errorf("lead text %q", lead.Text)
	}
	if lead.Anchor != "widgets" {
		t.Errorf("lead anchor %q", lead.Anchor)
	}
	if len(lead.Math) != 2 || lead.Math[0].Form != "math" || lead.Math[1].Form != "mvar" || lead.Math[1].TeX != "f" {
		t.Errorf("lead math %+v", lead.Math)
	}
	if len(lead.Citations) != 1 {
		t.Fatalf("lead citations %v", lead.Citations)
	}
	var cite *Object
	for i := range r.Objects {
		if r.Objects[i].Ref == lead.Citations[0] {
			cite = &r.Objects[i]
		}
	}
	if cite == nil || cite.Kind != "citation" || cite.Fields["title"] != "Widgets" || cite.Fields["template"] != "cite book" {
		t.Errorf("lead citation %+v", cite)
	}

	// A named ref that is used again gives the same citation.
	thm := find(r, "inline-head", "Theorem 1")
	if !slices.Equal(thm.Citations, lead.Citations) {
		t.Errorf("theorem citations %v, want %v", thm.Citations, lead.Citations)
	}
	if !strings.Contains(thm.Text, "The fixed point is unique.") {
		t.Errorf("the inline head does not hold the indented block: %q", thm.Text)
	}

	def := find(r, "heading", "Definition")
	if !slices.Contains(def.Flags, "citation-needed") {
		t.Errorf("definition flags %v", def.Flags)
	}
	if def.Section != "Definition" || def.SectionAnchor != "Definition" {
		t.Errorf("definition section %q %q", def.Section, def.SectionAnchor)
	}
	lemma := find(r, "math-theorem", "Widget lemma")
	if lemma.Text != "Each map $f$ is injective." || lemma.Section != "Definition > Statement" {
		t.Errorf("lemma %q in %q", lemma.Text, lemma.Section)
	}
	if o := find(r, "math-proof", "A second proof"); o.Math[0].TeX != `\frac{1}{2}` {
		t.Errorf("proof math %+v", o.Math)
	}
	if o := find(r, "collapse", "Proof of the widget lemma"); !slices.Contains(o.Flags, "collapsed") {
		t.Errorf("collapse flags %v", o.Flags)
	}
	if o := find(r, "table-row", "Tiny widget"); len(o.Links) != 1 || o.Links[0].Title != "Tiny widget" {
		t.Errorf("row links %+v", o.Links)
	}
	for _, o := range r.Objects {
		if o.Kind == "section" && (o.Text != "" || o.Math != nil) {
			t.Errorf("section %q keeps its text or its math", o.Name)
		}
		for _, l := range o.Links {
			if strings.HasPrefix(l.Title, "Category:") {
				t.Errorf("%s has a category link", o.Ref)
			}
		}
	}
}

func TestCensus(t *testing.T) {
	r := extractFile(t, "widget.wiki", "Widget")
	c := r.Census
	if c.Page != 12345 || c.Revision != 678 || c.Type != "article" || c.Strength != StrengthWeak {
		t.Errorf("census header %+v", c)
	}
	if c.Manifests["templates"] != 3 || c.Manifests["headings"] != 2 || c.Manifests["waivers"] != 1 {
		t.Errorf("manifest versions %v", c.Manifests)
	}
	if g := c.Gaps(); len(g) != 0 {
		t.Errorf("gaps %+v", g)
	}
	n := c.Counts
	if n.ItemTemplates["math theorem"] != 1 || n.ItemTemplates["math proof"] != 1 || n.InlineHeads != 2 || n.NumberedBlocks != 1 || n.ProofBlocks != 1 || n.TableRows != 1 || n.NamedHeadings != 1 || n.Anchors != 1 {
		t.Errorf("counts %+v", n)
	}
	if n.Citations != 3 || n.Math["math"] != 3 || n.Math["mvar"] != 2 || n.Math["template"] != 1 || n.Maintenance["citation needed"] != 1 {
		t.Errorf("counts %+v", n)
	}
	if c.Unmapped["unknown thing"] != 1 || len(c.Unmapped) != 1 {
		t.Errorf("unmapped templates %v", c.Unmapped)
	}
	if !slices.Equal(c.UnmappedHeadings, []string{"equations", "odd heading"}) {
		t.Errorf("unmapped headings %v", c.UnmappedHeadings)
	}

	// The Example heading matches the waiver, so it has no object.
	var waived int
	for _, q := range c.Cues {
		if q.Waiver != "" {
			waived++
			if q.Waiver != "example-in-test" || q.Object != "" || q.Reason == "" {
				t.Errorf("waived cue %+v", q)
			}
		}
	}
	if waived != 1 {
		t.Errorf("%d waived cues, want 1", waived)
	}
	refs := map[string]bool{}
	for _, o := range r.Objects {
		refs[o.Ref] = true
	}
	for _, q := range c.Cues {
		if q.Object != "" && !refs[q.Object] {
			t.Errorf("cue %+v names no object", q)
		}
	}
}

func TestGaps(t *testing.T) {
	c := Census{Cues: []Cue{{Cue: "a", Object: "o1"}, {Cue: "b", Waiver: "w"}, {Cue: "c"}}}
	if g := c.Gaps(); len(g) != 1 || g[0].Cue != "c" {
		t.Errorf("gaps %+v", g)
	}
}

func TestGlossary(t *testing.T) {
	r := extractFile(t, "glossary.wiki", "Glossary of widgets")
	if r.Census.Type != "glossary" {
		t.Fatalf("type %s", r.Census.Type)
	}
	for _, name := range []string{"alpha", "aleph"} {
		if o := find(r, "glossary-term", name); o == nil || o.Kind != "entry" {
			t.Errorf("no entry %q", name)
		}
	}
	if o := find(r, "glossary-term", "alpha"); o.Text != "alpha\nThe first word. See beta." || o.Links[0].Title != "Beta" {
		t.Errorf("alpha %q %+v", o.Text, o.Links)
	}
	beta := find(r, "term", "beta")
	if beta == nil || !strings.Contains(beta.Text, "The second word, with $b$.") {
		t.Fatalf("beta %+v", beta)
	}
	var defn int
	for _, q := range r.Census.Cues {
		if q.Cue == "defn" {
			defn++
			if q.Object != beta.Ref {
				t.Errorf("the defn cue goes to %s, want %s", q.Object, beta.Ref)
			}
		}
	}
	if defn != 1 || r.Census.Counts.GlossaryEntries != 3 {
		t.Errorf("%d defn cues and %d entries", defn, r.Census.Counts.GlossaryEntries)
	}
	if len(r.Census.UnmappedHeadings) != 0 {
		t.Errorf("the letter headings are unmapped: %v", r.Census.UnmappedHeadings)
	}
}

func TestList(t *testing.T) {
	r := extractFile(t, "list.wiki", "List of widgets")
	if r.Census.Counts.ListEntries != 2 {
		t.Errorf("%d list entries, want 2", r.Census.Counts.ListEntries)
	}
	o := find(r, "list-entry", "Blue widget")
	if o == nil || len(o.Links) != 1 || o.Links[0] != (Link{Title: "Blue widget", Anchor: "Size"}) {
		t.Errorf("blue widget %+v", o)
	}
}

func TestDeterministic(t *testing.T) {
	a := extractFile(t, "widget.wiki", "Widget")
	for range 5 {
		b := extractFile(t, "widget.wiki", "Widget")
		if len(a.Objects) != len(b.Objects) {
			t.Fatal("the object count changes")
		}
		for i := range a.Objects {
			if a.Objects[i].Hash != b.Objects[i].Hash || a.Objects[i].Ref != b.Objects[i].Ref || !slices.Equal(a.Objects[i].Flags, b.Objects[i].Flags) {
				t.Fatalf("object %d changes", i)
			}
		}
	}
}

func TestArticleType(t *testing.T) {
	tests := map[string]string{
		"Glossary of group theory":          "glossary",
		"List of finite simple groups":      "list",
		"Lists of integrals":                "list",
		"Outline of algebra":                "outline",
		"Timeline of calculus":              "timeline",
		"Lagrange's theorem (group theory)": "statement",
		"Cauchy–Schwarz inequality":         "statement",
		"Group (mathematics)":               "article",
	}
	for title, want := range tests {
		if got := ArticleType(title); got != want {
			t.Errorf("ArticleType(%q) = %s, want %s", title, got, want)
		}
	}
}

func TestKindOfName(t *testing.T) {
	tests := map[string]string{
		"Lemma 2":                   "statement/lemma",
		"Sylow theorems":            "statement/theorem",
		"Proof of Zorn's lemma":     "proof",
		"Counterexample to the law": "example/counterexample",
		"Cauchy's inequality":       "statement",
		"Motivation":                "x",
	}
	for name, want := range tests {
		if got := kindOfName(name, "x"); got != want {
			t.Errorf("kindOfName(%q) = %s, want %s", name, got, want)
		}
	}
}

func TestLinkTarget(t *testing.T) {
	tests := []struct {
		in   string
		want *Link
	}{
		{"group (mathematics)", &Link{Title: "Group (mathematics)"}},
		{"Finite difference#Newton's series", &Link{Title: "Finite difference", Anchor: "Newton's_series"}},
		{"Order_of_a_group", &Link{Title: "Order of a group"}},
		{"Category:Group theory", nil},
		{":Category:Group theory", nil},
		{"File:Cayley graph.svg", nil},
		{"fr:Groupe", nil},
		{"wikt:group", nil},
		{"#Proof", nil},
	}
	for _, tt := range tests {
		got := linkTarget(tt.in)
		if (got == nil) != (tt.want == nil) || got != nil && *got != *tt.want {
			t.Errorf("linkTarget(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestHash(t *testing.T) {
	if hash("A  group\nis") != hash("a group is") {
		t.Error("the hash does not fold case and space")
	}
	if hash("a group") == hash("a ring") {
		t.Error("two texts give one hash")
	}
}

// TestExtractPageFault checks that a fault in the extractor gives an error
// for the page and does not stop the caller.
func TestExtractPageFault(t *testing.T) {
	_, err := ExtractPage(&Page{ID: 7, Revision: 1, Title: "Widget", Wikitext: "{{Theorem|A widget is a gadget.}}\n== Proof ==\nText."}, nil, "wpcorpus-test", nil)
	if err == nil || !strings.Contains(err.Error(), "page 7: the extraction failed") {
		t.Errorf("err %v", err)
	}
}
