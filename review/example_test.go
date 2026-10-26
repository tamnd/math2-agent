package review_test

import (
	"fmt"
	"os"

	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/review"
	"github.com/tamnd/math2-agent/status"
)

func ExampleFile() {
	fmt.Println(review.File("3C53TD"))
	// Output: reviews/3C/3C53TD.yaml
}

func ExamplePerson() {
	for _, id := range []string{"github:octocat", "orcid:0000-0002-1825-0097", "github:dependabot[bot]", "m2-agent/review@1.0.0"} {
		fmt.Println(id, review.Person(id))
	}
	// Output:
	// github:octocat true
	// orcid:0000-0002-1825-0097 true
	// github:dependabot[bot] false
	// m2-agent/review@1.0.0 false
}

func ExampleExports() {
	reviews := map[string]status.Review{
		"r-3C53TD-01": {ID: "r-3C53TD-01", Reviewer: "github:octocat", Date: "2026-10-02", Decl: "Subgroup.card_subgroup_dvd_card", Verdict: status.Faithful},
		"r-G5S1M9-01": {ID: "r-G5S1M9-01", Reviewer: "github:octocat", Date: "2026-10-09", Decl: "Group", Verdict: status.FaithfulWithConventions},
	}
	names := map[string]string{"3C53TD": "Lagrange's theorem", "G5S1M9": "Group"}
	for _, e := range review.Exports(reviews, nil, names) {
		_ = e.WriteText(os.Stdout, "https://tamnd.github.io/math2/reviewers/"+review.Slug(e.Reviewer)+"/")
	}
	// Output:
	// github:octocat: 2 fidelity reviews of Lean formalizations for math2, from 2026-10-02 to 2026-10-09.
	// https://tamnd.github.io/math2/reviewers/github-octocat/
	//
	// 2026-10-09  r-G5S1M9-01  Group (m2://G5S1M9), Group: faithful-with-conventions
	// 2026-10-02  r-3C53TD-01  Lagrange's theorem (m2://3C53TD), Subgroup.card_subgroup_dvd_card: faithful
}

func ExampleSlug() {
	fmt.Println(review.Slug("github:octocat"))
	fmt.Println(review.Slug("orcid:0000-0002-1825-0097"))
	// Output:
	// github-octocat
	// orcid-0000-0002-1825-0097
}

func ExampleQueue() {
	n, err := node.Parse("nodes/1B/1B0000.md", []byte("---\ntag: 1B0000\nkind: statement\nname: Zero add\nformal:\n  - {system: lean4, library: mathlib, decl: Nat.zero_add, match: exact}\n---\n\n## Statement\n\nFor each natural number n, 0 + n = n.\n"))
	if err != nil {
		panic(err)
	}
	records := map[string]lean.Record{"Nat.zero_add": {Decl: "Nat.zero_add", TypeHash: "blake3:zeroadd"}}
	passed := map[status.Key]bool{{Tag: "1B0000", Decl: "Nat.zero_add"}: true}
	st := status.Compute(status.Input{Nodes: []*node.Node{n}, Records: records, Battery: passed})
	for _, t := range review.Queue(review.Input{Nodes: []*node.Node{n}, Status: st}, review.Filter{}) {
		fmt.Println(t.Tag, t.Fidelity, t.Decl, t.Dependents)
	}
	// Output: 1B0000 F1 Nat.zero_add 0
}

func ExampleRate_Share() {
	r := review.Rate{Level: status.F2, Area: "all", Drawn: 60, Done: 50, Disagrees: 1}
	fmt.Printf("%s %.1f%%\n", r.Level, 100*r.Share())
	// Output: F2 2.0%
}
