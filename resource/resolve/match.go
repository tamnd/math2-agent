package resolve

import (
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
)

// Citation is a reference of a bibliography with its parsed fields (RS3.7 step 1). A pinned reference parser fills the fields. Parse fills them from plain text when no parser is at hand.
type Citation struct {
	Text    string   `json:"text,omitempty"`
	Authors []string `json:"authors,omitempty"`
	Title   string   `json:"title,omitempty"`
	Year    int      `json:"year,omitempty"`
	Venue   string   `json:"venue,omitempty"`
	Volume  string   `json:"volume,omitempty"`
	Pages   string   `json:"pages,omitempty"`
	Edition int      `json:"edition,omitempty"`
	IDs     []string `json:"ids,omitempty"` // in the form sys:value
}

var (
	doiRE     = regexp.MustCompile(`(?i)\b(10\.[0-9]{4,9}/[^\s,;"<>]+)`)
	arxivRE   = regexp.MustCompile(`(?i)\barxiv[:\s]*((?:[0-9]{4}\.[0-9]{4,5}|[a-z-]+(?:\.[A-Z]{2})?/[0-9]{7})(?:v[0-9]+)?)`)
	mrRE      = regexp.MustCompile(`\bMR\s?([0-9]{4,8})\b`)
	zblRE     = regexp.MustCompile(`\bZbl\s?([0-9]{4}\.[0-9]{5}|[0-9]{2,8})\b`)
	isbnRE    = regexp.MustCompile(`(?i)\bISBN[:\s-]*((?:97[89][\s-]?)?(?:[0-9][\s-]?){9}[0-9X])`)
	yearRE    = regexp.MustCompile(`\b(1[5-9][0-9]{2}|20[0-9]{2})\b`)
	quoteRE   = regexp.MustCompile(`["“«]([^"”»]{4,})["”»]`)
	initialRE = regexp.MustCompile(`^\p{Lu}\.(-?\p{Lu}\.)*$`)
	particles = map[string]bool{"van": true, "von": true, "de": true, "der": true, "den": true, "la": true, "le": true, "du": true, "di": true, "da": true, "and": true, "&": true}
)

// Parse fills a citation from plain text. It finds the ids, the year and the edition, and it takes the authors and the title from the form "Authors, Title, ..." or "Authors: Title. ...". The matcher does not trust the title alone, so a wrong split costs little.
func Parse(text string) Citation {
	c := Citation{Text: strings.TrimSpace(text)}
	add := func(sys, v string) {
		if k, err := Key(sys + ":" + v); err == nil && !slices.Contains(c.IDs, k) {
			c.IDs = append(c.IDs, k)
		}
	}
	for _, m := range doiRE.FindAllStringSubmatch(text, -1) {
		add("doi", strings.TrimRight(m[1], ".)]"))
	}
	for _, m := range arxivRE.FindAllStringSubmatch(text, -1) {
		add("arxiv", m[1])
	}
	for _, m := range mrRE.FindAllStringSubmatch(text, -1) {
		add("mr", m[1])
	}
	for _, m := range zblRE.FindAllStringSubmatch(text, -1) {
		add("zbl", m[1])
	}
	for _, m := range isbnRE.FindAllStringSubmatch(text, -1) {
		add("isbn", m[1])
	}
	if ys := yearRE.FindAllString(text, -1); len(ys) > 0 {
		c.Year, _ = strconv.Atoi(ys[len(ys)-1])
	}
	c.Edition = cluster.Edition(text)
	if m := quoteRE.FindStringSubmatchIndex(text); m != nil {
		c.Title = strings.Trim(text[m[2]:m[3]], " ,.")
		c.Authors = splitAuthors(text[:m[0]])
		return c
	}
	if i := strings.Index(text, ": "); i > 0 && i < 120 && looksLikeNames(text[:i]) {
		c.Authors = splitAuthors(text[:i])
		rest := text[i+2:]
		if j := strings.Index(rest, ". "); j > 0 {
			rest = rest[:j]
		}
		c.Title = strings.Trim(strings.SplitN(rest, ", ", 2)[0], " .")
		return c
	}
	parts := strings.Split(text, ", ")
	k := 0
	for k < len(parts)-1 && looksLikeNames(parts[k]) {
		k++
	}
	for _, p := range parts[:k] {
		c.Authors = append(c.Authors, splitAuthors(p)...)
	}
	if k < len(parts) {
		c.Title = strings.Trim(parts[k], " .")
	}
	return c
}

// looksLikeNames reports whether a part of a reference is a list of names: each word is a capital word, an initial or a particle, and the part has an initial or the word "and".
func looksLikeNames(s string) bool {
	words := strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == ',' || r == ';' })
	if len(words) == 0 || len(words) > 12 {
		return false
	}
	mark := false
	for _, w := range words {
		switch {
		case initialRE.MatchString(w):
			mark = true
		case particles[strings.ToLower(w)]:
			if w == "and" || w == "&" {
				mark = true
			}
		case unicode.IsUpper([]rune(w)[0]):
		default:
			return false
		}
	}
	return mark
}

