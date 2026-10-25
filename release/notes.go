package release

import (
	"bufio"
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/node"
)

// Snapshot is the state of the tree at one release.
type Snapshot struct {
	Manifest *Manifest
	Levels   Levels
	Nodes    []*node.Node // the node files at the commit of the manifest
	Bindings []string     // the records of registers/bindings.reg at the commit of the manifest
}

// Load reads the snapshot of a release from the tree root. The manifest and the levels come from the working tree. The node files and the register of the bindings come from the commit of the manifest, with git.
func Load(ctx context.Context, root, release string) (*Snapshot, error) {
	m, err := Read(filepath.Join(root, filepath.FromSlash(ManifestFile(release))))
	if err != nil {
		return nil, err
	}
	s := &Snapshot{Manifest: m}
	if s.Levels, err = ReadLevels(filepath.Join(root, filepath.FromSlash(LevelsFile(release)))); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "m2-notes-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := checkout(ctx, root, m.Commit, tmp, "nodes"); err != nil {
		return nil, err
	}
	if s.Nodes, err = node.Load(tmp); err != nil {
		return nil, err
	}
	const register = "registers/bindings.reg"
	if _, err := git(root, "cat-file", "-e", m.Commit+":"+register); err == nil {
		text, err := git(root, "show", m.Commit+":"+register)
		if err != nil {
			return nil, err
		}
		for l := range strings.SplitSeq(text, "\n") {
			if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
				s.Bindings = append(s.Bindings, l)
			}
		}
	}
	return s, nil
}

// Notes are the release notes of spec 06.2. [Diff] makes all the parts except the summary, which a person writes.
type Notes struct {
	From, To string // the names of the two releases. From is empty for the first release.
	Summary  string // the prose of a person, in Markdown
	Counts   []Change
	Topics   []TopicCount
	Moves    []Move    // the nodes that moved evidence level
	Resolved []Resolve // the conjectures and the problems that a statement settles now
	Retired  []Retire
	Rebinds  []Rebind // the binding changes that Mathlib renames and refactors caused
}

// Change is the change of one count of the manifest.
type Change struct {
	Count         string // nodes_live, by_kind.KIND or by_evidence.LEVEL
	Before, After int
	Cause         string // the cause of the change. It is empty when the node files do not give the change.
}

// TopicCount is the number of live nodes of one topic in the two releases.
type TopicCount struct {
	Tag, Name     string
	Before, After int
}

// Move is a node that moved evidence level. An empty level means that the node had no level.
type Move struct {
	Tag, Name     string
	Before, After string
}

// Resolve is a conjecture or a problem that has a resolved_by edge now and had none at the release before.
type Resolve struct {
	Tag, Name string
	By        []string
}

// Retire is a node that was live at the release before and is not live now.
type Retire struct {
	Tag, Name  string
	Reason     string // the reason of the retired field, or empty when the node file is gone
	Successors []string
}

// Rebind is a record of the register of the bindings that a Mathlib bump added.
type Rebind struct {
	Tag, Old, New, Cause, Mathlib string
}

