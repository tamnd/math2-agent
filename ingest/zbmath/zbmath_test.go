package zbmath_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/zbmath"
)

// harvest serves the two pages of testdata as an OAI-PMH list.
func harvest(t *testing.T) *httptest.Server {
	t.Helper()
	page := func(name string) []byte {
		b, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	p1, p2 := page("page1.xml"), page("page2.xml")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("resumptionToken") == "7ddd54a1-0de1-423a-a054-8ee14bf374d9":
			_, _ = w.Write(p2)
		case q.Get("metadataPrefix") == "oai_zb_preview":
			_, _ = w.Write(p1)
		default:
			http.Error(w, "bad request", http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPull(t *testing.T) {
	srv := harvest(t)
	root := t.TempDir()
	opt := ingest.Options{Root: root, Cache: t.TempDir(), URL: srv.URL + "/v1/?verb=ListRecords&metadataPrefix=oai_zb_preview&set=03", Date: "2026-10-25"}
	res, err := ingest.Pull(context.Background(), zbmath.Adapter{}, opt)
	if err != nil {
		t.Fatal(err)
	}
	if m := res.Manifest; m.Records != 4 || m.Classes[ingest.Open] != 4 || m.Adapter != "zbmath/1" {
		t.Errorf("the manifest is %+v, want 4 open records from zbmath/1", m)
	}
	recs, _, err := ingest.ReadRecords(root, "zbmath")
	if err != nil {
		t.Fatal(err)
	}
	var maynard *ingest.Record
	for i, r := range recs {
		if r.Meta["msc"] == "" {
			t.Errorf("the record %s has no MSC codes", r.ID)
		}
		if r.Text != "" || r.Meta["zbl"] == "None" {
			t.Errorf("the record %s has the text %q and the zbl %q", r.ID, r.Text, r.Meta["zbl"])
		}
		if r.ID == "6383667" {
			maynard = &recs[i]
		}
	}
	if maynard == nil {
		t.Fatal("no record 6383667")
	}
	want := ingest.Record{
		ID:      "6383667",
		Kind:    "article",
		Title:   "Small gaps between primes",
		URL:     "https://zbmath.org/?q=an:6383667",
		Locator: "Ann. Math. (2) 181, No. 1, 383-413 (2015).",
		Class:   ingest.Open,
	}
	got := *maynard
	meta := got.Meta
	got.Meta = nil
	if got.ID != want.ID || got.Kind != want.Kind || got.Title != want.Title || got.URL != want.URL || got.Locator != want.Locator || got.Class != want.Class {
		t.Errorf("the record is %+v, want %+v", got, want)
	}
	if meta["msc"] != "11N05 11N36" || meta["zbl"] != "1306.11073" || meta["year"] != "2015" || meta["doi"] != "10.4007/annals.2015.181.1.7" || meta["authors"] != "Maynard, James" || len(strings.Fields(meta["refs"])) != 2 {
		t.Errorf("the meta is %v", meta)
	}

	// The same snapshot from the cache gives the same records.
	again, err := ingest.Pull(context.Background(), zbmath.Adapter{}, ingest.Options{Root: root, Cache: opt.Cache, Snapshot: res.Snapshot.Hash})
	if err != nil {
		t.Fatal(err)
	}
	if again.Manifest.Hash != res.Manifest.Hash || again.New {
		t.Errorf("the pinned import has the hash %s, want %s", again.Manifest.Hash, res.Manifest.Hash)
	}
}

func TestRead(t *testing.T) {
	page1, err := os.ReadFile("testdata/page1.xml")
	if err != nil {
		t.Fatal(err)
	}
	none := func(ingest.Record) error { return nil }
	if err := (zbmath.Adapter{}).Read(strings.NewReader(string(page1)), none); err == nil || !strings.Contains(err.Error(), "the harvest has 3 records, and zbMATH Open gives a list of 4") {
		t.Errorf("a short harvest gives %v", err)
	}
	empty := `<?xml version="1.0" encoding="utf-8"?><OAI-PMH><error code="400">noRecordsMatch</error></OAI-PMH>`
	if err := (zbmath.Adapter{}).Read(strings.NewReader(empty), none); err != nil {
		t.Errorf("noRecordsMatch gives %v, want no error", err)
	}
	bad := `<?xml version="1.0" encoding="utf-8"?><OAI-PMH><error code="badArgument">the set is unknown</error></OAI-PMH>`
	if err := (zbmath.Adapter{}).Read(strings.NewReader(bad), none); err == nil {
		t.Error("an OAI-PMH error gives no error")
	}
}

func TestSource(t *testing.T) {
	s := zbmath.Adapter{}.Source()
	if p := s.Problems(); len(p) > 0 {
		t.Errorf("the catalogue entry has problems: %v", p)
	}
	if a, ok := ingest.Lookup("zbmath"); !ok || a.Version() != "zbmath/1" {
		t.Error("the adapter is not registered")
	}
}

func TestCountErrors(t *testing.T) {
	for _, s := range []string{``, `{"status": {}}`, `{"status": {"nr_total_results": -1}}`, `<html>`} {
		if _, err := zbmath.Count(strings.NewReader(s)); err == nil {
			t.Errorf("Count(%q) gives no error", s)
		}
	}
}
