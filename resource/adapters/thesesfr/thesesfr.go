// Package thesesfr is the adapter of theses.fr, the catalogue of the doctoral theses of France of ABES (spec corpus resources RS6.6). It reads the defended theses of mathematics, and it gives an ingest record and a source record for each thesis.
//
// A thesis is of mathematics when theses.fr gives it the domain Mathématiques, or a discipline whose name holds "math". Fetch reads the search API of theses.fr. It first reads the facets to find the names of the disciplines of mathematics. Then, for the domain and for each group of disciplines, it reads the theses in pages. The search API gives at most 10000 hits for a query, and the order of a page is not stable, so Fetch splits a query into ranges of years of the defence until one page holds all the hits of a range. A thesis with no date of the defence is lost when a query is split. Fetch writes one JSON object for each thesis and each line, compressed with gzip. A thesis that two queries give is written once.
//
// The about page of theses.fr puts the data in the public domain under the Etalab open licence 2.0. ABES also puts a dump of the theses defended since 1985 on data.gouv.fr once a year. This adapter reads the API, which also holds the theses of the last year. It sends no email and no key, and it waits Gap between two requests.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/thesesfr"
package thesesfr

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Base is the API of the theses of theses.fr.
const Base = "https://theses.fr/api/v1/theses/"

// Domain is the domain of mathematics of theses.fr.
const Domain = "Mathématiques"

// PageSize is the number of theses in one page.
const PageSize = 1000

// MaxHits is the most hits that the search API gives for one query.
const MaxHits = 10000

// Group is the number of disciplines in one query.
const Group = 20

// FirstYear is the first year of the ranges of years.
const FirstYear = 1800

// Gap is the time between two requests (spec corpus resources RS8.2).
var Gap = time.Second

// Adapter is the adapter of theses.fr.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of theses.fr.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "thesesfr",
		Name:        "theses.fr, the doctoral theses of France",
		Home:        "https://theses.fr",
		Licence:     "etalab-2.0",
		Class:       ingest.Open,
		Attribution: "theses.fr, ABES, https://theses.fr, Licence Ouverte 2.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "thesesfr/1" }

// Dump returns the base of the API.
func (Adapter) Dump() string { return Base }

// Filter gives the parameter filtres of the API for one field and its values, with the defended theses only, and with the range of years when from is not 0. The API takes the values of one field as alternatives, and it separates two filters with "~.
func Filter(field string, values []string, from, to int) string {
	var parts []string
	for _, v := range values {
		parts = append(parts, field+`="`+v+`"`)
	}
	parts = append(parts, `Statut="soutenue"`)
	if from != 0 {
		parts = append(parts, `datedebut="`+strconv.Itoa(from)+`"`, `datefin="`+strconv.Itoa(to)+`"`)
	}
	return "[" + strings.Join(parts, "~") + "]"
}

type fetcher struct {
	base  string
	get   ingest.Getter
	enc   *gzip.Writer
	seen  map[string]bool
	calls int
}

// getJSON waits Gap after the first request, then gets one URL and decodes its JSON.
func (f *fetcher) getJSON(ctx context.Context, u string, v any) error {
	if f.calls > 0 && Gap > 0 {
		t := time.NewTimer(Gap)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
	f.calls++
	body, err := f.get(ctx, u)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	if err := json.NewDecoder(body).Decode(v); err != nil {
		return fmt.Errorf("%s: %w", u, err)
	}
	return nil
}

// disciplines reads the facets of the defended theses and gives the names of the disciplines that hold "math". A name with a quote is left out, because the API cannot read it in a filter.
func (f *fetcher) disciplines(ctx context.Context) ([]string, error) {
	q := url.Values{"q": {"*"}, "filtres": {`[Statut="soutenue"]`}}
	var facets []struct {
		Name       string `json:"name"`
		Checkboxes []struct {
			Name string `json:"name"`
		} `json:"checkboxes"`
	}
	if err := f.getJSON(ctx, f.base+"facets/?"+q.Encode(), &facets); err != nil {
		return nil, err
	}
	var out []string
	for _, fc := range facets {
		if fc.Name != "Disciplines" {
			continue
		}
		for _, c := range fc.Checkboxes {
			if strings.Contains(strings.ToLower(c.Name), "math") && !strings.Contains(c.Name, `"`) {
				out = append(out, c.Name)
			}
		}
	}
	return out, nil
}

// page reads one page of the theses of a filter and writes the theses that are new. It gives the number of hits of the filter.
func (f *fetcher) page(ctx context.Context, filter string, start int) (int, error) {
	q := url.Values{"q": {"*"}, "debut": {strconv.Itoa(start)}, "nombre": {strconv.Itoa(PageSize)}, "tri": {"dateAsc"}, "filtres": {filter}}
	var p struct {
		Total  int               `json:"totalHits"`
		Theses []json.RawMessage `json:"theses"`
	}
	if err := f.getJSON(ctx, f.base+"recherche/?"+q.Encode(), &p); err != nil {
		return 0, err
	}
	for _, t := range p.Theses {
		var head struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(t, &head); err != nil {
			return 0, err
		}
		if head.ID == "" || f.seen[head.ID] {
			continue
		}
		f.seen[head.ID] = true
		var line bytes.Buffer
		if err := json.Compact(&line, t); err != nil {
			return 0, err
		}
		line.WriteByte('\n')
		if _, err := f.enc.Write(line.Bytes()); err != nil {
			return 0, err
		}
	}
	return p.Total, nil
}

// walk reads the theses of the field and values with a defence in the years from to to. A range with more hits than one page is split in two. A range of one year with more hits than one page is read in pages.
func (f *fetcher) walk(ctx context.Context, field string, values []string, from, to int) error {
	total, err := f.page(ctx, Filter(field, values, from, to), 0)
	if err != nil || total <= PageSize {
		return err
	}
	if from == 0 {
		from, to = FirstYear, time.Now().Year()+1
	}
	if from < to {
		mid := from + (to-from)/2
		if err := f.walk(ctx, field, values, from, mid); err != nil {
			return err
		}
		return f.walk(ctx, field, values, mid+1, to)
	}
	if total > MaxHits {
		return fmt.Errorf("theses.fr: %d theses in %d, and the API gives at most %d", total, from, MaxHits)
	}
	for start := PageSize; start < total; start += PageSize {
		if _, err := f.page(ctx, Filter(field, values, from, to), start); err != nil {
			return err
		}
	}
	return nil
}

// Fetch reads the theses of mathematics from the API at req.URL and writes them as JSON Lines, compressed with gzip.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	f := &fetcher{base: req.URL, get: req.Get, enc: gzip.NewWriter(w), seen: map[string]bool{}}
	if !strings.HasSuffix(f.base, "/") {
		f.base += "/"
	}
	ds, err := f.disciplines(ctx)
	if err != nil {
		return err
	}
	if err := f.walk(ctx, "Domaines thématiques", []string{Domain}, 0, 0); err != nil {
		return err
	}
	for i := 0; i < len(ds); i += Group {
		if err := f.walk(ctx, "Disciplines", ds[i:min(i+Group, len(ds))], 0, 0); err != nil {
			return err
		}
	}
	return f.enc.Close()
}

