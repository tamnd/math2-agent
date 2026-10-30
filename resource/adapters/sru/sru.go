// Package sru reads the operation searchRetrieve of an SRU server in pages for the adapters of the catalogues of resources. It gives the XML of each record, and the adapter decodes the XML in its own schema.
package sru

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource/adapters/oai"
)

// MaxPages is the most pages of one query. It stops a server that gives a next position with no end.
const MaxPages = 100000

// Record is one record of a page: the identifier that the server gives, when it gives one, and the XML of the record data.
type Record struct {
	ID   string
	Data []byte
}

// Query is one search: the base URL of the server, the parameters of the search (version, query, recordSchema and the others) and the records on each page.
type Query struct {
	Base   string
	Params url.Values
	Size   int
	Gap    time.Duration // the shortest time between two requests
}

type page struct {
	Total   int `xml:"numberOfRecords"`
	Next    int `xml:"nextRecordPosition"`
	Records []struct {
		ID   string `xml:"recordIdentifier"`
		Data struct {
			Inner []byte `xml:",innerxml"`
		} `xml:"recordData"`
	} `xml:"records>record"`
	Diagnostics []struct {
		URI     string `xml:"uri"`
		Details string `xml:"details"`
		Message string `xml:"message"`
	} `xml:"diagnostics>diagnostic"`
}

// Harvest asks the server for each page of the query and calls fn for each record. It starts at the position 1 and follows the next position of each page until the server gives none. It waits q.Gap before each request after the first. A diagnostic of the server is an error.
func Harvest(ctx context.Context, get ingest.Getter, q Query, fn func(Record) error) error {
	if q.Size < 1 {
		return errors.New("sru: the page size is not positive")
	}
	start := 1
	for n := 0; n < MaxPages; n++ {
		if n > 0 {
			if err := Sleep(ctx, q.Gap); err != nil {
				return err
			}
		}
		v := url.Values{}
		for k, vs := range q.Params {
			v[k] = vs
		}
		v.Set("operation", "searchRetrieve")
		v.Set("startRecord", strconv.Itoa(start))
		v.Set("maximumRecords", strconv.Itoa(q.Size))
		u := q.Base + "?" + v.Encode()
		body, err := get(ctx, u)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(body, 256<<20))
		_ = body.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", u, err)
		}
		var p page
		if err := oai.Unmarshal(data, &p); err != nil {
			return fmt.Errorf("sru: page %d of %s: %w", n+1, q.Base, err)
		}
		if len(p.Diagnostics) > 0 && len(p.Records) == 0 {
			d := p.Diagnostics[0]
			return fmt.Errorf("sru: %s: %s %s %s", q.Base, strings.TrimSpace(d.URI), strings.TrimSpace(d.Details), strings.TrimSpace(d.Message))
		}
		for _, r := range p.Records {
			if err := fn(Record{ID: strings.TrimSpace(r.ID), Data: r.Data.Inner}); err != nil {
				return err
			}
		}
		if p.Next <= start || len(p.Records) == 0 {
			return nil
		}
		start = p.Next
	}
	return fmt.Errorf("sru: %s: more than %d pages", q.Base, MaxPages)
}

// Sleep waits for d, or until the context ends.
func Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Split gives the base of a URL, without the query, and the value of its parameter query. An adapter uses it to take one search from the URL of a request in place of its own searches.
func Split(raw string) (base, query string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", err
	}
	query = u.Query().Get("query")
	u.RawQuery, u.Fragment = "", ""
	return u.String(), query, nil
}
