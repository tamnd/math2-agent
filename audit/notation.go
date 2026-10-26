package audit

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/notation"
)

// notTree is the notation register and the register of conventions of the tree, read when a rule needs them.
type notTree struct {
	reg     *notation.Register // nil when the tree has no register
	err     error              // the error of a register that does not load
	conv    []*notation.Convention
	hasConv bool  // the tree has notation/conventions.yaml
	convErr error // the error of a register of conventions that does not load
	byID    map[string]*node.Node
}

func (a *auditor) notationOf() *notTree {
	if a.not != nil {
		return a.not
	}
	t := &notTree{byID: map[string]*node.Node{}}
	a.not = t
	for _, n := range a.nodes {
		t.byID[n.ID] = n
	}
	r, err := notation.Load(a.root)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		t.err = err
	default:
		t.reg = r
	}
	cs, err := notation.LoadConventions(a.root)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		t.hasConv, t.convErr = true, err
	default:
		t.hasConv, t.conv = true, cs
	}
	return t
}

// skip returns the outcome of a rule that reads the register when the register does not load or is not there.
func (t *notTree) skip() (outcome, bool) {
	var o outcome
	if t.err != nil {
		o.add(notation.Dir, 0, 0, "%v", t.err)
		return o, true
	}
	if t.reg == nil {
		o.reason = "the tree has no notation register in notation/"
		return o, true
	}
	return o, false
}

// topicsOf returns the topics of n in the form that the scope of a register entry names them: the tag and the aliases of each node in the topics field of n, and each MSC code of n at each level of MSC2020. For example the code 11A25 gives 11A25, 11Axx and 11-XX.
func (t *notTree) topicsOf(n *node.Node) map[string]bool {
	out := map[string]bool{}
	for _, tag := range n.List("topics") {
		out[tag] = true
		if topic := t.byID[tag]; topic != nil {
			for _, alias := range topic.List("aliases") {
				out[alias] = true
			}
		}
	}
	for _, code := range n.List("msc") {
		out[code] = true
		if len(code) >= 3 {
			out[code[:2]+"-XX"] = true
			if c := code[2]; c >= 'A' && c <= 'Z' {
				out[code[:3]+"xx"] = true
			}
		}
	}
	return out
}

// symbols returns the macros in the notation field of n.
func symbols(n *node.Node) []string {
	var out []string
	for _, m := range metaMaps(n.Meta["notation"]) {
		if macro := str(m["macro"]); macro != "" {
			out = append(out, macro)
		}
	}
	return out
}

