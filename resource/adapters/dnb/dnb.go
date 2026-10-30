// Package dnb is the adapter of the catalogue of the Deutsche Nationalbibliothek (spec corpus resources RS4.1 and RS8.3, phase R3). It reads the books of mathematics of the catalogue, and it gives an ingest record and a source record for each book.
//
// Fetch reads the SRU server of the DNB in MARC21-xml, in pages of 100 records. It makes one search for the subject group 510 of the DNB and one search for the DDC numbers that start with 51. The SRU server needs no token. The adapter keeps the records with the subject group 510, or with a class that passes the test S3 of RS9.2, and it writes one JSON object for each book and each line, compressed with gzip. The bibliographic data of the DNB is under CC0 1.0 (RQ06). The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/dnb"
package dnb

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource/adapters/marc"
	"github.com/tamnd/math2-agent/resource/adapters/sru"
	"github.com/tamnd/math2-agent/resource/cluster"
)

// Base is the SRU server of the catalogue of the DNB.
const Base = "https://services.dnb.de/sru/dnb"

// PageSize is the number of records on one page.
const PageSize = 100

// Gap is the time between two requests. The DNB states no rate, so the adapter asks for one page each second.
const Gap = time.Second

// Queries are the searches of Fetch: the subject group 510 (dnb.sgt) and the DDC numbers that start with 51 (dnb.ddc).
var Queries = []string{"dnb.sgt=510", "dnb.ddc=51*"}

// Adapter is the adapter of the catalogue of the DNB. Gap is the time between two requests.
type Adapter struct{ Gap time.Duration }

func init() { ingest.Register(Adapter{Gap: Gap}) }

// Source returns the catalogue entry of the DNB.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "dnb",
		Name:        "Deutsche Nationalbibliothek, the SRU server of the catalogue",
		Home:        "https://www.dnb.de",
		Licence:     "CC0-1.0",
		Class:       ingest.RecordOnly,
		Attribution: "Deutsche Nationalbibliothek, https://www.dnb.de",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "dnb/1" }

// Dump returns the SRU server.
func (Adapter) Dump() string { return Base }

// Math reports whether a book is a book of mathematics: it has the subject group 510 of the DNB, or a class that passes the test S3.
func Math(b marc.Book) bool {
	return slices.ContainsFunc(b.SDNB, func(s string) bool { return strings.HasPrefix(strings.TrimSpace(s), "510") }) || marc.Math(b)
}

// Fetch reads the SRU server at req.URL and writes the books of mathematics as JSON Lines, compressed with gzip. When req.URL has the parameter query, Fetch makes that one search in place of the searches of Queries.
func (a Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	base, query, err := sru.Split(req.URL)
	if err != nil {
		return err
	}
	queries := Queries
	if query != "" {
		queries = []string{query}
	}
	out := marc.NewLines(w)
	seen := map[string]bool{}
	for i, q := range queries {
		if i > 0 {
			if err := sru.Sleep(ctx, a.Gap); err != nil {
				return err
			}
		}
		params := url.Values{"version": {"1.1"}, "recordSchema": {"MARC21-xml"}, "query": {q}}
		err := sru.Harvest(ctx, req.Get, sru.Query{Base: base, Params: params, Size: PageSize, Gap: a.Gap}, func(r sru.Record) error {
			return marc.EachXML(bytes.NewReader(r.Data), func(rec marc.Record) error {
				b := marc.FromMARC21(rec)
				if b.ID == "" || seen[b.ID] || !Math(b) {
					return nil
				}
				seen[b.ID] = true
				return out.Add(b)
			})
		})
		if err != nil {
			return fmt.Errorf("dnb: %s: %w", q, err)
		}
	}
	return out.Close()
}

// Link gives the page of a book in the catalogue, from its id (IDN).
func Link(b marc.Book) string { return "https://d-nb.info/" + b.ID }

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
