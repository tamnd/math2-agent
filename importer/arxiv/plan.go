package arxiv

import (
	"fmt"
	"slices"

	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/queue"
)

// Tier is the planning numbers of one tier of the passes (AX7.8), for each unit, from the table of AX13.3.
type Tier struct {
	Name   string
	Passes string
	Unit   string // paper, item or proof
	Est    queue.Estimate
}

// Tiers are the model tiers of AX7.8. T0 calls no model, so it has no plan.
// T1 and T4 are measured in the MSC 20 pilot (#256, claude-haiku-5-5, input with the cache):
// T1 is 22k tokens in and 5k out for each paper, T4 is 11k in and 1k out for each proof.
// The high cost of T4 stays the estimate for a model of tier 2.
var Tiers = map[string]Tier{
	"T1": {Name: "T1", Passes: "classify, concepts", Unit: "paper", Est: queue.Estimate{Tier: 1, TokensIn: 22000, TokensOut: 5000, CostLow: 0.005, CostHigh: 0.006, CostKnown: true}},
	"T2": {Name: "T2", Passes: "prose and its check", Unit: "paper", Est: queue.Estimate{Tier: 2, TokensIn: 35000, TokensOut: 4000, CostLow: 0.05, CostHigh: 0.06, CostKnown: true}}, // #nosec G101 -- Passes names the passes of a tier, not a password
	"T3": {Name: "T3", Passes: "unfold and its check", Unit: "item", Est: queue.Estimate{Tier: 2, TokensIn: 17000, TokensOut: 900, CostLow: 0.03, CostHigh: 0.03, CostKnown: true}},  // #nosec G101 -- Passes names the passes of a tier, not a password
	"T4": {Name: "T4", Passes: "proof, summary", Unit: "proof", Est: queue.Estimate{Tier: 2, TokensIn: 11000, TokensOut: 1000, CostLow: 0.007, CostHigh: 0.1, CostKnown: true}},
}

// TierNames returns the names of the tiers with a plan.
func TierNames() []string {
	return []string{"T1", "T2", "T3", "T4"}
}

// PlanUnderstand estimates the cost of one tier on the arXiv mentions (AX7.8, AX13.3). With msc, only the mentions with a code in that area at two digits count. It calls no model. The units are:
//
//   - T1: each paper.
//   - T2: each paper with an open or share-alike-nc version.
//   - T3: each statement and definition whose row has a spread of 2 or more or holds more than one paper.
//   - T4: each proof of a statement of T3, until AX9.1 selects the nodes.
func PlanUnderstand(tier string, rows []inventory.Row, ms []inventory.Mention, msc string) (queue.Plan, error) {
	t, ok := Tiers[tier]
	if !ok {
		return queue.Plan{}, fmt.Errorf("the tier %q has no plan; the tiers are %v", tier, TierNames())
	}
	block := ""
	if msc != "" {
		block = msc[:min(2, len(msc))]
	}
	in := func(m *inventory.Mention) bool {
		return block == "" || slices.Contains(inventory.Blocks(m), block)
	}
	wide := map[string]bool{} // the mentions of the rows of T3
	byID := map[string]*inventory.Mention{}
	for i := range ms {
		byID[ms[i].ID] = &ms[i]
	}
	for i := range rows {
		r := &rows[i]
		if r.Outcome != nil && r.Outcome.Value == inventory.OutcomeMerged {
			continue
		}
		papers := map[string]bool{}
		for _, id := range r.Mentions {
			if m := byID[id]; m != nil && m.Source == Source {
				papers[PaperOf(id)] = true
			}
		}
		if r.Spread >= 2 || len(papers) > 1 {
			for _, id := range r.Mentions {
				wide[id] = true
			}
		}
	}
	units := map[string]bool{}
	for i := range ms {
		m := &ms[i]
		if m.Source != Source || !in(m) {
			continue
		}
		switch tier {
		case "T1":
			units[PaperOf(m.ID)] = true
		case "T2":
			if m.Class == "open" || m.Class == "share-alike-nc" {
				units[PaperOf(m.ID)] = true
			}
		case "T3":
			if (m.Kind == "statement" || m.Kind == "definition") && wide[m.ID] {
				units[m.ID] = true
			}
		case "T4":
			if m.Kind == "proof" && wide[m.AttachedTo] {
				units[m.ID] = true
			}
		}
	}
	n := len(units)
	e := t.Est
	p := queue.Plan{Queue: "understand " + tier, Jobs: n, Per: e}
	p.TokensIn, p.TokensOut = n*e.TokensIn, n*e.TokensOut
	p.CostLow, p.CostHigh = float64(n)*e.CostLow, 2*float64(n)*e.CostHigh
	p.Notes = append(p.Notes, fmt.Sprintf("the passes are %s, one job for each %s", t.Passes, t.Unit))
	p.Notes = append(p.Notes, "the review hours of the queues of the passes are not known until the pilot measures them (AX13.4)")
	if tier == "T4" {
		p.Notes = append(p.Notes, "AX9.1 does not select the nodes yet, so the proofs of the statements of T3 are counted")
	}
	return p, nil
}
