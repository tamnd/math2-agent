// Package depth computes the tier and the depth of each resource (spec corpus resources RS1.5 and RS1.6). The build writes them to File. No person and no agent writes a depth. A person can move a resource down a tier with a decision of the kind tier.
package depth

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/ingest/textbook"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/scope"
)

// File holds the tier and the depth of each resource in scope or on the border.
const File = "build/resources/depth.md"

// GapDays is the time below the target depth after which rule H13 reports a resource.
const GapDays = 180

// The tiers of RS1.6.
const (
	T1 = "T1" // core: a work that a curriculum names, or an open textbook of K5.4 Table A
	T2 = "T2" // standard: a closed textbook or a survey
	T3 = "T3" // literature: a paper with an open manifestation
	T4 = "T4" // record: each other work in scope
	T5 = "T5" // course: a course offering
	T6 = "T6" // border: a resource with the scope border
)

// Targets are the target depths of the tiers.
var Targets = map[string]int{T1: 4, T2: 3, T3: 3, T4: 1, T5: 2, T6: 0}

// Row is the tier and the depth of one resource. Depth -1 means that the resource does not have the fields of D0. Below is the day from which the depth is below the target, or empty.
type Row struct {
	ID     string `json:"id"`
	Tier   string `json:"tier"`
	Depth  int    `json:"depth"`
	Target int    `json:"target"`
	Below  string `json:"below,omitempty"`
}

// Mentions are the counts of the mentions of one resource.
type Mentions struct {
	Total   int // the mentions whose field resource names the resource
	WithAt  int // of them, those with a structured locator
	Decided int // of them, those in an inventory row with an outcome other than deferred
}

// Input is what Compute reads.
type Input struct {
	Resources []resource.Resource
	Named     map[string]bool     // the resources that a curriculum names
	OpenTexts map[string]bool     // the sources of the open textbooks of K5.4 Table A
	Mentions  map[string]Mentions // by resource id
	Previous  map[string]Row      // the rows of the last build
	Decisions map[string]string   // the tier that a person set, by resource id
	Day       string              // YYYY-MM-DD
}

// Compute gives the row of each live resource with the scope in or border, in the order of the ids.
func Compute(in Input) []Row {
	var out []Row
	for i := range in.Resources {
		r := &in.Resources[i]
		if !r.Live() || r.Scope == nil || (r.Scope.Value != resource.In && r.Scope.Value != resource.Border) {
			continue
		}
		prev, had := in.Previous[r.ID]
		t := Tier(r, in.Named[r.ID], in.OpenTexts)
		switch p := in.Decisions[r.ID]; {
		case p != "":
			t = p
		case had && t != T6 && prev.Tier != T6 && Targets[prev.Tier] > Targets[t]:
			t = prev.Tier // a tier never moves down by itself
		}
		row := Row{ID: r.ID, Tier: t, Depth: Depth(r, in.Mentions[r.ID]), Target: Targets[t]}
		if row.Depth < row.Target {
			row.Below = in.Day
			if had && prev.Below != "" {
				row.Below = prev.Below
			}
		}
		out = append(out, row)
	}
	slices.SortFunc(out, func(a, b Row) int { return cmp.Compare(a.ID, b.ID) })
	return out
}

// Tier gives the tier of a resource from its signals (RS1.6). The citation count of RS9.4 and the coverage of the extraction projects are not signals yet, so a book with many citations is T2 only when it is a textbook or a survey.
func Tier(r *resource.Resource, named bool, openTexts map[string]bool) string {
	switch {
	case r.Scope != nil && r.Scope.Value == resource.Border:
		return T6
	case named || slices.ContainsFunc(r.Catalogue, func(c resource.CatalogueRecord) bool { return openTexts[c.Source] }):
		return T1
	case r.Kind == "course" || slices.ContainsFunc(r.Expressions, func(e resource.Expression) bool { return e.Kind == "offering" }):
		return T5
	case r.Kind == "textbook" || r.Kind == "survey" || slices.Contains(r.Genre, "survey"):
		return T2
	case (r.Kind == "article" || r.Kind == "preprint" || r.Kind == "conference-paper") && open(r):
		return T3
	}
	return T4
}

func open(r *resource.Resource) bool {
	for _, e := range r.Expressions {
		for _, m := range e.Manifestations {
			if m.Access != nil && m.Access.Class == "open" {
				return true
			}
		}
	}
	return false
}

