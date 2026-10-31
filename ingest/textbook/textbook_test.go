package textbook_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/inventory"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/textbook"
	"github.com/tamnd/math2-agent/msc"
	"github.com/tamnd/math2-agent/resource/cluster"
)

// archive makes a git archive of the fixtures of a book like the one GitHub gives, with the files in one directory and the commit in the global header.
func archive(t *testing.T, b textbook.Book, commit string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if commit != "" {
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": commit}}); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join("testdata", b.ID)
	err := fs.WalkDir(os.DirFS(dir), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "book-" + commit + "/" + name, Mode: 0o644, Size: int64(len(data))}); err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = zw.Close()
	return buf.Bytes()
}

// read reads the fixtures of a book at its pinned commit.
func read(t *testing.T, b textbook.Book) []ingest.Record {
	t.Helper()
	tr, err := gzip.NewReader(bytes.NewReader(archive(t, b, b.Commit)))
	if err != nil {
		t.Fatal(err)
	}
	var recs []ingest.Record
	if err := b.Read(tr, func(r ingest.Record) error { recs = append(recs, r); return nil }); err != nil {
		t.Fatalf("%s: %v", b.ID, err)
	}
	return recs
}

func book(t *testing.T, id string) textbook.Book {
	t.Helper()
	for _, b := range textbook.Books {
		if b.ID == id {
			return b
		}
	}
	t.Fatalf("no book %s", id)
	return textbook.Book{}
}

func TestBooks(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range textbook.Books {
		src := b.Source()
		if p := src.Problems(); len(p) > 0 {
			t.Errorf("%s: %s", b.ID, strings.Join(p, "; "))
		}
		if seen[b.ID] {
			t.Errorf("two books have the id %s", b.ID)
		}
		seen[b.ID] = true
		if a, ok := ingest.Lookup(b.ID); !ok || a.Source().ID != b.ID {
			t.Errorf("%s is not registered", b.ID)
		}
		if src.Kind != "textbook" || src.Channel != "git" || src.Class != ingest.Open || src.Attribution == "" || len(b.Commit) != 40 {
			t.Errorf("the source of %s is %+v, at the commit %q", b.ID, src, b.Commit)
		}
		if want := b.Repository + "/archive/" + b.Commit + ".tar.gz"; b.Dump() != want {
			t.Errorf("the dump of %s is %s, want %s", b.ID, b.Dump(), want)
		}
		if (b.Format == textbook.LaTeX) != (b.Files != nil) || (b.Format == textbook.PreTeXt) != (b.Main != "") {
			t.Errorf("%s is in %s with the files %v and the main file %q", b.ID, b.Format, b.Files, b.Main)
		}
	}
	if seen["hefferon_linear_algebra"] {
		t.Error("Hefferon is in the books before a person settles its licence")
	}
	if src := book(t, "lebl_ra").Source(); src.Elected() != "CC-BY-SA-4.0" {
		t.Errorf("the elected licence of lebl_ra is %s", src.Elected())
	}
}

