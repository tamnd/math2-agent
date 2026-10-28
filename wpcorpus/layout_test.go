package wpcorpus

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPaths(t *testing.T) {
	if got := WikitextPath(1234567); got != filepath.FromSlash("content/wikitext/0123/1234567.wiki") {
		t.Errorf("WikitextPath = %s", got)
	}
	if got := ObjectsPath(42, 7); got != filepath.FromSlash("analysis/objects/0000/42-r7.md") {
		t.Errorf("ObjectsPath = %s", got)
	}
	if got := CensusPath(42, 7); got != filepath.FromSlash("analysis/census/0000/42-r7.md") {
		t.Errorf("CensusPath = %s", got)
	}
}

func TestContentRoundTrip(t *testing.T) {
	root := t.TempDir()
	p := &Page{ID: 98765, Title: "Widget (algebra)", Revision: 111, Timestamp: "2026-10-27T00:00:00Z", Wikitext: "A '''widget''' is a set.\n<!-- a comment -->\n"}
	if err := WriteContent(root, p); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, WikitextPath(p.ID)))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"licence: CC BY-SA 4.0", "curid=98765&action=history", "oldid=111"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("the header has no %q", want)
		}
	}
	q, err := ReadContent(root, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if *q != *p {
		t.Errorf("ReadContent = %+v, want %+v", q, p)
	}
	ids, err := Pages(root)
	if err != nil || !slices.Equal(ids, []int{98765}) {
		t.Errorf("Pages = %v, %v", ids, err)
	}
}

func TestParseContentErrors(t *testing.T) {
	for _, s := range []string{"no header", "<!-- wikipedia: en\n page: 1", "<!-- wikipedia: en\n     page: x\n     title: a\n     revision: 1 -->\n", "<!-- wikipedia: en\n     title: a -->\n"} {
		if _, err := ParseContent([]byte(s)); err == nil {
			t.Errorf("ParseContent(%q) gives no error", s)
		}
	}
}

func TestPagesEmpty(t *testing.T) {
	ids, err := Pages(t.TempDir())
	if err != nil || len(ids) != 0 {
		t.Errorf("Pages of an empty corpus = %v, %v", ids, err)
	}
}

func TestResultRoundTrip(t *testing.T) {
	root := t.TempDir()
	r := extractFile(t, "widget.wiki", "Widget")
	if err := WriteResult(root, r); err != nil {
		t.Fatal(err)
	}
	objs, err := ReadObjects(root, 12345, 678)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != len(r.Objects) || objs[0].Text != r.Objects[0].Text || objs[0].Hash != r.Objects[0].Hash {
		t.Errorf("ReadObjects gives %d objects, want %d", len(objs), len(r.Objects))
	}
	c, err := ReadCensus(root, 12345, 678)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Cues) != len(r.Census.Cues) || c.Counts.InlineHeads != r.Census.Counts.InlineHeads || c.Extractor != "wpcorpus-test" {
		t.Errorf("ReadCensus = %+v", c)
	}
	data, err := os.ReadFile(filepath.Join(root, ObjectsPath(12345, 678)))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `\u003c`) {
		t.Error("the objects file escapes HTML")
	}
}

func TestObjectsMarkdown(t *testing.T) {
	objs := []Object{
		{Page: 5, Revision: 9, Ref: "o1", Kind: "subject", Cue: "lead", Strength: StrengthNormal, Name: "Widget", Span: [2]int{0, 10}, Text: "A widget is a set.\n\n## Proof\n\nIt is $x$.", Math: []Math{{Form: "math", TeX: `\mathbb{Z}/2`, Source: `<math>\mathbb{Z}/2</math>`, Span: [2]int{1, 2}}}, Hash: "h1", Extractor: "m2/test"},
		{Page: 5, Revision: 9, Ref: "o2", Kind: "proof", Cue: "item/proof", Strength: StrengthStrong, Span: [2]int{10, 20}, Hash: "h2", Extractor: "m2/test"},
		{Page: 5, Revision: 9, Ref: "o3", Kind: "example", Cue: "item/example", Strength: StrengthWeak, Span: [2]int{20, 30}, Text: "## o3 is not a heading here", Hash: "h3", Extractor: "m2/test"},
	}
	data, err := MarshalObjects(5, 9, "Widget", objs)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"---\npage: 5\nrevision: 9\ntitle: Widget\n", "span: [0, 10]", "\n# Widget\n", "\n## o1 subject: Widget\n\nA widget is a set.\n", "\n## o2 proof\n\n## o3 example\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("the objects file has no %q:\n%s", want, s)
		}
	}
	got, err := ParseObjects(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(objs) {
		t.Fatalf("ParseObjects gives %d objects, want %d", len(got), len(objs))
	}
	for i := range objs {
		if got[i].Text != objs[i].Text || got[i].Page != 5 || got[i].Revision != 9 || got[i].Hash != objs[i].Hash || got[i].Span != objs[i].Span {
			t.Errorf("object %d: got %+v, want %+v", i, got[i], objs[i])
		}
	}
	if got[0].Math[0].TeX != `\mathbb{Z}/2` {
		t.Errorf("the TeX is %q", got[0].Math[0].TeX)
	}
	for _, bad := range []string{"no header", "---\npage: 5\n", "---\npage: 5\nobjects:\n  - ref: o1\n---\n\n## o2 x\n"} {
		if _, err := ParseObjects([]byte(bad)); err == nil {
			t.Errorf("ParseObjects(%q) gives no error", bad)
		}
	}
}

func TestObjectsMarkdownHeadingInText(t *testing.T) {
	objs := []Object{{Ref: "o1", Kind: "subject", Text: "a\n## o2 b"}, {Ref: "o2", Kind: "proof"}}
	if _, err := MarshalObjects(1, 2, "T", objs); err == nil {
		t.Error("MarshalObjects gives no error for a text with the heading of the next object")
	}
}
