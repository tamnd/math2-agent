package wpcorpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func render(t *testing.T, src string) string {
	t.Helper()
	m, err := Load(fixture)
	if err != nil {
		t.Fatal(err)
	}
	p := &Page{ID: 12345, Revision: 678, Title: "Widget", Timestamp: "2026-01-02T03:04:05Z", Wikitext: src}
	r, err := Extract(p, m, "wpcorpus-test")
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(title string) int {
		if title == "Small widget" {
			return 99
		}
		return 0
	}
	md, err := Render(p, m, r, resolve, "wpcorpus-test")
	if err != nil {
		t.Fatal(err)
	}
	return string(md)
}

func TestRender(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata/pages", "widget.wiki"))
	if err != nil {
		t.Fatal(err)
	}
	md := render(t, string(src))

	head, body, err := splitFrontMatter([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	var h renderHeader
	if err := yaml.Unmarshal(head, &h); err != nil {
		t.Fatal(err)
	}
	if h.Page != 12345 || h.Revision != 678 || h.Licence != licence || h.Source != "https://en.wikipedia.org/w/index.php?oldid=678" {
		t.Errorf("header = %+v", h)
	}
	for _, want := range []string{
		"# Widget\n",
		"A **widget** is a set $W$ with a map $f$.[^o2]",
		"## Definition {#Definition}",
		"A widget is a pair $(W, f)$.",
		"**Widget lemma.**\n\nEach map $f$ is injective.",
		"$$f(x) = x \\tag{1}$$",
		"A **gadget** is a widget with one element.",
		"- [Small widget](wikipedia://en/99)",
		"- [Gadget](https://en.wikipedia.org/wiki/Gadget)",
		"| Name | Size |\n|---|---|\n",
		"[^o2]: Doe. (2001). \"Widgets\"",
		"[^n1]: Doe 2001, 3.",
		"::: {#o9 .statement kind=statement/lemma cue=math-theorem name=\"Widget lemma\"}",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("the rendering has no %q:\n%s", want, md)
		}
	}
	for _, bad := range []string{"{{", "}}", "'''", "<ref", "[[", "Category:"} {
		if strings.Contains(string(body), bad) {
			t.Errorf("the rendering has %q", bad)
		}
	}
	// The note of a named ref that is used two times is written once.
	if n := strings.Count(md, "[^o2]:"); n != 1 {
		t.Errorf("the note o2 is written %d times", n)
	}
	// Each fence that opens closes.
	open, closed := 0, 0
	for _, l := range strings.Split(md, "\n") {
		switch {
		case strings.HasPrefix(l, "::: {"):
			open++
		case l == ":::":
			closed++
		}
	}
	if open == 0 || open != closed {
		t.Errorf("%d fences open and %d close", open, closed)
	}
}

func TestRenderTable(t *testing.T) {
	md := render(t, "{| class=\"wikitable\"\n|-\n! A !! B !! C\n|-\n|rowspan=2| x || 1 || 2\n|-\n| 3 || 4\n|-\n|colspan=3|\n|-\n|colspan=2| wide || 5\n|}\n")
	want := "| A | B | C |\n|---|---|---|\n| x | 1 | 2 |\n|  | 3 | 4 |\n| wide |  | 5 |\n"
	if !strings.Contains(md, want) {
		t.Errorf("the table is not\n%s\nin\n%s", want, md)
	}
	if md := render(t, "{| class=\"infobox\"\n|-\n| a || b\n|}\nText.\n"); strings.Contains(md, "| a |") {
		t.Errorf("the infobox is rendered:\n%s", md)
	}
}

func TestRenderInline(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"[[File:Cube.svg|thumb|right|The cube.|alt=A cube]]\n", "*Image: Cube.svg.* The cube."},
		{"A cost of $5 and 2*3 [sic].\n", `A cost of \$5 and 2\*3 \[sic\].`},
		{"Is\n:<math>x = 1.</math>\n", "\n\n$$x = 1.$$\n"},
		{"# not a list\n", "1. not a list"},
		{"A {{unknown|kept}} word.\n", "A kept word."},
		{"See [https://example.org the site].\n", "See [the site](https://example.org)."},
		{"<code>a_b</code> and <sup>2</sup>.\n", "`a_b` and <sup>2</sup>."},
	} {
		if md := render(t, c.src); !strings.Contains(md, c.want) {
			t.Errorf("%q: the rendering has no %q:\n%s", c.src, c.want, md)
		}
	}
}
