package cli

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/wpcorpus"
)

func TestWPCitedReport(t *testing.T) {
	tree := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tree, "nodes"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "nodes", "a.yaml"), []byte("wikipedia:\n  - Widget\n  - Widgets\n  - Paris\n  - Nowhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	corpus := t.TempDir()
	if err := os.MkdirAll(filepath.Join(corpus, wpcorpus.ManifestDir), 0o750); err != nil {
		t.Fatal(err)
	}
	var cands bytes.Buffer
	cs := []*wpcorpus.Candidate{
		{PageID: 12345, Title: "Widget", InScope: true, Verdict: "math", Rule: "strong"},
		{PageID: 900, Title: "Paris", Verdict: "not-math", Rule: "model"},
	}
	if err := wpcorpus.WriteCandidates(&cands, cs, "20261001", false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corpus, wpcorpus.CandidatesPath), cands.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	storePath := filepath.Join(t.TempDir(), "scope.sqlite")
	db, err := sql.Open("sqlite", storePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE page (page_id INTEGER, page_namespace INTEGER, page_title TEXT, page_is_redirect INTEGER, page_len INTEGER, page_latest INTEGER);
CREATE TABLE redirect (rd_from INTEGER, rd_namespace INTEGER, rd_title TEXT, rd_fragment TEXT);
INSERT INTO page VALUES (12345, 0, 'Widget', 0, 1, 678), (500, 0, 'Widgets', 1, 1, 501), (900, 0, 'Paris', 0, 1, 901);
INSERT INTO redirect VALUES (500, 0, 'Widget', '');`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	code, stdout, stderr := run("wp", "cited", "-root", tree, "-store", storePath, "-corpus", corpus)
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q, stdout %q", code, stderr, stdout)
	}
	if stdout != "4 titles, 2 with a mention, 1 out-of-scope, 1 no-page, 0 no-candidate, report reports/cited.md\n" {
		t.Errorf("stdout %q", stdout)
	}
	b, err := os.ReadFile(filepath.Join(corpus, wpcorpus.CitedReportPath))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"mentions: 2\n", "| Nowhere |  | no-page |  |\n", "| Paris | 900 | out-of-scope | Paris, verdict not-math, rule model |\n"} {
		if !strings.Contains(string(b), w) {
			t.Errorf("report has no %q:\n%s", w, b)
		}
	}
}
