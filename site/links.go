package site

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// sourceURL gives the URL for each kind of source identifier.
var sourceURL = map[string]func(id string) string{
	"zbmath": func(id string) string { return "https://zbmath.org/?q=an:" + url.QueryEscape(id) },
	"arxiv":  func(id string) string { return "https://arxiv.org/abs/" + id },
	"doi":    func(id string) string { return "https://doi.org/" + id },
	"mr": func(id string) string {
		return "https://mathscinet.ams.org/mathscinet-getitem?mr=" + url.QueryEscape(id)
	},
	"stacks":       func(id string) string { return "https://stacks.math.columbia.edu/tag/" + id },
	"kerodon":      func(id string) string { return "https://kerodon.net/tag/" + id },
	"mathoverflow": func(id string) string { return "https://mathoverflow.net/q/" + id },
	"mathse":       func(id string) string { return "https://math.stackexchange.com/q/" + id },
	"erdos":        func(id string) string { return "https://www.erdosproblems.com/" + id },
	"url":          func(id string) string { return id },
	"book": func(id string) string {
		if isbn, ok := strings.CutPrefix(id, "isbn:"); ok {
			return "https://openlibrary.org/isbn/" + isbn
		}
		return ""
	},
	"wikidata": func(id string) string { return "https://www.wikidata.org/wiki/" + id },
	"mardi":    func(id string) string { return "https://portal.mardi4nfdi.de/wiki/Item:" + id },
	"oeis":     func(id string) string { return "https://oeis.org/" + id },
	"nlab":     func(id string) string { return "https://ncatlab.org/nlab/show/" + url.PathEscape(id) },
	"wikipedia": func(id string) string {
		return "https://en.wikipedia.org/wiki/" + url.PathEscape(strings.ReplaceAll(id, " ", "_"))
	},
	"proofwiki": func(id string) string {
		return "https://proofwiki.org/wiki/" + url.PathEscape(strings.ReplaceAll(id, " ", "_"))
	},
	"lmfdb":     func(id string) string { return "https://www.lmfdb.org/" + id },
	"mathworld": func(id string) string { return "https://mathworld.wolfram.com/" + url.PathEscape(id) + ".html" },
	"eom": func(id string) string {
		return "https://encyclopediaofmath.org/wiki/" + url.PathEscape(strings.ReplaceAll(id, " ", "_"))
	},
	"msc":             func(id string) string { return "https://zbmath.org/classification/?q=cc:" + url.QueryEscape(id) },
	"lmfdb_knowl":     func(id string) string { return "https://www.lmfdb.org/knowledge/show/" + url.PathEscape(id) },
	"house_of_graphs": func(id string) string { return "https://houseofgraphs.org/graphs/" + url.PathEscape(id) },
	"mathlib": func(id string) string {
		return "https://leanprover-community.github.io/mathlib4_docs/find/?pattern=" + url.QueryEscape(id) + "#doc"
	},
}

// sourceLabel is the name of each identifier on the page.
var sourceLabel = map[string]string{
	"zbmath": "zbMATH", "arxiv": "arXiv", "doi": "DOI", "book": "Book", "mr": "MR", "stacks": "Stacks",
	"kerodon": "Kerodon", "mathoverflow": "MathOverflow", "mathse": "Math SE", "erdos": "Erdős problem",
	"formal_conjectures": "formal-conjectures", "palomar": "Palomar", "hexagon": "Hexagon", "url": "Web",
	"wikidata": "Wikidata", "mardi": "MaRDI", "oeis": "OEIS", "nlab": "nLab", "wikipedia": "Wikipedia",
	"proofwiki": "ProofWiki", "lmfdb": "LMFDB", "mathworld": "MathWorld", "eom": "EoM", "msc": "MSC",
	"lmfdb_knowl": "LMFDB knowl", "house_of_graphs": "House of Graphs", "mathlib": "Mathlib",
}

func idLink(key, id string) link {
	l := link{Text: sourceLabel[key] + " " + id}
	if sourceLabel[key] == "" {
		l.Text = key + " " + id
	}
	if f, ok := sourceURL[key]; ok {
		l.URL = f(id)
	}
	return l
}

