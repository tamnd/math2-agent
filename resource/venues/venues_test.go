package venues

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/resource"
)

func series(id, title string, issn ...string) resource.Resource {
	return resource.Resource{ID: id, Kind: "series", Title: title, Expressions: []resource.Expression{{Key: "e1", Kind: "edition", IDs: resource.IDs{"issn": issn}}}}
}

func TestBuild(t *testing.T) {
	dup := series("8F2DDDDDDT", "Old copy", "0075-8434")
	dup.Scope = &resource.Scope{Value: resource.Duplicate}
	rs := []resource.Resource{
		series("8F2AAAAAA3", "Graduate Texts in Mathematics", "0072-5285", "2197-5612"),
		series("8F2BBBBBBT", "A series of physics", "1234-5679"),
		{ID: "8F2CCCCCCK", Kind: "book", Title: "A book"},
		dup,
		{ID: "8F2FFFFFF1", Kind: "competition", Title: "Putnam"},
	}
	prev := []Venue{
		{ID: "8F2BBBBBBT", Kind: "series", Title: "Old title", Scope: "border", From: "person"},
		{ID: "8F2EEEEEE7", Kind: "journal", Title: "Gone"},
	}
	known := map[string]Known{"2197-5612": {Scope: "core", From: "springer-series"}, "1234-5679": {Scope: "core", From: "x"}}
	vs, c := Build(prev, rs, known)
	if c != (Counts{Venues: 4, Added: 2, Withdrawn: 1, Scoped: 3}) {
		t.Fatalf("counts %+v", c)
	}
	want := map[string]string{"8F2AAAAAA3": "core", "8F2BBBBBBT": "border", "8F2FFFFFF1": "core"}
	if got := Scopes(vs); !maps.Equal(got, want) {
		t.Fatalf("scopes %v, want %v", got, want)
	}
	if vs[1].Title != "A series of physics" || vs[0].From != "springer-series" || len(vs[0].ISSN) != 2 || vs[2].Status != "withdrawn" || vs[3].From != "RS7.2" {
		t.Fatalf("venues %+v", vs)
	}
	root := t.TempDir()
	if err := Write(root, vs); err != nil {
		t.Fatal(err)
	}
	back, err := Read(root)
	if err != nil || len(back) != 4 || back[0].Scope != "core" {
		t.Fatalf("read %+v, %v", back, err)
	}
}

func TestReadNone(t *testing.T) {
	vs, err := Read(t.TempDir())
	if err != nil || vs != nil {
		t.Fatalf("read %v, %v", vs, err)
	}
}

func TestReadBadScope(t *testing.T) {
	root := t.TempDir()
	if err := Write(root, []Venue{{ID: "8F2AAAAAA3", Kind: "series", Title: "X", Scope: "core"}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, File)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), "scope: core", "scope: maybe", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(root); err == nil {
		t.Fatal("no error for the scope maybe")
	}
}
