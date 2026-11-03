package wikipedia

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/inventory"
)

// The places of the parity data and of the parity reports (corpus wikipedia WP9.7), relative to the root of the tree.
const (
	ParityDir       = "records/parity/wikipedia"
	ParityReportDir = "reports/parity"
)

// The outcomes of the parity report that are not outcomes of a row.
const (
	ParityAttached = "attached" // a proof with no node, attached to the row of the item that it proves
	ParityOpen     = "open"     // a row with no outcome, which rule Y18 reports
	ParityNoRow    = "no_row"   // a mention that is in no row and is not attached, which rule Y18 reports
)

// ParityItem is one item of an article in the parity data.
type ParityItem struct {
	Article  string   `json:"article"`            // the mention of the article
	Title    string   `json:"title"`              // the title of the article
	Revision int      `json:"revision"`           // the revision of record
	Mention  string   `json:"mention"`            // the mention of the item
	Kind     string   `json:"kind"`               // the kind of the mention
	Name     string   `json:"name,omitempty"`     // the name of the item
	Row      string   `json:"row,omitempty"`      // the row of the item, or of the item that a proof proves
	Outcome  string   `json:"outcome"`            // an outcome of a row, or attached, open or no_row
	Target   []string `json:"target,omitempty"`   // the nodes or the rows of the outcome
	Reason   string   `json:"reason,omitempty"`   // the reason of the outcome
	Until    string   `json:"until,omitempty"`    // the date of a deferred row
	Node     string   `json:"node,omitempty"`     // the node that has text from the item
	Origin   string   `json:"origin,omitempty"`   // adopted or adapted, when a node has text from the item
	Assessed string   `json:"assessed,omitempty"` // the class and the importance of the article, as B/High
}

// Missing reports whether the item has no outcome, which rule Y18 reports.
func (p *ParityItem) Missing() bool { return p.Outcome == ParityOpen || p.Outcome == ParityNoRow }

// ParityNode is what the parity report reads from a node: the mentions of its wikipedia sources and the note of each one.
type ParityNode struct {
	ID       string
	Mentions map[string]string // mention to note (adopted or adapted)
}

// Parity gives the parity data of the articles of the pages, or of each article when pages is nil. The items of an article are in the order of their mentions, with the article mention first.
func Parity(rows []inventory.Row, ms []inventory.Mention, nodes []ParityNode, pages map[int]bool, day string) []ParityItem {
	rowOf := map[string]*inventory.Row{}
	for i := range rows {
		for _, m := range rows[i].Mentions {
			rowOf[m] = &rows[i]
		}
		for _, m := range rows[i].Proofs {
			if rowOf[m] == nil {
				rowOf[m] = &rows[i]
			}
		}
	}
	text := map[string][2]string{}
	for _, n := range nodes {
		for m, note := range n.Mentions {
			text[m] = [2]string{n.ID, note}
		}
	}
	byPage := map[int][]*inventory.Mention{}
	for i := range ms {
		m := &ms[i]
		if m.Source != Source || pages != nil && !pages[m.Page] {
			continue
		}
		byPage[m.Page] = append(byPage[m.Page], m)
	}
	var out []ParityItem
	for _, p := range slices.Sorted(mapKeys(byPage)) {
		list := byPage[p]
		var art *inventory.Mention
		for _, m := range list {
			if !strings.Contains(m.ID, "#") {
				art = m
			}
		}
		if art == nil {
			continue
		}
		slices.SortFunc(list, func(a, b *inventory.Mention) int {
			if (a == art) != (b == art) {
				if a == art {
					return -1
				}
				return 1
			}
			return strings.Compare(a.ID, b.ID)
		})
		for _, m := range list {
			it := ParityItem{Article: art.ID, Title: art.NativeID, Revision: art.Revision, Mention: m.ID, Kind: m.Kind, Name: m.Name, Assessed: art.Assessment}
			if t, ok := text[m.ID]; ok {
				it.Node, it.Origin = t[0], t[1]
			}
			r := rowOf[m.ID]
			attached := r != nil && !slices.Contains(r.Mentions, m.ID)
			if r == nil && m.AttachedTo != "" {
				r, attached = rowOf[m.AttachedTo], true
			}
			switch {
			case r == nil:
				it.Outcome = ParityNoRow
			case attached:
				it.Row = r.ID
				it.Outcome = ParityAttached
				if it.Node != "" {
					it.Outcome, it.Target = inventory.OutcomeNode, []string{it.Node}
				}
			case r.Outcome == nil || r.IsOpen(dayTime(day)):
				it.Row, it.Outcome = r.ID, ParityOpen
			default:
				it.Row = r.ID
				it.Outcome, it.Target, it.Reason, it.Until = r.Outcome.Value, r.Outcome.Target, r.Outcome.Reason, r.Outcome.Until
			}
			out = append(out, it)
		}
	}
	return out
}

