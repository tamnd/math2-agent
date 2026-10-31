// Package fetch makes the census records of RS15.2 from the APIs of the catalogues and from the metadata plane of arXiv. A Sampler takes a random sample of a catalogue, and a Looker tells which DOIs a catalogue has, so the census can measure the overlap of a sampled catalogue.
//
// The samplers take whole pages at random offsets, because the APIs page by offset. Each record has the weight total / n, the number of records of the catalogue that it stands for.
package fetch

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census"
	"github.com/tamnd/math2-agent/resource/ids"
	"github.com/tamnd/math2-agent/resource/scope"
)

// Info tells what a sample read.
type Info struct {
	Total  int    // the records of the catalogue that match the filter
	Filter string // the filter of the sample, in words
}

// A Sampler takes a random sample of about n records of one catalogue.
type Sampler interface {
	Name() string
	Sample(ctx context.Context, c *Client, n int, seed uint64, emit func(census.Record) error) (Info, error)
}

// A Looker tells which DOIs of a list one catalogue has. The DOIs are in the normal form of resource/ids.
type Looker interface {
	Name() string
	Batch() int // the DOIs of one request
	Lookup(ctx context.Context, c *Client, dois []string) (map[string]bool, error)
}

// ISBNLooker is a Looker that can also look up ISBNs, for the books with no DOI.
type ISBNLooker interface {
	LookupISBN(ctx context.Context, c *Client, isbns []string) (map[string]bool, error)
}

// Paths names the local files that some samplers read.
type Paths struct {
	Arxiv string // the metadata plane of a checkout of tamnd/arxiv
	DBLP  string // the XML dump of dblp
	Hathi string // a Hathifile
}

// Samplers gives the samplers by the name of the catalogue. The samplers of arXiv, dblp and HathiTrust read the local files of p.
func Samplers(p Paths) map[string]Sampler {
	return map[string]Sampler{
		"arxiv":           Arxiv{Dir: p.Arxiv},
		"zbmath":          ZBMath{Base: ZBMathBase},
		"openalex":        OpenAlex{Base: OpenAlexBase, Filter: OpenAlexMath},
		"crossref":        Crossref{Base: CrossrefBase},
		"hal":             HAL{Base: HALBase},
		"openlibrary":     OpenLibrary{Base: OpenLibraryBase, Subject: "mathematics"},
		"datacite":        DataCite{Base: DataCiteBase},
		"doaj":            DOAJ{Base: DOAJBase},
		"gutenberg":       Gutenberg{Base: GutenbergBase},
		"internetarchive": Archive{Base: ArchiveBase},
		"wikidata":        Wikidata{Base: WikidataBase},
		"numdam":          Numdam{Base: NumdamOAIBase},
		"loc":             LoC{Base: LoCSRUBase},
		"hathitrust":      Hathi{File: p.Hathi},
		"doab":            OAIBooks{Catalogue: "doab", Base: DOABOAIBase, Set: DOABBooks},
		"oapen":           OAIBooks{Catalogue: "oapen", Base: OAPENOAIBase},
		"eudml":           EuDML{Base: EuDMLOAIBase},
		"dblp":            DBLP{File: p.DBLP},
		"semanticscholar": SemanticScholar{Base: SemanticScholarBase},
		"mathnetru":       MathNetRu{Base: MathNetRuBase, Gap: MathNetRuGap},
		"cyberleninka":    CyberLeninka{Base: CyberLeninkaOAIBase},
	}
}

// Lookers gives the lookers by the name of the catalogue.
func Lookers() map[string]Looker {
	return map[string]Looker{
		"zbmath":   ZBMath{Base: ZBMathBase},
		"openalex": OpenAlex{Base: OpenAlexBase},
		"crossref": Crossref{Base: CrossrefBase},
		"hal":      HAL{Base: HALBase},
	}
}