// Depth gives the depth of a resource (RS1.5). D1 does not check that the record has each id that the resolver can find, because the resolver adds them. D3 counts a resource whose mentions each have a structured locator. D4 counts a resource whose mentions are each in a row with an outcome.
func Depth(r *resource.Resource, m Mentions) int {
	if r.ID == "" || r.Kind == "" || r.Title == "" || len(r.Catalogue) == 0 {
		return -1
	}
	lang := slices.ContainsFunc(r.Expressions, func(e resource.Expression) bool { return e.Language != "" })
	access := true
	for _, e := range r.Expressions {
		for _, mf := range e.Manifestations {
			access = access && mf.Access != nil
		}
	}
	if len(r.Creators) == 0 || r.Date == "" || !lang || len(r.MSC) == 0 || !access {
		return 0
	}
	if !slices.ContainsFunc(r.Expressions, func(e resource.Expression) bool { return e.Structure != "" }) {
		return 1
	}
	if m.Total == 0 || m.WithAt < m.Total {
		return 2
	}
	if m.Decided < m.Total {
		return 3
	}
	return 4
}

// Late reports whether the row has been below its target for more than GapDays on the day (rule H13).
func (r Row) Late(day string) bool {
	if r.Below == "" {
		return false
	}
	b, err1 := time.Parse(time.DateOnly, r.Below)
	d, err2 := time.Parse(time.DateOnly, day)
	return err1 == nil && err2 == nil && d.Sub(b) > GapDays*24*time.Hour
}

// Read reads File in the tree at root. A tree with no file gives no row.
func Read(root string) (map[string]Row, error) {
	recs, err := mdrec.Read(filepath.Join(root, File))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Row{}, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := mdrec.Unmarshal[Row](recs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	out := make(map[string]Row, len(rows))
	for _, r := range rows {
		out[r.ID] = r
	}
	return out, nil
}

// Write writes the rows to File in the tree at root.
func Write(root string, rows []Row) error {
	recs, err := mdrec.Marshal(rows)
	if err != nil {
		return err
	}
	path := filepath.Join(root, File)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return mdrec.Write(path, recs)
}

// CountMentions counts the mentions of each resource in the tree at root, and the mentions that are in an inventory row with an outcome other than deferred.
func CountMentions(root string) (map[string]Mentions, error) {
	out := map[string]Mentions{}
	of := map[string]string{} // the resource of each mention id
	err := inventory.WalkMentions(root, func(_ string, _ int, m inventory.Mention) error {
		id, _, _ := strings.Cut(m.Resource, "/")
		if id == "" {
			return nil
		}
		c := out[id]
		c.Total++
		if m.At != nil {
			c.WithAt++
		}
		out[id] = c
		of[m.ID] = id
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(of) == 0 {
		return out, nil
	}
	err = inventory.WalkRows(root, func(_ string, _ int, row inventory.Row) error {
		if row.Outcome == nil || row.Outcome.Value == inventory.OutcomeDeferred {
			return nil
		}
		for _, m := range row.Mentions {
			if id := of[m]; id != "" {
				c := out[id]
				c.Decided++
				out[id] = c
			}
		}
		return nil
	})
	return out, err
}

// OpenTexts gives the sources of the open textbooks of K5.4 Table A: one source for each book of the textbook adapter.
func OpenTexts() map[string]bool {
	out := map[string]bool{}
	for _, b := range textbook.Books {
		out[b.ID] = true
	}
	return out
}

// Build computes the rows of the tree at root on the day and writes them to File. Named holds the resources that a curriculum names.
func Build(root, day string, rs []resource.Resource, named map[string]bool) ([]Row, error) {
	prev, err := Read(root)
	if err != nil {
		return nil, err
	}
	ms, err := CountMentions(root)
	if err != nil {
		return nil, err
	}
	ds, err := scope.ReadDecisions(root)
	if err != nil {
		return nil, err
	}
	dec := map[string]string{}
	for _, d := range ds {
		if d.Kind == "tier" {
			if _, ok := Targets[d.Value]; !ok {
				return nil, fmt.Errorf("%s: the tier decision on %s has the value %q, which is not a tier", scope.DecisionsFile, d.ID, d.Value)
			}
			dec[d.ID] = d.Value
		}
	}
	rows := Compute(Input{Resources: rs, Named: named, OpenTexts: OpenTexts(), Mentions: ms, Previous: prev, Decisions: dec, Day: day})
	return rows, Write(root, rows)
}
