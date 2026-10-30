// Package status computes the measures of the catalogue of resources and writes the report of RS11.8. This version measures the coverage of each source (RS11.2), the decision open count (RS9.6) and the curriculum closure (RS11.6). It also computes the tier and the depth of each resource with package depth and gives the depth by tier (RS1.6). It gives the recall by stratum from the capture histories of the catalogues (RS11.4). The reference closure comes in RS-M4.
package status

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/courses"
	"github.com/tamnd/math2-agent/resource/depth"
	"github.com/tamnd/math2-agent/resource/institutions"
)

// The files of the report.
const (
	ReportFile = "reports/resources.md"
	JSONFile   = "reports/resources.json"
)

// MaxList is the most records that a list of the report gives. The JSON file has the same lists.
const MaxList = 1000

// Coverage is the coverage of one source (RS11.2): the records that map to a resource with an outcome, over the records that the import read. A rejected record counts in the records that it read.
type Coverage struct {
	Source   string         `json:"source"`
	Snapshot string         `json:"snapshot,omitempty"`
	Read     int            `json:"read"`
	Rejected int            `json:"rejected"`
	Mapped   int            `json:"mapped"`
	Coverage float64        `json:"coverage"`
	Reasons  map[string]int `json:"reasons,omitempty"` // the count of the rejected records by reason
	Missing  []string       `json:"missing,omitempty"` // the native ids of the records with no resource, at most MaxList
}

// Open is the decision open count of RS9.6: the live resources that wait for a decision of a person, by reason.
type Open struct {
	Total    int            `json:"total"`
	ByReason map[string]int `json:"by_reason,omitempty"`
}

// Missing is one text of a curriculum with no live resource.
type Missing struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// Closure is the curriculum closure of RS11.6: the texts that the curricula name, and those with no live resource. The target is 0.
type Closure struct {
	Texts   int       `json:"texts"`
	Missing []Missing `json:"missing,omitempty"`
}

// Report holds the measures of one build.
type Report struct {
	Day       string          `json:"day"`
	Resources int             `json:"resources"` // the live resources
	Coverage  []Coverage      `json:"coverage"`
	Open      Open            `json:"open"`
	Closure   Closure         `json:"closure"`
	Recall    []Recall        `json:"recall"`
	Depth     []Tier          `json:"depth"`
	Late      []string        `json:"late,omitempty"` // the resources more than depth.GapDays below their target (rule H13)
	Courses   courses.Numbers `json:"courses"`        // the four numbers of RS5.8
}

// Tier is the depth of the resources of one tier. ByDepth counts the resources at each depth from -1 (below D0) to 4, at the index depth+1.
type Tier struct {
	Tier    string `json:"tier"`
	Target  int    `json:"target"`
	ByDepth [6]int `json:"by_depth"`
	Below   int    `json:"below"` // the depth gap: the resources below the target
}

