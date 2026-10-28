package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/wpcorpus"
)

func TestWPCensus(t *testing.T) {
	root := wpCensusCorpus(t)
	code, stdout, stderr := run("wp", "census", "report", "-corpus", root)
	if code != ExitOK || stdout != "3 articles in scope, report reports/census.md\n" {
		t.Fatalf("exit code %d, stderr %q, stdout %q", code, stderr, stdout)
	}
	report, err := os.ReadFile(filepath.Join(root, wpcorpus.CensusReportPath))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"The scope list has 3 articles. 1 of them are parsed, 0 are fetched and not parsed, 1 are only listed, and 1 are stopped.",
		"| articles in scope that are not parsed or stopped | 1 | rule Y01 and rule Y02 |",
		"| list | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |",
		"| 05 | 1 | 1 |",
		"| 11 | 1 | 1 |",
		"Of the 1 parsed articles, W-html was read for 0 and W-in for 1.",
		"No census has a gap.",
		"The censuses have 1 incoming pointer. 1 of them point to an anchor that the article does not have (`broken-anchor`)",
		"No census has the witness W-html yet.",
		"| equations | 1 | 1 | Widget |",
	} {
		if !strings.Contains(string(report), want) {
			t.Errorf("the report has no %q:\n%s", want, report)
		}
	}

	code, stdout, stderr = run("wp", "census", "diff", "-corpus", root, "12345")
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	for _, want := range []string{"Widget, page 12345, revision 678, census weak, witnesses text in", "The census has no witness W-html.", "- the redirect \"Lost widget\" to #Lost part: broken-anchor"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the diff has no %q:\n%s", want, stdout)
		}
	}
}

// wpCensusCorpus gives a corpus with a scope of three articles: Widget is
// extracted, Gadget is stopped, and List of widgets is only listed.
func wpCensusCorpus(t *testing.T) string {
	t.Helper()
	root := wpCorpus(t)
	widget, err := os.ReadFile(filepath.Join("..", "wpcorpus", "testdata", "pages", "widget.wiki"))
	if err != nil {
		t.Fatal(err)
	}
	var scope bytes.Buffer
	cs := []*wpcorpus.Candidate{
		{PageID: 12345, Title: "Widget", InScope: true, Verdict: "math", Rule: "strong"},
		{PageID: 200, Title: "Gadget", InScope: true, Verdict: "math", Rule: "strong"},
		{PageID: 300, Title: "List of widgets", InScope: true, Verdict: "math", Rule: "strong"},
	}
	if err := wpcorpus.WriteCandidates(&scope, cs, "20261001", true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, wpcorpus.ScopePath), scope.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := wpcorpus.WriteStops(root, []wpcorpus.Stop{{PageID: 200, Reason: "deleted"}}); err != nil {
		t.Fatal(err)
	}
	if err := wpcorpus.WriteContent(root, &wpcorpus.Page{ID: 12345, Revision: 678, Title: "Widget", Wikitext: string(widget)}); err != nil {
		t.Fatal(err)
	}
	if err := wpcorpus.WriteMetadata(root, &wpcorpus.Metadata{Page: 12345, Title: "Widget", Revision: 678, Type: "article", Class: "B", MSC: []string{"05C10", "05A15", "11B37"}}); err != nil {
		t.Fatal(err)
	}
	if err := wpcorpus.WritePointers(root, &wpcorpus.PointerFile{Page: 12345, Title: "Widget", Pointers: []wpcorpus.Pointer{{Via: "redirect", From: "Lost widget", Anchor: "Lost part"}}}); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := run("wp", "extract", "-corpus", root, "-page", "12345"); code != ExitOK {
		t.Fatalf("exit code %d, stderr %q, stdout %q", code, stderr, stdout)
	}

	return root
}
