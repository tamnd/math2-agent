package site

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/resource"
)

func TestResources(t *testing.T) {
	root := newContent(t)
	rudin := resource.Resource{
		ID: "8F2K1QW7ZB", Kind: "textbook", Title: "Principles of Mathematical Analysis", Date: "1953",
		Creators:  []resource.Creator{{Name: "Walter Rudin", Role: "author"}},
		MSC:       []resource.Code{{Code: "26-01", From: "zbmath"}},
		IDs:       resource.IDs{"oclc": {"1502474"}},
		Catalogue: []resource.CatalogueRecord{{Source: "openlibrary", Snapshot: "s1", NativeID: "OL1W", Level: "work"}},
		Expressions: []resource.Expression{{Key: "e3", Kind: "edition", Label: "3rd edition", Language: "en", Date: "1976", Structure: "8F2K1QW7ZB/e3.json",
			Manifestations: []resource.Manifestation{{Key: "m1", Kind: "print", Publisher: "McGraw-Hill", IDs: resource.IDs{"isbn": {"9780070542358"}}}}}},
	}
	dup := resource.Resource{ID: "8F3AAAAAAA", Kind: "textbook", Title: "Principles of mathematical analysis", Scope: &resource.Scope{Value: resource.Duplicate, Target: rudin.ID, By: "a test"},
		Relations: []resource.Relation{{Kind: "version_of", To: rudin.ID + "/e3"}}}
	if err := resource.WriteResources(root, []resource.Resource{rudin, dup}); err != nil {
		t.Fatal(err)
	}
	s := &resource.Structure{Resource: rudin.ID + "/e3", Source: "a test", Snapshot: "s1", Nodes: []resource.Node{{Key: "1", Kind: "chapter", Number: "1", Title: "The Real and Complex Number Systems", Page: "1",
		Children: []resource.Node{{Key: "1.1", Kind: "section", Title: "Introduction", Page: "1"}}}}}
	if err := resource.WriteStructure(root, s); err != nil {
		t.Fatal(err)
	}
	ms := []inventory.Mention{{ID: "m2://rec/rudin/1#1", Source: "rudin", Kind: "statement", Name: "Heine-Borel theorem", Resource: rudin.ID + "/e3", At: &inventory.At{Kind: "theorem", Number: "2.41", Section: "2"}}}
	if err := inventory.WriteMentions(root, "rudin", ms); err != nil {
		t.Fatal(err)
	}
	if err := inventory.WriteRows(root, []inventory.Row{{ID: "7K3QX2M9R2", Mentions: []string{ms[0].ID}, Kind: "statement", Name: "Heine-Borel theorem", Layer: inventory.Textbook, Spread: 1}}); err != nil {
		t.Fatal(err)
	}
	out := buildSite(t, root, "")
	index := readFile(t, out, "res/index.html")
	for _, want := range []string{`href="/math2/res/kind/textbook/"`, "Live resources", "1950s"} {
		if !strings.Contains(index, want) {
			t.Errorf("res/index.html has no %q", want)
		}
	}
	list := readFile(t, out, "res/kind/textbook/index.html")
	if !strings.Contains(list, `<a href="/math2/res/8F2K1QW7ZB/">Principles of Mathematical Analysis</a>`) || !strings.Contains(list, "(duplicate)") {
		t.Error("the list of textbooks does not link to each resource")
	}
	page := readFile(t, out, "res/8F2K1QW7ZB/index.html")
	for _, want := range []string{"<code>m2://res/8F2K1QW7ZB</code>", "Walter Rudin", "McGraw-Hill", "https://openlibrary.org/isbn/9780070542358", "https://www.worldcat.org/oclc/1502474",
		"The Real and Complex Number Systems", `<li class="d1">section`, `<a href="/math2/inventory/row/7K/#7K3QX2M9R2">Heine-Borel theorem</a>`, "theorem 2.41, section 2",
		`<a href="/math2/res/8F3AAAAAAA/">Principles of mathematical analysis</a> is version_of this resource`} {
		if !strings.Contains(page, want) {
			t.Errorf("res/8F2K1QW7ZB/index.html has no %q", want)
		}
	}
	if d := readFile(t, out, "res/8F3AAAAAAA/index.html"); !strings.Contains(d, `The live resource is <a href="/math2/res/8F2K1QW7ZB/">`) {
		t.Error("the page of a duplicate does not link to the live resource")
	}
	if card := readFile(t, out, "inventory/row/7K/index.html"); !strings.Contains(card, `In <a href="/math2/res/8F2K1QW7ZB/#e3">8F2K1QW7ZB/e3</a>, theorem 2.41, section 2`) {
		t.Error("the card of a mention does not link to its resource")
	}
	var sum struct{ Total, Live int }
	if err := json.Unmarshal([]byte(readFile(t, out, "api/resources.json")), &sum); err != nil || sum.Total != 2 || sum.Live != 1 {
		t.Errorf("api/resources.json: %+v, error %v", sum, err)
	}
	checkLinks(t, out)
}
