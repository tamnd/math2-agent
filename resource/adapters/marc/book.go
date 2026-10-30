package marc

import (
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
	"github.com/tamnd/math2-agent/resource/scope"
)

// Creator is one creator of a book, with the relator of the record.
type Creator struct {
	Name     string `json:"name"`
	Relator  string `json:"relator,omitempty"`
	Personal bool   `json:"personal,omitempty"`
	Main     bool   `json:"main,omitempty"` // the main entry (1XX in MARC 21, 700 or 710 in UNIMARC)
}

// Book is one book, as one line of the snapshot of an adapter.
type Book struct {
	ID         string    `json:"id"` // the control number (001), or the ARK of a record of the BnF
	LCCN       string    `json:"lccn,omitempty"`
	ISBN       []string  `json:"isbn,omitempty"`
	ISSN       []string  `json:"issn,omitempty"`
	OCLC       []string  `json:"oclc,omitempty"`
	URN        []string  `json:"urn,omitempty"`
	LCC        []string  `json:"lcc,omitempty"`
	DDC        []string  `json:"ddc,omitempty"`
	UDC        []string  `json:"udc,omitempty"`
	SDNB       []string  `json:"sdnb,omitempty"` // the subject groups of the DNB (084 with $2 sdnb)
	BK         []string  `json:"bk,omitempty"`   // the Basisklassifikation (084 with $2 bkl)
	Title      string    `json:"title"`
	Edition    string    `json:"edition,omitempty"`
	Publisher  string    `json:"publisher,omitempty"`
	Date       string    `json:"date,omitempty"`
	Language   string    `json:"language,omitempty"`
	Creators   []Creator `json:"creators,omitempty"`
	Series     []string  `json:"series,omitempty"`
	Headings   []string  `json:"headings,omitempty"`
	Forms      []string  `json:"forms,omitempty"`      // the form subdivisions and the genre terms, as Problems, exercises, etc.
	Manuscript bool      `json:"manuscript,omitempty"` // the leader gives the type manuscript language material
	Contents   []string  `json:"contents,omitempty"`
}

// clean takes the punctuation of ISBD from the end of a value, as the / in Calculus /. It also takes out the marks of the start and the end of the part of a title that is not sorted.
func clean(s string) string {
	s = strings.NewReplacer("\u0088", "", "\u0089", "", "\u0098", "", "\u009c", "").Replace(s)
	return strings.TrimSpace(strings.TrimRight(strings.TrimSpace(s), " /:;,="))
}

// cleanEnd also takes a period from the end, but not the period of an initial or an abbreviation.
func cleanEnd(s string) string {
	s = clean(s)
	if strings.HasSuffix(s, ".") && !initial.MatchString(s) {
		s = strings.TrimSuffix(s, ".")
	}
	return clean(s)
}

// initial finds a value that ends in an initial or an abbreviation, as G. H. or ed.
var initial = regexp.MustCompile(`(?:\b[A-Z]|\b[a-z]{1,3})\.$`)

// year finds the first year in a date of publication, as 2001 in cop. 2001.
var year = regexp.MustCompile(`[0-9]{4}`)

// oclcNumber gives the OCLC number of a value of 035, as 2779601 in (OCoLC)ocm2779601 or in ocm2779601OCoLC. The second value is false when the value is not an OCLC number.
func oclcNumber(v string) (string, bool) {
	v = strings.TrimSpace(v)
	n, ok := strings.CutPrefix(v, "(OCoLC)")
	if !ok {
		if n, ok = strings.CutSuffix(v, "OCoLC"); !ok {
			return "", false
		}
	}
	return strings.TrimLeft(strings.TrimSpace(n), "ocmnl"), true
}

