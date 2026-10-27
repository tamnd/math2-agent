package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/wpcorpus"
)

// wpCorpus makes a corpus with the manifests of the wpcorpus tests.
func wpCorpus(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join("..", "wpcorpus", "testdata", "corpus", wpcorpus.ManifestDir)
	dst := filepath.Join(root, wpcorpus.ManifestDir)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"templates.yaml", "headings.yaml", "waivers.yaml"} {
		data, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, f), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// wpAPI serves the page Widget with the revision rev. Other titles are
// missing.
func wpAPI(t *testing.T, rev *int) *httptest.Server {
	t.Helper()
	text, err := os.ReadFile(filepath.Join("..", "wpcorpus", "testdata", "pages", "widget.wiki"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("action") != "query" || q.Get("maxlag") == "" || !strings.HasPrefix(r.UserAgent(), "m2/") {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		page := map[string]any{"ns": 0, "title": q.Get("titles"), "missing": true}
		if q.Get("titles") == "Widget" {
			page = map[string]any{"pageid": 12345, "ns": 0, "title": "Widget", "revisions": []any{
				map[string]any{"revid": *rev, "timestamp": "2026-10-01T00:00:00Z", "slots": map[string]any{"main": map[string]any{"content": string(text)}}},
			}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"query": map[string]any{"pages": []any{page}}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestWPExtractFetch(t *testing.T) {
	wpDelay = 0
	root := wpCorpus(t)
	rev := 678
	srv := wpAPI(t, &rev)

	code, stdout, stderr := run("wp", "extract", "-corpus", root, "-api", srv.URL, "Widget")
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "12345 r678 Widget: ") || !strings.Contains(stdout, "1 pages, ") || !strings.Contains(stdout, " 0 gaps, 0 failed") {
		t.Errorf("stdout is:\n%s", stdout)
	}
	p, err := wpcorpus.ReadContent(root, 12345)
	if err != nil || p.Title != "Widget" || p.Revision != 678 {
		t.Fatalf("the content file is %+v, %v", p, err)
	}
	objs, err := wpcorpus.ReadObjects(root, 12345, 678)
	if err != nil || len(objs) == 0 || objs[0].Extractor != "m2/v0.0.0-test" {
		t.Fatalf("the objects are %d, %v", len(objs), err)
	}
	c, err := wpcorpus.ReadCensus(root, 12345, 678)
	if err != nil || c.Title != "Widget" {
		t.Fatalf("the census is %+v, %v", c, err)
	}

	// A new revision replaces the files of the old revision.
	rev = 700
	if code, _, stderr := run("wp", "extract", "-corpus", root, "-api", srv.URL, "Widget"); code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, wpcorpus.ObjectsPath(12345, 678))); !os.IsNotExist(err) {
		t.Errorf("the objects file of r678 is still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, wpcorpus.CensusPath(12345, 678))); !os.IsNotExist(err) {
		t.Errorf("the census file of r678 is still there: %v", err)
	}
	if _, err := wpcorpus.ReadCensus(root, 12345, 700); err != nil {
		t.Errorf("the census of r700: %v", err)
	}
}

func TestWPExtractAgain(t *testing.T) {
	wpDelay = 0
	root := wpCorpus(t)
	rev := 678
	srv := wpAPI(t, &rev)
	if code, _, stderr := run("wp", "extract", "-corpus", root, "-api", srv.URL, "Widget"); code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	path := filepath.Join(root, wpcorpus.ObjectsPath(12345, 678))
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	// With no titles, the command reads the content plane and needs no API.
	for _, args := range [][]string{{}, {"-page", "12345"}, {"-type", "article"}} {
		code, stdout, stderr := run(append([]string{"wp", "extract", "-corpus", root, "-api", "http://127.0.0.1:1"}, args...)...)
		if code != ExitOK || !strings.Contains(stdout, "1 pages, ") {
			t.Fatalf("%v: exit code %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
		again, err := os.ReadFile(path)
		if err != nil || string(again) != string(first) {
			t.Fatalf("%v: the objects file changed: %v", args, err)
		}
	}
	code, stdout, _ := run("wp", "extract", "-corpus", root, "-type", "glossary")
	if code != ExitOK || !strings.Contains(stdout, "0 pages, ") {
		t.Errorf("-type glossary: exit code %d, stdout %q", code, stdout)
	}
	code, _, stderr := run("wp", "extract", "-corpus", root, "-page", "99")
	if code != ExitFailure || !strings.Contains(stderr, "m2 wp extract: ") {
		t.Errorf("-page 99: exit code %d, stderr %q", code, stderr)
	}
}

func TestWPExtractList(t *testing.T) {
	wpDelay = 0
	root := wpCorpus(t)
	rev := 678
	srv := wpAPI(t, &rev)
	list := filepath.Join(t.TempDir(), "titles.txt")
	if err := os.WriteFile(list, []byte("# the pilot\nWidget\n\nNo such page\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("wp", "extract", "-corpus", root, "-api", srv.URL, "-list", list)
	if code != ExitFailure {
		t.Fatalf("exit code %d, want %d", code, ExitFailure)
	}
	if !strings.Contains(stdout, "1 pages, ") || !strings.Contains(stdout, " 1 failed") || !strings.Contains(stderr, "No such page: the page does not exist") {
		t.Errorf("stdout %q, stderr %q", stdout, stderr)
	}
}

func TestWPUsage(t *testing.T) {
	root := wpCorpus(t)
	for _, c := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"wp"}, ExitUsage, "usage: m2 wp extract"},
		{[]string{"wp", "frob"}, ExitUsage, "unknown subcommand"},
		{[]string{"wp", "scope"}, ExitUsage, "usage: m2 wp scope -store DB"},
		{[]string{"wp", "gold"}, ExitNotWritten, "It arrives in WP-M1, the scope."},
		{[]string{"wp", "fetch"}, ExitNotWritten, "It arrives in WP-M2, the content plane."},
		{[]string{"wp", "census", "report"}, ExitNotWritten, "It arrives in WP-M3, extraction and the census."},
		{[]string{"wp", "refresh"}, ExitNotWritten, "It arrives in WP-M7, all of the scope."},
		{[]string{"wp", "extract", "-corpus", root, "-page", "1", "Widget"}, ExitUsage, "give titles or -page, not both"},
		{[]string{"wp", "extract", "-corpus", t.TempDir()}, ExitFailure, "templates.yaml"},
	} {
		code, stdout, stderr := run(c.args...)
		if code != c.code || !strings.Contains(stderr, c.want) {
			t.Errorf("%v: exit code %d, stderr %q", c.args, code, stderr)
		}
		if code == ExitNotWritten && stdout != "" {
			t.Errorf("%v: stdout %q", c.args, stdout)
		}
	}
}

func TestAuditWikipedia(t *testing.T) {
	wpDelay = 0
	corpus := wpCorpus(t)
	rev := 678
	srv := wpAPI(t, &rev)
	if code, _, stderr := run("wp", "extract", "-corpus", corpus, "-api", srv.URL, "Widget"); code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	root := t.TempDir()
	code, stdout, stderr := run("audit", "-root", root, "-wikipedia", corpus, "-group", "y", "-hard", "-q")
	if code != ExitOK {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Y02   pass") || !strings.Contains(stdout, "Y03   pass") || strings.Contains(stdout, "Y08") {
		t.Errorf("stdout is:\n%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(root, "reports", "audit", "Y-hard.txt")); err != nil {
		t.Error(err)
	}
	if code, _, stderr := run("audit", "-root", root, "-group", "Q"); code != ExitUsage || !strings.Contains(stderr, "no rule of the audit starts with the letter") {
		t.Errorf("-group Q: exit code %d, stderr %q", code, stderr)
	}
}
