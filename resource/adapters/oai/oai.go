// Package oai harvests a repository through OAI-PMH 2.0 for the adapters of the catalogue of resources. It follows the resumption tokens, and it decodes the XML in the non-strict mode, because some repositories send a bad entity, as "&G" with no semicolon.
package oai

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
)

// MaxPages is the most pages of one harvest. It stops a repository that gives a resumption token with no end.
const MaxPages = 100000

// Record is one record of a page: the identifier, the status of the header and the XML of the metadata.
type Record struct {
	ID       string
	Deleted  bool
	Sets     []string
	Metadata []byte
}

type page struct {
	Error struct {
		Code string `xml:"code,attr"`
		Text string `xml:",chardata"`
	} `xml:"error"`
	Records []struct {
		Header struct {
			ID     string   `xml:"identifier"`
			Status string   `xml:"status,attr"`
			Sets   []string `xml:"setSpec"`
		} `xml:"header"`
		Metadata struct {
			Inner []byte `xml:",innerxml"`
		} `xml:"metadata"`
	} `xml:"ListRecords>record"`
	Token string `xml:"ListRecords>resumptionToken"`
}

// Harvest asks base for ListRecords with the metadata prefix and the set, and calls fn for each record of each page, until the resumption token ends. An empty set means the whole repository. The error noRecordsMatch means no records, and is not an error.
func Harvest(ctx context.Context, get ingest.Getter, base, prefix, set string, fn func(Record) error) error {
	q := url.Values{"verb": {"ListRecords"}, "metadataPrefix": {prefix}}
	if set != "" {
		q.Set("set", set)
	}
	for n := 0; n < MaxPages; n++ {
		u := base + "?" + q.Encode()
		body, err := get(ctx, u)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", u, err)
		}
		var p page
		if err := Unmarshal(data, &p); err != nil {
			return fmt.Errorf("page %d of %s: %w", n+1, base, err)
		}
		switch p.Error.Code {
		case "":
		case "noRecordsMatch":
			return nil
		default:
			return fmt.Errorf("%s: %s: %s", base, p.Error.Code, strings.TrimSpace(p.Error.Text))
		}
		for _, r := range p.Records {
			rec := Record{ID: strings.TrimSpace(r.Header.ID), Deleted: r.Header.Status == "deleted", Sets: r.Header.Sets, Metadata: r.Metadata.Inner}
			if err := fn(rec); err != nil {
				return err
			}
		}
		token := strings.TrimSpace(p.Token)
		if token == "" {
			return nil
		}
		q = url.Values{"verb": {"ListRecords"}, "resumptionToken": {token}}
	}
	return fmt.Errorf("%s: more than %d pages", base, MaxPages)
}

// Unmarshal decodes XML in the non-strict mode. A bad entity stays as text, and the entities of HTML become their characters.
func Unmarshal(data []byte, v any) error {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	d.Entity = xml.HTMLEntity
	return d.Decode(v)
}

// Value is one element of the format oai_dc, with its language and its type.
type Value struct {
	Lang  string `xml:"lang,attr"`
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

// DC is the metadata of a record in the format oai_dc, which each OAI-PMH endpoint must give. It has no description, because the adapters keep the class record only and do not keep the abstracts.
type DC struct {
	Title      []Value `xml:"title"`
	Creator    []Value `xml:"creator"`
	Subject    []Value `xml:"subject"`
	Publisher  []Value `xml:"publisher"`
	Date       []Value `xml:"date"`
	Type       []Value `xml:"type"`
	Identifier []Value `xml:"identifier"`
	Source     []Value `xml:"source"`
	Language   []Value `xml:"language"`
	Relation   []Value `xml:"relation"`
}

// Texts gives the text of each value that is not empty, with the spaces folded.
func Texts(vs []Value) []string {
	var out []string
	for _, v := range vs {
		if s := strings.Join(strings.Fields(v.Value), " "); s != "" {
			out = append(out, s)
		}
	}
	return out
}