// Changes returns each count of the manifest cur that is different in the manifest prev, in the order of the count names. The causes are empty. prev is nil for the first release, and then each count starts at 0.
func Changes(prev, cur *Manifest) []Change {
	flat := func(m *Manifest) map[string]int {
		out := map[string]int{}
		if m == nil {
			return out
		}
		out["nodes_live"] = m.Counts.NodesLive
		for k, v := range m.Counts.ByKind {
			out["by_kind."+k] = v
		}
		for k, v := range m.Counts.ByEvidence {
			out["by_evidence."+k] = v
		}
		return out
	}
	a, b := flat(prev), flat(cur)
	keys := slices.Sorted(maps.Keys(b))
	for k := range a {
		if _, ok := b[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, func(x, y string) int {
		if (x == "nodes_live") != (y == "nodes_live") {
			if x == "nodes_live" {
				return -1
			}
			return 1
		}
		return strings.Compare(x, y)
	})
	var out []Change
	for _, k := range keys {
		if a[k] != b[k] {
			out = append(out, Change{Count: k, Before: a[k], After: b[k]})
		}
	}
	return out
}

// Diff makes the notes of the release to from the release before it. from is nil for the first release.
func Diff(from, to *Snapshot) *Notes {
	n := &Notes{To: to.Manifest.Release}
	var prev *Manifest
	old := index(nil)
	oldLevels := Levels{}
	if from != nil {
		n.From, prev, old, oldLevels = from.Manifest.Release, from.Manifest, index(from.Nodes), from.Levels
	}
	cur := index(to.Nodes)

	added, gone, kept := []string{}, []string{}, []string{}
	for t, nd := range cur.live {
		if _, ok := old.live[t]; !ok {
			added = append(added, t)
		} else if nd != nil {
			kept = append(kept, t)
		}
	}
	for t := range old.live {
		if _, ok := cur.live[t]; !ok {
			gone = append(gone, t)
		}
	}
	slices.Sort(added)
	slices.Sort(gone)
	slices.Sort(kept)

	for _, c := range Changes(prev, to.Manifest) {
		var p []int // new, in, out, retired
		switch {
		case c.Count == "nodes_live":
			p = []int{len(added), 0, 0, len(gone)}
		case strings.HasPrefix(c.Count, "by_kind."):
			k := strings.TrimPrefix(c.Count, "by_kind.")
			p = split(added, gone, kept, func(t string) bool { return old.live[t].Kind() == k }, func(t string) bool { return cur.live[t].Kind() == k })
		default:
			e := strings.TrimPrefix(c.Count, "by_evidence.")
			p = split(added, gone, kept, func(t string) bool { return oldLevels[t].Evidence == e }, func(t string) bool { return to.Levels[t].Evidence == e })
		}
		if c.Before+p[0]+p[1]-p[2]-p[3] == c.After {
			c.Cause = cause(c.Count, p)
		}
		n.Counts = append(n.Counts, c)
	}

	topics := map[string]string{}
	for _, ix := range []nodes{old, cur} {
		for t, nd := range ix.all {
			if nd.Kind() == "topic" {
				topics[t] = nd.Name()
			}
		}
	}
	for _, t := range slices.Sorted(maps.Keys(topics)) {
		if b, a := old.members(t), cur.members(t); b != a {
			n.Topics = append(n.Topics, TopicCount{Tag: t, Name: topics[t], Before: b, After: a})
		}
	}

	for _, t := range kept {
		if b, a := oldLevels[t].Evidence, to.Levels[t].Evidence; b != a {
			n.Moves = append(n.Moves, Move{Tag: t, Name: cur.live[t].Name(), Before: b, After: a})
		}
	}
	for _, t := range slices.Sorted(maps.Keys(cur.live)) {
		nd := cur.live[t]
		if k := nd.Kind(); k != "conjecture" && k != "problem" {
			continue
		}
		by := nd.List("resolved_by")
		if was := old.all[t]; len(by) > 0 && (was == nil || len(was.List("resolved_by")) == 0) {
			n.Resolved = append(n.Resolved, Resolve{Tag: t, Name: nd.Name(), By: by})
		}
	}
	for _, t := range gone {
		r := Retire{Tag: t, Name: old.live[t].Name()}
		if nd := cur.all[t]; nd != nil {
			if m, ok := nd.Meta["retired"].(map[string]any); ok {
				r.Reason, _ = m["reason"].(string)
				r.Successors = asList(m["successor"])
			}
		}
		n.Retired = append(n.Retired, r)
	}

	was := map[string]bool{}
	if from != nil {
		for _, l := range from.Bindings {
			was[l] = true
		}
	}
	for _, l := range to.Bindings {
		f := strings.Fields(l)
		if was[l] || len(f) < 6 || f[4] != "rename" && f[4] != "refactor" {
			continue
		}
		n.Rebinds = append(n.Rebinds, Rebind{Tag: f[0], Old: f[2], New: f[3], Cause: f[4], Mathlib: f[5]})
	}
	return n
}

// nodes indexes the node files of a release by tag.
type nodes struct {
	all  map[string]*node.Node
	live map[string]*node.Node // the nodes that are not retired
}

func index(ns []*node.Node) nodes {
	ix := nodes{all: map[string]*node.Node{}, live: map[string]*node.Node{}}
	for _, nd := range ns {
		ix.all[nd.ID] = nd
		if nd.Meta["retired"] == nil {
			ix.live[nd.ID] = nd
		}
	}
	return ix
}

// members returns the number of live nodes that are part of the topic.
func (ix nodes) members(topic string) int {
	n := 0
	for _, nd := range ix.live {
		if slices.Contains(nd.List("topics"), topic) {
			n++
		}
	}
	return n
}

// split counts the nodes of a count: the new nodes in it, the kept nodes that came in and went out, and the gone nodes that were in it.
func split(added, gone, kept []string, wasIn, isIn func(string) bool) []int {
	p := make([]int, 4)
	for _, t := range added {
		if isIn(t) {
			p[0]++
		}
	}
	for _, t := range kept {
		switch a, b := wasIn(t), isIn(t); {
		case !a && b:
			p[1]++
		case a && !b:
			p[2]++
		}
	}
	for _, t := range gone {
		if wasIn(t) {
			p[3]++
		}
	}
	return p
}

// cause gives the parts of a change in words.
func cause(count string, p []int) string {
	in, out := [2]string{"node came in", "nodes came in"}, [2]string{"node went out", "nodes went out"}
	switch {
	case strings.HasPrefix(count, "by_kind."):
		in, out = [2]string{"node changed to this kind", "nodes changed to this kind"}, [2]string{"node changed to another kind", "nodes changed to another kind"}
	case strings.HasPrefix(count, "by_evidence."):
		in, out = [2]string{"node moved to this level", "nodes moved to this level"}, [2]string{"node moved to another level", "nodes moved to another level"}
	}
	words := [][2]string{{"new node", "new nodes"}, in, out, {"retired node", "retired nodes"}}
	var s []string
	for i, v := range p {
		if v > 0 {
			s = append(s, plural(v, words[i][0], words[i][1]))
		}
	}
	return strings.Join(s, ", ")
}

func plural(n int, one, more string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, more)
}

