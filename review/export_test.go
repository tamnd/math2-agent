package review_test

import (
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/review"
	"github.com/tamnd/math2-agent/status"
)

func TestExports(t *testing.T) {
	reviews := map[string]status.Review{
		"r-7K3QX2-01": {ID: "r-7K3QX2-01", Reviewer: "orcid:0000-0002-1825-0097", Date: "2026-10-02", Decl: "A", Verdict: status.Faithful},
		"r-3C53TD-02": {ID: "r-3C53TD-02", Reviewer: "github:octocat", Date: "2026-10-02", Decl: "A", Verdict: status.NotFaithful, Class: "wrong-quantifier"},
		"r-G5S1M9-01": {ID: "r-G5S1M9-01", Reviewer: "github:octocat", Date: "2026-10-02", Decl: "B", Verdict: status.Faithful},
		"r-G5S1M9-02": {ID: "r-G5S1M9-02", Reviewer: "github:octocat", Date: "2026-10-05", Decl: "B", Verdict: status.Faithful},
	}
	reviewers := map[string]status.Reviewer{"github:octocat": {ID: "github:octocat", First: "2026-10-02", Areas: []string{"20"}}}
	got := review.Exports(reviews, reviewers, map[string]string{"3C53TD": "Lagrange's theorem"})
	if len(got) != 2 || got[0].Reviewer != "github:octocat" || got[1].Reviewer != "orcid:0000-0002-1825-0097" {
		t.Fatalf("the exports are %+v", got)
	}
	o := got[0]
	ids := []string{o.Reviews[0].ID, o.Reviews[1].ID, o.Reviews[2].ID}
	if strings.Join(ids, " ") != "r-G5S1M9-02 r-3C53TD-02 r-G5S1M9-01" {
		t.Errorf("the order is %v", ids)
	}
	if o.First != "2026-10-02" || len(o.Areas) != 1 || o.Verdicts[status.Faithful] != 2 || o.Verdicts[status.NotFaithful] != 1 {
		t.Errorf("the export is %+v", o)
	}
	if r := o.Reviews[1]; r.Tag != "3C53TD" || r.Name != "Lagrange's theorem" || r.URI != "m2://3C53TD" || r.Class != "wrong-quantifier" {
		t.Errorf("the entry is %+v", r)
	}
	if got[1].First != "" || got[1].Reviews[0].Name != "" {
		t.Errorf("a reviewer with no record: %+v", got[1])
	}
	var b strings.Builder
	if err := got[1].WriteText(&b, ""); err != nil {
		t.Fatal(err)
	}
	want := "orcid:0000-0002-1825-0097: 1 fidelity review of Lean formalizations for math2, on 2026-10-02.\n\n2026-10-02  r-7K3QX2-01  7K3QX2 (m2://7K3QX2), A: faithful\n"
	if b.String() != want {
		t.Errorf("the text is\n%s", b.String())
	}
	if len(review.Exports(nil, reviewers, nil)) != 0 {
		t.Error("a reviewer with no review gets an export")
	}
}

func TestTag(t *testing.T) {
	for id, want := range map[string]string{"r-3C53TD-01": "3C53TD", "r-3C53TD": "", "x-3C53TD-01": "", "r--01": ""} {
		if got := review.Tag(id); got != want {
			t.Errorf("Tag(%q) = %q, want %q", id, got, want)
		}
	}
}
