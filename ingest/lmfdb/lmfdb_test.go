package lmfdb_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/lmfdb"
)

// The elliptic curves over Q of conductor 11 and 14.
var curves = []string{"11.a1", "11.a2", "11.a3", "14.a1", "14.a2", "14.a3", "14.a4", "14.a5", "14.a6"}

// api answers like the API of the LMFDB, with pages of 4 objects. Like the LMFDB, it gives a next page also after the last one.
func api(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/api/ec_curvedata/" || q.Get("_format") != "json" || q.Get("_fields") != "lmfdb_label" || q.Get("conductor") != `py{"$lte":14}` {
			http.Error(w, "bad request "+r.URL.String(), http.StatusBadRequest)
			return
		}
		start, _ := strconv.Atoi(q.Get("_offset"))
		end := min(start+4, len(curves))
		data := []map[string]any{}
		for i := start; i < end; i++ {
			data = append(data, map[string]any{"id": i, "lmfdb_label": curves[i]})
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"table": "ec_curvedata", "data": data, "start": start, "offset": max(end, start)}); err != nil {
			t.Error(err)
		}
	}))
}

func TestPull(t *testing.T) {
	srv := api(t)
	defer srv.Close()

	root := t.TempDir()
	u := srv.URL + `/api/ec_curvedata/?conductor=py{"$lte":14}&_offset=7`
	res, err := ingest.Pull(context.Background(), lmfdb.Adapter{}, ingest.Options{Root: root, Cache: t.TempDir(), URL: u, Date: "2026-10-25"})
	if err != nil {
		t.Fatal(err)
	}
	if m := res.Manifest; m.Records != len(curves) || m.Classes[ingest.Open] != len(curves) || m.Adapter != "lmfdb/1" {
		t.Errorf("the manifest is %+v, want %d open records from lmfdb/1", m, len(curves))
	}
	recs, _, err := ingest.ReadRecords(root, "lmfdb")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, r := range recs {
		seen[r.ID] = true
		if err := crosswalk.Check("lmfdb", r.ID); err != nil {
			t.Error(err)
		}
		if r.Kind != "ec_curvedata" || r.URL != lmfdb.Home+r.ID || !strings.HasSuffix(r.ID, "/"+r.Title) {
			t.Errorf("the record is %+v", r)
		}
	}
	for _, c := range curves {
		if !seen["EllipticCurve/Q/"+c] {
			t.Errorf("no record of %s", c)
		}
	}
}

func TestFetch(t *testing.T) {
	for _, u := range []string{"https://www.lmfdb.org/api/g2c_curves/", "https://www.lmfdb.org/EllipticCurve/Q/", "%"} {
		if _, err := ingest.Pull(context.Background(), lmfdb.Adapter{}, ingest.Options{Root: t.TempDir(), Cache: t.TempDir(), URL: u}); err == nil {
			t.Errorf("Pull of %s gives no error", u)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<!doctype html><html>captcha</html>"))
	}))
	defer srv.Close()
	_, err := ingest.Pull(context.Background(), lmfdb.Adapter{}, ingest.Options{Root: t.TempDir(), Cache: t.TempDir(), URL: srv.URL + "/api/nf_fields/?degree=i2"})
	if err == nil || !strings.Contains(err.Error(), "captcha") {
		t.Errorf("a page that is not JSON gives the error %v", err)
	}
}

func TestRead(t *testing.T) {
	none := func(ingest.Record) error { return nil }
	for _, bad := range []string{
		"",
		`{"table":"ec_curvedata","query":""}`,
		`{"table":"g2c_curves","query":""}` + "\n" + `"249.a.249.1"`,
		`{"table":"ec_curvedata","query":""}` + "\n" + `"11 a1"`,
		`{"table":"ec_curvedata","query":""}` + "\n" + `11`,
	} {
		if err := (lmfdb.Adapter{}).Read(strings.NewReader(bad), none); err == nil {
			t.Errorf("Read(%q) gives no error", bad)
		}
	}
}

func TestSource(t *testing.T) {
	s := lmfdb.Adapter{}.Source()
	if p := s.Problems(); len(p) > 0 {
		t.Errorf("the catalogue entry has problems: %v", p)
	}
	if a, ok := ingest.Lookup("lmfdb"); !ok || a.Version() != "lmfdb/1" || a.Dump() != lmfdb.Default {
		t.Error("the adapter is not registered")
	}
}
