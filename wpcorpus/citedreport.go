package wpcorpus

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

// CitedReportPath is the report of the titles that math2 cites (WP14.6).
var CitedReportPath = "reports/cited.md"

// The reasons of a cited title that has no mention.
const (
	CitedNoPage     = "no-page"      // no article of the dump has the title
	CitedOutOfScope = "out-of-scope" // the article is a candidate, but not in scope
	CitedNoSignal   = "no-candidate" // the article is not a candidate of the scope run
)

// CitedTitle is the outcome of one cited title. Mention is not "" when the
// article is in scope. Else Reason tells why the title has no mention.
type CitedTitle struct {
	Title   string
	Page    int64
	Mention string
	Reason  string
	Detail  string
}

// ResolveCited gives the outcome of each cited title. page maps a title,
// with underscores for the spaces, to its page id after one redirect, as
// Store.Resolve does. cands are the candidates of the scope run.
func ResolveCited(titles []string, page map[string]int64, cands []*Candidate) []CitedTitle {
	byID := make(map[int64]*Candidate, len(cands))
	for _, c := range cands {
		byID[c.PageID] = c
	}
	out := make([]CitedTitle, 0, len(titles))
	for _, t := range titles {
		r := CitedTitle{Title: t}
		id, ok := page[strings.ReplaceAll(strings.TrimSpace(t), " ", "_")]
		c := byID[id]
		switch {
		case !ok || id == 0:
			r.Reason = CitedNoPage
		case c == nil:
			r.Page, r.Reason = id, CitedNoSignal
		case c.InScope:
			r.Page, r.Mention = id, MentionID(int(id), "")
		default:
			r.Page, r.Reason = id, CitedOutOfScope
			r.Detail = fmt.Sprintf("%s, verdict %s, rule %s", c.Title, c.Verdict, c.Rule)
		}
		out = append(out, r)
	}
	return out
}

// CitedReport writes the report of the cited titles as Markdown with a YAML
// header. The titles with a reason are in a table, sorted by reason and title.
func CitedReport(w io.Writer, rs []CitedTitle) error {
	count := map[string]int{}
	mentions := 0
	var rest []CitedTitle
	for _, r := range rs {
		if r.Mention != "" {
			mentions++
			continue
		}
		count[r.Reason]++
		rest = append(rest, r)
	}
	slices.SortFunc(rest, func(a, b CitedTitle) int {
		if c := strings.Compare(a.Reason, b.Reason); c != 0 {
			return c
		}
		return strings.Compare(a.Title, b.Title)
	})
	var b strings.Builder
	fmt.Fprintf(&b, "---\ntitle: Cited titles\ntitles: %d\nmentions: %d\n%s: %d\n%s: %d\n%s: %d\n---\n\n", len(rs), mentions, CitedNoPage, count[CitedNoPage], CitedOutOfScope, count[CitedOutOfScope], CitedNoSignal, count[CitedNoSignal])
	b.WriteString("# Cited titles\n\n")
	fmt.Fprintf(&b, "math2 cites %d Wikipedia titles. %d of them resolve to an article in scope and have an article mention. Each other title has a reason: `%s` when no article of the dump has the title, `%s` when the article is a candidate that the scope rule did not take, and `%s` when the article is not a candidate.\n\n", len(rs), mentions, CitedNoPage, CitedOutOfScope, CitedNoSignal)
	if len(rest) > 0 {
		b.WriteString("| Title | Page | Reason | Detail |\n|---|---|---|---|\n")
		for _, r := range rest {
			page := ""
			if r.Page != 0 {
				page = fmt.Sprint(r.Page)
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", citedCell(r.Title), page, r.Reason, citedCell(r.Detail))
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func citedCell(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}
