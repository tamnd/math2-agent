package prov

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/node"
)

const discloseNode = `---
tag: 3C53TD
kind: statement
name: Lagrange's theorem
msc: [20D60]
formal:
  - system: lean4
    library: mathlib
    decl: Subgroup.card_subgroup_dvd_card
    match: stronger
    adapter: Math2.Match.lagrange
sources:
  - book: isbn:9780471433347
    locator: Section 3.2
  - arxiv: 2401.01234v2
    locator: Theorem 1
  - palomar: lagrange-1
    locator: entry
---

## Statement

Let $G$ be a finite [[group|G5S1M9]] and let $H$ be a subgroup of $G$.
Then the order of $H$ divides the order of $G$.

## Explanations

### L1 agent

A small group fits into a big group a whole number of times.
`

// discloseFacts returns facts with a person, two agent runs that changed the node and one run that failed.
func discloseFacts(t *testing.T) Facts {
	t.Helper()
	n, err := node.Parse("nodes/3C5/3C53TD.md", []byte(discloseNode))
	if err != nil {
		t.Fatal(err)
	}
	log := []Line{
		{Field: Whole, Op: Add, Human: "github:writer", At: "2026-09-01T10:00:00Z"},
		{Field: "formal[0]", Op: Add, Agent: "m2-agent/formalize@0.9.1", Model: "claude-opus-5-5", Prompt: "formalize/3", Run: "r-2026-09-02-1", At: "2026-09-02T10:00:00Z"},
		{Field: "formal[0]", Op: Confirm, Human: "orcid:0000-0002-1825-0097", At: "2026-09-03T10:00:00Z"},
		{Field: "body.explanations.L1", Op: Add, Agent: "m2-agent/explain@1.2.0", Model: "claude-sonnet-5-5", Prompt: "explain/7", Run: "r-2026-09-04-2", At: "2026-09-04T10:00:00Z"},
	}
	runs := map[string]*Run{
		"r-2026-09-02-1": {ID: "r-2026-09-02-1", Agent: "m2-agent/formalize@0.9.1", Prompt: "formalize/3", Models: []string{"claude-opus-5-5"}, Tools: map[string]string{"lean": "v4.35.0"}, OnBehalfOf: "github:writer", Started: "2026-09-02T09:00:00Z", Outcome: "success", Seconds: 1800, SpendUSD: 2.5},
		"r-2026-09-04-2": {ID: "r-2026-09-04-2", Agent: "m2-agent/explain@1.2.0", Prompt: "explain/7", Models: []string{"claude-sonnet-5-5"}, OnBehalfOf: "schedule:weekly-explain", Started: "2026-09-04T09:00:00Z", Outcome: "success", Seconds: 120, SpendUSD: 0.25},
		"r-2026-09-05-3": {ID: "r-2026-09-05-3", Agent: "m2-agent/prove@0.3.0", Prompt: "prove/2", Models: []string{"claude-haiku-5-5"}, OnBehalfOf: "schedule:nightly", Started: "2026-09-05T09:00:00Z", Outcome: "failure", Targets: []string{"3C53TD"}, Seconds: 60, SpendUSD: 0.05},
		"r-2026-09-06-4": {ID: "r-2026-09-06-4", Agent: "m2-agent/prove@0.3.0", Prompt: "prove/2", Models: []string{"claude-haiku-5-5"}, OnBehalfOf: "schedule:nightly", Started: "2026-09-06T09:00:00Z", Outcome: "failure", Targets: []string{"G5S1M9"}},
	}
	build := map[string]Binding{"Subgroup.card_subgroup_dvd_card": {Level: "E5", Axioms: []string{"propext", "Quot.sound", "Classical.choice"}, Fidelity: "F2", Reviewers: []string{"orcid:0000-0002-1825-0097"}}}
	return Facts{Node: n, Log: log, Runs: runs, Build: build, Mathlib: "56a005855ea17e0b0958f73ec801ae140b0a1679", Repo: "https://github.com/tamnd/math2", Commit: "293ecb3"}
}

