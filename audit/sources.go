package audit

import (
	"fmt"
	"slices"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/inventory"
)

// licenceAge is the age at which rule L12 reports the last reading of a licence (corpus K5.14).
const licenceAge = 12 // months

// catalogueOutcome starts the outcome of a rule that reads the source documents. It reports a catalogue that does not load. A tree with no catalogue skips the rule.
func catalogueOutcome(im *imports) (outcome, bool) {
	var o outcome
	switch {
	case im.catErr != nil:
		o.checked++
		o.add(ingest.CatalogueFile, 1, 1, "%v", im.catErr)
		return o, false
	case im.cat == nil || len(im.cat.Sources) == 0:
		o.reason = "the tree has no source documents in " + ingest.CatalogueFile
		return o, false
	}
	return o, true
}

// l08 checks that each source document has the fields that corpus K5.2 requires, and that each mention names a source with a complete document (rule L08).
func l08(a *auditor) outcome {
	im := a.importsOf()
	o, ok := catalogueOutcome(im)
	if !ok {
		return o
	}
	complete := map[string]bool{}
	for i := range im.cat.Sources {
		s := &im.cat.Sources[i]
		o.checked++
		missing := documentProblems(s)
		for _, p := range missing {
			o.add(ingest.CatalogueFile, im.catLine(s.ID), 1, "%s: %s", s.ID, p)
		}
		complete[s.ID] = len(missing) == 0
	}
	bad := map[string]bool{}
	err := inventory.WalkMentions(a.root, func(file string, line int, m inventory.Mention) error {
		o.checked++
		if bad[m.Source] {
			return nil
		}
		switch ok, known := complete[m.Source]; {
		case !known:
			o.add(file, line, 1, "the mention %s names the source %q, which has no entry in %s", m.ID, m.Source, ingest.CatalogueFile)
			bad[m.Source] = true
		case !ok:
			o.add(file, line, 1, "the mention %s names the source %s, whose source document is not complete", m.ID, m.Source)
			bad[m.Source] = true
		}
		return nil
	})
	if err != nil {
		o.add(inventory.MentionsDir, 0, 0, "%v", err)
	}
	return o
}

// documentProblems returns the required fields of K5.2 that the source document lacks or that have a value that is not allowed, one sentence each.
func documentProblems(s *ingest.Source) []string {
	var out []string
	need := func(ok bool, field string) {
		if !ok {
			out = append(out, "the source document has no "+field)
		}
	}
	need(s.Licence != "", "licence")
	need(s.Class != "", "class")
	need(s.LicenceRead != nil, "licence_read")
	if r := s.LicenceRead; r != nil {
		need(r.At != "", "licence_read.at")
		need(r.By != "", "licence_read.by")
		if _, err := time.Parse(time.DateOnly, r.On); err != nil {
			out = append(out, fmt.Sprintf("licence_read.on is %q, which is not a date YYYY-MM-DD", r.On))
		}
	}
	oneOf := func(v, field string, values []string) {
		switch {
		case v == "":
			need(false, field)
		case !slices.Contains(values, v):
			out = append(out, fmt.Sprintf("%s is %q, which is not one of %v", field, v, values))
		}
	}
	oneOf(s.Authored, "authored", ingest.Authorships)
	oneOf(s.Channel, "channel", ingest.Channels)
	if s.Kind != "" {
		oneOf(s.Kind, "kind", ingest.Kinds)
	}
	need(len(s.Snapshots) > 0, "snapshot")
	need(s.Attribution != "", "attribution")
	need(s.ApprovedBy != "", "approved_by")
	return out
}

// l09 checks that a source with a choice of licences has an election that is one of the choices, and that the elected licence gives the class (rule L09).
func l09(a *auditor) outcome {
	im := a.importsOf()
	o, ok := catalogueOutcome(im)
	if !ok {
		return o
	}
	for i := range im.cat.Sources {
		s := &im.cat.Sources[i]
		choices := ingest.Choices(s.Licence)
		if choices == nil && s.Election == "" {
			continue
		}
		o.checked++
		line := im.catLine(s.ID)
		switch {
		case choices == nil:
			o.add(ingest.CatalogueFile, line, 1, "%s: the licence %s is not a choice of licences, and the source has the election %s", s.ID, s.Licence, s.Election)
		case s.Election == "":
			o.add(ingest.CatalogueFile, line, 1, "%s: the licence %s is a choice of licences, and the source has no election", s.ID, s.Licence)
		case !slices.Contains(choices, s.Election):
			o.add(ingest.CatalogueFile, line, 1, "%s: the election %s is not one of the choices of %s", s.ID, s.Election, s.Licence)
		case s.Class != ingest.ClassOf(s.Election) && s.Class != ingest.RecordOnly:
			o.add(ingest.CatalogueFile, line, 1, "%s: the elected licence %s gives the class %s, and the source has the class %s", s.ID, s.Election, ingest.ClassOf(s.Election), s.Class)
		}
	}
	if o.checked == 0 && len(o.findings) == 0 {
		o.reason = "no source has a choice of licences"
	}
	return o
}

