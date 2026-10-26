package site

import (
	"cmp"
	"fmt"

	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/review"
	"github.com/tamnd/math2-agent/status"
)

// reviewerView is the page of one reviewer: the export of spec 16.4 with the links of the site.
type reviewerView struct {
	review.Export
	Who     link   // the reviewer, with the link to their GitHub or ORCID profile
	URL     string // the stable link of the page, to cite
	Page    string // the path of the page on the site
	JSONURL string
	Entries []reviewEntry
}

type reviewEntry struct {
	review.Entry
	Node link   // the node
	File string // the review file at the commit of the site
}

// loadReviews reads the reviews and the reviewers of the tree, and makes the page of each reviewer.
func (b *builder) loadReviews() error {
	reviews, err := status.LoadReviews(b.opt.Content)
	if err != nil {
		return fmt.Errorf("site: %w", err)
	}
	reviewers, err := status.LoadReviewers(b.opt.Content)
	if err != nil {
		return fmt.Errorf("site: %w", err)
	}
	names := map[string]string{}
	for _, v := range b.order {
		names[v.Tag] = v.Name
	}
	commit := cmp.Or(b.opt.Commit, "main")
	for _, e := range review.Exports(reviews, reviewers, names) {
		slug := review.Slug(e.Reviewer)
		v := &reviewerView{Export: e, URL: b.abs("reviewers/" + slug + "/"), Page: b.url("reviewers/" + slug + "/"), JSONURL: b.url("api/reviewers/" + slug + ".json")}
		v.Who = byLink(prov.Line{Human: e.Reviewer})
		for _, r := range e.Reviews {
			v.Entries = append(v.Entries, reviewEntry{Entry: r, Node: b.nodeLink(r.Tag), File: "https://github.com/" + b.opt.Repo + "/blob/" + commit + "/" + review.File(r.Tag)})
		}
		b.reviewers = append(b.reviewers, v)
	}
	return nil
}

// writeReviewers writes the list of the reviewers and the page and the JSON copy of each one.
func (b *builder) writeReviewers() error {
	for _, v := range b.reviewers {
		slug := review.Slug(v.Reviewer)
		if err := b.writeJSON("api/reviewers/"+slug+".json", v.Export); err != nil {
			return err
		}
		p := page{Title: "Reviews by " + v.Who.Text, Description: fmt.Sprintf("The %d fidelity reviews of %s for Math 2.0.", len(v.Reviews), v.Reviewer), Section: "reviewers", JSONURL: v.JSONURL}
		if err := b.writePage("reviewers/"+slug, "reviewer", v, p); err != nil {
			return err
		}
	}
	type entry struct {
		Reviewer string `json:"reviewer"`
		Reviews  int    `json:"reviews"`
		URL      string `json:"url"`
		JSON     string `json:"json"`
	}
	index := []entry{}
	for _, v := range b.reviewers {
		index = append(index, entry{v.Reviewer, len(v.Reviews), v.URL, b.abs("api/reviewers/" + review.Slug(v.Reviewer) + ".json")})
	}
	if err := b.writeJSON("api/reviewers.json", index); err != nil {
		return err
	}
	return b.writePage("reviewers", "reviewers", b.reviewers, page{Title: "Reviewers", Description: "The people who review the Lean formalizations of Math 2.0, with their reviews.", Section: "reviewers", JSONURL: b.url("api/reviewers.json")})
}
