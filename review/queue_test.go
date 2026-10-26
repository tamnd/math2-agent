package review_test

import (
	"testing"

	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/review"
	"github.com/tamnd/math2-agent/status"
)

// queueInput returns the input of a build of three nodes. 1A0000 and 1B0000 are at F1, and 1C0000, which uses both, has a review that counts and is at F2.
func queueInput(t *testing.T) review.Input {
	t.Helper()
	texts := map[string]string{
		"1A0000": "---\ntag: 1A0000\nkind: definition\nname: Group\nmsc: [20A05]\nformal:\n  - {system: lean4, library: mathlib, decl: Group, match: exact}\n---\n\n## Definition\n\nA group.\n",
		"1B0000": "---\ntag: 1B0000\nkind: statement\nname: Zero add\nmsc: ['03E10']\nformal:\n  - {system: lean4, library: mathlib, decl: Nat.zero_add, match: exact}\n---\n\n## Statement\n\nFor each natural number n, 0 + n = n.\n",
		"1C0000": "---\ntag: 1C0000\nkind: statement\nname: Lagrange's theorem\nmsc: [20D60]\nuses: [1A0000, 1B0000]\nformal:\n  - {system: lean4, library: mathlib, decl: Subgroup.card_subgroup_dvd_card, match: exact, fidelity_reviews: [r-1C0000-01]}\n---\n\n## Statement\n\nThe order of a subgroup divides the order of the group.\n",
	}
	var nodes []*node.Node
	for _, tag := range []string{"1A0000", "1B0000", "1C0000"} {
		n, err := node.Parse("nodes/"+tag[:3]+"/"+tag+".md", []byte(texts[tag]))
		if err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, n)
	}
	records := map[string]lean.Record{}
	passed := map[status.Key]bool{}
	for _, b := range []status.Key{{Tag: "1A0000", Decl: "Group"}, {Tag: "1B0000", Decl: "Nat.zero_add"}, {Tag: "1C0000", Decl: "Subgroup.card_subgroup_dvd_card"}} {
		records[b.Decl] = lean.Record{Decl: b.Decl, Kind: "theorem", TypeHash: "blake3:" + b.Decl}
		passed[b] = true
	}
	reviews := map[string]status.Review{"r-1C0000-01": {ID: "r-1C0000-01", Reviewer: "github:ada", Decl: "Subgroup.card_subgroup_dvd_card", StatementHash: status.StatementHash(nodes[2]), TypeHash: "blake3:Subgroup.card_subgroup_dvd_card", Verdict: status.Faithful}}
	reviewers := map[string]status.Reviewer{"github:ada": {ID: "github:ada"}}
	authors := map[status.Key]string{{Tag: "1B0000", Decl: "Nat.zero_add"}: "github:bob"}
	st := status.Compute(status.Input{Nodes: nodes, Records: records, Battery: passed, Reviews: reviews, Reviewers: reviewers, Authors: authors})
	return review.Input{Nodes: nodes, Status: st, Records: records, Authors: authors}
}

func tags(tasks []review.Task) []string {
	out := []string{}
	for _, t := range tasks {
		out = append(out, t.Tag)
	}
	return out
}

func TestQueue(t *testing.T) {
	in := queueInput(t)
	q := review.Queue(in, review.Filter{})
	if len(q) != 2 || q[0].Tag != "1A0000" || q[0].Dependents != 1 || q[0].Decl != "Group" || q[0].Fidelity != status.F1 || q[1].Tag != "1B0000" {
		t.Fatalf("the queue is %+v", q)
	}
	for _, c := range []struct {
		name string
		f    review.Filter
		want string
	}{
		{"the author", review.Filter{Reviewer: "github:bob"}, "1A0000"},
		{"an area", review.Filter{Area: "20-XX"}, "1A0000"},
		{"a subject", review.Filter{Area: "03E"}, "1B0000"},
		{"no node in the area", review.Filter{Area: "11"}, ""},
		{"a second review", review.Filter{Second: true}, "1C0000"},
		{"the reviewer of the first review", review.Filter{Second: true, Reviewer: "github:ada"}, ""},
	} {
		if got := tags(review.Queue(in, c.f)); len(got) > 1 || (c.want == "") != (len(got) == 0) || (c.want != "" && got[0] != c.want) {
			t.Errorf("%s: the queue has %v, and the test wants %q", c.name, got, c.want)
		}
	}
}

func TestQueueNotFaithful(t *testing.T) {
	in := queueInput(t)
	for i := range in.Status {
		for j := range in.Status[i].Bindings {
			if in.Status[i].Tag == "1A0000" {
				in.Status[i].Bindings[j].Reviews = []status.ReviewState{{ID: "r-1A0000-01", Reviewer: "github:ada", State: status.NotFaithfulRv}}
			}
		}
	}
	if got := tags(review.Queue(in, review.Filter{})); len(got) != 1 || got[0] != "1B0000" {
		t.Errorf("the queue has %v, and a binding with a not-faithful review does not wait", got)
	}
}
