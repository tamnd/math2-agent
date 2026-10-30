package marc

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// XMLRecord is one record in MARCXML or in MarcXchange. The names of its elements have no namespace, so the decoder takes the elements of both forms.
type XMLRecord struct {
	Leader  string       `xml:"leader"`
	Control []xmlControl `xml:"controlfield"`
	Data    []xmlData    `xml:"datafield"`
}

type xmlControl struct {
	Tag   string `xml:"tag,attr"`
	Value string `xml:",chardata"`
}

type xmlData struct {
	Tag       string   `xml:"tag,attr"`
	Ind1      string   `xml:"ind1,attr"`
	Ind2      string   `xml:"ind2,attr"`
	Subfields []xmlSub `xml:"subfield"`
}

type xmlSub struct {
	Code  string `xml:"code,attr"`
	Value string `xml:",chardata"`
}

// Record gives the record in the form of a record of ISO 2709.
func (x XMLRecord) Record() Record {
	rec := Record{Leader: x.Leader}
	for _, c := range x.Control {
		rec.Fields = append(rec.Fields, Field{Tag: c.Tag, Value: c.Value})
	}
	for _, d := range x.Data {
		f := Field{Tag: d.Tag, Ind: pad(d.Ind1) + pad(d.Ind2)}
		for _, s := range d.Subfields {
			f.Subfields = append(f.Subfields, [2]string{s.Code, strings.TrimSpace(s.Value)})
		}
		rec.Fields = append(rec.Fields, f)
	}
	return rec
}

// pad gives one indicator, or a space when the indicator is empty.
func pad(s string) string {
	if len(s) != 1 {
		return " "
	}
	return s
}

// Empty reports whether the record has no leader and no field.
func (x XMLRecord) Empty() bool { return x.Leader == "" && len(x.Control) == 0 && len(x.Data) == 0 }

// EachXML reads a file of records in MARCXML or in MarcXchange and calls fn for each element record. It decodes the XML in the non-strict mode.
func EachXML(r io.Reader, fn func(Record) error) error {
	dec := xml.NewDecoder(r)
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("marc: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "record" {
			continue
		}
		var x XMLRecord
		if err := dec.DecodeElement(&x, &se); err != nil {
			return fmt.Errorf("marc: %w", err)
		}
		if x.Empty() {
			continue
		}
		if err := fn(x.Record()); err != nil {
			return err
		}
	}
}
