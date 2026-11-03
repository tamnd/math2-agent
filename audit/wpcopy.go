package audit

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/copycheck"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/wpcorpus"
	"github.com/tamnd/math2-agent/wpsource"
)

// wpText is the Markdown rendering of an article of the content plane, as the copy check reads it.
type wpText struct {
	page     int64
	title    string
	revision int64
	text     string // the body with no fences of objects and no link targets
}

var (
	wpFence = regexp.MustCompile(`(?m)^:::.*$`)
	wpLink  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
)

// wpTextOf reads the Markdown rendering of a page in the checkout, or gives nil when the checkout does not have the page.
func (a *auditor) wpTextOf(page int64) (*wpText, error) {
	if t, ok := a.wpTexts[page]; ok {
		return t, nil
	}
	if a.wpTexts == nil {
		a.wpTexts = map[int64]*wpText{}
	}
	rel := wpcorpus.MarkdownPath(int(page))
	data, err := os.ReadFile(filepath.Join(a.wikipedia, rel)) // #nosec G304 -- the path is in the checkout that the user names.
	if errors.Is(err, fs.ErrNotExist) {
		a.wpTexts[page] = nil
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rest, ok := bytes.CutPrefix(data, []byte("---\n"))
	head, body, ok2 := bytes.Cut(rest, []byte("\n---\n"))
	if !ok || !ok2 {
		return nil, fmt.Errorf("%s: the file has no YAML header", rel)
	}
	var h struct {
		Title    string `yaml:"title"`
		Revision int64  `yaml:"revision"`
	}
	if err := yaml.Unmarshal(head, &h); err != nil {
		return nil, fmt.Errorf("%s: %w", rel, err)
	}
	text := wpLink.ReplaceAllString(wpFence.ReplaceAllString(string(body), ""), "$1")
	t := &wpText{page: page, title: h.Title, revision: h.Revision, text: text}
	a.wpTexts[page] = t
	return t, nil
}

// wpURI gives the URI of a revision of an article in the copy check.
func wpURI(page, rev int64) string { return fmt.Sprintf("wikipedia://en/%d@r%d", page, rev) }

// l04Wikipedia compares the body of each node with the Wikipedia articles that the node is linked to (corpus wikipedia WP9.4). These are the articles of the wikipedia sources of the node and the articles of the Wikipedia mentions of its rows. A match with the revision that a source item attributes, in a section that the item lists, is allowed. A match must be an exact run of copycheck.MatchRun characters or more, or a near copy. Each other match is a finding of rule L04: a match with another revision, a match in a section that no item lists, or a match with an article that the node does not attribute. The rule needs the checkout of tamnd/wikipedia-math, and it reads the Markdown rendering of each article. The checkout has only the revision of record of each article.
func (a *auditor) l04Wikipedia(o *outcome) {
	if a.wikipedia == "" {
		return
	}
	inv := a.inventory()
	for _, n := range a.scopeNodes() {
		if retired(n) {
			continue
		}
		allowed := map[string][]string{}  // the URI of an attributed revision, to the sections of its source items
		attributed := map[int64][]int64{} // the page, to the revisions that the node attributes
		var pages []int64
		byURL := map[string]bool{} // the titles of the articles that a source item names only by URL
		list, _ := n.Meta["sources"].([]any)
		for _, v := range list {
			m, _ := v.(map[string]any)
			if t, ok := wpURLTitle(str(m["url"])); ok {
				byURL[t] = true
			}
			ref, err := wpsource.Parse(str(m[wpsource.Key]))
			if err != nil {
				continue
			}
			pages = append(pages, ref.Page)
			if note := strings.TrimSpace(str(m["note"])); note != noteAdopted && note != noteAdapted {
				continue
			}
			u := wpURI(ref.Page, ref.Revision)
			allowed[u] = append(allowed[u], strs(m["sections"])...)
			attributed[ref.Page] = append(attributed[ref.Page], ref.Revision)
		}
		if inv.err == nil {
			for _, id := range strs(n.Meta["inventory"]) {
				if r := inv.byID[id]; r != nil {
					for _, m := range r.Mentions {
						if p, ok := wpPageOf(m); ok {
							pages = append(pages, p)
						}
					}
				}
			}
		}
		slices.Sort(pages)
		var texts []copycheck.Text
		arts := map[string]*wpText{}
		for _, p := range slices.Compact(pages) {
			t, err := a.wpTextOf(p)
			if err != nil {
				o.add(a.wikipedia, 0, 0, "%v", err)
				return
			}
			if t == nil {
				continue
			}
			u := wpURI(t.page, t.revision)
			arts[u] = t
			texts = append(texts, copycheck.Text{Source: "wikipedia", URI: u, Licence: "CC-BY-SA-4.0", Class: ingest.Open, Text: t.text})
		}
		if len(texts) == 0 {
			continue
		}
		o.checked++
		a.wpCopyFindings(o, n, copycheck.New(texts, nil), arts, allowed, attributed, byURL)
	}
}

// wpCopyFindings checks the sections of the body of n against the corpus and adds one finding for each section and article with a match that is not allowed.
func (a *auditor) wpCopyFindings(o *outcome, n *node.Node, c *copycheck.Corpus, arts map[string]*wpText, allowed map[string][]string, attributed map[int64][]int64, byURL map[string]bool) {
	for _, s := range check.Sections(n) {
		r := c.Check(copycheck.Section{Field: s.Name, Text: s.Text})
		seen := map[string]bool{}
		for _, f := range r.Findings {
			// A short run that covers a share of a short section is often the first words of a definition, such as "an irreducible element of an integral domain is", thus only a long run or a near copy counts.
			short := f.Stage == 1 && f.Length < copycheck.MatchRun
			if !f.Match || short || seen[f.URI] || slices.Contains(allowed[f.URI], s.Name) {
				continue
			}
			seen[f.URI] = true
			t := arts[f.URI]
			revs := attributed[t.page]
			switch {
			case len(revs) > 0 && !slices.Contains(revs, t.revision):
				o.add(n.Path, s.Line, 1, "the section %s matches revision %d of the Wikipedia article %s, and the node attributes revision %d. Check the text against the revision of record and update the source item", s.Name, t.revision, t.title, revs[0])
			case len(revs) > 0:
				o.add(n.Path, s.Line, 1, "the section %s matches the Wikipedia article %s, and no source item of the article lists the section. Add the section to sections of the source item, or write the text again", s.Name, t.title)
			case byURL[t.title]:
				o.add(n.Path, s.Line, 1, "the section %s matches the Wikipedia article %s, and the node names the article only by URL. Give the source item the key wikipedia with the revision %d, the note adopted or adapted and the sections (WP9.3), or write the text again", s.Name, t.title, t.revision)
			default:
				o.add(n.Path, s.Line, 1, "the section %s matches the Wikipedia article %s (revision %d), and the node does not attribute the article. Write the text again, or adopt it with the attribution of WP9.3", s.Name, t.title, t.revision)
			}
		}
	}
}

// wpPageOf gives the page id of a Wikipedia mention id.
func wpPageOf(mention string) (int64, bool) {
	rest, ok := strings.CutPrefix(mention, "wikipedia://en/")
	if !ok {
		return 0, false
	}
	rest, _, _ = strings.Cut(rest, "#")
	p, err := strconv.ParseInt(rest, 10, 64)
	return p, err == nil
}

// wpURLTitle gives the title of the article of a URL of the English Wikipedia.
func wpURLTitle(u string) (string, bool) {
	i := strings.Index(u, "en.wikipedia.org/wiki/")
	if i < 0 {
		return "", false
	}
	t, _, _ := strings.Cut(u[i+len("en.wikipedia.org/wiki/"):], "#")
	if s, err := url.PathUnescape(t); err == nil {
		t = s
	}
	return strings.ReplaceAll(t, "_", " "), t != ""
}
