package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/wpcorpus"
)

func TestWPRefresh(t *testing.T) {
	wpDelay = 0
	root := wpCorpus(t)
	widget, err := os.ReadFile(filepath.Join("..", "wpcorpus", "testdata", "pages", "widget.wiki"))
	if err != nil {
		t.Fatal(err)
	}
	var scope bytes.Buffer
	cs := []*wpcorpus.Candidate{
		{PageID: 300, Title: "Recent", InScope: true, Verdict: "math", Rule: "strong"},
		{PageID: 400, Title: "Gone", InScope: true, Verdict: "math", Rule: "strong"},
		{PageID: 500, Title: "Old name", InScope: true, Verdict: "math", Rule: "strong"},
		{PageID: 600, Title: "Same", InScope: true, Verdict: "math", Rule: "strong"},
		{PageID: 12345, Title: "Widget", Class: "B", InScope: true, Verdict: "math", Rule: "strong"},
	}
	if err := wpcorpus.WriteCandidates(&scope, cs, "20261001", true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, wpcorpus.ScopePath), scope.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, md := range []*wpcorpus.Metadata{
		{Page: 300, Title: "Recent", Revision: 300, Timestamp: "2026-08-01T00:00:00Z", Type: "article"},
		{Page: 400, Title: "Gone", Revision: 401, Timestamp: "2026-08-01T00:00:00Z", Type: "article"},
		{Page: 500, Title: "Old name", Revision: 501, Timestamp: "2026-08-01T00:00:00Z", Type: "article"},
		{Page: 600, Title: "Same", Revision: 601, Timestamp: "2026-08-01T00:00:00Z", Type: "article"},
		{Page: 12345, Title: "Widget", Revision: 678, Timestamp: "2026-01-02T03:04:05Z", Type: "article", Wikidata: "Q1", Short: "A set with a map", Redirects: []string{"Widgets"}, MSC: []string{"20A05"}},
	} {
		if err := wpcorpus.WriteMetadata(root, md); err != nil {
			t.Fatal(err)
		}
	}

	rev := func(id int, ts, sha string) map[string]any {
		return map[string]any{"revid": id, "timestamp": ts, "sha1": sha}
	}
	pages := func(ps ...any) map[string]any { return map[string]any{"query": map[string]any{"pages": ps}} }
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		asked = append(asked, q.Get("pageids")+q.Get("revids"))
		var resp any
		switch {
		case q.Get("prop") == "revisions|info":
			ids := strings.Split(q.Get("pageids"), "|")
			slices.Sort(ids)
			if strings.Join(ids, "|") != "12345|300|400|500|600" {
				http.Error(w, "bad pageids "+q.Get("pageids"), http.StatusBadRequest)
				return
			}
			resp = pages(
				map[string]any{"pageid": 300, "revisions": []any{rev(302, "2026-10-30T00:00:00Z", "c")}},
				map[string]any{"pageid": 400, "missing": true},
				map[string]any{"pageid": 500, "redirect": true, "revisions": []any{rev(502, "2026-10-01T00:00:00Z", "r")}},
				map[string]any{"pageid": 600, "revisions": []any{rev(601, "2026-08-01T00:00:00Z", "s")}},
				map[string]any{"pageid": 12345, "revisions": []any{rev(700, "2026-10-01T00:00:00Z", "w")}},
			)
		case q.Get("redirects") == "1" && q.Get("pageids") == "500":
			resp = pages(map[string]any{"pageid": 12345, "title": "Widget"})
		case q.Get("rvlimit") != "" && q.Get("pageids") == "300":
			// 302 is too young, so the revision of record stays 300.
			resp = pages(map[string]any{"pageid": 300, "revisions": []any{rev(302, "2026-10-30T00:00:00Z", "c"), rev(300, "2026-08-01T00:00:00Z", "a")}})
		case q.Get("rvlimit") != "" && q.Get("pageids") == "12345":
			resp = pages(map[string]any{"pageid": 12345, "revisions": []any{rev(700, "2026-10-01T00:00:00Z", "w"), rev(678, "2026-01-02T03:04:05Z", "v")}})
		case q.Get("revids") == "700":
			resp = pages(map[string]any{"pageid": 12345, "title": "Widget", "revisions": []any{
				map[string]any{"revid": 700, "timestamp": "2026-10-01T00:00:00Z", "slots": map[string]any{"main": map[string]any{"content": string(widget)}}},
			}})
		default:
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	code, stdout, stderr := run("wp", "refresh", "-corpus", root, "-api", srv.URL, "-dry-run", "-now", "2026-11-03")
	if code != ExitOK || !strings.Contains(stdout, "5 articles, 2 with a new revision, 1 requests") {
		t.Fatalf("dry run: exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, wpcorpus.StoppedPath)); err == nil {
		t.Error("the dry run wrote the stops")
	}

	asked = nil
	code, stdout, stderr = run("wp", "refresh", "-corpus", root, "-api", srv.URL, "-now", "2026-11-03")
	if code != ExitOK {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "5 articles, 2 with a new revision, 1 refreshed, 1 kept, 2 stopped, 0 failed, 5 requests, run record reports/runs/") {
		t.Errorf("stdout is:\n%s", stdout)
	}
	p, err := wpcorpus.ReadContent(root, 12345)
	if err != nil || p.Revision != 700 {
		t.Errorf("the content of Widget is %+v, %v", p, err)
	}
	read := func(id int) *wpcorpus.Metadata {
		data, err := os.ReadFile(filepath.Join(root, wpcorpus.MetadataPath(id)))
		if err != nil {
			t.Fatal(err)
		}
		m, err := wpcorpus.ParseMetadata(data)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	if m := read(12345); m.Revision != 700 || m.SHA1 != "w" || m.Wikidata != "Q1" || m.Short != "A set with a map" || !slices.Equal(m.Redirects, []string{"Widgets"}) || !slices.Equal(m.MSC, []string{"20A05"}) || m.Class != "B" {
		t.Errorf("the metadata of Widget is %+v", m)
	}
	if m := read(300); m.Revision != 300 || m.Settled != 0 || m.Unstable {
		t.Errorf("the metadata of Recent is %+v", m)
	}
	stops, err := wpcorpus.ReadStops(root)
	if err != nil || len(stops) != 2 {
		t.Fatalf("the stops are %+v, %v", stops, err)
	}
	slices.SortFunc(stops, func(a, b wpcorpus.Stop) int { return int(a.PageID - b.PageID) })
	if stops[0].PageID != 400 || stops[0].Reason != "deleted" || stops[1].PageID != 500 || stops[1].Reason != "merged" || stops[1].Into != 12345 {
		t.Errorf("the stops are %+v", stops)
	}
}