// The records of each book, in order: the id and the kind.
var wantIDs = map[string][]string{
	"open_logic": {
		"content/first-order-logic/syntax-and-semantics/extensionality.tex#defn:agree definition",
		"content/first-order-logic/syntax-and-semantics/extensionality.tex#prop:extensionality statement",
		"content/first-order-logic/syntax-and-semantics/extensionality.tex#proof-1 proof",
		"content/first-order-logic/syntax-and-semantics/extensionality.tex#probtag-1 exercise",
	},
	"hott_book": {
		"basics.tex#defn:homotopy definition", "basics.tex#lem:homotopy-refl statement", "basics.tex#proof-1 proof",
		"basics.tex#eg-1 example", "basics.tex#ex:homotopy-sym exercise",
	},
	"lebl_ra": {
		"ch-der.tex#defn-1 definition", "ch-der.tex#lemma:relmaxdzero statement", "ch-der.tex#proof-1 proof",
		"ch-der.tex#thm:rolle statement", "ch-der.tex#proof-2 proof", "ch-der.tex#example-1 example",
		"ch-der.tex#exercise-1 exercise", "ch-der.tex#exercise-2 exercise",
	},
	"lebl_diffyqs": {
		"ch-first-order-ode.tex#thm:picard statement", "ch-first-order-ode.tex#example-1 example", "ch-first-order-ode.tex#exercise-1 exercise",
	},
	"lebl_scv": {"scv.tex#defn-1 definition", "scv.tex#thm:hartogs statement", "scv.tex#exbox-1 exercise"},
	"napkin": {
		"tex/H113/sylow.tex#definition-1 definition", "tex/H113/sylow.tex#example-1 example", "tex/H113/sylow.tex#thm:sylow statement",
		"tex/H113/sylow.tex#proof-1 proof", "tex/H113/sylow.tex#lem:orbit-count statement", "tex/H113/sylow.tex#ques-1 exercise",
		"tex/H113/sylow.tex#dproblem-1 exercise",
	},
	"levin_dmoi": {
		"ptx/sec_intro-sets.ptx#def_subset definition", "ptx/sec_intro-sets.ptx#thm_subset-trans statement", "ptx/sec_intro-sets.ptx#proof-1 proof",
		"ptx/sec_intro-sets.ptx#example-1 example", "ptx/sec_intro-sets.ptx#exercise-1 exercise",
	},
	"applied_combinatorics": {
		"source/ch_induction.ptx#thm_strong-ind statement", "source/ch_induction.ptx#proof-1 proof",
		"source/ch_induction.ptx#ex_sum-odd example", "source/ch_induction.ptx#exercise-1 exercise",
	},
	"ernst_ibl": {
		"MAAPressVersion/Induction.tex#fieldaxioms-1 statement", "MAAPressVersion/Induction.tex#def:natural definition",
		"MAAPressVersion/Induction.tex#thm:PMI statement", "MAAPressVersion/Induction.tex#skeleton-1 proof",
		"MAAPressVersion/Induction.tex#problem-1 exercise", "MAAPressVersion/Induction.tex#question-1 exercise",
	},
	"austin_ula": {
		"src/section1-1.xml#exploration-lines exercise", "src/section1-1.xml#definition-linear-system definition",
		"src/section1-1.xml#proposition-solutions statement", "src/section1-1.xml#activity-planes exercise",
		"index/1.1/linear-system concept",
	},
	"active_calculus": {
		"source/previews/PA-1-1.xml#PA-1-1 exercise", "source/sec-1-1-vel.xml#def-avg-vel definition",
		"source/activities/act-1-1-1.xml#act-1-1-1 exercise", "source/exercises/ez-1-1.xml#ez-1-1-1 exercise",
	},
}