// metaMaps returns the maps of a list value of the front matter.
func metaMaps(v any) []map[string]any {
	list, _ := v.([]any)
	var out []map[string]any
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// entryInScope reports whether a body with the topics can use the entry e.
func entryInScope(e *notation.Entry, topics map[string]bool) bool {
	return e.IsGlobal() || slices.ContainsFunc(e.Scopes(), func(s string) bool { return topics[s] })
}

// n01 checks that each macro of the tree in a body has an entry of the register in the scope of the node, or is in the notation field of the node (rule N01, corpus K10.4). B01 checks the control words that are not macros of the tree.
func n01(a *auditor) outcome {
	t := a.notationOf()
	o, stop := t.skip()
	if stop {
		return o
	}
	for _, n := range a.scopeNodes() {
		own := symbols(n)
		var topics map[string]bool
		seen := map[string]bool{}
		for _, c := range check.Commands(n) {
			macro := `\` + c.Name
			entries := t.reg.Lookup(macro)
			if _, ok := a.macros[c.Name]; !ok && len(entries) == 0 || seen[macro] {
				continue
			}
			seen[macro] = true
			o.checked++
			if slices.Contains(own, macro) {
				continue
			}
			if len(entries) == 0 {
				o.add(n.Path, c.Line, c.Col, "the macro %s has no entry in the notation register. Add an entry to a file in notation/, or list the macro in the notation field of the node that introduces it", macro)
				continue
			}
			if topics == nil {
				topics = t.topicsOf(n)
			}
			if !slices.ContainsFunc(entries, func(e *notation.Entry) bool { return entryInScope(e, topics) }) {
				o.add(n.Path, c.Line, c.Col, "the macro %s has the scope %s, and the node has none of these topics", macro, strings.Join(entries[0].Scopes(), ", "))
			}
		}
	}
	return o
}

// n02 checks that two entries that print the same glyph in scopes with a topic in common list each other in conflicts (rule N02). It also reports a conflict that names no entry.
func n02(a *auditor) outcome {
	t := a.notationOf()
	o, stop := t.skip()
	if stop {
		return o
	}
	ids := map[string]bool{}
	for _, e := range t.reg.Entries {
		ids[e.ID()] = true
	}
	glyphs := make([]string, len(t.reg.Entries))
	for i, e := range t.reg.Entries {
		glyphs[i] = notation.Glyph(e.LaTeX)
		o.checked++
		for _, id := range e.Conflicts {
			if !ids[id] {
				o.add(e.File, e.Line, 1, "the conflict %s of %s names no entry of the register. An id is the name of the file with no extension, a dot and the macro with no backslash, for example global.ip", id, e.Macro)
			}
		}
	}
	for i, e := range t.reg.Entries {
		for j := i + 1; j < len(t.reg.Entries); j++ {
			f := t.reg.Entries[j]
			if glyphs[i] != glyphs[j] || e.Macro == f.Macro || !overlap(e, f) {
				continue
			}
			if !slices.Contains(e.Conflicts, f.ID()) || !slices.Contains(f.Conflicts, e.ID()) {
				o.add(f.File, f.Line, 1, "%s and %s (%s:%d) print %s in scopes with a topic in common. List each in the conflicts field of the other", f.ID(), e.ID(), e.File, e.Line, glyphs[i])
			}
		}
	}
	return o
}

// overlap reports whether the scopes of e and f have a topic in common. The global scope has each topic.
func overlap(e, f *notation.Entry) bool {
	if e.IsGlobal() || f.IsGlobal() {
		return true
	}
	return slices.ContainsFunc(e.Scopes(), func(s string) bool { return slices.Contains(f.Scopes(), s) })
}

// curriculaDir holds the curricula (corpus K10.3).
const curriculaDir = "topics/curricula"

// curriculum is a file of topics/curricula. Each course lists its items in teaching order, and the node field of an item is the tag of its node.
type curriculum struct {
	Courses []struct {
		Course string      `yaml:"course"`
		Items  []yaml.Node `yaml:"items"`
	} `yaml:"courses"`
}

// n03 checks that no course of a curriculum puts a member before a node in its prerequisite closure (rule N03).
func n03(a *auditor) outcome {
	var o outcome
	paths, err := filepath.Glob(filepath.Join(a.root, curriculaDir, "*.yaml"))
	if err != nil {
		o.add(curriculaDir, 0, 0, "%v", err)
		return o
	}
	g := a.graphOf()
	for _, p := range paths {
		rel := curriculaDir + "/" + filepath.Base(p)
		data, err := os.ReadFile(p) // #nosec G304 -- the path is in the working tree, which the caller owns.
		if err != nil {
			o.add(rel, 0, 0, "%v", err)
			continue
		}
		var c curriculum
		if err := yaml.Unmarshal(data, &c); err != nil {
			o.add(rel, 0, 0, "%v", err)
			continue
		}
		for _, course := range c.Courses {
			var tags []string
			var lines []int
			pos := map[string]int{}
			for _, item := range course.Items {
				var it struct {
					Node string `yaml:"node"`
				}
				if item.Decode(&it) != nil || it.Node == "" {
					continue
				}
				if _, ok := pos[it.Node]; !ok {
					pos[it.Node] = len(tags)
				}
				tags = append(tags, it.Node)
				lines = append(lines, item.Line)
			}
			for i, tag := range tags {
				o.checked++
				alt, ok := g.Closure(tag, graph.MinDepth)
				if !ok {
					continue
				}
				var later []string
				for _, need := range alt.Nodes {
					if j, ok := pos[need]; ok && j > i && need != tag {
						later = append(later, need)
					}
				}
				if len(later) > 0 {
					o.add(rel, lines[i], 1, "the course %s puts %s before %s, which it needs", course.Course, tag, strings.Join(later, ", "))
				}
			}
		}
	}
	return o
}

// sourceIDs returns the identifiers of the sources of n in the form of the register of conventions: a URL, a book id such as isbn:9780471433347, or the key and the id, such as doi:10.1007/978-1-4612-0979-9.
func sourceIDs(n *node.Node) []string {
	var out []string
	for _, m := range metaMaps(n.Meta["sources"]) {
		for k, v := range m {
			id := str(v)
			switch {
			case k == "locator" || k == "note" || id == "":
			case k == "book" || k == "url":
				out = append(out, id)
			default:
				out = append(out, k+":"+id)
			}
		}
	}
	slices.Sort(out)
	return out
}

// n04 checks the conventions field of each node against the register of conventions, and reports a node that cites a source of an option other than the default, links the concept of the convention, and does not name the convention in its front matter (rule N04, corpus K10.5).
func n04(a *auditor) outcome {
	t := a.notationOf()
	var o outcome
	if t.convErr != nil {
		o.add(notation.ConventionsFile, 0, 0, "%v", t.convErr)
		return o
	}
	if !t.hasConv {
		o.reason = "the tree has no register of conventions in " + notation.ConventionsFile
		return o
	}
	byID := map[string]*notation.Convention{}
	type use struct {
		c      *notation.Convention
		option string
	}
	bySource := map[string][]use{}
	for _, c := range t.conv {
		byID[c.ID] = c
		for _, opt := range c.Options {
			if opt.Option == c.Default {
				continue
			}
			for _, s := range opt.Sources {
				bySource[s] = append(bySource[s], use{c, opt.Option})
			}
		}
	}
	links := map[string]map[string]bool{}
	for _, e := range a.graphOf().Edges {
		if links[e.From] == nil {
			links[e.From] = map[string]bool{}
		}
		links[e.From][e.To] = true
	}
	for _, n := range a.scopeNodes() {
		o.checked++
		named := map[string]bool{}
		for _, m := range metaMaps(n.Meta["conventions"]) {
			id, option := str(m["convention"]), str(m["option"])
			named[id] = true
			line, col := keyPos(n, "conventions")
			switch c := byID[id]; {
			case c == nil:
				o.add(n.Path, line, col, "the convention %s is not in %s", id, notation.ConventionsFile)
			case c.Option(option) == nil:
				o.add(n.Path, line, col, "the convention %s has no option %q. Its options are %s", id, option, optionNames(c))
			}
		}
		reported := map[string]bool{}
		for _, s := range sourceIDs(n) {
			for _, u := range bySource[s] {
				if named[u.c.ID] || reported[u.c.ID] {
					continue
				}
				if u.c.Concept != "" && n.ID != u.c.Concept && !links[n.ID][u.c.Concept] {
					continue
				}
				reported[u.c.ID] = true
				line, col := keyPos(n, "sources")
				o.add(n.Path, line, col, "the node cites %s, which uses the option %s of the convention %s, and the front matter does not name the convention. Add the convention and its option to the conventions field. The default is %s", s, u.option, u.c.ID, u.c.Default)
			}
		}
	}
	return o
}

func optionNames(c *notation.Convention) string {
	names := make([]string, len(c.Options))
	for i, opt := range c.Options {
		names[i] = opt.Option
	}
	return strings.Join(names, ", ")
}

// n05 reports a concept with no specializes edge in or out and no topics field (rule N05, corpus K10.2). Its MSC codes alone do not place it.
func n05(a *auditor) outcome {
	var o outcome
	g := a.graphOf()
	for _, n := range a.scopeNodes() {
		if n.Kind() != "concept" || n.Meta["retired"] != nil {
			continue
		}
		o.checked++
		if len(n.List("topics")) > 0 || len(g.Out(graph.Specializes, n.ID)) > 0 || len(g.In(graph.Specializes, n.ID)) > 0 {
			continue
		}
		line, col := keyPos(n, "msc", "name")
		o.add(n.Path, line, col, "the concept has no specializes edge and no topic. Add a specializes edge to a more general concept, or a topic")
	}
	return o
}

// n06 checks that each macro has one entry with a denotes that names a live node, and that the macro files are the output of the register (rule N06).
func n06(a *auditor) outcome {
	t := a.notationOf()
	o, stop := t.skip()
	if stop {
		return o
	}
	first := map[string]*notation.Entry{}
	for _, e := range t.reg.Entries {
		o.checked++
		if f := first[e.Macro]; f != nil {
			o.add(e.File, e.Line, 1, "the macro %s has an entry at %s:%d too. A macro has one entry and one meaning", e.Macro, f.File, f.Line)
			continue
		}
		first[e.Macro] = e
		switch n := t.byID[e.Denotes]; {
		case e.Denotes == "":
			o.add(e.File, e.Line, 1, "the macro %s has no denotes. Give the tag of the concept or the definition that it means", e.Macro)
		case n == nil:
			o.add(e.File, e.Line, 1, "the macro %s denotes %s, which is not a node", e.Macro, e.Denotes)
		case n.Meta["retired"] != nil:
			o.add(e.File, e.Line, 1, "the macro %s denotes %s, which is retired", e.Macro, e.Denotes)
		}
	}
	outputs := t.reg.Outputs()
	for _, name := range []string{notation.StyFile, notation.KaTeXFile} {
		o.checked++
		data, err := a.readFile(filepath.FromSlash(name))
		if err != nil {
			o.add(name, 0, 0, "%v", err)
			continue
		}
		if want := outputs[name]; !bytes.Equal(data, want) {
			line, col := firstDiff(data, want)
			o.add(name, line, col, "the file is not the output of the notation register. Run m2 notation build, and do not change the file by hand")
		}
	}
	return o
}

// firstDiff returns the line and the column of the first byte where a and b differ.
func firstDiff(a, b []byte) (line, col int) {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	return lineCol(a, i)
}

// n07 checks that each symbol in the notation field of a node has a register entry with the node as introduced_by (rule N07).
func n07(a *auditor) outcome {
	t := a.notationOf()
	o, stop := t.skip()
	if stop {
		return o
	}
	for _, n := range a.scopeNodes() {
		for _, macro := range symbols(n) {
			o.checked++
			entries := t.reg.Lookup(macro)
			if slices.ContainsFunc(entries, func(e *notation.Entry) bool { return e.IntroducedBy == n.ID }) {
				continue
			}
			line, col := keyPos(n, "notation")
			if len(entries) == 0 {
				o.add(n.Path, line, col, "the node introduces %s in its notation field, and the notation register has no entry for it", macro)
			} else {
				o.add(n.Path, line, col, "the node introduces %s in its notation field, and the entry %s has introduced_by %q", macro, entries[0].ID(), entries[0].IntroducedBy)
			}
		}
	}
	return o
}