// l10 checks that each source of the open class whose licence is the public domain is in the public domain in the United States and in the EU (rule L10, corpus K5.2.3).
func l10(a *auditor) outcome {
	im := a.importsOf()
	o, ok := catalogueOutcome(im)
	if !ok {
		return o
	}
	for i := range im.cat.Sources {
		s := &im.cat.Sources[i]
		if s.Elected() != ingest.PublicDomainLicence || s.Class != ingest.Open {
			continue
		}
		o.checked++
		line := im.catLine(s.ID)
		pd := s.PublicDomain
		switch {
		case pd == nil:
			o.add(ingest.CatalogueFile, line, 1, "%s: the source is open as public domain, and it has no public_domain", s.ID)
		case !pd.US || !pd.EU:
			o.add(ingest.CatalogueFile, line, 1, "%s: the source is open as public domain, and it is not in the public domain in both the United States and the EU. Its class is record", s.ID)
		case pd.Basis == "":
			o.add(ingest.CatalogueFile, line, 1, "%s: public_domain gives no basis", s.ID)
		}
	}
	if o.checked == 0 {
		o.reason = "no open source is in the public domain"
	}
	return o
}

// l11 checks that each mention with text comes from a snapshot whose licence was read on or before the date of the snapshot (rule L11, corpus K5.2.1).
func l11(a *auditor) outcome {
	im := a.importsOf()
	o, ok := catalogueOutcome(im)
	if !ok {
		return o
	}
	reported := map[string]bool{}
	err := inventory.WalkMentions(a.root, func(file string, line int, m inventory.Mention) error {
		if m.Text == "" {
			return nil
		}
		o.checked++
		key := m.Source + " " + m.Snapshot
		if reported[key] {
			return nil
		}
		s := im.cat.Source(m.Source)
		if s == nil {
			return nil // L08 reports it
		}
		sn := s.Snapshot(m.Snapshot)
		read := ""
		if sn != nil {
			read = s.ReadOn(sn)
		}
		switch {
		case sn == nil:
			o.add(file, line, 1, "the mention %s has text from the snapshot %q, which is not a snapshot of %s", m.ID, m.Snapshot, m.Source)
		case read == "":
			o.add(file, line, 1, "the mention %s has text from a snapshot of %s whose licence was not read", m.ID, m.Source)
		case read > sn.Date:
			o.add(file, line, 1, "the mention %s has text from the snapshot of %s of %s, and its licence was read on %s, after the snapshot", m.ID, m.Source, sn.Date, read)
		default:
			return nil
		}
		reported[key] = true
		return nil
	})
	if err != nil {
		o.add(inventory.MentionsDir, 0, 0, "%v", err)
	}
	if o.checked == 0 && len(o.findings) == 0 {
		o.reason = "no mention has text"
	}
	return o
}

// l12 reports each source whose licence was last read more than twelve months before the date of the HEAD commit (rule L12).
func l12(a *auditor) outcome {
	im := a.importsOf()
	o, ok := catalogueOutcome(im)
	if !ok {
		return o
	}
	now, err := headDate(a.root)
	if err != nil {
		o.reason = "the tree has no HEAD commit, thus the age of a reading is not known"
		return o
	}
	limit := now.AddDate(0, -licenceAge, 0)
	for i := range im.cat.Sources {
		s := &im.cat.Sources[i]
		if s.LicenceRead == nil {
			continue // L08 reports it
		}
		read, err := time.Parse(time.DateOnly, s.LicenceRead.On)
		if err != nil {
			continue // L08 reports it
		}
		o.checked++
		if read.Before(limit) {
			o.add(ingest.CatalogueFile, im.catLine(s.ID), 1, "%s: the licence was last read on %s, more than %d months ago. Read it again at %s", s.ID, s.LicenceRead.On, licenceAge, s.LicenceRead.At)
		}
	}
	return o
}

// l06 checks that each item of a source with a licence for each item records the licence of the item (rule L06, corpus K13.4).
func l06(a *auditor) outcome {
	im := a.importsOf()
	o, ok := catalogueOutcome(im)
	if !ok {
		return o
	}
	perItem := map[string]bool{}
	for _, s := range im.cat.Sources {
		perItem[s.ID] = s.PerItemLicence
	}
	for _, r := range im.records {
		src := im.source[r]
		if !perItem[src] {
			continue
		}
		o.checked++
		if r.Licence == "" {
			o.add(ingest.RecordsDir+"/"+src+".jsonl", 1, 1, "the record %s of %s has no licence, and each item of %s has its own licence", r.ID, src, src)
		}
	}
	err := inventory.WalkMentions(a.root, func(file string, line int, m inventory.Mention) error {
		if !perItem[m.Source] {
			return nil
		}
		o.checked++
		if m.Licence == "" {
			o.add(file, line, 1, "the mention %s has no licence, and each item of %s has its own licence", m.ID, m.Source)
		}
		return nil
	})
	if err != nil {
		o.add(inventory.MentionsDir, 0, 0, "%v", err)
	}
	if o.checked == 0 && len(o.findings) == 0 {
		o.reason = "no source has a licence for each item"
	}
	return o
}
