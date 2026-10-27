package arxiv

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/inventory"
)

// PaperOf returns the paper id of an arXiv mention id, as 2401.00001 for ax://paper/2401.00001#K3F9.
func PaperOf(id string) string {
	p, _, _ := strings.Cut(strings.TrimPrefix(id, "ax://paper/"), "#")
	return p
}

// Year returns the year of submission of a paper id: 2024 for 2401.00001, and 1998 for math/9802134.
func Year(id string) string {
	m := Month(id)
	if len(m) < 2 {
		return ""
	}
	yy, err := strconv.Atoi(m[:2])
	if err != nil {
		return ""
	}
	if yy >= 91 {
		return strconv.Itoa(1900 + yy)
	}
	return strconv.Itoa(2000 + yy)
}

// Candidate is a row of another layer whose name is the name of an arXiv mention (the name pass of AX8.2). The dedup judge decides each candidate, because the name of a variant can be the name of the textbook result.
type Candidate struct {
	Mention string `json:"mention"`
	Row     string `json:"row"`    // the row of the mention now
	Target  string `json:"target"` // the row of the other layer
	Name    string `json:"name"`
}

// NameKey returns the form of a name that the name pass compares: lower case, with a space for each dash, one space between words and no leading "the".
func NameKey(name string) string {
	name = strings.NewReplacer("-", " ", "\u2013", " ", "\u2014", " ", "\u2212", " ", "\u2019", "'").Replace(strings.ToLower(name))
	name = strings.Join(strings.Fields(name), " ")
	return strings.TrimPrefix(name, "the ")
}

// nameBlock is the kind group of the name pass: statements and conjectures share a block (AX8.1).
func nameBlock(kind string) string {
	if kind == "conjecture" {
		return "statement"
	}
	return kind
}

// Candidates returns the name candidates of the arXiv mentions. A candidate is a row with a mention of another source, in the block of the mention, that is the only row of the block with the name of the mention. A mention that is already in a row with a mention of another source has no candidate. With msc, only the blocks of that code at two digits are read.
func Candidates(rows []inventory.Row, ms []inventory.Mention, msc string) []Candidate {
	byID := map[string]*inventory.Mention{}
	for i := range ms {
		byID[ms[i].ID] = &ms[i]
	}
	type entry struct{ rows []string }
	names := map[string]*entry{} // the rows of another layer for each kind, block and name
	rowOf := map[string]string{}
	mixed := map[string]bool{}
	for i := range rows {
		r := &rows[i]
		if r.Outcome != nil && r.Outcome.Value == inventory.OutcomeMerged {
			continue
		}
		var other []*inventory.Mention
		for _, id := range r.Mentions {
			rowOf[id] = r.ID
			if m := byID[id]; m != nil && m.Source != Source {
				other = append(other, m)
			}
		}
		if len(other) == 0 {
			continue
		}
		mixed[r.ID] = true
		keys := map[string]bool{}
		for _, m := range other {
			for _, n := range append([]string{m.Name}, m.Aliases...) {
				if k := NameKey(n); k != "" {
					for _, b := range inventory.Blocks(&inventory.Mention{MSC: r.MSC}) {
						keys[nameBlock(r.Kind)+"|"+b+"|"+k] = true
					}
				}
			}
		}
		for k := range keys {
			e := names[k]
			if e == nil {
				e = &entry{}
				names[k] = e
			}
			e.rows = append(e.rows, r.ID)
		}
	}
	var out []Candidate
	for i := range ms {
		m := &ms[i]
		if m.Source != Source || m.Name == "" || m.Attached() || mixed[rowOf[m.ID]] {
			continue
		}
		var found []string
		for _, b := range inventory.Blocks(m) {
			if msc != "" && b != msc[:min(2, len(msc))] {
				continue
			}
			if e := names[nameBlock(m.Kind)+"|"+b+"|"+NameKey(m.Name)]; e != nil {
				found = append(found, e.rows...)
			}
		}
		slices.Sort(found)
		if found = slices.Compact(found); len(found) == 1 {
			out = append(out, Candidate{Mention: m.ID, Row: rowOf[m.ID], Target: found[0], Name: m.Name})
		}
	}
	slices.SortFunc(out, func(a, b Candidate) int { return strings.Compare(a.Mention, b.Mention) })
	return out
}

// tally is one line of the report.
type tally struct {
	mentions, rows, spread, papers, mixed, waiting, candidates int
	outcomes                                                   map[string]int
}

