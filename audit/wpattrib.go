package audit

import (
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/wpsource"
)

// The notes of a Wikipedia source item: the node has the text of the revision as it is, or restyled (corpus wikipedia WP9.2 and WP9.5).
const (
	noteAdopted = "adopted"
	noteAdapted = "adapted"
)

// y11 reports a node with text adopted or adapted from Wikipedia that does not have the attribution of WP9.3 in its sources (rule Y11).
//
// A source item with the key wikipedia must name the revision, give the title and the section in its locator, have the note adopted or adapted, name the mention in mention, and list in sections the sections of the node that it filled. Each of these sections must be in the body. A source item with the note adopted or adapted and a URL of Wikipedia must use the key wikipedia. The run record and the release attribution file are outside the node, and the release audit checks them.
func y11(a *auditor) outcome {
	var o outcome
	for _, n := range a.scopeNodes() {
		if retired(n) {
			continue
		}
		list, _ := n.Meta["sources"].([]any)
		var body []string
		for i, v := range list {
			m, _ := v.(map[string]any)
			note := strings.TrimSpace(str(m["note"]))
			value := str(m[wpsource.Key])
			adopted := note == noteAdopted || note == noteAdapted
			if value == "" {
				if adopted && strings.Contains(str(m["url"]), "wikipedia.org/") {
					o.checked++
					line, col := keyPos(n, "sources")
					o.add(n.Path, line, col, "source %d has the note %s and a URL of Wikipedia. Use the key wikipedia with the page id and the revision", i+1, note)
				}
				continue
			}
			o.checked++
			line, col := keyPos(n, "sources")
			if _, err := wpsource.Parse(value); err != nil {
				o.add(n.Path, line, col, "source %d: %v", i+1, err)
			}
			if !adopted {
				o.add(n.Path, line, col, "source %d is a Wikipedia revision with the note %q. The note must be adopted or adapted", i+1, note)
			}
			if strings.TrimSpace(str(m["locator"])) == "" {
				o.add(n.Path, line, col, "source %d is a Wikipedia revision with no locator. Give the title and the section of the article", i+1)
			}
			if strings.TrimSpace(str(m["mention"])) == "" {
				o.add(n.Path, line, col, "source %d is a Wikipedia revision with no mention. Give the id of the mention that the text came from", i+1)
			}
			secs := strs(m["sections"])
			if len(secs) == 0 {
				o.add(n.Path, line, col, "source %d is a Wikipedia revision with no sections. List the sections of the node that it filled", i+1)
			}
			if body == nil {
				body = sectionNames(n)
			}
			for _, s := range secs {
				if !slices.Contains(body, s) {
					o.add(n.Path, line, col, "source %d names the section %q, and the body has no section of that name", i+1, s)
				}
			}
		}
	}
	return o
}

// sectionNames gives the names of the sections of the body of n.
func sectionNames(n *node.Node) []string {
	out := []string{}
	for _, s := range check.Sections(n) {
		out = append(out, s.Name)
	}
	return out
}

// strs gives the strings of a list value, and nil for a value that is not a list.
func strs(v any) []string {
	list, _ := v.([]any)
	var out []string
	for _, x := range list {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