// FromMARC21 gives the book of a record in MARC 21.
func FromMARC21(r Record) Book {
	b := Book{ID: strings.TrimSpace(r.Control("001"))}
	if f := r.Control("008"); len(f) >= 38 {
		if d := f[7:11]; strings.Trim(d, "0123456789") == "" {
			b.Date = d
		}
		b.Language = strings.TrimSpace(f[35:38])
	}
	for _, f := range r.All("010") {
		// A revision mark follows a slash, as 00004426 //r842.
		if v, _, _ := strings.Cut(f.First("a"), "/"); b.LCCN == "" {
			b.LCCN = strings.TrimSpace(v)
		}
	}
	for _, f := range r.All("020") {
		for _, v := range f.Sub("a") {
			if v, _, _ = strings.Cut(strings.TrimSpace(v), " "); v != "" {
				b.ISBN = append(b.ISBN, v)
			}
		}
	}
	for _, f := range r.All("022") {
		b.ISSN = append(b.ISSN, f.Sub("a")...)
	}
	for _, f := range r.All("024") {
		if strings.EqualFold(f.First("2"), "urn") {
			b.URN = append(b.URN, f.Sub("a")...)
		}
	}
	for _, f := range r.All("035") {
		for _, v := range f.Sub("a") {
			if n, ok := oclcNumber(v); ok {
				b.OCLC = append(b.OCLC, n)
			}
		}
	}
	for _, f := range r.All("050") {
		if a := f.First("a"); a != "" {
			b.LCC = append(b.LCC, strings.TrimSpace(a+" "+f.First("b")))
		}
	}
	for _, f := range r.All("080") {
		b.UDC = append(b.UDC, f.Sub("a")...)
	}
	for _, f := range r.All("082") {
		for _, v := range f.Sub("a") {
			b.DDC = append(b.DDC, strings.ReplaceAll(strings.TrimSpace(v), "/", ""))
		}
	}
	for _, f := range r.All("084") {
		switch strings.ToLower(f.First("2")) {
		case "sdnb":
			b.SDNB = append(b.SDNB, f.Sub("a")...)
		case "bkl":
			b.BK = append(b.BK, f.Sub("a")...)
		}
	}
	if fs := r.All("245"); len(fs) > 0 {
		t := clean(fs[0].First("a"))
		if sub := clean(strings.Join(fs[0].Sub("b"), " ")); sub != "" {
			t += ": " + sub
		}
		b.Title = cleanEnd(strings.Join(strings.Fields(t), " "))
	}
	if fs := r.All("250"); len(fs) > 0 {
		b.Edition = cleanEnd(fs[0].First("a"))
	}
	for _, tag := range []string{"264", "260"} {
		for _, f := range r.All(tag) {
			if b.Publisher == "" {
				b.Publisher = cleanEnd(f.First("b"))
			}
		}
	}
	for _, tag := range []string{"100", "110", "700", "710"} {
		for _, f := range r.All(tag) {
			if n := cleanEnd(f.First("a")); n != "" {
				b.Creators = append(b.Creators, Creator{Name: n, Relator: cleanEnd(strings.Join(append(f.Sub("e"), f.Sub("4")...), " ")), Personal: tag[1:] == "00", Main: tag[0] == '1'})
			}
		}
	}
	for _, tag := range []string{"490", "830"} {
		for _, f := range r.All(tag) {
			b.Series = addSeries(b.Series, f.First("a"), f.First("v"))
		}
	}
	for _, f := range r.All("650") {
		if h := heading(f); h != "" {
			b.Headings = append(b.Headings, h)
		}
		b.Forms = addForms(b.Forms, f.Sub("v")...)
	}
	for _, f := range r.All("655") {
		b.Forms = addForms(b.Forms, f.Sub("a")...)
		b.Forms = addForms(b.Forms, f.Sub("v")...)
	}
	b.Manuscript = len(r.Leader) > 6 && r.Leader[6] == 't'
	for _, f := range r.All("505") {
		if a := strings.TrimSpace(f.First("a")); a != "" {
			b.Contents = append(b.Contents, a)
		}
		for _, t := range f.Sub("t") {
			if t = cleanEnd(t); t != "" {
				b.Contents = append(b.Contents, t)
			}
		}
	}
	return b
}