func asList(v any) []string {
	var out []string
	switch v := v.(type) {
	case string:
		out = append(out, v)
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// The headings of the sections of the notes.
const (
	summaryHeading = "## Summary"
	countsHeading  = "## Counts"
)

// summaryHint is the summary of new notes.
const summaryHint = "<!-- Write the summary of the release here. m2 release notes keeps this section when it writes the notes again. -->"

// Markdown returns the notes as Markdown.
func (n *Notes) Markdown() []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# math2 %s\n\n", n.To)
	if n.From == "" {
		b.WriteString("This is the first release of math2.")
	} else {
		fmt.Fprintf(&b, "These are the changes from %s to %s.", n.From, n.To)
	}
	b.WriteString(" m2 release notes writes each section after the summary from the manifests, the levels and the node files of the two releases. Rule R03 checks that the counts table gives the cause of each change in a count of the manifest.\n\n")

	summary := strings.TrimSpace(n.Summary)
	if summary == "" {
		summary = summaryHint
	}
	fmt.Fprintf(&b, "%s\n\n%s\n\n", summaryHeading, summary)

	fmt.Fprintf(&b, "%s\n\n", countsHeading)
	if len(n.Counts) == 0 {
		b.WriteString("No count changed.\n\n")
	} else {
		b.WriteString("| Count | Before | After | Change | Cause |\n|---|---|---|---|---|\n")
		for _, c := range n.Counts {
			fmt.Fprintf(&b, "| %s | %d | %d | %+d | %s |\n", c.Count, c.Before, c.After, c.After-c.Before, cell(c.Cause))
		}
		b.WriteString("\n")
	}

	section := func(title, none string, empty bool, table func()) {
		fmt.Fprintf(&b, "## %s\n\n", title)
		if empty {
			b.WriteString(none + "\n\n")
			return
		}
		table()
		b.WriteString("\n")
	}
	section("Topics", "No topic changed its number of nodes.", len(n.Topics) == 0, func() {
		b.WriteString("| Topic | Name | Before | After |\n|---|---|---|---|\n")
		for _, t := range n.Topics {
			fmt.Fprintf(&b, "| %s | %s | %d | %d |\n", t.Tag, cell(t.Name), t.Before, t.After)
		}
	})
	section("Evidence", "No node moved evidence level.", len(n.Moves) == 0, func() {
		b.WriteString("| Node | Name | Before | After |\n|---|---|---|---|\n")
		for _, m := range n.Moves {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", m.Tag, cell(m.Name), orNone(m.Before), orNone(m.After))
		}
	})
	section("Resolved", "No conjecture and no problem was resolved.", len(n.Resolved) == 0, func() {
		b.WriteString("| Node | Name | Resolved by |\n|---|---|---|\n")
		for _, r := range n.Resolved {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", r.Tag, cell(r.Name), strings.Join(r.By, ", "))
		}
	})
	section("Retired", "No node was retired.", len(n.Retired) == 0, func() {
		b.WriteString("| Node | Name | Reason | Successors |\n|---|---|---|---|\n")
		for _, r := range n.Retired {
			reason := r.Reason
			if reason == "" {
				reason = "the node file is gone"
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", r.Tag, cell(r.Name), reason, strings.Join(r.Successors, ", "))
		}
	})
	section("Bindings", "No Mathlib rename or refactor changed a binding.", len(n.Rebinds) == 0, func() {
		b.WriteString("| Node | Old | New | Cause | Mathlib |\n|---|---|---|---|---|\n")
		for _, r := range n.Rebinds {
			fmt.Fprintf(&b, "| %s | `%s` | `%s` | %s | %s |\n", r.Tag, r.Old, r.New, r.Cause, short(r.Mathlib))
		}
	})
	return []byte(strings.TrimRight(b.String(), "\n") + "\n")
}

// ParseNotes reads the parts of release notes that a person can edit: the summary and the counts table. Rule R03 reads the counts table, and m2 release notes keeps the summary when it writes the notes again.
func ParseNotes(data []byte) (*Notes, error) {
	n := &Notes{}
	var section string
	var summary []string
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for line := 1; sc.Scan(); line++ {
		l := sc.Text()
		if strings.HasPrefix(l, "## ") {
			section = strings.TrimSpace(l)
			continue
		}
		switch section {
		case summaryHeading:
			summary = append(summary, l)
		case countsHeading:
			if !strings.HasPrefix(l, "|") || strings.HasPrefix(l, "|---") || strings.HasPrefix(l, "| Count |") {
				continue
			}
			f := cells(l)
			if len(f) != 5 {
				return nil, fmt.Errorf("release: line %d of the notes: a row of the counts table has 5 cells", line)
			}
			c := Change{Count: f[0], Cause: f[4]}
			var err1, err2 error
			c.Before, err1 = strconv.Atoi(f[1])
			c.After, err2 = strconv.Atoi(f[2])
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("release: line %d of the notes: the counts %q and %q are not numbers", line, f[1], f[2])
			}
			n.Counts = append(n.Counts, c)
		}
	}
	if s := strings.TrimSpace(strings.Join(summary, "\n")); s != summaryHint {
		n.Summary = s
	}
	return n, sc.Err()
}

// cells splits a row of a Markdown table. A cell can have an escaped bar.
func cells(row string) []string {
	row = strings.TrimSpace(row)
	row = strings.TrimSuffix(strings.TrimPrefix(row, "|"), "|")
	var out []string
	var cur strings.Builder
	for i := 0; i < len(row); i++ {
		switch {
		case row[i] == '\\' && i+1 < len(row) && row[i+1] == '|':
			cur.WriteByte('|')
			i++
		case row[i] == '|':
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteByte(row[i])
		}
	}
	return append(out, strings.TrimSpace(cur.String()))
}

// cell returns s on one line, for a cell of a table.
func cell(s string) string {
	return strings.ReplaceAll(strings.Join(strings.Fields(s), " "), "|", `\|`)
}

func orNone(level string) string {
	if level == "" {
		return "none"
	}
	return level
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
