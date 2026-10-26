package curricula_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/curricula"
	"github.com/tamnd/math2-agent/inventory"
)

// The commits of the snapshots that the fixtures come from.
const (
	mathlibCommit  = "2b2e96747ca44b376def99bf9571ef72cc6a7f2d"
	thousandCommit = "8e04b97dd24adc6e931be78a884da7e935bc8780"
)

func list(t *testing.T, id string) curricula.List {
	t.Helper()
	for _, l := range curricula.Lists {
		if l.ID == id {
			return l
		}
	}
	t.Fatalf("no list %s", id)
	return curricula.List{}
}

// read imports the fixture of the list and returns its records by id.
func read(t *testing.T, a ingest.Adapter, file string) map[string]ingest.Record {
	t.Helper()
	f, err := os.Open(file) // #nosec G304 -- a fixture of the test.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	src := a.Source()
	recs, err := ingest.Import(a, &src, f)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]ingest.Record{}
	for _, r := range recs {
		out[r.ID] = r
	}
	if len(out) != len(recs) {
		t.Errorf("%d records have %d ids", len(recs), len(out))
	}
	return out
}

func TestUndergrad(t *testing.T) {
	recs := read(t, list(t, "mathlib_undergrad"), "testdata/mathlib/undergrad.yaml")
	if len(recs) != 25 {
		t.Errorf("the fixture gives %d records, want 25", len(recs))
	}
	vs := recs["Linear algebra > Fundamentals > vector space"]
	if vs.Title != "vector space" || vs.Locator != "docs/undergrad.yaml:7" || vs.URL != curricula.Site+"undergrad.html" || vs.Class != ingest.Open {
		t.Errorf("the record of vector space is %+v", vs)
	}
	if vs.Meta["mathlib"] != "Module" || vs.Meta["section"] != "Linear algebra > Fundamentals" || vs.Meta["order"] != "1" {
		t.Errorf("the meta of vector space is %v", vs.Meta)
	}
	cases := map[string]map[string]string{
		"Single Variable Real Analysis > Convexity > continuity and differentiability of convex functions > continuity":        {"mathlib": "ConvexOn.continuousOn"},
		"Single Variable Real Analysis > Convexity > continuity and differentiability of convex functions > differentiability": {"link": "https://en.wikipedia.org/wiki/Convex_function#Functions_of_one_variable"},
		"Single Variable Real Analysis > Convexity > convexity inequalities":                                                   {"link": "https://leanprover-community.github.io/mathlib4_docs/Mathlib/Analysis/MeanInequalities.html"},
		"Single Variable Complex Analysis > Complex-valued series > antiderivative":                                            {},
	}
	for id, want := range cases {
		r, ok := recs[id]
		if !ok {
			t.Errorf("no record %q", id)
			continue
		}
		for _, k := range []string{"mathlib", "link"} {
			if r.Meta[k] != want[k] {
				t.Errorf("%s: the meta %s is %q, want %q", id, k, r.Meta[k], want[k])
			}
		}
	}
	if r := recs["Single Variable Complex Analysis > Complex-valued series > antiderivative"]; r.Meta["order"] != "25" {
		t.Errorf("the last item has the order %q", r.Meta["order"])
	}
}

func TestOverview(t *testing.T) {
	recs := read(t, list(t, "mathlib_overview"), "testdata/mathlib/overview.yaml")
	if r := recs["General algebra > Category theory > functor"]; r.Meta["mathlib"] != "CategoryTheory.Functor" {
		t.Errorf("the record of functor is %+v", r)
	}
}

func TestHundred(t *testing.T) {
	recs := read(t, list(t, "mathlib_100"), "testdata/mathlib/100.yaml")
	if len(recs) != 8 {
		t.Errorf("the fixture gives %d records, want 8", len(recs))
	}
	r := recs["7"]
	if r.Title != "Law of Quadratic Reciprocity" || r.Kind != "statement" || r.URL != curricula.Site+"100.html#7" {
		t.Errorf("the record 7 is %+v", r)
	}
	if r.Meta["mathlib"] != "legendreSym.quadratic_reciprocity|jacobiSym.quadratic_reciprocity" || r.Meta["status"] != "formalized" || r.Meta["wikidata"] != "" {
		t.Errorf("the meta of 7 is %v", r.Meta)
	}
	if r := recs["5"]; r.Meta["mathlib"] != "" || r.Meta["status"] != "" || strings.Count(r.Meta["links"], "|") != 2 {
		t.Errorf("the meta of 5 is %v", r.Meta)
	}
}

