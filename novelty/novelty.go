// Package novelty runs the novelty check of spec 17.5 on an output that can be called new: a conjecture of explore, a proof of an open node, or a counterexample.
//
// A check runs each [Searcher] on the [Claim]: the graph, the record plane, the external indexes and the problem databases. A [Judge] compares the claim with the best matches of each search, and [Check] gives one of three verdicts: [Known] with the match, [LikelyKnown] with the close matches that a person must read, or [NoMatch]. No verdict says that the claim is new. [NoMatch] means only that the searches found nothing, and only a person makes a statement of novelty.
//
// The [Result] records each search and each match, thus the run record of the output can keep it. A match keeps only its source, its id, its link and the verdict of the judge, because math2 stores only the ids of the matches of an index that it does not import (spec 09.6).
package novelty

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// The verdicts of a check.
const (
	Known       = "known"          // a match states the claim, or a result that implies it
	LikelyKnown = "likely-known"   // close matches that a person must read
	NoMatch     = "no-match-found" // the searches found nothing
)

// The relations that a [Judge] gives between the claim and a match. They are the verdicts of the judge of curate.
const (
	Same        = "same"        // the claim states the result of the match
	Generalizes = "generalizes" // the claim states more than the match
	Specializes = "specializes" // the claim states less than the match, thus the match implies it
	Related     = "related"     // the claim and the match are about the same thing, and neither implies the other
	Different   = "different"   // the claim and the match are about different things
)

// Judged is the number of the best matches of each search that the judge compares with the claim.
const Judged = 3

// A Claim is the output to check.
type Claim struct {
	Tag       string   // the node of the claim, if it has one. The graph search does not give it as a match.
	Name      string   // a short name of the result, for example Infinitude of the primes
	Statement string   // the statement in words, in Markdown with LaTeX
	Terms     []string // the first terms of an integer sequence, for a claim about one. The search of the OEIS needs them.
}

var (
	formula = regexp.MustCompile(`\$\$?[^$]*\$\$?`)
	link    = regexp.MustCompile(`\[\[([^\]|]*)(?:\|[^\]]*)?\]\]`)
)

// QueryWords is the number of words of the statement in a [Claim.Query].
const QueryWords = 12

// Query returns the words of a search of the claim on the web: the name, or the first words of the statement with no formulas when the claim has no name.
func (c Claim) Query() string {
	if s := strings.Join(strings.Fields(c.Name), " "); s != "" {
		return s
	}
	text := link.ReplaceAllString(formula.ReplaceAllString(c.Statement, " "), "$1")
	var ws []string
	for _, w := range strings.Fields(text) {
		if w = strings.Trim(w, ".,;:()"); w != "" {
			ws = append(ws, w)
		}
	}
	return strings.Join(ws[:min(len(ws), QueryWords)], " ")
}

// A Match is a result that a search found.
type Match struct {
	Source  string  `json:"source"`            // the search, for example zbmath
	ID      string  `json:"id"`                // the id in the source, for example a tag, a record id or an A-number
	URL     string  `json:"url,omitempty"`     // the home of the match
	Score   float64 `json:"score,omitempty"`   // the score of the search, when it gives one
	Status  string  `json:"status,omitempty"`  // the status that a problem database gives, for example solved
	Verdict string  `json:"verdict,omitempty"` // the relation that the judge gave, or "" when it did not compare the match
	Why     string  `json:"why,omitempty"`     // the reason of the judge

	Title string `json:"-"` // the title or the name, for the judge. The result does not keep it.
	Text  string `json:"-"` // the statement or the summary, for the judge. The result does not keep it.
}

// A Search is one search of a check.
type Search struct {
	Source  string `json:"source"`
	Query   string `json:"query"`
	Matches int    `json:"matches"`
	Error   string `json:"error,omitempty"`
}

// A Result is the result of a check, which the run record of the output keeps.
type Result struct {
	Verdict  string   `json:"verdict"`
	Match    *Match   `json:"match,omitempty"` // the match that makes the claim known
	Matches  []Match  `json:"matches"`         // the matches, best first in each search
	Searches []Search `json:"searches"`
}

// A Searcher is one search of the check.
type Searcher interface {
	// Name is the name of the search, for example graph or zbmath.
	Name() string
	// Search returns the query that it ran and the matches, best first. An empty query means that the search does not apply to the claim, for example the OEIS to a claim with no terms.
	Search(ctx context.Context, c Claim) (query string, ms []Match, err error)
}

// A Judge compares the claim with the match, and gives a relation and its reason.
type Judge func(ctx context.Context, c Claim, m Match) (relation, why string, err error)

// Check runs each search on the claim, and the judge on the [Judged] best matches of each search. A match whose relation is [Same] or [Specializes] makes the claim [Known]. A match that is [Generalizes] or [Related] makes it [LikelyKnown], and so does each match when the judge is nil, because then a person must read the matches. With no such match, the verdict is [NoMatch].
//
// A search that fails is in the result with its error. When the verdict is [NoMatch] and a search failed, Check also returns an error, because then the searches did not all run. An error of the judge stops the check.
func Check(ctx context.Context, c Claim, judge Judge, ss ...Searcher) (*Result, error) {
	if strings.TrimSpace(c.Statement) == "" && strings.TrimSpace(c.Name) == "" {
		return nil, errors.New("novelty: the claim has no statement")
	}
	r := &Result{Verdict: NoMatch, Matches: []Match{}, Searches: []Search{}}
	var failed []string
	for _, s := range ss {
		q, ms, err := s.Search(ctx, c)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			r.Searches = append(r.Searches, Search{Source: s.Name(), Query: q, Error: err.Error()})
			failed = append(failed, s.Name())
			continue
		}
		if q == "" {
			continue
		}
		r.Searches = append(r.Searches, Search{Source: s.Name(), Query: q, Matches: len(ms)})
		for i, m := range ms {
			if m.Source == "" {
				m.Source = s.Name()
			}
			if judge != nil && i < Judged {
				rel, why, err := judge(ctx, c, m)
				if err != nil {
					return nil, fmt.Errorf("novelty: the judge of %s %s: %w", m.Source, m.ID, err)
				}
				if !slices.Contains([]string{Same, Generalizes, Specializes, Related, Different}, rel) {
					return nil, fmt.Errorf("novelty: the judge gave the relation %q to %s %s", rel, m.Source, m.ID)
				}
				m.Verdict, m.Why = rel, why
			}
			r.Matches = append(r.Matches, m)
		}
	}
	for i, m := range r.Matches {
		switch {
		case m.Verdict == Same || m.Verdict == Specializes:
			if r.Verdict != Known {
				r.Verdict, r.Match = Known, &r.Matches[i]
			}
		case r.Verdict == NoMatch && (m.Verdict == Generalizes || m.Verdict == Related || judge == nil):
			r.Verdict = LikelyKnown
		}
	}
	if r.Verdict == NoMatch && len(failed) > 0 {
		return r, fmt.Errorf("novelty: the searches %s failed, thus the check found no match only in the other searches", strings.Join(failed, ", "))
	}
	return r, nil
}
