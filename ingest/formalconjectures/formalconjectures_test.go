package formalconjectures_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/formalconjectures"
)

const commit = "8b1d6c7a0f3e4d2c9b5a1e0f7d6c5b4a3e2d1c0b"

// archive makes a git archive like the one GitHub gives, with the files in one directory.
func archive(t *testing.T, withCommit bool, files map[string]string) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if withCommit {
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": commit}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: "formal-conjectures-main/", Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "formal-conjectures-main/" + name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write([]byte(body))
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = zw.Close()
	return buf.Bytes()
}

// fixtures gives the Lean files of testdata at their paths in the repository.
func fixtures(t *testing.T) map[string]string {
	files := map[string]string{"README.md": "# Formal Conjectures\n", "FormalConjecturesUtil.lean": "@[category research open]\ntheorem outside : True := trivial\n"}
	for _, name := range []string{"ErdosProblems/1.lean", "Wikipedia/AmicableNumbers.lean"} {
		b, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		files[formalconjectures.Dir+name] = string(b)
	}
	return files
}

func TestPull(t *testing.T) {
	tgz := archive(t, true, fixtures(t))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tgz) }))
	defer srv.Close()

	root := t.TempDir()
	res, err := ingest.Pull(context.Background(), formalconjectures.Adapter{}, ingest.Options{Root: root, Cache: t.TempDir(), URL: srv.URL + "/main.tar.gz", Date: "2026-10-26"})
	if err != nil {
		t.Fatal(err)
	}
	if m := res.Manifest; m.Records != 12 || m.Classes[ingest.Open] != 12 || m.Adapter != "formal_conjectures/1" {
		t.Errorf("the manifest is %+v, want 12 open records from formal_conjectures/1", m)
	}
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		t.Fatal(err)
	}
	if s := cat.Source("formal_conjectures"); s == nil || len(s.Snapshots) != 1 || s.Snapshots[0].Commit != commit {
		t.Errorf("the catalogue entry is %+v, want one snapshot at %s", s, commit)
	}
	recs, _, err := ingest.ReadRecords(root, "formal_conjectures")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]ingest.Record{}
	for _, r := range recs {
		byID[r.ID] = r
		if r.Kind != "statement" || r.Text == "" || r.Meta["statement"] == "" || !strings.HasPrefix(r.URL, formalconjectures.Repository+"/blob/"+commit+"/FormalConjectures/") {
			t.Errorf("the record is %+v", r)
		}
	}

	one := byID["Erdos1.erdos_1"]
	want := map[string]string{
		"category":     "research solved",
		"ams":          "5 11",
		"formal_proof": "https://github.com/tadamcz/erdos1/blob/0e395153306f34b3829d118b85bdd704136f2843/Erdos1/Resolutions/Erdos1_219usd_38h.lean#L2419",
		"statement":    ": ¬ ∃ C > (0 : ℝ), ∀ (N : ℕ) (A : Finset ℕ) (_ : IsSumDistinctSet A N), N ≠ 0 → C * 2 ^ A.card < N",
		"module":       "FormalConjectures.ErdosProblems.1",
		"title":        "Erdős Problem 1",
		"references":   "https://www.erdosproblems.com/1",
		"erdos":        "1",
		"posed_by":     "Paul Erdős",
	}
	for k, v := range want {
		if one.Meta[k] != v {
			t.Errorf("the %s of Erdos1.erdos_1 is %q, want %q", k, one.Meta[k], v)
		}
	}
	if len(one.Meta) != len(want) {
		t.Errorf("the meta of Erdos1.erdos_1 is %v, want only the keys %v", one.Meta, want)
	}
	if one.Title != "Erdős Problem 1" || one.URL != formalconjectures.Repository+"/blob/"+commit+"/FormalConjectures/ErdosProblems/1.lean#L53" || !strings.HasPrefix(one.Text, "If $A\\subseteq\\{1, ..., N\\}$") {
		t.Errorf("Erdos1.erdos_1 is %+v", one)
	}
	if r := byID["Erdos1.erdos_1.variants.weaker"]; r.Meta["category"] != "textbook" || r.Meta["formal_proof"] != "" {
		t.Errorf("Erdos1.erdos_1.variants.weaker is %+v", r)
	}

	am := byID["AmicableNumbers.relatively_prime_amicable"]
	for k, v := range map[string]string{
		"category":  "research open",
		"question":  "yes",
		"oeis":      "A063990",
		"wikipedia": "Amicable_numbers",
		"title":     "Amicable numbers",
	} {
		if am.Meta[k] != v {
			t.Errorf("the %s of AmicableNumbers.relatively_prime_amicable is %q, want %q", k, am.Meta[k], v)
		}
	}
	if am.Meta["posed_by"] != "" || am.Meta["erdos"] != "" {
		t.Errorf("AmicableNumbers.relatively_prime_amicable is %+v", am)
	}
	for _, id := range []string{"AmicableNumbers.amicable_220_284", "AmicableNumbers.IsAmicable.symm", "outside"} {
		if _, ok := byID[id]; ok {
			t.Errorf("the records have %s, which is a test or is not under %s", id, formalconjectures.Dir)
		}
	}
}

func TestRead(t *testing.T) {
	none := func(ingest.Record) error { return nil }
	stmt := "/-- A. -/\n@[category research open]\ntheorem a : True := trivial\n"
	for name, tgz := range map[string][]byte{
		"no commit":     archive(t, false, map[string]string{formalconjectures.Dir + "A.lean": stmt}),
		"no statements": archive(t, true, map[string]string{formalconjectures.Dir + "A.lean": "theorem a : True := trivial\n"}),
		"two names":     archive(t, true, map[string]string{formalconjectures.Dir + "A.lean": stmt, formalconjectures.Dir + "B.lean": stmt}),
	} {
		zr, err := gzip.NewReader(bytes.NewReader(tgz))
		if err != nil {
			t.Fatal(err)
		}
		if err := (formalconjectures.Adapter{}).Read(zr, none); err == nil {
			t.Errorf("Read of an archive with %s gives no error", name)
		}
	}
	if err := (formalconjectures.Adapter{}).Read(strings.NewReader("not a tar file"), none); err == nil {
		t.Error("Read of bytes that are not a tar file gives no error")
	}
}

func TestSource(t *testing.T) {
	s := formalconjectures.Adapter{}.Source()
	if p := s.Problems(); len(p) > 0 {
		t.Errorf("the catalogue entry has problems: %v", p)
	}
	if a, ok := ingest.Lookup("formal_conjectures"); !ok || a.Version() != "formal_conjectures/1" {
		t.Error("the adapter is not registered")
	}
}
