package inventory

import (
	"cmp"
	"maps"
	"math"
	"slices"
)

// RecallFloor is the estimated recall under which rule I06 reports an area (corpus spec K4.9).
const RecallFloor = 0.9

// Capture is the estimate of the total of one area from the overlap of two sources (corpus spec K4.7).
type Capture struct {
	A       string  `json:"a"`       // the first source
	B       string  `json:"b"`       // the second source
	InA     int     `json:"in_a"`    // the rows of the area with a mention of A
	InB     int     `json:"in_b"`    // the rows of the area with a mention of B
	InBoth  int     `json:"in_both"` // the rows with a mention of each
	Total   float64 `json:"total"`   // the estimate of the number of items of the area
	Low     float64 `json:"low"`     // the lower end of the 95% interval of Total
	High    float64 `json:"high"`    // the upper end of the 95% interval of Total
	Default bool    `json:"default,omitempty"`
}

// Area is the estimated recall of the inventory for one MSC code at two digits.
type Area struct {
	MSC      string    `json:"msc"`                // the two digits, for example 11
	Rows     int       `json:"rows"`               // the rows of the area
	Sources  int       `json:"sources"`            // the sources with a mention of a row of the area
	Method   string    `json:"method"`             // lincoln-petersen, or none when no two independent sources cover the area
	Recall   *float64  `json:"recall"`             // Rows divided by the estimate of the total, at most 1. Nil when the method is none.
	Low      *float64  `json:"low"`                // the recall at the upper end of the interval of the total
	High     *float64  `json:"high"`               // the recall at the lower end of the interval of the total
	Captures []Capture `json:"captures,omitempty"` // the estimate of each pair of independent sources. The pair of the area has Default.
}

// Family returns the family of a source for capture and recapture. Two sources of one family are not independent, so their overlap gives no estimate: the lists of Mathlib come from the community of Mathlib, the two copies of the 1000+ list come from one list, and all the models count as one capture occasion (corpus spec K7.4).
func Family(source string) string {
	if f, ok := families[source]; ok {
		return f
	}
	return source
}

var families = map[string]string{
	"mathlib":           "mathlib",
	"mathlib_undergrad": "mathlib",
	"mathlib_overview":  "mathlib",
	"mathlib_100":       "mathlib",
	"mathlib_1000":      "thousand_plus",
	"thousand_plus":     "thousand_plus",
	"arxiv_results":     "arxiv",
}

// Estimate gives the estimated recall of the inventory for each MSC code at two digits of the rows, in the order of the codes (corpus spec K4.7). A row counts in each area of its MSC codes. A row that the clustering merged or split, or that is out of scope, is not an item, and it does not count. A source captures a row when the row has a mention of the source.
//
// For each pair of sources of different families with a row in the area, Estimate computes the Lincoln and Petersen estimate of the total, in the form of Chapman, which has no bias for small samples and is finite when the overlap is empty: (a+1)(b+1)/(m+1) - 1, where a and b are the rows of each source and m the rows of both. The interval is the estimate plus and minus 1.96 standard deviations, and it starts at no fewer than the a+b-m rows that the sources hold. The pair of the area is the pair with the largest overlap, because its estimate has the smallest variance, then the pair with the larger total, then the first pair by name.
func Estimate(rows []Row, ms []Mention) []Area {
	source := map[string]string{}
	model := map[string]bool{}
	for i := range ms {
		source[ms[i].ID] = ms[i].Source
		model[ms[i].ID] = FromModel(&ms[i])
	}
	type item map[string]bool // the families that capture a row, by source
	areas := map[string][]item{}
	for i := range rows {
		r := &rows[i]
		if r.Outcome != nil && slices.Contains([]string{OutcomeMerged, OutcomeSplit, OutcomeOutOfScope}, r.Outcome.Value) {
			continue
		}
		in := item{}
		for _, id := range r.Mentions {
			if s, ok := source[id]; ok {
				if model[id] {
					s = "models"
				}
				in[s] = true
			}
		}
		var codes []string
		for _, c := range r.MSC {
			if len(c) >= 2 {
				codes = append(codes, c[:2])
			}
		}
		slices.Sort(codes)
		for _, c := range slices.Compact(codes) {
			areas[c] = append(areas[c], in)
		}
	}
	var out []Area
	for _, code := range slices.Sorted(maps.Keys(areas)) {
		items := areas[code]
		count := map[string]int{}
		for _, in := range items {
			for s := range in {
				count[s]++
			}
		}
		srcs := slices.Sorted(maps.Keys(count))
		a := Area{MSC: code, Rows: len(items), Sources: len(srcs), Method: "none"}
		for i, sa := range srcs {
			for _, sb := range srcs[i+1:] {
				if Family(sa) == Family(sb) {
					continue
				}
				c := Capture{A: sa, B: sb, InA: count[sa], InB: count[sb]}
				for _, in := range items {
					if in[sa] && in[sb] {
						c.InBoth++
					}
				}
				c.Total, c.Low, c.High = chapman(c.InA, c.InB, c.InBoth)
				a.Captures = append(a.Captures, c)
			}
		}
		if len(a.Captures) > 0 {
			best := slices.MinFunc(a.Captures, func(x, y Capture) int {
				return cmp.Or(cmp.Compare(y.InBoth, x.InBoth), cmp.Compare(y.Total, x.Total))
			})
			for i := range a.Captures {
				if a.Captures[i] == best {
					a.Captures[i].Default = true
					break
				}
			}
			a.Method = "lincoln-petersen"
			a.Recall = share(a.Rows, best.Total)
			a.Low = share(a.Rows, best.High)
			a.High = share(a.Rows, best.Low)
		}
		out = append(out, a)
	}
	return out
}

// chapman returns the estimate of Chapman of the total from two captures of a and b items with m items in both, and its 95% interval.
func chapman(a, b, m int) (total, low, high float64) {
	fa, fb, fm := float64(a), float64(b), float64(m)
	total = (fa+1)*(fb+1)/(fm+1) - 1
	v := (fa + 1) * (fb + 1) * (fa - fm) * (fb - fm) / ((fm + 1) * (fm + 1) * (fm + 2))
	d := 1.96 * math.Sqrt(v)
	return total, max(total-d, fa+fb-fm), total + d
}

// share returns n/total, at most 1.
func share(n int, total float64) *float64 {
	r := 1.0
	if total > 0 {
		r = min(1, float64(n)/total)
	}
	return &r
}