// Build computes the report of the tree at root on the day, YYYY-MM-DD.
func Build(root, day string) (*Report, error) {
	rs, err := resource.ReadResources(root)
	if err != nil {
		return nil, err
	}
	rep := &Report{Day: day, Open: Open{ByReason: map[string]int{}}}
	mapped := map[string]bool{} // the keys source:native_id of the records of a resource with an outcome
	byID := map[string]*resource.Resource{}
	for i := range rs {
		r := &rs[i]
		byID[r.ID] = r
		if r.Live() {
			rep.Resources++
		}
		if r.Scope != nil && r.Scope.Value != "" {
			for _, c := range r.Catalogue {
				mapped[c.Source+":"+c.NativeID] = true
			}
		}
		if r.Live() && r.Scope != nil && r.Scope.Value == resource.Deferred && r.Scope.Until != "" && r.Scope.Until <= day {
			rep.Open.Total++
			rep.Open.ByReason[cmp.Or(r.Scope.Reason, "no reason")]++
		}
	}
	recs, err := cluster.ReadRecords(root)
	if err != nil {
		return nil, err
	}
	bySource := map[string]*Coverage{}
	get := func(s string) *Coverage {
		if bySource[s] == nil {
			bySource[s] = &Coverage{Source: s}
		}
		return bySource[s]
	}
	for _, r := range recs {
		c := get(r.Source)
		c.Read++
		c.Snapshot = r.Snapshot
		if mapped[r.Key()] {
			c.Mapped++
		} else if len(c.Missing) < MaxList {
			c.Missing = append(c.Missing, r.NativeID)
		}
	}
	rejected, err := filepath.Glob(filepath.Join(root, cluster.RejectedDir, "*.md"))
	if err != nil {
		return nil, err
	}
	for _, path := range rejected {
		source := strings.TrimSuffix(filepath.Base(path), ".md")
		xs, err := cluster.ReadRejects(root, source)
		if err != nil {
			return nil, err
		}
		c := get(source)
		for _, x := range xs {
			c.Read++
			c.Rejected++
			if c.Reasons == nil {
				c.Reasons = map[string]int{}
			}
			c.Reasons[x.Reason]++
		}
	}
	for _, s := range slices.Sorted(maps.Keys(bySource)) {
		c := bySource[s]
		if c.Read > 0 {
			c.Coverage = round(float64(c.Mapped) / float64(c.Read))
		}
		rep.Coverage = append(rep.Coverage, *c)
	}
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		return nil, err
	}
	rep.Recall = recalls(rs, cat)
	texts, err := resource.ReadTexts(root)
	if err != nil {
		return nil, err
	}
	rep.Closure.Texts = len(texts)
	named := map[string]bool{}
	for _, t := range texts {
		id, _, _ := strings.Cut(t.Resource, "/")
		r := byID[id]
		why := ""
		switch {
		case t.Resource == "":
			why = "no resource id"
		case r == nil:
			why = "the resource " + id + " is not in the catalogue"
		case !r.Live():
			why = "the resource " + id + " is a duplicate of " + r.Scope.Target
		}
		if why != "" && len(rep.Closure.Missing) < MaxList {
			rep.Closure.Missing = append(rep.Closure.Missing, Missing{t.File, t.Line, t.Name, why})
		}
		if why == "" {
			named[id] = true
		}
	}
	rows, err := depth.Build(root, day, rs, named)
	if err != nil {
		return nil, err
	}
	byTier := map[string]*Tier{}
	for _, row := range rows {
		t := byTier[row.Tier]
		if t == nil {
			t = &Tier{Tier: row.Tier, Target: row.Target}
			byTier[row.Tier] = t
		}
		t.ByDepth[min(max(row.Depth, -1), 4)+1]++
		if row.Depth < row.Target {
			t.Below++
		}
		if row.Late(day) && len(rep.Late) < MaxList {
			rep.Late = append(rep.Late, row.ID)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(byTier)) {
		rep.Depth = append(rep.Depth, *byTier[k])
	}
	ins, err := institutions.Read(root)
	if err != nil {
		return nil, err
	}
	rep.Courses = courses.Count(rs, ins)
	return rep, nil
}

func round(f float64) float64 { return float64(int(f*10000+0.5)) / 10000 }

// Write writes the report to ReportFile and JSONFile in the tree at root.
func (rep *Report) Write(root string) error {
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "reports"), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, JSONFile), append(data, '\n'), 0o644); err != nil { // #nosec G306 -- reports are public text in a git tree.
		return err
	}
	return os.WriteFile(filepath.Join(root, ReportFile), rep.Markdown(), 0o644) // #nosec G306 -- reports are public text in a git tree.
}

