package site

import (
	"cmp"
	"fmt"
	"html/template"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/status"
)

// maxFigure is the largest number of nodes that a graph figure shows. A larger closure shows only as a list.
const maxFigure = 40

// dagItem is one node of a closure or a blueprint.
type dagItem struct {
	link
	Kind     string
	Evidence string // the short name of the evidence level, or ""
	Word     string // the name of the evidence level, or ""
	Member   bool   // the node is a member of the topic of a blueprint
	Root     bool   // the node is the node of the page
	layer    int
	x, y     int
	deps     []string
}

// dagView is a set of nodes in the order of their dependencies, with a figure when the set is small.
type dagView struct {
	ID     string     // the id of the figure caption, unique on its page
	Wide   bool       // the figure takes the full width of the page, below the infobox
	Items  []*dagItem // the foundations first
	SVG    template.HTML
	Legend []legendItem
	Proofs []link // the proofs that the closure takes
}

type legendItem struct{ Class, Label string }

// statusOf returns the status of the node with the tag, or nil when the build gave no status.
func (b *builder) statusOf(tag string) *status.Status {
	if b.status == nil {
		return nil
	}
	if s, ok := b.status[tag]; ok {
		return &s
	}
	return nil
}

// deps returns the nodes in set that the node with the tag needs: each node that it uses and, for a statement, the proofs in set that prove it.
func (b *builder) deps(tag string, set map[string]bool) []string {
	var out []string
	for _, e := range b.graph.Out(graph.Uses, tag) {
		if set[e.To] && e.To != tag {
			out = append(out, e.To)
		}
	}
	for _, e := range b.graph.In(graph.Proves, tag) {
		if set[e.From] {
			out = append(out, e.From)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// The most boxes in one row of a figure. A closure has 3, so that it fits next to the infobox. A blueprint has 5, and it goes below the infobox.
const (
	narrowRow = 3
	wideRow   = 5
)

// dag lays out the nodes in tags. root is the node of the page, or "". members marks the nodes of a topic. wide gives a figure with the full width of the page.
func (b *builder) dag(id string, tags []string, root string, members map[string]bool, wide bool) *dagView {
	set := map[string]bool{}
	for _, t := range tags {
		set[t] = true
	}
	items := map[string]*dagItem{}
	for _, t := range tags {
		it := &dagItem{link: b.nodeLink(t), Root: t == root, Member: members[t], deps: b.deps(t, set)}
		if v, ok := b.nodes[t]; ok {
			it.Kind = v.Kind
		}
		if s := b.statusOf(t); s != nil && s.Evidence != status.None {
			it.Evidence, it.Word = s.Evidence.String(), s.Evidence.Word()
		}
		items[t] = it
	}
	// The layer of a node is the length of the longest chain of dependencies below it. A cycle cannot occur in a closure, and the guard stops a cycle in a blueprint.
	var layer func(t string, seen map[string]bool) int
	layer = func(t string, seen map[string]bool) int {
		it := items[t]
		if it.layer > 0 || seen[t] {
			return it.layer
		}
		seen[t] = true
		l := 0
		for _, d := range it.deps {
			l = max(l, layer(d, seen)+1)
		}
		it.layer = l
		return l
	}
	for _, t := range tags {
		layer(t, map[string]bool{})
	}
	var layers [][]*dagItem
	for _, t := range tags {
		it := items[t]
		for len(layers) <= it.layer {
			layers = append(layers, nil)
		}
		layers[it.layer] = append(layers[it.layer], it)
	}
	// Order each layer by the mean position of the dependencies of each node, then by name.
	pos := map[string]float64{}
	for _, l := range layers {
		mean := func(it *dagItem) float64 {
			if len(it.deps) == 0 {
				return 0
			}
			s := 0.0
			for _, d := range it.deps {
				s += pos[d]
			}
			return s / float64(len(it.deps))
		}
		slices.SortStableFunc(l, func(x, y *dagItem) int {
			return cmp.Or(cmp.Compare(mean(x), mean(y)), strings.Compare(x.Text, y.Text), strings.Compare(x.Tag, y.Tag))
		})
		for i, it := range l {
			pos[it.Tag] = float64(i)
		}
	}
	v := &dagView{ID: id, Wide: wide}
	for _, l := range layers {
		v.Items = append(v.Items, l...)
	}
	if len(tags) > 1 && len(tags) <= maxFigure {
		perRow := narrowRow
		if wide {
			perRow = wideRow
		}
		v.SVG = svg(id, layers, perRow)
	}
	seen := map[string]bool{}
	for _, it := range v.Items {
		c, l := evidenceClass(it.Evidence)
		if !seen[c] {
			seen[c] = true
			v.Legend = append(v.Legend, legendItem{c, l})
		}
	}
	slices.SortFunc(v.Legend, func(x, y legendItem) int { return strings.Compare(x.Class, y.Class) })
	return v
}

// evidenceClass returns the class and the legend label of an evidence level.
func evidenceClass(e string) (string, string) {
	switch e {
	case "":
		return "ev-none", "no evidence level"
	case "E⊥":
		return "ev-refuted", "E⊥ refuted"
	case "E0":
		return "ev-e0", "E0 open"
	}
	l := status.None
	_ = l.UnmarshalText([]byte(e))
	return "ev-" + strings.ToLower(e), e + " " + l.Word()
}

// The size of the boxes of a figure, in SVG units.
const (
	boxW, boxH = 176, 46
	gapX, gapY = 16, 36
	pad        = 4
)

// svg draws the layers from the foundations at the top to the node of the page at the bottom. A layer with more than perRow nodes takes more than one row. Each box is a link to its node, and its title gives the full name.
func svg(id string, layers [][]*dagItem, perRow int) template.HTML {
	var rows [][]*dagItem
	for _, l := range layers {
		for len(l) > perRow {
			rows = append(rows, l[:perRow])
			l = l[perRow:]
		}
		rows = append(rows, l)
	}
	layers = rows
	width := 0
	for _, l := range layers {
		width = max(width, len(l)*(boxW+gapX)-gapX)
	}
	height := len(layers)*(boxH+gapY) - gapY
	at := map[string]*dagItem{}
	for i, l := range layers {
		w := len(l)*(boxW+gapX) - gapX
		for j, it := range l {
			it.x = pad + (width-w)/2 + j*(boxW+gapX)
			it.y = pad + i*(boxH+gapY)
			at[it.Tag] = it
		}
	}
	var s strings.Builder
	fmt.Fprintf(&s, `<svg class="dag" viewBox="0 0 %d %d" width="%d" role="img" aria-labelledby="%s" xmlns="http://www.w3.org/2000/svg">`, width+2*pad, height+2*pad, width+2*pad, id)
	s.WriteString(`<g class="edges">`)
	for _, l := range layers {
		for _, it := range l {
			for _, d := range it.deps {
				dep := at[d]
				x1, y1 := it.x+boxW/2, it.y
				x2, y2 := dep.x+boxW/2, dep.y+boxH
				my := (y1 + y2) / 2
				fmt.Fprintf(&s, `<path d="M%d %dC%d %d %d %d %d %d"/>`, x1, y1, x1, my, x2, my, x2, y2)
			}
		}
	}
	s.WriteString(`</g>`)
	for _, l := range layers {
		for _, it := range l {
			c, label := evidenceClass(it.Evidence)
			cls := "box " + c
			if it.Root {
				cls += " root"
			}
			if it.Member {
				cls += " member"
			}
			sub := it.Kind
			if it.Evidence != "" {
				sub += " · " + it.Evidence
			}
			fmt.Fprintf(&s, `<a href="%s" class="%s"><title>%s, %s, %s</title>`, template.HTMLEscapeString(it.URL), cls, template.HTMLEscapeString(it.Text), template.HTMLEscapeString(it.Kind), template.HTMLEscapeString(label))
			fmt.Fprintf(&s, `<rect x="%d" y="%d" width="%d" height="%d" rx="4"/>`, it.x, it.y, boxW, boxH)
			fmt.Fprintf(&s, `<rect class="ev" x="%d" y="%d" width="6" height="%d" rx="2"/>`, it.x+3, it.y+3, boxH-6)
			fmt.Fprintf(&s, `<text x="%d" y="%d">%s</text>`, it.x+16, it.y+19, template.HTMLEscapeString(clip(it.Text, 22)))
			fmt.Fprintf(&s, `<text class="sub" x="%d" y="%d">%s</text>`, it.x+16, it.y+36, template.HTMLEscapeString(sub))
			s.WriteString(`</a>`)
		}
	}
	s.WriteString(`</svg>`)
	return template.HTML(s.String()) // #nosec G203 -- each value in the markup is escaped above.
}

// clip shortens s to n runes, with an ellipsis.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// explore adds the closure, the dependents, the status and, for a topic, the blueprint to each node page.
func (b *builder) explore() {
	for _, v := range b.order {
		v.Status = b.statusOf(v.Tag)
		if alt, ok := b.graph.Closure(v.Tag, graph.MinDepth); ok && len(alt.Nodes) > 1 {
			v.Closure = b.dag("closure-caption", alt.Nodes, v.Tag, nil, false)
			for _, p := range alt.Proofs {
				v.Closure.Proofs = append(v.Closure.Proofs, b.nodeLink(p))
			}
		}
		for _, t := range b.graph.Dependents(v.Tag) {
			v.UsedBy = append(v.UsedBy, b.nodeLink(t))
		}
		if v.Kind == "topic" {
			members := map[string]bool{}
			set := map[string]bool{}
			for _, e := range b.graph.In(graph.PartOf, v.Tag) {
				members[e.From] = true
				set[e.From] = true
				if alt, ok := b.graph.Closure(e.From, graph.MinDepth); ok {
					for _, t := range alt.Nodes {
						set[t] = true
					}
				}
			}
			if len(set) > 0 {
				tags := make([]string, 0, len(set))
				for t := range set {
					tags = append(tags, t)
				}
				slices.Sort(tags)
				v.Blueprint = b.dag("blueprint-caption", tags, "", members, true)
			}
		}
	}
}

// frontier is the data of the frontier page.
type frontier struct {
	Unproved, Open []*dagItem
	HasStatus      bool
}

// frontierOf returns the statements with no proof in the graph, and the conjectures and problems that no node resolves.
func (b *builder) frontierOf() frontier {
	f := frontier{HasStatus: b.status != nil}
	for _, v := range b.order {
		if v.Retired != nil {
			continue
		}
		it := &dagItem{link: b.nodeLink(v.Tag), Kind: v.Kind}
		if s := b.statusOf(v.Tag); s != nil && s.Evidence != status.None {
			it.Evidence, it.Word = s.Evidence.String(), s.Evidence.Word()
		}
		switch v.Kind {
		case "statement":
			if len(b.graph.In(graph.Proves, v.Tag)) == 0 {
				f.Unproved = append(f.Unproved, it)
			}
		case "conjecture", "problem":
			if len(b.graph.In(graph.Resolves, v.Tag)) == 0 && len(b.graph.Out(graph.ResolvedBy, v.Tag)) == 0 {
				f.Open = append(f.Open, it)
			}
		}
	}
	return f
}

func (b *builder) writeExplore() error {
	f := b.frontierOf()
	p := page{Title: "Frontier", Description: "The statements of Math 2.0 with no proof, and the open conjectures and problems.", Section: "explore"}
	return b.writePage("explore/frontier", "frontier", f, p)
}
