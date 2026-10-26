package models_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/models"
)

func TestDefault(t *testing.T) {
	r := models.Default()
	if len(r.Models) == 0 {
		t.Fatal("the register has no models")
	}
	// Each model that has a price can be called, thus it needs an entry, and each entry needs a price for the budget of a run.
	for id := range llm.Prices {
		if r.Lookup(id) == nil {
			t.Errorf("the model %s has a price and no entry in the register", id)
		}
	}
	for _, m := range r.Models {
		if _, ok := llm.Prices[m.ID]; !ok {
			t.Errorf("the model %s has an entry and no price", m.ID)
		}
		if len(r.TermsOf(m.ID)) != len(m.Terms) {
			t.Errorf("the model %s names terms that the register does not have", m.ID)
		}
	}
	if f := r.Family("claude-opus-5-5"); f != "claude" {
		t.Errorf("the family of claude-opus-5-5 is %q", f)
	}
	if f := r.Family("gpt-0"); f != "" {
		t.Errorf("the family of a model with no entry is %q", f)
	}
	if err := r.Check("gpt-0"); err == nil || !strings.Contains(err.Error(), "no entry") {
		t.Errorf("Check of a model with no entry gives %v", err)
	}
	if err := r.Check("claude-opus-5-5"); err != nil {
		t.Error(err)
	}
}

// The register records the ban of the usage policy of Anthropic on training with outputs (corpus K7.9).
func TestTrainingBan(t *testing.T) {
	r := models.Default()
	var found bool
	for _, d := range r.TermsOf("claude-sonnet-5-5") {
		found = found || slices.ContainsFunc(d.Outputs, func(c models.Clause) bool { return c.Duty == models.NoTraining && c.From == "2026-11-12" })
	}
	if !found {
		t.Error("the terms of claude-sonnet-5-5 have no ban on training from 2026-11-12")
	}
}

func TestParseProblems(t *testing.T) {
	cases := []struct {
		name, data, want string
	}{
		{"version", "version: models.v0\n", `the version is "models.v0"`},
		{"unknown field", "version: models.v1\nmodel: []\n", "field model not found"},
		{"two models", "version: models.v1\nterms: [{id: t, name: T, url: 'https://x', read: 2026-10-26, by: a}]\nmodels: [{id: m, family: f, provider: p, terms: [t]}, {id: m, family: f, provider: p, terms: [t]}]\n", "two entries"},
		{"no terms", "version: models.v1\nmodels: [{id: m, family: f, provider: p}]\n", "names no terms"},
		{"missing terms", "version: models.v1\nmodels: [{id: m, family: f, provider: p, terms: [x]}]\n", `no terms "x"`},
		{"family", "version: models.v1\nterms: [{id: t, name: T, url: 'https://x', read: 2026-10-26, by: a}]\nmodels: [{id: m, family: F, provider: p, terms: [t]}]\n", `the family "F"`},
		{"read", "version: models.v1\nterms: [{id: t, name: T, url: 'https://x', read: soon, by: a}]\n", `the reading "soon"`},
		{"by", "version: models.v1\nterms: [{id: t, name: T, url: 'https://x', read: 2026-10-26}]\n", "who read them"},
		{"url", "version: models.v1\nterms: [{id: t, name: T, url: 'http://x', read: 2026-10-26, by: a}]\n", "not an HTTPS URL"},
		{"duty", "version: models.v1\nterms: [{id: t, name: T, url: 'https://x', read: 2026-10-26, by: a, outputs: [{duty: maybe, says: s}]}]\n", `the duty "maybe"`},
		{"retires", "version: models.v1\nterms: [{id: t, name: T, url: 'https://x', read: 2026-10-26, by: a}]\nmodels: [{id: m, family: f, provider: p, terms: [t], retires: 2027}]\n", "retirement date"},
	}
	for _, c := range cases {
		_, err := models.Parse([]byte(c.data), "test")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: Parse gives %v, want an error with %q", c.name, err, c.want)
		}
	}
}
