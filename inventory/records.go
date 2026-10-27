package inventory

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/ingest"
)

// FromRecords gives one mention for each record of one import of the source src. The snapshot is the hash of the snapshot that the import read, and by names the importer and its version. The licence gate of ingest has already removed the text that math2 cannot store. FromRecords also removes text under a share-alike licence, because the record plane keeps no such text. A mention of a source with a licence for each item records the licence of the item (rule L06). A mention of a formal library records the licence of the item, the commit of the snapshot, the intake class and the field authored of the library (rules F10 and F12).
func FromRecords(src *ingest.Source, snapshot, by string, recs []ingest.Record) []Mention {
	out := make([]Mention, 0, len(recs))
	var pin string
	for _, sn := range src.Snapshots {
		if sn.Hash == snapshot {
			pin = sn.Commit
		}
	}
	for _, r := range recs {
		m := Mention{
			ID:       MentionID(src.ID, r.ID, 1),
			Source:   src.ID,
			Snapshot: snapshot,
			Locator:  cmp.Or(r.URL, r.Locator),
			NativeID: r.ID,
			Kind:     kindOf(src, &r),
			Name:     clean(r.Title),
			Class:    string(r.Class),
			By:       by,
		}
		licence := cmp.Or(r.Licence, src.Elected())
		if r.Class.StoresText() && !ingest.ShareAlike(licence) {
			m.Text = r.Text
		}
		if src.PerItemLicence || src.Kind == ingest.Library {
			m.Licence = licence
		}
		if src.Kind == ingest.Library {
			m.Pin, m.Intake, m.Authored = pin, src.Intake, src.Authored
		}
		for _, k := range slices.Sorted(maps.Keys(r.Meta)) {
			v := r.Meta[k]
			switch {
			case k == "msc":
				m.MSC = append(m.MSC, strings.FieldsFunc(v, func(r rune) bool { return r == '|' || r == ' ' })...)
			case k == "instance_of":
				m.Types = split(v)
			case k == "type_hash":
				// Only a statement keeps its hash. Pass 2 of Cluster joins statements and never definitions.
				if m.Kind == "statement" {
					m.TypeHash = v
				}
			case crosswalk.Known(k) || k == "mathlib" || src.ID == "wikidata":
				if m.Same == nil {
					m.Same = map[string][]string{}
				}
				m.Same[k] = split(v)
			}
		}
		out = append(out, m)
	}
	sortMentions(out)
	return out
}

// kindOf reads the kind of a record of the source. A record of a textbook has a kind of the inventory. It returns Unknown when the record does not show its kind.
func kindOf(src *ingest.Source, r *ingest.Record) string {
	if src.Kind == "textbook" && slices.Contains(Kinds(), r.Kind) {
		return r.Kind
	}
	switch src.ID {
	case "wikidata":
		if r.Meta["msc"] != "" {
			return "topic"
		}
		for _, q := range split(r.Meta["instance_of"]) {
			if k := classKinds[q]; k != "" {
				return k
			}
		}
		switch pw := r.Meta["proofwiki"]; {
		case strings.HasPrefix(pw, "Definition:"):
			return "definition"
		case strings.HasPrefix(pw, "Axiom:"):
			return "statement"
		case pw != "" && !strings.Contains(pw, ":"):
			return "statement"
		case r.Meta["oeis"] != "" && len(r.Meta) == 1:
			return "object"
		}
	case "mathlib":
		switch r.Kind {
		case "theorem", "axiom":
			return "statement"
		case "def", "inductive", "opaque":
			return "definition"
		}
	case "planetmath":
		switch r.Kind {
		case "definition":
			return "definition"
		case "theorem", "corollary", "result", "derivation", "axiom":
			return "statement"
		case "conjecture", "example", "proof", "topic":
			return r.Kind
		}
	case "proofwiki":
		switch r.Kind {
		case "definition", "problem", "proof":
			return r.Kind
		case "theorem", "proposition", "lemma", "corollary", "axiom":
			return "statement"
		case "conjecture", "open question":
			return "conjecture"
		}
	case "oeis", "lmfdb", "house_of_graphs":
		return "object"
	case "erdosproblems":
		return "problem"
	case "formal_conjectures":
		return "conjecture"
	case "mathlib_100", "mathlib_1000", "thousand_plus":
		return "statement"
	}
	return Unknown
}

// classKinds gives the kind of a Wikidata item from its class (P31). The classes are in the list of the Wikidata adapter.
var classKinds = map[string]string{
	"Q24034552": "concept",    // mathematical concept
	"Q65943":    "statement",  // theorem
	"Q207505":   "statement",  // lemma
	"Q319141":   "conjecture", // conjecture
	"Q1166625":  "problem",    // mathematical problem
	"Q1936384":  "topic",      // branch of mathematics
	"Q20026918": "topic",      // mathematical theory
	"Q186509":   "object",     // mathematical constant
	"Q172937":   "object",     // polyhedron
	"Q2297602":  "object",     // integer sequence
}

// split splits the values of a meta field, which ingest joins with "|".
func split(v string) []string {
	var out []string
	for s := range strings.SplitSeq(v, "|") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
