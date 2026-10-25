package arxiv_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/arxiv"
)

// plane serves testdata as the metadata plane of tamnd/arxiv, with a GitHub contents API listing at /contents/metadata.
func plane(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/contents/metadata" {
			type entry struct {
				Name     string `json:"name"`
				Type     string `json:"type"`
				Download string `json:"download_url"`
			}
			list := []entry{{Name: "README.md", Type: "file", Download: srv.URL + "/raw/README.md"}}
			for _, n := range []string{"2312.jsonl", "0211.jsonl", "1311.jsonl"} {
				list = append(list, entry{Name: n, Type: "file", Download: srv.URL + "/raw/" + n})
			}
			_ = json.NewEncoder(w).Encode(list)
			return
		}
		b, err := os.ReadFile("testdata/" + strings.TrimPrefix(r.URL.Path, "/raw/"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPull(t *testing.T) {
	srv := plane(t)
	root := t.TempDir()
	opt := ingest.Options{Root: root, Cache: t.TempDir(), URL: srv.URL + "/contents/metadata?ref=main", Date: "2026-10-25"}
	res, err := ingest.Pull(context.Background(), arxiv.Adapter{}, opt)
	if err != nil {
		t.Fatal(err)
	}
	if m := res.Manifest; m.Records != 3 || m.Classes[ingest.RecordOnly] != 2 || m.Classes[ingest.Open] != 1 {
		t.Errorf("the manifest is %+v, want 3 records: 2 record and 1 open", m)
	}
	recs, _, err := ingest.ReadRecords(root, "arxiv")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range recs {
		ids = append(ids, r.ID)
		if r.Text != "" || r.Locator != "ax://paper/"+r.ID || r.URL != "https://arxiv.org/abs/"+r.ID {
			t.Errorf("the record %s has the text %q, the locator %s and the URL %s", r.ID, r.Text, r.Locator, r.URL)
		}
	}
	if got := strings.Join(ids, " "); got != "1311.4600 9912.99999 math/0211159" {
		t.Errorf("the records are %s, want the three math papers", got)
	}
	s := recs[1]
	if s.Licence != "CC-BY-SA-4.0" || s.Class != ingest.Open || s.Meta["msc"] != "05C15 05C10" || s.Meta["licence_version"] != "v2" || s.Meta["categories"] != "cs.DM math.CO" || s.Meta["authors"] != "Ada Tester" {
		t.Errorf("the record is %+v", s)
	}
	if p := recs[2]; p.Licence != "LicenseRef-arXiv" || p.Class != ingest.RecordOnly {
		t.Errorf("a paper with no licence has the licence %s and the class %s", p.Licence, p.Class)
	}

	// One month by its URL.
	one, err := ingest.Pull(context.Background(), arxiv.Adapter{}, ingest.Options{Root: t.TempDir(), Cache: opt.Cache, URL: srv.URL + "/raw/1311.jsonl"})
	if err != nil {
		t.Fatal(err)
	}
	if one.Manifest.Records != 1 {
		t.Errorf("one month gives %d records, want 1", one.Manifest.Records)
	}
}

func TestFetchErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"README.md","type":"file","download_url":"x"}]`))
	}))
	defer srv.Close()
	_, err := ingest.Pull(context.Background(), arxiv.Adapter{}, ingest.Options{Root: t.TempDir(), Cache: t.TempDir(), URL: srv.URL + "/contents/metadata"})
	if err == nil || !strings.Contains(err.Error(), "lists no .jsonl files") {
		t.Errorf("a listing without months gives %v", err)
	}
}

func TestLicence(t *testing.T) {
	for in, want := range map[string]string{"cc0": "CC0-1.0", "cc-by": "CC-BY-4.0", "cc-by-nc-nd": "CC-BY-NC-ND-4.0", "arxiv-1.0": "LicenseRef-arXiv", "unknown": "LicenseRef-arXiv", "": "LicenseRef-arXiv"} {
		if got := arxiv.Licence(in); got != want {
			t.Errorf("Licence(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestMath(t *testing.T) {
	for cats, want := range map[string]bool{"math.NT": true, "cs.LG math.CO": true, "math-ph": true, "cs.LG cs.AI": false, "": false, "mathematics": false} {
		if got := arxiv.Math(strings.Fields(cats)); got != want {
			t.Errorf("Math(%q) = %v, want %v", cats, got, want)
		}
	}
}
