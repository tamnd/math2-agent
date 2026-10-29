package site

import (
	"cmp"
	"slices"

	"github.com/tamnd/math2-agent/card"
)

// levelCount is the number of nodes at one writing level.
type levelCount struct {
	Name, Word, Text, URL string
	Count                 int
}

// levelTexts say what each writing level needs (corpus K2.4).
var levelTexts = [...]string{
	"A tag, a kind, a name, an inventory row, and a same_as or a source.",
	"The main section of the kind, a source with a locator, and a body that passes the rules B01 to B03.",
	"The items of the card of the kind for W2, for example examples, non-examples and the edges to the related nodes.",
	"The items of the card for W3, for example the history and the explanations, and a person confirmed each section that an agent wrote.",
}

func (b *builder) levelCounts() []levelCount {
	if b.levels == nil {
		return nil
	}
	out := make([]levelCount, 0, len(levelTexts))
	for l := card.W0; l <= card.W3; l++ {
		out = append(out, levelCount{Name: l.String(), Word: l.Word(), Text: levelTexts[l], URL: b.url("levels/" + l.String() + "/"), Count: b.levels[l.String()]})
	}
	return out
}

// needCount is one item of the work lists, with the number of nodes that need it for their next level.
type needCount struct {
	What, Level string
	Count       int
}

// writeLevels writes the page of the writing levels and the list of the nodes at each level.
func (b *builder) writeLevels() error {
	if b.levels == nil {
		return nil
	}
	counts := map[[2]string]int{}
	for _, v := range b.order {
		if v.Writing == nil {
			continue
		}
		for _, n := range v.Writing.Next {
			counts[[2]string{v.NextLevel(), n.What}]++
		}
	}
	var needs []needCount
	for k, n := range counts {
		needs = append(needs, needCount{Level: k[0], What: k[1], Count: n})
	}
	slices.SortFunc(needs, func(x, y needCount) int {
		return cmp.Or(cmp.Compare(y.Count, x.Count), cmp.Compare(x.Level, y.Level), cmp.Compare(x.What, y.What))
	})
	d := struct {
		Levels []levelCount
		Total  int
		Needs  []needCount
		Spec   string // the URL of the spec of the cards, or ""
	}{Levels: b.levelCounts(), Total: len(b.order), Needs: needs[:min(len(needs), 15)]}
	for _, doc := range b.specs {
		if doc.Slug == "corpus/08-the-node-card" {
			d.Spec = b.url("spec/" + doc.Slug + "/")
		}
	}
	if err := b.writePage("levels", "levels", d, page{Title: "Writing levels", Description: "How complete the text of each node of Math 2.0 is.", Section: "nodes"}); err != nil {
		return err
	}
	type listData struct {
		Title, Text string
		Kinds       []kindCount
		Levels      []levelCount
		Nodes       []*nodeView
	}
	for i, lc := range d.Levels {
		var nodes []*nodeView
		for _, v := range b.order {
			if v.Writing != nil && v.Writing.Level == card.Level(i) {
				nodes = append(nodes, v)
			}
		}
		title := "Nodes at " + lc.Name + ", " + lc.Word
		if err := b.writePage("levels/"+lc.Name, "list", listData{Title: title, Text: lc.Text, Levels: d.Levels, Nodes: nodes}, page{Title: title, Description: lc.Text, Section: "nodes"}); err != nil {
			return err
		}
	}
	return nil
}

// HasLevels says if the site has the pages of the writing levels.
func (b *builder) HasLevels() bool { return b.levels != nil }
