// Package wikidata is the ingest adapter of Wikidata (spec 09.2). It gives one record for each item of Wikidata that is a mathematical concept, with its QID, its English label and the ids that the item gives in other systems. The QID is the id that a node names in same_as, and the other ids are the crosswalk that Wikidata has now.
//
// The full dump of Wikidata is too large for this use. The adapter asks the SPARQL service of Wikidata instead, with one query for each property below. An item is in the snapshot when it has an id in one of these systems:
//
//	P4215   nLab              nlab
//	P2812   MathWorld         mathworld
//	P6781   ProofWiki         proofwiki
//	P7554   EoM               eom
//	P3285   MSC               msc
//	P829    OEIS              oeis
//	P14534  Mathlib           mathlib
//	P12987  LMFDB knowl       lmfdb_knowl
//	P12934  House of Graphs   house_of_graphs
//	P14106  Erdős problems    erdos
//
// The last column is the key of the id in the meta of the record. When an item has more than one id in a system, the meta has the ids in order, with "|" between them. The data of Wikidata is under CC0.
//
// The adapter does not ask for the items with a defining formula (P2534). There are more than 100,000 of them, most are single numbers such as large primes, and the query does not end in the 60 seconds that the service gives it.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/wikidata"
package wikidata

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/ingest"
)

// Endpoint is the SPARQL service of Wikidata.
const Endpoint = "https://query.wikidata.org/sparql"

// property is a property of Wikidata whose value is an id in another system.
type property struct {
	id  string // for example P4215
	key string // the key in the meta of a record
}

var properties = []property{
	{"P4215", "nlab"},
	{"P2812", "mathworld"},
	{"P6781", "proofwiki"},
	{"P7554", "eom"},
	{"P3285", "msc"},
	{"P829", "oeis"},
	{"P14534", "mathlib"},
	{"P12987", "lmfdb_knowl"},
	{"P12934", "house_of_graphs"},
	{"P14106", "erdos"},
}

// Adapter is the adapter of Wikidata.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of Wikidata.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "wikidata",
		Name:        "Wikidata",
		Home:        "https://www.wikidata.org",
		Licence:     "CC0-1.0",
		Class:       ingest.Open,
		Attribution: "Wikidata, the Wikimedia Foundation, https://www.wikidata.org, CC0 1.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "wikidata/1" }

// Dump returns the SPARQL service of Wikidata.
func (Adapter) Dump() string { return Endpoint }

// row is one answer of a query: an item, its English label and one value of the property.
type row struct {
	P     string `json:"p"`
	Item  string `json:"item"`
	Label string `json:"label,omitempty"`
	Value string `json:"value"`
}

// query returns the SPARQL query of the property p.
func query(p string) string {
	return `SELECT ?item ?label ?value WHERE { ?item wdt:` + p + ` ?value . OPTIONAL { ?item rdfs:label ?label FILTER(LANG(?label) = "en") } }`
}

// Fetch asks the SPARQL service at req.URL one query for each property of the crosswalk. The snapshot is one JSON line for each answer, compressed with gzip. The lines of each query are in order, thus two fetches of the same data give the same bytes.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	zw := gzip.NewWriter(w)
	bw := bufio.NewWriter(zw)
	enc := json.NewEncoder(bw)
	enc.SetEscapeHTML(false)
	for _, p := range properties {
		rows, err := ask(ctx, req.Get, req.URL, p.id)
		if err != nil {
			return fmt.Errorf("%s: %w", p.id, err)
		}
		for _, r := range rows {
			if err := enc.Encode(r); err != nil {
				return err
			}
		}
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	return zw.Close()
}

// ask runs the query of the property p and returns the rows in order.
func ask(ctx context.Context, get ingest.Getter, endpoint, p string) ([]row, error) {
	body, err := get(ctx, endpoint+"?format=json&query="+url.QueryEscape(query(p)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()
	var res struct {
		Results struct {
			Bindings []map[string]struct {
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if err := json.NewDecoder(body).Decode(&res); err != nil {
		return nil, fmt.Errorf("the answer of the SPARQL service: %w", err)
	}
	var rows []row
	for _, b := range res.Results.Bindings {
		item, ok := strings.CutPrefix(b["item"].Value, "http://www.wikidata.org/entity/")
		if !ok {
			continue // a lexeme or another entity that is not an item
		}
		rows = append(rows, row{P: p, Item: item, Label: b["label"].Value, Value: b["value"].Value})
	}
	slices.SortFunc(rows, cmpRow)
	return slices.Compact(rows), nil
}

// cmpRow orders rows by item, then by value.
func cmpRow(a, b row) int {
	if c := cmpQID(a.Item, b.Item); c != 0 {
		return c
	}
	if c := strings.Compare(a.Value, b.Value); c != 0 {
		return c
	}
	return strings.Compare(a.Label, b.Label)
}

// cmpQID orders QIDs by their numbers: Q9 comes before Q10.
func cmpQID(a, b string) int {
	if len(a) != len(b) {
		return len(a) - len(b)
	}
	return strings.Compare(a, b)
}

// item is the record of one item while Read collects its rows.
type item struct {
	label string
	ids   map[string][]string
}

// Read reads a snapshot and emits one record for each item, in the order of the QIDs. Read fails on a QID that does not pass crosswalk.Check and on a snapshot with no items.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	keys := map[string]string{}
	for _, p := range properties {
		keys[p.id] = p.key
	}
	items := map[string]*item{}
	dec := json.NewDecoder(r)
	for n := 1; ; n++ {
		var rw row
		err := dec.Decode(&rw)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("line %d: %w", n, err)
		}
		if err := crosswalk.Check("wikidata", rw.Item); err != nil {
			return fmt.Errorf("line %d: %w", n, err)
		}
		it := items[rw.Item]
		if it == nil {
			it = &item{ids: map[string][]string{}}
			items[rw.Item] = it
		}
		if it.label == "" {
			it.label = rw.Label
		}
		key, ok := keys[rw.P]
		if !ok {
			return fmt.Errorf("line %d: the property %s is not in the crosswalk", n, rw.P)
		}
		if rw.Value == "" {
			return fmt.Errorf("line %d: %s has no value of %s", n, rw.Item, rw.P)
		}
		if !slices.Contains(it.ids[key], rw.Value) {
			it.ids[key] = append(it.ids[key], rw.Value)
		}
	}
	if len(items) == 0 {
		return errors.New("the snapshot has no items")
	}
	qids := make([]string, 0, len(items))
	for q := range items {
		qids = append(qids, q)
	}
	slices.SortFunc(qids, cmpQID)
	for _, q := range qids {
		if err := emit(record(q, items[q])); err != nil {
			return err
		}
	}
	return nil
}

func record(qid string, it *item) ingest.Record {
	meta := map[string]string{}
	for k, vs := range it.ids {
		slices.Sort(vs)
		meta[k] = strings.Join(vs, "|")
	}
	return ingest.Record{
		ID:    qid,
		Kind:  "item",
		Title: it.label,
		URL:   "https://www.wikidata.org/wiki/" + qid,
		Meta:  meta,
	}
}
