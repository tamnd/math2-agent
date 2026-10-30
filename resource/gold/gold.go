// Package gold holds the gold sets of the clustering (RS10.7). A gold set is a list of pairs of source records. Two people, or one person and the judge, label each pair as one work or two works, and a third label resolves a disagreement. The package samples the pairs, reads and writes the gold files, and measures the precision and the recall of the merges of each pass on the labelled pairs.
package gold

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource/cluster"
)

// Dir holds the gold sets in the tree.
const Dir = "records/resources/gold"

// The gold sets of pairs. G-edition needs the works of a run, and it comes in a later change.
const (
	Work     = "work"
	Preprint = "preprint"
)

// Files gives the file of each gold set of pairs.
var Files = map[string]string{
	Work:     Dir + "/work-pairs.md",
	Preprint: Dir + "/preprint-pairs.md",
}

// The values of a label.
const (
	Same      = "same"
	Different = "different"
)

// The targets of RS10.7 for the merges.
const (
	TargetPrecision = 0.995
	TargetRecall    = 0.97
)

// Label is the label of one person or of the judge.
type Label struct {
	Value string `json:"value"`
	By    string `json:"by"`
	At    string `json:"at"`
	Note  string `json:"note,omitempty"`
}

// Pair is one pair of a gold set. A and B are the keys of the two records (source:native_id). Stratum names the kind, the decade, the language and the case that the sample drew the pair from.
type Pair struct {
	A       string  `json:"a"`
	B       string  `json:"b"`
	Stratum string  `json:"stratum"`
	Labels  []Label `json:"labels,omitempty"`
}

// Value gives the label of the pair: the value of the first two labels when they agree, else the value of the third label. It gives "" while the pair is open.
func (p *Pair) Value() string {
	switch {
	case len(p.Labels) >= 2 && p.Labels[0].Value == p.Labels[1].Value:
		return p.Labels[0].Value
	case len(p.Labels) >= 3:
		return p.Labels[2].Value
	}
	return ""
}

// Key names the pair in the order of the two keys.
func (p *Pair) Key() string { return min(p.A, p.B) + " " + max(p.A, p.B) }

