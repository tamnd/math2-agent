// Package doaj is the adapter of the articles of DOAJ, the Directory of Open Access Journals (spec corpus resources RS4.1 and RS8.3, phase R2). It reads the articles of the LCC sets of mathematics, and it gives an ingest record and a source record for each article.
//
// The search API of DOAJ gives at most 1000 hits for a query, and the data dump needs an approved account. So Fetch harvests the OAI-PMH endpoint of the articles, in the format oai_doaj, for each set of Sets. It writes one JSON object for each article and each line, compressed with gzip. An article in two sets is written once. The metadata of DOAJ is under CC0. The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/doaj"
package doaj

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/adapters/oai"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Endpoint is the OAI-PMH endpoint of the articles of DOAJ.
const Endpoint = "https://doaj.org/oai.article"

// Sets are the LCC subjects of DOAJ that Fetch harvests. The spec of a set is the name in URL-safe base64.
var Sets = []string{"LCC:Mathematics", "LCC:Applied mathematics. Quantitative methods", "LCC:Probabilities. Mathematical statistics"}

// SetSpec gives the OAI-PMH spec of a set of DOAJ, as TENDOk1hdGhlbWF0aWNz for LCC:Mathematics. DOAJ writes the padding as ~.
func SetSpec(name string) string {
	return strings.ReplaceAll(base64.URLEncoding.EncodeToString([]byte(name)), "=", "~")
}

// Adapter is the adapter of the articles of DOAJ.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of DOAJ.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "doaj",
		Name:        "DOAJ, the Directory of Open Access Journals, the articles of mathematics",
		Home:        "https://doaj.org",
		Licence:     "CC0-1.0",
		Class:       ingest.Open,
		Attribution: "DOAJ, https://doaj.org, CC0 1.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "doaj/1" }

// Dump returns the OAI-PMH endpoint.
func (Adapter) Dump() string { return Endpoint }

// Article is one article, as one line of the snapshot.
type Article struct {
	ID        string   `json:"id" xml:"-"`
	Title     string   `json:"title,omitempty" xml:"title"`
	Language  string   `json:"language,omitempty" xml:"language"`
	Publisher string   `json:"publisher,omitempty" xml:"publisher"`
	Journal   string   `json:"journal,omitempty" xml:"journalTitle"`
	ISSN      string   `json:"issn,omitempty" xml:"issn"`
	EISSN     string   `json:"eissn,omitempty" xml:"eissn"`
	Date      string   `json:"date,omitempty" xml:"publicationDate"`
	Volume    string   `json:"volume,omitempty" xml:"volume"`
	Issue     string   `json:"issue,omitempty" xml:"issue"`
	Page      string   `json:"page,omitempty" xml:"startPage"`
	DOI       string   `json:"doi,omitempty" xml:"doi"`
	URL       string   `json:"url,omitempty" xml:"fullTextUrl"`
	Authors   []Author `json:"authors,omitempty" xml:"authors>author"`
	Sets      []string `json:"sets,omitempty" xml:"-"`
}

// Author is one author of an article.
type Author struct {
	Name  string `json:"name" xml:"name"`
	ORCID string `json:"orcid,omitempty" xml:"orcid_id"`
}

// Fetch harvests each set of Sets at req.URL and writes the articles as JSON Lines, compressed with gzip.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	zw := gzip.NewWriter(w)
	enc := json.NewEncoder(zw)
	enc.SetEscapeHTML(false)
	seen := map[string]bool{}
	for _, set := range Sets {
		err := oai.Harvest(ctx, req.Get, req.URL, "oai_doaj", SetSpec(set), func(r oai.Record) error {
			id := strings.TrimPrefix(r.ID, "oai:doaj.org/article:")
			if r.Deleted || seen[id] {
				return nil
			}
			seen[id] = true
			var a Article
			if err := oai.Unmarshal(r.Metadata, &a); err != nil {
				return fmt.Errorf("%s: %w", r.ID, err)
			}
			a.ID = id
			for _, s := range r.Sets {
				if b, err := base64.URLEncoding.DecodeString(strings.ReplaceAll(s, "~", "=")); err == nil {
					a.Sets = append(a.Sets, strings.TrimPrefix(string(b), "LCC:"))
				}
			}
			return enc.Encode(a)
		})
		if err != nil {
			return err
		}
	}
	return zw.Close()
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

// Read reads the articles and emits one ingest record for each article.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(a Article) error {
		return emit(ingest.Record{ID: a.ID, Kind: "article", Title: a.Title, URL: "https://doaj.org/article/" + a.ID})
	})
}

// Records reads the articles and emits one source record for each article (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(a Article) error { return emit(SourceRecord(a)) })
}

// Classes maps the names of the LCC sets of DOAJ to their LCC classes, for the test S3 of RS9.2.
var Classes = map[string]string{
	"Mathematics": "QA1", "Applied mathematics. Quantitative methods": "T57", "Probabilities. Mathematical statistics": "QA273",
	"Statistics": "HA1", "Electronic computers. Computer science": "QA75.5", "Instruments and machines": "QA71",
}

// SourceRecord makes the source record of one article. The venue is the ISSN of the journal, or the eISSN when the journal has no print ISSN.
func SourceRecord(a Article) cluster.Record {
	rec := cluster.Record{NativeID: a.ID, Level: "work", Kind: "article", Title: strings.Join(strings.Fields(a.Title), " "), Language: fetch.Lang(a.Language), Volume: a.Volume, Page: a.Page, Publisher: a.Publisher, IDs: resource.IDs{}}
	if len(a.Date) >= 4 {
		rec.Date = strings.TrimSuffix(strings.TrimSuffix(a.Date[:min(10, len(a.Date))], "-01"), "-01")
	}
	if d, err := ids.Normalize("doi", a.DOI); err == nil {
		rec.IDs["doi"] = resource.Values{d}
	}
	for _, v := range []string{a.ISSN, a.EISSN} {
		if n, err := ids.Normalize("issn", v); err == nil {
			rec.Venue = n
			break
		}
	}
	for _, au := range a.Authors {
		cr := resource.Creator{Name: strings.TrimSpace(au.Name), Role: "author"}
		if o, err := ids.Normalize("orcid", au.ORCID); err == nil {
			cr.IDs = resource.IDs{"orcid": {o}}
		}
		if cr.Name != "" {
			rec.Creators = append(rec.Creators, cr)
		}
	}
	for _, s := range a.Sets {
		if c := Classes[s]; c != "" {
			rec.Subjects = append(rec.Subjects, resource.Subject{System: "lcc", Code: c})
		}
	}
	if a.URL != "" {
		rec.Links = []string{a.URL}
	}
	if len(rec.IDs) == 0 {
		rec.IDs = nil
	}
	return rec
}