// Person is one person of a thesis. The ppn is the id of the person in IdRef.
type Person struct {
	PPN       string `json:"ppn"`
	Name      string `json:"nom"`
	FirstName string `json:"prenom"`
}

// Thesis is one thesis of theses.fr, with the fields that the adapter reads.
type Thesis struct {
	ID          string   `json:"id"`
	Title       string   `json:"titrePrincipal"`
	TitleEn     string   `json:"titreEN"`
	Institution string   `json:"etabSoutenanceN"`
	Defence     string   `json:"dateSoutenance"` // DD/MM/YYYY
	Authors     []Person `json:"auteurs"`
	Advisors    []Person `json:"directeurs"`
	NNT         string   `json:"nnt"`
	DOI         string   `json:"doi"`
	Discipline  string   `json:"discipline"`
	Status      string   `json:"status"`
}

func each(r io.Reader, fn func(Thesis) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 16<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var t Thesis
		if err := json.Unmarshal(sc.Bytes(), &t); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if t.ID == "" {
			return fmt.Errorf("line %d: the thesis has no id", line)
		}
		if err := fn(t); err != nil {
			return err
		}
	}
	return sc.Err()
}

// Read reads the theses and emits one ingest record for each thesis.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(t Thesis) error {
		rec := SourceRecord(t)
		return emit(ingest.Record{ID: rec.NativeID, Kind: rec.Kind, Title: rec.Title, URL: "https://theses.fr/" + rec.NativeID})
	})
}

// Records reads the theses and emits one source record for each thesis (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(t Thesis) error { return emit(SourceRecord(t)) })
}

// Date gives the date of the defence in ISO 8601, as 2020-01-06 for 06/01/2020.
func Date(s string) string {
	p := strings.Split(strings.TrimSpace(s), "/")
	if len(p) != 3 || len(p[0]) != 2 || len(p[1]) != 2 || len(p[2]) != 4 {
		return ""
	}
	return p[2] + "-" + p[1] + "-" + p[0]
}

// creator makes a creator from a person, with the IdRef id when the ppn is valid. A person with no name, or the name that theses.fr gives to an unknown advisor, gives false.
func creator(p Person, role string) (resource.Creator, bool) {
	name := strings.Join(strings.Fields(p.FirstName+" "+p.Name), " ")
	if name == "" || strings.HasPrefix(name, "Directeur de thèse inconnu") {
		return resource.Creator{}, false
	}
	c := resource.Creator{Name: name, Role: role}
	if id, err := ids.Normalize("idref", p.PPN); err == nil {
		c.IDs = resource.IDs{"idref": {id}}
	}
	return c, true
}

// SourceRecord makes the source record of one thesis. Each thesis of theses.fr is a doctoral thesis. The institution of the defence goes into the publisher, and the date is the date of the defence.
func SourceRecord(t Thesis) cluster.Record {
	rec := cluster.Record{NativeID: t.ID, Level: "work", Kind: "thesis", Thesis: "doctoral", Title: strings.Join(strings.Fields(t.Title), " "), Date: Date(t.Defence), Publisher: strings.TrimSpace(t.Institution), IDs: resource.IDs{}}
	if en := strings.Join(strings.Fields(t.TitleEn), " "); en != "" && en != rec.Title {
		rec.TitleEn = en
	}
	nnt := t.NNT
	if nnt == "" {
		nnt = t.ID
	}
	if n, err := ids.Normalize("nnt", nnt); err == nil {
		rec.IDs["nnt"] = resource.Values{n}
		rec.Links = []string{"https://theses.fr/" + n}
	}
	if n, err := ids.Normalize("doi", t.DOI); err == nil {
		rec.IDs["doi"] = resource.Values{n}
	}
	for _, p := range t.Authors {
		if c, ok := creator(p, "author"); ok {
			rec.Creators = append(rec.Creators, c)
		}
	}
	for _, p := range t.Advisors {
		if c, ok := creator(p, "advisor"); ok {
			rec.Creators = append(rec.Creators, c)
		}
	}
	if len(rec.IDs) == 0 {
		rec.IDs = nil
	}
	return rec
}
