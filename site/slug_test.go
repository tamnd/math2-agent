package site

import (
	"fmt"
	"testing"
)

func ExampleSlug() {
	fmt.Println(Slug("Lagrange's theorem"))
	fmt.Println(Slug("Cauchy–Schwarz inequality"))
	fmt.Println(Slug("Gödel's incompleteness theorems"))
	fmt.Println(Slug("  L^p space (functional analysis) "))
	// Output:
	// lagranges-theorem
	// cauchy-schwarz-inequality
	// godels-incompleteness-theorems
	// l-p-space-functional-analysis
}

func TestAssignSlugs(t *testing.T) {
	vs := []*nodeView{
		{Tag: "BBBBBB", Kind: "definition", Name: "Subgroup"},
		{Tag: "AAAAAA", Kind: "concept", Name: "Subgroup"},
		{Tag: "CCCCCC", Kind: "definition", Name: "Subgroup"},
		{Tag: "DDDDDD", Kind: "statement", Name: "∑"},
		{Tag: "EEEEEE", Kind: "concept", Name: "Subgroup definition"},
	}
	assignSlugs(vs)
	want := map[string]string{"AAAAAA": "subgroup", "BBBBBB": "subgroup-bbbbbb", "CCCCCC": "subgroup-cccccc", "DDDDDD": "dddddd", "EEEEEE": "subgroup-definition"}
	for _, v := range vs {
		if v.Slug != want[v.Tag] {
			t.Errorf("the slug of %s is %q, want %q", v.Tag, v.Slug, want[v.Tag])
		}
	}
}
