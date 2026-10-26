package site

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReviewers(t *testing.T) {
	content := newContent(t)
	writeFile(t, content, "nodes/7K3/7K3QX2.md", statement)
	writeFile(t, content, "nodes/1H8/1H8D2C.md", concept)
	writeFile(t, content, "registers/reviewers.reg", "github:octocat 2026-10-01 20 -\n")
	writeFile(t, content, "reviews/7K3/7K3QX2.yaml", "reviews:\n  - id: r-7K3QX2-01\n    reviewer: github:octocat\n    date: \"2026-10-02\"\n    decl: Subgroup.card_subgroup_dvd_card\n    statement_hash: blake3:s\n    type_hash: blake3:t\n    verdict: faithful-with-conventions\n    conventions: [\"a finite group\"]\n  - id: r-7K3QX2-02\n    reviewer: orcid:0000-0002-1825-0097\n    date: \"2026-10-03\"\n    decl: Subgroup.card_subgroup_dvd_card\n    statement_hash: blake3:s\n    type_hash: blake3:t\n    verdict: faithful\n")
	out := buildSite(t, content, "")

	list := readFile(t, out, "reviewers/index.html")
	for _, want := range []string{`href="/math2/reviewers/github-octocat/"`, `href="/math2/reviewers/orcid-0000-0002-1825-0097/"`, "octocat"} {
		if !strings.Contains(list, want) {
			t.Errorf("the list of reviewers has no %s", want)
		}
	}
	p := readFile(t, out, "reviewers/github-octocat/index.html")
	for _, want := range []string{`<a href="https://github.com/octocat">octocat</a>`, `href="/math2/node/7K3QX2/"`, "Lagrange&#39;s theorem", `href="https://github.com/o/math2/blob/main/reviews/7K3/7K3QX2.yaml"`, "a finite group", "in the MSC areas 20"} {
		if !strings.Contains(p, want) {
			t.Errorf("the page of the reviewer has no %s:\n%s", want, p)
		}
	}
	if !strings.Contains(p, `Cite this page as <a href="https://example.org/math2/reviewers/github-octocat/">`) {
		t.Error("the page of the reviewer has no stable link to cite")
	}
	if strings.Contains(p, "r-7K3QX2-02") {
		t.Error("the page of a reviewer lists the review of another reviewer")
	}
	if !strings.Contains(readFile(t, out, "reviewers/orcid-0000-0002-1825-0097/index.html"), "has no record of orcid:0000-0002-1825-0097") {
		t.Error("the page of a reviewer with no record does not say so")
	}
	var e struct {
		Reviewer string         `json:"reviewer"`
		Verdicts map[string]int `json:"verdicts"`
		Reviews  []struct {
			ID, Name, URI string
		} `json:"reviews"`
	}
	if err := json.Unmarshal([]byte(readFile(t, out, "api/reviewers/github-octocat.json")), &e); err != nil || e.Reviewer != "github:octocat" || len(e.Reviews) != 1 || e.Reviews[0].URI != "m2://7K3QX2" || e.Verdicts["faithful-with-conventions"] != 1 {
		t.Errorf("the JSON copy is %+v, %v", e, err)
	}
	var index []struct{ Reviewer string }
	if err := json.Unmarshal([]byte(readFile(t, out, "api/reviewers.json")), &index); err != nil || len(index) != 2 {
		t.Errorf("api/reviewers.json is %+v, %v", index, err)
	}
	if !strings.Contains(readFile(t, out, "sitemap.xml"), "reviewers/github-octocat/") {
		t.Error("the sitemap has no page of a reviewer")
	}
}

func TestNoReviewers(t *testing.T) {
	out := buildSite(t, t.TempDir(), "")
	if !strings.Contains(readFile(t, out, "reviewers/index.html"), "No person has reviewed a binding yet") {
		t.Error("the list of reviewers does not say that it is empty")
	}
	if got := strings.TrimSpace(readFile(t, out, "api/reviewers.json")); got != "[]" {
		t.Errorf("api/reviewers.json is %s", got)
	}
}
