package cli

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/wpcorpus"
)

func TestWPPointers(t *testing.T) {
	root := wpCorpus(t)
	widget, err := os.ReadFile(filepath.Join("..", "wpcorpus", "testdata", "pages", "widget.wiki"))
	if err != nil {
		t.Fatal(err)
	}
	dumpPath, indexPath := writeDump(t, []string{
		dumpXML(12345, "Widget", "", 678, "2026-01-02T03:04:05Z", string(widget)),
		dumpXML(500, "Widgets", "Widget", 501, "2026-01-02T03:04:05Z", "#REDIRECT [[Widget#Gadget theorem]]"),
		dumpXML(600, "Calculus", "", 601, "2026-01-02T03:04:05Z", "See [[Widget#Gadget theorem]], [[widgets#Example|an example]], [[Widget#Gadget_theorem]] and [[Calculus#Self]]."),
	})
	var scope bytes.Buffer
	cs := []*wpcorpus.Candidate{{PageID: 12345, Title: "Widget", InScope: true, Verdict: "math", Rule: "strong"}}
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
INSERT INTO page VALUES (12345, 0, 'Widget', 0, 1, 678), (500, 0, 'Widgets', 1, 1, 501), (700, 0, 'Widget_statement', 1, 1, 701), (800, 0, 'Lost_widget', 1, 1, 801);
INSERT INTO redirect VALUES (500, 0, 'Widget', 'Gadget theorem'), (700, 0, 'Widget', 'Statement'), (800, 0, 'Widget', 'Lost_part');`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	code, stdout, stderr := run("wp", "pointers", "-corpus", root, "-store", storePath, "-dump", dumpPath, "-index", indexPath)
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q, stdout %q", code, stderr, stdout)
	}
	if !strings.HasPrefix(stdout, "1 articles, 3 redirects and 2 links with an anchor, 2 pages of the dump read, run record ") {
		t.Errorf("stdout is %q", stdout)
	}
	f, err := wpcorpus.ReadPointers(root, 12345)
	if err != nil || f == nil {
		t.Fatalf("pointers %+v, %v", f, err)
	}
	want := []wpcorpus.Pointer{
		{Via: "redirect", From: "Lost widget", Anchor: "Lost part"},
		{Via: "redirect", From: "Widget statement", Anchor: "Statement"},
		{Via: "redirect", From: "Widgets", Anchor: "Gadget theorem"},
		{Via: "link", From: "Calculus", Anchor: "Example"},
		{Via: "link", From: "Calculus", Anchor: "Gadget theorem"},
	}
	if !slices.Equal(f.Pointers, want) {
		t.Errorf("pointers = %+v", f.Pointers)
	}

	// The extract reads the pointers file into the census.
	if err := wpcorpus.WriteContent(root, &wpcorpus.Page{ID: 12345, Revision: 678, Title: "Widget", Wikitext: string(widget)}); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := run("wp", "extract", "-corpus", root, "-page", "12345"); code != ExitOK {
		t.Fatalf("exit code %d, stderr %q, stdout %q", code, stderr, stdout)
	}
	data, err := os.ReadFile(filepath.Join(root, wpcorpus.CensusPath(12345, 678)))
	if err != nil {
		t.Fatal(err)
	}
	c, err := wpcorpus.ParseCensus(data)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Witnesses, []string{"text", "in"}) || len(c.Pointers) != 5 || c.Pointers[0].Finding != wpcorpus.FindingBrokenAnchor || c.Pointers[1].Object == "" || len(c.PointerGaps()) != 0 {
		t.Errorf("census witnesses %q, pointers %+v", c.Witnesses, c.Pointers)
	}
}