func TestDisclose(t *testing.T) {
	got := Disclose(discloseFacts(t))
	for _, want := range []string{
		"m2://3C53TD, Lagrange's theorem at the commit 293ecb3.",
		"These people changed it themselves: github:writer.",
		"These people checked the text of agents: orcid:0000-0002-1825-0097.",
		"The agent m2-agent/formalize@0.9.1 changed 1 field in the run r-2026-09-02-1, on behalf of github:writer. It used the models claude-opus-5-5 and the prompt formalize/3",
		"on behalf of schedule:weekly-explain",
		"name 1 run on this node that did not succeed, and math2 did not merge their output: r-2026-09-05-3.",
		"The runs took 33 minutes and spent 2.80 US dollars in all.",
		"The kind of agent contribution is: " + AgentSupport + ".",
		"An agent wrote these fields and no person checked them: body.explanations.L1.",
		"bound to Subgroup.card_subgroup_dvd_card in lean4 mathlib, and the match is stronger, with the adapter Math2.Match.lagrange.",
		"the evidence level E5 and the fidelity level F2, with reviews by orcid:0000-0002-1825-0097.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the disclosure has no %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "r-2026-09-06-4") {
		t.Error("the disclosure names a run on another node")
	}
	if strings.Count(got, "\n") != 1 {
		t.Errorf("the disclosure is not one paragraph:\n%s", got)
	}
}

func TestDiscloseLittle(t *testing.T) {
	f := discloseFacts(t)
	f.Log, f.Runs, f.Build, f.Commit = nil, nil, nil, ""
	got := Disclose(f)
	for _, want := range []string{"has no provenance log in prov/3C5/3C53TD.jsonl", "There is no build", NoAgent} {
		if !strings.Contains(got, want) {
			t.Errorf("the disclosure has no %q:\n%s", want, got)
		}
	}
	f = discloseFacts(t)
	for id, r := range f.Runs {
		r.Seconds, r.SpendUSD = 0, 0
		f.Runs[id] = r
	}
	if got := Disclose(f); !strings.Contains(got, "do not record the time or the spend of the runs") {
		t.Errorf("a disclosure with no cost says:\n%s", got)
	}
	f.Runs["r-2026-09-02-1"].Seconds = 90
	if got := Disclose(f); !strings.Contains(got, "The runs took 2 minutes and spent 0.00 US dollars, but the summaries of 2 runs do not record") {
		t.Errorf("a disclosure with some costs says:\n%s", got)
	}
}

func TestKind(t *testing.T) {
	person := Line{Field: Whole, Op: Add, Human: "github:writer"}
	confirm := Line{Field: "proof", Op: Confirm, Human: "github:writer"}
	explain := Line{Field: "body.explanations.L1", Op: Add, Agent: "m2-agent/explain@1.2.0", Run: "r-2026-09-01-1"}
	prove := Line{Field: "formal[0]", Op: Change, Agent: "m2-agent/prove@0.3.0", Run: "r-2026-09-01-2"}
	for _, c := range []struct {
		log  []Line
		want string
	}{
		{nil, NoAgent},
		{[]Line{person}, NoAgent},
		{[]Line{person, explain}, AgentSupport},
		{[]Line{prove, confirm}, AgentResult},
		{[]Line{person, prove}, Together},
	} {
		if got := Kind(c.log); got != c.want {
			t.Errorf("Kind(%v) = %q, want %q", c.log, got, c.want)
		}
	}
}

// The enums of formalization.yaml v0.4.
var (
	fyMethods   = []string{"manual", "copilot", "agent", "autonomous", "other"}
	fySourceRel = []string{"", "formalizes", "adapts", "independently-proves", "background", "other"}
	fyRelRel    = []string{"", "builds-on", "adapts", "independent", "supersedes", "other"}
	fyRoles     = []string{"", "substantive-development", "thin-wrapper"}
	fyReviews   = []string{"unchecked", "agent-reviewed", "self-assessed", "peer-reviewed", "author-verified"}
)