// Read reads the gold set in the tree at root. A tree with no file has an empty set.
func Read(root, set string) ([]Pair, error) {
	file, ok := Files[set]
	if !ok {
		return nil, fmt.Errorf("gold: the set %q is not one of work and preprint", set)
	}
	recs, err := mdrec.Read(filepath.Join(root, file))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ps, err := mdrec.Unmarshal[Pair](recs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	seen := map[string]bool{}
	for n, p := range ps {
		if p.A == "" || p.B == "" || p.A == p.B {
			return nil, fmt.Errorf("%s: record %d: a pair needs two keys", file, n+1)
		}
		if seen[p.Key()] {
			return nil, fmt.Errorf("%s: record %d: the pair %s is in the set twice", file, n+1, p.Key())
		}
		seen[p.Key()] = true
		for _, l := range p.Labels {
			if (l.Value != Same && l.Value != Different) || l.By == "" {
				return nil, fmt.Errorf("%s: record %d: a label needs the value same or different and a person", file, n+1)
			}
		}
	}
	return ps, nil
}

// Write writes the gold set to the tree at root.
func Write(root, set string, ps []Pair) error {
	file, ok := Files[set]
	if !ok {
		return fmt.Errorf("gold: the set %q is not one of work and preprint", set)
	}
	recs, err := mdrec.Marshal(ps)
	if err != nil {
		return err
	}
	path := filepath.Join(root, file)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return mdrec.Write(path, recs)
}

// volumeRE matches a title of one volume of a multi-volume work.
var volumeRE = regexp.MustCompile(`(?i)\b(vol\.?|volume|band|bd\.|tome|teil|part|tom)\s*[0-9ivxl]+\b`)

// The cases of a work pair. The first three are the hard cases of RS10.7, and the sample takes half of its pairs from them.
const (
	CaseTitle       = "title"       // one normal title and two first creators
	CaseTranslation = "translation" // a translation and its work, or one creator and two languages
	CaseVolume      = "volume"      // two volumes of a multi-volume work
	CaseBlock       = "block"       // one family name and one year
	CaseRandom      = "random"      // two records of one kind
)

// candidate is a pair that the sample can take.
type candidate struct {
	i, j    int
	stratum string
	rank    string
}

// Sample takes n new pairs of the set from the records. It leaves out each pair that have holds. The seed makes the sample the same on each run. A work sample takes half of its pairs from the hard cases and spreads the pairs over the kinds, the decades and the languages. A preprint sample pairs each arXiv preprint with the records of papers that can be its publication.
func Sample(recs []cluster.Record, set string, n int, seed string, have []Pair) ([]Pair, error) {
	seen := map[string]bool{}
	for _, p := range have {
		seen[p.Key()] = true
	}
	var groups map[string][]candidate
	var quota map[string]int
	var order []string // the hard cases first, so that an easy case does not take a hard pair
	switch set {
	case Work:
		groups = workCandidates(recs, seed)
		hard := n / 2
		quota = map[string]int{CaseTitle: hard / 3, CaseTranslation: hard / 3, CaseVolume: hard - 2*(hard/3), CaseBlock: (n - hard) / 2}
		quota[CaseRandom] = n - hard - quota[CaseBlock]
		order = []string{CaseTitle, CaseTranslation, CaseVolume, CaseBlock, CaseRandom}
	case Preprint:
		groups = preprintCandidates(recs, seed)
		quota = map[string]int{"id": n / 2}
		quota["title"] = n - quota["id"]
		order = []string{"id", "title"}
	default:
		return nil, fmt.Errorf("gold: the set %q is not one of work and preprint", set)
	}
	var out []Pair
	short := 0
	for _, c := range order {
		got := spread(groups[c], quota[c]+short, recs, seen)
		short = quota[c] + short - len(got)
		out = append(out, got...)
	}
	slices.SortFunc(out, func(a, b Pair) int { return cmp.Compare(a.Key(), b.Key()) })
	return out, nil
}

// spread takes up to n pairs of the candidates, one stratum after the other, so that each stratum gets a share.
func spread(cs []candidate, n int, recs []cluster.Record, seen map[string]bool) []Pair {
	byStratum := map[string][]candidate{}
	for _, c := range cs {
		byStratum[c.stratum] = append(byStratum[c.stratum], c)
	}
	strata := slices.Sorted(maps.Keys(byStratum))
	for _, s := range strata {
		slices.SortFunc(byStratum[s], func(a, b candidate) int { return cmp.Compare(a.rank, b.rank) })
	}
	var out []Pair
	for len(out) < n {
		took := false
		for _, s := range strata {
			for len(byStratum[s]) > 0 && len(out) < n {
				c := byStratum[s][0]
				byStratum[s] = byStratum[s][1:]
				p := Pair{A: recs[c.i].Key(), B: recs[c.j].Key(), Stratum: c.stratum}
				if seen[p.Key()] {
					continue
				}
				seen[p.Key()] = true
				out = append(out, p)
				took = true
				break
			}
		}
		if !took {
			break
		}
	}
	return out
}

func rank(seed, a, b string) string {
	h := sha256.Sum256([]byte(seed + "\x00" + min(a, b) + "\x00" + max(a, b)))
	return hex.EncodeToString(h[:8])
}

func decade(r *cluster.Record) string {
	if y := r.Year(); y > 0 {
		return strconv.Itoa(y/10*10) + "s"
	}
	return "undated"
}

func first(r *cluster.Record) string {
	for _, c := range r.Creators {
		if f := cluster.Family(c.Name); f != "" {
			return f
		}
	}
	return ""
}

// maxGroup is the most records of one key that the sample pairs with each other. A bigger group gives too many pairs of one kind.
const maxGroup = 50

func workCandidates(recs []cluster.Record, seed string) map[string][]candidate {
	out := map[string][]candidate{}
	add := func(c string, i, j int) {
		a, b := &recs[i], &recs[j]
		lang := cmp.Or(a.Language, "und")
		out[c] = append(out[c], candidate{i, j, a.Kind + " " + decade(a) + " " + lang + " " + c, rank(seed, a.Key(), b.Key())})
	}
	pairs := func(c string, by map[string][]int, ok func(a, b *cluster.Record) bool) {
		for _, k := range slices.Sorted(maps.Keys(by)) {
			idx := by[k]
			if len(idx) > maxGroup {
				idx = idx[:maxGroup]
			}
			for x := range idx {
				for y := x + 1; y < len(idx); y++ {
					if ok(&recs[idx[x]], &recs[idx[y]]) {
						add(c, idx[x], idx[y])
					}
				}
			}
		}
	}
	byTitle, byCreator, byBlock, byKind, byID := map[string][]int{}, map[string][]int{}, map[string][]int{}, map[string][]int{}, map[string][]int{}
	for i := range recs {
		r := &recs[i]
		f := first(r)
		if t := cluster.WorkTitle(r.Title); len([]rune(t)) >= 12 {
			byTitle[t] = append(byTitle[t], i)
		}
		if f != "" {
			byCreator[f] = append(byCreator[f], i)
			if r.Year() > 0 {
				byBlock[f+" "+strconv.Itoa(r.Year())] = append(byBlock[f+" "+strconv.Itoa(r.Year())], i)
			}
		}
		byKind[r.Kind] = append(byKind[r.Kind], i)
		for sys, vs := range r.IDs {
			for _, v := range vs {
				byID[sys+":"+v] = append(byID[sys+":"+v], i)
			}
		}
	}
	pairs(CaseTitle, byTitle, func(a, b *cluster.Record) bool { return first(a) != first(b) })
	pairs(CaseTranslation, byCreator, func(a, b *cluster.Record) bool {
		return a.Language != "" && b.Language != "" && a.Language != b.Language
	})
	for i := range recs {
		for _, rel := range recs[i].Relations {
			if rel.Kind == "translation_of" {
				for _, j := range byID[rel.To] {
					add(CaseTranslation, min(i, j), max(i, j))
				}
			}
		}
	}
	pairs(CaseVolume, byCreator, func(a, b *cluster.Record) bool {
		return volumeRE.MatchString(a.Title) && volumeRE.MatchString(b.Title) && cluster.Similarity(a.Title, b.Title) >= 0.5
	})
	pairs(CaseBlock, byBlock, func(_, _ *cluster.Record) bool { return true })
	// The random pairs: each record with the next record of its kind in the order of the rank.
	for _, k := range slices.Sorted(maps.Keys(byKind)) {
		idx := slices.Clone(byKind[k])
		slices.SortFunc(idx, func(x, y int) int { return cmp.Compare(rank(seed, recs[x].Key(), ""), rank(seed, recs[y].Key(), "")) })
		for x := 0; x+1 < len(idx); x += 2 {
			add(CaseRandom, min(idx[x], idx[x+1]), max(idx[x], idx[x+1]))
		}
	}
	return out
}

// papers are the kinds that can be the publication of a preprint.
var papers = map[string]bool{"article": true, "conference-paper": true, "survey": true, "chapter": true}

func preprintCandidates(recs []cluster.Record, seed string) map[string][]candidate {
	out := map[string][]candidate{}
	byID, byCreator := map[string][]int{}, map[string][]int{}
	for i := range recs {
		r := &recs[i]
		if r.Source == "arxiv" || !papers[r.Kind] {
			continue
		}
		for _, v := range r.IDs["arxiv"] {
			byID[v] = append(byID[v], i)
		}
		if f := first(r); f != "" {
			byCreator[f] = append(byCreator[f], i)
		}
	}
	for i := range recs {
		a := &recs[i]
		if a.Source != "arxiv" {
			continue
		}
		stratum := func(c string) string { return decade(a) + " " + c }
		took := map[int]bool{}
		for _, v := range a.IDs["arxiv"] {
			for _, j := range byID[v] {
				took[j] = true
				out["id"] = append(out["id"], candidate{min(i, j), max(i, j), stratum("id"), rank(seed, a.Key(), recs[j].Key())})
			}
		}
		for _, j := range byCreator[first(a)] {
			b := &recs[j]
			if took[j] || (a.Year() > 0 && b.Year() > 0 && (b.Year() < a.Year() || b.Year()-a.Year() > cluster.Window)) {
				continue
			}
			if cluster.Similarity(a.Title, b.Title) >= 0.5 {
				out["title"] = append(out["title"], candidate{min(i, j), max(i, j), stratum("title"), rank(seed, a.Key(), b.Key())})
			}
		}
	}
	return out
}

// Score is the precision and the recall of the merges of one pass, or of all passes, on the labelled pairs.
type Score struct {
	Pass      string
	Merged    int // the labelled pairs that the pass merged
	Wrong     int // the merged pairs with the label different
	Missed    int // the pairs with the label same that no pass merged. Only the row of all passes has it
	Precision float64
	Recall    float64
}

// Eval gives the score of each pass and of all passes on the labelled pairs of a gold set. A pair that is open or whose records are not in the run does not count. The last score is the score of all passes, with the pass "all". Met tells if the score of all passes meets the targets of RS10.7.
func Eval(res *cluster.Result, ps []Pair) (scores []Score, met bool, counted int) {
	in := map[string]bool{}
	for _, r := range res.Resources {
		for _, c := range r.Catalogue {
			in[c.Source+":"+c.NativeID] = true
		}
	}
	var keys [][2]string
	var values []string
	for _, p := range ps {
		v := p.Value()
		if v == "" || !in[p.A] || !in[p.B] {
			continue
		}
		keys = append(keys, [2]string{p.A, p.B})
		values = append(values, v)
	}
	passes := res.JoinedBy(keys)
	by := map[string]*Score{}
	all := &Score{Pass: "all"}
	same := 0
	for i, pass := range passes {
		if values[i] == Same {
			same++
		}
		if pass == "" {
			if values[i] == Same {
				all.Missed++
			}
			continue
		}
		s := by[pass]
		if s == nil {
			s = &Score{Pass: pass}
			by[pass] = s
		}
		for _, x := range []*Score{s, all} {
			x.Merged++
			if values[i] == Different {
				x.Wrong++
			}
		}
	}
	for _, pass := range slices.Sorted(maps.Keys(by)) {
		s := by[pass]
		s.Precision = ratio(s.Merged-s.Wrong, s.Merged)
		s.Recall = ratio(s.Merged-s.Wrong, same)
		scores = append(scores, *s)
	}
	all.Precision = ratio(all.Merged-all.Wrong, all.Merged)
	all.Recall = ratio(all.Merged-all.Wrong, same)
	scores = append(scores, *all)
	met = all.Merged > 0 && same > 0 && all.Precision >= TargetPrecision && all.Recall >= TargetRecall
	return scores, met, len(keys)
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(int(float64(a)/float64(b)*10000+0.5)) / 10000
}

// Labelled gives the pairs with a final label, the open pairs, and the pairs whose first two labels disagree and that wait for a third label.
func Labelled(ps []Pair) (done, open, disputed int) {
	for _, p := range ps {
		switch {
		case p.Value() != "":
			done++
		case len(p.Labels) == 2:
			disputed++
		default:
			open++
		}
	}
	return
}

// AddLabel adds a label to the pair with the keys a and b. A person labels a pair once.
func AddLabel(ps []Pair, a, b string, l Label) error {
	if l.Value != Same && l.Value != Different {
		return fmt.Errorf("gold: the value %q is not same or different", l.Value)
	}
	key := (&Pair{A: a, B: b}).Key()
	for i := range ps {
		if ps[i].Key() != key {
			continue
		}
		if slices.ContainsFunc(ps[i].Labels, func(x Label) bool { return x.By == l.By }) {
			return fmt.Errorf("gold: %s already labelled the pair %s", l.By, key)
		}
		if ps[i].Value() != "" {
			return fmt.Errorf("gold: the pair %s has its label", key)
		}
		ps[i].Labels = append(ps[i].Labels, l)
		return nil
	}
	return fmt.Errorf("gold: the pair %s is not in the set", key)
}
