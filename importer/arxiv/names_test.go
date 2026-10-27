package arxiv_test

import (
	"slices"
	"testing"

	"github.com/tamnd/math2-agent/importer/arxiv"
	"github.com/tamnd/math2-agent/inventory"
)

func TestNamesFind(t *testing.T) {
	rows := []inventory.Row{
		{ID: "r-hb", Name: "Hahn–Banach theorem", MSC: []string{"46A22"}, Mentions: []string{"wikipedia:Hahn–Banach_theorem"}},
		{ID: "r-zorn", Name: "Zorn's lemma", Mentions: []string{"proofwiki:Zorn's_Lemma"}},
		// Two rows with one name in the same area: the name gives no edge.
		{ID: "r-ext1", Name: "Extension theorem", MSC: []string{"46B20"}, Mentions: []string{"wikipedia:Extension_theorem"}},
		{ID: "r-ext2", Name: "Extension theorem", MSC: []string{"46E35"}, Mentions: []string{"nlab:extension+theorem"}},
		// A name that one paper gives, and a name with no specific word.
		{ID: "r-ax", Name: "Kakeya estimate theorem", Mentions: []string{"ax://paper/2401.00001#THM1"}},
		{ID: "r-main", Name: "Main theorem", Mentions: []string{"wikipedia:Main_theorem"}},
		// A row in another area.
		{ID: "r-far", Name: "Riemann mapping theorem", MSC: []string{"30C20"}, Mentions: []string{"wikipedia:Riemann_mapping_theorem"}},
		{ID: "r-merged", Name: "Baire category theorem", Outcome: &inventory.Outcome{Value: inventory.OutcomeMerged}, Mentions: []string{"wikipedia:Baire"}},
	}
	n := arxiv.NewNames(rows)
	text := "By the Hahn-Banach theorem and Zorn’s lemma, the extension theorem holds. The main theorem, the Kakeya estimate theorem, the Riemann mapping theorem and the Baire category theorem do not count."
	got := n.Find(text, []string{"46B20"})
	if want := []string{"r-hb", "r-zorn"}; !slices.Equal(got, want) {
		t.Errorf("Find gives %v, and it must give %v", got, want)
	}
	if got := n.Find("This is the Hahn–Banach theorem.", []string{"46A22"}, "Hahn-Banach theorem"); len(got) != 0 {
		t.Errorf("the name of the item itself gives %v, and it must give nothing", got)
	}
	var none *arxiv.Names
	if got := none.Find(text, nil); got != nil {
		t.Errorf("no index gives %v", got)
	}
}
