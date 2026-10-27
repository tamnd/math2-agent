package pack

import (
	"strconv"
	"strings"
)

// Source is a work that the items of the volume cite in their field sources.
type Source struct {
	Number int    // the number of the work in the list of sources, from 1, in the order of the first citation
	Key    string // the citation key, the same in each output, for example src-doi-10.1007-978-3-642-
	System string // the identifier key of the node schema, for example doi or book
	ID     string
	Title  string // the identifier in words, for example arXiv:2101.00001
	URL    string // the page of the work, or "" when the identifier has no page
}

// Citation is a source of an item, with the place in the source.
type Citation struct {
	Number  int
	Key     string
	Locator string // for example Section 3.2, Theorem 8, or ""
}

// sourceKeys are the identifier keys of a source in the node schema, in the order of the schema.
var sourceKeys = []string{"zbmath", "arxiv", "doi", "book", "mr", "stacks", "kerodon", "mathoverflow", "mathse", "erdos", "formal_conjectures", "palomar", "hexagon", "url"}

// sources gives each item its citations and makes the list of sources of the document.
func (p *packer) sources() {
	keys := map[string]*Source{}
	used := map[string]bool{}
	p.d.walk(func(it *Item) {
		list, _ := p.t.nodes[it.Tag].Meta["sources"].([]any)
		for _, x := range list {
			m, _ := x.(map[string]any)
			system, id := "", ""
			for _, k := range sourceKeys {
				if v, _ := m[k].(string); v != "" {
					system, id = k, v
					break
				}
			}
			if system == "" {
				continue
			}
			s := keys[system+" "+id]
			if s == nil {
				s = &Source{Number: len(p.d.Sources) + 1, System: system, ID: id, Title: sourceTitle(system, id), URL: sourceURL(system, id)}
				s.Key = citationKey(system, id, used)
				keys[system+" "+id] = s
				p.d.Sources = append(p.d.Sources, *s)
			}
			locator, _ := m["locator"].(string)
			it.Sources = append(it.Sources, Citation{Number: s.Number, Key: s.Key, Locator: locator})
		}
	})
}

// citationKey returns a key for the source that is an XML name and a BibTeX key, and that no other source of the document has.
func citationKey(system, id string, used map[string]bool) string {
	var b strings.Builder
	b.WriteString("src-" + strings.ReplaceAll(system, "_", "-") + "-")
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	key := strings.TrimRight(b.String(), "-.")
	for i := 2; used[key]; i++ {
		key = strings.TrimSuffix(key, "-"+strconv.Itoa(i-1)) + "-" + strconv.Itoa(i)
	}
	used[key] = true
	return key
}

// sourceTitle returns the identifier of a source in words.
func sourceTitle(system, id string) string {
	switch system {
	case "zbmath":
		return "zbMATH " + id
	case "arxiv":
		return "arXiv:" + id
	case "doi":
		return "DOI " + id
	case "book":
		if isbn, ok := strings.CutPrefix(id, "isbn:"); ok {
			return "ISBN " + isbn
		}
		return id
	case "mr":
		return "MR " + strings.TrimPrefix(id, "MR")
	case "stacks":
		return "Stacks Project, tag " + id
	case "kerodon":
		return "Kerodon, tag " + id
	case "mathoverflow":
		return "MathOverflow, question " + id
	case "mathse":
		return "Mathematics Stack Exchange, question " + id
	case "erdos":
		return "Erdős problem " + id
	case "formal_conjectures":
		return "formal-conjectures, " + id
	case "palomar":
		return "Palomar, entry " + id
	case "hexagon":
		return "Hexagon, " + id
	}
	return id
}

// sourceURL returns the page of a source, or "" when the identifier has no page.
func sourceURL(system, id string) string {
	switch system {
	case "zbmath":
		return "https://zbmath.org/?q=an:" + id
	case "arxiv":
		return "https://arxiv.org/abs/" + id
	case "doi":
		return "https://doi.org/" + id
	case "mr":
		return "https://mathscinet.ams.org/mathscinet-getitem?mr=" + strings.TrimPrefix(id, "MR")
	case "stacks":
		return "https://stacks.math.columbia.edu/tag/" + id
	case "kerodon":
		return "https://kerodon.net/tag/" + id
	case "mathoverflow":
		return "https://mathoverflow.net/q/" + id
	case "mathse":
		return "https://math.stackexchange.com/q/" + id
	case "erdos":
		return "https://www.erdosproblems.com/" + id
	case "url":
		if strings.HasPrefix(id, "https://") || strings.HasPrefix(id, "http://") {
			return id
		}
	}
	return ""
}
