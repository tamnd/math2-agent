// Package mediawiki reads the XML dumps of a MediaWiki site, such as Wikipedia and ProofWiki (corpus K5.11). The adapters of these sites use it: Walk gives each page of a dump, and Plain, Included and Categories read the wikitext of a page.
//
// Walk reads the export format of MediaWiki, versions 0.10 and 0.11, one page after the other, so a large dump does not have to fit in memory. It gives the last revision of each page in the dump.
package mediawiki

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Site is the site information at the start of a dump.
type Site struct {
	Name       string         // the name of the site, for example ProofWiki
	Base       string         // the URL of the main page, for example https://proofwiki.org/wiki/Main_Page
	Namespaces map[int]string // the name of each namespace, for example 102 is Definition. The main namespace 0 has the name "".
}

// Page is one page of a dump with its last revision.
type Page struct {
	Title       string // the full title, with the namespace, for example Definition:Group
	NS          int    // the namespace
	ID          int64  // the page id
	Redirect    string // the title that the page redirects to, or ""
	Revision    int64  // the id of the revision
	Timestamp   string // the time of the revision, for example 2026-03-18T19:22:39Z
	Contributor string // the user name or the IP address of the author of the revision
	Text        string // the wikitext of the revision
}

// Name returns the title of the page without its namespace, as the namespaces of the site give it.
func (p *Page) Name(s *Site) string {
	if ns := s.Namespaces[p.NS]; ns != "" {
		return strings.TrimPrefix(p.Title, ns+":")
	}
	return p.Title
}

// xmlPage is a page as the export format writes it.
type xmlPage struct {
	Title    string `xml:"title"`
	NS       int    `xml:"ns"`
	ID       int64  `xml:"id"`
	Redirect *struct {
		Title string `xml:"title,attr"`
	} `xml:"redirect"`
	Revisions []struct {
		ID          int64  `xml:"id"`
		Timestamp   string `xml:"timestamp"`
		Contributor struct {
			Username string `xml:"username"`
			IP       string `xml:"ip"`
		} `xml:"contributor"`
		Text string `xml:"text"`
	} `xml:"revision"`
}

// xmlSite is the site information as the export format writes it.
type xmlSite struct {
	Name       string `xml:"sitename"`
	Base       string `xml:"base"`
	Namespaces []struct {
		Key  int    `xml:"key,attr"`
		Name string `xml:",chardata"`
	} `xml:"namespaces>namespace"`
}

// Walk reads a dump and calls fn for each page, in the order of the dump. It returns the site information. Walk fails when the input is not a MediaWiki dump, on a page with no title or no revision, and with the first error from fn.
func Walk(r io.Reader, fn func(*Site, *Page) error) (*Site, error) {
	dec := xml.NewDecoder(r)
	site := &Site{Namespaces: map[int]string{}}
	root := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "mediawiki":
			root = true
		case "siteinfo":
			var s xmlSite
			if err := dec.DecodeElement(&s, &start); err != nil {
				return nil, err
			}
			site.Name, site.Base = s.Name, s.Base
			for _, ns := range s.Namespaces {
				site.Namespaces[ns.Key] = strings.TrimSpace(ns.Name)
			}
		case "page":
			if !root {
				return nil, errors.New("the input is not a MediaWiki dump")
			}
			var x xmlPage
			if err := dec.DecodeElement(&x, &start); err != nil {
				return nil, err
			}
			if x.Title == "" || len(x.Revisions) == 0 {
				return nil, fmt.Errorf("the page %d has no title or no revision", x.ID)
			}
			rev := x.Revisions[len(x.Revisions)-1]
			p := &Page{Title: x.Title, NS: x.NS, ID: x.ID, Revision: rev.ID, Timestamp: rev.Timestamp, Contributor: rev.Contributor.Username, Text: rev.Text}
			if p.Contributor == "" {
				p.Contributor = rev.Contributor.IP
			}
			if x.Redirect != nil {
				p.Redirect = x.Redirect.Title
			}
			if err := fn(site, p); err != nil {
				return nil, err
			}
		}
	}
	if !root {
		return nil, errors.New("the input is not a MediaWiki dump")
	}
	return site, nil
}

