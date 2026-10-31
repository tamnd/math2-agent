// Package competitions builds the competition register of RS7.2, records/resources/competitions.md, and finds its gaps. The register starts from Wikidata (CC0): each item that is an instance of a mathematics competition (Q1572106) or of a mathematical olympiad (Q131830746), or of a subclass of one. A person may add a competition that Wikidata does not have. Each competition lists its years through the resources of kind problem-set that are part_of the resource of the competition. A year with no problem set is a gap.
package competitions

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource"
)

// File is the competition register.
const File = resource.Dir + "/competitions.md"

// Competition is one record of the register.
type Competition struct {
	Wikidata string `json:"wikidata,omitempty"` // the QID
	Name     string `json:"name"`
	Country  string `json:"country,omitempty"` // the ISO 3166-1 code. Empty means an international competition
	Site     string `json:"site,omitempty"`
	Since    int    `json:"since,omitempty"`    // the first year
	Until    int    `json:"until,omitempty"`    // the last year of a competition that ended
	Skipped  []int  `json:"skipped,omitempty"`  // the years in which the competition did not take place, as a person read them
	Resource string `json:"resource,omitempty"` // the id of the resource of kind competition
	Status   string `json:"status,omitempty"`   // withdrawn when Wikidata no longer has the item as a competition
}

// Key gives the key of a competition: the QID, or the name of a competition that a person added.
func (c Competition) Key() string { return cmp.Or(c.Wikidata, "name:"+c.Name) }

