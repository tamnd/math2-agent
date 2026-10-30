package textbook_test

import (
	"bytes"
	"compress/gzip"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest/textbook"
	"github.com/tamnd/math2-agent/resource"
)

// outline gives one line for each node of a tree: the depth as dots, the key, the kind, the number and the title.
func outline(ns []resource.Node, depth int) []string {
	var out []string
	for _, n := range ns {
		out = append(out, strings.Repeat(".", depth)+n.Key+" "+n.Kind+" "+n.Number+" "+n.Title)
		out = append(out, outline(n.Children, depth+1)...)
	}
	return out
}

// structure reads the table of contents of the fixtures of a book.
func structure(t *testing.T, b textbook.Book) ([]resource.Node, string, error) {
	t.Helper()
	tr, err := gzip.NewReader(bytes.NewReader(archive(t, b, b.Commit)))
	if err != nil {
		t.Fatal(err)
	}
	return b.Structure(tr)
}

func TestStructureLaTeX(t *testing.T) {
	b := textbook.Book{ID: "toc_latex", Commit: "0123456789abcdef0123456789abcdef01234567", Format: textbook.LaTeX, Files: regexp.MustCompile(`^[a-c]-[^/]*\.tex$`)}
	ns, commit, err := structure(t, b)
	if err != nil {
		t.Fatal(err)
	}
	if commit != b.Commit {
		t.Errorf("the commit is %q", commit)
	}
	want := []string{
		"u1 chapter  Preface",
		"part-I part I Foundations",
		".1 chapter 1 Sets and functions",
		"..1.1 section 1.1 Sets",
		"...1.1.1 subsection 1.1.1 Subsets",
		"..1.u1 section  Exercises",
		"..1.2 section 1.2 Functions and maps",
		".2 chapter 2 Logic",
		"..2.1 section 2.1 Proofs",
		".A appendix A Tables",
		"..A.1 section A.1 Greek letters",
		".ch:index chapter  Index",
	}
	if got := outline(ns, 0); !slices.Equal(got, want) {
		t.Errorf("the tree is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestStructurePreTeXt(t *testing.T) {
	b := textbook.Book{ID: "toc_pretext", Commit: "0123456789abcdef0123456789abcdef01234567", Format: textbook.PreTeXt, Main: "main.ptx"}
	ns, _, err := structure(t, b)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"part-I part I Foundations",
		".1 chapter 1 First $x$ steps",
		"..1.1 section 1.1 Start",
		"...1.1.1 subsection 1.1.1 Sub",
		"..1.2 section 1.2 Next",
		"2 chapter 2 Second",
		".2.1 section 2.1 Only",
		"A appendix A Notation",
		".A.1 section A.1 Symbols",
	}
	if got := outline(ns, 0); !slices.Equal(got, want) {
		t.Errorf("the tree is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestStructureBooks reads the tables of contents of the fixtures of the books of Books.
func TestStructureBooks(t *testing.T) {
	for id, want := range map[string][]string{
		"lebl_ra":         {"1 chapter 1 The derivative", ".1.1 section 1.1 Mean value theorem", "..1.1.1 subsection 1.1.1 Exercises"},
		"hott_book":       {"1 chapter 1 Type theory, the fixture", ".1.1 section 1.1 Homotopies", ".1.u1 section  Exercises"},
		"active_calculus": {"1 chapter 1 Understanding the Derivative", ".1.1 section 1.1 How do we measure velocity?", "..1.1.1 subsection 1.1.1 Position and average velocity"},
	} {
		ns, _, err := structure(t, book(t, id))
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if got := outline(ns, 0); !slices.Equal(got, want) {
			t.Errorf("the tree of %s is\n%s\nwant\n%s", id, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}

func TestStructureErrors(t *testing.T) {
	b := book(t, "levin_dmoi")
	b.Main = "ptx/none.ptx"
	if _, _, err := structure(t, b); err == nil || !strings.Contains(err.Error(), "ptx/none.ptx") {
		t.Errorf("Structure of a book with no main file gives %v", err)
	}
	b = book(t, "lebl_scv")
	b.Files = regexp.MustCompile(`^none\.tex$`)
	if _, _, err := structure(t, b); err == nil {
		t.Error("Structure of an archive with no headings gives no error")
	}
}
