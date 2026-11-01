package status

import (
	"bytes"
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census"
	"github.com/tamnd/math2-agent/resource/cluster"
)

// ClosureDepth is the least depth of a citing resource whose references count in the reference closure (RS11.5).
const ClosureDepth = 1

// RefRow is the reference closure of one stratum of the cited works (RS11.5). The kind group and the decade are those of the cited resource when the reference resolves. When it does not resolve, they come from the parsed fields: a reference with a venue or a volume is an article, and another reference is a book.
type RefRow struct {
	Kind     string  `json:"kind"`
	Decade   string  `json:"decade"`
	Parsed   int     `json:"parsed"`   // the references of the stratum
	Resolved int     `json:"resolved"` // of them, those that resolve to a resource
	Closure  float64 `json:"closure"`
	Target   float64 `json:"target"`
	Below    bool    `json:"below,omitempty"`
	Works    int     `json:"works"` // the distinct cited works of the stratum: the resources that the references resolve to, and the unresolved works by folded title and family of the first creator
	Held     int     `json:"held"`  // of them, the live resources in scope or border, which are works of the catalogue (RS11.4.3 item 3)
}

// Uncited is one work that references cite and that resolves to no resource. The references of one work have the same folded title and the same family of the first creator.
type Uncited struct {
	Title  string   `json:"title"`
	Author string   `json:"author,omitempty"`
	Year   string   `json:"year,omitempty"`
	Citing int      `json:"citing"` // the count of the citing resources
	Refs   []string `json:"refs"`   // the first references, as <id>/e<n>#<n>
}

// RefClosure is the reference closure of RS11.5 and the H14 list: the unresolved works that two or more resources cite, most cited first, at most MaxList.
type RefClosure struct {
	Citing     int       `json:"citing"` // the citing resources with at least one reference that counts
	Rows       []RefRow  `json:"rows,omitempty"`
	Unresolved []Uncited `json:"unresolved,omitempty"`
}

// closureTarget gives the target of RS1.7 for the references of works of the decade: 99% after 1990, and 95% before or when the decade is not known.
func closureTarget(decade string) float64 {
	if decade != "unknown" && decade >= "1990s" {
		return 0.99
	}
	return 0.95
}

// refClosure computes the reference closure. A reference counts when its citing resource is live, in scope and at ClosureDepth or more, and the reference resolves or has parsed fields.
func refClosure(rs []resource.Resource, cs []resource.Cite, depthOf map[string]int) RefClosure {
	byID := map[string]*resource.Resource{}
	for i := range rs {
		byID[rs[i].ID] = &rs[i]
	}
	type key struct{ kind, decade string }
	rows := map[key]*RefRow{}
	citing := map[string]bool{}
	works := map[string]*Uncited{}
	who := map[string]map[string]bool{}
	distinct := map[key]map[string]bool{}
	held := map[key]map[string]bool{}
	add := func(m map[key]map[string]bool, k key, w string) {
		if m[k] == nil {
			m[k] = map[string]bool{}
		}
		m[k][w] = true
	}
	for _, e := range cs {
		id, _, _ := strings.Cut(e.From, "/")
		r := byID[id]
		if r == nil || !r.Live() || r.Scope == nil || r.Scope.Value != resource.In || depthOf[id] < ClosureDepth {
			continue
		}
		var k key
		to, _, _ := strings.Cut(e.To, "/")
		switch t := byID[to]; {
		case e.To != "" && t != nil:
			k = key{census.KindGroup(t.Kind), census.Decade(t.Date)}
		case e.Parsed != nil:
			k = key{"book", census.Decade(e.Parsed.Year)}
			if e.Parsed.Venue != "" || e.Parsed.Volume != "" {
				k.kind = "article"
			}
		default:
			continue
		}
		citing[id] = true
		row := rows[k]
		if row == nil {
			row = &RefRow{Kind: k.kind, Decade: k.decade, Target: closureTarget(k.decade)}
			rows[k] = row
		}
		row.Parsed++
		if e.To != "" {
			row.Resolved++
			if t := byID[to]; t != nil {
				add(distinct, k, "r:"+t.ID)
				if t.Live() && t.Scope != nil && (t.Scope.Value == resource.In || t.Scope.Value == resource.Border) {
					add(held, k, "r:"+t.ID)
				}
			}
			continue
		}
		if e.Parsed == nil || len(e.Parsed.Authors) == 0 {
			continue
		}
		title, family := cluster.WorkTitle(e.Parsed.Title), cluster.Family(e.Parsed.Authors[0])
		if title == "" || family == "" {
			continue
		}
		w := title + "|" + family
		add(distinct, k, "u:"+w)
		if works[w] == nil {
			works[w] = &Uncited{Title: e.Parsed.Title, Author: e.Parsed.Authors[0], Year: e.Parsed.Year}
			who[w] = map[string]bool{}
		}
		who[w][id] = true
		if len(works[w].Refs) < 5 {
			works[w].Refs = append(works[w].Refs, e.From+"#"+strconv.Itoa(e.N))
		}
	}
	out := RefClosure{Citing: len(citing)}
	for _, k := range slices.SortedFunc(maps.Keys(rows), func(a, b key) int { return cmp.Or(cmp.Compare(a.kind, b.kind), cmp.Compare(a.decade, b.decade)) }) {
		row := rows[k]
		row.Closure = round(float64(row.Resolved) / float64(row.Parsed))
		row.Below = row.Closure < row.Target
		row.Works, row.Held = len(distinct[k]), len(held[k])
		out.Rows = append(out.Rows, *row)
	}
	for w, u := range works {
		u.Citing = len(who[w])
		if u.Citing >= 2 {
			out.Unresolved = append(out.Unresolved, *u)
		}
	}
	slices.SortFunc(out.Unresolved, func(a, b Uncited) int {
		return cmp.Or(cmp.Compare(b.Citing, a.Citing), cmp.Compare(a.Title, b.Title), cmp.Compare(a.Author, b.Author))
	})
	out.Unresolved = out.Unresolved[:min(len(out.Unresolved), MaxList)]
	return out
}

