// Package contests is the home of the adapters of the competition sources of RS7.2 (spec corpus resources RS7.2 and RS8.3, phase R5). Each adapter reads the public pages of one competition and gives a source record of kind competition, a source record of kind problem-set for each year or issue, and the problems of each set by number, with a link each.
//
// Most sites state no licence for their problem statements, and a statement is a short text with an author. Thus each adapter keeps facts only: the years, the host, the numbers of the problems and their links. It never keeps the text of a problem (RS14.2). Each year record has a part_of edge to the competition. The target is the resource of the competition in the register records/resources/competitions.md when the register has one, else the Wikidata id of the competition, which m2 res cluster resolves. Thus competitions.Gaps finds the missing years.
//
// The adapters read pages, so they wait Gap between two requests (RS8.2). The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/contests"
package contests

import (
	"bufio"
	"cmp"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/competitions"
)

// Gap is the time between two requests to a page channel (RS8.2).
const Gap = 5 * time.Second

// Competition is the first line of a snapshot: the competition, with the ids that the register gave at the time of the fetch.
type Competition struct {
	Name     string `json:"name"`
	Site     string `json:"site"`
	Wikidata string `json:"wikidata,omitempty"` // the QID of the competition in the register
	Resource string `json:"resource,omitempty"` // the resource of the competition in the register
}

// Problem is one problem of a set. The snapshot holds no statement.
type Problem struct {
	Key    string   `json:"key"`    // the key of the node, as 3 or B5390
	Number string   `json:"number"` // the number as the source prints it, as B. 5390
	Links  []string `json:"links,omitempty"`
}

// Set is one year or one issue of a competition.
type Set struct {
	ID        string    `json:"id"` // the native id, as 2024 or 2024-05
	Title     string    `json:"title"`
	Date      string    `json:"date,omitempty"`
	Host      string    `json:"host,omitempty"` // the place of the competition in that year, as the source gives it
	Links     []string  `json:"links,omitempty"`
	Shortlist bool      `json:"shortlist,omitempty"` // a shortlist is not a year of the competition, so it has no part_of edge
	Problems  []Problem `json:"problems,omitempty"`
}

// Line is one line of a snapshot: the competition or one set.
type Line struct {
	Competition *Competition `json:"competition,omitempty"`
	Set         *Set         `json:"set,omitempty"`
}

// base holds what the adapters share. Each adapter adds its own Fetch.
type base struct {
	id, name, home, dump string
	competition          string   // the name of the competition
	names                []string // other names of the competition in the register
	cadence              string
	terms                ingest.Terms
	gap                  time.Duration
}

// Source returns the catalogue entry of the source.
func (b base) Source() ingest.Source {
	t := b.terms
	return ingest.Source{ID: b.id, Name: b.name, Kind: "exercises", Home: b.home, Licence: "LicenseRef-facts", Class: ingest.RecordOnly, Channel: "page", Terms: &t, Cadence: b.cadence, Attribution: b.name + ", " + b.home}
}

// Version returns the version of the adapter.
func (b base) Version() string { return b.id + "/1" }

// Dump returns the first page that Fetch reads.
func (b base) Dump() string { return b.dump }

// lookup gives the competition with the ids of its record in the register of the tree at root. A record matches when its name is one of the names of the competition, or else when its site has the host of the source. A withdrawn record does not match.
func (b base) lookup(root string) (Competition, error) {
	c := Competition{Name: b.competition, Site: b.home}
	if root == "" {
		return c, nil
	}
	cs, err := competitions.Read(root)
	if err != nil {
		return c, err
	}
	names := append([]string{b.competition}, b.names...)
	var byName, bySite *competitions.Competition
	for i := range cs {
		r := &cs[i]
		if r.Status == "withdrawn" {
			continue
		}
		for _, n := range names {
			if byName == nil && strings.EqualFold(strings.TrimSpace(r.Name), n) {
				byName = r
			}
		}
		if bySite == nil && r.Site != "" && host(r.Site) == host(b.home) {
			bySite = r
		}
	}
	if r := cmp.Or(byName, bySite); r != nil {
		c.Wikidata, c.Resource = r.Wikidata, r.Resource
	}
	return c, nil
}

// host gives the host of a URL with no www.
func host(u string) string {
	p, err := url.Parse(strings.TrimSpace(u))
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(p.Hostname()), "www.")
}

// writer writes the lines of a snapshot as JSON Lines, compressed with gzip.
type writer struct {
	zw  *gzip.Writer
	enc *json.Encoder
}

// start writes the competition line of the snapshot.
func (b base) start(req *ingest.Request, w io.Writer) (*writer, error) {
	c, err := b.lookup(req.Root)
	if err != nil {
		return nil, err
	}
	zw := gzip.NewWriter(w)
	enc := json.NewEncoder(zw)
	enc.SetEscapeHTML(false)
	out := &writer{zw: zw, enc: enc}
	return out, enc.Encode(Line{Competition: &c})
}

func (w *writer) set(s Set) error { return w.enc.Encode(Line{Set: &s}) }

func (w *writer) close() error { return w.zw.Close() }

// pager gets pages with the Getter of the request and waits gap before each request but the first.
type pager struct {
	get   ingest.Getter
	gap   time.Duration
	asked bool
}

// MaxPage is the largest page that an adapter reads.
const MaxPage = 8 << 20

// page gets the body of one page. A page that the server does not have gives nil and no error.
func (p *pager) page(ctx context.Context, u string) ([]byte, error) {
	if p.asked && p.gap > 0 {
		t := time.NewTimer(p.gap)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, ctx.Err()
		case <-t.C:
		}
	}
	p.asked = true
	body, err := p.get(ctx, u)
	var se *ingest.StatusError
	if errors.As(err, &se) && se.Code == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(io.LimitReader(body, MaxPage+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", u, err)
	}
	if len(data) > MaxPage {
		return nil, fmt.Errorf("%s: the page is larger than %d bytes", u, MaxPage)
	}
	return data, nil
}

