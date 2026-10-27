package wpcorpus

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// The verdicts of the scope classifier (WP1.3).
const (
	VerdictMath     = "math"
	VerdictUsesMath = "uses-math"
	VerdictNotMath  = "not-math"
)

// acceptAt is the lowest confidence at which the rule classifier puts a
// candidate with one weak signal in scope.
const acceptAt = 0.6

// Candidate is one article that one or more signals name.
type Candidate struct {
	PageID     int64
	Title      string
	Signals    Signals
	Projects   []string
	Class      string
	Importance string
	// CatDepth is the lowest depth of a category of the article in the tree
	// of S6, or -1.
	CatDepth int
	// Links is the number of articles in S1 that link to the article.
	Links int

	Verdict    string
	Confidence float64
	InScope    bool
	Rule       string
}

// weak is the set of the signals that put an article in scope only in a
// pair, or with the classifier.
const weak = S3 | S5 | S6 | S7 | S8 | S9

// Classify applies the scope rule of WP1.2 and the rule classifier of WP1.3.
// The model pass of WP7.2 replaces the rule verdicts later. Gold set S
// measures both.
func Classify(c *Candidate) {
	n := 0
	for s := S1; s <= S9; s <<= 1 {
		if c.Signals&weak&s != 0 {
			n++
		}
	}
	switch {
	case c.Signals&(S1|S2|S4) != 0:
		c.Verdict, c.Confidence, c.Rule = VerdictMath, 1, "strong"
	case n >= 2:
		c.Verdict, c.Confidence, c.Rule = VerdictMath, 0.9, "two weak"
	case c.Signals&S5 != 0:
		c.Verdict, c.Confidence, c.Rule = VerdictMath, 0.8, "S5"
	case c.Signals&S6 != 0 && c.CatDepth <= 1:
		c.Verdict, c.Confidence, c.Rule = VerdictMath, 0.8, "S6 near"
	case c.Signals&S6 != 0 && c.CatDepth == 2:
		c.Verdict, c.Confidence, c.Rule = VerdictMath, 0.65, "S6 mid"
	case c.Signals&S6 != 0:
		c.Verdict, c.Confidence, c.Rule = VerdictUsesMath, 0.5, "S6 far"
	case c.Signals&S8 != 0:
		c.Verdict, c.Confidence, c.Rule = VerdictMath, 0.7, "S8"
	case c.Signals&S9 != 0:
		c.Verdict, c.Confidence, c.Rule = VerdictMath, 0.6, "S9"
	case c.Signals&S7 != 0:
		c.Verdict, c.Confidence, c.Rule = VerdictUsesMath, 0.5, "S7"
	case c.Signals&S3 != 0:
		c.Verdict, c.Confidence, c.Rule = VerdictUsesMath, 0.4, "S3 alone"
	default:
		c.Verdict, c.Confidence, c.Rule = VerdictNotMath, 1, "no signal"
	}
	c.InScope = c.Verdict == VerdictMath && c.Confidence >= acceptAt
}

// candidatesHeader is the header of candidates.tsv. scope.tsv has the same
// columns, with the rows that are in scope.
const candidatesHeader = "page_id\ttitle\tsignals\tprojects\tclass\timportance\tcat_depth\ts1_links\tverdict\tconfidence\trule\tin_scope\tdump"

// WriteCandidates writes the rows of the candidates, in page id order. With
// inScope, it writes only the rows that are in scope.
func WriteCandidates(w io.Writer, cs []*Candidate, dump string, inScope bool) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintln(bw, candidatesHeader)
	sorted := slices.Clone(cs)
	slices.SortFunc(sorted, func(a, b *Candidate) int { return int(a.PageID - b.PageID) })
	for _, c := range sorted {
		if inScope && !c.InScope {
			continue
		}
		depth := ""
		if c.CatDepth >= 0 {
			depth = strconv.Itoa(c.CatDepth)
		}
		fmt.Fprintf(bw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%.2f\t%s\t%t\t%s\n",
			c.PageID, c.Title, c.Signals, strings.Join(c.Projects, ","), c.Class, c.Importance,
			depth, c.Links, c.Verdict, c.Confidence, c.Rule, c.InScope, dump)
	}
	return bw.Flush()
}

