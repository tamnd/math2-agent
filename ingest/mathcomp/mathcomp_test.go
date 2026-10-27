package mathcomp_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/mathcomp"
)

// archive makes a git archive of the files, like the one GitHub gives, at the commit.
func archive(t *testing.T, commit string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": commit}}); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		h := &tar.Header{Name: "math-comp-" + commit + "/" + name, Mode: 0o644, Size: int64(len(body)), ModTime: time.Unix(0, 0)}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fixtures gives the files under testdata and the extra files.
func fixtures(t *testing.T, extra map[string]string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := os.ReadFile(p) // #nosec G304 -- a fixture of the test.
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("testdata", p)
		files[filepath.ToSlash(rel)] = string(body)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	maps.Copy(files, extra)
	return files
}

// unzip gives the tar archive in a git archive, which Import unzips for Read.
func unzip(t *testing.T, body []byte) io.Reader {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return zr
}

func read(t *testing.T, body []byte) map[string]ingest.Record {
	t.Helper()
	a := mathcomp.Adapter{}
	src := a.Source()
	recs, err := ingest.Import(a, &src, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]ingest.Record{}
	for _, r := range recs {
		out[r.ID] = r
	}
	return out
}

func TestRead(t *testing.T) {
	recs := read(t, archive(t, mathcomp.Commit, fixtures(t, map[string]string{"boot/again.v": "Lemma twice : True. Proof. by []. Qed.\nLemma twice : False. Proof. Admitted.\n"})))
	const nat, alg = "mathcomp.boot.toynat.", "mathcomp.algebra.hierarchy.toyalg."
	want := map[string][2]string{
		nat + "double":                {"definition", "Definition"},
		nat + "half":                  {"definition", "Fixpoint"},
		nat + "double0":               {"statement", "Lemma"},
		nat + "doubleS":               {"statement", "Lemma"},
		nat + "hidden_twice":          {"statement", "Lemma"},
		nat + "Toy.Inner.half_double": {"statement", "Theorem"},
		nat + "Toy.half0":             {"statement", "Corollary"},
		nat + "double_fact":           {"statement", "Fact"},
		alg + "HasToyOp":              {"definition", "HB.mixin Record"},
		alg + "ToyType":               {"definition", "HB.structure Definition"},
		alg + "ToyFromNat":            {"definition", "HB.factory Record"},
		alg + "locked_op":             {"definition", "HB.lock Definition"},
		alg + "toy_spec":              {"definition", "Variant"},
		alg + "tree":                  {"definition", "Inductive"},
		alg + "point":                 {"definition", "Record"},
		alg + "pack":                  {"definition", "Structure"},
		alg + "toy_prop":              {"statement", "Proposition"},
		alg + "toy_remark":            {"statement", "Remark"},
		"mathcomp.boot.again.twice":   {"statement", "Lemma"},
	}
	for id, w := range want {
		r, ok := recs[id]
		if !ok {
			t.Errorf("no record %s", id)
			continue
		}
		if r.Kind != w[0] || r.Meta["keyword"] != w[1] || r.Class != ingest.RecordOnly || r.Text != "" {
			t.Errorf("the record %s is %+v", id, r)
		}
	}
	if len(recs) != len(want) {
		var ids []string
		for id := range recs {
			if _, ok := want[id]; !ok {
				ids = append(ids, id)
			}
		}
		t.Errorf("the archive gives %d records, want %d. The others are %v", len(recs), len(want), ids)
	}
	if got := recs["mathcomp.boot.again.twice"].Locator; got != "boot/again.v:1" {
		t.Errorf("a name given twice has the place %s", got)
	}
	d := recs[nat+"doubleS"]
	if d.Title != "doubleS" || d.Locator != "boot/toynat.v:17" || d.URL != mathcomp.Repository+"/blob/"+mathcomp.Commit+"/boot/toynat.v#L17" {
		t.Errorf("the record of doubleS is %+v", d)
	}
	if d.Meta["section"] != "Doubling" || d.Meta["file"] != "mathcomp.boot.toynat" {
		t.Errorf("the meta of doubleS is %v", d.Meta)
	}
	if _, ok := recs[nat+"Toy.Inner.half_double"].Meta["section"]; ok {
		t.Errorf("a declaration in modules only has a section")
	}
}

func TestReadErrors(t *testing.T) {
	cases := map[string][]byte{
		"open comment": archive(t, mathcomp.Commit, fixtures(t, map[string]string{"boot/bad.v": "(* no end\nLemma x : True.\n"})),
		"open string":  archive(t, mathcomp.Commit, fixtures(t, map[string]string{"boot/bad.v": "Notation \"x := x.\n"})),
		"no commit":    archive(t, "", fixtures(t, nil)),
	}
	for name, body := range cases {
		err := (mathcomp.Adapter{}).Read(unzip(t, body), func(ingest.Record) error { return nil })
		if err == nil {
			t.Errorf("%s: Read gives no error", name)
		} else {
			t.Logf("%s: %v", name, err)
		}
	}
	empty := archive(t, mathcomp.Commit, map[string]string{"test_suite/only.v": "Lemma x : True. Proof. by []. Qed.\n"})
	if err := (mathcomp.Adapter{}).Read(unzip(t, empty), func(ingest.Record) error { return nil }); err == nil || !strings.Contains(err.Error(), "no declarations") {
		t.Errorf("an empty archive gives %v", err)
	}
}

func TestPull(t *testing.T) {
	body := archive(t, mathcomp.Commit, fixtures(t, nil))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer srv.Close()
	res, err := ingest.Pull(context.Background(), mathcomp.Adapter{}, ingest.Options{Root: t.TempDir(), Cache: t.TempDir(), URL: srv.URL + "/a.tar.gz", Date: "2026-10-27"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Snapshot.Commit != mathcomp.Commit || res.Manifest.Records != 18 || res.Manifest.Classes[ingest.RecordOnly] != 18 {
		t.Errorf("the pull gives the commit %s and the manifest %+v", res.Snapshot.Commit, res.Manifest)
	}
	other := archive(t, strings.Repeat("a", 40), fixtures(t, nil))
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(other) }))
	defer srv2.Close()
	if _, err := ingest.Pull(context.Background(), mathcomp.Adapter{}, ingest.Options{Root: t.TempDir(), Cache: t.TempDir(), URL: srv2.URL + "/a.tar.gz", Date: "2026-10-27"}); err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Errorf("a pull of another commit gives %v", err)
	}
	if got := (mathcomp.Adapter{}).Dump(); got != mathcomp.Repository+"/archive/"+mathcomp.Commit+".tar.gz" {
		t.Errorf("Dump gives %s", got)
	}
}