// Markdown gives the report as Markdown, in the sections of RS11.8.
func (rep *Report) Markdown() []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# The catalogue of resources\n\nThe report of %s. The catalogue has %d live resources. The sections follow RS11.8. The sections 5, 8 and 9 are not measured yet.\n", rep.Day, rep.Resources)
	fmt.Fprintf(&b, "\n## 1. The measures\n\n| Measure | Value | Target | State |\n|---|---:|---:|---|\n")
	gaps := 0
	for _, c := range rep.Coverage {
		if c.Coverage < 1 {
			gaps++
		}
	}
	fmt.Fprintf(&b, "| sources below 100%% coverage | %d | 0 | %s |\n", gaps, state(gaps == 0 && len(rep.Coverage) > 0))
	fmt.Fprintf(&b, "| curriculum works with no resource | %d | 0 | %s |\n", len(rep.Closure.Missing), state(len(rep.Closure.Missing) == 0))
	fmt.Fprintf(&b, "| open decisions | %d | 0 | %s |\n", rep.Open.Total, state(rep.Open.Total == 0))
	t1 := 0
	for _, t := range rep.Depth {
		if t.Tier == depth.T1 {
			t1 = t.Below
		}
	}
	below, unmeasured := Unmet(rep.Recall)
	fmt.Fprintf(&b, "| strata below the recall target | %d | 0 | %s |\n", len(below), state(len(below) == 0 && len(unmeasured) == 0 && len(rep.Recall) > 0))
	fmt.Fprintf(&b, "| depth gap of tier T1 | %d | 0 | %s |\n", t1, state(t1 == 0))
	fmt.Fprintf(&b, "\n## 2. Coverage by source\n\n")
	if len(rep.Coverage) == 0 {
		fmt.Fprintf(&b, "The tree has no source records. Run `m2 res import` for each source.\n")
	} else {
		fmt.Fprintf(&b, "| Source | Read | Rejected | Mapped | Coverage |\n|---|---:|---:|---:|---:|\n")
		for _, c := range rep.Coverage {
			fmt.Fprintf(&b, "| %s | %d | %d | %d | %.2f%% |\n", c.Source, c.Read, c.Rejected, c.Mapped, 100*c.Coverage)
		}
		for _, c := range rep.Coverage {
			if c.Rejected == 0 && len(c.Missing) == 0 {
				continue
			}
			fmt.Fprintf(&b, "\n### %s\n", c.Source)
			for _, r := range slices.Sorted(maps.Keys(c.Reasons)) {
				fmt.Fprintf(&b, "\n- rejected, %s: %d", r, c.Reasons[r])
			}
			if c.Rejected > 0 {
				b.WriteString("\n")
			}
			if n := c.Read - c.Rejected - c.Mapped; n > 0 {
				fmt.Fprintf(&b, "\n%d records have no resource with an outcome. The first of them: %s.\n", n, strings.Join(first(c.Missing, 20), ", "))
			}
		}
	}
	fmt.Fprintf(&b, "\n## 3. Open decisions\n\n")
	if rep.Open.Total == 0 {
		fmt.Fprintf(&b, "No decision is open.\n")
	} else {
		fmt.Fprintf(&b, "%d live resources are deferred past their date.\n\n| Reason | Count |\n|---|---:|\n", rep.Open.Total)
		for _, r := range slices.Sorted(maps.Keys(rep.Open.ByReason)) {
			fmt.Fprintf(&b, "| %s | %d |\n", r, rep.Open.ByReason[r])
		}
	}
	fmt.Fprintf(&b, "\n## 4. Recall by stratum\n\n")
	if len(rep.Recall) == 0 {
		fmt.Fprintf(&b, "No live resource in scope has a date.\n")
	} else {
		fmt.Fprintf(&b, "The estimate of each stratum is a log-linear model of the capture histories, with the catalogues that share lineage merged into one capture. The interval is the 95%% interval of a bootstrap. A stratum uses at most %d captures.\n\n| Kind | Decade | Resources | Captures | Model | Estimate | Interval | Recall | Recall at least | Target |\n|---|---|---:|---|---|---:|---|---:|---:|---:|\n", MaxCaptures)
		for _, r := range rep.Recall {
			if r.Model == "" {
				fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | | | | | %s |\n", r.Kind, r.Decade, r.Resources, strings.Join(r.Captures, ", "), r.Reason, target(r))
				continue
			}
			fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %.0f | %.0f to %.0f | %.3f | %.3f | %s |\n", r.Kind, r.Decade, r.Resources, strings.Join(r.Captures, ", "), r.Model, r.N, r.Low, r.High, r.Recall, r.RecallLow, target(r))
		}
		fmt.Fprintf(&b, "\n%d strata are below their target (rule H12), and %d strata with a target have no estimate.", len(below), len(unmeasured))
		if len(below) > 0 {
			fmt.Fprintf(&b, " Below the target: %s.", strings.Join(strata(below), ", "))
		}
		if len(unmeasured) > 0 {
			fmt.Fprintf(&b, " With no estimate: %s.", strings.Join(strata(unmeasured), ", "))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "\n## 6. Curriculum closure\n\nThe curricula name %d texts, and %d of them have no live resource (rule H15).\n", rep.Closure.Texts, len(rep.Closure.Missing))
	if len(rep.Closure.Missing) > 0 {
		fmt.Fprintf(&b, "\n| Text | Where | Why |\n|---|---|---|\n")
		for _, m := range rep.Closure.Missing {
			fmt.Fprintf(&b, "| %s | %s:%d | %s |\n", strings.ReplaceAll(m.Name, "|", "\\|"), m.File, m.Line, m.Reason)
		}
	}
	fmt.Fprintf(&b, "\n## 7. Depth by tier\n\n")
	if len(rep.Depth) == 0 {
		fmt.Fprintf(&b, "No resource has the scope in or border.\n")
	} else {
		fmt.Fprintf(&b, "| Tier | Target | below D0 | D0 | D1 | D2 | D3 | D4 | Below the target |\n|---|---|---:|---:|---:|---:|---:|---:|---:|\n")
		for _, t := range rep.Depth {
			fmt.Fprintf(&b, "| %s | D%d", t.Tier, t.Target)
			for _, n := range t.ByDepth {
				fmt.Fprintf(&b, " | %d", n)
			}
			fmt.Fprintf(&b, " | %d |\n", t.Below)
		}
		fmt.Fprintf(&b, "\n%d resources are more than %d days below their target depth (rule H13).", len(rep.Late), depth.GapDays)
		if len(rep.Late) > 0 {
			fmt.Fprintf(&b, " The first of them: %s.", strings.Join(first(rep.Late, 20), ", "))
		}
		b.WriteString("\n")
	}
	n := rep.Courses
	fmt.Fprintf(&b, "\n## Courses (RS5.8)\n\n| Number | Value |\n|---|---:|\n")
	fmt.Fprintf(&b, "| 1. courses of the I1 institutions and platforms | %d |\n| 1. offerings of those courses | %d |\n", n.I1Courses, n.I1Offerings)
	fmt.Fprintf(&b, "| 2. courses of the I2 and I3 institutions | %d |\n| 3. courses that a resource of another kind names | %d |\n", n.I2I3Courses, n.Named)
	fmt.Fprintf(&b, "| 4. estimate of the courses of the I4 institutions | %.0f (%.0f to %.0f) |\n", n.I4.Courses, n.I4.Low, n.I4.High)
	fmt.Fprintf(&b, "\nThe fourth number is an estimate, not a count (RS11.4.4). It is the sum, over the I4 institutions, of the median count of courses of the I2 institutions of the same level and country, and the interval is the sum of the quartiles. %d of the %d I4 institutions have such a group. The register has no student count, so the groups have no size band.\n", n.I4.Estimated, n.I4.Institutions)
	return b.Bytes()
}

func state(ok bool) string {
	if ok {
		return "met"
	}
	return "not met"
}

// target gives the target of a stratum for the report, with the mark below when the stratum is below it.
func target(r Recall) string {
	switch {
	case r.Target == 0:
		return "none"
	case r.Below:
		return fmt.Sprintf("%.2f, below", r.Target)
	}
	return fmt.Sprintf("%.2f", r.Target)
}

func strata(rs []Recall) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Kind+" "+r.Decade)
	}
	return out
}

func first(xs []string, n int) []string { return xs[:min(n, len(xs))] }