var (
	tagRE   = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRE = regexp.MustCompile(`\s+`)
	hrefRE  = regexp.MustCompile(`(?i)href\s*=\s*"([^"]*)"`)
)

// text gives the text of a piece of HTML: no tags, no entities, one space between words. It drops the word joiner U+2060.
func text(s string) string {
	s = html.UnescapeString(tagRE.ReplaceAllString(s, " "))
	s = strings.ReplaceAll(s, "\u2060", "")
	return strings.TrimSpace(spaceRE.ReplaceAllString(s, " "))
}

// hrefs gives the links of a piece of HTML, resolved against the URL of the page.
func hrefs(page, s string) []string {
	var out []string
	for _, m := range hrefRE.FindAllStringSubmatch(s, -1) {
		if u := resolve(page, html.UnescapeString(m[1])); u != "" {
			out = append(out, u)
		}
	}
	return out
}

// resolve gives the absolute URL of the link ref on the page, or "".
func resolve(page, ref string) string {
	b, err := url.Parse(page)
	if err != nil {
		return ""
	}
	r, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return ""
	}
	return b.ResolveReference(r).String()
}

// numbered gives the problems 1 to n with the links.
func numbered(n int, links ...string) []Problem {
	out := make([]Problem, 0, n)
	for i := 1; i <= n; i++ {
		k := fmt.Sprint(i)
		out = append(out, Problem{Key: k, Number: k, Links: links})
	}
	return out
}

// each reads the snapshot and calls the function of each line. The competition comes first.
func each(r io.Reader, comp func(Competition) error, set func(Set) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 64<<20)
	seen := false
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var l Line
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		switch {
		case l.Competition != nil:
			if seen {
				return fmt.Errorf("line %d: a second competition", line)
			}
			seen = true
			if err := comp(*l.Competition); err != nil {
				return err
			}
		case l.Set != nil:
			if !seen {
				return fmt.Errorf("line %d: a set before the competition", line)
			}
			if l.Set.ID == "" || l.Set.ID == NativeID {
				return fmt.Errorf("line %d: the set has the id %q", line, l.Set.ID)
			}
			if err := set(*l.Set); err != nil {
				return err
			}
		default:
			return fmt.Errorf("line %d: the line has no competition and no set", line)
		}
	}
	return sc.Err()
}

// NativeID is the native id of the record of the competition.
const NativeID = "competition"

// Read reads the snapshot and emits one ingest record for the competition, one for each set and one for each problem. The record of a problem has no text.
func (b base) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(c Competition) error {
		return emit(ingest.Record{ID: NativeID, Kind: "competition", Title: c.Name, URL: c.Site})
	}, func(s Set) error {
		rec := ingest.Record{ID: s.ID, Kind: "problem-set", Title: s.Title, URL: first(s.Links)}
		if s.Host != "" {
			rec.Meta = map[string]string{"host": s.Host}
		}
		if err := emit(rec); err != nil {
			return err
		}
		for _, p := range s.Problems {
			if err := emit(ingest.Record{ID: s.ID + "#" + p.Key, Kind: "problem", Title: s.Title + ", problem " + p.Number, URL: first(p.Links), Locator: s.ID}); err != nil {
				return err
			}
		}
		return nil
	})
}

func first(vs []string) string {
	if len(vs) == 0 {
		return ""
	}
	return vs[0]
}

// Records reads the snapshot and emits the source record of the competition and of each set (RS13.1). The record of the competition has the id of the source, because the source is the work of the competition (RS7.6). A set that is a year of the competition has a part_of edge to the resource of the competition, or to its Wikidata id, or else to the id of the source.
func (b base) Records(r io.Reader, emit func(cluster.Record) error) error {
	to := "source:" + b.id
	return each(r, func(c Competition) error {
		rec := cluster.Record{NativeID: NativeID, Level: "work", Kind: "competition", Title: c.Name, Links: []string{c.Site}, IDs: resource.IDs{"source": {b.id}}}
		if c.Wikidata != "" {
			rec.IDs["wikidata"] = resource.Values{c.Wikidata}
		}
		switch {
		case c.Resource != "":
			to = c.Resource
		case c.Wikidata != "":
			to = "wikidata:" + c.Wikidata
		}
		return emit(rec)
	}, func(s Set) error {
		rec := cluster.Record{NativeID: s.ID, Level: "work", Kind: "problem-set", Title: s.Title, Date: s.Date, Links: s.Links}
		if to != "" && !s.Shortlist {
			rec.Relations = []resource.Relation{{Kind: "part_of", To: to, Source: b.id}}
		}
		return emit(rec)
	})
}

// SetStructures reads the snapshot and gives the problems of each set by native id, as nodes of kind problem (RS2.9). A set with no problem has no entry.
func (b base) SetStructures(r io.Reader) (map[string][]resource.Node, error) {
	out := map[string][]resource.Node{}
	err := each(r, func(Competition) error { return nil }, func(s Set) error {
		for _, p := range s.Problems {
			out[s.ID] = append(out[s.ID], resource.Node{Key: p.Key, Kind: "problem", Number: p.Number, Links: p.Links})
		}
		return nil
	})
	return out, err
}

func init() {
	ingest.Register(IMO)
	ingest.Register(Putnam)
	ingest.Register(KoMaL)
	ingest.Register(Kvant)
	ingest.Register(BMO)
}
