package erdosproblems_test

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
	"github.com/tamnd/math2-agent/ingest/erdosproblems"
)

const commit = "4d22d408cbafe9ecf5b2b1223e689783eaf76803"

// archive makes a git archive like the one GitHub gives, with the files in one directory.
func archive(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	hs := []*tar.Header{
		{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": commit}},
		{Typeflag: tar.TypeDir, Name: "erdosproblems-main/", Mode: 0o755},
	}
	for _, h := range hs {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "erdosproblems-main/" + name, Mode: 0o644, Size: int64(len(body))}); err != nil {
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

func TestPull(t *testing.T) {
	data, err := os.ReadFile("testdata/problems.yaml")
	if err != nil {
		t.Fatal(err)
	}
	tgz := archive(t, map[string]string{"README.md": "# Erdős problem database\n", erdosproblems.Data: string(data)})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tgz) }))
	defer srv.Close()

	root := t.TempDir()
	res, err := ingest.Pull(context.Background(), erdosproblems.Adapter{}, ingest.Options{Root: root, Cache: t.TempDir(), URL: srv.URL + "/main.tar.gz", Date: "2026-10-25"})
	if err != nil {
		t.Fatal(err)
	}
	if m := res.Manifest; m.Records != 6 || m.Classes[ingest.Open] != 6 || m.Adapter != "erdosproblems/1" {
		t.Errorf("the manifest is %+v, want 6 open records from erdosproblems/1", m)
	}
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		t.Fatal(err)
	}
	if s := cat.Source("erdosproblems"); s == nil || len(s.Snapshots) != 1 || s.Snapshots[0].Commit != commit {
		t.Errorf("the catalogue entry is %+v, want one snapshot at %s", s, commit)
	}
	recs, _, err := ingest.ReadRecords(root, "erdosproblems")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]ingest.Record{}
	for _, r := range recs {
		byID[r.ID] = r
		if r.Kind != "problem" || r.URL != "https://www.erdosproblems.com/"+r.ID || r.Meta["posed_by"] != erdosproblems.Poser || r.Text != "" {
			t.Errorf("the record is %+v", r)
		}
	}
	one := byID["1"]
	if one.Meta["status"] != "disproved (Lean)" || one.Meta["status_date"] != "2026-09-03" || one.Meta["prize"] != "$500" || one.Meta["oeis"] != "A276661" || one.Meta["tags"] != "number theory|additive combinatorics" {
		t.Errorf("problem 1 is %+v", one)
	}
	if p := byID["18"]; p.Meta["name"] != "practical numbers" || p.Meta["prize"] != "" || p.Title != "Erdős Problem #18" {
		t.Errorf("problem 18 is %+v", p)
	}
}

func TestRead(t *testing.T) {
	none := func(ingest.Record) error { return nil }
	for name, files := range map[string]map[string]string{
		"no data file":  {"README.md": "x"},
		"no problems":   {erdosproblems.Data: ""},
		"bad number":    {erdosproblems.Data: "- number: \"A1\"\n  status:\n    state: \"open\"\n"},
		"no status":     {erdosproblems.Data: "- number: \"1\"\n"},
		"not yaml list": {erdosproblems.Data: "number: 1\n"},
	} {
		zr, err := gzip.NewReader(bytes.NewReader(archive(t, files)))
		if err != nil {
			t.Fatal(err)
		}
		if err := (erdosproblems.Adapter{}).Read(zr, none); err == nil {
			t.Errorf("Read of an archive with %s gives no error", name)
		}
	}
	if err := (erdosproblems.Adapter{}).Read(strings.NewReader("not a tar file"), none); err == nil {
		t.Error("Read of bytes that are not a tar file gives no error")
	}
}

func TestSource(t *testing.T) {
	s := erdosproblems.Adapter{}.Source()
	if p := s.Problems(); len(p) > 0 {
		t.Errorf("the catalogue entry has problems: %v", p)
	}
	if a, ok := ingest.Lookup("erdosproblems"); !ok || a.Version() != "erdosproblems/1" {
		t.Error("the adapter is not registered")
	}
}
