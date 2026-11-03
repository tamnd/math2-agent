package release

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/tag"
	"github.com/tamnd/math2-agent/wpsource"
)

// AttributionFile is the release attribution file of corpus wikipedia WP9.3, relative to the root of the tree. Its YAML header is the machine form, and its body is the text for people.
const AttributionFile = "ATTRIBUTION.md"

// AttributionLicence is the licence of the text that a node adopted or adapted from Wikipedia.
const AttributionLicence = "CC-BY-SA-4.0"

// Article is the attribution of one Wikipedia article whose text a live node adopted or adapted.
type Article struct {
	Page      int64      `yaml:"page"`
	Title     string     `yaml:"title"`
	History   string     `yaml:"history"` // the history page, which lists the authors
	Revisions []Revision `yaml:"revisions"`
}

// Revision is one revision of an article and the nodes that have text from it.
type Revision struct {
	Revision int64    `yaml:"revision"`
	URL      string   `yaml:"url"` // the permanent link of the revision
	Nodes    []string `yaml:"nodes,flow"`
}

// Attribution gives the articles whose text the live minted nodes adopted or adapted, in the order of the titles. A source item counts when it has the key wikipedia and the note adopted or adapted. The title is the locator up to ", section".
func Attribution(nodes []*node.Node) []Article {
	byPage := map[int64]*Article{}
	for _, n := range nodes {
		if _, gone := n.Meta["retired"]; gone || tag.IsProvisional(n.ID) {
			continue
		}
		list, _ := n.Meta["sources"].([]any)
		for _, v := range list {
			m, _ := v.(map[string]any)
			value, _ := m[wpsource.Key].(string)
			note, _ := m["note"].(string)
			if note = strings.TrimSpace(note); note != "adopted" && note != "adapted" {
				continue
			}
			ref, err := wpsource.Parse(value)
			if err != nil {
				continue
			}
			a := byPage[ref.Page]
			if a == nil {
				a = &Article{Page: ref.Page, History: ref.HistoryURL()}
				byPage[ref.Page] = a
			}
			if loc, _ := m["locator"].(string); a.Title == "" {
				a.Title, _, _ = strings.Cut(strings.TrimSpace(loc), ", section")
			}
			i := slices.IndexFunc(a.Revisions, func(r Revision) bool { return r.Revision == ref.Revision })
			if i < 0 {
				a.Revisions = append(a.Revisions, Revision{Revision: ref.Revision, URL: ref.URL()})
				i = len(a.Revisions) - 1
			}
			if !slices.Contains(a.Revisions[i].Nodes, n.ID) {
				a.Revisions[i].Nodes = append(a.Revisions[i].Nodes, n.ID)
			}
		}
	}
	out := make([]Article, 0, len(byPage))
	for _, a := range byPage {
		slices.SortFunc(a.Revisions, func(x, y Revision) int { return cmp.Compare(x.Revision, y.Revision) })
		for i := range a.Revisions {
			slices.Sort(a.Revisions[i].Nodes)
		}
		out = append(out, *a)
	}
	slices.SortFunc(out, func(x, y Article) int { return cmp.Or(strings.Compare(x.Title, y.Title), cmp.Compare(x.Page, y.Page)) })
	return out
}

// AttributionMarkdown gives the release attribution file: a YAML header with the articles, and a list of the articles for people.
func AttributionMarkdown(arts []Article) ([]byte, error) {
	nodes := map[string]bool{}
	for _, a := range arts {
		for _, r := range a.Revisions {
			for _, id := range r.Nodes {
				nodes[id] = true
			}
		}
	}
	head := struct {
		Source   string    `yaml:"source"`
		Licence  string    `yaml:"licence"`
		Articles int       `yaml:"articles"`
		Nodes    int       `yaml:"nodes"`
		List     []Article `yaml:"attribution"`
	}{"wikipedia", AttributionLicence, len(arts), len(nodes), arts}
	if head.List == nil {
		head.List = []Article{}
	}
	var y bytes.Buffer
	enc := yaml.NewEncoder(&y)
	enc.SetIndent(2)
	if err := enc.Encode(head); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	b.Write(y.Bytes())
	b.WriteString("---\n\n# Attribution\n\n")
	fmt.Fprintf(&b, "Some nodes of math2 have text that was adopted or adapted from articles of the English Wikipedia. This file lists each of these articles (corpus wikipedia WP9.3). The text of the articles is under the licence CC BY-SA 4.0 (https://creativecommons.org/licenses/by-sa/4.0/), and the nodes that use it are under the same licence. The history page of each article lists its authors. m2 release attribution writes this file from the sources of the nodes, so do not edit it.\n\n")
	fmt.Fprintf(&b, "%d articles give text to %d nodes.\n", len(arts), len(nodes))
	for _, a := range arts {
		fmt.Fprintf(&b, "\n## %s\n\nPage %d, history %s, licence CC BY-SA 4.0.\n\n", a.Title, a.Page, a.History)
		for _, r := range a.Revisions {
			fmt.Fprintf(&b, "- Revision %d (%s): %s\n", r.Revision, r.URL, strings.Join(r.Nodes, ", "))
		}
	}
	return b.Bytes(), nil
}
