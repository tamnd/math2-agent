// Package mathnetru is the adapter of Math-Net.Ru, the All-Russian Mathematical Portal (spec corpus resources RS6.1 and RS8.3, phase R3). It reads the articles of the journals of Math-Net.Ru, and it gives an ingest record and a source record for each article.
//
// Math-Net.Ru has no OAI-PMH endpoint and no dump. So Fetch reads the pages of the site: the list of the journals, the list of the issues of each journal and the page of each issue. It writes one JSON object for each article and each line, compressed with gzip. The robots.txt of the site asks for 2 seconds between two requests, and RS8.2 asks for 5 seconds between two requests to a page channel, so Fetch waits Gap between two requests. The records of Math-Net.Ru are under all rights reserved (RQ11). Thus the snapshot keeps only the ids, the links and the bibliographic facts: the title, the authors, the year, the volume, the issue and the first page. The adapter keeps the class record only. RQ11 asks for written permission before a harvest. The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/mathnetru"
package mathnetru

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Base is the home of Math-Net.Ru. Fetch reads the pages under it.
const Base = "https://www.mathnet.ru"

// DefaultGap is the shortest time between two requests when the field Gap is zero.
const DefaultGap = 5 * time.Second

// Adapter is the adapter of Math-Net.Ru.
type Adapter struct {
	Gap time.Duration // the shortest time between two requests. Zero means DefaultGap.
}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of Math-Net.Ru.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "mathnetru",
		Name:        "Math-Net.Ru, the All-Russian Mathematical Portal",
		Home:        Base,
		Licence:     "LicenseRef-all-rights-reserved",
		Class:       ingest.RecordOnly,
		Attribution: "Math-Net.Ru, Steklov Mathematical Institute of RAS, https://www.mathnet.ru",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "mathnetru/1" }

// Dump returns the home of the site.
func (Adapter) Dump() string { return Base }

// Article is one article, as one line of the snapshot.
type Article struct {
	ID      string   `json:"id"`      // the id of the article, as im8810
	Journal string   `json:"journal"` // the code of the journal, as im
	Title   string   `json:"title"`
	Year    string   `json:"year,omitempty"`
	Volume  string   `json:"volume,omitempty"`
	Issue   string   `json:"issue,omitempty"`
	Page    string   `json:"page,omitempty"`
	DOI     []string `json:"doi,omitempty"`
	Authors []string `json:"authors,omitempty"`
}

var (
	journalLink = regexp.MustCompile(`journal\.phtml\?jrnid=([a-z0-9]+)`)
	issueLink   = regexp.MustCompile(`archive\.phtml\?jrnid=([a-z0-9]+)&(?:amp;)?wshow=issue&[^\s'">]*`)
	paperLink   = regexp.MustCompile(`<a class=SLink href="/eng/([a-z]+[0-9]+)">`)
	doiLink     = regexp.MustCompile(`doi\.org/(10\.[0-9]{4,9}/[^\s'"<>]+)`)
	pages       = regexp.MustCompile(`Pages\s+([0-9]+)`)
	tag         = regexp.MustCompile(`<[^>]*>`)
)

// Fetch reads the pages of the site at req.URL and writes the articles as JSON Lines, compressed with gzip.
func (a Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	gap := a.Gap
	if gap <= 0 {
		gap = DefaultGap
	}
	var last time.Time
	get := func(path string) (string, error) {
		if d := time.Until(last.Add(gap)); !last.IsZero() && d > 0 {
			t := time.NewTimer(d)
			select {
			case <-ctx.Done():
				t.Stop()
				return "", ctx.Err()
			case <-t.C:
			}
		}
		last = time.Now()
		page, err := getPage(ctx, req.Get, req.URL+path)
		if err != nil {
			return "", fmt.Errorf("mathnetru: %w", err)
		}
		return page, nil
	}
	page, err := get("/ej.phtml?option_lang=eng")
	if err != nil {
		return err
	}
	var journals []string
	for _, x := range journalLink.FindAllStringSubmatch(page, -1) {
		if !slices.Contains(journals, x[1]) {
			journals = append(journals, x[1])
		}
	}
	zw := gzip.NewWriter(w)
	enc := json.NewEncoder(zw)
	enc.SetEscapeHTML(false)
	seen := map[string]bool{}
	for _, j := range journals {
		page, err := get("/php/archive.phtml?jrnid=" + j + "&wshow=contents&option_lang=eng")
		if err != nil {
			return err
		}
		var issues []string
		for _, x := range issueLink.FindAllStringSubmatch(page, -1) {
			if u := strings.ReplaceAll(x[0], "&amp;", "&"); x[1] == j && !slices.Contains(issues, u) {
				issues = append(issues, u)
			}
		}
		for _, is := range issues {
			page, err := get("/php/" + is)
			if err != nil {
				return err
			}
			for _, art := range Articles(page, j, is) {
				if seen[art.ID] {
					continue
				}
				seen[art.ID] = true
				if err := enc.Encode(art); err != nil {
					return err
				}
			}
		}
	}
	return zw.Close()
}

