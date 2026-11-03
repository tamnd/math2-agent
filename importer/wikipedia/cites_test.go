package wikipedia

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/resource"
	resresolve "github.com/tamnd/math2-agent/resource/resolve"
	"github.com/tamnd/math2-agent/wpcorpus"
)

func TestCitationOf(t *testing.T) {
	c, bad := CitationOf(map[string]string{
		"last1": "Aigner", "first1": "Martin", "last2": "Ziegler", "first2": "Günter M.",
		"title": "Proofs from THE BOOK", "edition": "Revised and enlarged sixth", "date": "2018",
		"isbn": "978-3-662-57264-1", "doi": "not a doi", "publisher": "Springer",
	})
	if want := []string{"Martin Aigner", "Günter M. Ziegler"}; !slices.Equal(c.Authors, want) {
		t.Errorf("the authors are %q, want %q", c.Authors, want)
	}
	if c.Title != "Proofs from THE BOOK" || c.Year != 2018 || c.Venue != "Springer" {
		t.Errorf("the citation is %+v", c)
	}
	if !slices.Equal(c.IDs, []string{"isbn:9783662572641"}) || !slices.Equal(bad, []string{"doi:not a doi"}) {
		t.Errorf("the ids are %q and the bad ids are %q", c.IDs, bad)
	}
	if c.Edition != 0 {
		t.Errorf("the edition is %d. Revised is not an ordinal, so it must be 0", c.Edition)
	}
	if c, _ := CitationOf(map[string]string{"title": "Analysis", "edition": "3rd"}); c.Edition != 3 {
		t.Errorf("the edition of 3rd is %d", c.Edition)
	}
	if c, _ := CitationOf(map[string]string{"title": "Analysis", "edition": "second"}); c.Edition != 2 {
		t.Errorf("the edition of second is %d", c.Edition)
	}
}

func TestResolveCitations(t *testing.T) {
	rs := []resource.Resource{
		{ID: "8F2K1QW7ZJ", Kind: "textbook", Title: "Principles of Mathematical Analysis", Creators: []resource.Creator{{Name: "Rudin, Walter", Role: "author"}}, Date: "1953", Expressions: []resource.Expression{
			{Key: "e3", Kind: "edition", Label: "3rd edition", Date: "1976", Manifestations: []resource.Manifestation{{Key: "m1", Kind: "print", IDs: resource.IDs{"isbn": {"9780070542358"}}}}},
		}},
	}
	r := &resresolve.Resolver{Index: resresolve.Build(rs)}
	objs := []wpcorpus.Object{
		{Ref: "o1", Kind: "statement/theorem"},
		{Ref: "o2", Kind: "citation", Cue: "ref", Fields: map[string]string{"last": "Rudin", "first": "Walter", "title": "Principles of Mathematical Analysis", "isbn": "0-07-054235-X", "page": "47"}},
		{Ref: "o3", Kind: "citation", Cue: "ref", Fields: map[string]string{"last": "Rudin", "first": "Walter", "title": "Principles of Mathematical Analysis", "year": "1953"}},
		{Ref: "o4", Kind: "citation", Cue: "sfn"},
		{Ref: "o5", Kind: "citation", Cue: "ref", Fields: map[string]string{"title": "Widgets", "doi": "10.9999/widget.1"}},
	}
	dead := func(id string) bool { return id == "doi:10.9999/widget.1" }
	f, err := ResolveCitations(context.Background(), r, 100, 5, "Widget theorem", objs, dead)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range f.Citations {
		got = append(got, c.Ref+" "+c.Status+" "+c.Resource+" "+c.Locator+" "+strings.Join(c.DeadIDs, ","))
	}
	want := []string{
		"o2 found 8F2K1QW7ZJ page 47 ",
		"o3 found 8F2K1QW7ZJ  ",
		"o4 no-fields   ",
		"o5 not-found   doi:10.9999/widget.1",
	}
	if !slices.Equal(got, want) {
		t.Errorf("the citations are\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	data, err := MarshalCiteFile(f)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseCiteFile(data)
	if err != nil || len(back.Citations) != 4 || back.Citations[3].DeadIDs[0] != "doi:10.9999/widget.1" {
		t.Errorf("the citation file does not read back: %v\n%s", err, data)
	}
	if !strings.Contains(string(data), "Revision 5 has 4 citations. 2 resolve to a resource, 0 have candidates, 1 do not resolve, and 1 have no fields to resolve.") {
		t.Errorf("the summary of the citation file is wrong:\n%s", data)
	}
}
