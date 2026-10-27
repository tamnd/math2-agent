// Package proofwiki is the ingest adapter of ProofWiki, the wiki of mathematical proofs (spec 09.2, corpus K5.3). It gives one record for each result, proof, definition and axiom in the daily XML dump of the site.
//
// The content of ProofWiki is under CC BY-SA 3.0 and the GNU Free Documentation Licence (https://proofwiki.org/wiki/ProofWiki:Copyrights). The page names the GFDL with no version, so any version applies. math2 elects CC BY-SA 3.0, as corpus K5.2.2 asks, so the source has the class open. The licence is share-alike, so the inventory keeps no text, and rule L03 asks that a node that uses the text names the page in same_as under proofwiki.
//
// The adapter reads the pages of the main namespace, of Definition and of Axiom. It skips the redirects, the disambiguation pages and the main page. The id of a record is the full title of the page, for example Definition:Group or Lagrange's Theorem (Group Theory)/Proof 1. The kind is definition, axiom or proof, or for a page of the main namespace the first heading of the page in lower case: theorem, proposition, lemma, corollary, conjecture, open question, problem or definition. It is page for any other heading. A subpage whose last part is Proof or Proof N is a proof. The title is the title without the namespace. The URL is the page, and the locator is the permanent link to the revision of the dump.
//
// The text of a record is the plain text of the parts in <onlyinclude> tags, where ProofWiki puts the statement or the proof. A page with no such part gives its plain text up to the section Sources. The licence gate keeps the text with its fingerprints. The meta of a record has these keys, each one only when the page has a value:
//
//	categories   the categories of the page, with | between them. They are crosswalk data, not the taxonomy of math2 (corpus K10.2).
//	proves       of a proof: the title of the page that it proves
//	revision     the id of the revision of the dump
//	date         the date of the revision, for example 2026-03-18
//	contributor  the author of the revision. The history of the page names all the authors.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/proofwiki"
package proofwiki

import (
	"errors"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/mediawiki"
)

// Home is the home of ProofWiki.
const Home = "https://proofwiki.org"

// Dump is the daily XML dump of the site, with the last revision of each page.
const Dump = Home + "/xmldump/latest.xml.gz"

// The namespaces that the adapter reads.
const (
	Main       = 0
	Axiom      = 100
	Definition = 102
)

// Adapter is the adapter of ProofWiki.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of ProofWiki.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "proofwiki",
		Name:        "ProofWiki",
		Home:        Home,
		Licence:     "CC-BY-SA-3.0 OR GFDL-1.1-or-later",
		Election:    "CC-BY-SA-3.0",
		Class:       ingest.Open,
		Attribution: "The ProofWiki contributors, ProofWiki, " + Home + ", CC BY-SA 3.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "proofwiki/1" }

// Dump returns the daily XML dump.
func (Adapter) Dump() string { return Dump }

var (
	firstHeading = regexp.MustCompile(`(?m)^==\s*([^=].*?)\s*==\s*$`)
	proofPage    = regexp.MustCompile(`^Proof(?: [0-9]+)?$`)
	disambig     = regexp.MustCompile(`(?i)\{\{\s*disambiguation\s*[|}]`)
	sources      = regexp.MustCompile(`(?m)^==\s*Sources\s*==\s*$`)
)

// headings maps the first heading of a page of the main namespace to its kind.
var headings = map[string]string{
	"Theorem": "theorem", "Theorems": "theorem", "Proposition": "proposition", "Lemma": "lemma", "Corollary": "corollary",
	"Conjecture": "conjecture", "Open Question": "open question", "Problem": "problem", "Classic Problem": "problem", "Definition": "definition",
}

// Read reads the XML dump and emits one record for each page that it reads. Read fails when the input is not a MediaWiki dump, on a title that does not pass crosswalk.Check, and on a dump with no pages to read.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	n := 0
	_, err := mediawiki.Walk(r, func(site *mediawiki.Site, p *mediawiki.Page) error {
		if (p.NS != Main && p.NS != Axiom && p.NS != Definition) || p.Redirect != "" || p.Title == "Main Page" || disambig.MatchString(p.Text) {
			return nil
		}
		if err := crosswalk.Check("proofwiki", p.Title); err != nil {
			return err
		}
		rec := record(site, p)
		n++
		return emit(rec)
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("the dump has no results, definitions or axioms")
	}
	return nil
}

// record gives the record of a page.
func record(site *mediawiki.Site, p *mediawiki.Page) ingest.Record {
	rec := ingest.Record{
		ID:      p.Title,
		Kind:    kind(p),
		Title:   p.Name(site),
		URL:     Page(p.Title),
		Locator: Home + "/w/index.php?title=" + url.QueryEscape(underscores(p.Title)) + "&oldid=" + strconv.FormatInt(p.Revision, 10),
		Meta:    map[string]string{"revision": strconv.FormatInt(p.Revision, 10)},
	}
	text := mediawiki.Included(p.Text)
	if text == "" {
		text = p.Text
		if loc := sources.FindStringIndex(text); loc != nil {
			text = text[:loc[0]]
		}
	}
	rec.Text = mediawiki.Plain(text)
	if cs := mediawiki.Categories(p.Text); len(cs) > 0 {
		rec.Meta["categories"] = strings.Join(cs, "|")
	}
	if rec.Kind == "proof" {
		parent, _ := cut(p.Title)
		rec.Meta["proves"] = parent
	}
	if date, _, _ := strings.Cut(p.Timestamp, "T"); date != "" {
		rec.Meta["date"] = date
	}
	if p.Contributor != "" {
		rec.Meta["contributor"] = p.Contributor
	}
	return rec
}

// kind gives the kind of a page.
func kind(p *mediawiki.Page) string {
	switch p.NS {
	case Definition:
		return "definition"
	case Axiom:
		return "axiom"
	}
	if _, last := cut(p.Title); proofPage.MatchString(last) {
		return "proof"
	}
	if m := firstHeading.FindStringSubmatch(p.Text); m != nil {
		if k, ok := headings[mediawiki.Plain(m[1])]; ok {
			return k
		}
	}
	return "page"
}

// cut splits the title of a subpage at its last slash. A page that is not a subpage gives "" and the title.
func cut(title string) (parent, last string) {
	i := strings.LastIndexByte(title, '/')
	if i < 0 {
		return "", title
	}
	return title[:i], title[i+1:]
}

// Page returns the URL of the page with a title, for example Definition:Group.
func Page(title string) string {
	parts := strings.Split(underscores(title), "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return Home + "/wiki/" + strings.Join(parts, "/")
}

func underscores(title string) string { return strings.ReplaceAll(title, " ", "_") }