func splitAuthors(s string) []string {
	s = strings.NewReplacer(" and ", ";", " & ", ";").Replace(strings.Trim(s, " ,:;"))
	var out []string
	for _, p := range strings.Split(s, ";") {
		if p = strings.Trim(p, " ,"); p != "" {
			out = append(out, p)
		}
	}
	// "Rudin, W." is one name. A list with commas only, as "W. Rudin, A. Weil", splits at each comma that does not follow a family name.
	if len(out) == 1 && strings.Count(out[0], ",") > 1 {
		parts := strings.Split(out[0], ", ")
		out = out[:0]
		for i := 0; i < len(parts); i++ {
			if i+1 < len(parts) && initialRE.MatchString(strings.Fields(parts[i+1])[0]) && !strings.Contains(parts[i], ".") {
				out = append(out, parts[i]+", "+parts[i+1])
				i++
				continue
			}
			out = append(out, parts[i])
		}
	}
	return out
}

func blockKey(family string, year int) string { return family + ":" + strconv.Itoa(year) }

// first gives the family name of the first creator and the year of a resource.
func first(r *resource.Resource) (string, int) {
	y := 0
	if len(r.Date) >= 4 {
		y, _ = strconv.Atoi(r.Date[:4])
	}
	for _, c := range r.Creators {
		if f := cluster.Family(c.Name); f != "" {
			return f, y
		}
	}
	return "", y
}

// Score gives the score of a resource for a citation, from 0 to 1: the title, the authors and the year (RS3.7 step 4). A title of the resource that the text of the citation has in full counts as a match of the title.
func Score(c *Citation, r *resource.Resource) float64 {
	title := cluster.Similarity(c.Title, r.Title)
	if r.TitleEn != "" {
		title = max(title, cluster.Similarity(c.Title, r.TitleEn))
	}
	if t := cluster.WorkTitle(r.Title); len([]rune(t)) >= 12 && strings.Contains(cluster.Fold(c.Text), t) {
		title = 1
	}
	authors := 0.0
	if len(c.Authors) > 0 && len(r.Creators) > 0 {
		var have []string
		for _, cr := range r.Creators {
			have = append(have, cluster.Family(cr.Name))
		}
		n := 0
		for _, a := range c.Authors {
			if slices.Contains(have, cluster.Family(a)) {
				n++
			}
		}
		authors = float64(n) / float64(min(len(c.Authors), len(have)))
	}
	_, y := first(r)
	year := 0.5
	switch {
	case c.Year == 0 || y == 0:
	case c.Year == y:
		year = 1
	case c.Year-y == 1 || y-c.Year == 1:
		year = 0.7
	case c.Year > y:
		year = 0.4 // a later edition or a reprint
	default:
		year = 0
	}
	return 0.6*title + 0.25*authors + 0.15*year
}

// Cite resolves one citation: the ids first, then the blocks and the scores (RS3.7 steps 2 to 5).
func (r *Resolver) Cite(ctx context.Context, c Citation) (Result, error) {
	for _, id := range c.IDs {
		res, err := r.ID(ctx, id)
		if err != nil {
			return Result{}, err
		}
		if res.Status != NotFound {
			res.Query = c.Text
			return r.edition(res, c), nil
		}
	}
	res := Result{Query: c.Text, Status: NotFound, Pass: Match, Version: Version}
	seen := map[string]bool{}
	var cands []Candidate
	for _, a := range c.Authors {
		f := cluster.Family(a)
		if f == "" {
			continue
		}
		var keys []string
		if c.Year > 0 {
			keys = []string{blockKey(f, c.Year-1), blockKey(f, c.Year), blockKey(f, c.Year+1)}
		}
		for k := range r.Index.block {
			if c.Year == 0 || c.Edition > 1 {
				if strings.HasPrefix(k, f+":") {
					keys = append(keys, k)
				}
			}
		}
		for _, k := range keys {
			for _, id := range r.Index.block[k] {
				if seen[id] {
					continue
				}
				seen[id] = true
				if s := Score(&c, r.Index.res[id]); s >= Middle {
					cands = append(cands, Candidate{Resource: id, Score: round(s)})
				}
			}
		}
	}
	slices.SortFunc(cands, func(a, b Candidate) int {
		if a.Score != b.Score {
			if a.Score > b.Score {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Resource, b.Resource)
	})
	switch {
	case len(cands) == 0:
		return res, nil
	case cands[0].Score >= High && (len(cands) == 1 || cands[1].Score < High):
		res.Status, res.Score = Found, cands[0].Score
		res.Ref = &Ref{Resource: cands[0].Resource}
		return r.edition(res, c), nil
	default:
		res.Status, res.Candidates = Candidates, cands
		return res, nil
	}
}

// edition narrows a match to the expression of the edition that the citation names, when the resource has it.
func (r *Resolver) edition(res Result, c Citation) Result {
	if res.Ref == nil || res.Ref.Expression != "" || c.Edition == 0 {
		return res
	}
	rs := r.Index.res[res.Ref.Resource]
	if rs == nil {
		return res
	}
	for _, e := range rs.Expressions {
		if e.Kind == "edition" && cluster.Edition(e.Label) == c.Edition {
			res.Ref.Expression = e.Key
			break
		}
	}
	return res
}

func round(f float64) float64 { return float64(int(f*1000+0.5)) / 1000 }
