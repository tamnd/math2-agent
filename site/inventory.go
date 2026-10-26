package site

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/tamnd/math2-agent/inventory"
)

// invRow is one row of the inventory on a page.
type invRow struct {
	ID, Name, Kind, Layer string
	Outcome, Reason       string // the outcome, or "open"
	Spread                int
	Links                 []link // the pages of the item in the sources
}

// invLetter is one page of the inventory: the rows whose names start with one letter.
type invLetter struct {
	Letter, Dir, URL string
	Rows             []invRow
}

// invCount is one line of a table of counts.
type invCount struct {
	Name  string
	Count int
}

type invIndex struct {
	Summary  *inventory.Summary
	Letters  []*invLetter
	Kinds    []invCount
	Layers   []invCount
	Outcomes []invCount
	Sources  []invCount
	MSC      []invCount
	SpecURL  string // the page of the inventory in the corpus spec, or "" when the site has no corpus spec
}

type invPage struct {
	Letter  *invLetter
	Letters []*invLetter
}

// loadInventory reads the inventory of the tree. A tree with no inventory gives no pages but the index.
func (b *builder) loadInventory() error {
	rows, err := inventory.ReadRows(b.opt.Content)
	if err != nil {
		return fmt.Errorf("site: read the inventory: %w", err)
	}
	ms, err := inventory.ReadMentions(b.opt.Content)
	if err != nil {
		return fmt.Errorf("site: read the mentions: %w", err)
	}
	day := b.opt.Date
	if day.IsZero() {
		day = time.Now()
	}
	b.invSummary = inventory.Summarize(rows, ms, day)
	byID := make(map[string]*inventory.Mention, len(ms))
	for i := range ms {
		byID[ms[i].ID] = &ms[i]
	}
	letters := map[string]*invLetter{}
	for i := range rows {
		r := &rows[i]
		if r.Outcome != nil && r.Outcome.Value == inventory.OutcomeMerged {
			continue
		}
		v := invRow{ID: r.ID, Name: cmp.Or(r.Name, r.ID), Kind: r.Kind, Layer: r.Layer, Spread: r.Spread, Outcome: "open"}
		if r.Outcome != nil && !r.IsOpen(day) {
			v.Outcome, v.Reason = r.Outcome.Value, r.Outcome.Reason
		}
		seen := map[string]bool{}
		add := func(l link) {
			if !seen[l.Text] {
				seen[l.Text] = true
				v.Links = append(v.Links, l)
			}
		}
		for _, id := range r.Mentions {
			m := byID[id]
			if m == nil {
				continue
			}
			if m.NativeID != "" {
				add(idLink(inventory.SystemOf(m.Source), m.NativeID))
			}
			for _, sys := range slices.Sorted(maps.Keys(m.Same)) {
				for _, x := range m.Same[sys] {
					add(idLink(sys, x))
				}
			}
		}
		l := letterOf(v.Name)
		p := letters[l]
		if p == nil {
			dir := strings.ToLower(l)
			if l == "#" {
				dir = "other"
			}
			p = &invLetter{Letter: l, Dir: "inventory/" + dir, URL: b.url("inventory/" + dir + "/")}
			letters[l] = p
		}
		p.Rows = append(p.Rows, v)
	}
	for _, l := range slices.Sorted(maps.Keys(letters)) {
		p := letters[l]
		slices.SortFunc(p.Rows, func(a, c invRow) int {
			return cmp.Or(cmp.Compare(strings.ToLower(a.Name), strings.ToLower(c.Name)), cmp.Compare(a.ID, c.ID))
		})
		b.invLetters = append(b.invLetters, p)
	}
	return nil
}

// letterOf returns the first letter of a name in upper case, or # when the name does not start with a letter of A to Z.
func letterOf(name string) string {
	for _, r := range name {
		r = unicode.ToUpper(r)
		if 'A' <= r && r <= 'Z' {
			return string(r)
		}
		return "#"
	}
	return "#"
}

// counts returns the counts of a map, the largest first.
func counts(m map[string]int) []invCount {
	out := make([]invCount, 0, len(m))
	for k, n := range m {
		out = append(out, invCount{k, n})
	}
	slices.SortFunc(out, func(a, b invCount) int { return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Name, b.Name)) })
	return out
}

// writeInventory writes the index of the inventory, one page for each first letter and the JSON summary.
func (b *builder) writeInventory() error {
	s := b.invSummary
	d := invIndex{Summary: s, Letters: b.invLetters, Kinds: counts(s.Kinds), Layers: counts(s.Layers), Outcomes: counts(s.Outcomes), Sources: counts(s.Sources)}
	for _, sd := range b.specs {
		if sd.File == "corpus/04-inventory.md" {
			d.SpecURL = sd.URL
		}
	}
	for _, c := range counts(s.MSC) {
		if c.Name != "none" {
			c.Name += " " + mscTop[c.Name]
		}
		d.MSC = append(d.MSC, c)
	}
	desc := "The inventory of Math 2.0: the list of each item of mathematics that a source presents, and the open count."
	if err := b.writePage("inventory", "inventory", d, page{Title: "Inventory", Description: desc, Section: "inventory", JSONURL: b.url("api/inventory.json")}); err != nil {
		return err
	}
	for _, l := range b.invLetters {
		title := "Inventory: " + l.Letter
		if l.Letter == "#" {
			title = "Inventory: other names"
		}
		if err := b.writePage(l.Dir, "invletter", invPage{Letter: l, Letters: b.invLetters}, page{Title: title, Description: desc, Section: "inventory"}); err != nil {
			return err
		}
	}
	return b.writeJSON("api/inventory.json", s)
}
