package wpcorpus

import (
	"bufio"
	"fmt"
	"io"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
)

// The strata of gold set S (WP11.2).
const (
	StratumStrong = "strong"   // S1 or S2
	StratumCited  = "cited"    // S4 with no project tag
	StratumWeak   = "one weak" // one signal of S3 and S5 to S9
)

// GoldSizes is the size of each stratum in the first version of gold set S.
var GoldSizes = map[string]int{StratumStrong: 500, StratumCited: 500, StratumWeak: 1000}

// Stratum gives the stratum of gold set S for a candidate, or "" when the
// candidate is in no stratum.
func Stratum(c *Candidate) string {
	switch {
	case c.Signals&(S1|S2) != 0:
		return StratumStrong
	case c.Signals&S4 != 0 && c.Signals&S3 == 0:
		return StratumCited
	case c.Signals&weak != 0 && c.Signals&(c.Signals-1) == 0:
		return StratumWeak
	}
	return ""
}

// GoldRow is one row of manifests/wikipedia/gold/scope.tsv.
type GoldRow struct {
	PageID  int64
	Title   string
	Stratum string
	Signals Signals
	Rule    string
	// Population is the number of candidates in the stratum when the
	// sample was drawn. Measure uses it to weight the stratum.
	Population int
	// Mark is math, uses-math or not-math. It is empty until a reviewer
	// marks the row.
	Mark   string
	Marker string
	Note   string
}

// DrawGold draws a sample of each stratum with the seed. A stratum with
// fewer candidates than its size gives all of them.
func DrawGold(cs []*Candidate, sizes map[string]int, seed uint64) []GoldRow {
	by := map[string][]*Candidate{}
	for _, c := range cs {
		if s := Stratum(c); s != "" {
			by[s] = append(by[s], c)
		}
	}
	var out []GoldRow
	for _, s := range sortedKeys(sizes) {
		pool := slices.Clone(by[s])
		slices.SortFunc(pool, func(a, b *Candidate) int { return cmpID(a.PageID, b.PageID) })
		r := rand.New(rand.NewPCG(seed, uint64(len(s)))) // #nosec G404 -- a seeded sample must repeat, it is not a secret
		r.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
		if len(pool) > sizes[s] {
			pool = pool[:sizes[s]]
		}
		for _, c := range pool {
			out = append(out, GoldRow{PageID: c.PageID, Title: c.Title, Stratum: s, Signals: c.Signals, Rule: c.Rule, Population: len(by[s])})
		}
	}
	slices.SortFunc(out, func(a, b GoldRow) int { return cmpID(a.PageID, b.PageID) })
	return out
}

func cmpID(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

const goldHeader = "page_id\ttitle\tstratum\tsignals\trule\tpopulation\tmark\tmarker\tnote"

// WriteGold writes the rows of gold set S.
func WriteGold(w io.Writer, rows []GoldRow) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintln(bw, goldHeader)
	for _, g := range rows {
		fmt.Fprintf(bw, "%d\t%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n", g.PageID, g.Title, g.Stratum, g.Signals, g.Rule, g.Population, g.Mark, g.Marker, g.Note)
	}
	return bw.Flush()
}

// ReadGold reads a file that WriteGold wrote, with the marks of the
// reviewers.
func ReadGold(r io.Reader) ([]GoldRow, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	var out []GoldRow
	line := 0
	for sc.Scan() {
		line++
		if line == 1 {
			if sc.Text() != goldHeader {
				return nil, fmt.Errorf("line 1: the header is not the header of gold set S")
			}
			continue
		}
		f := strings.Split(sc.Text(), "\t")
		if len(f) != 9 {
			return nil, fmt.Errorf("line %d: want 9 fields, have %d", line, len(f))
		}
		g := GoldRow{Title: f[1], Stratum: f[2], Rule: f[4], Mark: f[6], Marker: f[7], Note: f[8]}
		var err error
		if g.PageID, err = strconv.ParseInt(f[0], 10, 64); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if g.Signals, err = ParseSignals(f[3]); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if g.Population, err = strconv.Atoi(f[5]); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		switch g.Mark {
		case "", VerdictMath, VerdictUsesMath, VerdictNotMath:
		default:
			return nil, fmt.Errorf("line %d: the mark %q is not math, uses-math or not-math", line, g.Mark)
		}
		out = append(out, g)
	}
	return out, sc.Err()
}

