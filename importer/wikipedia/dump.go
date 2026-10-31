// This file is the ingest adapter of the English Wikipedia (spec 09.2, corpus K5.3), with the version wikipedia/1. It came from the package ingest/wikipedia (corpus wikipedia WP12.4). It gives one record for each article that the math2 tree names, from the monthly multistream XML dump. The weekly refresh of one page uses its range read, and the copy check uses its text.
//
// The dump has all the articles of the wiki, 27 GB with compression. The adapter does not download it. A multistream dump is a list of bzip2 streams of 100 pages each, and its index gives the offset of the stream of each title. Fetch downloads the index, finds the titles that the tree names, and gets only their streams with HTTP range requests, one at a time, with a pause between them. The tree names a title in the same_as or close_to edges of a node under wikipedia, or in the crosswalk ids of a mention, for example of a theorem of the 1000+ list. A title that is a redirect gets the page that it redirects to, in a second pass over the index. The snapshot is the XML of the pages that Fetch found, in the export format of MediaWiki. Fetch does not read the live site (corpus K5.10).
//
// The text is under CC BY-SA 4.0 (https://en.wikipedia.org/wiki/Wikipedia:Copyrights), so the source has the class open. Most text is also under the GFDL, but not all of it, so math2 uses CC BY-SA 4.0. The licence is share-alike, so the inventory keeps no text, and rule L03 asks that a node that uses the text names the article in same_as under wikipedia.
//
// The adapter reads the articles of the main namespace. It skips the disambiguation pages, and it reads a redirect only to know the other titles of an article. The id of a record is the title of the article, for example Lagrange's theorem (group theory), and the kind is article. The URL is the article, and the locator is the permanent link to the revision of the dump. The text is the plain text of the article up to its first closing section, such as See also or References, for the copy check. The meta of a record has these keys, each one only when the article has a value:
//
//	wikipedia    the title of the article and the titles of the redirects to it in the snapshot, with | between them
//	description  the short description of the article
//	categories   the categories of the article, with | between them. They are crosswalk data, not the taxonomy of math2 (corpus K10.2).
//	revision     the id of the revision of the dump
//	date         the date of the revision, for example 2026-09-30
//	contributor  the author of the revision. The history of the article names all the authors.
//
// The package cli imports this package, so the init of this file registers the adapter in the m2 command.

package wikipedia

import (
	"bufio"
	"bytes"
	"compress/bzip2"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"maps"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/mediawiki"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/node"
)

// Home is the home of the English Wikipedia.
const Home = "https://en.wikipedia.org"

// Dump is the latest multistream dump of the articles. A step of the import plan pins a dated run, such as https://dumps.wikimedia.org/enwiki/20261001/enwiki-20261001-pages-articles-multistream.xml.bz2.
const Dump = "https://dumps.wikimedia.org/enwiki/latest/enwiki-latest-pages-articles-multistream.xml.bz2"

// Adapter is the adapter of the English Wikipedia. Pause is the time between two requests of a fetch. The registered adapter waits one second.
type Adapter struct {
	Pause time.Duration
}

func init() { ingest.Register(Adapter{Pause: time.Second}) }

// Source returns the catalogue entry of the English Wikipedia.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "wikipedia",
		Name:        "Wikipedia",
		Home:        Home,
		Licence:     "CC-BY-SA-4.0",
		Class:       ingest.Open,
		Attribution: "The Wikipedia contributors, Wikipedia, " + Home + ", CC BY-SA 4.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "wikipedia/1" }

// Dump returns the latest multistream dump.
func (Adapter) Dump() string { return Dump }

// Index returns the URL of the index of a multistream dump.
func Index(dump string) (string, error) {
	base, ok := strings.CutSuffix(dump, "-multistream.xml.bz2")
	if !ok {
		return "", fmt.Errorf("%s is not a multistream dump: the name does not end in -multistream.xml.bz2", dump)
	}
	return base + "-multistream-index.txt.bz2", nil
}

// Titles returns the Wikipedia titles that the tree at root names, in order: the same_as and close_to edges of the nodes under wikipedia and the crosswalk ids of the mentions. A title has spaces, not underscores, and its first letter is a capital, as in the dump. Titles drops a title with a section, such as Group (mathematics)#Notes.
func Titles(root string) ([]string, error) {
	nodes, err := node.Load(root)
	if err != nil {
		return nil, err
	}
	ms, err := inventory.ReadMentions(root)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	add := func(t string) {
		if t, ok := title(t); ok {
			seen[t] = true
		}
	}
	for _, n := range nodes {
		if same, ok := n.Meta["same_as"].(map[string]any); ok {
			if s, ok := same["wikipedia"].(string); ok {
				add(s)
			}
		}
		if near, ok := n.Meta["close_to"].([]any); ok {
			for _, c := range near {
				if m, ok := c.(map[string]any); ok && m["system"] == "wikipedia" {
					if s, ok := m["id"].(string); ok {
						add(s)
					}
				}
			}
		}
	}
	for _, m := range ms {
		for _, t := range m.Same["wikipedia"] {
			add(t)
		}
	}
	return slices.Sorted(maps.Keys(seen)), nil
}