// Stale is a source whose last snapshot is older than twice its cadence (rule H17).
type Stale struct {
	Source  string `json:"source"`
	Cadence string `json:"cadence"`
	Last    string `json:"last"`
	Days    int    `json:"days"` // the age of the last snapshot on the day of the report
}

// stale gives the sources of the catalogue whose last snapshot is older than twice their cadence on the day.
func stale(cat *ingest.Catalogue, day string) []Stale {
	now, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return nil
	}
	var out []Stale
	for i := range cat.Sources {
		s := &cat.Sources[i]
		days, last := s.CadenceDays(), s.Latest()
		if days == 0 || last == nil {
			continue
		}
		on, err := time.Parse(time.DateOnly, last.Date)
		if err != nil {
			continue
		}
		if age := int(now.Sub(on).Hours() / 24); age > 2*days {
			out = append(out, Stale{Source: s.ID, Cadence: s.Cadence, Last: last.Date, Days: age})
		}
	}
	return out
}

// Unlisted is the estimate of the works of the estimated strata that no source lists (RS11.1): the sum of the estimate of each stratum less its resources, with the interval.
type Unlisted struct {
	Strata int     `json:"strata"` // the strata with an estimate
	N      float64 `json:"n"`
	Low    float64 `json:"low"`
	High   float64 `json:"high"`
}

func unlisted(rs []Recall) Unlisted {
	var u Unlisted
	for _, r := range rs {
		if r.Model == "" {
			continue
		}
		u.Strata++
		have := float64(r.Resources)
		u.N += max(0, r.N-have)
		u.Low += max(0, r.Low-have)
		u.High += max(0, r.High-have)
	}
	return u
}

// refsMarkdown writes section 5 of the report: the reference closure and the H14 list.
func (rep *Report) refsMarkdown(b *bytes.Buffer) {
	c := rep.Refs
	fmt.Fprintf(b, "\n## 5. Reference closure\n\n")
	if len(c.Rows) == 0 {
		fmt.Fprintf(b, "No live resource in scope at depth D%d or more has a reference with parsed fields. Run `m2 res cites` and `m2 res resolve`.\n", ClosureDepth)
		return
	}
	fmt.Fprintf(b, "The references of %d live resources in scope at depth D%d or more. The stratum is the kind group and the decade of the cited work.\n\n| Kind | Decade | References | Resolved | Closure | Target |\n|---|---|---:|---:|---:|---:|\n", c.Citing, ClosureDepth)
	for _, r := range c.Rows {
		mark := ""
		if r.Below {
			mark = ", below"
		}
		fmt.Fprintf(b, "| %s | %s | %d | %d | %.2f%% | %.0f%%%s |\n", r.Kind, r.Decade, r.Parsed, r.Resolved, 100*r.Closure, 100*r.Target, mark)
	}
	fmt.Fprintf(b, "\n%d works that two or more resources cite resolve to no resource (rule H14).", len(c.Unresolved))
	if len(c.Unresolved) == 0 {
		b.WriteString("\n")
		return
	}
	fmt.Fprintf(b, " The list gives the first %d, most cited first.\n\n| Title | First creator | Year | Citing | References |\n|---|---|---|---:|---|\n", min(len(c.Unresolved), MaxList))
	for _, u := range c.Unresolved {
		fmt.Fprintf(b, "| %s | %s | %s | %d | %s |\n", cell(u.Title), cell(u.Author), u.Year, u.Citing, strings.Join(u.Refs, ", "))
	}
}

// staleMarkdown writes sections 8 and 9 of the report: the stale snapshots and the statement of what cannot be claimed.
func (rep *Report) staleMarkdown(b *bytes.Buffer) {
	fmt.Fprintf(b, "\n## 8. Stale snapshots\n\n")
	if len(rep.Stale) == 0 {
		fmt.Fprintf(b, "No source has a last snapshot older than twice its cadence (rule H17).\n")
	} else {
		fmt.Fprintf(b, "%d sources have a last snapshot older than twice their cadence (rule H17).\n\n| Source | Cadence | Last snapshot | Age in days |\n|---|---|---|---:|\n", len(rep.Stale))
		for _, s := range rep.Stale {
			fmt.Fprintf(b, "| %s | %s | %s | %d |\n", s.Source, s.Cadence, s.Last, s.Days)
		}
	}
	u := rep.Unlisted
	fmt.Fprintf(b, "\n## 9. What cannot be claimed\n\nThe catalogue can show exactly that each record of each source has a resource with an outcome, that each reference is resolved or listed, and that each work that a curriculum names has a resource. It can give the recall of each stratum only as an estimate with an interval. It cannot show that no work exists that no catalogue, no bibliography and no course lists (RS11.1).\n")
	if u.Strata == 0 {
		fmt.Fprintf(b, "\nNo stratum has an estimate, so the report gives no estimate of the works that no source lists.\n")
		return
	}
	fmt.Fprintf(b, "\nIn the %d strata with an estimate, the estimate of the works that no source lists is %.0f, with the interval %.0f to %.0f. The strata with no estimate are not in this number.\n", u.Strata, u.N, u.Low, u.High)
}

// cell escapes the text for a cell of a Markdown table.
func cell(s string) string { return strings.ReplaceAll(s, "|", "\\|") }
