package oeis_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/oeis"
)

func TestPull(t *testing.T) {
	names, err := os.ReadFile("testdata/names")
	if err != nil {
		t.Fatal(err)
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write(names)
	_ = zw.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(gz.Bytes()) }))
	defer srv.Close()

	root := t.TempDir()
	res, err := ingest.Pull(context.Background(), oeis.Adapter{}, ingest.Options{Root: root, Cache: t.TempDir(), URL: srv.URL + "/names.gz", Date: "2026-10-25"})
	if err != nil {
		t.Fatal(err)
	}
	if m := res.Manifest; m.Records != 8 || m.Classes[ingest.Open] != 8 || m.Adapter != "oeis/1" {
		t.Errorf("the manifest is %+v, want 8 open records from oeis/1", m)
	}
	recs, _, err := ingest.ReadRecords(root, "oeis")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if err := crosswalk.Check("oeis", r.ID); err != nil {
			t.Error(err)
		}
		if r.Title == "" || r.Text != "" || r.URL != "https://oeis.org/"+r.ID {
			t.Errorf("the record is %+v", r)
		}
	}
	last := recs[len(recs)-1]
	if last.ID != "A000045" || !strings.HasPrefix(last.Title, "Fibonacci numbers:") || last.Kind != "sequence" {
		t.Errorf("the last record is %+v", last)
	}
}

func TestRead(t *testing.T) {
	none := func(ingest.Record) error { return nil }
	for _, bad := range []string{"# only a comment\n", "A45 Too short.\n", "A000001 Number of groups of order n.\nB000002 Not an A-number.\n"} {
		if err := (oeis.Adapter{}).Read(strings.NewReader(bad), none); err == nil {
			t.Errorf("Read(%q) gives no error", bad)
		}
	}
}

func TestSource(t *testing.T) {
	s := oeis.Adapter{}.Source()
	if p := s.Problems(); len(p) > 0 {
		t.Errorf("the catalogue entry has problems: %v", p)
	}
	if a, ok := ingest.Lookup("oeis"); !ok || a.Version() != "oeis/1" {
		t.Error("the adapter is not registered")
	}
}
