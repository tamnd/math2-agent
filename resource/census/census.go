// Package census measures the catalogues of resources before the import (corpus resources RS15.2). It counts the records of each catalogue by scope outcome and by stratum, it measures the overlap of each pair of catalogues on a sample, and it gives the Chapman estimate of each independent pair.
//
// The input of the census is one file of JSON lines for each catalogue, with one Record on each line. The adapters of the sources make these files from a dump or from a sample of an API. The census does not read a dump itself.
package census

import (
	"cmp"
	"math/rand/v2"
	"slices"
	"strings"
	"unicode"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/ids"
	"github.com/tamnd/math2-agent/resource/scope"
)

// Record is one record of a catalogue as the census reads it.
type Record struct {
	NativeID string       `json:"native_id"`
	Kind     string       `json:"kind"`               // one of resource.Kinds
	Date     string       `json:"date,omitempty"`     // ISO 8601, as precise as known
	Language string       `json:"language,omitempty"` // BCP 47
	Title    string       `json:"title,omitempty"`
	Authors  []string     `json:"authors,omitempty"` // the names as the catalogue gives them, the first author first
	IDs      resource.IDs `json:"ids,omitempty"`     // the external ids, by the keys of resource/ids
	Weight   float64      `json:"weight,omitempty"`  // the number of records of the catalogue that this record stands for. Zero means 1
	Signals  scope.Record `json:"signals"`           // the signals of the scope tests
}

// weight gives the weight of the record, 1 when it has none.
func (r *Record) weight() float64 {
	if r.Weight <= 0 {
		return 1
	}
	return r.Weight
}

// KindGroup gives the kind group of a kind (RS11.3), or other.
func KindGroup(kind string) string {
	switch kind {
	case "article", "conference-paper", "survey", "preprint":
		return "article"
	case "monograph", "textbook", "edited-volume", "proceedings", "problem-book", "handbook":
		return "book"
	case "thesis":
		return "thesis"
	case "report", "lecture-notes":
		return "report-notes"
	case "course", "offering":
		return "course"
	}
	return "other"
}

// MSC2 gives the first two digits of the primary MSC code, or none.
func MSC2(r *Record) string {
	if len(r.Signals.MSC) == 0 || len(r.Signals.MSC[0]) < 2 {
		return "none"
	}
	return r.Signals.MSC[0][:2]
}

// Decade gives the decade of the date, for example 1990s, or unknown.
func Decade(date string) string {
	if len(date) < 4 {
		return "unknown"
	}
	for _, c := range date[:4] {
		if c < '0' || c > '9' {
			return "unknown"
		}
	}
	return date[:3] + "0s"
}

// Language gives the primary language subtag in lower case, or und.
func Language(tag string) string {
	p, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(tag)), "-")
	if p == "" {
		return "und"
	}
	return p
}

// The dimensions of a count.
const (
	DimKind     = "kind group"
	DimMSC2     = "MSC2"
	DimDecade   = "decade"
	DimLanguage = "language"
)

// Dims are the dimensions of the counts, in the order of the report.
var Dims = []string{DimKind, DimMSC2, DimDecade, DimLanguage}

// Outcomes are the outcomes that the census counts. An undecided record is deferred, because the classifier is not in the census.
var Outcomes = []string{"in", "border", "out", "deferred"}

// Tally holds the weighted count of each outcome.
type Tally map[string]float64

// Total gives the sum of the outcomes.
func (t Tally) Total() float64 {
	var n float64
	for _, v := range t {
		n += v
	}
	return n
}

// Catalogue holds the counts of one catalogue and its sample of in records.
type Catalogue struct {
	Name    string
	Records int                         // the lines read
	All     Tally                       // the counts of all records
	By      map[string]map[string]Tally // the counts by dimension and by value
	Strata  map[Stratum]float64         // the weighted count of in records by kind group and decade
	Weight  float64                     // the mean weight of the records
	sample  *Reservoir[Record]
	keys    map[string]bool // the match keys of all records
	weights float64
}

// Stratum is a cell of the Chapman estimates: a kind group and a decade (RS15.2).
type Stratum struct {
	Kind, Decade string
}

// Census holds the catalogues of one census.
type Census struct {
	Env        scope.Env
	SampleSize int    // the size of the sample of in records of each catalogue
	Seed       uint64 // the seed of the samples and of the bootstrap
	cats       map[string]*Catalogue
}

// New makes an empty census.
func New(env scope.Env, sampleSize int, seed uint64) *Census {
	return &Census{Env: env, SampleSize: sampleSize, Seed: seed, cats: map[string]*Catalogue{}}
}