// title gives the form of a title in the dump. It reports false for a title that is not a page title.
func title(t string) (string, bool) {
	t = strings.TrimSpace(strings.ReplaceAll(t, "_", " "))
	if t == "" || crosswalk.Check("wikipedia", t) != nil {
		return "", false
	}
	r, n := utf8.DecodeRuneInString(t)
	return string(unicode.ToUpper(r)) + t[n:], true
}

// Fetch gets the pages that the tree at req.Root names from the multistream dump at req.URL and writes them as one XML export. It fails when the tree names no titles and when no title is in the dump.
func (a Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	if req.GetRange == nil {
		return errors.New("the request has no range getter")
	}
	index, err := Index(req.URL)
	if err != nil {
		return err
	}
	titles, err := Titles(req.Root)
	if err != nil {
		return err
	}
	if len(titles) == 0 {
		return errors.New("the tree names no Wikipedia titles")
	}
	f, err := os.CreateTemp(req.Cache, ".wikipedia-index-*")
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	body, err := req.Get(ctx, index)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, body)
	_ = body.Close()
	if err != nil {
		return err
	}
	f2 := &fetch{a: a, ctx: ctx, req: req, w: bufio.NewWriter(w), index: f, done: map[string]bool{}}
	first, streams, err := f2.scan(set(titles))
	if err != nil {
		return err
	}
	if len(streams) == 0 {
		return errors.New("no title that the tree names is in the dump")
	}
	if err := f2.header(first); err != nil {
		return err
	}
	targets, err := f2.pages(streams)
	if err != nil {
		return err
	}
	if len(targets) > 0 {
		_, more, err := f2.scan(targets)
		if err != nil {
			return err
		}
		if _, err := f2.pages(more); err != nil {
			return err
		}
	}
	if _, err := f2.w.WriteString("</mediawiki>\n"); err != nil {
		return err
	}
	return f2.w.Flush()
}

// fetch is the state of one Fetch.
type fetch struct {
	a     Adapter
	ctx   context.Context
	req   *ingest.Request
	w     *bufio.Writer
	index *os.File
	n     int             // the number of requests so far
	done  map[string]bool // the titles in the snapshot
}

// stream is the range of one bzip2 stream of the dump and the titles that Fetch wants from it. An end below 0 is the end of the dump.
type stream struct {
	start, end int64
	titles     map[string]bool
}

func set(ts []string) map[string]bool {
	m := map[string]bool{}
	for _, t := range ts {
		m[t] = true
	}
	return m
}

// scan reads the index, one offset:id:title line for each page, and returns the offset of the first stream and the streams of the titles in want, in the order of the dump.
func (f *fetch) scan(want map[string]bool) (int64, []*stream, error) {
	if _, err := f.index.Seek(0, io.SeekStart); err != nil {
		return 0, nil, err
	}
	sc := bufio.NewScanner(bzip2.NewReader(bufio.NewReader(f.index)))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var (
		out   []*stream
		cur   *stream
		first = int64(-1)
		last  = int64(-1)
	)
	for sc.Scan() {
		off, rest, ok := strings.Cut(sc.Text(), ":")
		_, t, ok2 := strings.Cut(rest, ":")
		o, err := strconv.ParseInt(off, 10, 64)
		if !ok || !ok2 || err != nil || o < last {
			return 0, nil, fmt.Errorf("the index has a bad line %q", sc.Text())
		}
		if first < 0 {
			first = o
		}
		if o != last {
			if cur != nil {
				cur.end = o - 1
				cur = nil
			}
			last = o
		}
		if want[t] && !f.done[t] {
			if cur == nil {
				cur = &stream{start: o, end: -1, titles: map[string]bool{}}
				out = append(out, cur)
			}
			cur.titles[t] = true
		}
	}
	if err := sc.Err(); err != nil {
		return 0, nil, fmt.Errorf("read the index: %w", err)
	}
	if first < 0 {
		return 0, nil, errors.New("the index is empty")
	}
	return first, out, nil
}

// get gets one range of the dump, after the pause, and returns the text of its bzip2 stream.
func (f *fetch) get(start, end int64) ([]byte, error) {
	if f.n > 0 && f.a.Pause > 0 {
		select {
		case <-f.ctx.Done():
			return nil, f.ctx.Err()
		case <-time.After(f.a.Pause):
		}
	}
	f.n++
	body, err := f.req.GetRange(f.ctx, f.req.URL, start, end)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(bzip2.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("the stream at %d: %w", start, err)
	}
	return data, nil
}

// header writes the start of the dump, up to the end of the site information, which is in the stream before the first stream of pages.
func (f *fetch) header(first int64) error {
	if first <= 0 {
		return errors.New("the dump has no header before its first page")
	}
	data, err := f.get(0, first-1)
	if err != nil {
		return err
	}
	i := bytes.Index(data, []byte("</siteinfo>"))
	if !bytes.Contains(data, []byte("<mediawiki")) || i < 0 {
		return errors.New("the first stream of the dump has no site information")
	}
	_, err = f.w.Write(append(data[:i+len("</siteinfo>")], '\n'))
	return err
}