func TestRead(t *testing.T) {
	if len(wantIDs) != len(textbook.Books) {
		t.Errorf("the test has %d books, and Books has %d", len(wantIDs), len(textbook.Books))
	}
	for _, b := range textbook.Books {
		recs := read(t, b)
		var got []string
		for _, r := range recs {
			got = append(got, r.ID+" "+r.Kind)
			if r.Meta["environment"] == textbook.IndexEnv {
				continue // TestIndex checks the terms of the index
			}
			file, line, _ := strings.Cut(r.Locator, ":")
			if !strings.HasPrefix(r.ID, file+"#") || r.URL != b.Repository+"/blob/"+b.Commit+"/"+file+"#L"+line {
				t.Errorf("%s %s is at %s, %s", b.ID, r.ID, r.Locator, r.URL)
			}
			if r.Text == "" || r.Meta["environment"] == "" {
				t.Errorf("%s %s has the text %q and the meta %v", b.ID, r.ID, r.Text, r.Meta)
			}
			if strings.Contains(r.Text, "not read") || strings.Contains(r.Text, "not an item") || strings.Contains(r.Text, "not part of the text") || strings.Contains(r.Text, "not include") {
				t.Errorf("%s %s has the text %q", b.ID, r.ID, r.Text)
			}
		}
		if want := wantIDs[b.ID]; !slices.Equal(got, want) {
			t.Errorf("the records of %s are\n%s\nwant\n%s", b.ID, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}

func TestReadDetails(t *testing.T) {
	type want struct {
		title, locator, text string
		meta                 map[string]string
	}
	const ext = "content/first-order-logic/syntax-and-semantics/extensionality.tex"
	for _, tc := range []struct {
		book, id string
		want     want
	}{
		{"open_logic", ext + "#prop:extensionality", want{"Extensionality", ext + ":21",
			"If $M_1$ and $M_2$ agree on the symbols of a sentence $A$\n(\\olref{defn:agree}), then $A$ is true in $M_1$ iff it is true in $M_2$.",
			map[string]string{"environment": "prop", "label": "prop:extensionality", "section": "Extensionality", "uses": ext + "#defn:agree"}}},
		{"open_logic", ext + "#proof-1", want{"", ext + ":27",
			"By induction on the formation of $A$, with \\olref[fol][syn][ext]{prop:extensionality}\nfor the subformulas. The cost is 100\\% routine.",
			map[string]string{"environment": "proof", "section": "Extensionality", "proves": ext + "#prop:extensionality"}}},
		{"open_logic", ext + "#probtag-1", want{"", ext + ":36", "Write out the step for $\\forall$ in the proof of\n\\olref{prop:extensionality}.",
			map[string]string{"environment": "probtag", "section": "Extensionality", "uses": ext + "#prop:extensionality"}}},
		{"hott_book", "basics.tex#ex:homotopy-sym", want{"", "basics.tex:28",
			"Show that a homotopy from $f$ to $g$ gives one from $g$ to $f$, and use\n\\cref{lem:homotopy-refl,defn:homotopy}.",
			map[string]string{"environment": "ex", "label": "ex:homotopy-sym", "section": "Exercises", "uses": "basics.tex#lem:homotopy-refl|basics.tex#defn:homotopy"}}},
		{"lebl_ra", "ch-der.tex#proof-2", want{"", "ch-der.tex:25", "The function has an extreme value inside the interval, and\n\\lemmaref{lemma:relmaxdzero} applies there:\n\\begin{equation}\n\\label{eq:rolle-zero}\nf'(c) = 0 .\n\\end{equation}",
			map[string]string{"environment": "proof", "section": "Mean value theorem", "proves": "ch-der.tex#thm:rolle", "uses": "ch-der.tex#lemma:relmaxdzero"}}},
		{"lebl_ra", "ch-der.tex#exercise-2", want{"", "ch-der.tex:45", "Prove \\hyperref[thm:rolle]{Rolle's theorem} again with a different extreme value.",
			map[string]string{"environment": "exercise", "section": "Exercises", "uses": "ch-der.tex#thm:rolle"}}},
		{"lebl_diffyqs", "ch-first-order-ode.tex#exercise-1", want{"easy", "ch-first-order-ode.tex:15", "Solve $y' = 2x$ with $y(0) = 3$.",
			map[string]string{"environment": "exercise", "section": "Integrals as solutions"}}},
		{"napkin", "tex/H113/sylow.tex#proof-1", want{"", "tex/H113/sylow.tex:20",
			"Let the group act on the subsets of size $p^n$, and count the orbits.\n\t\\begin{lemma}\n\t\t\\label{lem:orbit-count}\n\t\tSome orbit has a size that $p$ does not divide.\n\t\\end{lemma}\n\tThen \\Cref{lem:orbit-count} gives a stabilizer of order $p^n$.",
			map[string]string{"environment": "proof", "section": "Statement", "proves": "tex/H113/sylow.tex#thm:sylow", "uses": "tex/H113/sylow.tex#lem:orbit-count"}}},
		{"napkin", "tex/H113/sylow.tex#lem:orbit-count", want{"", "tex/H113/sylow.tex:22", "Some orbit has a size that $p$ does not divide.",
			map[string]string{"environment": "lemma", "label": "lem:orbit-count", "section": "Statement"}}},
		{"ernst_ibl", "MAAPressVersion/Induction.tex#thm:PMI", want{"Principle of Mathematical Induction", "MAAPressVersion/Induction.tex:12",
			"If $1$ is in a set $S$ of natural numbers, and $k+1$ is in $S$ whenever $k$\nis, then $S$ holds every natural number.",
			map[string]string{"environment": "theorem", "label": "thm:PMI", "section": "Induction"}}},
		{"levin_dmoi", "ptx/sec_intro-sets.ptx#def_subset", want{"Subset", "ptx/sec_intro-sets.ptx:4",
			"A set $A$ is a subset of a set $B$ when each element of $A$ is an element of $B$.",
			map[string]string{"environment": "definition", "label": "def_subset", "section": "Sets"}}},
		{"levin_dmoi", "ptx/sec_intro-sets.ptx#proof-1", want{"", "ptx/sec_intro-sets.ptx:17", "Follow one element of $A$ through $B$ to $C$.",
			map[string]string{"environment": "proof", "section": "Sets", "proves": "ptx/sec_intro-sets.ptx#thm_subset-trans"}}},
		{"levin_dmoi", "ptx/sec_intro-sets.ptx#exercise-1", want{"", "ptx/sec_intro-sets.ptx:26", "List the subsets of $\\{a,b\\}$.",
			map[string]string{"environment": "exercise", "section": "Sets"}}},
		{"applied_combinatorics", "source/ch_induction.ptx#ex_sum-odd", want{"", "source/ch_induction.ptx:18",
			"The sum of the first $n$ odd numbers is $n^2$, by [thm_strong-ind].\n\n$$1 + 3 + \\cdots + (2n-1) = n^2$$",
			map[string]string{"environment": "example", "label": "ex_sum-odd", "section": "The principle of induction", "uses": "source/ch_induction.ptx#thm_strong-ind"}}},
		{"applied_combinatorics", "source/ch_induction.ptx#exercise-1", want{"", "source/ch_induction.ptx:26", "Prove that $2^n \\ge n+1$ for each positive integer $n$.",
			map[string]string{"environment": "exercise", "section": "Induction"}}},
		{"austin_ula", "src/section1-1.xml#exploration-lines", want{"", "src/section1-1.xml:4",
			"Two lines in the plane meet in a point, in no point, or in a whole line.\n\nDraw two lines that meet in one point.\n\nDraw two lines that do not meet.",
			map[string]string{"environment": "exploration", "label": "exploration-lines", "section": "What can we expect?"}}},
		{"austin_ula", "src/section1-1.xml#definition-linear-system", want{"", "src/section1-1.xml:9",
			"A linear system is a set of linear equations in the same unknowns.",
			map[string]string{"environment": "definition", "label": "definition-linear-system", "section": "What can we expect?"}}},
		{"active_calculus", "source/activities/act-1-1-1.xml#act-1-1-1", want{"", "source/activities/act-1-1-1.xml:1", "Use [def-avg-vel] on a table of heights.",
			map[string]string{"environment": "activity", "label": "act-1-1-1", "section": "Position and average velocity", "uses": "source/sec-1-1-vel.xml#def-avg-vel"}}},
	} {
		recs := read(t, book(t, tc.book))
		i := slices.IndexFunc(recs, func(r ingest.Record) bool { return r.ID == tc.id })
		if i < 0 {
			t.Errorf("%s has no record %s", tc.book, tc.id)
			continue
		}
		r := recs[i]
		if got := (want{r.Title, r.Locator, r.Text, r.Meta}); got.title != tc.want.title || got.locator != tc.want.locator || got.text != tc.want.text || !mapsEqual(got.meta, tc.want.meta) {
			t.Errorf("%s %s is\n%#v, want\n%#v", tc.book, tc.id, got, tc.want)
		}
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestReadErrors(t *testing.T) {
	b := book(t, "levin_dmoi")
	b.Main = "ptx/none.ptx"
	tr, _ := gzip.NewReader(bytes.NewReader(archive(t, b, b.Commit)))
	if err := b.Read(tr, func(ingest.Record) error { return nil }); err == nil || !strings.Contains(err.Error(), "ptx/none.ptx") {
		t.Errorf("Read of a book with no main file gives %v", err)
	}
	b = book(t, "lebl_scv")
	b.Files = textbook.Books[0].Files // no file of scv is in content/
	tr, _ = gzip.NewReader(bytes.NewReader(archive(t, b, b.Commit)))
	if err := b.Read(tr, func(ingest.Record) error { return nil }); err == nil {
		t.Error("Read of an archive with no items gives no error")
	}
	b = book(t, "lebl_scv")
	stop := io.ErrShortWrite
	tr, _ = gzip.NewReader(bytes.NewReader(archive(t, b, b.Commit)))
	if err := b.Read(tr, func(ingest.Record) error { return stop }); err != stop {
		t.Errorf("Read gives %v, want the error of emit", err)
	}
}

func TestFetch(t *testing.T) {
	b := book(t, "hott_book")
	for _, tc := range []struct {
		commit string
		ok     bool
	}{{b.Commit, true}, {"0123456789abcdef0123456789abcdef01234567", false}} {
		data := archive(t, b, tc.commit)
		var out bytes.Buffer
		req := &ingest.Request{URL: b.Dump(), Get: func(context.Context, string) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }}
		err := b.Fetch(context.Background(), req, &out)
		if (err == nil) != tc.ok {
			t.Errorf("Fetch of an archive at %s gives %v", tc.commit, err)
		}
		if tc.ok && (req.Commit != b.Commit || out.Len() == 0) {
			t.Errorf("Fetch gives the commit %s and %d bytes", req.Commit, out.Len())
		}
	}
}

func TestPull(t *testing.T) {
	b := book(t, "lebl_ra")
	tgz := archive(t, b, b.Commit)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tgz) }))
	defer srv.Close()
	root := t.TempDir()
	res, err := ingest.Pull(context.Background(), b, ingest.Options{Root: root, Cache: t.TempDir(), URL: srv.URL + "/archive.tar.gz", Date: "2026-10-27"})
	if err != nil {
		t.Fatal(err)
	}
	if m := res.Manifest; m.Records != 8 || m.Classes[ingest.Open] != 8 || m.Adapter != "textbook/1" {
		t.Errorf("the manifest is %+v, want 8 open records from textbook/1", m)
	}
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		t.Fatal(err)
	}
	src := cat.Source("lebl_ra")
	if src == nil || len(src.Snapshots) != 1 || src.Snapshots[0].Commit != b.Commit {
		t.Fatalf("the catalogue entry is %+v, want one snapshot at %s", src, b.Commit)
	}
	recs, _, err := ingest.ReadRecords(root, "lebl_ra")
	if err != nil {
		t.Fatal(err)
	}
	ms := inventory.FromRecords(src, src.Snapshots[0].Hash, "textbook/1", recs)
	kinds := map[string]int{}
	for _, m := range ms {
		kinds[m.Kind]++
		if m.Text != "" {
			t.Errorf("the mention %s keeps share-alike text", m.ID)
		}
	}
	if kinds["statement"] != 2 || kinds["proof"] != 2 || kinds["definition"] != 1 || kinds["example"] != 1 || kinds["exercise"] != 2 {
		t.Errorf("the kinds of the mentions are %v", kinds)
	}
}

func TestRecords(t *testing.T) {
	for _, b := range textbook.Books {
		var rs []cluster.Record
		if err := b.Records(nil, func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
			t.Fatal(err)
		}
		if len(rs) != 1 || rs[0].NativeID != b.ID || rs[0].Kind != "textbook" || rs[0].Title == "" || len(rs[0].Creators) == 0 {
			t.Errorf("%s: records = %+v", b.ID, rs)
		}
		if r := rs[0]; r.Year() == 0 || r.Language != "en" || len(r.MSC) == 0 || slices.ContainsFunc(r.MSC, func(c string) bool { return !msc.Valid(c) }) {
			t.Errorf("%s: date %q, language %q, msc %v", b.ID, r.Date, r.Language, r.MSC)
		}
	}
}
