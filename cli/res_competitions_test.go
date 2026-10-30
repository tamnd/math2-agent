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
