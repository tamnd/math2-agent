package marc

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func book() Record {
	return Record{Fields: []Field{
		{Tag: "001", Value: "42"},
		{Tag: "008", Value: "850101s1908    enk           000 0 eng  "},
		{Tag: "020", Ind: "  ", Subfields: [][2]string{{"a", "0521720559 (pbk.)"}}},
		{Tag: "080", Ind: "  ", Subfields: [][2]string{{"a", "517.9"}}},
		{Tag: "084", Ind: "  ", Subfields: [][2]string{{"a", "510"}, {"q", "DE-101"}, {"2", "sdnb"}}},
		{Tag: "245", Ind: "10", Subfields: [][2]string{{"a", "Analysis /"}, {"c", "by A. B."}}},
	}}
}

func TestEncodeAndEach(t *testing.T) {
	data := append(Encode(book()), Encode(book())...)
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write(data)
	_ = zw.Close()
	for name, in := range map[string][]byte{"plain": data, "gzip": gz.Bytes()} {
		var got []Book
		if err := Each(bytes.NewReader(in), func(r Record) error { got = append(got, FromMARC21(r)); return nil }); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(got) != 2 {
			t.Fatalf("%s: %d books", name, len(got))
		}
		b := got[0]
		if b.ID != "42" || b.Title != "Analysis" || b.Date != "1908" || b.ISBN[0] != "0521720559" || b.UDC[0] != "517.9" || b.SDNB[0] != "510" || !Math(b) {
			t.Errorf("%s: book %+v", name, b)
		}
	}
}

func TestEachXML(t *testing.T) {
	const doc = `<?xml version="1.0"?><collection xmlns="http://www.loc.gov/MARC21/slim"><record><leader>00000nam a2200000 c 4500</leader>
<controlfield tag="001">1234567890</controlfield><datafield tag="024" ind1="7" ind2=" "><subfield code="a">urn:nbn:de:101:1-2020</subfield><subfield code="2">urn</subfield></datafield>
<datafield tag="082" ind1="0" ind2="4"><subfield code="a">515.7</subfield></datafield><datafield tag="245" ind1="1" ind2="0"><subfield code="a">Funktionalanalysis</subfield></datafield></record></collection>`
	var got []Book
	if err := Each(strings.NewReader("\n  "+doc), func(r Record) error { got = append(got, FromMARC21(r)); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "1234567890" || got[0].DDC[0] != "515.7" || got[0].URN[0] != "urn:nbn:de:101:1-2020" || got[0].Title != "Funktionalanalysis" {
		t.Errorf("books %+v", got)
	}
}

func TestFromUNIMARC(t *testing.T) {
	r := Record{Fields: []Field{
		{Tag: "001", Value: "FRBNF389727900000002"},
		{Tag: "003", Value: "http://catalogue.bnf.fr/ark:/12148/cb38972790p"},
		{Tag: "010", Ind: "  ", Subfields: [][2]string{{"a", "1-58488-210-7"}, {"b", "rel."}}},
		{Tag: "035", Ind: "  ", Subfields: [][2]string{{"a", "ocm44467383OCoLC"}}},
		{Tag: "100", Ind: "  ", Subfields: [][2]string{{"a", "20040920d2001    m  y0frey50      ba"}}},
		{Tag: "101", Ind: "0 ", Subfields: [][2]string{{"a", "eng"}}},
		{Tag: "200", Ind: "1 ", Subfields: [][2]string{{"a", "\u0098The \u009cElliptic polynomials"}, {"b", "Texte imprimé"}, {"f", "J.S. Lomont"}}},
		{Tag: "210", Ind: "  ", Subfields: [][2]string{{"a", "Boca Raton"}, {"c", "Chapman & Hall/CRC"}, {"d", "cop. 2001"}}},
		{Tag: "676", Ind: "  ", Subfields: [][2]string{{"a", "515.5"}, {"v", "23"}}},
		{Tag: "700", Ind: " |", Subfields: [][2]string{{"a", "Lomont"}, {"b", "John S."}, {"4", "070"}}},
		{Tag: "702", Ind: " |", Subfields: [][2]string{{"a", "Brillhart"}, {"b", "John"}, {"4", "340"}}},
	}}
	b := FromUNIMARC(r)
	if b.ID != "cb38972790p" || b.Title != "The Elliptic polynomials" || b.Date != "2001" || b.Language != "eng" || b.Publisher != "Chapman & Hall/CRC" || b.OCLC[0] != "44467383" || !Math(b) {
		t.Errorf("book %+v", b)
	}
	rec := SourceRecord(b, "https://catalogue.bnf.fr/ark:/12148/cb38972790p")
	if rec.Kind != "monograph" || rec.Language != "en" || rec.IDs["isbn"][0] != "9781584882107" || rec.IDs["oclc"][0] != "44467383" || len(rec.Creators) != 2 ||
		rec.Creators[0].Name != "Lomont, John S." || rec.Creators[0].Role != "author" || rec.Creators[1].Role != "editor" || rec.Subjects[0].System != "ddc" || rec.Subjects[0].Code != "515.5" {
		t.Errorf("record %+v", rec)
	}
}

func TestParseError(t *testing.T) {
	if _, err := NewReader(strings.NewReader("00010xxxxx")).Read(); err == nil {
		t.Error("a short record gives no error")
	}
}

func TestKindOfForms(t *testing.T) {
	with := func(leader string, fs ...Field) Record {
		r := book()
		r.Leader = leader
		r.Fields = append(r.Fields, fs...)
		return r
	}
	f650 := func(subs ...[2]string) Field { return Field{Tag: "650", Ind: " 0", Subfields: subs} }
	cases := []struct {
		r    Record
		want string
	}{
		{with(""), "monograph"},
		{with("", f650([2]string{"a", "Calculus"}, [2]string{"v", "Problems, exercises, etc."})), "problem-book"},
		{with("", f650([2]string{"a", "Logarithms"}, [2]string{"v", "Tables."})), "table"},
		{with("", f650([2]string{"a", "Mathematics"}, [2]string{"x", "Dictionaries."})), "encyclopedia"},
		{with("", f650([2]string{"a", "Mathematics"}, [2]string{"v", "Handbooks, manuals, etc."})), "handbook"},
		{with("", Field{Tag: "655", Ind: " 7", Subfields: [][2]string{{"a", "Textbooks."}}}), "textbook"},
		{with("00000ctm a2200000 a 4500"), "manuscript"},
	}
	for _, c := range cases {
		if got := Kind(FromMARC21(c.r)); got != c.want {
			t.Errorf("Kind = %q, want %q", got, c.want)
		}
	}
	u := Record{Leader: "00000cbm  2200000   4500", Fields: []Field{{Tag: "001", Value: "x"}, {Tag: "606", Ind: "  ", Subfields: [][2]string{{"a", "Algèbre"}, {"j", "Problèmes et exerc" + "ices"}}}}}
	if b := FromUNIMARC(u); !b.Manuscript || Kind(b) != "manuscript" {
		t.Errorf("UNIMARC manuscript %+v", b)
	}
	u.Leader = ""
	if b := FromUNIMARC(u); Kind(b) != "problem-book" {
		t.Errorf("UNIMARC kind %q, forms %v", Kind(b), b.Forms)
	}
}
