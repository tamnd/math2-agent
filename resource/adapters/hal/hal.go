// Package hal is the adapter of HAL, the open archive of France (spec corpus resources RS4.1 and RS8.3, phase R2). It reads the documents of the domain math of HAL, and it gives an ingest record and a source record for each document.
//
// Fetch reads the search API in pages of 1000 with a cursor, in the order of the document id, and writes one JSON object for each document and each line, compressed with gzip. The metadata of HAL is under the Etalab open licence 2.0. The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/hal"
package hal

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Base is the search API of HAL.
const Base = "https://api.archives-ouvertes.fr/search/"

// Filter selects the documents of the domain math, as a primary or a secondary domain.
const Filter = "level0_domain_s:math"

// Fields names the fields of a document that Fetch asks for.
const Fields = "docid,halId_s,version_i,title_s,en_title_s,authFullName_s,authORCIDIdExt_s,doiId_s,arxivId_s,isbn_s,journalIssn_s,journalEissn_s,volume_s,page_s,publisher_s,producedDate_s,language_s,docType_s,primaryDomain_s,domain_s,uri_s"

// PageSize is the number of documents in one page.
const PageSize = 1000

// Adapter is the adapter of HAL.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of HAL.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "hal",
		Name:        "HAL, the open archive",
		Home:        "https://hal.science",
		Licence:     "etalab-2.0",
		Class:       ingest.Open,
		Attribution: "HAL, CCSD, https://hal.science, Licence Ouverte 2.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "hal/1" }

// Dump returns the URL of the documents of the domain math in the API.
func (Adapter) Dump() string { return Base + "?q=*:*&fq=" + url.QueryEscape(Filter) }

// Fetch reads each page of the documents at req.URL with a cursor and writes the documents as JSON Lines, compressed with gzip. The URL keeps its query and filter, and Fetch sets the fields, the order, the page size and the cursor.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	u, err := url.Parse(req.URL)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("wt", "json")
	q.Set("fl", Fields)
	q.Set("sort", "docid asc")
	q.Set("rows", strconv.Itoa(PageSize))
	zw := gzip.NewWriter(w)
	for cursor := "*"; ; {
		q.Set("cursorMark", cursor)
		u.RawQuery = q.Encode()
		var page struct {
			Response struct {
				Docs []json.RawMessage `json:"docs"`
			} `json:"response"`
			Next string `json:"nextCursorMark"`
		}
		body, err := req.Get(ctx, u.String())
		if err != nil {
			return err
		}
		err = json.NewDecoder(body).Decode(&page)
		_ = body.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", u, err)
		}
		for _, d := range page.Response.Docs {
			var line bytes.Buffer
			if err := json.Compact(&line, d); err != nil {
				return err
			}
			line.WriteByte('\n')
			if _, err := zw.Write(line.Bytes()); err != nil {
				return err
			}
		}
		if page.Next == "" || page.Next == cursor || len(page.Response.Docs) == 0 {
			return zw.Close()
		}
		cursor = page.Next
	}
}

// Doc is one document of HAL, with the fields that the adapter reads.
type Doc struct {
	HalID         string   `json:"halId_s"`
	Version       int      `json:"version_i"`
	Title         []string `json:"title_s"`
	TitleEn       []string `json:"en_title_s"`
	Authors       []string `json:"authFullName_s"`
	ORCIDs        []string `json:"authORCIDIdExt_s"`
	DOI           string   `json:"doiId_s"`
	ArXiv         string   `json:"arxivId_s"`
	ISBN          any      `json:"isbn_s"`
	ISSN          string   `json:"journalIssn_s"`
	EISSN         string   `json:"journalEissn_s"`
	Volume        string   `json:"volume_s"`
	Page          string   `json:"page_s"`
	Publisher     []string `json:"publisher_s"`
	Date          string   `json:"producedDate_s"`
	Language      []string `json:"language_s"`
	DocType       string   `json:"docType_s"`
	PrimaryDomain string   `json:"primaryDomain_s"`
	Domains       []string `json:"domain_s"`
	URI           string   `json:"uri_s"`
}

func each(r io.Reader, fn func(Doc) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 16<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var d Doc
		if err := json.Unmarshal(sc.Bytes(), &d); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if d.HalID == "" {
			return fmt.Errorf("line %d: the document has no HAL id", line)
		}
		if err := fn(d); err != nil {
			return err
		}
	}
	return sc.Err()
}