// getPage gets a page of the site, which is in the encoding Windows-1251, and gives it in UTF-8.
func getPage(ctx context.Context, get ingest.Getter, u string) (string, error) {
	body, err := get(ctx, u)
	if err != nil {
		return "", err
	}
	data, err := io.ReadAll(io.LimitReader(body, 16<<20))
	_ = body.Close()
	if err != nil {
		return "", fmt.Errorf("%s: %w", u, err)
	}
	text, err := charmap.Windows1251.NewDecoder().Bytes(data)
	if err != nil {
		return "", fmt.Errorf("%s: %w", u, err)
	}
	return string(text), nil
}

// text gives the text of a part of a page, with no tags and no entities.
func text(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tag.ReplaceAllString(s, ""))), " ")
}

// Articles reads the articles of the page of an issue of the journal. The link of the issue gives the year, the volume and the number of the issue. Each article is a link to /eng/<id> with the title, the authors after the link, and a block with the pages and the DOIs.
func Articles(page, journal, issue string) []Article {
	q := url.Values{}
	if _, raw, ok := strings.Cut(issue, "?"); ok {
		q, _ = url.ParseQuery(raw)
	}
	locs := paperLink.FindAllStringSubmatchIndex(page, -1)
	var out []Article
	for i, loc := range locs {
		end := len(page)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		part := page[loc[1]:end]
		title, rest, _ := strings.Cut(part, "</a>")
		a := Article{ID: page[loc[2]:loc[3]], Journal: journal, Title: text(title), Year: q.Get("year"), Volume: q.Get("volume"), Issue: q.Get("issue")}
		rest = strings.TrimPrefix(strings.TrimSpace(rest), "<br>")
		names, _, _ := strings.Cut(rest, "<div")
		for _, n := range strings.Split(text(names), ",") {
			if n = strings.TrimSpace(n); n != "" {
				a.Authors = append(a.Authors, n)
			}
		}
		if x := pages.FindStringSubmatch(part); x != nil {
			a.Page = x[1]
		}
		for _, x := range doiLink.FindAllStringSubmatch(part, -1) {
			if d := strings.ToLower(x[1]); !slices.Contains(a.DOI, d) {
				a.DOI = append(a.DOI, d)
			}
		}
		out = append(out, a)
	}
	return out
}

func each(r io.Reader, fn func(Article) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 16<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var a Article
		if err := json.Unmarshal(sc.Bytes(), &a); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if a.ID == "" {
			return fmt.Errorf("line %d: the article has no id", line)
		}
		if err := fn(a); err != nil {
			return err
		}
	}
	return sc.Err()
}

func link(a Article) string { return Base + "/eng/" + a.ID }

// Read reads the articles and emits one ingest record for each article.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(a Article) error {
		return emit(ingest.Record{ID: a.ID, Kind: "article", Title: a.Title, URL: link(a)})
	})
}

// Records reads the articles and emits one source record for each article (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(a Article) error { return emit(SourceRecord(a)) })
}

// SourceRecord makes the source record of one article. The DOIs are the DOI of the Russian article and the DOI of its translation, which are two expressions of one work.
func SourceRecord(a Article) cluster.Record {
	rec := cluster.Record{NativeID: a.ID, Level: "work", Kind: "article", Title: a.Title, Date: a.Year, Volume: a.Volume, Page: a.Page, IDs: resource.IDs{}, Links: []string{link(a)}}
	put := func(key string, vs ...string) {
		for _, v := range vs {
			if n, err := ids.Normalize(key, v); err == nil {
				rec.IDs[key] = append(rec.IDs[key], n)
			}
		}
	}
	put("mathnetru", a.ID)
	put("doi", a.DOI...)
	for _, name := range a.Authors {
		rec.Creators = append(rec.Creators, resource.Creator{Name: name, Role: "author"})
	}
	return rec
}
