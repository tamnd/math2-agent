// Package lmfdb is the ingest adapter of the LMFDB, the L-functions and modular forms database (spec 09.2). It gives one record for each object of a table of the LMFDB that a query of the API selects. The id of a record is the path of the page of the object, for example EllipticCurve/Q/11.a2, which is the id that a node names in same_as.
//
// The adapter knows these tables:
//
//	ec_curvedata   EllipticCurve/Q/   the elliptic curves over Q
//	nf_fields      NumberField/       the number fields
//
// The bulk channel is a URL of the API, with the table in the path and the filters in the query, for example
//
//	https://www.lmfdb.org/api/ec_curvedata/?conductor=py{"$lte":100}
//
// The adapter asks for the labels only, one page of the API after the other, with a pause between the pages. The LMFDB asks automated clients to use its bulk downloads for large slices, so give a filter that selects a small slice. When the LMFDB answers with a page that is not JSON, such as a captcha, the adapter stops. The data of the LMFDB is under CC BY-SA 4.0.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/lmfdb"
package lmfdb

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/ingest"
)

// Home is the home of the LMFDB. The page of an object is Home and the id of the object.
const Home = "https://www.lmfdb.org/"

// Default is the slice that m2 ingest lmfdb gets when no URL is given: the elliptic curves over Q of conductor at most 100.
const Default = Home + `api/ec_curvedata/?conductor=py{"$lte":100}`

// table is a table of the LMFDB that the adapter knows.
type table struct {
	field  string // the field of the label
	prefix string // the path of the pages of the objects
}

var tables = map[string]table{
	"ec_curvedata": {"lmfdb_label", "EllipticCurve/Q/"},
	"nf_fields":    {"label", "NumberField/"},
}

// pause is the time between two pages of the API.
var pause = 2 * time.Second

// Adapter is the adapter of the LMFDB.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of the LMFDB.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "lmfdb",
		Name:        "The L-functions and modular forms database",
		Home:        Home,
		Licence:     "CC-BY-SA-4.0",
		Class:       ingest.Open,
		Attribution: "The LMFDB Collaboration, The L-functions and modular forms database, https://www.lmfdb.org, CC BY-SA 4.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "lmfdb/1" }

// Dump returns the default slice.
func (Adapter) Dump() string { return Default }

// header is the first line of a snapshot.
type header struct {
	Table string `json:"table"`
	Query string `json:"query"` // the filters of the API URL
}

// Fetch gets the labels of the objects that the API URL req.URL selects and writes a snapshot, compressed with gzip: a header line with the table and the filters, and then one line for each label, in the order of the API.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	u, err := url.Parse(req.URL)
	if err != nil {
		return err
	}
	name, ok := strings.CutPrefix(strings.Trim(u.Path, "/"), "api/")
	t, known := tables[name]
	if !ok || !known {
		return fmt.Errorf("%s is not the API URL of a table that the adapter knows", req.URL)
	}
	q := u.Query()
	for k := range q {
		if strings.HasPrefix(k, "_") {
			q.Del(k)
		}
	}
	filters := q.Encode()
	q.Set("_format", "json")
	q.Set("_fields", t.field)

	zw := gzip.NewWriter(w)
	bw := bufio.NewWriter(zw)
	enc := json.NewEncoder(bw)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(header{Table: name, Query: filters}); err != nil {
		return err
	}
	for offset := 0; ; {
		q.Set("_offset", strconv.Itoa(offset))
		u.RawQuery = q.Encode()
		labels, next, err := page(ctx, req.Get, u.String(), t.field)
		if err != nil {
			return fmt.Errorf("offset %d: %w", offset, err)
		}
		for _, l := range labels {
			if err := enc.Encode(l); err != nil {
				return err
			}
		}
		if len(labels) == 0 || next <= offset {
			break
		}
		offset = next
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pause):
		}
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	return zw.Close()
}

// page gets one page of the API and returns its labels and the offset of the next page.
func page(ctx context.Context, get ingest.Getter, u, field string) ([]string, int, error) {
	body, err := get(ctx, u)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = body.Close() }()
	var res struct {
		Data   []map[string]any `json:"data"`
		Offset int              `json:"offset"`
	}
	if err := json.NewDecoder(body).Decode(&res); err != nil {
		return nil, 0, fmt.Errorf("the answer is not JSON, and the LMFDB can ask for a captcha when it gets too many requests: %w", err)
	}
	labels := make([]string, 0, len(res.Data))
	for i, d := range res.Data {
		l, ok := d[field].(string)
		if !ok || l == "" {
			return nil, 0, fmt.Errorf("object %d has no %s", i, field)
		}
		labels = append(labels, l)
	}
	return labels, res.Offset, nil
}

// Read reads a snapshot and emits one record for each label. Read fails on a table that the adapter does not know, on an id that does not pass crosswalk.Check and on a snapshot with no labels.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	dec := json.NewDecoder(r)
	var h header
	if err := dec.Decode(&h); err != nil {
		return fmt.Errorf("the header: %w", err)
	}
	t, ok := tables[h.Table]
	if !ok {
		return fmt.Errorf("the adapter does not know the table %q", h.Table)
	}
	n := 0
	for {
		var label string
		err := dec.Decode(&label)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("label %d: %w", n+1, err)
		}
		n++
		id := t.prefix + label
		if err := crosswalk.Check("lmfdb", id); err != nil {
			return fmt.Errorf("label %d: %w", n, err)
		}
		if err := emit(ingest.Record{ID: id, Kind: h.Table, Title: label, URL: Home + id}); err != nil {
			return err
		}
	}
	if n == 0 {
		return errors.New("the snapshot has no labels")
	}
	return nil
}