// Read reads the documents and emits one ingest record for each document.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(d Doc) error {
		rec := SourceRecord(d)
		return emit(ingest.Record{ID: rec.NativeID, Kind: rec.Kind, Title: rec.Title, URL: "https://hal.science/" + rec.NativeID})
	})
}

// Records reads the documents and emits one source record for each document (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(d Doc) error { return emit(SourceRecord(d)) })
}

// Kinds maps the document types of HAL to the kinds of RS2.3. A type that is not in the map gives the kind preprint.
var Kinds = map[string]string{
	"ART": "article", "COMM": "conference-paper", "POSTER": "conference-paper", "THESE": "thesis", "HDR": "thesis", "MEM": "thesis",
	"OUV": "monograph", "COUV": "chapter", "DOUV": "edited-volume", "PROCEEDINGS": "proceedings", "ISSUE": "edited-volume",
	"PREPRINT": "preprint", "UNDEFINED": "preprint", "REPORT": "report", "LECTURE": "lecture-notes", "SOFTWARE": "software",
	"BLOG": "website", "NOTICE": "encyclopedia", "TRAD": "monograph",
}

// SourceRecord makes the source record of one document. The domains of HAL go into the subjects with the system arxiv, by the map of fetch.HALDomain, for the test S2 of RS9.2.
func SourceRecord(d Doc) cluster.Record {
	rec := cluster.Record{NativeID: d.HalID, Level: "work", Kind: Kinds[d.DocType], Volume: d.Volume, Page: firstPage(d.Page), IDs: resource.IDs{}}
	if rec.Kind == "" {
		rec.Kind = "preprint"
	}
	if hid, err := ids.Normalize("hal", d.HalID); err == nil {
		rec.IDs["hal"] = resource.Values{hid}
	}
	if len(d.Title) > 0 {
		rec.Title = strings.Join(strings.Fields(d.Title[0]), " ")
	}
	if len(d.TitleEn) > 0 {
		if en := strings.Join(strings.Fields(d.TitleEn[0]), " "); en != rec.Title {
			rec.TitleEn = en
		}
	}
	if len(d.Date) >= 4 {
		rec.Date = d.Date[:min(10, len(d.Date))]
	}
	if len(d.Language) > 0 {
		rec.Language = d.Language[0]
	}
	if len(d.Publisher) > 0 {
		rec.Publisher = d.Publisher[0]
	}
	orcid := map[string]string{}
	for _, o := range d.ORCIDs {
		// The field gives a name and an ORCID URL for each author with an ORCID, as "Ann Smith_FacetSep_https://orcid.org/0000-...". An older form gives the URL only.
		name, id, ok := strings.Cut(o, "_FacetSep_")
		if !ok {
			continue
		}
		if n, err := ids.Normalize("orcid", id); err == nil {
			orcid[name] = n
		}
	}
	for _, a := range d.Authors {
		cr := resource.Creator{Name: strings.TrimSpace(a), Role: "author"}
		if o := orcid[a]; o != "" {
			cr.IDs = resource.IDs{"orcid": {o}}
		}
		rec.Creators = append(rec.Creators, cr)
	}
	for key, v := range map[string]string{"doi": d.DOI, "arxiv": d.ArXiv} {
		if n, err := ids.Normalize(key, v); err == nil {
			rec.IDs[key] = resource.Values{n}
		}
	}
	for _, v := range strs(d.ISBN) {
		if n, err := ids.Normalize("isbn", v); err == nil && !slices.Contains(rec.IDs["isbn"], n) {
			rec.IDs["isbn"] = append(rec.IDs["isbn"], n)
		}
	}
	for _, v := range []string{d.ISSN, d.EISSN} {
		if n, err := ids.Normalize("issn", v); err == nil {
			rec.Venue = n
			break
		}
	}
	for _, dom := range append([]string{d.PrimaryDomain}, d.Domains...) {
		if c := fetch.HALDomain(dom); c != "" && !slices.ContainsFunc(rec.Subjects, func(s resource.Subject) bool { return s.Code == c }) {
			rec.Subjects = append(rec.Subjects, resource.Subject{System: "arxiv", Code: c})
		}
	}
	if d.URI != "" {
		rec.Links = []string{d.URI}
	}
	for k, v := range rec.IDs {
		if len(v) == 0 {
			delete(rec.IDs, k)
		}
	}
	return rec
}

// firstPage gives the first page of a range of pages, as 12 for 12-34.
func firstPage(p string) string {
	p, _, _ = strings.Cut(strings.TrimSpace(p), "-")
	return strings.TrimSpace(p)
}

// strs gives a string or a list of strings of the JSON of HAL as a list.
func strs(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		var out []string
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
