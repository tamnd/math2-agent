// Package classify is the classifier of RS9.3. It decides the scope of a work that the tests S1 to S7 cannot decide, and it gives the MSC2 class of a work in scope. The model is a multinomial naive Bayes model over the words of the title, the venue, the subject headings and the classes of the works that the work cites. It is fitted on records that a test decided, so it needs no labels from people.
//
// A model is a JSON file. Its version is the hash of the file, so a run names the exact model that it used. The model gives a score, never a fact: its output is a classification with a source, not an MSC code of a catalogue.
package classify

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
)

// ModelFile is the place of the scope model in the tree. It is rebuilt and is not in git, and the version in each decision names it.
const ModelFile = "build/resources/models/scope.json"

// GoldFile holds the labelled sample of RS9.3 that sets the thresholds.
const GoldFile = "records/resources/gold/scope.jsonl"

// The targets of RS9.3 for the threshold of in.
const (
	TargetPrecision = 0.98
	TargetRecall    = 0.95
)

// Example is one labelled record.
type Example struct {
	ID       string   `json:"id,omitempty"`
	Features []string `json:"features"`
	Label    string   `json:"label"` // in, border or out, or an MSC2 class for the class model
}

// stop holds the words that carry no topic.
var stop = map[string]bool{
	"a": true, "an": true, "the": true, "of": true, "and": true, "in": true, "on": true, "for": true, "to": true, "with": true, "by": true, "from": true, "at": true, "as": true, "is": true, "its": true, "or": true,
	"der": true, "die": true, "das": true, "und": true, "von": true, "zur": true, "zum": true, "uber": true, "ein": true, "eine": true, "des": true, "den": true, "dem": true,
	"le": true, "la": true, "les": true, "de": true, "du": true, "et": true, "sur": true, "une": true, "un": true, "pour": true, "dans": true,
}

// Features gives the features of a resource: the words and the pairs of words of the title, the venues, the subjects and the MSC2 classes of the cited works. The caller gives the cited classes, because the cites edges are not in the resource.
func Features(r *resource.Resource, cited []string) []string {
	var out []string
	words := Words(r.Title + " " + r.TitleEn)
	for i, w := range words {
		out = append(out, "w:"+w)
		if i > 0 {
			out = append(out, "b:"+words[i-1]+"_"+w)
		}
	}
	for _, rel := range r.Relations {
		if rel.Kind == "part_of" {
			id, _, _ := strings.Cut(rel.To, "/")
			out = append(out, "v:"+id)
		}
	}
	for _, s := range r.Subjects {
		out = append(out, "s:"+strings.ToLower(s.System)+":"+s.Code)
		if strings.EqualFold(s.System, "arxiv") {
			top, _, _ := strings.Cut(s.Code, ".")
			out = append(out, "s:arxiv:"+top)
		}
	}
	for _, c := range cited {
		if len(c) >= 2 {
			out = append(out, "c:"+c[:2])
		}
	}
	out = append(out, "k:"+r.Kind)
	return out
}

// Words gives the folded words of a text without the stop words and the words of one letter.
func Words(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(cluster.Fold(s), func(r rune) bool { return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r <= 127 }) {
		if len([]rune(w)) > 1 && !stop[w] {
			out = append(out, w)
		}
	}
	return out
}

// Model is a fitted naive Bayes model.
type Model struct {
	Labels []string                  `json:"labels"`
	Docs   map[string]int            `json:"docs"`   // the examples of each label
	Totals map[string]int            `json:"totals"` // the features of each label
	Counts map[string]map[string]int `json:"counts"` // the count of each feature for each label
	Vocab  int                       `json:"vocab"`
	High   float64                   `json:"high,omitempty"` // a score of in at or above it gives in
	Low    float64                   `json:"low,omitempty"`  // a score of in below it gives the best other label
	// Classes is the model of the MSC2 class of a work in scope, fitted on the works in scope that have an MSC code.
	Classes *Model `json:"classes,omitempty"`
	version string
}

// MinCount is the least count of a feature in the training set. A rarer feature is left out, so the model stays small.
const MinCount = 2

// Fit fits a model on the examples.
func Fit(ex []Example) *Model {
	freq := map[string]int{}
	for _, e := range ex {
		for _, f := range e.Features {
			freq[f]++
		}
	}
	m := &Model{Docs: map[string]int{}, Totals: map[string]int{}, Counts: map[string]map[string]int{}}
	for _, e := range ex {
		if m.Counts[e.Label] == nil {
			m.Counts[e.Label] = map[string]int{}
			m.Labels = append(m.Labels, e.Label)
		}
		m.Docs[e.Label]++
		for _, f := range e.Features {
			if freq[f] >= MinCount {
				m.Counts[e.Label][f]++
				m.Totals[e.Label]++
			}
		}
	}
	for _, n := range freq {
		if n >= MinCount {
			m.Vocab++
		}
	}
	slices.Sort(m.Labels)
	return m
}

