package cli

import (
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/competitions"
)

func TestResCompetitions(t *testing.T) {
	root := t.TempDir()
	rs := []resource.Resource{
		{ID: "8F2AAAAAA3", Kind: "competition", Title: "Kvant problems"},
		{ID: "8F2BBBBBBT", Kind: "problem-set", Title: "2022", Date: "2022", Relations: []resource.Relation{{Kind: "part_of", To: "8F2AAAAAA3"}}},
	}
	if err := resource.WriteResources(root, rs); err != nil {
		t.Fatal(err)
	}
	if err := competitions.Write(root, []competitions.Competition{{Name: "Kvant problems", Since: 2021, Resource: "8F2AAAAAA3"}}); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("res", "competitions", "-root", root, "-wikidata", "none", "-year", "2024")
	if code != ExitOK || !strings.Contains(stdout, "1 competitions have a gap, with 2 missing years") || !strings.Contains(stdout, "Kvant problems: 2021 2023") {
		t.Fatalf("m2 res competitions = %d\n%s\n%s", code, stdout, stderr)
	}
}

func TestResVenues(t *testing.T) {
	root := t.TempDir()
	rs := []resource.Resource{
		{ID: "8F2AAAAAA3", Kind: "series", Title: "Graduate Texts in Mathematics", Expressions: []resource.Expression{{Key: "e1", Kind: "edition", IDs: resource.IDs{"issn": {"0072-5285"}}}}},
		{ID: "8F2BBBBBBT", Kind: "book", Title: "Algebra", Relations: []resource.Relation{{Kind: "part_of", To: "8F2AAAAAA3"}}},
	}
	if err := resource.WriteResources(root, rs); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("res", "venues", "-root", root)
	if code != ExitOK || !strings.Contains(stdout, "1 venues in the register; added 1, withdrew 0, 1 with a scope") {
		t.Fatalf("m2 res venues = %d\n%s\n%s", code, stdout, stderr)
	}
	code, stdout, stderr = run("res", "scope", "-root", root, "-no-model")
	if code != ExitOK || !strings.Contains(stdout, "in 1,") {
		t.Fatalf("m2 res scope = %d\n%s\n%s", code, stdout, stderr)
	}
	back, err := resource.ReadResources(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range back {
		if r.ID == "8F2BBBBBBT" && (r.Scope == nil || r.Scope.Value != resource.In) {
			t.Fatalf("the book of a core series has the scope %+v", r.Scope)
		}
	}
}
