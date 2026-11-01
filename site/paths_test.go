package site

import (
	"slices"
	"testing"
)

func TestHeadings(t *testing.T) {
	data := []byte(`# The file header is not a heading.
courses:
  - course: a
    items:
      - {name: x, kind: concept}
      # Second part
      - {name: y, kind: statement}
  - course: b
    items:
      - {name: z, kind: concept}
`)
	got := headings(data)
	want := []string{"", "Second part", ""}
	if !slices.Equal(got, want) {
		t.Fatalf("headings = %q, want %q", got, want)
	}
}

func TestNewPath(t *testing.T) {
	b := &builder{nodes: map[string]*nodeView{"AAAAAA": {Slug: "x"}, "BBBBBB": {Slug: "z"}}}
	items := []rawItem{{Name: "x", Node: "AAAAAA"}, {Name: "y"}, {Name: "z", Node: "BBBBBB"}}
	p := b.newPath("a", "A", "/learn/a/", items, []string{"", "Two", "Two"})
	if len(p.Sections) != 2 || p.Done != 2 || p.Sections[1].Done != 1 || p.Percent() != 66 {
		t.Fatalf("sections %d, done %d, second done %d, percent %d", len(p.Sections), p.Done, p.Sections[1].Done, p.Percent())
	}
	b.steps = map[*nodeView][]pathStep{}
	b.addSteps(p)
	s := b.steps[b.nodes["BBBBBB"]]
	if len(s) != 1 || s[0].N != 2 || s[0].Of != 2 || s[0].Prev != b.nodes["AAAAAA"] || s[0].Next != nil {
		t.Fatalf("steps of z = %+v", s)
	}
}
