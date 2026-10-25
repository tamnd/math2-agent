package wikidata_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/wikidata"
)

// sparql answers each query with testdata/<property>.json, and with no bindings when there is no such file.
func sparql(t *testing.T) *httptest.Server {
	prop := regexp.MustCompile(`wdt:(P[0-9]+)`)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") != "json" {
			http.Error(w, "no format", http.StatusBadRequest)
			return
		}
		m := prop.FindStringSubmatch(r.URL.Query().Get("query"))
		if m == nil {
			http.Error(w, "no property", http.StatusBadRequest)
			return
		}
		data, err := os.ReadFile("testdata/" + m[1] + ".json")
		if os.IsNotExist(err) {
			data = []byte(`{"head":{"vars":["item","label","value"]},"results":{"bindings":[]}}`)
		} else if err != nil {
			t.Error(err)
		}
		_, _ = w.Write(data)
	}))
}

func TestPull(t *testing.T) {
	srv := sparql(t)
	defer srv.Close()

	root := t.TempDir()
	opt := ingest.Options{Root: root, Cache: t.TempDir(), URL: srv.URL, Date: "2026-10-25"}
	res, err := ingest.Pull(context.Background(), wikidata.Adapter{}, opt)
	if err != nil {
		t.Fatal(err)
	}
	if m := res.Manifest; m.Records != 4 || m.Classes[ingest.Open] != 4 || m.Adapter != "wikidata/1" {
		t.Errorf("the manifest is %+v, want 4 open records from wikidata/1", m)
	}
	recs, _, err := ingest.ReadRecords(root, "wikidata")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]ingest.Record{}
	for _, r := range recs {
		byID[r.ID] = r
		if r.Kind != "item" || r.URL != "https://www.wikidata.org/wiki/"+r.ID || r.Title == "" {
			t.Errorf("the record is %+v", r)
		}
	}
	p := byID["Q11518"]
	want := map[string]string{
		"nlab":      "Pythagorean theorem",
		"mathworld": "PythagoreanTheorem",
		"proofwiki": "Pythagoras's_Theorem|Pythagorean_Theorem",
		"eom":       "Pythagoras_theorem|Pythagorean_theorem",
	}
	if p.Title != "Pythagorean theorem" || len(p.Meta) != len(want) {
		t.Errorf("Q11518 is %+v", p)
	}
	for k, v := range want {
		if p.Meta[k] != v {
			t.Errorf("Q11518 has %s %q, want %q", k, p.Meta[k], v)
		}
	}
	if d := byID["Q178546"]; d.Meta["mathlib"] != "Matrix.det" {
		t.Errorf("Q178546 is %+v", d)
	}
	if g := byID["Q131476"]; g.Meta["msc"] != "05Cxx" {
		t.Errorf("Q131476 is %+v", g)
	}

	// A second pull of the same answers gives the same snapshot.
	again, err := ingest.Pull(context.Background(), wikidata.Adapter{}, ingest.Options{Root: t.TempDir(), Cache: t.TempDir(), URL: srv.URL, Date: "2026-10-25"})
	if err != nil {
		t.Fatal(err)
	}
	if again.Manifest.Snapshot != res.Manifest.Snapshot {
		t.Errorf("two pulls give the snapshots %s and %s", res.Manifest.Snapshot, again.Manifest.Snapshot)
	}
}

func TestRead(t *testing.T) {
	none := func(ingest.Record) error { return nil }
	for _, bad := range []string{
		"",
		`{"p":"P4215","item":"L1","value":"x"}`,
		`{"p":"P31","item":"Q1","value":"Q5"}`,
		`{"p":"P4215","item":"Q1","value":""}`,
		`{"p":"P4215",`,
	} {
		if err := (wikidata.Adapter{}).Read(strings.NewReader(bad), none); err == nil {
			t.Errorf("Read(%q) gives no error", bad)
		}
	}
}

func TestSource(t *testing.T) {
	s := wikidata.Adapter{}.Source()
	if p := s.Problems(); len(p) > 0 {
		t.Errorf("the catalogue entry has problems: %v", p)
	}
	if a, ok := ingest.Lookup("wikidata"); !ok || a.Version() != "wikidata/1" {
		t.Error("the adapter is not registered")
	}
}