// Probs gives the probability of each label for the features, in the order of the labels.
func (m *Model) Probs(features []string) []float64 {
	n := 0
	for _, l := range m.Labels {
		n += m.Docs[l]
	}
	logs := make([]float64, len(m.Labels))
	for i, l := range m.Labels {
		lp := math.Log(float64(m.Docs[l]) / float64(n))
		den := math.Log(float64(m.Totals[l] + m.Vocab + 1))
		for _, f := range features {
			c, known := 0, false
			for _, other := range m.Labels {
				if m.Counts[other][f] > 0 {
					known = true
					break
				}
			}
			if !known {
				continue
			}
			c = m.Counts[l][f]
			lp += math.Log(float64(c+1)) - den
		}
		logs[i] = lp
	}
	top := slices.Max(logs)
	sum := 0.0
	for i := range logs {
		logs[i] = math.Exp(logs[i] - top)
		sum += logs[i]
	}
	for i := range logs {
		logs[i] /= sum
	}
	return logs
}

// Score gives the probability of the label.
func (m *Model) Score(features []string, label string) float64 {
	i := slices.Index(m.Labels, label)
	if i < 0 {
		return 0
	}
	return m.Probs(features)[i]
}

// Decide gives the outcome of RS9.3 for the features: in when the score of in is at or above High, the best other label when the score of in is below Low, and an empty outcome in the middle band, for the queue res-scope.
func (m *Model) Decide(features []string) (string, float64) {
	p := m.Probs(features)
	in := 0.0
	best, bestP := "", -1.0
	for i, l := range m.Labels {
		if l == resource.In {
			in = p[i]
			continue
		}
		if p[i] > bestP {
			best, bestP = l, p[i]
		}
	}
	switch {
	case in >= m.High:
		return resource.In, round(in)
	case in < m.Low:
		return best, round(in)
	}
	return "", round(in)
}

// Class gives the MSC2 class of a work in scope and its score, or an empty class when the model has no class model.
func (m *Model) Class(features []string) (string, float64) {
	if m.Classes == nil || len(m.Classes.Labels) == 0 {
		return "", 0
	}
	p := m.Classes.Probs(features)
	i := 0
	for j := range p {
		if p[j] > p[i] {
			i = j
		}
	}
	return m.Classes.Labels[i], round(p[i])
}

func round(f float64) float64 { return math.Round(f*1000) / 1000 }

// Eval is the outcome of the thresholds on a labelled sample.
type Eval struct {
	High, Low float64
	Precision float64 // of in, on the records above High
	Recall    float64 // of in, on the records of the label in
	Middle    int     // the records in the middle band
	Total     int
	Met       bool // the precision and the recall meet the targets of RS9.3
}

// Tune sets High to the lowest score that gives a precision of in of at least TargetPrecision on the sample, and Low to the highest score below which the share of in is at most 1 - TargetPrecision. It gives the precision and the recall at these thresholds.
func (m *Model) Tune(gold []Example) Eval {
	type scored struct {
		s  float64
		in bool
	}
	var xs []scored
	pos := 0
	for _, e := range gold {
		x := scored{m.Score(e.Features, resource.In), e.Label == resource.In}
		if x.in {
			pos++
		}
		xs = append(xs, x)
	}
	ev := Eval{Total: len(xs), High: 1, Low: 0}
	if len(xs) == 0 || pos == 0 {
		m.High, m.Low = ev.High, ev.Low
		return ev
	}
	slices.SortFunc(xs, func(a, b scored) int { return cmp.Compare(b.s, a.s) })
	// High: walk down the scores and keep the lowest score that keeps the precision.
	tp, n := 0, 0
	for i, x := range xs {
		n++
		if x.in {
			tp++
		}
		if i+1 < len(xs) && xs[i+1].s == x.s {
			continue
		}
		if float64(tp)/float64(n) >= TargetPrecision {
			ev.High = x.s
		}
	}
	// Low: walk up the scores and keep the highest score below which few records are in.
	fn, n := 0, 0
	for i := len(xs) - 1; i >= 0; i-- {
		x := xs[i]
		if x.s >= ev.High {
			break
		}
		n++
		if x.in {
			fn++
		}
		if i > 0 && xs[i-1].s == x.s {
			continue
		}
		if float64(fn)/float64(n) <= 1-TargetPrecision {
			ev.Low = x.s + 1e-9
		}
	}
	m.High, m.Low = ev.High, ev.Low
	tp, above := 0, 0
	for _, x := range xs {
		switch {
		case x.s >= ev.High:
			above++
			if x.in {
				tp++
			}
		case x.s >= ev.Low:
			ev.Middle++
		}
	}
	if above > 0 {
		ev.Precision = round(float64(tp) / float64(above))
	}
	ev.Recall = round(float64(tp) / float64(pos))
	ev.High, ev.Low = round(ev.High), round(ev.Low)
	ev.Met = ev.Precision >= TargetPrecision && ev.Recall >= TargetRecall
	return ev
}

// Version gives the version of the model: classify/1 and the first twelve hex digits of the hash of its file.
func (m *Model) Version() string { return m.version }

// Write writes the model to path and sets its version.
func (m *Model) Write(path string) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil { // #nosec G306 -- the model is public.
		return err
	}
	m.version = version(data)
	return nil
}

// Read reads a model from path. A missing file gives a nil model and no error.
func Read(path string) (*Model, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the caller owns the tree.
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m Model
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("classify: %s: %w", path, err)
	}
	m.version = version(data)
	return &m, nil
}

func version(data []byte) string {
	h := sha256.Sum256(data)
	return "classify/1@" + hex.EncodeToString(h[:6])
}