func TestThousandList(t *testing.T) {
	recs := read(t, list(t, "mathlib_1000"), "testdata/mathlib/1000.yaml")
	if r := recs["Q180345X"]; r.Meta["wikidata"] != "Q180345" || r.Meta["mathlib"] != "isInteger_of_is_root_of_monic" || r.URL != curricula.Site+"1000.html#Q180345X" {
		t.Errorf("the record Q180345X is %+v", r)
	}
	if r := recs["Q132469"]; r.Meta["status"] != "statement" || r.Meta["mathlib"] != "FermatLastTheorem" || !strings.HasPrefix(r.Meta["note"], "Formalisation of the proof") {
		t.Errorf("the record Q132469 is %+v", r)
	}
	if r := recs["Q132427"]; r.Meta["date"] != "2023" || r.Meta["authors"] != "Sébastien Gouëzel" {
		t.Errorf("the record Q132427 is %+v", r)
	}
	if r := recs["Q12524"]; r.Meta["status"] != "" || r.Meta["mathlib"] != "" || r.Meta["wikidata"] != "Q12524" {
		t.Errorf("the record Q12524 is %+v", r)
	}
}

func TestListErrors(t *testing.T) {
	none := func(ingest.Record) error { return nil }
	tree, items := list(t, "mathlib_undergrad"), list(t, "mathlib_1000")
	cases := []struct {
		name string
		l    curricula.List
		in   string
	}{
		{"empty", tree, ""},
		{"not a mapping", tree, "- a\n- b\n"},
		{"no items", tree, "Algebra: {}\n"},
		{"a list as an item", tree, "Algebra:\n  groups: [Group, Monoid]\n"},
		{"a value with spaces", tree, "Algebra:\n  groups: 'see the group page'\n"},
		{"two items with one id", tree, "Algebra:\n  groups: Group\n  groups : Monoid\n"},
		{"a bad key", items, "Q01:\n  title: x\n"},
		{"no title", items, "Q1:\n  decl: x\n"},
		{"two keys", items, "Q1:\n  title: x\nQ1:\n  title: y\n"},
	}
	for _, c := range cases {
		if err := c.l.Read(strings.NewReader(c.in), none); err == nil {
			t.Errorf("%s: Read gives no error", c.name)
		}
	}
}