// GoldMeasure is the result of Measure.
type GoldMeasure struct {
	Marked, Unmarked int
	// Precision and Recall are weighted by the population of each stratum,
	// so that they estimate the measures over all the candidates.
	Precision, Recall float64
	// The counts in the sample, not weighted.
	TruePos, FalsePos, FalseNeg int
	// ByRule gives the precision of each rule in the sample, not weighted.
	ByRule map[string][2]int // in scope and marked math, in scope
	// Misses lists the rows that are marked math and are not in scope.
	Misses []GoldRow
}

// Measure compares the marks of gold set S with the scope list. A row is
// true when it is marked math. The rows that are not marked are not
// counted.
func Measure(rows []GoldRow, cs []*Candidate) GoldMeasure {
	in := map[int64]*Candidate{}
	for _, c := range cs {
		in[c.PageID] = c
	}
	m := GoldMeasure{ByRule: map[string][2]int{}}
	sampled := map[string]int{}
	for _, g := range rows {
		if g.Mark != "" {
			sampled[g.Stratum]++
		}
	}
	var tp, fp, fn float64
	for _, g := range rows {
		if g.Mark == "" {
			m.Unmarked++
			continue
		}
		m.Marked++
		w := float64(g.Population) / float64(sampled[g.Stratum])
		c := in[g.PageID]
		scoped := c != nil && c.InScope
		rule := g.Rule
		if c != nil {
			rule = c.Rule
		}
		truth := g.Mark == VerdictMath
		if scoped {
			v := m.ByRule[rule]
			v[1]++
			if truth {
				v[0]++
			}
			m.ByRule[rule] = v
		}
		switch {
		case scoped && truth:
			m.TruePos++
			tp += w
		case scoped:
			m.FalsePos++
			fp += w
		case truth:
			m.FalseNeg++
			fn += w
			m.Misses = append(m.Misses, g)
		}
	}
	if tp+fp > 0 {
		m.Precision = tp / (tp + fp)
	}
	if tp+fn > 0 {
		m.Recall = tp / (tp + fn)
	}
	return m
}

// GoldReport writes reports/gold-s.md.
func GoldReport(w io.Writer, m GoldMeasure) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "# Gold set S\n\n")
	fmt.Fprintf(bw, "This report is made by `m2 wp gold measure`. The sample has %d marked rows and %d rows that are not marked yet. The precision and the recall are weighted by the population of each stratum (WP11.2).\n\n", m.Marked, m.Unmarked)
	fmt.Fprintf(bw, "| Measure | Value | Target |\n|---|---:|---:|\n")
	fmt.Fprintf(bw, "| Precision of the scope list | %.3f | 0.950 |\n", m.Precision)
	fmt.Fprintf(bw, "| Recall of the scope list | %.3f | 0.990 |\n\n", m.Recall)
	fmt.Fprintf(bw, "In the sample, %d rows are in scope and marked math, %d rows are in scope and not marked math, and %d rows are marked math and not in scope.\n\n", m.TruePos, m.FalsePos, m.FalseNeg)
	fmt.Fprintf(bw, "## Precision by rule\n\n| Rule | In scope | Marked math | Precision |\n|---|---:|---:|---:|\n")
	for _, r := range sortedKeys(m.ByRule) {
		v := m.ByRule[r]
		fmt.Fprintf(bw, "| %s | %d | %d | %.3f |\n", r, v[1], v[0], float64(v[0])/float64(v[1]))
	}
	if len(m.Misses) > 0 {
		fmt.Fprintf(bw, "\n## Misses\n\nThese rows are marked math and are not in scope.\n\n| Page | Signals | Rule | Note |\n|---|---|---|---|\n")
		for _, g := range m.Misses {
			fmt.Fprintf(bw, "| %s | %s | %s | %s |\n", g.Title, g.Signals, g.Rule, g.Note)
		}
	}
	return bw.Flush()
}