// Read reads the register of the tree at root. A tree with no register gives no record and no error.
func Read(root string) ([]Competition, error) {
	recs, err := mdrec.Read(filepath.Join(root, File))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out, err := mdrec.Unmarshal[Competition](recs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	return out, nil
}

// Write writes the register to the tree at root, in the order of the keys.
func Write(root string, cs []Competition) error {
	slices.SortFunc(cs, func(a, b Competition) int { return cmp.Compare(a.Key(), b.Key()) })
	recs, err := mdrec.Marshal(cs)
	if err != nil {
		return err
	}
	path := filepath.Join(root, File)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return mdrec.Write(path, recs)
}

// Endpoint is the SPARQL service of Wikidata.
const Endpoint = "https://query.wikidata.org/sparql"

// Classes are the classes of the competitions: a mathematics competition (Q1572106) and a mathematical olympiad (Q131830746).
var Classes = []string{"Q1572106", "Q131830746"}

// ClassQuery asks Wikidata for the classes and each of their subclasses. The path P31/P279* in one query is too slow for the service, which stops it with 504, thus FromWikidata asks for the subclasses first and then for the instances of each of them.
const ClassQuery = `SELECT ?sub WHERE {
  VALUES ?class { %s }
  ?sub wdt:P279* ?class .
}`

// Query asks Wikidata for the instances of the classes, with their country, site, first year and last year. The %s is the list of the classes.
const Query = `SELECT ?c ?cLabel ?iso ?site ?inception ?dissolved WHERE {
  VALUES ?class { %s }
  ?c wdt:P31 ?class .
  OPTIONAL { ?c wdt:P17 ?country . ?country wdt:P297 ?iso }
  OPTIONAL { ?c wdt:P856 ?site }
  OPTIONAL { ?c wdt:P571 ?inception }
  OPTIONAL { ?c wdt:P576 ?dissolved }
  SERVICE wikibase:label { bd:serviceParam wikibase:language "en,mul" }
}`

// entity is the prefix of the URI of an item of Wikidata.
const entity = "http://www.wikidata.org/entity/"

// Getter gets the body of a URL.
type Getter func(ctx context.Context, url string) ([]byte, error)

// binding is one row of an answer of the SPARQL service.
type binding map[string]struct {
	Value string `json:"value"`
}

// sparql asks the service at endpoint and gives the rows of the answer.
func sparql(ctx context.Context, get Getter, endpoint, query string) ([]binding, error) {
	q := url.Values{"query": {query}, "format": {"json"}}
	body, err := get(ctx, endpoint+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var res struct {
		Results struct {
			Bindings []binding `json:"bindings"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("competitions: the answer of Wikidata: %w", err)
	}
	return res.Results.Bindings, nil
}

// values gives the QIDs as the items of a VALUES clause, as wd:Q1 wd:Q2.
func values(qids []string) string {
	out := make([]string, len(qids))
	for i, q := range qids {
		out[i] = "wd:" + q
	}
	return strings.Join(out, " ")
}

// FromWikidata asks the service at endpoint for the subclasses of Classes, then for the instances of the classes and the subclasses. An item with more than one row keeps the first value of each field. It gives the competitions in the order of their QIDs.
func FromWikidata(ctx context.Context, get Getter, endpoint string) ([]Competition, error) {
	subs, err := sparql(ctx, get, endpoint, fmt.Sprintf(ClassQuery, values(Classes)))
	if err != nil {
		return nil, err
	}
	classes := slices.Clone(Classes)
	for _, b := range subs {
		if q := strings.TrimPrefix(b["sub"].Value, entity); q != "" && !slices.Contains(classes, q) {
			classes = append(classes, q)
		}
	}
	rows, err := sparql(ctx, get, endpoint, fmt.Sprintf(Query, values(classes)))
	if err != nil {
		return nil, err
	}
	by := map[string]*Competition{}
	for _, b := range rows {
		qid := strings.TrimPrefix(b["c"].Value, entity)
		c := by[qid]
		if c == nil {
			c = &Competition{Wikidata: qid, Name: cmp.Or(b["cLabel"].Value, qid)}
			by[qid] = c
		}
		c.Country = cmp.Or(c.Country, b["iso"].Value)
		c.Site = cmp.Or(c.Site, b["site"].Value)
		if c.Since == 0 {
			c.Since = year(b["inception"].Value)
		}
		if c.Until == 0 {
			c.Until = year(b["dissolved"].Value)
		}
	}
	out := make([]Competition, 0, len(by))
	for _, c := range by {
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b Competition) int { return cmp.Compare(a.Key(), b.Key()) })
	return out, nil
}

// year gives the year of a date of Wikidata, as 1959-01-01T00:00:00Z, or 0.
func year(s string) int {
	if len(s) < 4 {
		return 0
	}
	y, err := strconv.Atoi(s[:4])
	if err != nil {
		return 0
	}
	return y
}

// Counts are the outcomes of one build.
type Counts struct {
	Wikidata, Added, Withdrawn, Linked int
}

// Build merges the competitions of Wikidata into the register prev. The name, the country, the site and the years come from Wikidata at each build, unless the record has them and Wikidata does not. A record that a person added, with no QID, stays as it is. A record whose item is no longer in the answer stays with the status withdrawn. A record with no resource gets the live resource of kind competition with the same QID in its ids.
func Build(prev, wd []Competition, rs []resource.Resource) ([]Competition, Counts) {
	c := Counts{Wikidata: len(wd)}
	by := map[string]Competition{}
	for _, p := range prev {
		by[p.Key()] = p
	}
	seen := map[string]bool{}
	for _, w := range wd {
		seen[w.Key()] = true
		p, had := by[w.Key()]
		if !had {
			c.Added++
		}
		p.Wikidata, p.Status = w.Wikidata, ""
		p.Name = cmp.Or(w.Name, p.Name)
		p.Country, p.Site = cmp.Or(w.Country, p.Country), cmp.Or(w.Site, p.Site)
		p.Since, p.Until = cmp.Or(w.Since, p.Since), cmp.Or(w.Until, p.Until)
		by[w.Key()] = p
	}
	byQID := map[string]string{}
	for i := range rs {
		r := &rs[i]
		if r.Live() && r.Kind == "competition" {
			for _, q := range r.IDs["wikidata"] {
				byQID[q] = r.ID
			}
		}
	}
	out := make([]Competition, 0, len(by))
	for k, p := range by {
		if p.Wikidata != "" && !seen[k] && len(wd) > 0 {
			if p.Status != "withdrawn" {
				c.Withdrawn++
			}
			p.Status = "withdrawn"
		}
		if p.Resource == "" && p.Wikidata != "" {
			p.Resource = byQID[p.Wikidata]
		}
		if p.Resource != "" {
			c.Linked++
		}
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Competition) int { return cmp.Compare(a.Key(), b.Key()) })
	return out, c
}

// Gap is the missing years of one competition.
type Gap struct {
	Competition string `json:"competition"` // the key of the competition
	Name        string `json:"name"`
	Years       []int  `json:"years,omitempty"` // the years with no problem set
	Reason      string `json:"reason,omitempty"`
}

// Gaps gives the gaps of the register up to the year before now: each competition with no resource, with no first year, or with years that have no problem set. A year in Skipped is not a gap. A year counts when a live resource of kind problem-set is part_of the resource of the competition and has a date in that year.
func Gaps(cs []Competition, rs []resource.Resource, now int) []Gap {
	years := map[string]map[int]bool{}
	for i := range rs {
		r := &rs[i]
		if !r.Live() || r.Kind != "problem-set" {
			continue
		}
		y := year(r.Date)
		for _, rel := range r.Relations {
			if rel.Kind == "part_of" && rel.From == "" && y > 0 {
				to, _, _ := strings.Cut(rel.To, "/")
				if years[to] == nil {
					years[to] = map[int]bool{}
				}
				years[to][y] = true
			}
		}
	}
	var out []Gap
	for _, c := range cs {
		if c.Status == "withdrawn" {
			continue
		}
		g := Gap{Competition: c.Key(), Name: c.Name}
		switch {
		case c.Resource == "":
			g.Reason = "the competition has no resource"
		case c.Since == 0:
			g.Reason = "the competition has no first year"
		default:
			last := now - 1
			if c.Until > 0 {
				last = min(last, c.Until)
			}
			for y := c.Since; y <= last; y++ {
				if !years[c.Resource][y] && !slices.Contains(c.Skipped, y) {
					g.Years = append(g.Years, y)
				}
			}
		}
		if g.Reason != "" || len(g.Years) > 0 {
			out = append(out, g)
		}
	}
	return out
}