// Report returns reports/inventory-arxiv.md (AX8.7): for each MSC code at two digits and each year, the mentions, the rows, the outcomes, the mean spread, the rows that hold more than one paper or another source, the mentions that wait for a row, and the name candidates. A row or a mention with codes in two areas counts in both.
func Report(rows []inventory.Row, ms []inventory.Mention, cands []Candidate, day time.Time) string {
	byID := map[string]*inventory.Mention{}
	for i := range ms {
		byID[ms[i].ID] = &ms[i]
	}
	byCode, byYear := map[string]*tally{}, map[string]*tally{}
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
	held := map[string]bool{}
	for i := range rows {
		r := &rows[i]
		var axs []*inventory.Mention
		papers, other := map[string]bool{}, false
		for _, id := range r.Mentions {
			held[id] = true
			m := byID[id]
			switch {
			case m == nil:
			case m.Source == Source:
				axs = append(axs, m)
				papers[PaperOf(id)] = true
			default:
				other = true
			}
		}
		if len(axs) == 0 || r.Outcome != nil && r.Outcome.Value == inventory.OutcomeMerged {
			continue
		}
		outcome := "open"
		if r.Outcome != nil {
			outcome = r.Outcome.Value
		}
		year := Year(PaperOf(axs[0].ID))
		for _, t := range append(blockTallies(byCode, get, r.MSC), get(byYear, year)) {
			t.rows++
			t.spread += r.Spread
			t.outcomes[outcome]++
			if len(papers) > 1 {
				t.papers++
			}
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
		for _, t := range append(blockTallies(byCode, get, m.MSC), get(byYear, Year(PaperOf(m.ID)))) {
			t.mentions++
			if !held[m.ID] {
				t.waiting++
			}
		}
	}
	for _, c := range cands {
		if m := byID[c.Mention]; m != nil {
			for _, t := range append(blockTallies(byCode, get, m.MSC), get(byYear, Year(PaperOf(m.ID)))) {
				t.candidates++
			}
		}
	}
	all := &tally{outcomes: map[string]int{}}
	for _, t := range byYear {
		all.mentions += t.mentions
		all.rows += t.rows
		all.waiting += t.waiting
		all.candidates += t.candidates
		all.papers += t.papers
		all.mixed += t.mixed
		for k, v := range t.outcomes {
			all.outcomes[k] += v
		}
	}
	var b strings.Builder
	b.WriteString("# The arXiv layer of the inventory\n\n")
	fmt.Fprintf(&b, "The inventory has %s that are not attached to another item, in %s. %s the mentions of more than one paper, which the hash pass joined, and %s also a mention of another source. %s for a row. The name pass found %s, which wait for the dedup judge. Counted on %s. These are the counts of AX8.7.\n\n",
		count(all.mentions, "arXiv mention", "arXiv mentions"), count(all.rows, "row", "rows"), count(all.papers, "row holds", "rows hold"), count(all.mixed, "row holds", "rows hold"),
		count(all.waiting, "mention waits", "mentions wait"), count(all.candidates, "candidate", "candidates"), day.UTC().Format("2 January 2006"))
	b.WriteString("## The outcomes\n\n| Outcome | Rows |\n| --- | ---: |\n")
	for _, k := range slices.Sorted(maps.Keys(all.outcomes)) {
		fmt.Fprintf(&b, "| %s | %d |\n", k, all.outcomes[k])
	}
	b.WriteString("\nThe outcome open is a row with no outcome yet. Most of these rows hold an item of the kind unknown, which waits for the pass classify (AX7.2).\n")
	table := func(title, key string, m map[string]*tally) {
		fmt.Fprintf(&b, "\n## %s\n\n| %s | Mentions | Rows | record | out_of_scope | open | Mean spread | More than one paper | Another source | Waiting | Name candidates |\n", title, key)
		b.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
		for _, k := range slices.Sorted(maps.Keys(m)) {
			t := m[k]
			mean := "-"
			if t.rows > 0 {
				mean = strconv.FormatFloat(float64(t.spread)/float64(t.rows), 'f', 2, 64)
			}
			fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %s | %d | %d | %d | %d |\n", k, t.mentions, t.rows,
				t.outcomes[inventory.OutcomeRecord], t.outcomes[inventory.OutcomeOutOfScope], t.outcomes["open"], mean, t.papers, t.mixed, t.waiting, t.candidates)
		}
	}
	table("By MSC code at two digits", "Code", byCode)
	table("By year of submission", "Year", byYear)
	b.WriteString("\n## What is not measured yet\n\n")
	b.WriteString("The cited-result pass needs the cross-paper edges of AX6.5, and the similarity pass needs the unfolded forms of tier T3, so no row comes from them yet. The precision and the recall of the passes need gold set K for arXiv pairs (AX11.2).\n")
	return b.String()
}

// blockTallies returns the tallies of the blocks of the codes, or of the block none when there is no code.
func blockTallies(m map[string]*tally, get func(map[string]*tally, string) *tally, msc []string) []*tally {
	var out []*tally
	for _, b := range inventory.Blocks(&inventory.Mention{MSC: msc}) {
		out = append(out, get(m, b))
	}
	return out
}

// FormatCandidates returns the candidates, one JSON object on each line.
func FormatCandidates(cs []Candidate) ([]byte, error) {
	var b strings.Builder
	for _, c := range cs {
		data, err := json.Marshal(c)
		if err != nil {
			return nil, err
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return []byte(b.String()), nil
}

// count returns n with the word in the singular or the plural form.
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}