// Add counts one record of the catalogue.
func (c *Census) Add(catalogue string, r Record) {
	cat := c.cats[catalogue]
	if cat == nil {
		cat = &Catalogue{
			Name:   catalogue,
			All:    Tally{},
			By:     map[string]map[string]Tally{},
			Strata: map[Stratum]float64{},
			sample: NewReservoir[Record](c.SampleSize, c.Seed^hash(catalogue)),
			keys:   map[string]bool{},
		}
		c.cats[catalogue] = cat
	}
	w := r.weight()
	cat.Records++
	cat.weights += w
	cat.Weight = cat.weights / float64(cat.Records)
	d := scope.Decide(r.Signals, c.Env)
	cat.All[d.Value] += w
	vals := map[string]string{DimKind: KindGroup(r.Kind), DimMSC2: MSC2(&r), DimDecade: Decade(r.Date), DimLanguage: Language(r.Language)}
	for _, dim := range Dims {
		if cat.By[dim] == nil {
			cat.By[dim] = map[string]Tally{}
		}
		if cat.By[dim][vals[dim]] == nil {
			cat.By[dim][vals[dim]] = Tally{}
		}
		cat.By[dim][vals[dim]][d.Value] += w
	}
	for _, k := range Keys(&r) {
		cat.keys[k] = true
	}
	if d.Value == "in" {
		cat.Strata[Stratum{vals[DimKind], vals[DimDecade]}] += w
		cat.sample.Add(r)
	}
}

// Catalogues gives the catalogues in the order of their names.
func (c *Census) Catalogues() []*Catalogue {
	out := make([]*Catalogue, 0, len(c.cats))
	for _, cat := range c.cats {
		out = append(out, cat)
	}
	slices.SortFunc(out, func(a, b *Catalogue) int { return cmp.Compare(a.Name, b.Name) })
	return out
}

// Catalogue gives the catalogue with the name, or nil.
func (c *Census) Catalogue(name string) *Catalogue { return c.cats[name] }

// strongIDs are the id systems that identify one work or one expression, in the order of the match.
var strongIDs = []string{"doi", "arxiv", "zbl", "jfm", "mr", "isbn", "oclc", "lccn", "openlibrary_work", "wikidata", "openalex", "hal", "numdam", "eudml", "hathi"}

// Keys gives the match keys of a record: one key for each strong id in its normal form, and one key of the title and the family name of the first author. A value that does not normalize is not a key.
func Keys(r *Record) []string {
	var out []string
	for _, sys := range strongIDs {
		for _, v := range r.IDs[sys] {
			if n, err := ids.Normalize(sys, v); err == nil {
				out = append(out, sys+":"+n)
			}
		}
	}
	if k := titleKey(r); k != "" {
		out = append(out, k)
	}
	return out
}

// titleKey gives the key of the title and of the family name of the first author: the letters and digits in lower case, with one space between words. It is empty when the record has no title or no author.
func titleKey(r *Record) string {
	t := fold(r.Title)
	if t == "" || len(r.Authors) == 0 {
		return ""
	}
	a := r.Authors[0]
	if before, _, ok := strings.Cut(a, ","); ok {
		a = before
	} else if f := strings.Fields(a); len(f) > 0 {
		a = f[len(f)-1]
	}
	if a = fold(a); a == "" {
		return ""
	}
	return "ta:" + t + "|" + a
}

func fold(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
}

// Overlap is the overlap of the sample of in records of catalogue A with catalogue B.
type Overlap struct {
	A, B    string
	Sampled int                // the records of the sample of A
	Matched int                // the records of the sample of A with a key in B
	Strata  map[Stratum][2]int // the sampled and the matched records by stratum
}

// Fraction gives the share of the sample of A that B has, corrected by the weight of B: a record of a sample of B with weight w stands for w records, so a match is found with chance 1/w. The fraction is never more than 1.
func (o Overlap) Fraction(wB float64) float64 {
	return fraction(o.Matched, o.Sampled, wB)
}

func fraction(matched, sampled int, wB float64) float64 {
	if sampled == 0 {
		return 0
	}
	return min(1, float64(matched)/float64(sampled)*max(1, wB))
}

// Overlaps gives the overlap of each ordered pair of catalogues.
func (c *Census) Overlaps() []Overlap {
	var out []Overlap
	cats := c.Catalogues()
	for _, a := range cats {
		for _, b := range cats {
			if a == b {
				continue
			}
			o := Overlap{A: a.Name, B: b.Name, Strata: map[Stratum][2]int{}}
			for _, r := range a.sample.Items() {
				s := Stratum{KindGroup(r.Kind), Decade(r.Date)}
				n := o.Strata[s]
				n[0]++
				o.Sampled++
				if slices.ContainsFunc(Keys(&r), func(k string) bool { return b.keys[k] }) {
					n[1]++
					o.Matched++
				}
				o.Strata[s] = n
			}
			out = append(out, o)
		}
	}
	return out
}

