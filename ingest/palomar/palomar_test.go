package palomar_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/palomar"
)

// registry serves the pages and the complete record of testdata. The first conflicts requests with a cursor get 409 Conflict.
func registry(t *testing.T, conflicts int32) (*httptest.Server, *atomic.Int32) {
	files := map[string][]byte{}
	for _, name := range []string{"page1", "page2", "entry"} {
		b, err := os.ReadFile("testdata/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		files[name] = b
	}
	var n, conflicted atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/entries/PALOMAR-2026-10-23-000018-v1.json":
			_, _ = w.Write(files["entry"])
		case r.URL.Path != "/api/v1/results" || q.Get("msc") != "54":
			http.NotFound(w, r)
		case q.Get("cursor") == "":
			_, _ = w.Write(files["page1"])
		case conflicted.Add(1) <= conflicts:
			http.Error(w, "the registry changed", http.StatusConflict)
		case q.Get("cursor") == "YzI=":
			_, _ = w.Write(files["page2"])
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func TestPull(t *testing.T) {
	srv, requests := registry(t, 1)
	root := t.TempDir()
	res, err := ingest.Pull(context.Background(), palomar.Adapter{}, ingest.Options{Root: root, Cache: t.TempDir(), URL: srv.URL + "/api/v1/results?order=registered&msc=54", Date: "2026-10-26"})
	if err != nil {
		t.Fatal(err)
	}
	// Page 1, the conflict, page 1 again, page 2 and the complete record.
	if n := requests.Load(); n != 5 {
		t.Errorf("the fetch made %d requests, want 5", n)
	}
	if m := res.Manifest; m.Records != 3 || m.Classes[ingest.Open] != 3 || m.Adapter != "palomar/1" {
		t.Errorf("the manifest is %+v, want 3 open records from palomar/1", m)
	}
	recs, _, err := ingest.ReadRecords(root, "palomar")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]ingest.Record{}
	for _, r := range recs {
		byID[r.ID] = r
		if r.Kind != "result" || r.Text == "" || r.URL != palomar.Home+"/entry?id="+r.ID+"&version="+r.Meta["version"] {
			t.Errorf("the record is %+v", r)
		}
	}

	r := byID["PALOMAR-2026-10-23-000018"]
	want := map[string]string{
		"version":      "1",
		"authors":      "Arthur Freitas Ramos|David Barros Hulak|Ruy Jose Guerra Barretto de Queiroz",
		"arxiv":        "math.GN",
		"msc":          "54C20 54E50 54H05",
		"repository":   "Arthur742Ramos/Math-Formalization",
		"commit":       "a8c08adf51ec122bff60955a260a3b2f772ea0ac",
		"project_path": "topology/lavrentieff-extension",
		"trust":        "high",
		"status":       "registered",
		"published_at": "2026-10-23T20:31:04Z",
	}
	for k, v := range want {
		if r.Meta[k] != v {
			t.Errorf("the %s of %s is %q, want %q", k, r.ID, r.Meta[k], v)
		}
	}
	// The listing abbreviates this entry, so the record comes from the complete record.
	if th := strings.Fields(r.Meta["theorems"]); len(th) != 8 || th[0] != "Lavrentieff.extensionDomain_isGδ" || !strings.HasSuffix(r.Text, "with no mathematical novelty or priority claim.") {
		t.Errorf("the theorems of %s are %v and the text is %q, want 8 theorems and the complete abstract", r.ID, th, r.Text)
	}
	if r.Title != "Lavrentieff's Gδ homeomorphism extension theorem" {
		t.Errorf("the title of %s is %q", r.ID, r.Title)
	}
	if r := byID["PALOMAR-2026-10-23-000017"]; r.Meta["project_path"] != "" || r.Meta["theorems"] == "" {
		t.Errorf("PALOMAR-2026-10-23-000017 is %+v", r)
	}
}

func TestFetchErrors(t *testing.T) {
	srv, _ := registry(t, 10)
	for name, u := range map[string]string{
		"the registry changes":   srv.URL + "/api/v1/results?msc=54",
		"not the listing":        srv.URL + "/entries/x.json",
		"the listing is missing": srv.URL + "/api/v1/results?msc=11",
	} {
		_, err := ingest.Pull(context.Background(), palomar.Adapter{}, ingest.Options{Root: t.TempDir(), Cache: t.TempDir(), URL: u})
		if err == nil {
			t.Errorf("a fetch where %s gives no error", name)
		}
	}
}

func TestRead(t *testing.T) {
	none := func(ingest.Record) error { return nil }
	const (
		h  = `{"query":"","revision":1,"release":"r","results":1}` + "\n"
		ok = `{"id":"PALOMAR-2026-10-23-000018","version":1,"title":"T"}` + "\n"
	)
	for name, snap := range map[string]string{
		"no header":   "",
		"no entries":  h,
		"bad id":      h + `{"id":"PALOMAR-1","version":1,"title":"T"}` + "\n",
		"two times":   h + ok + ok,
		"no title":    h + `{"id":"PALOMAR-2026-10-23-000018","version":1}` + "\n",
		"no version":  h + `{"id":"PALOMAR-2026-10-23-000018","title":"T"}` + "\n",
		"abbreviated": h + `{"id":"PALOMAR-2026-10-23-000018","version":1,"title":"T","abbreviated":true}` + "\n",
		"not JSON":    h + "<html>",
	} {
		if err := (palomar.Adapter{}).Read(strings.NewReader(snap), none); err == nil {
			t.Errorf("Read of a snapshot with %s gives no error", name)
		}
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte(h + ok))
	_ = zw.Close()
	zr, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if err := (palomar.Adapter{}).Read(zr, none); err != nil {
		t.Errorf("Read of a good snapshot gives %v", err)
	}
}

func TestSource(t *testing.T) {
	s := palomar.Adapter{}.Source()
	if p := s.Problems(); len(p) > 0 {
		t.Errorf("the catalogue entry has problems: %v", p)
	}
	if a, ok := ingest.Lookup("palomar"); !ok || a.Version() != "palomar/1" {
		t.Error("the adapter is not registered")
	}
}
