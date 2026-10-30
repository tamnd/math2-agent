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
		if src.Kind == "textbook" && src.Resource != "" {
			m.Resource, m.At = src.Resource, textbookAt(r.Meta)
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

// kindOf reads the kind of a record of the source. A record of a textbook or a library can have a kind of the inventory. It returns Unknown when the record does not show its kind.
func kindOf(src *ingest.Source, r *ingest.Record) string {
	if (src.Kind == "textbook" || src.Kind == ingest.Library) && slices.Contains(Kinds(), r.Kind) {
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
		if r.Meta["instance_of"] != "" {
			// A person, a work, a place, an algorithm or a paradox: the rules decide.
			return Unknown
		}
		if k := mathlibKind(split(r.Meta["mathlib"])); k != "" {
			return k
		}
		return NameKind(r.Title)
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
	case "arxiv_results":
		if slices.Contains(Kinds(), r.Kind) {
			return r.Kind
		}
	case "palomar":
		if r.Kind == "result" {
			return "statement"
		}
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

// mathlibKind gives the kind of a Wikidata item from the names of its Mathlib declarations (P14534), by the naming rules of Mathlib: a theorem has a name in snake_case, as integral_comp_mul_deriv, and a structure, a class or a function has a name in camel case, as CommRing, IsCoprime or Real.log. It returns "" with no name, and when the names disagree.
func mathlibKind(names []string) string {
	k := ""
	for _, n := range names {
		last := n[strings.LastIndex(n, ".")+1:]
		got := "concept"
		if strings.Contains(last, "_") {
			got = "statement"
		}
		if k != "" && k != got {
			return ""
		}
		k = got
	}
	return k
}

// headKinds gives the kind of a name of a Wikidata item from its last word, the head of the noun phrase, as the "theorem" of "Gershgorin circle theorem" or the "group" of "knot group". The words are the frequent heads of the items with no class on 2026-10-28. A word that heads names of more than one kind, as "algebra" in "Lie algebra" and "linear algebra", or "method", is not in the list, and the name gives no kind.
var headKinds = map[string]string{}

func init() {
	for kind, ws := range map[string][]string{
		"statement":  {"theorem", "theorems", "lemma", "inequality", "inequalities", "identity", "identities", "formula", "formulas", "formulae", "law", "laws", "principle", "criterion", "corollary", "postulate", "axiom", "reciprocity"},
		"conjecture": {"conjecture"},
		"problem":    {"problem"},
		"topic":      {"theory", "combinatorics", "mathematics", "statistics", "dynamics", "calculus", "analysis"},
		"concept": {"graph", "graphs", "function", "functions", "number", "numbers", "group", "groups", "space", "spaces", "matrix", "matrices", "category", "curve", "curves", "set", "sets",
			"distribution", "polynomial", "polynomials", "field", "map", "mapping", "form", "manifold", "bundle", "sequence", "ring", "surface", "operator", "integral", "knot",
			"functor", "series", "module", "tree", "cohomology", "homology", "extension", "relation", "variety", "representation", "projection", "transform", "transformation", "vector",
			"tensor", "measure", "subgroup", "dimension", "lattice", "ideal", "morphism", "homomorphism", "isomorphism", "metric", "norm", "derivative", "matroid", "process", "scheme",
			"sheaf", "stack", "complex", "monoid", "semigroup", "groupoid", "topology", "filter", "invariant", "polytope", "order", "ordering", "equation", "equations", "system", "code",
			"coloring", "colouring", "partition", "permutation", "quotient", "subspace", "submanifold", "connection", "flow", "game", "language", "automaton", "grammar"},
	} {
		for _, w := range ws {
			headKinds[w] = kind
		}
	}
}

// NameKind returns the kind that the name of an item tells by the last word of its first part, before a comma, a bracket or a preposition, or Unknown. "Hahn–Banach theorem" is a statement and "knot group" a concept. A name of one word gives no kind, because one word, as "group" or "theorem", names the concept or the word and not an item.
func NameKind(name string) string {
	name = strings.ToLower(name)
	if i := strings.IndexAny(name, ",(["); i >= 0 {
		name = name[:i]
	}
	// The head of "second fundamental theorem of calculus" is "theorem", and the name of "degree of a field extension" has no head of the list.
	for _, p := range []string{" of ", " for ", " on ", " in "} {
		if i := strings.Index(name, p); i >= 0 {
			name = name[:i]
		}
	}
	ws := strings.Fields(name)
	if len(ws) < 2 {
		return Unknown
	}
	if k := headKinds[strings.Trim(ws[len(ws)-1], ".:;'\"")]; k != "" {
		return k
	}
	return Unknown
}

// textbookAt gives the structured locator of an item of a textbook: the environment as the kind, and the section when its heading starts with a number.
func textbookAt(meta map[string]string) *At {
	at := At{Kind: meta["environment"]}
	if sec := strings.Fields(meta["section"]); len(sec) > 0 && sec[0][0] >= '0' && sec[0][0] <= '9' {
		at.Section = strings.TrimSuffix(sec[0], ".")
	}
	if at == (At{}) {
		return nil
	}
	return &at
}