func TestPullPinned(t *testing.T) {
	body, err := os.ReadFile("testdata/mathlib/undergrad.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	root := t.TempDir()
	url := srv.URL + "/leanprover-community/mathlib4/" + mathlibCommit + "/docs/undergrad.yaml"
	res, err := ingest.Pull(context.Background(), list(t, "mathlib_undergrad"), ingest.Options{Root: root, Cache: t.TempDir(), URL: url, Date: "2026-10-27"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Snapshot.Commit != mathlibCommit || !strings.Contains(path, mathlibCommit) {
		t.Errorf("the snapshot has the commit %q from the path %s", res.Snapshot.Commit, path)
	}
	if m := res.Manifest; m.Records != 25 || m.Classes[ingest.Open] != 25 {
		t.Errorf("the manifest is %+v", m)
	}
	if got := (curricula.List{File: "docs/100.yaml"}).Dump(); got != curricula.Raw+"master/docs/100.yaml" {
		t.Errorf("Dump gives %s", got)
	}
}

// archive makes a git archive of the files under dir, like the one GitHub gives.
func archive(t *testing.T, dir string, extra map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": thousandCommit}}); err != nil {
		t.Fatal(err)
	}
	add := func(name string, body []byte) {
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "1000-plus.github.io-main/" + name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write(body)
	}
	if dir != "" {
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := os.ReadFile(p) // #nosec G304 -- a fixture of the test.
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(dir, p)
			add(filepath.ToSlash(rel), b)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range extra {
		add(name, []byte(body))
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = zw.Close()
	return buf.Bytes()
}

func TestThousand(t *testing.T) {
	tgz := archive(t, "testdata/thousand_plus", nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tgz) }))
	defer srv.Close()
	root := t.TempDir()
	res, err := ingest.Pull(context.Background(), curricula.Thousand{}, ingest.Options{Root: root, Cache: t.TempDir(), URL: srv.URL + "/main.tar.gz", Date: "2026-10-27"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Snapshot.Commit != thousandCommit || res.Manifest.Records != 7 || res.Manifest.Classes[ingest.Open] != 7 {
		t.Errorf("the snapshot is %+v and the manifest %+v", res.Snapshot, res.Manifest)
	}
	all, _, err := ingest.ReadRecords(root, "thousand_plus")
	if err != nil {
		t.Fatal(err)
	}
	recs := map[string]ingest.Record{}
	for _, r := range all {
		recs[r.ID] = r
	}
	r := recs["Q1033910"]
	if r.Title != "Cantor–Bernstein–Schroeder theorem" || r.Kind != "statement" || r.Locator != "_thm/Q1033910.md" || r.URL != curricula.ThousandHome+"/all?length=-1#Q1033910" {
		t.Errorf("the record Q1033910 is %+v", r)
	}
	want := map[string]string{"wikidata": "Q1033910", "msc": "03-XX", "wikipedia": "Cantor–Bernstein–Schroeder theorem", "formalized": "hol_light|isabelle|lean|metamath|mizar|rocq", "mathlib": "Function.Embedding.schroeder_bernstein"}
	for k, v := range want {
		if r.Meta[k] != v {
			t.Errorf("Q1033910: the meta %s is %q, want %q", k, r.Meta[k], v)
		}
	}
	if r := recs["Q132469"]; r.Meta["formalized"] != "" || r.Meta["stated"] != "lean" || r.Meta["mathlib"] != "FermatLastTheorem" {
		t.Errorf("the record Q132469 is %+v", r)
	}
	if r := recs["Q180345X"]; r.Meta["wikidata"] != "Q180345" || r.URL != curricula.ThousandHome+"/all?length=-1#Q180345" {
		t.Errorf("the record Q180345X is %+v", r)
	}
	// The second link of Lamé's theorem is to a section of the page on the Euclidean algorithm, and it is not a page of the theorem.
	if r := recs["Q10380294"]; r.Meta["wikipedia"] != "Lamé’s theorem" || r.Meta["msc"] != "68-XX" {
		t.Errorf("the record Q10380294 is %+v", r)
	}
	if r := recs["Q10369454"]; r.Meta["formalized"] != "" || r.Meta["mathlib"] != "" {
		t.Errorf("the record Q10369454 is %+v", r)
	}
}

func TestThousandErrors(t *testing.T) {
	none := func(ingest.Record) error { return nil }
	good := "---\n# A theorem\n\nwikidata: Q5\nmsc_classification: '20'\nwikipedia_links:\n- '[[A theorem]]'\n---\n"
	cases := map[string]map[string]string{
		"no entries":        {"README.md": "x"},
		"no front matter":   {"_thm/Q5.md": "# A theorem\nwikidata: Q5\n"},
		"another file name": {"_thm/Q6.md": good},
		"no title":          {"_thm/Q5.md": "---\nwikidata: Q5\n---\n"},
		"a bad status":      {"_thm/Q5.md": "---\n# A theorem\nwikidata: Q5\nlean:\n- status: done\n---\n"},
		"a bad msc":         {"_thm/Q5.md": "---\n# A theorem\nwikidata: Q5\nmsc_classification: '2'\n---\n"},
		"a bad suffix":      {"_thm/Q5x.md": "---\n# A theorem\nwikidata: Q5\nid_suffix: x\n---\n"},
	}
	for name, files := range cases {
		zr, err := gzip.NewReader(bytes.NewReader(archive(t, "", files)))
		if err != nil {
			t.Fatal(err)
		}
		if err := (curricula.Thousand{}).Read(zr, none); err == nil {
			t.Errorf("%s: Read gives no error", name)
		}
	}
	links := "---\n# A theorem\nwikidata: Q5\nwikipedia_links:\n- '[[A theorem]] (in words)'\n- 'Another_page'\n- '[[X (disambiguation)|X]]s'\n- '[[Y#Z|A theorem]]'\n---\n"
	for name, body := range map[string]string{"good": good, "links": links} {
		zr, err := gzip.NewReader(bytes.NewReader(archive(t, "", map[string]string{"_thm/Q5.md": body})))
		if err != nil {
			t.Fatal(err)
		}
		var got ingest.Record
		if err := (curricula.Thousand{}).Read(zr, func(r ingest.Record) error { got = r; return nil }); err != nil {
			t.Errorf("%s: Read gives %v", name, err)
		}
		if name == "links" && got.Meta["wikipedia"] != "A theorem|Another page" {
			t.Errorf("the links give the pages %q", got.Meta["wikipedia"])
		}
	}
}

// The mentions of a theorem list join the mention of its Mathlib declaration and the mention of its Wikidata item.
func TestMentionsJoin(t *testing.T) {
	recs := read(t, list(t, "mathlib_1000"), "testdata/mathlib/1000.yaml")
	src := list(t, "mathlib_1000").Source()
	ms := inventory.FromRecords(&src, "blake3:00", "curricula/1", []ingest.Record{recs["Q26708"]})
	ml := ingest.Source{ID: "mathlib", Licence: "Apache-2.0", Class: ingest.Open}
	ms = append(ms, inventory.FromRecords(&ml, "blake3:01", "mathlib/1", []ingest.Record{{ID: "add_pow", Kind: "theorem", Title: "add_pow", Class: ingest.Open}})...)
	day, _ := time.Parse(time.DateOnly, "2026-10-27")
	res, err := inventory.Cluster(nil, ms, inventory.Options{Day: day, By: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || len(res.Rows[0].Mentions) != 2 || res.Rows[0].Kind != "statement" {
		t.Fatalf("the rows are %+v, want one statement with the two mentions", res.Rows)
	}
	if ms[0].Same["wikidata"][0] != "Q26708" || ms[0].Same["mathlib"][0] != "add_pow" {
		t.Errorf("the mention of the list has the ids %v", ms[0].Same)
	}
}
