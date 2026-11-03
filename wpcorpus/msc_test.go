package wpcorpus

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const mscCategories = `version: 1
roots:
  - {title: Mathematics, depth: 1}
msc:
  - {words: topology, code: 54-XX}
  - {words: algebraic topology, code: 55-XX}
  - {words: homotopy theory, code: 55-XX}
  - {words: group theory, code: 20-XX}
  - {words: quantum field theory, code: 81-XX}
  - {words: field theory, code: 12-XX}
  - {words: 3-manifolds, code: 57-XX}
`

func mscTree(t *testing.T, body string) (*CategoryTree, error) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ManifestDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "categories.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return LoadCategoryTree(root)
}

func TestMSCOf(t *testing.T) {
	tree, err := mscTree(t, mscCategories)
	if err != nil {
		t.Fatal(err)
	}
	for text, want := range map[string]string{
		"Algebraic topology":           "55-XX",
		"Theorems in topology":         "54-XX",
		"Quantum field theory":         "81-XX",
		"Field theory":                 "12-XX",
		"3-manifolds":                  "57-XX",
		"Topologists":                  "",
		"Group theory (mathematics)":   "20-XX",
		"In mathematics, group theory": "20-XX",
		"Living people":                "",
	} {
		if got := tree.MSCOf(text); got != want {
			t.Errorf("MSCOf(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestMSCRulesChecked(t *testing.T) {
	for _, body := range []string{
		"version: 1\nroots:\n  - {title: Mathematics, depth: 1}\nmsc:\n  - {words: topology, code: 54}\n",
		"version: 1\nroots:\n  - {title: Mathematics, depth: 1}\nmsc:\n  - {words: '', code: 54-XX}\n",
		"version: 1\nroots:\n  - {title: Mathematics, depth: 1}\nmsc:\n  - {words: Topology, code: 54-XX}\n  - {words: topology, code: 55-XX}\n",
	} {
		if _, err := mscTree(t, body); err == nil || !strings.Contains(err.Error(), "msc rule") {
			t.Errorf("%q: got %v, want an error on an msc rule", body, err)
		}
	}
}

func TestContextPhrase(t *testing.T) {
	for lead, want := range map[string]string{
		"In mathematics, a field $F$ is algebraically closed if every polynomial has a root.": "In mathematics",
		"In mathematics, specifically in algebraic topology, the Euler class is a class.":     "In mathematics, specifically in algebraic topology",
		"In group theory, a group is simple if it has no normal subgroup.":                    "In group theory",
		"In topology. More text.":             "In topology",
		"A group is a set with an operation.": "",
	} {
		if got := ContextPhrase(lead); got != want {
			t.Errorf("ContextPhrase(%q) = %q, want %q", lead, got, want)
		}
	}
}

func TestLeadParagraph(t *testing.T) {
	md := "# Euler class\n\n::: {#o1 .subject cue=lead name=\"Euler class\"}\n\nIn [mathematics](wikipedia://en/18831), specifically in [algebraic topology](wikipedia://en/2), the **Euler class** is a class.\n\n:::\n"
	want := "In mathematics, specifically in algebraic topology, the Euler class is a class."
	if got := LeadParagraph(md); got != want {
		t.Errorf("LeadParagraph = %q, want %q", got, want)
	}
	if got := LeadParagraph("# Empty\n\nNo lead.\n"); got != "" {
		t.Errorf("LeadParagraph with no lead = %q", got)
	}
	if got := LeadLinks(md + "[ring](wikipedia://en/3) after the lead\n"); !slices.Equal(got, []int{18831, 2}) {
		t.Errorf("LeadLinks = %v, want [18831 2]", got)
	}
}

func TestClassifyMSC(t *testing.T) {
	tree, err := mscTree(t, mscCategories)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		cats   []string
		lead   string
		linked []string
		want   []string
	}{
		{[]string{"Algebraic topology", "Homotopy theory", "Living people"}, "", nil, []string{"55-XX"}},
		{[]string{"Topology"}, "In algebraic topology, a space is a space.", nil, []string{"55-XX"}},
		{[]string{"Topology", "Group theory"}, "", nil, []string{"20-XX", "54-XX"}},
		{[]string{"Topology", "Group theory", "Theorems in topology"}, "In mathematics, a thing.", nil, []string{"54-XX"}},
		{[]string{"Living people"}, "In mathematics, a thing.", nil, nil},
		// The links decide a tie, and they cannot add a new area to it.
		{[]string{"Topology", "Group theory"}, "", []string{"20-XX", "12-XX", "12-XX"}, []string{"20-XX"}},
		// The links give a code when the other signals give none.
		{[]string{"Living people"}, "", []string{"55-XX", "54-XX", "55-XX"}, []string{"55-XX"}},
		// The links do not change a code that the other signals give.
		{[]string{"Topology"}, "", []string{"55-XX", "55-XX"}, []string{"54-XX"}},
	}
	for _, c := range cases {
		got := ClassifyMSC(tree, &Metadata{Categories: c.cats}, c.lead, c.linked)
		if !slices.Equal(got.Codes, c.want) {
			t.Errorf("ClassifyMSC(%v, %q, %v) = %v, want %v", c.cats, c.lead, c.linked, got.Codes, c.want)
		}
	}
}