// StratumEstimate is the Chapman estimate of one stratum for one pair.
type StratumEstimate struct {
	A, B    string
	Stratum Stratum
	Estimate
	RecallA, RecallB float64
}

// Estimates gives the Chapman estimate of each stratum for each pair of catalogues that the caller names as independent. The count m of the works in both is the count of A in the stratum times the share of the sample of A in the stratum that B has. A stratum with no work in A or in B, or with no sample of A, has no estimate.
func (c *Census) Estimates(pairs [][2]string) []StratumEstimate {
	ov := map[[2]string]Overlap{}
	for _, o := range c.Overlaps() {
		ov[[2]string{o.A, o.B}] = o
	}
	var out []StratumEstimate
	for _, p := range pairs {
		a, b := c.cats[p[0]], c.cats[p[1]]
		if a == nil || b == nil {
			continue
		}
		o := ov[p]
		var strata []Stratum
		for s := range a.Strata {
			strata = append(strata, s)
		}
		slices.SortFunc(strata, func(x, y Stratum) int { return cmp.Or(cmp.Compare(x.Kind, y.Kind), cmp.Compare(x.Decade, y.Decade)) })
		for _, s := range strata {
			nA, nB := a.Strata[s], b.Strata[s]
			n := o.Strata[s]
			if nA == 0 || nB == 0 || n[0] == 0 {
				continue
			}
			m := min(nA, nB, nA*fraction(n[1], n[0], b.Weight))
			e := Pair(round(nA), round(nB), round(m), c.Seed^hash(p[0]+"|"+p[1]+"|"+s.Kind+"|"+s.Decade))
			out = append(out, StratumEstimate{A: p[0], B: p[1], Stratum: s, Estimate: e, RecallA: Recall(e.NA, e.N), RecallB: Recall(e.NB, e.N)})
		}
	}
	return out
}

// Uncovered gives the strata of in records that no estimate of the pairs covers, in order.
func (c *Census) Uncovered(est []StratumEstimate) []Stratum {
	have := map[Stratum]bool{}
	for _, e := range est {
		have[e.Stratum] = true
	}
	all := map[Stratum]bool{}
	for _, cat := range c.cats {
		for s := range cat.Strata {
			if !have[s] {
				all[s] = true
			}
		}
	}
	out := make([]Stratum, 0, len(all))
	for s := range all {
		out = append(out, s)
	}
	slices.SortFunc(out, func(x, y Stratum) int { return cmp.Or(cmp.Compare(x.Kind, y.Kind), cmp.Compare(x.Decade, y.Decade)) })
	return out
}

func round(f float64) int { return int(f + 0.5) }

// hash gives the FNV-1a hash of s, to derive a seed for each catalogue and each pair.
func hash(s string) uint64 {
	h := uint64(14695981039346656037)
	for i := range len(s) {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

// Reservoir keeps a uniform random sample of k items of a stream of unknown length (Algorithm R). The census uses it to sample a dump.
type Reservoir[T any] struct {
	k     int
	n     int
	items []T
	rng   *rand.Rand
}

// NewReservoir makes a reservoir of k items with a generator from the seed.
func NewReservoir[T any](k int, seed uint64) *Reservoir[T] {
	return &Reservoir[T]{k: k, rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))} // #nosec G404 -- a sample needs a seeded generator, not a secure one.
}

// Add offers one item to the sample.
func (s *Reservoir[T]) Add(v T) {
	s.n++
	if len(s.items) < s.k {
		s.items = append(s.items, v)
		return
	}
	if j := s.rng.IntN(s.n); j < s.k {
		s.items[j] = v
	}
}

// Items gives the sample.
func (s *Reservoir[T]) Items() []T { return s.items }

// Seen gives the number of items that Add was given.
func (s *Reservoir[T]) Seen() int { return s.n }

// Offsets gives k distinct random offsets in [0, total), in order. The census uses them to sample an API that pages by offset. When k is total or more, it gives each offset.
func Offsets(total, k int, seed uint64) []int {
	if k >= total {
		out := make([]int, max(total, 0))
		for i := range out {
			out[i] = i
		}
		return out
	}
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) // #nosec G404 -- a sample needs a seeded generator, not a secure one.
	seen := make(map[int]bool, k)
	out := make([]int, 0, k)
	for len(out) < k {
		if o := rng.IntN(total); !seen[o] {
			seen[o] = true
			out = append(out, o)
		}
	}
	slices.Sort(out)
	return out
}