func sourceLinks(v any) []link {
	items, _ := v.([]any)
	var out []link
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		var l link
		for _, key := range sortedKeys(m) {
			if key == "locator" || key == "note" {
				continue
			}
			l = idLink(key, fmt.Sprint(m[key]))
		}
		if loc, ok := m["locator"].(string); ok {
			l.Note = loc
		}
		if note, ok := m["note"].(string); ok {
			l.Note = strings.TrimSpace(l.Note + ". " + note)
		}
		out = append(out, l)
	}
	return out
}

func sameAsLinks(v any) []link {
	m, _ := v.(map[string]any)
	var out []link
	for _, key := range sortedKeys(m) {
		if m[key] == nil {
			out = append(out, link{Text: sourceLabel[key], Note: "no entry"})
			continue
		}
		out = append(out, idLink(key, fmt.Sprint(m[key])))
	}
	return out
}

func formalLinks(v any) []link {
	items, _ := v.([]any)
	var out []link
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		system, _ := m["system"].(string)
		decl, _ := m["decl"].(string)
		match, _ := m["match"].(string)
		l := link{Text: decl, Note: system + ", " + match + " match"}
		if lib, _ := m["library"].(string); system == "lean4" && lib == "mathlib" {
			l.URL = "https://leanprover-community.github.io/mathlib4_docs/find/?pattern=" + url.QueryEscape(decl) + "#doc"
		}
		out = append(out, l)
	}
	return out
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// mscTop names the 63 top level classes of MSC2020.
var mscTop = map[string]string{
	"00": "General and overarching topics", "01": "History and biography", "03": "Mathematical logic and foundations",
	"05": "Combinatorics", "06": "Order, lattices, ordered algebraic structures", "08": "General algebraic systems",
	"11": "Number theory", "12": "Field theory and polynomials", "13": "Commutative algebra", "14": "Algebraic geometry",
	"15": "Linear and multilinear algebra; matrix theory", "16": "Associative rings and algebras",
	"17": "Nonassociative rings and algebras", "18": "Category theory; homological algebra", "19": "K-theory",
	"20": "Group theory and generalizations", "22": "Topological groups, Lie groups", "26": "Real functions",
	"28": "Measure and integration", "30": "Functions of a complex variable", "31": "Potential theory",
	"32": "Several complex variables and analytic spaces", "33": "Special functions", "34": "Ordinary differential equations",
	"35": "Partial differential equations", "37": "Dynamical systems and ergodic theory", "39": "Difference and functional equations",
	"40": "Sequences, series, summability", "41": "Approximations and expansions", "42": "Harmonic analysis on Euclidean spaces",
	"43": "Abstract harmonic analysis", "44": "Integral transforms, operational calculus", "45": "Integral equations",
	"46": "Functional analysis", "47": "Operator theory", "49": "Calculus of variations and optimal control; optimization",
	"51": "Geometry", "52": "Convex and discrete geometry", "53": "Differential geometry", "54": "General topology",
	"55": "Algebraic topology", "57": "Manifolds and cell complexes", "58": "Global analysis, analysis on manifolds",
	"60": "Probability theory and stochastic processes", "62": "Statistics", "65": "Numerical analysis", "68": "Computer science",
	"70": "Mechanics of particles and systems", "74": "Mechanics of deformable solids", "76": "Fluid mechanics",
	"78": "Optics, electromagnetic theory", "80": "Classical thermodynamics, heat transfer", "81": "Quantum theory",
	"82": "Statistical mechanics, structure of matter", "83": "Relativity and gravitational theory",
	"85": "Astronomy and astrophysics", "86": "Geophysics", "90": "Operations research, mathematical programming",
	"91": "Game theory, economics, finance, and other social and behavioral sciences", "92": "Biology and other natural sciences",
	"93": "Systems theory; control", "94": "Information and communication theory, circuits", "97": "Mathematics education",
}

func mscCodes() []string {
	codes := make([]string, 0, len(mscTop))
	for c := range mscTop {
		codes = append(codes, c)
	}
	slices.Sort(codes)
	return codes
}
