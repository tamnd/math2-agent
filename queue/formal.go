package queue

import (
	"fmt"

	"github.com/tamnd/math2-agent/inventory"
)

// FormalResearch is the queue of corpus arxiv AX10.2: the research nodes that go to the formal track, in its order.
const FormalResearch = "formal-research"

// The reasons of AX10.2 in their order.
const (
	formalStated     = 1 // a formal source states the item
	formalOpen       = 2 // a conjecture or a problem with a spread of 2 or more
	formalDefinition = 3 // a definition that a research node uses
	formalLemma      = 4 // a lemma that the proof of a main theorem of its paper uses
	formalOther      = 5
)

var formalNames = map[int]string{formalStated: "a formal source states it", formalOpen: "open", formalDefinition: "a definition that research nodes use", formalLemma: "a lemma of a main theorem", formalOther: "other"}

// formalResearchItems gives each node of a row of the research layer, in the order of AX10.2. The kinds are those that get a formal outcome (status.FormalKinds). A row with the outcome record is not a node and does not go to the formal track. The input has no nodes, thus the queue reads the edges of the mentions: a definition that research nodes use is the row of a mention that a mention of another node row, or an attached proof of it, uses, and a lemma of a main theorem is a statement of the paper of a node row whose attached proof uses it, as the long-proof rule of AX9.4 makes it. The priority is the reason first, the lowest reason number first, and then the number of papers that cite the row. The job of reason 1 is an align run, because a formal statement exists (spec 10.2, "search before writing").
func formalResearchItems(in Input) []Item {
	byID, rowOf := index(in)
	citers := citersOf(in, rowOf)
	node := func(r *inventory.Row) bool {
		return r != nil && r.Layer == inventory.Research && r.Outcome != nil && r.Outcome.Value == inventory.OutcomeNode && len(r.Outcome.Target) > 0
	}
	used, lemma := map[string]bool{}, map[string]bool{}
	for i := range in.Rows {
		r := &in.Rows[i]
		if !node(r) {
			continue
		}
		for _, id := range r.Mentions {
			if m := byID[id]; m != nil && m.Structure != nil {
				for _, u := range m.Structure.Uses {
					if to := rowOf[u]; to != r.ID {
						used[to] = true
					}
				}
			}
		}
		for _, id := range r.Proofs {
			m := byID[id]
			if m == nil || m.Structure == nil {
				continue
			}
			for _, u := range m.Structure.Uses {
				if to := rowOf[u]; to != r.ID {
					used[to] = true
				}
				if l := byID[u]; l != nil && l.Kind == "statement" && paperOf(l) == paperOf(m) && rowOf[u] != r.ID {
					lemma[rowOf[u]] = true
				}
			}
		}
	}
	out := []Item{}
	for i := range in.Rows {
		r := &in.Rows[i]
		if !node(r) || !writtenKinds[r.Kind] {
			continue
		}
		stated := false
		for _, id := range r.Mentions {
			if m := byID[id]; m != nil && formalSource(m.Source) {
				stated = true
			}
		}
		spread := spreadOf(r, byID, citers)
		reason := formalOther
		switch {
		case stated:
			reason = formalStated
		case (r.Kind == "conjecture" || r.Kind == "problem") && max(spread, r.Spread) >= 2:
			reason = formalOpen
		case r.Kind == "definition" && used[r.ID]:
			reason = formalDefinition
		case r.Kind == "statement" && lemma[r.ID]:
			reason = formalLemma
		}
		tag := r.Outcome.Target[0]
		goal := fmt.Sprintf("node %s of row %s has a formal outcome (AX10.2 reason %d, %s", tag, r.ID, reason, formalNames[reason])
		if reason == formalStated {
			goal += ", thus an align run"
		}
		if spread > 0 {
			goal += fmt.Sprintf(", %d papers cite it", spread)
		}
		out = append(out, Item{Target: tag, Name: r.Name, Kind: r.Kind, Goal: goal + ")", Priority: float64(10-reason)*1e6 + priority(spread)})
	}
	return out
}