// FromUNIMARC gives the book of a record in UNIMARC, as the BnF gives it. The id is the ARK name of the record, as cb38972790p, from the field 003, or the control number when the record has no ARK.
func FromUNIMARC(r Record) Book {
	b := Book{ID: strings.TrimSpace(r.Control("001"))}
	if _, ark, ok := strings.Cut(r.Control("003"), "ark:/12148/"); ok && strings.TrimSpace(ark) != "" {
		b.ID = strings.TrimSpace(ark)
	}
	for _, f := range r.All("100") {
		// The general data has the type of date at the position 8 and the first date at the positions 9 to 12.
		if a := f.First("a"); len(a) >= 13 && b.Date == "" {
			if d := a[9:13]; strings.Trim(d, "0123456789") == "" {
				b.Date = d
			}
		}
	}
	for _, f := range r.All("101") {
		if b.Language == "" {
			b.Language = strings.TrimSpace(f.First("a"))
		}
	}
	for _, f := range r.All("010") {
		for _, v := range f.Sub("a") {
			if v, _, _ = strings.Cut(strings.TrimSpace(v), " "); v != "" {
				b.ISBN = append(b.ISBN, v)
			}
		}
	}
	for _, f := range r.All("011") {
		b.ISSN = append(b.ISSN, f.Sub("a")...)
	}
	for _, f := range r.All("035") {
		for _, v := range f.Sub("a") {
			if n, ok := oclcNumber(v); ok {
				b.OCLC = append(b.OCLC, n)
			}
		}
	}
	for _, f := range r.All("675") {
		b.UDC = append(b.UDC, f.Sub("a")...)
	}
	for _, f := range r.All("676") {
		for _, v := range f.Sub("a") {
			b.DDC = append(b.DDC, strings.ReplaceAll(strings.TrimSpace(v), "/", ""))
		}
	}
	for _, f := range r.All("680") {
		if a := f.First("a"); a != "" {
			b.LCC = append(b.LCC, strings.TrimSpace(a+" "+f.First("b")))
		}
	}
	if fs := r.All("200"); len(fs) > 0 {
		t := clean(fs[0].First("a"))
		if sub := clean(strings.Join(fs[0].Sub("e"), " : ")); sub != "" {
			t += ": " + sub
		}
		b.Title = cleanEnd(strings.Join(strings.Fields(t), " "))
	}
	if fs := r.All("205"); len(fs) > 0 {
		b.Edition = cleanEnd(fs[0].First("a"))
	}
	for _, tag := range []string{"214", "210"} {
		for _, f := range r.All(tag) {
			if b.Publisher == "" {
				b.Publisher = cleanEnd(f.First("c"))
			}
			if d := year.FindString(f.First("d")); b.Date == "" && d != "" {
				b.Date = d
			}
		}
	}
	for _, tag := range []string{"700", "710", "701", "711", "702", "712"} {
		for _, f := range r.All(tag) {
			n := cleanEnd(f.First("a"))
			if rest := cleanEnd(f.First("b")); n != "" && rest != "" {
				n += ", " + rest
			}
			if n != "" {
				b.Creators = append(b.Creators, Creator{Name: n, Relator: strings.Join(f.Sub("4"), " "), Personal: tag[1] == '0', Main: tag[2] == '0'})
			}
		}
	}
	for _, f := range r.All("225") {
		b.Series = addSeries(b.Series, f.First("a"), f.First("v"))
	}
	for _, f := range r.All("606") {
		if h := heading(f); h != "" {
			b.Headings = append(b.Headings, h)
		}
		b.Forms = addForms(b.Forms, f.Sub("j")...)
	}
	for _, f := range r.All("608") {
		b.Forms = addForms(b.Forms, f.Sub("a")...)
	}
	b.Manuscript = len(r.Leader) > 6 && r.Leader[6] == 'b'
	for _, f := range r.All("327") {
		for _, a := range f.Sub("a") {
			if a = cleanEnd(a); a != "" {
				b.Contents = append(b.Contents, a)
			}
		}
	}
	return b
}

// addSeries adds the series a with the volume v to the list, when the list does not have it.
func addSeries(list []string, a, v string) []string {
	s := cleanEnd(a)
	if v = cleanEnd(v); v != "" {
		s += " ; " + v
	}
	if s != "" && !slices.Contains(list, s) {
		list = append(list, s)
	}
	return list
}

// heading gives a subject heading with its subdivisions, as Calculus -- Textbooks.
func heading(f Field) string {
	h := cleanEnd(f.First("a"))
	for _, x := range f.Sub("x") {
		h += " -- " + cleanEnd(x)
	}
	return h
}

// addForms adds each form term that list does not hold.
func addForms(list []string, terms ...string) []string {
	for _, t := range terms {
		if t = cleanEnd(t); t != "" && !slices.Contains(list, t) {
			list = append(list, t)
		}
	}
	return list
}

// forms are the kinds of RS2.3 that a form term gives, with the start of the term in lower case, in English, French and German. The first kind that matches wins.
var forms = []struct {
	kind  string
	words []string
}{
	{"problem-book", []string{"problems, exercises", "problèmes et exerc", "aufgabensammlung"}},
	{"table", []string{"tables", "tables numériques", "tabellen"}},
	{"encyclopedia", []string{"encyclopedias", "dictionaries", "dictionnaires", "encyclopédies", "wörterbuch", "enzyklopädie"}},
	{"handbook", []string{"handbooks, manuals", "guides, manuels", "handbuch"}},
	{"textbook", []string{"textbooks", "manuels d'enseignement", "lehrbuch"}},
}

