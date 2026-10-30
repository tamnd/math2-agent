// Package bnf is the adapter of the general catalogue of the Bibliothèque nationale de France (spec corpus resources RS4.1 and RS8.3, phase R3). It reads the books of mathematics of the catalogue, and it gives an ingest record and a source record for each book.
//
// Fetch reads the SRU server of the catalogue with one search for each Dewey class from 510 to 519, in pages of 100 records. The records are in UNIMARC, in the XML form MarcXchange. The adapter keeps the records that pass the test S3 of RS9.2, and it writes one JSON object for each book and each line, compressed with gzip. The records that the BnF made are under the Etalab open licence 2.0, which asks for the source and the date of retrieval (RQ06). The attribution names the source, and the catalogue keeps the date of each snapshot, which is the date of retrieval. The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/bnf"
package bnf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource/adapters/marc"
	"github.com/tamnd/math2-agent/resource/adapters/sru"
	"github.com/tamnd/math2-agent/resource/cluster"
)

// Base is the SRU server of the catalogue of the BnF.
const Base = "https://catalogue.bnf.fr/api/SRU"

// PageSize is the most records on one page of the SRU server.
const PageSize = 100

// Gap is the time between two requests. The BnF states no rate, so the adapter asks for one page each second.
const Gap = time.Second

// Classes are the Dewey classes of the searches.
var Classes = []string{"510", "511", "512", "513", "514", "515", "516", "517", "518", "519"}

// Adapter is the adapter of the catalogue of the BnF. Gap is the time between two requests.
type Adapter struct{ Gap time.Duration }

func init() { ingest.Register(Adapter{Gap: Gap}) }

// Source returns the catalogue entry of the BnF.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "bnf",
		Name:        "Bibliothèque nationale de France, the SRU server of the general catalogue",
		Home:        "https://catalogue.bnf.fr",
		Licence:     "etalab-2.0",
		Class:       ingest.RecordOnly,
		Attribution: "Bibliothèque nationale de France, catalogue général, https://catalogue.bnf.fr",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "bnf/1" }

// Dump returns the SRU server.
func (Adapter) Dump() string { return Base }

// Query gives the search of one Dewey class, as bib.dewey all "515".
func Query(class string) string { return `bib.dewey all "` + class + `"` }

// Fetch reads the SRU server at req.URL and writes the books of mathematics as JSON Lines, compressed with gzip. When req.URL has the parameter query, Fetch makes that one search in place of the searches of Classes.
func (a Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	base, query, err := sru.Split(req.URL)
	if err != nil {
		return err
	}
	queries := []string{query}
	if query == "" {
		queries = queries[:0]
		for _, c := range Classes {
			queries = append(queries, Query(c))
		}
	}
	out := marc.NewLines(w)
	seen := map[string]bool{}
	for i, q := range queries {
		if i > 0 {
			if err := sru.Sleep(ctx, a.Gap); err != nil {
				return err
			}
		}
		params := url.Values{"version": {"1.2"}, "recordSchema": {"unimarcxchange"}, "query": {q}}
		err := sru.Harvest(ctx, req.Get, sru.Query{Base: base, Params: params, Size: PageSize, Gap: a.Gap}, func(r sru.Record) error {
			return marc.EachXML(bytes.NewReader(r.Data), func(rec marc.Record) error {
				b := marc.FromUNIMARC(rec)
				if b.ID == "" || seen[b.ID] || !marc.Math(b) {
					return nil
				}
				seen[b.ID] = true
				return out.Add(b)
			})
		})
		if err != nil {
			return fmt.Errorf("bnf: %s: %w", q, err)
		}
	}
	return out.Close()
}

// Link gives the page of a book in the catalogue, from its ARK name, as cb38972790p.
func Link(b marc.Book) string {
	if strings.HasPrefix(b.ID, "cb") {
		return "https://catalogue.bnf.fr/ark:/12148/" + b.ID
	}
	return ""
}

// Read reads the books and emits one ingest record for each book.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return marc.EachBook(r, func(b marc.Book) error {
		return emit(ingest.Record{ID: b.ID, Kind: marc.Kind(b), Title: b.Title, URL: Link(b)})
	})
}

// Records reads the books and emits one source record for each book (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return marc.EachBook(r, func(b marc.Book) error { return emit(marc.SourceRecord(b, Link(b))) })
}
