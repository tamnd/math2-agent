package wikipedia

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/inventory"
)

// The files that m2 corpus cluster -source wikipedia writes, relative to the root of the tree.
const (
	NamesPath     = "reports/inventory/wikipedia-names.md"
	InventoryPath = "reports/inventory-wikipedia.md"
)

// Candidates returns the name candidates of the Wikipedia mentions (pass 3 of WP8.2). With msc, only the blocks of that code at two digits are read.
func Candidates(rows []inventory.Row, ms []inventory.Mention, msc string) []inventory.NameCandidate {
	return inventory.NameCandidates(rows, ms, Source, msc)
}

// reportOutcomes are the outcomes of WP8.7 that the report gives a column, in order. The outcome open is a row with no outcome yet.
var reportOutcomes = []string{inventory.OutcomeNode, inventory.OutcomeMember, inventory.OutcomeSplit, inventory.OutcomeAligned, inventory.OutcomeRecord, inventory.OutcomeOutOfScope, inventory.OutcomeDeferred, "open"}

// tally is one line of the report.
type tally struct {
	mentions, rows, mixed, waiting, candidates int
	outcomes                                   map[string]int
}

func (t *tally) add(o *tally) {
	t.mentions += o.mentions
	t.rows += o.rows
	t.mixed += o.mixed
	t.waiting += o.waiting
	t.candidates += o.candidates
	for k, v := range o.outcomes {
		t.outcomes[k] += v
	}
}

// Report returns reports/inventory-wikipedia.md (WP8.7): for each article type and each MSC code at two digits, the mentions that are not attached, the rows, the outcomes, the rows that also hold a mention of another source, the mentions that wait for a row, and the name candidates. A row or a mention with codes in two areas counts in both blocks.
func Report(rows []inventory.Row, ms []inventory.Mention, cands []inventory.NameCandidate, day time.Time) string {
	byID := map[string]*inventory.Mention{}
	for i := range ms {
		byID[ms[i].ID] = &ms[i]
	}
	byType, byCode := map[string]*tally{}, map[string]*tally{}
	get := func(m map[string]*tally, k string) *tally {
		if k == "" {
			k = "none"
		}
		t := m[k]
		if t == nil {
			t = &tally{outcomes: map[string]int{}}
			m[k] = t
		}
		return t
	}
	codes := func(msc []string) []*tally {
		var out []*tally
		for _, b := range inventory.Blocks(&inventory.Mention{MSC: msc}) {
			out = append(out, get(byCode, b))
		}
		return out
	}
	held := map[string]bool{}
	for i := range rows {
		r := &rows[i]
		var wp *inventory.Mention
		other := false
		for _, id := range r.Mentions {
			held[id] = true
			switch m := byID[id]; {
			case m == nil:
			case m.Source == Source:
				if wp == nil {
					wp = m
				}
			default:
				other = true
			}
		}
		if wp == nil || r.Outcome != nil && r.Outcome.Value == inventory.OutcomeMerged {
			continue
		}
		outcome := "open"
		if r.Outcome != nil {
			outcome = r.Outcome.Value
		}
		for _, t := range append(codes(r.MSC), get(byType, wp.ArticleType)) {
			t.rows++
			t.outcomes[outcome]++
			if other {
				t.mixed++
			}
		}
	}
	for i := range ms {
		m := &ms[i]
		if m.Source != Source || m.Attached() {
			continue
		}
		for _, t := range append(codes(m.MSC), get(byType, m.ArticleType)) {
			t.mentions++
			if !held[m.ID] {
				t.waiting++
			}
		}
	}
	for _, c := range cands {
		if m := byID[c.Mention]; m != nil {
			for _, t := range append(codes(m.MSC), get(byType, m.ArticleType)) {
				t.candidates++
			}
		}
	}
	all := &tally{outcomes: map[string]int{}}
	for _, t := range byType {
		all.add(t)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "---\ntitle: The Wikipedia layer of the inventory\ndate: %s\nmentions: %d\nrows: %d\nanother_source: %d\nwaiting: %d\nname_candidates: %d\n---\n\n", day.UTC().Format("2006-01-02"), all.mentions, all.rows, all.mixed, all.waiting, all.candidates)
	b.WriteString("# The Wikipedia layer of the inventory\n\n")
	fmt.Fprintf(&b, "The inventory has %s with no attachment to another item, in %s. %s a mention of another source. %s for a row. The name pass found %s, which wait for the dedup judge (%s). Counted on %s. These are the counts of WP8.7.\n\n",
		count(all.mentions, "Wikipedia mention", "Wikipedia mentions"), count(all.rows, "row", "rows"), count(all.mixed, "row also holds", "rows also hold"),
		count(all.waiting, "mention waits", "mentions wait"), count(all.candidates, "candidate", "candidates"), NamesPath, day.UTC().Format("2 January 2006"))
	b.WriteString("## The outcomes\n\n| Outcome | Rows |\n| --- | ---: |\n")
	for _, k := range slices.Sorted(maps.Keys(all.outcomes)) {
		fmt.Fprintf(&b, "| %s | %d |\n", k, all.outcomes[k])
	}
	b.WriteString("\nThe outcome open is a row with no outcome yet. The passes of WP07 and the selection of WP9.1 give the outcomes node and split, and the judge of WP8.2 gives the outcome merged.\n")
	table := func(title, key string, m map[string]*tally) {
		fmt.Fprintf(&b, "\n## %s\n\n| %s | Mentions | Rows | %s | Another source | Waiting | Name candidates |\n", title, key, strings.Join(reportOutcomes, " | "))
		b.WriteString("| --- |" + strings.Repeat(" ---: |", len(reportOutcomes)+5) + "\n")
		for _, k := range slices.Sorted(maps.Keys(m)) {
			t := m[k]
			fmt.Fprintf(&b, "| %s | %d | %d |", k, t.mentions, t.rows)
			for _, o := range reportOutcomes {
				fmt.Fprintf(&b, " %d |", t.outcomes[o])
			}
			fmt.Fprintf(&b, " %d | %d | %d |\n", t.mixed, t.waiting, t.candidates)
		}
	}
	table("By article type", "Type", byType)
	table("By MSC code at two digits", "Code", byCode)
	b.WriteString("\n## What is not measured yet\n\n")
	b.WriteString("The statement pass needs the unfolded forms of tier T3, and the judge needs a model, so no row comes from passes 4 and 5 yet. The precision of an automatic same needs gold set D (WP11.2).\n")
	return b.String()
}

// FormatCandidates returns reports/inventory/wikipedia-names.md: a YAML header with the count, and a table with one candidate on each line.
func FormatCandidates(cs []inventory.NameCandidate, day time.Time) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "---\ntitle: The name candidates of the Wikipedia mentions\ndate: %s\ncandidates: %d\n---\n\n", day.UTC().Format("2006-01-02"), len(cs))
	b.WriteString("# The name candidates of the Wikipedia mentions\n\n")
	b.WriteString("Each line is a Wikipedia mention and the row of another source that is the only row in its block with the same name (pass 3 of WP8.2). The dedup judge decides each line.\n\n")
	b.WriteString("| Mention | Row | Target | Name |\n| --- | --- | --- | --- |\n")
	cell := strings.NewReplacer("|", `\|`, "\n", " ")
	for _, c := range cs {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", c.Mention, c.Row, c.Target, cell.Replace(c.Name))
	}
	return []byte(b.String())
}

// count returns n with the word in the singular or the plural form.
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}
