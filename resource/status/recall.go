package status

import (
	"cmp"
	"maps"
	"slices"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census"
)

// MaxCaptures is the most captures that the estimate of one stratum uses. The estimate uses the captures that hold the most works of the stratum. More captures make the fit slow, because the count of histories doubles with each capture.
const MaxCaptures = 6

// Seed is the seed of the bootstrap, so that two builds of the same tree give the same report.
const Seed = 2148

// Recall is the recall of one stratum (RS11.4): the live resources in scope over the estimate of the universe. The estimate comes from the capture histories of the catalogues that the resources group, with the catalogues that share lineage merged into one capture.
type Recall struct {
	Kind      string   `json:"kind"`
	Decade    string   `json:"decade"`
	Resources int      `json:"resources"`          // the live resources in scope of the stratum
	Captures  []string `json:"captures,omitempty"` // the captures that the estimate uses
	Model     string   `json:"model,omitempty"`    // the model that AIC chose, or empty when there is no estimate
	N         float64  `json:"n,omitempty"`        // the estimate of the universe
	Low       float64  `json:"low,omitempty"`      // the 95% interval of the estimate
	High      float64  `json:"high,omitempty"`
	Recall    float64  `json:"recall,omitempty"`     // Resources over N, at most 1
	RecallLow float64  `json:"recall_low,omitempty"` // Resources over High, at most 1
	Reason    string   `json:"reason,omitempty"`     // why there is no estimate
}

// recalls computes the recall of each stratum of the live resources in scope. A resource with no date is in no stratum, because the census has no decade for it.
func recalls(rs []resource.Resource, cat *ingest.Catalogue) []Recall {
	type work struct{ sources []string }
	byStratum := map[census.Stratum][]work{}
	var sources []string
	for i := range rs {
		r := &rs[i]
		if !r.Live() || r.Scope == nil || (r.Scope.Value != resource.In && r.Scope.Value != resource.Border) {
			continue
		}
		s := census.Stratum{Kind: census.KindGroup(r.Kind), Decade: census.Decade(r.Date)}
		if s.Decade == "unknown" {
			continue
		}
		var w work
		for _, c := range r.Catalogue {
			if !slices.Contains(w.sources, c.Source) {
				w.sources = append(w.sources, c.Source)
			}
			if !slices.Contains(sources, c.Source) {
				sources = append(sources, c.Source)
			}
		}
		byStratum[s] = append(byStratum[s], w)
	}
	captureOf := census.Captures(sources, cat.Lineage())
	var out []Recall
	for _, s := range slices.SortedFunc(maps.Keys(byStratum), func(a, b census.Stratum) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.Decade, b.Decade))
	}) {
		ws := byStratum[s]
		rec := Recall{Kind: s.Kind, Decade: s.Decade, Resources: len(ws)}
		held := map[string]int{}
		for _, w := range ws {
			seen := map[string]bool{}
			for _, src := range w.sources {
				if c := captureOf[src]; !seen[c] {
					seen[c] = true
					held[c]++
				}
			}
		}
		caps := slices.SortedFunc(maps.Keys(held), func(a, b string) int { return cmp.Or(cmp.Compare(held[b], held[a]), cmp.Compare(a, b)) })
		caps = caps[:min(len(caps), MaxCaptures)]
		slices.Sort(caps)
		rec.Captures = caps
		if len(caps) < 3 {
			rec.Reason = "fewer than three captures hold works of the stratum"
			out = append(out, rec)
			continue
		}
		h := census.NewHistories(len(caps))
		for _, w := range ws {
			mask := 0
			for _, src := range w.sources {
				if i := slices.Index(caps, captureOf[src]); i >= 0 {
					mask |= 1 << i
				}
			}
			h[mask]++
		}
		est, ok := census.EstimateLogLinear(h, nil, Seed)
		if !ok {
			rec.Reason = "no model converges"
			out = append(out, rec)
			continue
		}
		rec.Model, rec.N, rec.Low, rec.High = est.Model, round(est.N), round(est.CI.Low), round(est.CI.High)
		n := float64(rec.Resources)
		rec.Recall = round(min(1, n/max(est.N, n)))
		rec.RecallLow = round(min(1, n/max(est.CI.High, n)))
		out = append(out, rec)
	}
	return out
}
