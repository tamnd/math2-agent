package inventory

import (
	"slices"
	"strings"
)

// NameCandidate is a row of another layer whose name is the name of a mention of one source (the name pass of corpus K4.3, arxiv AX8.2 and wikipedia WP8.2). The dedup judge decides each candidate, because the name of a variant can be the name of the textbook result.
type NameCandidate struct {
	Mention string `json:"mention"`
	Row     string `json:"row"`    // the row of the mention now
	Target  string `json:"target"` // the row of the other layer
	Name    string `json:"name"`
}

// nameBlock is the kind group of the name pass: statements and conjectures share a block (AX8.1).
func nameBlock(kind string) string {
	if kind == "conjecture" {
		return "statement"
	}
	return kind
}

// NameCandidates returns the name candidates of the mentions of the source. A candidate is a row with a mention of another source, in the block of the mention, that is the only row of the block with the name of the mention. A mention that is already in a row with a mention of another source has no candidate. With msc, only the blocks of that code at two digits are read.
func NameCandidates(rows []Row, ms []Mention, source, msc string) []NameCandidate {
	byID := map[string]*Mention{}
	for i := range ms {
		byID[ms[i].ID] = &ms[i]
	}
	type entry struct{ rows []string }
	names := map[string]*entry{} // the rows of another layer for each kind, block and name
	rowOf := map[string]string{}
	mixed := map[string]bool{}
	for i := range rows {
		r := &rows[i]
		if r.Outcome != nil && r.Outcome.Value == OutcomeMerged {
			continue
		}
		var other []*Mention
		for _, id := range r.Mentions {
			rowOf[id] = r.ID
			if m := byID[id]; m != nil && m.Source != source {
				other = append(other, m)
			}
		}
		if len(other) == 0 {
			continue
		}
		mixed[r.ID] = true
		keys := map[string]bool{}
		for _, m := range other {
			for _, n := range append([]string{m.Name}, m.Aliases...) {
				if k := NameKey(n); k != "" {
					for _, b := range Blocks(&Mention{MSC: r.MSC}) {
						keys[nameBlock(r.Kind)+"|"+b+"|"+k] = true
					}
				}
			}
		}
		for k := range keys {
			e := names[k]
			if e == nil {
				e = &entry{}
				names[k] = e
			}
			e.rows = append(e.rows, r.ID)
		}
	}
	var out []NameCandidate
	for i := range ms {
		m := &ms[i]
		if m.Source != source || m.Name == "" || m.Attached() || mixed[rowOf[m.ID]] {
			continue
		}
		var found []string
		// A row with no code is in each block, as in the name method.
		blocks := Blocks(m)
		if !slices.Contains(blocks, "") {
			blocks = append(blocks, "")
		}
		for _, b := range blocks {
			if msc != "" && b != "" && b != msc[:min(2, len(msc))] {
				continue
			}
			if e := names[nameBlock(m.Kind)+"|"+b+"|"+NameKey(m.Name)]; e != nil {
				found = append(found, e.rows...)
			}
		}
		slices.Sort(found)
		if found = slices.Compact(found); len(found) == 1 {
			out = append(out, NameCandidate{Mention: m.ID, Row: rowOf[m.ID], Target: found[0], Name: m.Name})
		}
	}
	slices.SortFunc(out, func(a, b NameCandidate) int { return strings.Compare(a.Mention, b.Mention) })
	return out
}
