package arxiv

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/inventory"
)

// AdoptPath is the report of m2 corpus adopt -source arxiv, relative to the root of the tree.
const AdoptPath = "reports/adopt/arxiv.md"

// AdoptBy is the name of the run in the outcomes and the history of the rows.
const AdoptBy = "m2 corpus adopt"

// AdoptRule is the rule of schema/outcomes.v1.yaml that gives a row of the research layer the outcome node (corpus arxiv AX8.6, rule 3).
const AdoptRule = "research-node"

// The states of a row in the adopt report.
const (
	Adopted = "adopted" // the run wrote a node with the front matter of the row, for the write role
	Drafted = "drafted" // a node with a provisional id names the row, and waits for the write role and m2 mint
	Linked  = "linked"  // the run set the outcome of the row to the minted node
	Done    = "node"    // the row had the outcome node before the run
	Waiting = "waiting" // the row goes to a person, with the reason
)

// AdoptNode is what the adopt job reads from a node of the tree.
type AdoptNode struct {
	ID        string
	Inventory []string
	Mentions  []string // the mention of each arxiv source of the node
	Retired   bool
}

// AdoptOptions are the inputs of Adopt that are not the inventory and the nodes.
type AdoptOptions struct {
	Corpus string                 // the checkout of tamnd/arxiv, for the authors in the history notes. Empty means no history.
	Rows   []string               // the rows to adopt, in order, as the queue write-research gives them
	Draw   func() (string, error) // gives a new provisional id
	Day    string                 // the date of the run, YYYY-MM-DD
}

// AdoptItem is one line of the adopt report.
type AdoptItem struct {
	Row     string
	Name    string
	Kind    string
	Mention string // the mention that is the source of the node
	Locator string
	State   string
	Reason  string
	Node    string
}

// AdoptDraft is a node file that the run writes.
type AdoptDraft struct {
	ID   string
	Path string // nodes/~/<id>.md
	Data []byte
}

// AdoptResult is the result of Adopt.
type AdoptResult struct {
	Drafts []AdoptDraft
	Rows   []inventory.Row // each row, with the outcomes that the run set
	Linked int             // the rows that the run set to the outcome node
	Items  []AdoptItem
}

// nodeKind gives the kind of the node and the main section of its card for the kind of a row (K8.4).
func nodeKind(kind string) (string, string) {
	switch kind {
	case "definition":
		return "concept", "Summary"
	case "statement":
		return "statement", "Statement"
	case "conjecture":
		return "conjecture", "Statement"
	case "problem":
		return "problem", "Problem"
	}
	return "", ""
}

// statementRoles are the roles of spec 04.2 that a research statement keeps from its paper.
var statementRoles = []string{"theorem", "lemma", "proposition", "corollary"}

