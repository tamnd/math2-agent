package audit

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/align"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/prov"
)

// unconfirmedAge is the age after which rule A02 reports a same_as value that an agent wrote.
const unconfirmedAge = 90 // days

// a02 reports each same_as value that an agent wrote and that no person accepted or confirmed in 90 days (rule A02). The writer of a value is in the provenance log of the node, prov/<shard>/<tag>.jsonl: the last line on same_as.<system>, on same_as or on the whole node (prov.Author). A value that came through m2 align accept has a person already. The age is to the date of the HEAD commit, as in V04.
func a02(a *auditor) outcome {
	var o outcome
	if !isRepo(a.root) {
		o.reason = "the tree is not a git repository"
		return o
	}
	if _, err := os.Stat(filepath.Join(a.root, "prov")); err != nil {
		o.reason = "the tree has no provenance in prov/, thus the writer of a same_as value is not known"
		return o
	}
	now, err := headDate(a.root)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	decided, err := personal(a.root)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	for _, n := range a.scopeNodes() {
		m := sameAs(n)
		if m == nil || retired(n) {
			continue
		}
		log, err := prov.Read(a.root, n.ID)
		if err != nil {
			o.reason = err.Error()
			return o
		}
		for i := 0; i+1 < len(m.Content); i += 2 {
			k, v := m.Content[i], m.Content[i+1]
			if v.Kind != yaml.ScalarNode || v.Tag == "!!null" {
				continue
			}
			p, ok := prov.Author(log, align.SameAs+"."+k.Value)
			if !ok {
				continue
			}
			o.checked++
			if p.Agent == "" || p.Human != "" || decided[[3]string{n.ID, k.Value, v.Value}] {
				continue
			}
			at, err := time.Parse(time.RFC3339, p.At)
			if err != nil {
				continue // the time is the business of the provenance rules
			}
			if at.AddDate(0, 0, unconfirmedAge).Before(now) {
				o.add(n.Path, v.Line, v.Column, "the same_as value %s in %s is from %s on %s, and no person confirmed it in %d days. Run m2 align confirm, or remove the value", v.Value, k.Value, p.Agent, at.Format(time.DateOnly), unconfirmedAge)
			}
		}
	}
	return o
}

// personal returns the same_as values that a person accepted or confirmed, by tag, system and id.
func personal(root string) (map[[3]string]bool, error) {
	ds, err := align.ReadDecisions(root)
	if err != nil {
		return nil, err
	}
	out := map[[3]string]bool{}
	for _, d := range ds {
		if d.Edge == align.SameAs && (d.Decision == align.Accepted || d.Decision == align.Confirmed) {
			out[[3]string{d.Tag, d.System, d.ID}] = true
		}
	}
	return out, nil
}

// a03 checks each same_as value against the latest records of its system (rule A03). The record must still be there. When a person decided on the value, and the register has the hash of the record at that time, the record must still say the same: the same kind, title and text. The rule reads the records that m2 ingest wrote, and it does not fetch them again. Thus the audit does not use the network, and its result changes only with a new snapshot. A value whose system has no records in the tree is not checked.
func a03(a *auditor) outcome {
	var o outcome
	ds, err := align.ReadDecisions(a.root)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	type decision struct{ hash, person, date string }
	last := map[[3]string]decision{}
	for _, d := range ds {
		if d.Edge == align.SameAs && (d.Decision == align.Accepted || d.Decision == align.Confirmed) && d.Record != "-" {
			last[[3]string{d.Tag, d.System, d.ID}] = decision{d.Record, d.Person, d.Date}
		}
	}
	records := map[string]map[string]string{} // the hash of each record by id, for each source; nil when the tree has no records
	for _, n := range a.scopeNodes() {
		m := sameAs(n)
		if m == nil || retired(n) {
			continue
		}
		for i := 0; i+1 < len(m.Content); i += 2 {
			k, v := m.Content[i], m.Content[i+1]
			if v.Kind != yaml.ScalarNode || v.Tag == "!!null" {
				continue
			}
			src := align.SourceOf(k.Value)
			recs, ok := records[src]
			if !ok {
				if recs, err = hashes(a.root, src); err != nil {
					o.reason = err.Error()
					return o
				}
				records[src] = recs
			}
			if recs == nil {
				continue
			}
			o.checked++
			h, ok := recs[v.Value]
			if !ok {
				o.add(n.Path, v.Line, v.Column, "%s %s is not in the latest records of %s. The record is gone, or it has a new id", k.Value, v.Value, src)
				continue
			}
			if d, ok := last[[3]string{n.ID, k.Value, v.Value}]; ok && d.hash != h {
				o.add(n.Path, v.Line, v.Column, "the record %s %s changed after %s decided on it on %s. Check that it is the same thing still, and run m2 align confirm", k.Value, v.Value, d.person, d.date)
			}
		}
	}
	if o.checked == 0 && len(o.findings) == 0 {
		o.reason = "the tree has no records of the systems of its same_as values"
	}
	return o
}

// hashes returns the hash of each record of the source by id, or nil when the tree has no records of the source.
func hashes(root, source string) (map[string]string, error) {
	recs, _, err := ingest.ReadRecords(root, source)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(recs))
	for _, r := range recs {
		out[r.ID] = align.RecordHash(r)
	}
	return out, nil
}

// dedupAge is the age after which rule A04 reports a duplicate proposal.
const dedupAge = 30 // days

// a04 reports each duplicate proposal in the dedup queue that is older than 30 days (rule A04). A decision takes a proposal out of the queue, thus a proposal in the queue has no decision. A proposal whose node is retired is resolved. The age is to the date of the HEAD commit.
func a04(a *auditor) outcome {
	var o outcome
	if !isRepo(a.root) {
		o.reason = "the tree is not a git repository"
		return o
	}
	now, err := headDate(a.root)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	ps, err := align.ReadQueue(a.root)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	gone := map[string]bool{}
	for _, n := range a.nodes {
		if retired(n) {
			gone[n.ID] = true
		}
	}
	line := 0
	for _, p := range ps {
		if p.Edge != align.Duplicate {
			continue
		}
		line++
		o.checked++
		if gone[p.Node] || gone[p.Target] {
			continue
		}
		d, err := time.Parse(time.DateOnly, p.Proposed)
		if err != nil {
			o.add(align.DedupQueue, line, 1, "the proposal %s has the date %q, which is not YYYY-MM-DD", p.ID, p.Proposed)
			continue
		}
		if d.AddDate(0, 0, dedupAge).Before(now) {
			o.add(align.DedupQueue, line, 1, "%s and %s can be one node, and no person decided on it since %s. Run m2 align accept or m2 align reject on %s", p.Node, p.Target, p.Proposed, p.ID)
		}
	}
	return o
}
