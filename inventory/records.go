package inventory

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/ingest"
)

// FromRecords gives one mention for each record of one import of the source src. The snapshot is the hash of the snapshot that the import read, and by names the importer and its version. The licence gate of ingest has already removed the text that math2 cannot store. FromRecords also removes text under a share-alike licence, because the record plane keeps no such text.
func FromRecords(src *ingest.Source, snapshot, by string, recs []ingest.Record) []Mention {
	out := make([]Mention, 0, len(recs))
	for _, r := range recs {
		m := Mention{
			ID:       MentionID(src.ID, r.ID, 1),
			Source:   src.ID,
			Snapshot: snapshot,
			Locator:  cmp.Or(r.URL, r.Locator),
			NativeID: r.ID,
			Kind:     kindOf(src.ID, &r),
			Name:     clean(r.Title),
			Class:    string(r.Class),
			By:       by,
		}
		licence := cmp.Or(r.Licence, src.Licence)
		if r.Class.StoresText() && !ingest.ShareAlike(licence) {
			m.Text = r.Text
		}
		for _, k := range slices.Sorted(maps.Keys(r.Meta)) {
			v := r.Meta[k]
			switch {
			case k == "msc":
				m.MSC = append(m.MSC, split(v)...)
			case crosswalk.Known(k) || src.ID == "wikidata":
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

// kindOf reads the kind of a record of the source. It returns Unknown when the record does not show its kind.
func kindOf(source string, r *ingest.Record) string {
	switch source {
	case "wikidata":
		switch pw := r.Meta["proofwiki"]; {
		case r.Meta["msc"] != "":
			return "topic"
		case strings.HasPrefix(pw, "Definition:"):
			return "definition"
		case strings.HasPrefix(pw, "Axiom:"):
			return "statement"
		case pw != "" && !strings.Contains(pw, ":"):
			return "statement"
		case r.Meta["oeis"] != "" && len(r.Meta) == 1:
			return "object"
		}
	case "oeis", "lmfdb", "house_of_graphs":
		return "object"
	case "erdosproblems":
		return "problem"
	case "formalconjectures":
		return "conjecture"
	}
	return Unknown
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
