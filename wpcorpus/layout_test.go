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
	if got := ObjectsPath(42, 7); got != filepath.FromSlash("analysis/objects/0000/42-r7.jsonl") {
		t.Errorf("ObjectsPath = %s", got)
	}
	if got := CensusPath(42, 7); got != filepath.FromSlash("analysis/census/0000/42-r7.json") {
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