// formKind gives the kind that the form terms and the subdivisions of the headings of b give, or an empty string.
func formKind(b Book) string {
	var terms []string
	for _, f := range b.Forms {
		terms = append(terms, strings.ToLower(f))
	}
	for _, h := range b.Headings {
		for i, part := range strings.Split(h, " -- ") {
			if i > 0 {
				terms = append(terms, strings.ToLower(part))
			}
		}
	}
	for _, f := range forms {
		for _, t := range terms {
			for _, w := range f.words {
				if strings.HasPrefix(t, w) {
					return f.kind
				}
			}
		}
	}
	return ""
}

// Kind gives the kind of a book. A manuscript is a manuscript. Else a form term or a subdivision of a heading gives the kind, as Problems, exercises, etc. for a problem book. Else the book is an edited volume when it has no main entry and an added entry is an editor, and a monograph when not.
func Kind(b Book) string {
	if b.Manuscript {
		return "manuscript"
	}
	if k := formKind(b); k != "" {
		return k
	}
	editor := false
	for _, c := range b.Creators {
		if c.Main {
			return "monograph"
		}
		editor = editor || Role(c.Relator) == "editor"
	}
	if editor {
		return "edited-volume"
	}
	return "monograph"
}

// Role maps a relator, as editor or edt in MARC 21 or 340 in UNIMARC, to a role of RS2.5.
func Role(rel string) string {
	r := strings.ToLower(strings.TrimSpace(rel))
	switch {
	case r == "":
		return "author"
	case r == "070":
		return "author"
	case r == "340" || r == "651":
		return "editor"
	case r == "730":
		return "translator"
	case strings.Contains(r, "edt") || strings.Contains(r, "editor") || strings.HasPrefix(r, "ed") || strings.Contains(r, "hrsg"):
		return "editor"
	case strings.Contains(r, "trl") || strings.Contains(r, "transl") || strings.Contains(r, "übers"):
		return "translator"
	case strings.Contains(r, "aut") || strings.Contains(r, "author") || strings.Contains(r, "verfasser"):
		return "author"
	}
	return "contributor"
}

// Scope gives the classes of a book for the scope tests of RS9.2.
func Scope(b Book) scope.Record {
	return scope.Record{LCC: b.LCC, DDC: b.DDC, UDC: b.UDC, BK: b.BK}
}

// Math reports whether a book is a book of mathematics by the test S3 of RS9.2: LCC QA1 to QA939 without QA75.5 to QA76.95, DDC 510 to 519, UDC 51 or BK 31.
func Math(b Book) bool { return scope.S3(Scope(b)) }

// SourceRecord makes the source record of one book with the link of its catalogue page. A MARC record describes one edition, so its level is manifestation. The record has the ids isbn, issn, oclc, lccn and urn of RS3.2, and the classes lcc, ddc, udc, bk and sdnb.
func SourceRecord(b Book, link string) cluster.Record {
	rec := cluster.Record{NativeID: b.ID, Level: "manifestation", Kind: Kind(b), Title: b.Title, Date: b.Date, Language: fetch.Lang(b.Language), Edition: b.Edition, Publisher: b.Publisher, IDs: resource.IDs{}}
	if link != "" {
		rec.Links = []string{link}
	}
	put := func(key string, vs ...string) {
		for _, v := range vs {
			if n, err := ids.Normalize(key, v); err == nil && !slices.Contains(rec.IDs[key], n) {
				rec.IDs[key] = append(rec.IDs[key], n)
			}
		}
	}
	put("lccn", b.LCCN)
	put("isbn", b.ISBN...)
	put("issn", b.ISSN...)
	put("oclc", b.OCLC...)
	put("urn", b.URN...)
	for _, c := range b.Creators {
		rec.Creators = append(rec.Creators, resource.Creator{Name: c.Name, Role: Role(c.Relator)})
	}
	for _, s := range []struct {
		system string
		codes  []string
	}{{"lcc", b.LCC}, {"ddc", b.DDC}, {"udc", b.UDC}, {"bk", b.BK}, {"sdnb", b.SDNB}} {
		for _, c := range s.codes {
			if c = strings.TrimSpace(c); c != "" {
				rec.Subjects = append(rec.Subjects, resource.Subject{System: s.system, Code: c})
			}
		}
	}
	return rec
}
