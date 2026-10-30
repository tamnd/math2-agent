package gold

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource"
)

// CitationsFile holds the gold set G-cite (RS10.7): references of bibliographies with the resource that each one cites.
const CitationsFile = Dir + "/citations.md"

// NoResource is the label of a reference that cites no resource of the catalogue.
const NoResource = "none"

// CiteLanguages is the count of languages of the bibliographies that G-cite draws from.
const CiteLanguages = 5

// Citation is one reference of G-cite. Ref names the reference as <id>/e<n>#<n>. The value of a label is the cited resource, as <id>, or NoResource. Stratum names the language of the citing expression and whether the matcher gave a target.
type Citation struct {
	Ref     string  `json:"ref"`
	Stratum string  `json:"stratum"`
	Labels  []Label `json:"labels,omitempty"`
}

// Value gives the label of the reference with the rule of Pair.Value. It gives "" while the reference is open.
func (c *Citation) Value() string { return (&Pair{Labels: c.Labels}).Value() }

// ReadCitations reads G-cite in the tree at root. A tree with no file has an empty set.
func ReadCitations(root string) ([]Citation, error) {
	recs, err := mdrec.Read(filepath.Join(root, CitationsFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cs, err := mdrec.Unmarshal[Citation](recs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", CitationsFile, err)
	}
	seen := map[string]bool{}
	for n, c := range cs {
		if !strings.Contains(c.Ref, "#") {
			return nil, fmt.Errorf("%s: record %d: a reference needs the form <id>/e<n>#<n>", CitationsFile, n+1)
		}
		if seen[c.Ref] {
			return nil, fmt.Errorf("%s: record %d: the reference %s is in the set twice", CitationsFile, n+1, c.Ref)
		}
		seen[c.Ref] = true
		for _, l := range c.Labels {
			if l.Value == "" || strings.Contains(l.Value, "/") || l.By == "" {
				return nil, fmt.Errorf("%s: record %d: a label needs a resource id or %s, and a person", CitationsFile, n+1, NoResource)
			}
		}
	}
	return cs, nil
}

// WriteCitations writes G-cite to the tree at root.
func WriteCitations(root string, cs []Citation) error {
	recs, err := mdrec.Marshal(cs)
	if err != nil {
		return err
	}
	path := filepath.Join(root, CitationsFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return mdrec.Write(path, recs)
}

// ref names the reference of an edge.
func ref(c resource.Cite) string { return fmt.Sprintf("%s#%d", c.From, c.N) }

// SampleCitations draws n new references from the edges. It takes the CiteLanguages languages with the most edges, and the name of the language breaks a tie, and gives each language and each outcome of the matcher (matched or not matched) an equal part. lang gives the language of a citing expression. The same seed gives the same sample, and a reference of have is not drawn again.
func SampleCitations(edges []resource.Cite, lang func(from string) string, n int, seed string, have []Citation) []Citation {
	seen := map[string]bool{}
	for _, c := range have {
		seen[c.Ref] = true
	}
	byLang := map[string]int{}
	strata := map[string][]string{}
	for _, e := range edges {
		r := ref(e)
		l := cmp.Or(lang(e.From), "und")
		byLang[l]++
		if seen[r] {
			continue
		}
		out := "not matched"
		if e.To != "" {
			out = "matched"
		}
		strata[l+" "+out] = append(strata[l+" "+out], r)
	}
	langs := slices.SortedFunc(maps.Keys(byLang), func(a, b string) int { return cmp.Or(byLang[b]-byLang[a], strings.Compare(a, b)) })
	langs = langs[:min(len(langs), CiteLanguages)]
	var keys []string
	for _, l := range langs {
		for _, out := range []string{"matched", "not matched"} {
			if k := l + " " + out; len(strata[k]) > 0 {
				keys = append(keys, k)
				slices.SortFunc(strata[k], func(a, b string) int { return strings.Compare(rank(seed, a, ""), rank(seed, b, "")) })
			}
		}
	}
	var out []Citation
	for i := 0; len(out) < n; i++ {
		took := false
		for _, k := range keys {
			if i < len(strata[k]) && len(out) < n {
				out = append(out, Citation{Ref: strata[k][i], Stratum: k})
				took = true
			}
		}
		if !took {
			break
		}
	}
	return out
}

// CiteScore is the score of the matcher on the labelled references of G-cite.
type CiteScore struct {
	Labelled  int // the references with a final label
	Matched   int // the labelled references that the matcher gave a target
	Wrong     int // the matched references whose target is not the label
	Cited     int // the labelled references whose label is a resource
	Found     int // the references whose label is a resource and whose target is that resource
	Precision float64
	Recall    float64
}

// EvalCitations scores the edges on the labelled references. A target that names an expression counts as its resource. A labelled reference with no edge counts as not matched.
func EvalCitations(edges []resource.Cite, cs []Citation) CiteScore {
	to := map[string]string{}
	for _, e := range edges {
		id, _, _ := strings.Cut(e.To, "/")
		to[ref(e)] = id
	}
	var s CiteScore
	for _, c := range cs {
		v := c.Value()
		if v == "" {
			continue
		}
		s.Labelled++
		got := to[c.Ref]
		if got != "" {
			s.Matched++
			if got != v {
				s.Wrong++
			}
		}
		if v != NoResource {
			s.Cited++
			if got == v {
				s.Found++
			}
		}
	}
	s.Precision = ratio(s.Matched-s.Wrong, s.Matched)
	s.Recall = ratio(s.Found, s.Cited)
	return s
}

// AddCiteLabel adds a label to the reference ref. A person labels a reference once.
func AddCiteLabel(cs []Citation, r string, l Label) error {
	if l.Value == "" || strings.Contains(l.Value, "/") {
		return fmt.Errorf("gold: the value %q is not a resource id or %s", l.Value, NoResource)
	}
	for i := range cs {
		if cs[i].Ref != r {
			continue
		}
		if slices.ContainsFunc(cs[i].Labels, func(x Label) bool { return x.By == l.By }) {
			return fmt.Errorf("gold: %s already labelled the reference %s", l.By, r)
		}
		if cs[i].Value() != "" {
			return fmt.Errorf("gold: the reference %s has its label", r)
		}
		cs[i].Labels = append(cs[i].Labels, l)
		return nil
	}
	return fmt.Errorf("gold: the reference %s is not in the set", r)
}
