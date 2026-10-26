package review

import (
	"cmp"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/status"
)

// Export is the fidelity reviews of one reviewer, in a form to cite in a CV or a grant report (spec 16.4). A review is signed and public, thus the export names the reviewer.
type Export struct {
	Reviewer string         `json:"reviewer"`        // orcid:ID or github:LOGIN
	First    string         `json:"first,omitempty"` // the date of the first review in registers/reviewers.reg, or "" when the register has no record of the reviewer
	Areas    []string       `json:"areas,omitempty"` // the MSC2020 codes of the areas in the register
	Verdicts map[string]int `json:"verdicts"`        // the number of reviews with each verdict
	Reviews  []Entry        `json:"reviews"`         // the reviews, the newest first
}

// Entry is one review of an export.
type Entry struct {
	status.Review
	Tag  string `json:"tag"`  // the node of the review
	Name string `json:"name"` // the name of the node, or "" when the tree has no node with the tag
	URI  string `json:"uri"`  // m2://TAG
}

// Exports returns the export of each reviewer with a review, in the order of the reviewer id. The reviews are from status.LoadReviews and the reviewers from status.LoadReviewers. The names give the name of each node by tag. A reviewer with no record in the register gets an export too, because the review is in the tree, but the review does not count for the status of the node.
func Exports(reviews map[string]status.Review, reviewers map[string]status.Reviewer, names map[string]string) []Export {
	by := map[string]*Export{}
	for _, r := range reviews {
		e := by[r.Reviewer]
		if e == nil {
			e = &Export{Reviewer: r.Reviewer, Verdicts: map[string]int{}}
			if rv, ok := reviewers[r.Reviewer]; ok {
				e.First, e.Areas = rv.First, rv.Areas
			}
			by[r.Reviewer] = e
		}
		t := Tag(r.ID)
		e.Reviews = append(e.Reviews, Entry{Review: r, Tag: t, Name: names[t], URI: "m2://" + t})
		e.Verdicts[r.Verdict]++
	}
	out := make([]Export, 0, len(by))
	for _, id := range slices.Sorted(maps.Keys(by)) {
		e := by[id]
		slices.SortFunc(e.Reviews, func(a, b Entry) int {
			return cmp.Or(cmp.Compare(b.Date, a.Date), cmp.Compare(a.ID, b.ID))
		})
		out = append(out, *e)
	}
	return out
}

// Tag returns the tag of the node of a review id, for example 7K3QX2 for r-7K3QX2-01, or "" when the id has another form.
func Tag(id string) string {
	rest, ok := strings.CutPrefix(id, "r-")
	i := strings.LastIndexByte(rest, '-')
	if !ok || i <= 0 {
		return ""
	}
	return rest[:i]
}

// Slug returns the last segment of the stable link of a reviewer: the id with a hyphen for the colon, for example github-ada for github:ada. Each other character that is not a letter, a digit or a hyphen also becomes a hyphen.
func Slug(reviewer string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '-'
	}, reviewer)
}

// WriteText writes the export as text for a CV: a line that says what the reviews are, the link of the export, and one line for each review. The link is "" for no link.
func (e Export) WriteText(w io.Writer, link string) error {
	var b strings.Builder
	n := len(e.Reviews)
	s := "s"
	if n == 1 {
		s = ""
	}
	fmt.Fprintf(&b, "%s: %d fidelity review%s of Lean formalizations for math2", e.Reviewer, n, s)
	switch {
	case n > 0 && e.Reviews[n-1].Date == e.Reviews[0].Date:
		fmt.Fprintf(&b, ", on %s", e.Reviews[0].Date)
	case n > 0:
		fmt.Fprintf(&b, ", from %s to %s", e.Reviews[n-1].Date, e.Reviews[0].Date)
	}
	b.WriteString(".\n")
	if link != "" {
		b.WriteString(link + "\n")
	}
	if n > 0 {
		b.WriteString("\n")
	}
	for _, r := range e.Reviews {
		name := r.Name
		if name == "" {
			name = r.Tag
		}
		fmt.Fprintf(&b, "%s  %s  %s (%s), %s: %s\n", r.Date, r.ID, name, r.URI, r.Decl, r.Verdict)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
