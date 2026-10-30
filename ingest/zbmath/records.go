package zbmath

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// resourceKinds maps the document types of zbMATH Open to the kinds of RS2.3.
var resourceKinds = map[string]string{"j": "article", "b": "monograph", "a": "chapter"}

// languages maps the names of the languages of zbMATH Open to BCP 47 tags.
var languages = map[string]string{
	"english": "en", "german": "de", "french": "fr", "russian": "ru", "italian": "it", "spanish": "es", "chinese": "zh", "japanese": "ja",
	"portuguese": "pt", "polish": "pl", "romanian": "ro", "ukrainian": "uk", "czech": "cs", "hungarian": "hu", "bulgarian": "bg", "serbian": "sr",
	"croatian": "hr", "dutch": "nl", "swedish": "sv", "korean": "ko", "turkish": "tr", "greek": "el", "persian": "fa", "vietnamese": "vi",
}

// Records reads a snapshot of pages and emits one source record for each document that is not deleted (RS13.1). The document id goes into the ids as zbl_id, and the Zbl number as zbl. The field author has the names with a semicolon between them, and the author ids are in the same order. A document of an unknown type has the kind article.
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	d := xml.NewDecoder(r)
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		s, ok := tok.(xml.StartElement)
		if !ok || s.Name.Local != "record" {
			continue
		}
		var rec record
		if err := d.DecodeElement(&rec, &s); err != nil {
			return err
		}
		if rec.Header.Status == "deleted" {
			continue
		}
		if err := emit(sourceRecord(rec)); err != nil {
			return err
		}
	}
}

// sourceRecord makes the source record of one document.
func sourceRecord(rec record) cluster.Record {
	doc := rec.Doc
	id := strings.TrimSpace(doc.ID)
	if id == "" {
		id = strings.TrimPrefix(rec.Header.Identifier, "oai:zbmath.org:")
	}
	kind := resourceKinds[doc.Type]
	if kind == "" {
		kind = "article"
	}
	out := cluster.Record{
		NativeID: id,
		Level:    "work",
		Kind:     kind,
		Title:    clean(doc.Title),
		Date:     clean(doc.Year),
		Language: languages[strings.ToLower(clean(doc.Language))],
		IDs:      resource.IDs{},
		Links:    []string{"https://zbmath.org/?q=an:" + id},
	}
	var names []string
	for _, a := range doc.Authors {
		for n := range strings.SplitSeq(a, ";") {
			if n = clean(n); n != "" {
				names = append(names, n)
			}
		}
	}
	for i, n := range names {
		c := resource.Creator{Name: n, Role: "author"}
		if len(doc.AuthorIDs) == len(names) && clean(doc.AuthorIDs[i]) != "" {
			c.IDs = resource.IDs{"zbmath_author": resource.Values{clean(doc.AuthorIDs[i])}}
		}
		out.Creators = append(out.Creators, c)
	}
	for _, c := range doc.Classes {
		if c = clean(c); c != "" {
			out.MSC = append(out.MSC, c)
		}
	}
	for key, v := range map[string]string{"zbl_id": id, "zbl": clean(doc.Zbl), "doi": clean(doc.DOI)} {
		if n, err := ids.Normalize(key, v); err == nil {
			out.IDs[key] = resource.Values{n}
		}
	}
	return out
}
