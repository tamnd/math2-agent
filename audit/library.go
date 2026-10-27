package audit

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/lean"
)

// libraries returns the source documents of the formal libraries in the catalogue, by id.
func libraries(cat *ingest.Catalogue) map[string]*ingest.Source {
	out := map[string]*ingest.Source{}
	if cat == nil {
		return out
	}
	for i := range cat.Sources {
		if s := &cat.Sources[i]; s.Kind == ingest.Library {
			out[s.ID] = s
		}
	}
	return out
}

// libraryOutcome reads the catalogue for a rule on the formal libraries. It returns false when the rule cannot go on.
func libraryOutcome(im *imports) (outcome, map[string]*ingest.Source, bool) {
	o, ok := catalogueOutcome(im)
	if !ok {
		return o, nil, false
	}
	libs := libraries(im.cat)
	if len(libs) == 0 {
		o.reason = "the catalogue has no formal library"
		return o, nil, false
	}
	return o, libs, true
}

// f10 checks that each mention of a formal library records the library, the pin, the declaration name, the intake class and the licence of the item (rule F10, corpus K6.3). The source document of each library must give its intake class and the reason for it (K6.2). A mention keeps the intake class of its pin, thus it can differ from the class of the source document after the library changes its licence.
func f10(a *auditor) outcome {
	im := a.importsOf()
	o, libs, ok := libraryOutcome(im)
	if !ok {
		return o
	}
	for _, id := range slices.Sorted(maps.Keys(libs)) {
		s := libs[id]
		o.checked++
		line := im.catLine(id)
		switch {
		case s.Intake == "":
			o.add(ingest.CatalogueFile, line, 1, "%s: the library has no intake class. Give one of %v and the intake_reason", id, ingest.Intakes)
		case !slices.Contains(ingest.Intakes, s.Intake):
			o.add(ingest.CatalogueFile, line, 1, "%s: the intake class %q is not one of %v", id, s.Intake, ingest.Intakes)
		case s.IntakeReason == "":
			o.add(ingest.CatalogueFile, line, 1, "%s: the library has the intake class %s and no intake_reason", id, s.Intake)
		}
	}
	err := inventory.WalkMentions(a.root, func(file string, line int, m inventory.Mention) error {
		if libs[m.Source] == nil {
			return nil
		}
		o.checked++
		var miss []string
		for _, f := range [...]struct{ v, name string }{{m.Pin, "pin"}, {m.NativeID, "declaration name"}, {m.Intake, "intake class"}, {m.Licence, "licence"}} {
			if f.v == "" {
				miss = append(miss, f.name)
			}
		}
		switch {
		case len(miss) > 0:
			o.add(file, line, 1, "the mention %s of the library %s has no %s. Import the library again", m.ID, m.Source, strings.Join(miss, " and no "))
		case !slices.Contains(ingest.Intakes, m.Intake):
			o.add(file, line, 1, "the mention %s has the intake class %q, which is not one of %v", m.ID, m.Intake, ingest.Intakes)
		}
		return nil
	})
	if err != nil {
		o.add(inventory.MentionsDir, 0, 0, "%v", err)
	}
	return o
}

// f11 checks that no node body holds text from a library whose intake class is link, evaluate or exclude, or from an item of a library whose licence is not open (rule F11, corpus K6.2). It finds the text by the fingerprints, as rule L02 does.
func f11(a *auditor) outcome {
	var o outcome
	im := a.importsOf()
	if im.noRecords != "" {
		o.reason = im.noRecords
		return o
	}
	libs := libraries(im.cat)
	if len(libs) == 0 {
		o.reason = "the catalogue has no formal library"
		return o
	}
	for _, n := range a.scopeNodes() {
		if retired(n) {
			continue
		}
		o.checked++
		recs, shared := im.copies(prints(n))
		for _, r := range recs {
			s := libs[im.source[r]]
			if s == nil {
				continue
			}
			switch licence := cmp.Or(r.Licence, s.Elected()); {
			case ingest.NoText(s.Intake):
				o.add(n.Path, n.BodyLine, 1, "the body shares %d fingerprints with %s %s, and the library has the intake class %s, which gives no text. Write the text in your own words", shared[r], s.ID, r.ID, s.Intake)
			case ingest.ClassOf(licence) != ingest.Open:
				o.add(n.Path, n.BodyLine, 1, "the body shares %d fingerprints with %s %s, which has the licence %s. Only the text of an item with an open licence can go into a node. Write the text in your own words", shared[r], s.ID, r.ID, licence)
			}
		}
	}
	return o
}

