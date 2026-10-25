package align

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/ingest"
)

// The channels of a score. A Candidate has the score of each channel that it has data for.
const (
	ChannelID        = "id"        // the item and the record name the same thing by an id in a third system
	ChannelName      = "name"      // the names are the same or have the same rare words
	ChannelFormula   = "formula"   // the texts have the same formulas
	ChannelEmbedding = "embedding" // the vectors of the texts are near
)

// An Item is the thing that the aligner looks for in a source: a node, or in an evaluation a record of another source.
type Item struct {
	Ref    string            // the tag of the node, or another id of the item
	Names  []string          // the name and the other names of the item
	Text   string            // the statement or the summary, in Markdown with LaTeX
	SameAs map[string]string // the id of the item in each system where it has one
	Decls  []string          // the Mathlib declarations that formalize the item
}

// A Candidate is a record that can be the item, with its score from 0 to 1.
type Candidate struct {
	Record   ingest.Record
	Score    float64
	Channels map[string]float64
}

// systemOf maps a source whose id is not the name of its system in same_as.
var systemOf = map[string]string{"erdosproblems": "erdos"}

// papers are the sources whose records are papers. A paper states a node, and it is not the same thing as the node, thus the edge is aligned.
var papers = map[string]bool{"arxiv": true, "zbmath": true}

// identity gives the meta keys whose values are the ids of the same thing in another system. For Wikidata, each key that names a system is such a key, because the properties that the adapter reads are exact matches, and so is the key mathlib, which names the declaration of the item.
var identity = map[string][]string{"formal_conjectures": {"erdos"}}

// System returns the system of the same_as and close_to edges to the records of the source, and reports whether the aligner reads the source. The aligner does not read mathlib, because a node meets a Mathlib declaration through formalizes and not through same_as.
func System(source string) (string, bool) {
	if s, ok := systemOf[source]; ok {
		return s, true
	}
	if papers[source] {
		return source, true
	}
	return source, crosswalk.Known(source)
}

// SourceOf returns the ingest source whose records are the ids of the system: erdosproblems for erdos, and the system itself for the others.
func SourceOf(system string) string {
	for s, sys := range systemOf {
		if sys == system {
			return s
		}
	}
	return system
}

// Edge returns the edge that a match with a record of the source gives: aligned for a source of papers and same_as for the others.
func Edge(source string) string {
	if papers[source] {
		return Aligned
	}
	return SameAs
}

// mathlib is the system of the Mathlib declarations in a pair. A node meets Mathlib through formalizes, thus it is not a system of same_as.
const mathlib = "mathlib"

// pair is a system and an id in it.
type pair struct{ system, id string }

// An Index finds the candidates for an item in the records of one source. Its methods are safe for concurrent use after Bridge.
type Index struct {
	source, system string
	embed          Embedder
	recs           []ingest.Record
	keys           [][]string       // the name keys of each record
	exact          map[string][]int // the records of each name key
	post           map[string][]int // the records whose names have the word
	maths          [][]string       // the formulas of each record
	byMath         map[string][]int
	ids            map[pair][]int // the records that are each pair
	bridge         map[pair][]pair
}

// NewIndex returns the index of the records of the source. A nil Embedder means Hash.
func NewIndex(source string, recs []ingest.Record, e Embedder) (*Index, error) {
	system, ok := System(source)
	if !ok {
		return nil, fmt.Errorf("align: %s is not a source that the aligner reads", source)
	}
	if len(recs) == 0 {
		return nil, errors.New("align: the source has no records")
	}
	if e == nil {
		e = Hash{}
	}
	x := &Index{
		source: source, system: system, embed: e, recs: recs,
		keys: make([][]string, len(recs)), exact: map[string][]int{}, post: map[string][]int{},
		maths: make([][]string, len(recs)), byMath: map[string][]int{}, ids: map[pair][]int{}, bridge: map[pair][]pair{},
	}
	for i, r := range recs {
		x.keys[i] = names(recordNames(r)...)
		seen := map[string]bool{}
		for _, k := range x.keys[i] {
			x.exact[k] = append(x.exact[k], i)
			for _, w := range strings.Fields(k) {
				if !seen[w] {
					seen[w] = true
					x.post[w] = append(x.post[w], i)
				}
			}
		}
		x.maths[i] = formulas(r.Text)
		for _, f := range x.maths[i] {
			x.byMath[f] = append(x.byMath[f], i)
		}
		for _, p := range x.identities(r) {
			x.ids[p] = append(x.ids[p], i)
		}
	}
	return x, nil
}

// recordNames returns the names of a record: its title and the name and title in its meta.
func recordNames(r ingest.Record) []string {
	return []string{r.Title, r.Meta["name"], r.Meta["title"]}
}

// identities returns the pairs that the record is: its own id, and the ids in its meta that name the same thing.
func (x *Index) identities(r ingest.Record) []pair {
	return idsOf(x.source, x.system, r)
}

func idsOf(source, system string, r ingest.Record) []pair {
	out := []pair{{system, r.ID}}
	keys := identity[source]
	// The Wikidata adapter puts | between two ids, because a page title has spaces. The other adapters put a space.
	sep := func(c rune) bool { return c == ' ' || c == '|' }
	if source == "wikidata" {
		keys = nil
		for k := range r.Meta {
			if crosswalk.Known(k) || k == mathlib {
				keys = append(keys, k)
			}
		}
		slices.Sort(keys)
		sep = func(c rune) bool { return c == '|' }
	}
	for _, k := range keys {
		for v := range strings.FieldsFuncSeq(r.Meta[k], sep) {
			out = append(out, pair{k, v})
		}
	}
	return out
}