var (
	pageTitle = regexp.MustCompile(`<title>([^<]*)</title>`)
	redirect  = regexp.MustCompile(`<redirect title="([^"]*)"`)
)

// pages gets the streams and writes their pages with the wanted titles. It returns the titles that the redirects among them redirect to, when Fetch has not written them.
func (f *fetch) pages(streams []*stream) (map[string]bool, error) {
	targets := map[string]bool{}
	for _, s := range streams {
		data, err := f.get(s.start, s.end)
		if err != nil {
			return nil, err
		}
		for rest := data; ; {
			i := bytes.Index(rest, []byte("<page>"))
			if i < 0 {
				break
			}
			j := bytes.Index(rest[i:], []byte("</page>"))
			if j < 0 {
				return nil, fmt.Errorf("the stream at %d has a page with no end", s.start)
			}
			page := rest[i : i+j+len("</page>")]
			rest = rest[i+j+len("</page>"):]
			m := pageTitle.FindSubmatch(page)
			if m == nil {
				return nil, fmt.Errorf("the stream at %d has a page with no title", s.start)
			}
			t := html.UnescapeString(string(m[1]))
			if !s.titles[t] || f.done[t] {
				continue
			}
			f.done[t] = true
			if _, err := fmt.Fprintf(f.w, "  %s\n", page); err != nil {
				return nil, err
			}
			if r := redirect.FindSubmatch(page); r != nil {
				if to, ok := title(html.UnescapeString(string(r[1]))); ok && !f.done[to] {
					targets[to] = true
				}
			}
		}
	}
	for t := range targets {
		if f.done[t] {
			delete(targets, t)
		}
	}
	return targets, nil
}

var (
	disambig = regexp.MustCompile(`(?i)\{\{\s*(?:disambiguation|disambig|dab|disamb|hndis|human name disambiguation|geodis|set index article|sia)\s*[|}]`)
	short    = regexp.MustCompile(`(?i)\{\{\s*short description\s*\|\s*([^|}]*?)\s*[|}]`)
	closing  = regexp.MustCompile(`(?im)^==\s*(?:see also|notes|references|footnotes|citations|sources|further reading|external links|bibliography)\s*==\s*$`)
)

// Read reads an XML export of Wikipedia and emits one record for each article, in the order of the export. Read fails when the input is not a MediaWiki export, on a title that does not pass crosswalk.Check, and on an export with no articles.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	var recs []ingest.Record
	redirects := map[string][]string{}
	_, err := mediawiki.Walk(r, func(_ *mediawiki.Site, p *mediawiki.Page) error {
		if p.NS != 0 {
			return nil
		}
		if err := crosswalk.Check("wikipedia", p.Title); err != nil {
			return err
		}
		if p.Redirect != "" {
			if to, ok := title(p.Redirect); ok {
				redirects[to] = append(redirects[to], p.Title)
			}
			return nil
		}
		if disambig.MatchString(p.Text) {
			return nil
		}
		recs = append(recs, record(p))
		return nil
	})
	if err != nil {
		return err
	}
	if len(recs) == 0 {
		return errors.New("the export has no articles")
	}
	for _, rec := range recs {
		names := append([]string{rec.ID}, slices.Sorted(slices.Values(redirects[rec.ID]))...)
		rec.Meta["wikipedia"] = strings.Join(names, "|")
		if err := emit(rec); err != nil {
			return err
		}
	}
	return nil
}

// record gives the record of an article.
func record(p *mediawiki.Page) ingest.Record {
	rev := strconv.FormatInt(p.Revision, 10)
	rec := ingest.Record{
		ID:      p.Title,
		Kind:    "article",
		Title:   p.Title,
		URL:     Page(p.Title),
		Locator: Home + "/w/index.php?title=" + url.QueryEscape(strings.ReplaceAll(p.Title, " ", "_")) + "&oldid=" + rev,
		Meta:    map[string]string{"revision": rev},
	}
	text := p.Text
	if loc := closing.FindStringIndex(text); loc != nil {
		text = text[:loc[0]]
	}
	rec.Text = mediawiki.Plain(text)
	if m := short.FindStringSubmatch(p.Text); m != nil && m[1] != "" && !strings.EqualFold(m[1], "none") {
		rec.Meta["description"] = m[1]
	}
	if cs := mediawiki.Categories(p.Text); len(cs) > 0 {
		rec.Meta["categories"] = strings.Join(cs, "|")
	}
	if date, _, _ := strings.Cut(p.Timestamp, "T"); date != "" {
		rec.Meta["date"] = date
	}
	if p.Contributor != "" {
		rec.Meta["contributor"] = p.Contributor
	}
	return rec
}

// Page returns the URL of the article with a title.
func Page(title string) string {
	return Home + "/wiki/" + url.PathEscape(strings.ReplaceAll(title, " ", "_"))
}
