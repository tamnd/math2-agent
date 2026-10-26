// Package sources confirms the sources that an agent claims (corpus K9.5).
//
// A model makes up citations that look right: a paper with a real author and a title that the author never wrote, or a real paper with the wrong year. Thus each source that an agent writes into a node goes through the check of this package before the pull request opens. The check looks up the claimed source in a catalogue, the record plane of m2 ingest or a fake in the tests, and compares the title, the authors and the year of the claim with the record. A claim that resolves to no record counts as fabricated, and the merge cannot go on (rule P05).
//
// The check confirms that the source exists and that the claim names it correctly. It does not confirm that the source has the result at the locator: a person confirms the locator, because the record plane does not have the text of most sources (rule P06).
package sources

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/prov"
)

// TitleSimilarity is the least Jaccard similarity of the title words of a claim and its record for the titles to agree.
const TitleSimilarity = 0.8

// Claim is a source as an agent claims it. The run record keeps the claims of the run.
type Claim = prov.SourceClaim

// Record is a source as a catalogue has it.
type Record struct {
	System  string   // the system of the record, as in the sources of a node, for example zbmath
	ID      string   // the id in the system
	Title   string   // the title
	Authors []string // the authors, each one as the catalogue writes the name
	Year    string   // the year of the publication, or empty when the catalogue does not give it
	URL     string   // the home of the record
}

// Resolver looks up a source in a catalogue. Resolve returns false when the catalogue has no record with the id. An error means that the catalogue cannot answer, not that the source does not exist.
type Resolver interface {
	Resolve(system, id string) (Record, bool, error)
}

// Verdict is the result of a check of a claim.
type Verdict string

// The verdicts.
const (
	Confirmed  Verdict = "confirmed"  // the source exists and the claim names it correctly
	Mismatch   Verdict = "mismatch"   // the source exists, but the title, the authors or the year of the claim are not those of the record
	Fabricated Verdict = "fabricated" // the catalogue has no record of the source
)

// Result is the result of the check of one claim.
type Result struct {
	Verdict  Verdict
	Record   Record   // the record, when the source exists
	Problems []string // why the claim does not agree with the record, in plain words
	// LocatorConfirmed is always false: the check does not confirm the locator. A person confirms it, and a node with a locator that nobody confirmed has the field locator_unconfirmed.
	LocatorConfirmed bool
}

// Confirm checks the claim against the catalogue.
func Confirm(r Resolver, c Claim) (Result, error) {
	rec, ok, err := r.Resolve(c.System, c.ID)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return Result{Verdict: Fabricated, Problems: []string{"no record of " + c.System + " " + c.ID}}, nil
	}
	res := Result{Verdict: Confirmed, Record: rec}
	if c.Title != "" && rec.Title != "" {
		if s := Similarity(c.Title, rec.Title); s < TitleSimilarity {
			res.Problems = append(res.Problems, "the title is not the title of the record, "+strconv.Quote(rec.Title))
		}
	}
	have := map[string]bool{}
	for _, a := range rec.Authors {
		for _, w := range words(a) {
			have[w] = true
		}
	}
	for _, a := range c.Authors {
		if s := Surname(a); s != "" && len(rec.Authors) > 0 && !have[s] {
			res.Problems = append(res.Problems, "the record has no author "+strconv.Quote(a))
		}
	}
	if c.Year != "" && rec.Year != "" && c.Year != rec.Year {
		res.Problems = append(res.Problems, "the year is "+rec.Year+" in the record")
	}
	if len(res.Problems) > 0 {
		res.Verdict = Mismatch
	}
	return res, nil
}

// Similarity returns the Jaccard similarity of the sets of the words of two titles, after normalization.
func Similarity(a, b string) float64 {
	x, y := map[string]bool{}, map[string]bool{}
	for _, w := range words(a) {
		x[w] = true
	}
	for _, w := range words(b) {
		y[w] = true
	}
	if len(x) == 0 && len(y) == 0 {
		return 1
	}
	both := 0
	for w := range x {
		if y[w] {
			both++
		}
	}
	return float64(both) / float64(len(x)+len(y)-both)
}

// Surname returns the normalized surname of a name: the part before the comma of "Surname, Forename", or else the last word.
func Surname(name string) string {
	if i := strings.IndexByte(name, ','); i >= 0 {
		name = name[:i]
	}
	w := words(name)
	if len(w) == 0 {
		return ""
	}
	return w[len(w)-1]
}

// words returns the words of s in lower case, with no accents and no punctuation. A formula of TeX in a title gives its letters.
func words(s string) []string {
	s = norm.NFKD.String(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteByte(' ')
		}
	}
	return strings.Fields(b.String())
}