// Bridge adds the records of Wikidata to the index as links between systems. An item with a QID in same_as then matches the record that the Wikidata item names with an exact match, and an item with an id in a third system matches through the Wikidata item that has that id too.
func (x *Index) Bridge(wikidata []ingest.Record) {
	for _, w := range wikidata {
		ps := idsOf("wikidata", "wikidata", w)
		var to []pair
		for _, p := range ps {
			if p.system == x.system {
				to = append(to, p)
			}
		}
		if len(to) == 0 {
			continue
		}
		for _, p := range ps {
			if p.system != x.system {
				x.bridge[p] = append(x.bridge[p], to...)
			}
		}
	}
}

// pool is the number of records that the words of the names give to the scoring.
const pool = 200

// Match returns the k candidates with the highest scores, best first. Zero k means 5.
//
// The score of a record is 1 when the item and the record are the same thing by an id or by a Mathlib declaration, and only that record is. Otherwise it is a weighted sum of the channels: 0.55 name, 0.25 embedding and 0.2 formula when both texts have formulas, and 0.7 name and 0.3 embedding when they do not.
func (x *Index) Match(it Item, k int) []Candidate {
	if k <= 0 {
		k = 5
	}
	var ps []pair
	for s, v := range it.SameAs {
		ps = append(ps, pair{s, v})
	}
	for _, d := range it.Decls {
		ps = append(ps, pair{mathlib, d})
	}
	hits := map[int]bool{}
	for _, p := range ps {
		for _, i := range x.ids[p] {
			hits[i] = true
		}
		for _, q := range x.bridge[p] {
			for _, i := range x.ids[q] {
				hits[i] = true
			}
		}
	}

	keys := names(it.Names...)
	maths := formulas(it.Text)
	cand := map[int]bool{}
	for i := range hits {
		cand[i] = true
	}
	acc := map[int]float64{}
	for _, key := range keys {
		for _, i := range x.exact[key] {
			cand[i] = true
		}
		for _, w := range strings.Fields(key) {
			if ps := x.post[w]; len(ps) <= max(50, len(x.recs)/20) {
				for _, i := range ps {
					acc[i] += x.idf(w)
				}
			}
		}
	}
	for _, i := range topN(acc, pool) {
		cand[i] = true
	}
	for _, f := range maths {
		for _, i := range x.byMath[f] {
			cand[i] = true
		}
	}
	if len(cand) == 0 {
		return nil
	}

	itVec := x.embed.Embed(strings.Join(it.Names, ". ") + ". " + it.Text)
	nameVec := x.embed.Embed(strings.Join(it.Names, ". "))
	out := make([]Candidate, 0, len(cand))
	for i := range cand {
		r := x.recs[i]
		ch := map[string]float64{}
		if hits[i] {
			ch[ChannelID] = 1
		}
		ch[ChannelName] = x.nameScore(keys, x.keys[i])
		if r.Text != "" {
			ch[ChannelEmbedding] = max(0, cosine(itVec, x.embed.Embed(strings.Join(recordNames(r), ". ")+". "+r.Text)))
		} else {
			ch[ChannelEmbedding] = max(0, cosine(nameVec, x.embed.Embed(strings.Join(recordNames(r), ". "))))
		}
		score := 0.7*ch[ChannelName] + 0.3*ch[ChannelEmbedding]
		if len(maths) > 0 && len(x.maths[i]) > 0 {
			ch[ChannelFormula] = jaccard(maths, x.maths[i])
			score = 0.55*ch[ChannelName] + 0.25*ch[ChannelEmbedding] + 0.2*ch[ChannelFormula]
		}
		if hits[i] && len(hits) == 1 {
			score = 1
		}
		for c, v := range ch {
			ch[c] = round(v)
		}
		out = append(out, Candidate{Record: r, Score: round(score), Channels: ch})
	}
	slices.SortFunc(out, func(a, b Candidate) int {
		return cmp.Or(cmp.Compare(b.Score, a.Score), cmp.Compare(a.Record.ID, b.Record.ID))
	})
	return out[:min(k, len(out))]
}

// idf returns the weight of a word: high for a word that few names have.
func (x *Index) idf(w string) float64 {
	return math.Log(1 + float64(len(x.recs))/float64(max(1, len(x.post[w]))))
}

// nameScore returns 1 when the item and the record have a name in common, and else the highest share of the weight of the words that two names have in common.
func (x *Index) nameScore(a, b []string) float64 {
	best := 0.0
	for _, p := range a {
		for _, q := range b {
			if p == q {
				return 1
			}
			wp, wq := strings.Fields(p), strings.Fields(q)
			var both, all float64
			for _, w := range wp {
				all += x.idf(w)
				if slices.Contains(wq, w) {
					both += x.idf(w)
				}
			}
			for _, w := range wq {
				if !slices.Contains(wp, w) {
					all += x.idf(w)
				}
			}
			if all > 0 {
				best = max(best, both/all)
			}
		}
	}
	return best
}

// jaccard returns the share of the elements of a and b that are in both.
func jaccard(a, b []string) float64 {
	both := 0
	for _, s := range a {
		if slices.Contains(b, s) {
			both++
		}
	}
	return float64(both) / float64(len(a)+len(b)-both)
}

// topN returns the n keys with the highest values, in an order that does not change between runs.
func topN(acc map[int]float64, n int) []int {
	keys := make([]int, 0, len(acc))
	for i := range acc {
		keys = append(keys, i)
	}
	slices.SortFunc(keys, func(a, b int) int { return cmp.Or(cmp.Compare(acc[b], acc[a]), cmp.Compare(a, b)) })
	return keys[:min(n, len(keys))]
}

// round rounds a score to three places, thus the queue files do not change from one machine to the next.
func round(v float64) float64 { return math.Round(v*1000) / 1000 }
