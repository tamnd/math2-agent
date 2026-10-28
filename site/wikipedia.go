package site

import (
	"cmp"
	"errors"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/wpcorpus"
)

// wpArticle is one article in scope on the Wikipedia pages of the site.
type wpArticle struct {
	PageID  int64  `json:"page_id"`
	Title   string `json:"title"`
	Signals string `json:"signals"`
	Class   string `json:"class,omitempty"`
	Rule    string `json:"rule"`
	URL     string `json:"url"`
}

// wpCount is one row of a count table of the Wikipedia pages.
type wpCount struct {
	Name       string
	Candidates int
	InScope    int
}

// wpLetter is one list of the articles in scope, by the first letter of the title.
type wpLetter struct {
	Name, URL string
	Count     int
}

// wpPageSize is the number of articles on one page of a list, so that a page stays near the weight budget of the site.
var wpPageSize = 1500

// pageDir gives the directory of page n of the list of a letter. Page 1 is the directory of the letter.
func pageDir(l string, n int) string {
	if n == 1 {
		return letterPath(l)
	}
	return letterPath(l) + "/" + strconv.Itoa(n)
}

// letterPath gives the directory of the list of a letter of letterOf.
func letterPath(l string) string {
	if l == "#" {
		return "wikipedia/other"
	}
	return "wikipedia/" + strings.ToLower(l)
}

func countRows(m map[string]*wpCount) []wpCount {
	out := make([]wpCount, 0, len(m))
	for _, c := range m {
		out = append(out, *c)
	}
	slices.SortFunc(out, func(x, y wpCount) int {
		return cmp.Or(cmp.Compare(y.InScope, x.InScope), cmp.Compare(y.Candidates, x.Candidates), cmp.Compare(x.Name, y.Name))
	})
	return out
}

// writeWikipedia writes the pages of the scope of the Wikipedia corpus (WP1.2): the counts, a list of the articles in scope for each letter, and api/wikipedia/scope.json. The pages hold the titles and the links, and no article text (WP9.8). With no corpus, or with a corpus that has no candidates.tsv, it writes nothing.
func (b *builder) writeWikipedia() error {
	if b.opt.Wikipedia == "" {
		return nil
	}
	f, err := os.Open(filepath.Join(b.opt.Wikipedia, wpcorpus.CandidatesPath)) // #nosec G304 -- the path is in the corpus
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	cs, err := wpcorpus.ReadCandidates(f)
	_ = f.Close()
	if err != nil {
		return err
	}
	rules, signals, classes := map[string]*wpCount{}, map[string]*wpCount{}, map[string]*wpCount{}
	add := func(m map[string]*wpCount, name string, in bool) {
		c := m[name]
		if c == nil {
			c = &wpCount{Name: name}
			m[name] = c
		}
		c.Candidates++
		if in {
			c.InScope++
		}
	}
	var in []wpArticle
	pending := 0
	for _, c := range cs {
		add(rules, c.Rule, c.InScope)
		for i := range 9 {
			if s := wpcorpus.Signals(1 << i); c.Signals&s != 0 {
				add(signals, s.String(), c.InScope)
			}
		}
		if c.Verdict == wpcorpus.VerdictPending {
			pending++
		}
		if !c.InScope {
			continue
		}
		class := c.Class
		if class == "" {
			class = "(none)"
		}
		add(classes, class, true)
		in = append(in, wpArticle{PageID: c.PageID, Title: strings.ReplaceAll(c.Title, "_", " "), Signals: c.Signals.String(), Class: c.Class, Rule: c.Rule, URL: "https://en.wikipedia.org/wiki/" + url.PathEscape(c.Title)})
	}
	slices.SortFunc(in, func(x, y wpArticle) int {
		return cmp.Or(cmp.Compare(strings.ToLower(x.Title), strings.ToLower(y.Title)), cmp.Compare(x.PageID, y.PageID))
	})
	byLetter := map[string][]wpArticle{}
	for _, a := range in {
		l := letterOf(a.Title)
		byLetter[l] = append(byLetter[l], a)
	}
	var letters []wpLetter
	for _, l := range slices.Sorted(maps.Keys(byLetter)) {
		letters = append(letters, wpLetter{Name: l, URL: b.url(letterPath(l) + "/"), Count: len(byLetter[l])})
	}
	var sigs []wpCount
	for i := range 9 {
		if c := signals[wpcorpus.Signals(1<<i).String()]; c != nil {
			sigs = append(sigs, *c)
		}
	}
	d := struct {
		Candidates, InScope, Pending int
		Rules, Signals, Classes      []wpCount
		Letters                      []wpLetter
		JSON                         string
	}{len(cs), len(in), pending, countRows(rules), sigs, countRows(classes), letters, b.url("api/wikipedia/scope.json")}
	desc := "The Wikipedia articles that the corpus of Math 2.0 takes from Wikipedia, and why."
	if err := b.writePage("wikipedia", "wikipedia", d, page{Title: "Wikipedia scope", Description: desc, Section: "wikipedia"}); err != nil {
		return err
	}
	type listData struct {
		Letter   string
		Letters  []wpLetter
		Count    int // the number of articles of the letter
		Pages    []wpLetter
		Page     int
		Articles []wpArticle
	}
	for _, l := range letters {
		title := "Wikipedia articles in scope, " + l.Name
		if l.Name == "#" {
			title = "Wikipedia articles in scope, other"
		}
		all := byLetter[l.Name]
		var pages []wpLetter
		for i := 0; i < len(all); i += wpPageSize {
			pages = append(pages, wpLetter{Name: strconv.Itoa(len(pages) + 1), URL: b.url(pageDir(l.Name, len(pages)+1) + "/"), Count: min(wpPageSize, len(all)-i)})
		}
		if len(pages) == 1 {
			pages = nil
		}
		for i := 0; i < len(all); i += wpPageSize {
			n := i/wpPageSize + 1
			t := title
			if pages != nil {
				t += ", page " + strconv.Itoa(n)
			}
			d := listData{l.Name, letters, len(all), pages, n, all[i:min(i+wpPageSize, len(all))]}
			if err := b.writePage(pageDir(l.Name, n), "wikipedialist", d, page{Title: t, Description: desc, Section: "wikipedia"}); err != nil {
				return err
			}
		}
	}
	return b.writeJSON("api/wikipedia/scope.json", struct {
		Candidates int         `json:"candidates"`
		InScope    int         `json:"in_scope"`
		Pending    int         `json:"pending"`
		Articles   []wpArticle `json:"articles"`
	}{len(cs), len(in), pending, in})
}

// HasWikipedia says if the site has the pages of the Wikipedia scope.
func (b *builder) HasWikipedia() bool {
	if b.opt.Wikipedia == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(b.opt.Wikipedia, wpcorpus.CandidatesPath))
	return err == nil
}