// Adopt makes the research nodes of AX9.2 from the rows of opt.Rows. First it sets the outcome node on each row that a minted node names in its inventory field, when the node has an arxiv source of a mention of the row. Then it writes a node for each row of opt.Rows that no node names: the front matter of the row, a source with the versioned locator of its first mention, and an empty main section. The write role writes the section in math2's own words, so the node has no text of the paper.
func Adopt(rows []inventory.Row, ms []inventory.Mention, nodes []AdoptNode, opt AdoptOptions) (*AdoptResult, error) {
	res := &AdoptResult{Rows: rows}
	byRow := map[string]*inventory.Row{}
	for i := range res.Rows {
		byRow[res.Rows[i].ID] = &res.Rows[i]
	}
	byMention := map[string]*inventory.Mention{}
	for i := range ms {
		byMention[ms[i].ID] = &ms[i]
	}

	drafted := map[string]string{}
	linked := map[string]bool{}
	for _, n := range nodes {
		if n.Retired {
			continue
		}
		for _, id := range n.Inventory {
			r := byRow[id]
			if r == nil || !slices.ContainsFunc(n.Mentions, func(m string) bool { return slices.Contains(r.Mentions, m) }) {
				continue
			}
			if strings.HasPrefix(n.ID, "~") {
				drafted[id] = n.ID
				continue
			}
			if r.Outcome != nil && r.Outcome.Value == inventory.OutcomeNode {
				continue
			}
			r.Outcome = &inventory.Outcome{Value: inventory.OutcomeNode, Target: []string{n.ID}, Rule: AdoptRule, By: AdoptBy, Reason: "a node of the research layer states the row (corpus arxiv AX9.2)"}
			r.History = append(r.History, inventory.Event{Op: "outcome", At: opt.Day, By: AdoptBy, Note: "node " + n.ID + " states the row"})
			linked[id] = true
			res.Linked++
		}
	}

	papers := map[string]*Paper{}
	for _, id := range opt.Rows {
		r := byRow[id]
		if r == nil {
			return nil, fmt.Errorf("the inventory has no row %s", id)
		}
		it := AdoptItem{Row: r.ID, Name: r.Name, Kind: r.Kind, State: Waiting}
		m := mainMention(r, byMention)
		if m != nil {
			it.Mention = m.ID
			it.Locator = m.Locator
		}
		kind, section := nodeKind(r.Kind)
		switch {
		case r.Outcome != nil && r.Outcome.Value == inventory.OutcomeNode:
			it.State, it.Node = Done, strings.Join(r.Outcome.Target, ", ")
			if linked[r.ID] {
				it.State = Linked
			}
		case drafted[r.ID] != "":
			it.State, it.Node = Drafted, drafted[r.ID]
		case r.Layer != inventory.Research:
			it.Reason = "the row is not of the research layer"
		case kind == "":
			it.Reason = "a research node does not state an item of the kind " + r.Kind
		case m == nil:
			it.Reason = "the row has no arXiv mention"
		case r.Name == "":
			it.Reason = "the row has no name, and a person names it"
		}
		if it.State == Waiting && it.Reason == "" {
			id, err := opt.Draw()
			if err != nil {
				return nil, err
			}
			p := PaperOf(m.ID)
			if _, ok := papers[p]; !ok && opt.Corpus != "" {
				meta, err := readMetadata(opt.Corpus, map[string]bool{Month(p): true})
				if err != nil {
					return nil, err
				}
				papers[p] = meta[p]
			}
			res.Drafts = append(res.Drafts, AdoptDraft{ID: id, Path: "nodes/~/" + id + ".md", Data: draft(id, kind, section, r, m, papers[p])})
			it.State, it.Node = Adopted, id
		}
		res.Items = append(res.Items, it)
	}
	return res, nil
}