func mapKeys[K comparable, V any](m map[K]V) func(func(K) bool) {
	return func(yield func(K) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// WriteParity writes the parity data to records/parity/wikipedia/<c1c2c3>.jsonl, with the shard of the article mention, so that the items of one article are in one file. It replaces the files of the articles in the data and keeps the items of the other articles.
func WriteParity(root string, items []ParityItem) error {
	dir := filepath.Join(root, ParityDir)
	old, err := ReadParity(root)
	if err != nil {
		return err
	}
	now := map[string]bool{}
	for _, it := range items {
		now[it.Article] = true
	}
	all := slices.DeleteFunc(old, func(it ParityItem) bool { return now[it.Article] })
	all = append(all, items...)
	files := map[string]*bytes.Buffer{}
	for _, it := range all {
		s := inventory.MentionShard(it.Article)
		if files[s] == nil {
			files[s] = &bytes.Buffer{}
		}
		line, err := json.Marshal(it)
		if err != nil {
			return err
		}
		files[s].Write(line)     // #nosec G104 -- a bytes.Buffer write does not fail.
		files[s].WriteByte('\n') // #nosec G104 -- a bytes.Buffer write does not fail.
	}
	if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- record directories are public.
		return err
	}
	for s, b := range files {
		if err := os.WriteFile(filepath.Join(dir, s+".jsonl"), b.Bytes(), 0o644); err != nil { // #nosec G306 -- records are public text in a git tree.
			return err
		}
	}
	return nil
}

// ReadParity reads the parity data of the tree, in the order of the files and the lines.
func ReadParity(root string) ([]ParityItem, error) {
	var out []ParityItem
	err := WalkParity(root, func(_ string, _ int, it ParityItem) error {
		out = append(out, it)
		return nil
	})
	return out, err
}

// WalkParity calls fn for each item of the parity data, with its file and line.
func WalkParity(root string, fn func(file string, line int, it ParityItem) error) error {
	dir := filepath.Join(root, ParityDir)
	ents, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range ents {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(ParityDir, e.Name()))
		data, err := os.ReadFile(filepath.Join(dir, e.Name())) // #nosec G304 -- the path comes from a listing of the tree.
		if err != nil {
			return err
		}
		for i, line := range bytes.Split(data, []byte("\n")) {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var it ParityItem
			if err := json.Unmarshal(line, &it); err != nil {
				return fmt.Errorf("%s:%d: %w", rel, i+1, err)
			}
			if err := fn(rel, i+1, it); err != nil {
				return err
			}
		}
	}
	return nil
}

// ParityReport gives the parity report of an area: a YAML header with the counts, the summary, and a table of the items of each article.
func ParityReport(area string, items []ParityItem, day string) []byte {
	arts := map[string]bool{}
	count := map[string]int{}
	origin := map[string]int{}
	reasons := map[string]int{}
	for _, it := range items {
		arts[it.Article] = true
		count[it.Outcome]++
		if it.Origin != "" {
			origin[it.Origin]++
		}
		if it.Outcome == inventory.OutcomeDeferred {
			reasons[reasonClass(it.Reason)]++
		}
	}
	missing := count[ParityOpen] + count[ParityNoRow]
	var b bytes.Buffer
	b.WriteString("---\n")
	fmt.Fprintf(&b, "source: wikipedia\narea: %s\ndate: %s\narticles: %d\nitems: %d\nwithout_outcome: %d\noutcomes:\n", scalar(area), day, len(arts), len(items), missing)
	for _, k := range parityOrder {
		if count[k] > 0 {
			fmt.Fprintf(&b, "  %s: %d\n", k, count[k])
		}
	}
	if len(origin) > 0 {
		b.WriteString("origin:\n")
		for _, k := range slices.Sorted(mapKeys(origin)) {
			fmt.Fprintf(&b, "  %s: %d\n", k, origin[k])
		}
	}
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# Parity report: %s\n\n", area)
	fmt.Fprintf(&b, "This report gives each item of each Wikipedia article of the area %s and the outcome of the item in math2 (corpus wikipedia WP9.7). The data is in %s. Rule Y18 fails when an item has no outcome.\n\n", area, ParityDir)
	fmt.Fprintf(&b, "The area has %d articles and %d items. %d items have no outcome.\n\n", len(arts), len(items), missing)
	b.WriteString("| Outcome | Items |\n|---|---|\n")
	for _, k := range parityOrder {
		if count[k] > 0 {
			fmt.Fprintf(&b, "| %s | %d |\n", k, count[k])
		}
	}
	if len(origin) > 0 {
		b.WriteString("\n| Origin of the node text | Items |\n|---|---|\n")
		for _, k := range slices.Sorted(mapKeys(origin)) {
			fmt.Fprintf(&b, "| %s | %d |\n", k, origin[k])
		}
	}
	if len(reasons) > 0 {
		b.WriteString("\n| Reason of a deferred item | Items |\n|---|---|\n")
		keys := slices.Sorted(mapKeys(reasons))
		slices.SortStableFunc(keys, func(a, b string) int { return reasons[b] - reasons[a] })
		for _, k := range keys {
			fmt.Fprintf(&b, "| %s | %d |\n", cell(k), reasons[k])
		}
	}
	art := ""
	for _, it := range items {
		if it.Article != art {
			art = it.Article
			fmt.Fprintf(&b, "\n## %s\n\n%s, revision %d", cell(it.Title), it.Article, it.Revision)
			if it.Assessed != "" {
				fmt.Fprintf(&b, ", assessment %s", it.Assessed)
			}
			b.WriteString(".\n\n| Item | Kind | Name | Outcome | Target | Text | Reason |\n|---|---|---|---|---|---|---|\n")
		}
		item := "subject"
		if t := tagOf(it.Mention); t != "" {
			item = "#" + t
		}
		target := strings.Join(it.Target, ", ")
		if target == "" && it.Outcome == ParityAttached {
			target = "row " + it.Row
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n", item, it.Kind, cell(it.Name), it.Outcome, target, it.Origin, cell(it.Reason))
	}
	return b.Bytes()
}

var parityOrder = append(slices.Clone(inventory.Outcomes()), ParityAttached, ParityOpen, ParityNoRow)

// reasonClass removes the page ids and the node tags from a reason, so that the summary groups the reasons.
func reasonClass(s string) string {
	s, _, _ = strings.Cut(s, ": ")
	s = pageNum.ReplaceAllString(s, "page P")
	return nodeTag.ReplaceAllString(s, "node T")
}