var (
	category    = regexp.MustCompile(`\[\[\s*[Cc]ategory\s*:\s*([^\]|]+?)\s*(?:\|[^\]]*)?\]\]`)
	included    = regexp.MustCompile(`(?s)<onlyinclude>(.*?)</onlyinclude>`)
	comment     = regexp.MustCompile(`(?s)<!--.*?-->`)
	ref         = regexp.MustCompile(`(?s)<ref[^>/]*/>|<ref[^>]*>.*?</ref>`)
	math        = regexp.MustCompile(`(?s)<math[^>]*>(.*?)</math>`)
	tag         = regexp.MustCompile(`</?[A-Za-z][A-Za-z0-9]*(?:\s[^<>]*)?/?>`)
	link        = regexp.MustCompile(`\[\[([^\[\]|]*)(?:\|([^\[\]]*))?\]\]`)
	external    = regexp.MustCompile(`\[(?:https?:)?//[^\s\]]+(?:\s+([^\]]*))?\]`)
	heading     = regexp.MustCompile(`(?m)^(=+)\s*(.*?)\s*=+\s*$`)
	quotes      = regexp.MustCompile(`'{2,}`)
	blank       = regexp.MustCompile(`\n{3,}`)
	listMarkers = regexp.MustCompile(`(?m)^[*#:;]+\s*`)
)

// Categories returns the categories of a page, in their order in the wikitext and each one time, without the prefix Category:.
func Categories(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range category.FindAllStringSubmatch(comment.ReplaceAllString(text, ""), -1) {
		c := strings.ReplaceAll(m[1], "_", " ")
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// Included returns the parts of the wikitext in <onlyinclude> tags, with a blank line between them, or "" when the page has no such part. A page that other pages include puts its statement there.
func Included(text string) string {
	var parts []string
	for _, m := range included.FindAllStringSubmatch(text, -1) {
		if p := strings.TrimSpace(m[1]); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "\n\n")
}

// Plain returns the text of wikitext without its markup, for the fingerprints of the copy check. It removes comments, references, templates, files and categories, and the markup of links, headings, lists, bold and italic. A link gives its label, or its target when it has no label. Mathematics stays as LaTeX: <math>x</math> gives $x$.
func Plain(text string) string {
	text = comment.ReplaceAllString(text, "")
	text = ref.ReplaceAllString(text, "")
	text = math.ReplaceAllString(text, "$$$1$$")
	text = templates(text)
	text = category.ReplaceAllString(text, "")
	for {
		next := link.ReplaceAllStringFunc(text, func(s string) string {
			m := link.FindStringSubmatch(s)
			target := strings.TrimSpace(m[1])
			if ns, _, ok := strings.Cut(target, ":"); ok && (strings.EqualFold(ns, "file") || strings.EqualFold(ns, "image")) {
				return ""
			}
			if m[2] != "" {
				return m[2]
			}
			return strings.TrimPrefix(target, ":")
		})
		if next == text {
			break
		}
		text = next
	}
	text = external.ReplaceAllString(text, "$1")
	text = tag.ReplaceAllString(text, "")
	text = heading.ReplaceAllString(text, "$2")
	text = quotes.ReplaceAllString(text, "")
	text = listMarkers.ReplaceAllString(text, "")
	text = blank.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

// templates removes each template and each transclusion, {{...}}, with the templates in it. Mathematics between two dollar signs stays, because LaTeX can have two braces in a row. The rest of a template that does not end stays.
func templates(text string) string {
	var b strings.Builder
	depth, start, open := 0, 0, 0
	for i := 0; i < len(text); i++ {
		switch {
		case text[i] == '$' && depth == 0:
			if j := strings.IndexByte(text[i+1:], '$'); j >= 0 {
				i += j + 1
			}
		case strings.HasPrefix(text[i:], "{{"):
			if depth == 0 {
				b.WriteString(text[start:i])
				open = i
			}
			depth++
			i++
		case strings.HasPrefix(text[i:], "}}") && depth > 0:
			depth--
			i++
			if depth == 0 {
				start = i + 1
			}
		}
	}
	if depth > 0 {
		start = open
	}
	b.WriteString(text[start:])
	return b.String()
}