// mainMention gives the mention of the row that is the source of its node: an item that its paper states as its own before one that it cites, then the class open, then the first paper.
func mainMention(r *inventory.Row, byMention map[string]*inventory.Mention) *inventory.Mention {
	var out []*inventory.Mention
	for _, id := range r.Mentions {
		if m := byMention[id]; m != nil && m.Source == Source && !m.Attached() {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return nil
	}
	rank := func(b bool) int {
		if b {
			return 0
		}
		return 1
	}
	slices.SortStableFunc(out, func(a, b *inventory.Mention) int {
		return cmp.Or(
			cmp.Compare(rank(a.Origin != "cited"), rank(b.Origin != "cited")),
			cmp.Compare(rank(a.Class == "open"), rank(b.Class == "open")),
			cmp.Compare(PaperOf(a.ID), PaperOf(b.ID)),
			strings.Compare(a.ID, b.ID),
		)
	})
	return out[0]
}

// draft gives the node file of a row: the front matter and the main section with no body.
func draft(id, kind, section string, r *inventory.Row, m *inventory.Mention, meta *Paper) []byte {
	paper := PaperOf(m.ID)
	version := m.Version
	locator := m.Locator
	if v, rest, ok := strings.Cut(locator, " "); ok && strings.HasPrefix(v, paper+"v") {
		if n, err := strconv.Atoi(strings.TrimPrefix(v, paper+"v")); err == nil && version == 0 {
			version = n
		}
		locator = rest
	}
	version = max(version, 1)
	var b bytes.Buffer
	b.WriteString("---\n")
	fmt.Fprintf(&b, "tag: %s\nkind: %s\n", id, kind)
	if kind == "statement" {
		role := m.Role
		if !slices.Contains(statementRoles, role) {
			role = "proposition"
		}
		fmt.Fprintf(&b, "role: %s\n", role)
	}
	fmt.Fprintf(&b, "name: %s\n", scalar(r.Name))
	if len(r.MSC) > 0 {
		fmt.Fprintf(&b, "msc: %s\n", flow(r.MSC))
	}
	b.WriteString("sources:\n")
	fmt.Fprintf(&b, "  - arxiv: %s\n    locator: %s\n    mention: %s\n    note: the paper that states the item (corpus arxiv AX9.2)\n", scalar(fmt.Sprintf("%sv%d", paper, version)), scalar(cmp.Or(locator, "the paper")), scalar(m.ID))
	if meta != nil {
		if who := meta.AuthorNames(); who != "" {
			fmt.Fprintf(&b, "history:\n  notes: %s\n", scalar(fmt.Sprintf("The source is arXiv:%sv%d (%s), by %s. A paper also restates older results, so this is not the first statement until a person or a later pass confirms it.", paper, version, submitted(paper), who)))
		}
	}
	fmt.Fprintf(&b, "inventory: [%s]\n", r.ID)
	fmt.Fprintf(&b, "---\n\n## %s\n", section)
	return b.Bytes()
}

// submitted gives the month of submission of a paper from its id, as 2026-10 for 2610.13556.
func submitted(paper string) string {
	m := Month(paper)
	if len(m) != 4 {
		return m
	}
	year, _ := strconv.Atoi(m[:2])
	century := 2000
	if year >= 91 {
		century = 1900
	}
	return fmt.Sprintf("%d-%s", century+year, m[2:])
}

func scalar(s string) string {
	out, err := yaml.Marshal(s)
	if err != nil {
		return strconv.Quote(s)
	}
	return strings.TrimSuffix(string(out), "\n")
}

func flow(vs []string) string {
	q := make([]string, len(vs))
	for i, v := range vs {
		q[i] = scalar(v)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// AdoptReport gives the report of the run: a YAML header with the counts, and a table of the rows.
func AdoptReport(res *AdoptResult, day string) []byte {
	count := map[string]int{}
	for _, it := range res.Items {
		count[it.State]++
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	fmt.Fprintf(&b, "source: arxiv\ndate: %s\nrows: %d\nadopted: %d\ndrafted: %d\nlinked: %d\nnode: %d\nwaiting: %d\n", day, len(res.Items), count[Adopted], count[Drafted], res.Linked, count[Done], count[Waiting])
	b.WriteString("---\n\n# The research nodes from arXiv rows\n\n")
	fmt.Fprintf(&b, "This report gives the rows of the research layer that m2 corpus adopt read on %s (corpus arxiv AX9.2). The run wrote %d nodes for the write role, %d nodes wait for the write role or for m2 mint, it set the outcome node on %d rows, and %d rows had a node before. %d rows wait for a person.\n\n", day, count[Adopted], count[Drafted], res.Linked, count[Done], count[Waiting])
	b.WriteString("A node of this run has the front matter of its row and a source with the versioned locator of the paper. It has no text of the paper: the write role writes its main section in math2's own words, and the node is at W1 when that section passes the checks.\n\n")
	if len(res.Items) == 0 {
		return b.Bytes()
	}
	b.WriteString("| Row | Kind | Name | Source | State | Node or reason |\n|---|---|---|---|---|---|\n")
	for _, it := range res.Items {
		what := it.Node
		if it.State == Waiting {
			what = it.Reason
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", it.Row, it.Kind, cell(it.Name), cell(strings.TrimSpace(it.Mention+" "+it.Locator)), it.State, cell(what))
	}
	return b.Bytes()
}

func cell(s string) string { return strings.ReplaceAll(s, "|", `\|`) }
