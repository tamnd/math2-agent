package wpcorpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// extractRev extracts a wikitext as the revision rev of the page 12345.
func extractRev(t *testing.T, wikitext string, rev int) *Result {
	t.Helper()
	m, err := Load(fixture)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Extract(&Page{ID: 12345, Revision: rev, Title: "Widget", Wikitext: wikitext}, m, "wpcorpus-test")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// tagOf gives the tag of the object with a name.
func tagOf(t *testing.T, r *Result, name string) string {
	t.Helper()
	for _, o := range r.Objects {
		if o.Name == name {
			return o.Tag
		}
	}
	t.Fatalf("no object %q", name)
	return ""
}

func TestAssignTags(t *testing.T) {
	root := t.TempDir()
	src, err := os.ReadFile(filepath.Join("testdata/pages", "widget.wiki"))
	if err != nil {
		t.Fatal(err)
	}
	a := extractRev(t, string(src), 1)
	run, err := AssignTags(root, a)
	if err != nil {
		t.Fatal(err)
	}
	if run.Count(PassNew) != len(a.Objects) || run.From != 0 {
		t.Fatalf("the first run: %d new of %d objects, from %d", run.Count(PassNew), len(a.Objects), run.From)
	}
	seen := map[string]bool{}
	for _, o := range a.Objects {
		if !IsWPTag(o.Tag) || seen[o.Tag] {
			t.Errorf("the object %s has the tag %q", o.Ref, o.Tag)
		}
		seen[o.Tag] = true
	}
	if err := WriteResult(root, a); err != nil {
		t.Fatal(err)
	}

	// The revision 2 moves the section Gadget theorem before Definition,
	// changes a word of the widget lemma, and removes the second proof.
	text := string(src)
	gadget := "== Gadget theorem ==\nA {{dfn|gadget}} is a widget with one element.\n\n"
	text = strings.Replace(text, gadget, "", 1)
	text = strings.Replace(text, "== Definition ==", gadget+"== Definition ==", 1)
	text = strings.Replace(text, "{{math proof|title=A second proof|proof=Use <math>\\frac{1}{2}</math>.}}\n", "", 1)
	text = strings.Replace(text, "Each map {{mvar|f}} is injective.", "Each map {{mvar|f}} is injective too.", 1)
	b := extractRev(t, text, 2)
	run, err = AssignTags(root, b)
	if err != nil {
		t.Fatal(err)
	}
	if run.From != 1 || run.To != 2 {
		t.Errorf("the run is from %d to %d", run.From, run.To)
	}
	for _, name := range []string{"Gadget theorem", "gadget", "Widget lemma", "Theorem 1", "Definition"} {
		if tagOf(t, a, name) != tagOf(t, b, name) {
			t.Errorf("the object %q has a new tag", name)
		}
	}
	gone := tagOf(t, a, "A second proof")
	if run.Pass[gone] != PassGone {
		t.Errorf("the second proof: pass %q", run.Pass[gone])
	}
	if run.Count(PassNew) != 0 {
		t.Errorf("%d new tags", run.Count(PassNew))
	}
	reg, err := ReadTags(root, 12345)
	if err != nil {
		t.Fatal(err)
	}
	if len(reg) != len(a.Objects) {
		t.Errorf("the register has %d lines, want %d", len(reg), len(a.Objects))
	}
	for _, l := range reg {
		if (l.Tag == gone) != (l.Gone == "r2") {
			t.Errorf("the line %s", l)
		}
	}
	runs, err := os.ReadFile(filepath.Join(root, RunsPath(12345)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runs), "-,r1,") || !strings.Contains(string(runs), "r1,r2,"+gone+",gone\n") {
		t.Errorf("the runs file:\n%s", runs)
	}

	// The same revision again changes no tag and adds no run.
	if err := WriteResult(root, b); err != nil {
		t.Fatal(err)
	}
	if err := RemoveStale(root, 12345, 2); err != nil {
		t.Fatal(err)
	}
	c := extractRev(t, text, 2)
	if _, err := AssignTags(root, c); err != nil {
		t.Fatal(err)
	}
	for i := range c.Objects {
		if c.Objects[i].Tag != b.Objects[i].Tag {
			t.Errorf("the object %s has the tag %s, then %s", c.Objects[i].Ref, b.Objects[i].Tag, c.Objects[i].Tag)
		}
	}
	again, _ := os.ReadFile(filepath.Join(root, RunsPath(12345)))
	if string(again) != string(runs) {
		t.Errorf("the same revision added a run")
	}
}

func TestReadTagsErrors(t *testing.T) {
	for _, data := range []string{"03QK\n", "03QK,o1\n03QK,o2\n", "03qk,o1\n", "03QK,o1,lost\n"} {
		root := t.TempDir()
		if err := writeFile(filepath.Join(root, TagsPath(1)), []byte(data)); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadTags(root, 1); err == nil {
			t.Errorf("ReadTags(%q) gives no error", data)
		}
	}
}

func TestMatchTagsNear(t *testing.T) {
	old := []Object{
		{Tag: "0AAA", Kind: "example", Section: "Examples", Text: "the integers under addition form a group with the identity zero and the inverse minus x"},
		{Tag: "0BBB", Kind: "example", Section: "Examples", Text: "the rational numbers without zero form a group under multiplication"},
		{Tag: "0CCC", Kind: "statement", Section: "Properties", Hash: "h1", Name: "Fact"},
		{Tag: "0DDD", Kind: "statement", Section: "Properties", Hash: "h1", Name: "Fact"},
	}
	cur := []Object{
		{Kind: "example", Section: "Examples of groups", Text: "the integers under addition form a group with the identity zero and the inverse minus x ."},
		{Kind: "example", Section: "History", Text: "the rational numbers without zero form a group under multiplication"},
		{Kind: "statement", Section: "Properties", Hash: "h1", Name: "Fact"},
	}
	tags, passes := MatchTags(old, cur)
	if tags[0] != "0AAA" || passes[0] != PassNear {
		t.Errorf("the first example: %s %s", tags[0], passes[0])
	}
	// The section History is not near the section Examples.
	if tags[1] != "" {
		t.Errorf("the second example: %s %s", tags[1], passes[1])
	}
	// Two old objects have the same name and hash, so no pass can choose.
	if tags[2] != "" {
		t.Errorf("the fact: %s %s", tags[2], passes[2])
	}
}
