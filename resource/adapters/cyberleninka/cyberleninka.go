// Package cyberleninka is the adapter of CyberLeninka, the Russian library of open science (spec corpus resources RS6.1 and RS8.3, phase R3). It reads the articles of mathematics of CyberLeninka, and it gives an ingest record and a source record for each article.
//
// Fetch harvests the OAI-PMH endpoint of CyberLeninka in the format oai_dc, and keeps the records with the subject mathematics or математика, or with a subject that names a branch of mathematics. It writes one JSON object for each article and each line, compressed with gzip. The licence of the metadata of CyberLeninka is not read (RS8.3), so the adapter keeps the class record only. The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/cyberleninka"
package cyberleninka

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/adapters/oai"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Endpoint is the OAI-PMH endpoint of CyberLeninka.
const Endpoint = "https://cyberleninka.ru/oai"

// Prefix is the metadata format that Fetch asks for.
const Prefix = "oai_dc"

// Adapter is the adapter of CyberLeninka.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of CyberLeninka.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "cyberleninka",
		Name:        "CyberLeninka, the articles of mathematics",
		Home:        "https://cyberleninka.ru",
		Licence:     "LicenseRef-unknown",
		Class:       ingest.RecordOnly,
		Attribution: "CyberLeninka, https://cyberleninka.ru",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "cyberleninka/1" }

// Dump returns the OAI-PMH endpoint.
func (Adapter) Dump() string { return Endpoint }

// Article is one article, as one line of the snapshot.
type Article struct {
	ID       string   `json:"id"` // the slug of the article, as in https://cyberleninka.ru/article/n/<slug>
	Title    string   `json:"title"`
	TitleEn  string   `json:"title_en,omitempty"`
	Language string   `json:"language,omitempty"`
	Date     string   `json:"date,omitempty"`
	ISSN     []string `json:"issn,omitempty"`
	DOI      []string `json:"doi,omitempty"`
	Authors  []string `json:"authors,omitempty"`
}

var (
	slug     = regexp.MustCompile(`cyberleninka\.ru/article/n/([^/?#\s]+)`)
	doi      = regexp.MustCompile(`(?i)(?:^|doi\.org/|doi:\s*)(10\.[0-9]{4,9}/\S+)`)
	issnText = regexp.MustCompile(`\b([0-9]{4}-[0-9]{3}[0-9Xx])\b`)
	year     = regexp.MustCompile(`^[0-9]{4}`)
)

// Math reports whether a subject is mathematics, in Russian or in English, or names a branch of mathematics.
func Math(subjects []string) bool {
	for _, s := range subjects {
		v := strings.ToLower(s)
		if v == "математика" || strings.HasPrefix(v, "математика ") {
			return true
		}
	}
	return fetch.MathHeading(subjects)
}

// Fetch harvests the endpoint at req.URL and writes the articles of mathematics as JSON Lines, compressed with gzip.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	zw := gzip.NewWriter(w)
	enc := json.NewEncoder(zw)
	enc.SetEscapeHTML(false)
	err := oai.Harvest(ctx, req.Get, req.URL, Prefix, "", func(r oai.Record) error {
		if r.Deleted {
			return nil
		}
		var m oai.DC
		if err := oai.Unmarshal(r.Metadata, &m); err != nil {
			return fmt.Errorf("%s: %w", r.ID, err)
		}
		if !Math(oai.Texts(m.Subject)) {
			return nil
		}
		a := FromDC(r.ID, m)
		if a.ID == "" {
			return nil
		}
		return enc.Encode(a)
	})
	if err != nil {
		return err
	}
	return zw.Close()
}

// FromDC makes an article from the OAI identifier and the metadata of a record. The id is the slug of the URL of the article, or else the part of the OAI identifier after oai:host:.
func FromDC(oaiID string, m oai.DC) Article {
	var a Article
	add := func(list *[]string, v string) {
		if !slices.Contains(*list, v) {
			*list = append(*list, v)
		}
	}
	for _, v := range oai.Texts(m.Identifier) {
		if x := slug.FindStringSubmatch(v); x != nil && a.ID == "" {
			a.ID = x[1]
		}
		if x := doi.FindStringSubmatch(v); x != nil {
			add(&a.DOI, x[1])
		}
	}
	if a.ID == "" {
		a.ID = oaiID
		if _, after, ok := strings.Cut(strings.TrimPrefix(oaiID, "oai:"), ":"); ok {
			a.ID = after
		}
	}
	for _, v := range append(oai.Texts(m.Identifier), oai.Texts(m.Source)...) {
		for _, x := range issnText.FindAllStringSubmatch(v, -1) {
			add(&a.ISSN, strings.ToUpper(x[1]))
		}
	}
	for _, t := range m.Title {
		v := strings.Join(strings.Fields(t.Value), " ")
		switch {
		case v == "":
		case strings.HasPrefix(strings.ToLower(t.Lang), "en") && a.Title != "":
			a.TitleEn = v
		case a.Title == "":
			a.Title = v
		}
	}
	if l := oai.Texts(m.Language); len(l) > 0 {
		a.Language = l[0]
	}
	for _, v := range oai.Texts(m.Date) {
		if year.MatchString(v) {
			a.Date, _, _ = strings.Cut(v, "T")
			break
		}
	}
	a.Authors = oai.Texts(m.Creator)
	return a
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

func link(a Article) string { return "https://cyberleninka.ru/article/n/" + a.ID }

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

// SourceRecord makes the source record of one article.
func SourceRecord(a Article) cluster.Record {
	rec := cluster.Record{NativeID: a.ID, Level: "work", Kind: "article", Title: a.Title, TitleEn: a.TitleEn, Language: fetch.Lang(a.Language), Date: a.Date,
		IDs: resource.IDs{}, Links: []string{link(a)}}
	put := func(key string, vs ...string) {
		for _, v := range vs {
			if n, err := ids.Normalize(key, v); err == nil {
				rec.IDs[key] = append(rec.IDs[key], n)
			}
		}
	}
	put("cyberleninka", a.ID)
	put("doi", a.DOI...)
	for _, s := range a.ISSN {
		if n, err := ids.Normalize("issn", s); err == nil {
			rec.Venue = n
			break
		}
	}
	for _, name := range a.Authors {
		rec.Creators = append(rec.Creators, resource.Creator{Name: name, Role: "author"})
	}
	return rec
}
