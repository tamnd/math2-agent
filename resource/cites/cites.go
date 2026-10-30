// Package cites makes the cites edges of the catalogue (spec corpus resources RS2.7, RS3.7 and RS6.4). It turns the references of a bibliography into citations, runs the matcher of package resolve on each, and gives one edge for each reference. The candidates of a weak match go to the queue res-resolve, and the decision of a person on an item wins over the matcher.
package cites

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/ids"
	"github.com/tamnd/math2-agent/resource/resolve"
)

// PersonMatcher is the matcher of an edge that a person decided.
const PersonMatcher = "person"

// CrossrefRef is one reference of the field reference of a work of Crossref. The publisher deposits it. Most fields are optional.
type CrossrefRef struct {
	Key          string `json:"key"`
	DOI          string `json:"DOI"`
	Unstructured string `json:"unstructured"`
	ArticleTitle string `json:"article-title"`
	Author       string `json:"author"`
	Year         string `json:"year"`
	JournalTitle string `json:"journal-title"`
	Volume       string `json:"volume"`
	FirstPage    string `json:"first-page"`
	SeriesTitle  string `json:"series-title"`
	VolumeTitle  string `json:"volume-title"`
	ISBN         string `json:"ISBN"`
	Edition      string `json:"edition"`
}

// FromCrossref makes the citation of one reference of Crossref. The text of the reference gives the fields that the publisher did not deposit.
func FromCrossref(r CrossrefRef) resolve.Citation {
	c := resolve.Citation{}
	if r.Unstructured != "" {
		c = resolve.Parse(r.Unstructured)
	}
	if r.Unstructured == "" {
		c.Text = strings.Join(nonEmpty(r.Author, r.ArticleTitle, r.VolumeTitle, r.JournalTitle, r.SeriesTitle, r.Volume, r.Year), ", ")
	}
	if t := cmpOr(r.ArticleTitle, r.VolumeTitle); t != "" {
		c.Title = t
	}
	if r.Author != "" && len(c.Authors) == 0 {
		c.Authors = []string{r.Author}
	}
	if y, err := strconv.Atoi(strings.TrimSpace(r.Year)); err == nil && y > 0 {
		c.Year = y
	}
	if v := cmpOr(r.JournalTitle, r.SeriesTitle); v != "" {
		c.Venue = v
	}
	if r.Volume != "" {
		c.Volume = r.Volume
	}
	if r.FirstPage != "" {
		c.Pages = r.FirstPage
	}
	if e, err := strconv.Atoi(strings.TrimSpace(r.Edition)); err == nil && e > 0 {
		c.Edition = e
	}
	add := func(key, v string) {
		if n, err := ids.Normalize(key, v); err == nil && !slices.Contains(c.IDs, key+":"+n) {
			c.IDs = append(c.IDs, key+":"+n)
		}
	}
	if r.DOI != "" {
		add("doi", r.DOI)
	}
	if r.ISBN != "" {
		add("isbn", r.ISBN)
	}
	return c
}

// FromArxiv makes the citation of one bibliography entry of the analysis plane of tamnd/arxiv. The entry holds the year and the ids that the bibliography gives, and no text.
func FromArxiv(year int, doi, arxiv string) resolve.Citation {
	c := resolve.Citation{Year: year}
	for _, kv := range [][2]string{{"doi", doi}, {"arxiv", arxiv}} {
		if kv[1] == "" {
			continue
		}
		if n, err := ids.Normalize(kv[0], kv[1]); err == nil {
			c.IDs = append(c.IDs, kv[0]+":"+n)
		}
	}
	return c
}

func nonEmpty(vs ...string) []string {
	var out []string
	for _, v := range vs {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Citing gives the citing expression of a resource for one of its ids, as <id>/e<n>: the expression or the manifestation that has the id, or the first expression when the work has the id. A resource with no expression gives an empty string, because an edge comes from an expression.
func Citing(r *resource.Resource, key, value string) string {
	for _, e := range r.Expressions {
		if slices.Contains(e.IDs[key], value) {
			return r.ID + "/" + e.Key
		}
		for _, m := range e.Manifestations {
			if slices.Contains(m.IDs[key], value) {
				return r.ID + "/" + e.Key
			}
		}
	}
	if len(r.Expressions) > 0 {
		return r.ID + "/" + r.Expressions[0].Key
	}
	return ""
}

// Open reports whether the citing expression <id>/<key> has a manifestation of the class open. Only then an edge keeps the text of its reference (rule H09).
func Open(r *resource.Resource, from string) bool {
	for _, e := range r.Expressions {
		if r.ID+"/"+e.Key == from {
			return slices.ContainsFunc(e.Manifestations, func(m resource.Manifestation) bool { return m.Access != nil && m.Access.Class == "open" })
		}
	}
	return false
}

// Target gives the target of the queue item of one reference, as <from>#<n>. A decision of a person on res-resolve names it.
func Target(from string, n int) string { return from + "#" + strconv.Itoa(n) }

// Matcher holds what the matching of the references needs.
type Matcher struct {
	Resolver  *resolve.Resolver
	Decisions map[string]string // the resource that a person found for each target, or empty for not found
}

// Edges matches the citations of one citing expression, in the order of the bibliography from 1. It gives the edges and the targets of the references with candidates. keepText keeps the text of each reference in its edge.
func (m *Matcher) Edges(ctx context.Context, from string, cs []resolve.Citation, keepText bool) ([]resource.Cite, []resolve.Result, error) {
	var edges []resource.Cite
	var weak []resolve.Result
	for i, c := range cs {
		e := resource.Cite{From: from, N: i + 1, Parsed: parsed(c)}
		if keepText {
			e.Text = c.Text
		}
		if to, ok := m.Decisions[Target(from, e.N)]; ok {
			e.To, e.Matcher = to, PersonMatcher
			edges = append(edges, e)
			continue
		}
		res, err := m.Resolver.Cite(ctx, c)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", Target(from, e.N), err)
		}
		e.Matcher = cmpOr(res.Version, resolve.Version)
		switch res.Status {
		case resolve.Found:
			e.To, e.Score = res.Ref.At(), res.Score
		case resolve.Candidates:
			res.Query = Target(from, e.N)
			weak = append(weak, res)
		}
		edges = append(edges, e)
	}
	return edges, weak, nil
}

// parsed gives the fields of a citation that an edge keeps. The fields are facts.
func parsed(c resolve.Citation) *resource.Parsed {
	p := &resource.Parsed{Authors: c.Authors, Title: c.Title, Venue: c.Venue, Volume: c.Volume, Pages: c.Pages}
	if c.Year > 0 {
		p.Year = strconv.Itoa(c.Year)
	}
	if c.Edition > 0 {
		p.Edition = strconv.Itoa(c.Edition)
	}
	for _, id := range c.IDs {
		k, v, ok := strings.Cut(id, ":")
		if !ok {
			continue
		}
		if p.IDs == nil {
			p.IDs = resource.IDs{}
		}
		p.IDs[k] = append(p.IDs[k], v)
	}
	if p.Title == "" && len(p.Authors) == 0 && p.Year == "" && len(p.IDs) == 0 {
		return nil
	}
	return p
}

// Merge replaces the edges of the citing expressions of done in old with the new edges, and keeps the other edges.
func Merge(old, edges []resource.Cite, done map[string]bool) []resource.Cite {
	out := slices.DeleteFunc(slices.Clone(old), func(c resource.Cite) bool { return done[c.From] })
	return append(out, edges...)
}
