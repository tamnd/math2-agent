// Package courses gives the four numbers of RS5.8 and the course estimate of RS11.4.4. A course is a live resource of the kind course. Its institution is the ROR id before the first colon of its course id (RS5.2), or the platform whose source gave its catalogue record.
package courses

import (
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/institutions"
)

// Estimate is the estimate of the courses of the I4 institutions (RS11.4.4). It is a model, not a count.
type Estimate struct {
	Institutions int     `json:"institutions"` // the live I4 institutions
	Estimated    int     `json:"estimated"`    // the I4 institutions that have a group of I2 institutions to estimate from
	Courses      float64 `json:"courses"`      // the sum of the medians
	Low          float64 `json:"low"`          // the sum of the lower quartiles
	High         float64 `json:"high"`         // the sum of the upper quartiles
}

// Numbers are the four numbers of RS5.8.
type Numbers struct {
	I1Courses   int      `json:"i1_courses"`   // 1: the courses of the I1 institutions and platforms
	I1Offerings int      `json:"i1_offerings"` // 1: the offerings of those courses
	I2I3Courses int      `json:"i2_i3_courses"`
	Named       int      `json:"named"` // 3: the courses that a resource of another kind names
	I4          Estimate `json:"i4"`    // 4: the estimate
}

// Count gives the four numbers of the live resources rs and the register ins.
func Count(rs []resource.Resource, ins []institutions.Institution) Numbers {
	tier := map[string]string{} // the tier of each ROR id and of each platform source
	for _, in := range ins {
		if in.Status == "withdrawn" {
			continue
		}
		if in.ROR != "" {
			tier[in.ROR] = in.Tier
		} else if in.Level == institutions.Platform && in.Catalogue != nil && in.Catalogue.Format != "" {
			tier["source:"+in.Catalogue.Format] = in.Tier
		}
	}
	courses := map[string]bool{}
	for _, r := range rs {
		if r.Live() && r.Kind == "course" {
			courses[r.ID] = true
		}
	}
	named := map[string]bool{}
	for _, r := range rs {
		if !r.Live() || r.Kind == "course" || r.Kind == "course-material" {
			continue
		}
		for _, rel := range r.Relations {
			if id, _, _ := strings.Cut(rel.To, "/"); courses[id] {
				named[id] = true
			}
		}
	}
	var n Numbers
	n.Named = len(named)
	perROR := map[string]int{}
	for _, r := range rs {
		if !r.Live() || r.Kind != "course" {
			continue
		}
		t := ""
		for _, ror := range rors(r) {
			perROR[ror]++
			t = best(t, tier[ror])
		}
		for _, c := range r.Catalogue {
			t = best(t, tier["source:"+c.Source])
		}
		switch t {
		case institutions.I1:
			n.I1Courses++
			n.I1Offerings += len(r.Expressions)
		case institutions.I2, institutions.I3:
			n.I2I3Courses++
		}
	}
	n.I4 = estimate(ins, perROR)
	return n
}

// rors gives the ROR ids of the course ids of r, each once.
func rors(r resource.Resource) []string {
	var out []string
	for _, id := range r.IDs["course"] {
		if ror, _, ok := strings.Cut(id, ":"); ok && !slices.Contains(out, ror) {
			out = append(out, ror)
		}
	}
	return out
}

// best gives the lower tier of a and b, so that a course of an I1 platform and an I2 university counts as I1. An empty tier is the highest.
func best(a, b string) string {
	if a == "" || (b != "" && b < a) {
		return b
	}
	return a
}

// estimate gives, for each live I4 institution with a ROR id, the median count of courses of the I2 institutions of the same level and country, with the quartiles as the interval. When the country has no I2 institution of the level, it uses the I2 institutions of the level in each country. The register has no student count, so the size band of RS11.4.4 is not in the group yet.
func estimate(ins []institutions.Institution, perROR map[string]int) Estimate {
	group := map[[2]string][]float64{}
	for _, in := range ins {
		if in.Status == "withdrawn" || in.ROR == "" || in.Tier != institutions.I2 {
			continue
		}
		c := float64(perROR[in.ROR])
		group[[2]string{in.Level, in.Country}] = append(group[[2]string{in.Level, in.Country}], c)
		group[[2]string{in.Level, ""}] = append(group[[2]string{in.Level, ""}], c)
	}
	for _, k := range slices.Collect(maps.Keys(group)) {
		slices.Sort(group[k])
	}
	var e Estimate
	for _, in := range ins {
		if in.Status == "withdrawn" || in.ROR == "" || in.Tier != institutions.I4 {
			continue
		}
		e.Institutions++
		xs := group[[2]string{in.Level, in.Country}]
		if len(xs) == 0 {
			xs = group[[2]string{in.Level, ""}]
		}
		if len(xs) == 0 {
			continue
		}
		e.Estimated++
		e.Courses += quantile(xs, 0.5)
		e.Low += quantile(xs, 0.25)
		e.High += quantile(xs, 0.75)
	}
	return e
}

// quantile gives the q quantile of the sorted xs, with linear interpolation.
func quantile(xs []float64, q float64) float64 {
	pos := q * float64(len(xs)-1)
	i := int(math.Floor(pos))
	if i+1 >= len(xs) {
		return xs[len(xs)-1]
	}
	return xs[i] + (pos-float64(i))*(xs[i+1]-xs[i])
}