// ReadCandidates reads a file that WriteCandidates wrote.
func ReadCandidates(r io.Reader) ([]*Candidate, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	var out []*Candidate
	line := 0
	for sc.Scan() {
		line++
		if line == 1 {
			if sc.Text() != candidatesHeader {
				return nil, fmt.Errorf("line 1: the header is not the header of candidates.tsv")
			}
			continue
		}
		f := strings.Split(sc.Text(), "\t")
		if len(f) != 13 {
			return nil, fmt.Errorf("line %d: want 13 fields, have %d", line, len(f))
		}
		c := &Candidate{Title: f[1], Class: f[4], Importance: f[5], CatDepth: -1, Verdict: f[8], Rule: f[10]}
		var err error
		if c.PageID, err = strconv.ParseInt(f[0], 10, 64); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if c.Signals, err = ParseSignals(f[2]); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if f[3] != "" {
			c.Projects = strings.Split(f[3], ",")
		}
		if f[6] != "" {
			if c.CatDepth, err = strconv.Atoi(f[6]); err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
		}
		if c.Links, err = strconv.Atoi(f[7]); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if c.Confidence, err = strconv.ParseFloat(f[9], 64); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		c.InScope = f[11] == "true"
		out = append(out, c)
	}
	return out, sc.Err()
}

// ScopeReport writes reports/scope.md: the counts of the candidates and of
// the scope by signal, by rule, by verdict and by class.
func ScopeReport(w io.Writer, cs []*Candidate, dump string, pending []string) error {
	bw := bufio.NewWriter(w)
	in := 0
	bySignal := map[Signals][2]int{} // candidates, in scope
	byRule := map[string][2]int{}
	byClass := map[string]int{}
	only := map[Signals]int{}
	for _, c := range cs {
		if c.InScope {
			in++
			cl := c.Class
			if cl == "" {
				cl = "(none)"
			}
			byClass[cl]++
		}
		for s := S1; s <= S9; s <<= 1 {
			if c.Signals&s != 0 {
				v := bySignal[s]
				v[0]++
				if c.InScope {
					v[1]++
				}
				bySignal[s] = v
			}
		}
		if c.InScope && c.Signals&(c.Signals-1) == 0 {
			only[c.Signals]++
		}
		v := byRule[c.Rule]
		v[0]++
		if c.InScope {
			v[1]++
		}
		byRule[c.Rule] = v
	}
	fmt.Fprintf(bw, "# The scope of the Wikipedia corpus\n\n")
	fmt.Fprintf(bw, "This report is made by `m2 wp scope` from the dump %s. It has %d candidates, and %d of them are in scope (WP1.2).\n\n", dump, len(cs), in)
	if len(pending) > 0 {
		fmt.Fprintf(bw, "These signals are not computed yet: %s. The counts below do not include them.\n\n", strings.Join(pending, ", "))
	}
	fmt.Fprintf(bw, "## By signal\n\n| Signal | Candidates | In scope | In scope with only this signal |\n|---|---:|---:|---:|\n")
	for s := S1; s <= S9; s <<= 1 {
		v := bySignal[s]
		fmt.Fprintf(bw, "| %s | %d | %d | %d |\n", s, v[0], v[1], only[s])
	}
	fmt.Fprintf(bw, "\n## By rule\n\n| Rule | Candidates | In scope |\n|---|---:|---:|\n")
	for _, r := range sortedKeys(byRule) {
		fmt.Fprintf(bw, "| %s | %d | %d |\n", r, byRule[r][0], byRule[r][1])
	}
	fmt.Fprintf(bw, "\n## By assessment class\n\n| Class | In scope |\n|---|---:|\n")
	for _, cl := range sortedKeys(byClass) {
		fmt.Fprintf(bw, "| %s | %d |\n", cl, byClass[cl])
	}
	return bw.Flush()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
