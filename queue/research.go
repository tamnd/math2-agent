package queue

import (
	"fmt"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/inventory"
)

// WriteResearch is the queue of corpus arxiv AX9.1: the rows of the research layer that become research nodes.
const WriteResearch = "write-research"

// The reasons of AX9.1 in their order of priority. Reason 1, closure, needs the uses edges of the nodes to rows, and the input of a queue has no nodes, so this version does not give it.
const (
	reasonRequest = 2
	reasonNamed   = 3
	reasonSpread  = 4
	reasonOpen    = 5
	reasonFormal  = 6
)

var reasonNames = map[int]string{reasonRequest: "request", reasonNamed: "named", reasonSpread: "spread", reasonOpen: "open problem", reasonFormal: "formal"}

// writtenKinds are the kinds of the rows that a research node can state.
var writtenKinds = map[string]bool{"statement": true, "definition": true, "conjecture": true, "problem": true}

// problemSources are the lists of problems of K05.
var problemSources = map[string]bool{"erdos_problems": true, "formal_conjectures": true}

// writeResearchItems gives each row of the research layer with the outcome record, or with no outcome, that one of the reasons of AX9.1 selects. The priority is the reason first, the lowest reason number first, and then the number of papers that cite the row.
func writeResearchItems(in Input) []Item {
	asked := map[string]bool{}
	for _, c := range in.Asked {
		if len(c) >= 2 {
			asked[c[:2]] = true
		}
	}
	byID := make(map[string]*inventory.Mention, len(in.Mentions))
	for i := range in.Mentions {
		byID[in.Mentions[i].ID] = &in.Mentions[i]
	}
	rowOf := map[string]string{}
	for i := range in.Rows {
		for _, m := range in.Rows[i].Mentions {
			rowOf[m] = in.Rows[i].ID
		}
	}
	// citers holds, for each row, the papers whose items name it (the name method of AX6.2) or state it as a cited result.
	citers := map[string]map[string]bool{}
	cite := func(row, paper string) {
		if row == "" || paper == "" {
			return
		}
		if citers[row] == nil {
			citers[row] = map[string]bool{}
		}
		citers[row][paper] = true
	}
	for i := range in.Mentions {
		m := &in.Mentions[i]
		p := paperOf(m)
		if m.Structure != nil {
			for _, r := range m.Structure.Names {
				cite(r, p)
			}
		}
		for _, c := range m.CitesResult {
			cite(rowOf[c.To], p)
		}
	}
	out := []Item{}
	for i := range in.Rows {
		r := &in.Rows[i]
		if r.Layer != inventory.Research || !writtenKinds[r.Kind] || r.Outcome != nil && r.Outcome.Value != inventory.OutcomeRecord {
			continue
		}
		own := map[string]bool{}
		named := map[string]bool{}
		formal, listed := false, false
		for _, id := range r.Mentions {
			m := byID[id]
			if m == nil {
				continue
			}
			doc := m.Source
			if p := paperOf(m); p != "" {
				doc = p
				own[p] = true
			}
			if r.Name != "" && strings.EqualFold(m.Name, r.Name) {
				named[doc] = true
			}
			formal = formal || inventory.IsFormal(m.Source) || strings.HasPrefix(m.Source, "mathlib")
			listed = listed || problemSources[m.Source]
		}
		spread := 0
		for p := range citers[r.ID] {
			if !own[p] {
				spread++
			}
		}
		reason := 0
		switch {
		case slices.ContainsFunc(areas(r), func(a string) bool { return asked[a] }):
			reason = reasonRequest
		case len(named) >= 2:
			reason = reasonNamed
		case spread >= 3:
			reason = reasonSpread
		case (r.Kind == "conjecture" || r.Kind == "problem") && (spread >= 2 || listed):
			reason = reasonOpen
		case formal:
			reason = reasonFormal
		default:
			continue
		}
		goal := fmt.Sprintf("row %s is a research node at W1 (AX9.1 reason %d, %s", r.ID, reason, reasonNames[reason])
		if spread > 0 {
			goal += fmt.Sprintf(", %d papers cite it", spread)
		}
		out = append(out, Item{Target: r.ID, Name: r.Name, Kind: r.Kind, Goal: goal + ")", Priority: float64(10-reason)*1e6 + priority(spread)})
	}
	return out
}

// paperOf returns the arXiv id of the paper of a mention of the research layer, or "" for a mention of another source.
func paperOf(m *inventory.Mention) string {
	if !inventory.IsResearch(m.Source) {
		return ""
	}
	id, _, _ := strings.Cut(m.NativeID, "#")
	return id
}