// Plane is a Resolver on the records of m2 ingest. The zero value has no records.
type Plane struct {
	bySource map[string]map[string]*ingest.Record
	byKey    map[string]map[string]*ingest.Record // the records by doi and by isbn
	source   map[*ingest.Record]string
}

// systems maps a system of the sources of a node to the source of m2 ingest with its records, when the names differ.
var systems = map[string]string{"erdos": "erdosproblems"}

// Add adds the records of a source of m2 ingest, for example zbmath.
func (p *Plane) Add(source string, records ...ingest.Record) {
	if p.bySource == nil {
		p.bySource, p.byKey, p.source = map[string]map[string]*ingest.Record{}, map[string]map[string]*ingest.Record{"doi": {}, "isbn": {}}, map[*ingest.Record]string{}
	}
	m := p.bySource[source]
	if m == nil {
		m = map[string]*ingest.Record{}
		p.bySource[source] = m
	}
	for i := range records {
		r := &records[i]
		m[r.ID] = r
		p.source[r] = source
		for k := range p.byKey {
			if v := strings.ToLower(strings.TrimSpace(r.Meta[k])); v != "" {
				if _, ok := p.byKey[k][v]; !ok {
					p.byKey[k][v] = r
				}
			}
		}
	}
}

// arxivVersion is the version at the end of an arXiv id, for example v2.
var arxivVersion = regexp.MustCompile(`v[0-9]+$`)

// Resolve looks up the source. A doi resolves to a record of any source with the doi, and a book, isbn: and the ISBN, to a record with the isbn. An arXiv id with a version resolves to the record of the paper.
func (p *Plane) Resolve(system, id string) (Record, bool, error) {
	var r *ingest.Record
	switch system {
	case "doi":
		r = p.byKey["doi"][strings.ToLower(strings.TrimPrefix(id, "https://doi.org/"))]
	case "book":
		r = p.byKey["isbn"][strings.TrimPrefix(strings.ToLower(strings.ReplaceAll(id, "-", "")), "isbn:")]
	default:
		m := p.bySource[cmp.Or(systems[system], system)]
		r = m[id]
		if r == nil && system == "arxiv" {
			r = m[arxivVersion.ReplaceAllString(id, "")]
		}
	}
	if r == nil {
		return Record{}, false, nil
	}
	return p.record(r), true, nil
}

// record returns the record of an ingest record.
func (p *Plane) record(r *ingest.Record) Record {
	src := p.source[r]
	out := Record{System: src, ID: r.ID, Title: r.Title, URL: r.URL, Year: r.Meta["year"]}
	if src == "erdosproblems" {
		out.System = "erdos"
	}
	if a := r.Meta["authors"]; a != "" {
		sep := ";"
		if !strings.Contains(a, sep) {
			sep = "|"
		}
		for s := range strings.SplitSeq(a, sep) {
			if s = strings.TrimSpace(s); s != "" {
				out.Authors = append(out.Authors, s)
			}
		}
	}
	if out.Year == "" && src == "arxiv" {
		out.Year = ArxivYear(r.ID)
	}
	return out
}

// arxivID matches the arXiv ids of the new form, 0704.0001 to 2312.00752, and of the old form, math/0211159.
var arxivID = regexp.MustCompile(`^(?:[a-z.-]+/)?([0-9]{2})([0-9]{2})[.0-9]*(?:v[0-9]+)?$`)

// ArxivYear returns the year of the first version of an arXiv paper, from its id, or empty when the id has no year.
func ArxivYear(id string) string {
	m := arxivID.FindStringSubmatch(strings.ToLower(id))
	if m == nil {
		return ""
	}
	yy, _ := strconv.Atoi(m[1])
	mm, _ := strconv.Atoi(m[2])
	if mm < 1 || mm > 12 {
		return ""
	}
	if yy >= 91 {
		return strconv.Itoa(1900 + yy)
	}
	return strconv.Itoa(2000 + yy)
}

// Knows reports whether the plane can look up the sources of the system: it has records of the system, or records with a doi or an isbn for the systems doi and book. A source of a system that the plane does not know cannot resolve, but that does not show that it is fabricated.
func (p *Plane) Knows(system string) bool {
	switch system {
	case "doi":
		return len(p.byKey["doi"]) > 0
	case "book":
		return len(p.byKey["isbn"]) > 0
	}
	return len(p.bySource[cmp.Or(systems[system], system)]) > 0
}

// Systems returns the systems that the plane has records of, sorted.
func (p *Plane) Systems() []string {
	var out []string
	for s := range p.bySource {
		out = append(out, cmp.Or(map[string]string{"erdosproblems": "erdos"}[s], s))
	}
	slices.Sort(out)
	return out
}