// Names gives the keys of a map in order.
func Names[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// pages gives k distinct random pages of a catalogue of total records with size records on each page, from 0.
func pages(total, size, k int, seed uint64) []int {
	n := (total + size - 1) / size
	return census.Offsets(n, k, seed)
}

// weight gives total / n, or 1.
func weight(total, n int) float64 {
	if n <= 0 || total <= n {
		return 1
	}
	return float64(total) / float64(n)
}

// languages maps the names and the MARC codes of languages to BCP 47.
var languages = map[string]string{
	"english": "en", "eng": "en", "french": "fr", "fre": "fr", "fra": "fr", "german": "de", "ger": "de", "deu": "de",
	"russian": "ru", "rus": "ru", "italian": "it", "ita": "it", "spanish": "es", "spa": "es", "chinese": "zh", "chi": "zh", "zho": "zh",
	"japanese": "ja", "jpn": "ja", "polish": "pl", "pol": "pl", "portuguese": "pt", "por": "pt", "czech": "cs", "cze": "cs", "ces": "cs",
	"ukrainian": "uk", "ukr": "uk", "romanian": "ro", "rum": "ro", "ron": "ro", "hungarian": "hu", "hun": "hu", "bulgarian": "bg", "bul": "bg",
	"dutch": "nl", "dut": "nl", "nld": "nl", "serbian": "sr", "srp": "sr", "croatian": "hr", "hrv": "hr", "korean": "ko", "kor": "ko",
	"turkish": "tr", "tur": "tr", "persian": "fa", "per": "fa", "fas": "fa", "vietnamese": "vi", "vie": "vi", "swedish": "sv", "swe": "sv",
	"greek": "el", "gre": "el", "ell": "el", "slovak": "sk", "slo": "sk", "slk": "sk", "finnish": "fi", "fin": "fi", "arabic": "ar", "ara": "ar",
	"hebrew": "he", "heb": "he", "indonesian": "id", "ind": "id", "latin": "la", "lat": "la", "danish": "da", "dan": "da", "norwegian": "no", "nor": "no",
}

// Lang gives the BCP 47 tag of a language name, a MARC code or a tag.
func Lang(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if l, ok := languages[s]; ok {
		return l
	}
	return s
}

// LCC gives an LCC class of Open Library, as QA-0043.00000000 or QA-0076.73000000.P98, in the usual form, as QA43 or QA76.73.P98.
func LCC(s string) string {
	letters, rest, ok := strings.Cut(s, "-")
	if !ok {
		return s
	}
	num, cutter, _ := strings.Cut(rest, ".")
	frac := ""
	if i := strings.IndexFunc(cutter, func(r rune) bool { return r < '0' || r > '9' }); i >= 0 {
		frac, cutter = cutter[:i], cutter[i:]
	} else {
		frac, cutter = cutter, ""
	}
	num = strings.TrimLeft(num, "0")
	if num == "" {
		num = "0"
	}
	out := letters + num
	if frac = strings.TrimRight(frac, "0"); frac != "" {
		out += "." + frac
	}
	if cutter = strings.Trim(cutter, ". "); cutter != "" {
		out += "." + cutter
	}
	return out
}

// HALDomain gives the arXiv category of a HAL domain, as math.CO for math.math-co, or an empty string.
func HALDomain(d string) string {
	if i := strings.IndexByte(d, '.'); i == 1 && d[0] >= '0' && d[0] <= '9' {
		d = d[2:]
	}
	top, sub, _ := strings.Cut(d, ".")
	switch {
	case top == "math" && sub == "":
		return "math"
	case top == "math" && strings.HasPrefix(sub, "math-"):
		return "math." + strings.ToUpper(strings.TrimPrefix(sub, "math-"))
	case top == "phys" && sub == "mphy":
		return "math-ph"
	case top == "info" && strings.HasPrefix(sub, "info-"):
		return "cs." + strings.ToUpper(strings.TrimPrefix(sub, "info-"))
	case top == "stat" && strings.HasPrefix(sub, "stat-"):
		return "stat." + strings.ToUpper(strings.TrimPrefix(sub, "stat-"))
	case top != "" && sub == "":
		return top
	}
	return ""
}

// addUnique adds the values that the list does not have.
func addUnique(list []string, vs ...string) []string {
	for _, v := range vs {
		if v != "" && !slices.Contains(list, v) {
			list = append(list, v)
		}
	}
	return list
}

// batches cuts the list into parts of at most n.
func batches(list []string, n int) [][]string {
	var out [][]string
	for len(list) > 0 {
		k := min(n, len(list))
		out = append(out, list[:k])
		list = list[k:]
	}
	return out
}

// wrap adds the name of the catalogue to an error.
func wrap(name string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", name, err)
}

// Mark chooses at random up to n records of rs that are in scope and have a DOI, and looks up their DOIs in each looker that is not the catalogue self. Then it chooses at random up to n records that are in scope, have no DOI and have an ISBN, and looks up their ISBNs in each looker that is an ISBNLooker. A record of a book is found when the looker has one of its ISBNs. It sets the field Found of each chosen record, and gives the number of chosen records.
func Mark(ctx context.Context, c *Client, rs []census.Record, env scope.Env, self string, lookers []Looker, n int, seed uint64) (int, error) {
	var withDOI, withISBN []int
	keys := map[int][]string{}
	for i := range rs {
		if scope.Decide(rs[i].Signals, env).Value != resource.In {
			continue
		}
		for _, v := range rs[i].IDs["doi"] {
			if d, err := ids.Normalize("doi", v); err == nil {
				keys[i] = []string{d}
				withDOI = append(withDOI, i)
				break
			}
		}
		if keys[i] != nil {
			continue
		}
		for _, v := range rs[i].IDs["isbn"] {
			if d, err := ids.Normalize("isbn", v); err == nil && !slices.Contains(keys[i], d) {
				keys[i] = append(keys[i], d)
			}
		}
		if keys[i] != nil {
			withISBN = append(withISBN, i)
		}
	}
	choose := func(cand []int) []int {
		var out []int
		for _, k := range census.Offsets(len(cand), n, seed) {
			out = append(out, cand[k])
		}
		return out
	}
	byDOI, byISBN := choose(withDOI), choose(withISBN)
	for _, l := range lookers {
		if l.Name() == self {
			continue
		}
		look := []func(context.Context, *Client, []string) (map[string]bool, error){l.Lookup}
		chosen := [][]int{byDOI}
		if il, ok := l.(ISBNLooker); ok {
			look = append(look, il.LookupISBN)
			chosen = append(chosen, byISBN)
		}
		for j, set := range chosen {
			var list []string
			for _, i := range set {
				list = append(list, keys[i]...)
			}
			found := map[string]bool{}
			for _, b := range batches(list, l.Batch()) {
				f, err := look[j](ctx, c, b)
				if err != nil {
					return 0, err
				}
				for k := range f {
					found[k] = true
				}
			}
			for _, i := range set {
				if rs[i].Found == nil {
					rs[i].Found = map[string]bool{}
				}
				rs[i].Found[l.Name()] = slices.ContainsFunc(keys[i], func(k string) bool { return found[k] })
			}
		}
	}
	return len(byDOI) + len(byISBN), nil
}
