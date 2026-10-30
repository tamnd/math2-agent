// Package datacite is the adapter of DataCite (spec corpus resources RS4.1 and RS8.3, phase R2). It reads the DOIs of DataCite with a subject of mathematics, and it gives an ingest record and a source record for each DOI.
//
// Fetch reads the REST API in pages of 1000 with a cursor, and writes one JSON object for each DOI and each line, compressed with gzip. The object is the attributes of the DOI as the API gives them. The metadata of DataCite is under CC0. The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/datacite"
package datacite

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
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Base is the REST API of DataCite.
const Base = "https://api.datacite.org"

// Query selects the DOIs with the subject mathematics, as a field of science or a free subject.
const Query = "subjects.subject:Mathematics"

// PageSize is the number of DOIs in one page.
const PageSize = 1000

// Adapter is the adapter of DataCite.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of DataCite.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "datacite",
		Name:        "DataCite",
		Home:        "https://datacite.org",
		Licence:     "CC0-1.0",
		Class:       ingest.Open,
		Attribution: "DataCite metadata, https://datacite.org, CC0 1.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "datacite/1" }

// Dump returns the URL of the DOIs of mathematics in the API.
func (Adapter) Dump() string { return Base + "/dois?query=" + url.QueryEscape(Query) }

// Fetch reads each page of the DOIs at req.URL with a cursor and writes the attributes of each DOI as JSON Lines, compressed with gzip. The URL keeps its query, and Fetch sets the page size and the cursor.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	u, err := url.Parse(req.URL)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("page[size]", strconv.Itoa(PageSize))
	q.Set("page[cursor]", "1")
	u.RawQuery = q.Encode()
	zw := gzip.NewWriter(w)
	for next := u.String(); next != ""; {
		var page struct {
			Data []struct {
				Attributes json.RawMessage `json:"attributes"`
			} `json:"data"`
			Links struct {
				Next string `json:"next"`
			} `json:"links"`
		}
		body, err := req.Get(ctx, next)
		if err != nil {
			return err
		}
		err = json.NewDecoder(body).Decode(&page)
		_ = body.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", next, err)
		}
		for _, d := range page.Data {
			var line bytes.Buffer
			if err := json.Compact(&line, d.Attributes); err != nil {
				return err
			}
			line.WriteByte('\n')
			if _, err := zw.Write(line.Bytes()); err != nil {
				return err
			}
		}
		next = page.Links.Next
		if len(page.Data) == 0 {
			next = ""
		}
	}
	return zw.Close()
}

// DOI is the attributes of one DOI, with the fields that the adapter reads.
type DOI struct {
	DOI    string `json:"doi"`
	Titles []struct {
		Title     string `json:"title"`
		TitleType string `json:"titleType"`
		Lang      string `json:"lang"`
	} `json:"titles"`
	Creators []struct {
		Name        string `json:"name"`
		Identifiers []struct {
			ID     string `json:"nameIdentifier"`
			Scheme string `json:"nameIdentifierScheme"`
		} `json:"nameIdentifiers"`
	} `json:"creators"`
	Publisher       any    `json:"publisher"`
	PublicationYear any    `json:"publicationYear"`
	Language        string `json:"language"`
	Version         string `json:"version"`
	Subjects        []struct {
		Subject string `json:"subject"`
		Scheme  string `json:"subjectScheme"`
	} `json:"subjects"`
	Types struct {
		General string `json:"resourceTypeGeneral"`
	} `json:"types"`
	Related []struct {
		ID   string `json:"relatedIdentifier"`
		Type string `json:"relatedIdentifierType"`
		Rel  string `json:"relationType"`
	} `json:"relatedIdentifiers"`
	URL string `json:"url"`
}

func each(r io.Reader, fn func(DOI) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 16<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var d DOI
		if err := json.Unmarshal(sc.Bytes(), &d); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if d.DOI == "" {
			return fmt.Errorf("line %d: the record has no DOI", line)
		}
		if err := fn(d); err != nil {
			return err
		}
	}
	return sc.Err()
}

// Read reads the DOIs and emits one ingest record for each DOI.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(d DOI) error {
		rec := SourceRecord(d)
		return emit(ingest.Record{ID: rec.NativeID, Kind: rec.Kind, Title: rec.Title, URL: "https://doi.org/" + rec.NativeID})
	})
}

// Records reads the DOIs and emits one source record for each DOI (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(d DOI) error { return emit(SourceRecord(d)) })
}

