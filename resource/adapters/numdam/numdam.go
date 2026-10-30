// Package numdam is the adapter of Numdam, the digital library of French mathematics of Mathdoc (spec corpus resources RS4.1 and RS8.3, phase R2). It reads each article of Numdam, and it gives an ingest record and a source record for each article.
//
// Fetch harvests the OAI-PMH endpoint of Numdam in the format eudml-article2, a form of JATS, and writes one JSON object for each article and each line, compressed with gzip. Most articles have the id of their review in zbMATH Open. The metadata of Numdam is open, and the adapter keeps the class record only. The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/numdam"
package numdam

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/adapters/oai"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Endpoint is the OAI-PMH endpoint of Numdam.
const Endpoint = "https://www.numdam.org/oai/"

// Prefix is the metadata format that Fetch asks for.
const Prefix = "eudml-article2"

// Adapter is the adapter of Numdam.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of Numdam.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "numdam",
		Name:        "Numdam, the digital library of French mathematics",
		Home:        "https://www.numdam.org",
		Licence:     "LicenseRef-facts",
		Class:       ingest.RecordOnly,
		Attribution: "Numdam, Mathdoc, https://www.numdam.org",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "numdam/1" }

// Dump returns the OAI-PMH endpoint.
func (Adapter) Dump() string { return Endpoint }

// Article is one article, as one line of the snapshot.
type Article struct {
	ID       string   `json:"id"`
	Title    string   `json:"title,omitempty"`
	Language string   `json:"language,omitempty"`
	Journal  string   `json:"journal,omitempty"`
	ISSN     []string `json:"issn,omitempty"`
	Year     string   `json:"year,omitempty"`
	Volume   string   `json:"volume,omitempty"`
	Page     string   `json:"page,omitempty"`
	DOI      string   `json:"doi,omitempty"`
	ZBL      []string `json:"zbl,omitempty"`
	MR       []string `json:"mr,omitempty"`
	EuDML    string   `json:"eudml,omitempty"`
	Authors  []string `json:"authors,omitempty"` // as Surname, Given
	MSC      []string `json:"msc,omitempty"`
}

type jats struct {
	Lang    string `xml:"lang,attr"`
	Journal struct {
		Title string   `xml:"journal-title-group>journal-title"`
		ISSN  []string `xml:"issn"`
	} `xml:"front>journal-meta"`
	Meta struct {
		Title   string `xml:"title-group>article-title"`
		Contrib []struct {
			Type    string `xml:"contrib-type,attr"`
			Surname string `xml:"name>surname"`
			Given   string `xml:"name>given-names"`
		} `xml:"contrib-group>contrib"`
		Links []struct {
			Type  string `xml:"ext-link-type,attr"`
			Value string `xml:",chardata"`
		} `xml:"ext-link"`
		IDs []struct {
			Type  string `xml:"pub-id-type,attr"`
			Value string `xml:",chardata"`
		} `xml:"article-id"`
		Year   string `xml:"pub-date>year"`
		Volume string `xml:"volume"`
		FPage  string `xml:"fpage"`
		Kwd    []struct {
			Type  string   `xml:"kwd-group-type,attr"`
			Words []string `xml:"kwd"`
		} `xml:"kwd-group"`
	} `xml:"front>article-meta"`
}

// year finds the year in an id of Mathdoc, as 1994 in MSH_1994__126__5_0.
var year = regexp.MustCompile(`_((?:1[5-9]|20)[0-9]{2})_`)

// Fetch harvests the endpoint at req.URL and writes the articles as JSON Lines, compressed with gzip.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	zw := gzip.NewWriter(w)
	enc := json.NewEncoder(zw)
	enc.SetEscapeHTML(false)
	err := oai.Harvest(ctx, req.Get, req.URL, Prefix, "", func(r oai.Record) error {
		if r.Deleted {
			return nil
		}
		var j jats
		if err := oai.Unmarshal(r.Metadata, &j); err != nil {
			return fmt.Errorf("%s: %w", r.ID, err)
		}
		return enc.Encode(article(strings.TrimPrefix(r.ID, "oai:numdam.org:"), j))
	})
	if err != nil {
		return err
	}
	return zw.Close()
}

func article(id string, j jats) Article {
	m := j.Meta
	a := Article{ID: id, Title: strings.Join(strings.Fields(m.Title), " "), Language: j.Lang, Journal: strings.TrimSpace(j.Journal.Title), Year: strings.TrimSpace(m.Year),
		Volume: strings.TrimSpace(m.Volume), Page: strings.TrimSpace(m.FPage)}
	for _, s := range j.Journal.ISSN {
		if s = strings.TrimSpace(s); s != "" {
			a.ISSN = append(a.ISSN, s)
		}
	}
	if a.Year == "" {
		if y := year.FindStringSubmatch(id); y != nil {
			a.Year = y[1]
		}
	}
	for _, c := range m.Contrib {
		if c.Type != "" && c.Type != "author" {
			continue
		}
		name := strings.TrimSpace(c.Surname)
		if g := strings.TrimSpace(c.Given); g != "" {
			name += ", " + g
		}
		if name != "" {
			a.Authors = append(a.Authors, name)
		}
	}
	for _, l := range m.Links {
		v := strings.TrimSpace(l.Value)
		switch l.Type {
		case "zbl-item-id":
			a.ZBL = append(a.ZBL, v)
		case "mr-item-id":
			a.MR = append(a.MR, v)
		case "eudml-item-id":
			a.EuDML = strings.TrimPrefix(v, "urn:eudml:doc:")
		}
	}
	for _, x := range m.IDs {
		if x.Type == "doi" {
			a.DOI = strings.TrimSpace(x.Value)
		}
	}
	for _, k := range m.Kwd {
		if strings.HasPrefix(strings.ToLower(k.Type), "msc") {
			for _, w := range k.Words {
				if w = strings.TrimSpace(w); w != "" {
					a.MSC = append(a.MSC, w)
				}
			}
		}
	}
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

// Read reads the articles and emits one ingest record for each article.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(a Article) error {
		return emit(ingest.Record{ID: a.ID, Kind: "article", Title: a.Title, URL: "https://www.numdam.org/item/" + a.ID})
	})
}

// Records reads the articles and emits one source record for each article (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(a Article) error { return emit(SourceRecord(a)) })
}

// SourceRecord makes the source record of one article.
func SourceRecord(a Article) cluster.Record {
	rec := cluster.Record{NativeID: a.ID, Level: "work", Kind: "article", Title: a.Title, Language: fetch.Lang(a.Language), Date: a.Year, Volume: a.Volume, Page: a.Page, MSC: a.MSC,
		IDs: resource.IDs{}, Links: []string{"https://www.numdam.org/item/" + a.ID}}
	put := func(key string, vs ...string) {
		for _, v := range vs {
			if n, err := ids.Normalize(key, v); err == nil {
				rec.IDs[key] = append(rec.IDs[key], n)
			}
		}
	}
	put("numdam", a.ID)
	put("doi", a.DOI)
	put("zbl", a.ZBL...)
	put("mr", a.MR...)
	put("eudml", a.EuDML)
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