// f12 checks that the source document of each formal library has the field authored, and that each mention of the library copies it (rule F12, corpus K6.5).
func f12(a *auditor) outcome {
	im := a.importsOf()
	o, libs, ok := libraryOutcome(im)
	if !ok {
		return o
	}
	for _, id := range slices.Sorted(maps.Keys(libs)) {
		s := libs[id]
		o.checked++
		line := im.catLine(id)
		switch {
		case s.Authored == "":
			o.add(ingest.CatalogueFile, line, 1, "%s: the library has no field authored. Give one of %v", id, ingest.Authorships)
		case !slices.Contains(ingest.Authorships, s.Authored):
			o.add(ingest.CatalogueFile, line, 1, "%s: authored is %q, which is not one of %v", id, s.Authored, ingest.Authorships)
		}
	}
	err := inventory.WalkMentions(a.root, func(file string, line int, m inventory.Mention) error {
		s := libs[m.Source]
		if s == nil || s.Authored == "" {
			return nil
		}
		o.checked++
		switch {
		case m.Authored == "":
			o.add(file, line, 1, "the mention %s has no field authored, and the source document of %s has %s. Import the library again", m.ID, m.Source, s.Authored)
		case m.Authored != s.Authored:
			o.add(file, line, 1, "the mention %s has authored %q, and the source document of %s has %s. Import the library again", m.ID, m.Authored, m.Source, s.Authored)
		}
		return nil
	})
	if err != nil {
		o.add(inventory.MentionsDir, 0, 0, "%v", err)
	}
	return o
}

// definitionKinds are the kinds of the declarations that rule F13 counts as definitions.
var definitionKinds = []string{"def", "inductive", "opaque"}

// f13 reports each binding to a declaration of a Lean library that AI wrote, whose statement uses definitions of the same library that no review of the binding covers (rule F13, corpus K6.5). The source document of the library names its root modules, and the review lists the definitions that it read in covers. A constant without a record in extract.json is not counted, because the rule cannot tell its library.
func f13(a *auditor) outcome {
	var o outcome
	b := a.lastBuild()
	if reason := cmp.Or(b.noExtract, b.noReviews); reason != "" {
		o.reason = reason
		return o
	}
	byModule := map[string]*ingest.Source{}
	for _, s := range libraries(a.importsOf().cat) {
		if ingest.ByAI(s.Authored) {
			for _, m := range s.Modules {
				byModule[m] = s
			}
		}
	}
	if len(byModule) == 0 {
		o.reason = "the catalogue has no Lean library that AI wrote"
		return o
	}
	libraryOf := func(r lean.Record) *ingest.Source {
		root, _, _ := strings.Cut(r.Module, ".")
		return byModule[root]
	}
	for _, n := range a.scopeNodes() {
		if retired(n) {
			continue
		}
		for _, bd := range leanBindings(n) {
			r, ok := b.records[bd.decl]
			if !ok || r.Error != "" {
				continue
			}
			s := libraryOf(r)
			if s == nil {
				continue
			}
			o.checked++
			covered := map[string]bool{}
			for _, id := range bd.reviews {
				for _, c := range b.reviews[id].Covers {
					covered[c] = true
				}
			}
			var miss []string
			for _, c := range r.UsesConstantsInStatement {
				d, ok := b.records[c]
				if ok && c != r.Decl && slices.Contains(definitionKinds, d.Kind) && libraryOf(d) == s && !covered[c] {
					miss = append(miss, c)
				}
			}
			slices.Sort(miss)
			if miss = slices.Compact(miss); len(miss) > 0 {
				o.add(n.Path, bd.line, bd.col, "%s is from %s, which has authored: %s, and its statement uses the definitions %s of the same library. No review of the binding covers them. Read them and list them in covers of a review", r.Decl, s.ID, s.Authored, strings.Join(miss, ", "))
			}
		}
	}
	return o
}
