package arxiv_test

import (
	"slices"
	"testing"

	"github.com/tamnd/math2-agent/importer/arxiv"
)

func TestItemOf(t *testing.T) {
	for _, c := range []struct {
		item, title, env, want string
	}{
		{"statement/lemma", "Claim", "lem", "statement/lemma"},          // the record decides
		{"unknown", `\protect\theoremname`, "thm", "statement/theorem"}, // a name macro of the title
		{"unknown", "", "lem", "statement/lemma"},
		{"unknown", "", "thm*", "statement/theorem"},
		{"unknown", "", "assum_abbr", "context/hypothesis"},
		{"unknown", "", "acknowledgements", "other/section"},
		{"unknown", "", "ProofOf", "proof"},
		{"unknown", "Definitions", "point", "definition"}, // the title before the environment
		{"unknown", "", "point", "unknown"},               // nothing tells
		{"", "", "say", ""},
	} {
		o := arxiv.Object{Item: c.item, Title: c.title, Env: c.env}
		if got := arxiv.ItemOf(&o); got != c.want {
			t.Errorf("ItemOf(%q, %q, %q) = %q, want %q", c.item, c.title, c.env, got, c.want)
		}
	}
}

func TestCategoryMSC(t *testing.T) {
	got := arxiv.CategoryMSC([]string{"math.AG", "cs.LG", "math.CV", "math.AG"})
	if want := []string{"14-XX", "30-XX", "32-XX"}; !slices.Equal(got, want) {
		t.Errorf("CategoryMSC = %v, want %v", got, want)
	}
	if got := arxiv.CategoryMSC([]string{"math.CA"}); !slices.Equal(got, []string{"26-XX"}) {
		t.Errorf("CategoryMSC(math.CA) = %v, want its main area 26 only", got)
	}
	if got := arxiv.CategoryMSC([]string{"hep-th"}); got != nil {
		t.Errorf("CategoryMSC(hep-th) = %v, want none", got)
	}
}
