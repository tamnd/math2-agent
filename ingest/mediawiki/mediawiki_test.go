package mediawiki_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest/mediawiki"
)

const dump = `<mediawiki xmlns="http://www.mediawiki.org/xml/export-0.11/" version="0.11">
  <siteinfo>
    <sitename>A Wiki</sitename>
    <base>https://wiki.example/wiki/Main_Page</base>
    <namespaces>
      <namespace key="0" case="first-letter" />
      <namespace key="102" case="first-letter">Definition</namespace>
    </namespaces>
  </siteinfo>
  <page>
    <title>Definition:Group</title>
    <ns>102</ns>
    <id>7</id>
    <revision><id>1</id><timestamp>2026-01-01T00:00:00Z</timestamp><contributor><username>Old</username></contributor><text>old</text></revision>
    <revision><id>2</id><timestamp>2026-03-18T19:22:39Z</timestamp><contributor><username>New</username></contributor><text>new</text></revision>
  </page>
  <page>
    <title>Group</title>
    <ns>0</ns>
    <id>8</id>
    <redirect title="Definition:Group" />
    <revision><id>3</id><contributor><ip>192.0.2.1</ip></contributor><text>#REDIRECT [[Definition:Group]]</text></revision>
  </page>
</mediawiki>`

func TestWalk(t *testing.T) {
	var pages []mediawiki.Page
	site, err := mediawiki.Walk(strings.NewReader(dump), func(s *mediawiki.Site, p *mediawiki.Page) error {
		if s.Name != "A Wiki" {
			t.Errorf("the site of %s is %+v", p.Title, s)
		}
		pages = append(pages, *p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if site.Base != "https://wiki.example/wiki/Main_Page" || site.Namespaces[102] != "Definition" || site.Namespaces[0] != "" {
		t.Errorf("the site is %+v", site)
	}
	want := []mediawiki.Page{
		{Title: "Definition:Group", NS: 102, ID: 7, Revision: 2, Timestamp: "2026-03-18T19:22:39Z", Contributor: "New", Text: "new"},
		{Title: "Group", NS: 0, ID: 8, Redirect: "Definition:Group", Revision: 3, Contributor: "192.0.2.1", Text: "#REDIRECT [[Definition:Group]]"},
	}
	if !slices.Equal(pages, want) {
		t.Errorf("the pages are\n%+v, want\n%+v", pages, want)
	}
	if n := pages[0].Name(site); n != "Group" {
		t.Errorf("the name of Definition:Group is %q", n)
	}
	if n := pages[1].Name(site); n != "Group" {
		t.Errorf("the name of Group is %q", n)
	}

	stop := errors.New("stop")
	if _, err := mediawiki.Walk(strings.NewReader(dump), func(*mediawiki.Site, *mediawiki.Page) error { return stop }); !errors.Is(err, stop) {
		t.Errorf("Walk returns %v, want the error of fn", err)
	}
	none := func(*mediawiki.Site, *mediawiki.Page) error { return nil }
	for name, in := range map[string]string{
		"not a dump":     `<html><body/></html>`,
		"a page first":   `<page><title>A</title><revision><id>1</id></revision></page>`,
		"no title":       `<mediawiki><page><ns>0</ns><revision><id>1</id></revision></page></mediawiki>`,
		"no revision":    `<mediawiki><page><title>A</title><ns>0</ns></page></mediawiki>`,
		"bad XML":        `<mediawiki><page><title>A</title>`,
		"an empty input": ``,
	} {
		if _, err := mediawiki.Walk(strings.NewReader(in), none); err == nil {
			t.Errorf("Walk of %s gives no error", name)
		}
	}
}

func TestCategories(t *testing.T) {
	text := "Text.\n[[Category:Group Theory]]\n[[Category:Finite_Groups|Sort key]]\n[[category: Group Theory ]]\n<!-- [[Category:Hidden]] -->\n[[:Category:Linked]]"
	got := mediawiki.Categories(text)
	if want := []string{"Group Theory", "Finite Groups"}; !slices.Equal(got, want) {
		t.Errorf("Categories gives %q, want %q", got, want)
	}
	if got := mediawiki.Categories("no categories"); got != nil {
		t.Errorf("Categories of a page with no categories gives %q", got)
	}
}

func TestIncluded(t *testing.T) {
	text := "Before.<onlyinclude>\nThe first part.\n</onlyinclude>Between.<onlyinclude> </onlyinclude><onlyinclude>The second part.</onlyinclude>After."
	if got, want := mediawiki.Included(text), "The first part.\n\nThe second part."; got != want {
		t.Errorf("Included gives %q, want %q", got, want)
	}
	if got := mediawiki.Included("no parts"); got != "" {
		t.Errorf("Included of a page with no parts gives %q", got)
	}
}

func TestPlain(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"links", "A [[Definition:Group|group]] is a [[Monoid]] and a [[:Category:Algebra]].", "A group is a Monoid and a Category:Algebra."},
		{"a link in a label", "See [[A|the [[B]] page]].", "See the B page."},
		{"files", "Text [[File:Cayley.png|thumb|A [[Cayley table]]]] here.", "Text  here."},
		{"images", "[[Image:A.png]]Text.", "Text."},
		{"external links", "See [https://example.org the site] and [//example.org/a].", "See the site and ."},
		{"math", "Let <math>x^2</math> be <math display=\"block\">y</math>.", "Let $x^2$ be $y$."},
		{"templates", "{{about|a {{nested}} template}}Text{{qed}}.", "Text."},
		{"a template that does not end", "Text {{open|a", "Text {{open|a"},
		{"braces in mathematics", "The set $\\set {{x}}$ and {{template}}.", "The set $\\set {{x}}$ and ."},
		{"comments and refs", "A<!-- note -->B<ref name=\"a\">note</ref>C<ref name=\"b\" />D", "ABCD"},
		{"bold and italic", "'''Bold''' and ''italic''.", "Bold and italic."},
		{"headings", "== Theorem ==\nText.\n=== Proof ===", "Theorem\nText.\nProof"},
		{"lists", "* one\n# two\n: three\n;four", "one\ntwo\nthree\nfour"},
		{"tags", "A<br/>B <span class=\"x\">C</span>", "AB C"},
		{"categories", "Text.\n[[Category:Groups]]", "Text."},
		{"blank lines", "A\n\n\n\n\nB", "A\n\nB"},
	} {
		if got := mediawiki.Plain(tc.in); got != tc.want {
			t.Errorf("Plain of %s gives %q, want %q", tc.name, got, tc.want)
		}
	}
}
