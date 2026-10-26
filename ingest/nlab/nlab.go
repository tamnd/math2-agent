// Package nlab is the ingest adapter of the nLab, the wiki on category theory and higher mathematics (corpus K5.3). It gives one record for each page of the git mirror ncatlab/nlab-content.
//
// The nLab states no licence. The site allows use when the user acknowledges the source, and it gives no right to adapt. Thus the source has the class record (corpus K5.3), and math2 keeps only the page names and the link graph. The adapter gives the source of each page as the text of the record. The licence gate keeps only the fingerprints of the text and drops the text, and rule L02 uses the fingerprints to find nLab text in nodes/.
//
// The bulk channel is the git archive of the master branch, and the catalogue records its commit. In the mirror, each page is a directory pages/.../<n>/ with the file name, which holds the name of the page, and the file content.md, which holds its source. The id and the title of a record are the name of the page, which is also the form of the nLab ids of Wikidata (P4215). The URL is the page on the site, and the locator is the directory of the page at the commit of the snapshot. The meta of a record has these keys:
//
//	links      the names of the pages that the page links to, with "|" between them
//	redirects  the other names of the page, from its !redirects lines, with "|" between them
//
// A key is not in the meta when the page has no value for it.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/nlab"
package nlab

import (
	"context"
	"fmt"
	"io"
	"maps"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/ingest"
)

// Home is the home of the nLab. The page with the name N is Home/show/N.
const Home = "https://ncatlab.org/nlab"

// Repository is the git mirror of the pages of the nLab.
const Repository = "https://github.com/ncatlab/nlab-content"

// Archive is the git archive of the master branch of the mirror.
const Archive = Repository + "/archive/refs/heads/master.tar.gz"

// Adapter is the adapter of the nLab.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of the nLab.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "nlab",
		Name:        "nLab",
		Home:        Home,
		Licence:     "LicenseRef-unknown",
		Class:       ingest.RecordOnly,
		Attribution: "The nLab authors, nLab, " + Home,
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "nlab/1" }

// Dump returns the git archive of the master branch.
func (Adapter) Dump() string { return Archive }

// Fetch gets the git archive at req.URL and sets req.Commit to its commit.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	return ingest.FetchArchive(ctx, req, w)
}

// page is the part of one page that the archive has given.
type page struct {
	name, content string
	hasName       bool
	hasContent    bool
}

// Read reads a git archive of the mirror and emits one record for each page. Read fails on a page with no name or no content, on a name that does not pass crosswalk.Check, on a name that two pages have and on an archive with no pages.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	pages := map[string]*page{}
	seen := map[string]string{}
	n := 0
	_, err := ingest.WalkArchive(r, func(name string, f io.Reader) error {
		dir, file := path.Split(name)
		if !strings.HasPrefix(name, "pages/") || (file != "name" && file != "content.md") {
			return nil
		}
		dir = strings.TrimSuffix(dir, "/")
		b, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		p := pages[dir]
		if p == nil {
			p = &page{}
			pages[dir] = p
		}
		if file == "name" {
			p.name, p.hasName = strings.TrimSpace(string(b)), true
		} else {
			p.content, p.hasContent = string(b), true
		}
		if !p.hasName || !p.hasContent {
			return nil
		}
		delete(pages, dir)
		if err := crosswalk.Check("nlab", p.name); err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		if first, dup := seen[p.name]; dup {
			return fmt.Errorf("%s and %s both have the name %q", first, dir, p.name)
		}
		seen[p.name] = dir
		n++
		return emit(record(dir, p))
	})
	if err != nil {
		return err
	}
	if len(pages) > 0 {
		dir := slices.Min(slices.Collect(maps.Keys(pages)))
		if !pages[dir].hasName {
			return fmt.Errorf("%s has no name", dir)
		}
		return fmt.Errorf("%s has no content.md", dir)
	}
	if n == 0 {
		return fmt.Errorf("the archive has no pages")
	}
	return nil
}

// record makes the record of one page.
func record(dir string, p *page) ingest.Record {
	links, redirects := Links(p.content)
	redirects = slices.DeleteFunc(redirects, func(n string) bool { return n == p.name })
	meta := map[string]string{}
	if len(links) > 0 {
		meta["links"] = strings.Join(links, "|")
	}
	if len(redirects) > 0 {
		meta["redirects"] = strings.Join(redirects, "|")
	}
	if len(meta) == 0 {
		meta = nil
	}
	return ingest.Record{
		ID:      p.name,
		Title:   p.name,
		URL:     PageURL(p.name),
		Locator: dir,
		Text:    p.content,
		Meta:    meta,
	}
}

// PageURL returns the URL of the page with the name on the site. The site writes a space in a name as +.
func PageURL(name string) string {
	return Home + "/show/" + strings.ReplaceAll(url.PathEscape(name), "%20", "+")
}

// link matches a wiki link [[target]] or [[target|text]].
var link = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

// Links returns the names of the pages that the source of a page links to, and the names of its !redirects lines, each sorted and with no name two times. A link loses its text after | and its anchor after #. The !include lines and the other commands that start with ! are not links.
func Links(content string) (links, redirects []string) {
	ls, rs := map[string]bool{}, map[string]bool{}
	for _, m := range link.FindAllStringSubmatch(content, -1) {
		target := m[1]
		if cmd, rest, ok := strings.Cut(target, " "); ok && strings.HasPrefix(cmd, "!") {
			if cmd == "!redirects" {
				if name := strings.Join(strings.Fields(rest), " "); name != "" {
					rs[name] = true
				}
			}
			continue
		}
		if strings.HasPrefix(target, "!") {
			continue
		}
		target, _, _ = strings.Cut(target, "|")
		target, _, _ = strings.Cut(target, "#")
		if target = strings.Join(strings.Fields(target), " "); target != "" {
			ls[target] = true
		}
	}
	return slices.Sorted(maps.Keys(ls)), slices.Sorted(maps.Keys(rs))
}
