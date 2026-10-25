// Package zbmath is the ingest adapter of zbMATH Open, the abstracting and reviewing service of the published mathematical literature (spec 09.2).
//
// The adapter harvests the OAI-PMH interface of zbMATH Open in the oai_zb_preview format. It gives one record for each document, with its MSC codes, its identifiers and the zbMATH ids of its references. The metadata of zbMATH Open is under CC BY-SA 4.0. The adapter does not keep the text of the reviews, because math2 takes the metadata of the reviews and not the reviews.
//
// The full harvest has more than four million documents. To harvest one MSC class, give m2 ingest a URL with a set, for example:
//
//	m2 ingest zbmath -url 'https://oai.zbmath.org/v1/?verb=ListRecords&metadataPrefix=oai_zb_preview&set=11'
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/zbmath"
package zbmath

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
)

// Endpoint is the OAI-PMH interface of zbMATH Open.
const Endpoint = "https://oai.zbmath.org/v1/"

// Adapter is the adapter of zbMATH Open.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of zbMATH Open.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "zbmath",
		Name:        "zbMATH Open",
		Home:        "https://zbmath.org",
		Licence:     "CC-BY-SA-4.0",
		Class:       ingest.Open,
		Attribution: "zbMATH Open, FIZ Karlsruhe, https://zbmath.org, CC BY-SA 4.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "zbmath/1" }

// Dump returns the URL of the full harvest.
func (Adapter) Dump() string { return Endpoint + "?verb=ListRecords&metadataPrefix=oai_zb_preview" }

// Fetch harvests the OAI-PMH list at u, one page at a time, and follows the resumption tokens to the last page. The snapshot is the pages one after the other, compressed with gzip.
func (Adapter) Fetch(ctx context.Context, u string, get ingest.Getter, w io.Writer) error {
	base, _, _ := strings.Cut(u, "?")
	zw := gzip.NewWriter(w)
	for next := u; next != ""; {
		page, err := getPage(ctx, get, next)
		if err != nil {
			return err
		}
		if _, err := zw.Write(page); err != nil {
			return err
		}
		token, err := resumption(page)
		if err != nil {
			return fmt.Errorf("%s: %w", next, err)
		}
		next = ""
		if token != "" {
			next = base + "?verb=ListRecords&resumptionToken=" + url.QueryEscape(token)
		}
	}
	return zw.Close()
}

// getPage gets one page and ends it with a newline.
func getPage(ctx context.Context, get ingest.Getter, u string) ([]byte, error) {
	body, err := get(ctx, u)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()
	page, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	if !bytes.HasSuffix(page, []byte("\n")) {
		page = append(page, '\n')
	}
	return page, nil
}

// resumption returns the resumption token of one page. It is empty on the last page.
func resumption(page []byte) (string, error) {
	d := xml.NewDecoder(bytes.NewReader(page))
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		if s, ok := tok.(xml.StartElement); ok && s.Name.Local == "resumptionToken" {
			var t string
			if err := d.DecodeElement(&t, &s); err != nil {
				return "", err
			}
			return strings.TrimSpace(t), nil
		}
	}
}

// record is one record of the oai_zb_preview format.
type record struct {
	Header struct {
		Identifier string `xml:"identifier"`
		Status     string `xml:"status,attr"`
	} `xml:"header"`
	Doc struct {
		ID         string   `xml:"document_id"`
		Zbl        string   `xml:"zbl_id"`
		Title      string   `xml:"document_title"`
		Type       string   `xml:"document_type"`
		Authors    []string `xml:"author"`
		Year       string   `xml:"publication_year"`
		Source     string   `xml:"source"`
		DOI        string   `xml:"doi"`
		Classes    []string `xml:"classifications>classification"`
		References []string `xml:"references>reference>ref_id"`
	} `xml:"metadata>zbmath"`
}

// kinds names the document types of zbMATH Open.
var kinds = map[string]string{"j": "article", "b": "book", "a": "chapter"}

// Read reads a snapshot of pages and emits one record for each document. When the pages give the size of the complete list, the number of documents must be that size.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	d := xml.NewDecoder(r)
	n, size := 0, -1
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		s, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch s.Name.Local {
		case "error":
			var msg struct {
				Code string `xml:"code,attr"`
				Text string `xml:",chardata"`
			}
			if err := d.DecodeElement(&msg, &s); err != nil {
				return err
			}
			if text := strings.TrimSpace(msg.Text); text != "noRecordsMatch" && msg.Code != "noRecordsMatch" {
				return fmt.Errorf("the OAI-PMH interface gives the error %s %s", msg.Code, text)
			}
		case "resumptionToken":
			for _, a := range s.Attr {
				if a.Name.Local == "completeListSize" && size < 0 {
					if size, err = strconv.Atoi(a.Value); err != nil {
						return fmt.Errorf("the completeListSize %q is not a number", a.Value)
					}
				}
			}
		case "record":
			var rec record
			if err := d.DecodeElement(&rec, &s); err != nil {
				return err
			}
			n++
			if rec.Header.Status == "deleted" {
				continue
			}
			if err := emit(item(rec)); err != nil {
				return err
			}
		}
	}
	if size >= 0 && n != size {
		return fmt.Errorf("the harvest has %d records, and zbMATH Open gives a list of %d", n, size)
	}
	return nil
}

// clean removes the space around a field. zbMATH Open gives the string None for a field that a document does not have, and clean makes it empty.
func clean(s string) string {
	if s = strings.TrimSpace(s); s == "None" {
		return ""
	}
	return s
}

// item makes the ingest record of one document.
func item(rec record) ingest.Record {
	doc := rec.Doc
	id := strings.TrimSpace(doc.ID)
	if id == "" {
		id = strings.TrimPrefix(rec.Header.Identifier, "oai:zbmath.org:")
	}
	kind := kinds[doc.Type]
	if kind == "" {
		kind = doc.Type
	}
	meta := map[string]string{}
	set := func(k string, v ...string) {
		if s := clean(strings.Join(v, " ")); s != "" {
			meta[k] = s
		}
	}
	set("zbl", doc.Zbl)
	set("msc", doc.Classes...)
	set("year", doc.Year)
	set("doi", doc.DOI)
	set("refs", doc.References...)
	if a := clean(strings.Join(doc.Authors, "; ")); a != "" {
		meta["authors"] = a
	}
	return ingest.Record{
		ID:      id,
		Kind:    kind,
		Title:   clean(doc.Title),
		URL:     "https://zbmath.org/?q=an:" + id,
		Locator: clean(doc.Source),
		Meta:    meta,
	}
}
