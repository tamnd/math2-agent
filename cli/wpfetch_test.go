package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/wpcorpus"
)

// writeDump writes a multistream dump with one stream for each page, and
// its index. It needs the bzip2 program.
func writeDump(t *testing.T, pages []string) (dumpPath, indexPath string) {
	t.Helper()
	if _, err := exec.LookPath("bzip2"); err != nil {
		t.Skip("the test needs bzip2")
	}
	dir := t.TempDir()
	var dump, index bytes.Buffer
	streams := append(append([]string{"<mediawiki>\n<siteinfo></siteinfo>\n"}, pages...), "</mediawiki>\n")
	for i, s := range streams {
		if i > 0 && i < len(streams)-1 {
			var id int
			var title string
			_, _ = fmt.Sscanf(s[strings.Index(s, "<id>"):], "<id>%d</id>", &id)
			title = s[strings.Index(s, "<title>")+7 : strings.Index(s, "</title>")]
			fmt.Fprintf(&index, "%d:%d:%s\n", dump.Len(), id, title)
		}
		cmd := exec.Command("bzip2", "-c")
		cmd.Stdin = strings.NewReader(s)
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		dump.Write(out)
	}
	dumpPath = filepath.Join(dir, "enwiki-20261001-pages-articles-multistream.xml.bz2")
	indexPath = filepath.Join(dir, "enwiki-20261001-pages-articles-multistream-index.txt")
	if err := os.WriteFile(dumpPath, dump.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, index.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return dumpPath, indexPath
}

func dumpXML(id int, title, redirect string, rev int, ts, text string) string {
	r := ""
	if redirect != "" {
		r = fmt.Sprintf("<redirect title=%q />", redirect)
	}
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(text))
	return fmt.Sprintf("<page><title>%s</title><ns>0</ns><id>%d</id>%s<revision><id>%d</id><parentid>%d</parentid><timestamp>%s</timestamp><sha1>s%d</sha1><text>%s</text></revision></page>\n", title, id, r, rev, rev-1, ts, rev, b.String())
}

func TestWPFetch(t *testing.T) {
	wpDelay = 0
	root := wpCorpus(t)
	widget, err := os.ReadFile(filepath.Join("..", "wpcorpus", "testdata", "pages", "widget.wiki"))
	if err != nil {
		t.Fatal(err)
	}
	dumpPath, indexPath := writeDump(t, []string{
		dumpXML(12345, "Widget", "", 678, "2026-01-02T03:04:05Z", string(widget)+"\n[[Category:Widgets]]\n"),
		dumpXML(200, "Gadget", "Widget", 201, "2026-01-02T03:04:05Z", "#REDIRECT [[Widget]]"),
		dumpXML(300, "Recent", "", 302, "2026-09-30T00:00:00Z", "New text."),
	})

	var scope bytes.Buffer
	cs := []*wpcorpus.Candidate{
		{PageID: 200, Title: "Gadget", InScope: true, Verdict: "math", Rule: "strong"},
		{PageID: 300, Title: "Recent", InScope: true, Verdict: "math", Rule: "strong"},
		{PageID: 400, Title: "Gone", InScope: true, Verdict: "math", Rule: "strong"},
		{PageID: 12345, Title: "Widget", Projects: []string{"Mathematics"}, Class: "B", Importance: "Mid", InScope: true, Verdict: "math", Rule: "strong"},
	}
	if err := wpcorpus.WriteCandidates(&scope, cs, "20261001", true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, wpcorpus.ScopePath), scope.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	storePath := filepath.Join(t.TempDir(), "scope.sqlite")
	db, err := sql.Open("sqlite", storePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE page (page_id INTEGER, page_namespace INTEGER, page_title TEXT, page_is_redirect INTEGER, page_len INTEGER, page_latest INTEGER);
CREATE TABLE redirect (rd_from INTEGER, rd_namespace INTEGER, rd_title TEXT, rd_fragment TEXT);
CREATE TABLE props (pp_page INTEGER, pp_propname TEXT, pp_value TEXT);
INSERT INTO page VALUES (12345, 0, 'Widget', 0, 1, 678), (500, 0, 'Widgets', 1, 1, 501);
INSERT INTO redirect VALUES (500, 0, 'Widget', '');
INSERT INTO props VALUES (12345, 'wikibase_item', 'Q1'), (12345, 'wikibase-shortdesc', 'A set with a map');`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	// The history of the page Recent: 302 is too young, and 301 is undone
	// by 302, so the revision of record is 300.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var resp any
		switch {
		case q.Get("pageids") == "300":
			resp = map[string]any{"query": map[string]any{"pages": []any{map[string]any{"pageid": 300, "revisions": []any{
				map[string]any{"revid": 302, "timestamp": "2026-09-30T00:00:00Z", "sha1": "a"},
				map[string]any{"revid": 301, "timestamp": "2026-09-10T00:00:00Z", "sha1": "b"},
				map[string]any{"revid": 300, "timestamp": "2026-08-01T00:00:00Z", "sha1": "a"},
			}}}}}
		case q.Get("revids") == "300":
			resp = map[string]any{"query": map[string]any{"pages": []any{map[string]any{"pageid": 300, "title": "Recent", "revisions": []any{
				map[string]any{"revid": 300, "timestamp": "2026-08-01T00:00:00Z", "slots": map[string]any{"main": map[string]any{"content": "Old text."}}},
			}}}}}
		default:
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	code, stdout, stderr := run("wp", "fetch", "-corpus", root, "-dump", dumpPath, "-index", indexPath, "-store", storePath, "-api", srv.URL)
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q, stdout %q", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "4 articles, 2 pages, ") || !strings.Contains(stdout, "1 checked with the API, 2 stopped, 0 failed") {
		t.Errorf("stdout is:\n%s", stdout)
	}
	p, err := wpcorpus.ReadContent(root, 300)
	if err != nil || p.Revision != 300 || p.Wikitext != "Old text." {
		t.Errorf("the page Recent is %+v, %v", p, err)
	}
	data, err := os.ReadFile(filepath.Join(root, wpcorpus.MetadataPath(12345)))
	if err != nil {
		t.Fatal(err)
	}
	m, err := wpcorpus.ParseMetadata(data)
	if err != nil {
		t.Fatal(err)
	}
	if m.Revision != 678 || m.Wikidata != "Q1" || m.Short != "A set with a map" || m.Class != "B" || m.SHA1 != "s678" || len(m.Redirects) != 1 || m.Redirects[0] != "Widgets" || len(m.Categories) != 1 || m.Dump != "20261001" {
		t.Errorf("the metadata is %+v", m)
	}
	if _, err := os.Stat(filepath.Join(root, wpcorpus.MarkdownPath(12345))); err != nil {
		t.Error(err)
	}
	stops, err := wpcorpus.ReadStops(root)
	if err != nil || len(stops) != 2 || stops[0].PageID != 200 || stops[0].Reason != "merged" || stops[0].Into != 12345 || stops[1].Reason != "deleted" {
		t.Errorf("the stops are %+v, %v", stops, err)
	}
}
