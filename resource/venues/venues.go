// Package venues builds the venue register of RS6.5, records/resources/venues.md. A venue is a resource of kind journal or series. The register is an index of the venue resources: each record has the id of the resource, its title and ISSNs, and the scope of the venue (core, mixed or border). Test S4 of the scope predicate reads the scope: each paper or book of a venue with the scope core is in scope (RS09).
//
// Build adds each live venue resource of the tree to the register. A venue has a scope when a person set it in the register, or when a list of known venues, such as the lists of mathematics series of the series adapters, has one of its ISSNs. A venue with no scope is in the register, but it is not a signal of scope.
package venues

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/ids"
	"github.com/tamnd/math2-agent/resource/scope"
)

// File is the venue register.
const File = resource.Dir + "/venues.md"

// Kinds are the kinds of resource that are venues.
var Kinds = []string{"journal", "series"}

// Venue is one record of the register.
type Venue struct {
	ID        string   `json:"id"` // the id of the venue resource
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`
	Abbrev    string   `json:"abbrev,omitempty"` // the ISO 4 abbreviation
	ISSN      []string `json:"issn,omitempty"`
	Publisher string   `json:"publisher,omitempty"`
	Scope     string   `json:"scope,omitempty"`  // core, mixed or border. Empty means that nobody has set it
	From      string   `json:"from,omitempty"`   // what gave the scope: a person, or the list of known venues
	Status    string   `json:"status,omitempty"` // withdrawn when the resource is no longer a live venue
}

// Read reads the register of the tree at root. A tree with no register gives no record and no error.
func Read(root string) ([]Venue, error) {
	recs, err := mdrec.Read(filepath.Join(root, File))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out, err := mdrec.Unmarshal[Venue](recs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	for _, v := range out {
		if err := v.Check(); err != nil {
			return nil, fmt.Errorf("%s: %w", File, err)
		}
	}
	return out, nil
}

// Check reports a record with no id or with a scope that is not core, mixed or border.
func (v Venue) Check() error {
	if v.ID == "" {
		return fmt.Errorf("a venue has no id")
	}
	switch v.Scope {
	case "", scope.VenueCore, scope.VenueMixed, scope.VenueBorder:
		return nil
	}
	return fmt.Errorf("venue %s: the scope %q is not core, mixed or border", v.ID, v.Scope)
}

// Write writes the register to the tree at root, in the order of the ids.
func Write(root string, vs []Venue) error {
	slices.SortFunc(vs, func(a, b Venue) int { return cmp.Compare(a.ID, b.ID) })
	recs, err := mdrec.Marshal(vs)
	if err != nil {
		return err
	}
	path := filepath.Join(root, File)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return mdrec.Write(path, recs)
}

// Scopes gives the scope of each venue that has one and is not withdrawn, by the id of the resource. It is the Venues field of the environment of the scope predicate.
func Scopes(vs []Venue) map[string]string {
	out := map[string]string{}
	for _, v := range vs {
		if v.Scope != "" && v.Status == "" {
			out[v.ID] = v.Scope
		}
	}
	return out
}

// Known is the scope of a known venue and the name of the list that gives it.
type Known struct {
	Scope string
	From  string
}

// Counts are the outcomes of one build.
type Counts struct {
	Venues, Added, Withdrawn, Scoped int
}

// Build merges the live venue resources rs into the register prev. The title, the kind and the ISSNs come from the resource at each build. A scope that a person set stays. A record with no scope gets the scope of the first ISSN that known has. A record whose resource is not a live venue now stays with the status withdrawn, so that the register never loses a line.
func Build(prev []Venue, rs []resource.Resource, known map[string]Known) ([]Venue, Counts) {
	var c Counts
	by := map[string]Venue{}
	for _, p := range prev {
		by[p.ID] = p
	}
	seen := map[string]bool{}
	for i := range rs {
		r := &rs[i]
		if !r.Live() || !slices.Contains(Kinds, r.Kind) {
			continue
		}
		seen[r.ID] = true
		v, had := by[r.ID]
		if !had {
			c.Added++
		}
		v.ID, v.Kind, v.Title, v.Status = r.ID, r.Kind, r.Title, ""
		v.ISSN = issns(r)
		if v.Scope == "" {
			for _, n := range v.ISSN {
				if k, ok := known[n]; ok {
					v.Scope, v.From = k.Scope, k.From
					break
				}
			}
		}
		by[r.ID] = v
	}
	out := make([]Venue, 0, len(by))
	for id, v := range by {
		if !seen[id] && v.Status != "withdrawn" {
			v.Status = "withdrawn"
			c.Withdrawn++
		}
		if v.Scope != "" && v.Status == "" {
			c.Scoped++
		}
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b Venue) int { return cmp.Compare(a.ID, b.ID) })
	c.Venues = len(out)
	return out, c
}

// issns gives the ISSNs of the work and of its expressions, in normal form and in the order that they come.
func issns(r *resource.Resource) []string {
	var out []string
	add := func(vs []string) {
		for _, s := range vs {
			if n, err := ids.Normalize("issn", s); err == nil && !slices.Contains(out, n) {
				out = append(out, n)
			}
		}
	}
	add(r.IDs["issn"])
	for _, e := range r.Expressions {
		add(e.IDs["issn"])
		for _, m := range e.Manifestations {
			add(m.IDs["issn"])
		}
	}
	return out
}