// Kinds maps the general resource types of DataCite to the kinds of RS2.3. A type that is not in the map gives the kind dataset.
var Kinds = map[string]string{
	"JournalArticle": "article", "Preprint": "preprint", "Book": "monograph", "BookChapter": "chapter", "Dissertation": "thesis",
	"Report": "report", "ConferencePaper": "conference-paper", "ConferenceProceeding": "proceedings", "Software": "software",
	"Dataset": "dataset", "Text": "report", "Journal": "journal", "Audiovisual": "video-series", "ComputationalNotebook": "software",
	"OutputManagementPlan": "report", "PeerReview": "report", "Standard": "report",
}

// Relations maps the relation types of DataCite to the relations of RS2.7. A cites edge is not a relation, and RS-M3 reads it from the references.
var Relations = map[string]string{
	"IsVersionOf": "version_of", "IsNewVersionOf": "version_of", "IsPartOf": "part_of", "IsTranslationOf": "translation_of",
	"IsDerivedFrom": "based_on", "Reviews": "reviews",
}

// SourceRecord makes the source record of one DOI. The MSC codes go into the field msc, a DDC class goes into the subjects, and a related arXiv id goes into the ids.
func SourceRecord(d DOI) cluster.Record {
	doi, err := ids.Normalize("doi", d.DOI)
	if err != nil {
		doi = strings.ToLower(strings.TrimSpace(d.DOI))
	}
	rec := cluster.Record{NativeID: doi, Level: "work", Kind: Kinds[d.Types.General], Language: lang(d.Language), Edition: d.Version, IDs: resource.IDs{"doi": {doi}}, Publisher: text(d.Publisher)}
	if rec.Kind == "" {
		rec.Kind = "dataset"
	}
	for _, t := range d.Titles {
		switch {
		case t.TitleType == "" && rec.Title == "":
			rec.Title = strings.Join(strings.Fields(t.Title), " ")
		case t.TitleType == "TranslatedTitle" && strings.HasPrefix(t.Lang, "en") && rec.TitleEn == "":
			rec.TitleEn = strings.Join(strings.Fields(t.Title), " ")
		}
	}
	if y := text(d.PublicationYear); len(y) == 4 {
		rec.Date = y
	}
	for _, c := range d.Creators {
		cr := resource.Creator{Name: strings.TrimSpace(c.Name), Role: "author"}
		for _, x := range c.Identifiers {
			if strings.EqualFold(x.Scheme, "ORCID") {
				if o, err := ids.Normalize("orcid", x.ID); err == nil {
					cr.IDs = resource.IDs{"orcid": {o}}
				}
			}
		}
		if cr.Name != "" {
			rec.Creators = append(rec.Creators, cr)
		}
	}
	for _, s := range d.Subjects {
		v := strings.TrimSpace(s.Subject)
		switch strings.ToLower(s.Scheme) {
		case "msc", "msc2020", "msc2010", "msc2000":
			rec.MSC = add(rec.MSC, v)
		case "ddc", "dewey decimal classification":
			if code, _, _ := strings.Cut(v, " "); code != "" {
				rec.Subjects = append(rec.Subjects, resource.Subject{System: "ddc", Code: code})
			}
		}
	}
	for _, r := range d.Related {
		var key, v string
		switch strings.ToLower(r.Type) {
		case "arxiv":
			key, v = "arxiv", strings.TrimPrefix(strings.ToLower(r.ID), "arxiv:")
		case "doi":
			key, v = "doi", r.ID
		case "isbn":
			key, v = "isbn", r.ID
		default:
			continue
		}
		n, err := ids.Normalize(key, v)
		if err != nil {
			continue
		}
		switch {
		case key == "arxiv" && (r.Rel == "IsIdenticalTo" || r.Rel == "IsVersionOf" || r.Rel == "HasVersion"):
			rec.IDs["arxiv"] = resource.Values(add(rec.IDs["arxiv"], n))
		case key == "isbn" && r.Rel == "IsIdenticalTo":
			rec.IDs["isbn"] = resource.Values(add(rec.IDs["isbn"], n))
		case Relations[r.Rel] != "":
			rec.Relations = append(rec.Relations, resource.Relation{Kind: Relations[r.Rel], To: key + ":" + n, Source: "datacite"})
		}
	}
	if d.URL != "" {
		rec.Links = []string{d.URL}
	}
	return rec
}

// text gives a string or a number of the JSON of DataCite as a string. The publisher is a string in the API version 2.4 and an object with a name in later versions.
func text(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.Itoa(int(x))
	case map[string]any:
		return text(x["name"])
	}
	return ""
}

// lang gives the BCP 47 tag of a language of DataCite, or an empty string when it is not a tag.
func lang(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || len(s) > 8 || strings.ContainsAny(s, " ,;") {
		return ""
	}
	return s
}

func add(list []string, v string) []string {
	if v == "" || slices.Contains(list, v) {
		return list
	}
	return append(list, v)
}
