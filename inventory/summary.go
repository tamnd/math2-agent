package inventory

import "time"

// Summary is the counts of an inventory, as m2 corpus status and the site show them.
type Summary struct {
	Rows     int            `json:"rows"`
	Open     int            `json:"open"`     // the open count (corpus spec K2.1)
	Mentions int            `json:"mentions"` // the number of mentions
	Layers   map[string]int `json:"layers"`   // the rows of each layer
	OpenBy   map[string]int `json:"open_by"`  // the open count of each layer. Corpus spec K14.7 asks that the count of the research layer goes down each month.
	Kinds    map[string]int `json:"kinds"`    // the rows of each kind
	Outcomes map[string]int `json:"outcomes"` // the rows of each outcome, and open
	Sources  map[string]int `json:"sources"`  // the mentions of each source
	MSC      map[string]int `json:"msc"`      // the rows of each MSC code at two digits, and none
}

// Summarize counts the rows and the mentions on the day. A merged row is not counted, because its mentions are in another row.
func Summarize(rows []Row, ms []Mention, day time.Time) *Summary {
	s := &Summary{Mentions: len(ms), Layers: map[string]int{}, OpenBy: map[string]int{}, Kinds: map[string]int{}, Outcomes: map[string]int{}, Sources: map[string]int{}, MSC: map[string]int{}}
	for i := range rows {
		r := &rows[i]
		if r.Outcome != nil && r.Outcome.Value == OutcomeMerged {
			s.Outcomes[OutcomeMerged]++
			continue
		}
		s.Rows++
		if r.IsOpen(day) {
			s.Open++
			s.Outcomes["open"]++
			s.OpenBy[r.Layer]++
		} else {
			s.Outcomes[r.Outcome.Value]++
		}
		s.Layers[r.Layer]++
		s.Kinds[r.Kind]++
		seen := map[string]bool{}
		for _, c := range r.MSC {
			if len(c) >= 2 && !seen[c[:2]] {
				seen[c[:2]] = true
				s.MSC[c[:2]]++
			}
		}
		if len(seen) == 0 {
			s.MSC["none"]++
		}
	}
	for i := range ms {
		s.Sources[ms[i].Source]++
	}
	return s
}
