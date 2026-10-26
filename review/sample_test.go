package review_test

import (
	"maps"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/review"
	"github.com/tamnd/math2-agent/status"
)

var sampleReviewers = map[string]status.Reviewer{
	"github:ada":   {ID: "github:ada", Areas: []string{"20"}, Affiliations: []string{"ror:1"}},
	"github:bob":   {ID: "github:bob", Areas: []string{"03"}},
	"github:carol": {ID: "github:carol", Areas: []string{"20-XX"}},
	"github:dave":  {ID: "github:dave", Areas: []string{"20D"}, Affiliations: []string{"ror:1"}},
}

func TestDraw(t *testing.T) {
	in := queueInput(t)
	s, err := review.Draw(in, sampleReviewers, "2026-11", "s1")
	if err != nil {
		t.Fatal(err)
	}
	want := []review.Sample{{Month: "2026-11", Seed: "s1", Tag: "1C0000", Decl: "Subgroup.card_subgroup_dvd_card", Level: status.F2, First: []string{"r-1C0000-01"}, Reviewer: "github:carol"}}
	if !reflect.DeepEqual(s, want) {
		t.Errorf("the sample is %+v", s)
	}
	again, _ := review.Draw(in, sampleReviewers, "2026-11", "s1")
	if !reflect.DeepEqual(s, again) {
		t.Errorf("a second draw with the same seed gives %+v", again)
	}
	// Without carol, dave has an area of the node and shares an affiliation with ada, and bob has no area of the node. The area comes first, thus dave gets the re-review.
	others := maps.Clone(sampleReviewers)
	delete(others, "github:carol")
	if s, _ := review.Draw(in, others, "2026-11", "s1"); len(s) != 1 || s[0].Reviewer != "github:dave" {
		t.Errorf("with no carol, the sample is %+v", s)
	}
	if s, _ := review.Draw(in, map[string]status.Reviewer{"github:ada": sampleReviewers["github:ada"]}, "2026-11", "s1"); len(s) != 1 || s[0].Reviewer != "" {
		t.Errorf("with only the first reviewer, the sample is %+v", s)
	}
	for _, c := range [][2]string{{"2026-13", "s1"}, {"November", "s1"}, {"2026-11", ""}, {"2026-11", "a b"}} {
		if _, err := review.Draw(in, sampleReviewers, c[0], c[1]); err == nil {
			t.Errorf("Draw accepts the month %q and the seed %q", c[0], c[1])
		}
	}
}

func TestSamplesRegister(t *testing.T) {
	root := t.TempDir()
	if s, err := review.LoadSamples(root); err != nil || s != nil {
		t.Fatalf("a tree with no register: %v, %v", s, err)
	}
	a := review.Sample{Month: "2026-11", Seed: "s1", Tag: "1C0000", Decl: "D", Level: status.F2, First: []string{"r-1C0000-01", "r-1C0000-02"}, Reviewer: "github:carol"}
	b := review.Sample{Month: "2026-11", Seed: "s1", Tag: "1D0000", Decl: "E", Level: status.F3, First: []string{}}
	if err := review.AppendSamples(root, []review.Sample{a, b}); err != nil {
		t.Fatal(err)
	}
	c := a
	c.Reviewer = "github:bob"
	if err := review.AppendSamples(root, []review.Sample{c}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "registers", "samples.reg"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# month seed tag decl level first reviewer\n2026-11 s1 1C0000 D F2 r-1C0000-01,r-1C0000-02 github:carol\n2026-11 s1 1D0000 E F3 - -\n2026-11 s1 1C0000 D F2 r-1C0000-01,r-1C0000-02 github:bob\n"
	if string(data) != want {
		t.Errorf("the register is\n%s", data)
	}
	s, err := review.LoadSamples(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, []review.Sample{c, b}) {
		t.Errorf("the samples are %+v", s)
	}
	for _, line := range []string{"2026-11 s1 1C0000 D F1 - -", "2026-11 s1 1C0000 D F2 -", "26-11 s1 1C0000 D F2 - -"} {
		if err := os.WriteFile(filepath.Join(root, "registers", "samples.reg"), []byte(line+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := review.LoadSamples(root); err == nil || !strings.Contains(err.Error(), "line 1") {
			t.Errorf("the record %q: %v", line, err)
		}
	}
}

func TestMeasure(t *testing.T) {
	in := queueInput(t)
	samples := []review.Sample{
		{Month: "2026-11", Tag: "1C0000", Decl: "Subgroup.card_subgroup_dvd_card", Level: status.F2, First: []string{"r-1C0000-01"}, Reviewer: "github:carol"},
		{Month: "2026-11", Tag: "1A0000", Decl: "Group", Level: status.F2, First: []string{"r-1A0000-01"}, Reviewer: "github:bob"},
		{Month: "2026-11", Tag: "1B0000", Decl: "Nat.zero_add", Level: status.F3, First: []string{}, Reviewer: ""},
	}
	reviews := map[string]status.Review{
		"r-1C0000-01": {ID: "r-1C0000-01", Reviewer: "github:ada", Date: "2026-10-01", Decl: "Subgroup.card_subgroup_dvd_card", Verdict: status.Faithful},
		"r-1C0000-02": {ID: "r-1C0000-02", Reviewer: "github:carol", Date: "2026-10-20", Decl: "Subgroup.card_subgroup_dvd_card", Verdict: status.Faithful},
		"r-1C0000-03": {ID: "r-1C0000-03", Reviewer: "github:carol", Date: "2026-11-05", Decl: "Other", Verdict: status.Faithful},
		"r-1C0000-04": {ID: "r-1C0000-04", Reviewer: "github:carol", Date: "2026-11-06", Decl: "Subgroup.card_subgroup_dvd_card", Verdict: status.NotFaithful, Class: "M-def"},
		"r-1C0000-05": {ID: "r-1C0000-05", Reviewer: "github:carol", Date: "2026-11-07", Decl: "Subgroup.card_subgroup_dvd_card", Verdict: status.Faithful},
	}
	results, rates := review.Measure(samples, reviews, in.Nodes)
	if r := results[0]; r.State != review.Disagrees || r.Review != "r-1C0000-04" || r.Class != "M-def" || r.Area != "20" {
		t.Errorf("the first result is %+v", r)
	}
	if results[1].State != review.Open || results[2].State != review.Unassigned || results[2].Area != "03" {
		t.Errorf("the results are %+v", results)
	}
	want := []review.Rate{
		{Level: status.F2, Area: "all", Drawn: 2, Done: 1, Disagrees: 1},
		{Level: status.F2, Area: "20", Drawn: 2, Done: 1, Disagrees: 1},
		{Level: status.F3, Area: "all", Drawn: 1},
		{Level: status.F3, Area: "03", Drawn: 1},
	}
	if !reflect.DeepEqual(rates, want) {
		t.Errorf("the rates are %+v", rates)
	}
	if rates[0].Share() != 1 || !math.IsNaN(rates[2].Share()) {
		t.Errorf("the shares are %v and %v", rates[0].Share(), rates[2].Share())
	}
}
