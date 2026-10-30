package textbook

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestMakeindex(t *testing.T) {
	for arg, want := range map[string]string{
		"group":                   "group",
		"group!abelian":           "group, abelian",
		"abelian@\\emph{Abelian}": "Abelian",
		"set|see{collection}":     "",
		"set|seealso{class}":      "",
		"map|(":                   "map",
		"map|)":                   "",
		"map|textbf":              "map",
		`"!bang`:                  "!bang",
		"field!{$p$-adic}":        "field, $p$-adic",
		"  ":                      "",
	} {
		if got := makeindex(arg); got != want {
			t.Errorf("makeindex(%q) = %q, want %q", arg, got, want)
		}
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{"Group, abelian": "group-abelian", "$p$-adic": "p-adic", "!!": "term", "Gödel": "gödel"} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

// fixture reads the files of a directory of testdata.
func fixture(t *testing.T, dir string) (map[string][]byte, []string) {
	t.Helper()
	files := map[string][]byte{}
	var order []string
	entries, err := os.ReadDir(filepath.Join("testdata", dir))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join("testdata", dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[e.Name()] = data
		order = append(order, e.Name())
	}
	return files, order
}

func TestIndex(t *testing.T) {
	for _, tc := range []struct {
		book Book
		want []string
	}{
		{Book{ID: "toc_latex", Commit: "c", Repository: "https://github.com/x/y", Format: LaTeX, Files: regexp.MustCompile(`^[a-c]-[^/]*\.tex$`)}, []string{
			"index/1.1/set set 1.1 b-sets.tex:5",
			"index/1.1/set-empty set, empty 1.1 b-sets.tex:5",
			"index/1.1.1/subset subset 1.1.1 b-sets.tex:7",
			"index/1.1.1/bang !bang 1.1.1 b-sets.tex:7",
			"index/1.2/map map 1.2 b-sets.tex:11",
		}},
		{Book{ID: "toc_pretext", Commit: "c", Repository: "https://github.com/x/y", Format: PreTeXt, Main: "main.ptx"}, []string{
			"index/1.1/set-empty set, empty 1.1 ch-first.ptx:7",
			"index/1.1/collection collection 1.1 ch-first.ptx:7",
		}},
	} {
		files, order := fixture(t, tc.book.ID)
		tr, err := tc.book.build(files, order, true)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, r := range tc.book.termRecords(tr.terms, "") {
			if r.Kind != "concept" || r.Text != "" || r.Meta["environment"] != IndexEnv || !strings.HasSuffix(r.URL, "/blob/c/"+strings.Replace(r.Locator, ":", "#L", 1)) {
				t.Errorf("%s %s is %+v", tc.book.ID, r.ID, r)
			}
			got = append(got, r.ID+" "+r.Title+" "+r.Meta["node"]+" "+r.Locator)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("the terms of %s are\n%q\nwant\n%q", tc.book.ID, got, tc.want)
		}
		if tr2, _ := tc.book.build(files, order, false); len(tr2.terms) != 0 {
			t.Errorf("%s collects terms when it builds only the table of contents", tc.book.ID)
		}
	}
}
