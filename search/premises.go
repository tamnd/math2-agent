package search

import (
	"cmp"
	"errors"
	"maps"
	"slices"
	"strings"
)

// Goal is a request for premises: a Lean goal, and the node that the goal states when there is one.
type Goal struct {
	Goal string // the goal, as Lean prints it, with the hypotheses before ⊢
	Tag  string // the node of the goal, or "". The declarations of the nodes that it uses get a strong prior.
	K    int    // the number of premises. Zero means 20.
}

// Premise is a Lean declaration that can help to prove a goal.
type Premise struct {
	Decl     string   `json:"decl"`
	Tags     []string `json:"tags,omitempty"` // the nodes that bind the declaration
	Score    float64  `json:"score"`
	Channels []Hit    `json:"channels"` // the formal, dependency and embedding channels that found it
}

// neighbours is the number of declarations with a near statement whose proofs give the dependency priors.
const neighbours = 10

// Premises ranks the declarations that are most likely to help to prove the goal (spec 14.5). It fuses three lists with reciprocal rank fusion: the declarations whose conclusions fit the goal, the declarations that the proofs of near statements and the uses of the node use, and the declarations whose statements are near the goal by embedding. A learned reranker comes with the run records of the agents.
func (ix *Index) Premises(g Goal) ([]Premise, error) {
	if strings.TrimSpace(g.Goal) == "" && g.Tag == "" {
		return nil, errors.New("search: the goal is empty")
	}
	if g.K <= 0 {
		g.K = 20
	}
	lists := map[string][]cand{}
	if strings.TrimSpace(g.Goal) != "" {
		lists[Formal] = ix.formalDecls(parseLean(g.Goal))
		lists[Embedding] = ix.nearDecls(g.Goal)
	}

	// The dependency priors. A declaration that a node in the uses of the goal binds gets the score 2. A constant in the proof of a near declaration gets the similarity of that declaration.
	deps := map[string]float64{}
	if i, ok := ix.byTag[g.Tag]; ok {
		for _, u := range ix.docs[i].uses {
			for _, d := range ix.decls {
				if slices.Contains(d.tags, u) {
					deps[d.name] += 2
				}
			}
		}
	}
	near := lists[Embedding]
	if len(near) > neighbours {
		near = near[:neighbours]
	}
	for _, c := range near {
		if i, ok := ix.byDecl[c.decl]; ok {
			for _, u := range ix.decls[i].uses {
				deps[u] += c.score
			}
		}
	}
	lists[Dependency] = ranked(deps)
	for i := range lists[Dependency] {
		lists[Dependency][i].decl = lists[Dependency][i].key
	}

	acc := map[string]*Premise{}
	for _, ch := range []string{Formal, Dependency, Embedding} {
		var top float64
		if len(lists[ch]) > 0 {
			top = lists[ch][0].score
		}
		for i, c := range lists[ch] {
			p, ok := acc[c.decl]
			if !ok {
				p = &Premise{Decl: c.decl}
				if j, ok := ix.byDecl[c.decl]; ok {
					p.Tags = ix.decls[j].tags
				}
				acc[c.decl] = p
			}
			p.Score += rrf(Weights[ch], i+1, c.score, top)
			p.Channels = append(p.Channels, Hit{Channel: ch, Rank: i + 1, Score: c.score})
		}
	}
	out := slices.SortedFunc(maps.Values(acc), func(a, b *Premise) int {
		return cmp.Or(cmp.Compare(b.Score, a.Score), strings.Compare(a.Decl, b.Decl))
	})
	if len(out) > g.K {
		out = out[:g.K]
	}
	res := make([]Premise, len(out))
	for i, p := range out {
		res[i] = *p
	}
	return res, nil
}

// formalDecls returns the declarations that the formal channel finds for the query, each one once, best first.
func (ix *Index) formalDecls(q leanQuery) []cand {
	var out []cand
	seen := map[string]bool{}
	for _, c := range ix.formal(q) {
		if seen[c.decl] {
			continue
		}
		seen[c.decl] = true
		out = append(out, cand{key: c.decl, score: c.score, decl: c.decl})
	}
	return out
}

// nearDecls returns the declarations whose statements are near the text by embedding, best first.
func (ix *Index) nearDecls(text string) []cand {
	q := quantize(ix.embedder.Embed(text))
	best := map[string]float64{}
	for _, o := range ix.declVecs {
		if s := dot(q, o.v); s > 0 && s > best[o.owner] {
			best[o.owner] = s
		}
	}
	out := ranked(best)
	for i := range out {
		out[i].decl = out[i].key
	}
	return out
}