// checkFY checks the keys and the values that formalization.yaml v0.4 requires.
func checkFY(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var y map[string]any
	if err := yaml.Unmarshal(data, &y); err != nil {
		t.Fatalf("the file is not YAML: %v\n%s", err, data)
	}
	obj := func(v any) map[string]any { m, _ := v.(map[string]any); return m }
	arr := func(v any) []any { a, _ := v.([]any); return a }
	for _, k := range []string{"project", "sources", "automation", "review"} {
		if y[k] == nil {
			t.Errorf("the file has no %s", k)
		}
	}
	p := obj(y["project"])
	if p["name"] == "" || p["license"] == "" || len(arr(p["authors"])) == 0 {
		t.Errorf("the project is %v", p)
	}
	if r := obj(y["repository"]); r != nil {
		if !slices.Contains(fyRoles, r["role"].(string)) {
			t.Errorf("the repository role is %v", r["role"])
		}
		if r["role"] == "thin-wrapper" && obj(r["substantive_formalization"])["id"] == nil {
			t.Error("a thin wrapper has no substantive formalization")
		}
	}
	if len(arr(y["sources"])) == 0 {
		t.Error("the file has no source")
	}
	for _, s := range arr(y["sources"]) {
		s := obj(s)
		rel, _ := s["relationship"].(string)
		if s["title"] == nil || !slices.Contains(fySourceRel, rel) {
			t.Errorf("the source is %v", s)
		}
	}
	for _, s := range arr(y["related_formalizations"]) {
		s := obj(s)
		rel, _ := s["relationship"].(string)
		if s["id"] == nil || !slices.Contains(fyRelRel, rel) {
			t.Errorf("the related formalization is %v", s)
		}
	}
	methods := arr(obj(y["automation"])["methods"])
	if len(methods) == 0 {
		t.Error("the automation has no method")
	}
	for _, m := range methods {
		if !slices.Contains(fyMethods, obj(m)["method"].(string)) {
			t.Errorf("the method is %v", m)
		}
	}
	if s, _ := obj(y["review"])["status"].(string); !slices.Contains(fyReviews, s) {
		t.Errorf("the review status is %q", s)
	}
	return y
}

func TestFormalization(t *testing.T) {
	data, err := Formalization(discloseFacts(t))
	if err != nil {
		t.Fatal(err)
	}
	y := checkFY(t, data)
	if !strings.HasPrefix(string(data), "version: v0.4\nproject:\n") {
		t.Errorf("the file does not start with the version and the project:\n%s", data)
	}
	for _, want := range []string{
		"description: 'The math2 node m2://3C53TD, a statement: Let $G$ be a finite group and let $H$ be a subgroup of $G$. Then the order of $H$ divides the order of $G$.'",
		"authors:\n    - github:writer\n  license: Apache-2.0\n",
		"role: thin-wrapper",
		"id: https://github.com/leanprover-community/mathlib4\n    revision: 56a005855ea17e0b0958f73ec801ae140b0a1679",
		"id: m2://3C53TD",
		"relationship: adapts",
		"https://github.com/tamnd/math2/blob/293ecb3/nodes/3C5/3C53TD.md",
		"id: isbn:9780471433347\n    type: book\n    location: Section 3.2",
		"id: arXiv:2401.01234v2",
		"id: palomar:lagrange-1",
		"msc2020:\n    - 20D60",
		"method: manual",
		"method: agent\n      models:\n        - claude-opus-5-5\n      framework: math2-agent, the role formalize\n      tool_setup: lean v4.35.0\n      cost:\n        wall_time: 30 minutes\n        spend_usd: 2.5",
		"method: autonomous",
		"spend_usd: 2.75",
		"sorry_count: 0",
		"declaration: Math2.Match.lagrange",
		"divergences: Subgroup.card_subgroup_dvd_card is stronger than the statement of the node. The adapter Math2.Match.lagrange states the node from it.",
		"status: peer-reviewed",
		"informal: m2://3C53TD\n    formal: Subgroup.card_subgroup_dvd_card",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("the file has no %q:\n%s", want, data)
		}
	}
	if y["alignment"] == nil || y["acknowledgements"] == nil {
		t.Error("the file has no alignment or no acknowledgements")
	}
}

func TestFormalizationLittle(t *testing.T) {
	f := discloseFacts(t)
	f.Log, f.Runs, f.Build, f.Mathlib = nil, nil, nil, ""
	data, err := Formalization(f)
	if err != nil {
		t.Fatal(err)
	}
	y := checkFY(t, data)
	if a := y["project"].(map[string]any)["authors"].([]any); len(a) != 1 || a[0] != "the math2 contributors" {
		t.Errorf("the authors are %v", a)
	}
	for _, want := range []string{"method: other", "status: unchecked", "There is no build"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("the file has no %q:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), "sorry_count") || strings.Contains(string(data), "revision") {
		t.Errorf("the file gives values that it does not know:\n%s", data)
	}

	n, err := node.Parse("nodes/G5S/G5S1M9.md", []byte("---\ntag: G5S1M9\nkind: definition\nname: Group\n---\n\n## Statement\n\nA set with an operation.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Formalization(Facts{Node: n}); !errors.Is(err, ErrNoBinding) {
		t.Errorf("a node with no binding gives %v", err)
	}
}
