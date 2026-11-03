package write

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/agent/tools"
	axImport "github.com/tamnd/math2-agent/importer/arxiv"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/workspace"
)

// SourceItem is an item of a paper that a research node states (corpus arxiv AX9.2): an arxiv source of the node with a mention.
type SourceItem struct {
	ID      string // the mention, ax://paper/ID#TAG
	Paper   string
	Version int
	Locator string
	Class   string // the class of the version in tamnd/arxiv, or "" when the run has no checkout
	Licence string
	Title   string
	Authors string
	Text    string // the body, only for the class open
}

// SourceItems returns the items of papers that the arxiv sources of the node n name by their mention. arxiv is the checkout of tamnd/arxiv, which gives the text of an open item and the licence; with arxiv "" no item has text.
func SourceItems(_ *workspace.Workspace, n *node.Node, arxiv string) ([]SourceItem, error) {
	srcs, _ := n.Meta["sources"].([]any)
	var out []SourceItem
	seen := map[string]bool{}
	papers := map[string]*paperData{}
	for _, s := range srcs {
		m, _ := s.(map[string]any)
		id, _ := m["mention"].(string)
		ver, _ := m["arxiv"].(string)
		if id == "" || ver == "" || !strings.HasPrefix(id, "ax://paper/") || seen[id] {
			continue
		}
		seen[id] = true
		it := SourceItem{ID: id, Paper: axImport.PaperOf(id)}
		it.Locator, _ = m["locator"].(string)
		if _, v, ok := strings.Cut(ver, it.Paper+"v"); ok {
			it.Version, _ = strconv.Atoi(v)
		}
		if arxiv != "" {
			pd := papers[it.Paper]
			if pd == nil {
				var err error
				if pd, err = readPaper(arxiv, it.Paper); err != nil {
					return nil, err
				}
				papers[it.Paper] = pd
			}
			it.Title, it.Authors, it.Licence, it.Class = pd.title, pd.authors, pd.licence, pd.class
			if it.Class == "open" {
				it.Text = pd.bodies[id]
			}
		}
		out = append(out, it)
	}
	return out, nil
}

// SourceTexts returns the texts of papers that the brief gives for the items, for the run record (rule D17).
func SourceTexts(items []SourceItem) []prov.ContextText {
	var out []prov.ContextText
	for _, it := range items {
		if it.Class == "open" && it.Text != "" {
			out = append(out, prov.ContextText{Source: "arxiv", Version: fmt.Sprintf("%sv%d", it.Paper, it.Version), Item: it.ID, Class: it.Class, Licence: it.Licence})
		}
	}
	return out
}

// briefSources writes the part of the brief on the items of papers that the target states.
func briefSources(b *strings.Builder, items []SourceItem) {
	if len(items) == 0 {
		return
	}
	b.WriteString("The target states an item of a paper (corpus arxiv AX9.2). Write its main section from the item in math2's own words and in the house style, with the notation of the register and with links to the nodes of the terms that it uses. Make the statement self-contained: give the meaning of a local term of the paper in the statement, or link the node that defines it. Do not copy the sentences of the paper: the copy check (AX9.5) flags a copy. Keep the sources and the history of the front matter.\n\n")
	for _, it := range items {
		fmt.Fprintf(b, "Item %s, %s of arXiv:%sv%d", it.ID, it.Locator, it.Paper, it.Version)
		if it.Title != "" {
			fmt.Fprintf(b, ", %q by %s", it.Title, it.Authors)
		}
		if it.Class != "" {
			fmt.Fprintf(b, " (the class %s)", it.Class)
		}
		b.WriteString(".\n")
		if it.Class == "open" && it.Text != "" {
			fmt.Fprintf(b, "%s\n\n", tools.Data("arxiv:"+it.ID, it.Licence, cut(it.Text, maxProofChars)))
			continue
		}
		b.WriteString("The brief has no text of the item, because the version is not open or the run has no checkout of tamnd/arxiv. Write the statement only when you know it for certain from its name and its locator. Otherwise call report_stuck.\n\n")
	}
}
