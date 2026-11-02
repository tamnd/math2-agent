package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/wpcorpus"
)

// mark sets marked to true in the file of a page of a gold set, as a person
// does after the check.
func mark(t *testing.T, root, set string, page int, extra string) {
	t.Helper()
	path := filepath.Join(root, wpcorpus.GoldPath(set, wpcorpus.GoldVersion, page))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.Replace(string(data), "marked: false\n", "marked: true\nmarker: tester\n", 1)
	s = strings.Replace(s, "items:\n", "items:\n"+extra, 1)
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestWPGoldYM(t *testing.T) {
	root := wpCensusCorpus(t)

	code, stdout, stderr := run("wp", "gold", "draw", "y", "-corpus", root)
	if code != ExitOK || !strings.HasPrefix(stdout, "1 articles and ") {
		t.Fatalf("draw y: exit code %d, stderr %q, stdout %q", code, stderr, stdout)
	}
	if code, _, stderr := run("wp", "gold", "draw", "y", "-corpus", root); code != ExitFailure || !strings.Contains(stderr, "exists") {
		t.Errorf("a second draw: exit code %d, stderr %q", code, stderr)
	}
	code, stdout, stderr = run("wp", "gold", "measure", "y", "-corpus", root)
	if code != ExitOK || stdout != "0 marked, cue recall 0.000, cue precision 0.000, prose recall 0.000\n" {
		t.Fatalf("measure y before the marks: exit code %d, stderr %q, stdout %q", code, stderr, stdout)
	}
	// The person adds one item in running text that the extractor does not
	// find.
	mark(t, root, wpcorpus.GoldSetY, 12345, "  - {kind: definition, name: gizmo, span: [1, 5]}\n")
	code, stdout, stderr = run("wp", "gold", "measure", "y", "-corpus", root)
	if code != ExitOK || stdout != "1 marked, cue recall 1.000, cue precision 1.000, prose recall 0.000\n" {
		t.Fatalf("measure y: exit code %d, stderr %q, stdout %q", code, stderr, stdout)
	}
	report, err := os.ReadFile(filepath.Join(root, "reports", "gold-y.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"| prose recall | 0 | 1 | 0.000 | 0.900 |", "1.000 | 0.980 |", "| concept | 0 | 1 | 0.000 |", "- Widget: definition \"gizmo\" at 1"} {
		if !strings.Contains(string(report), want) {
			t.Errorf("gold-y.md has no %q:\n%s", want, report)
		}
	}

	code, stdout, stderr = run("wp", "gold", "draw", "m", "-corpus", root)
	if code != ExitOK || !strings.HasPrefix(stdout, "1 articles and ") {
		t.Fatalf("draw m: exit code %d, stderr %q, stdout %q", code, stderr, stdout)
	}
	mark(t, root, wpcorpus.GoldSetM, 12345, "")
	code, stdout, stderr = run("wp", "gold", "measure", "m", "-corpus", root)
	if code != ExitOK || !strings.HasPrefix(stdout, "1 marked, math 1.000") {
		t.Fatalf("measure m: exit code %d, stderr %q, stdout %q", code, stderr, stdout)
	}

	if code, _, _ := run("wp", "gold", "draw", "x", "-corpus", root); code != ExitUsage {
		t.Errorf("the set x: exit code %d", code)
	}
}

func TestWPGoldL(t *testing.T) {
	root := edgeCorpus(t)
	for _, md := range []*wpcorpus.Metadata{{Page: 100, Title: "Widget", Revision: 1, Type: "article"}, {Page: 200, Title: "Gadget", Revision: 2, Type: "article"}} {
		if err := wpcorpus.WriteMetadata(root, md); err != nil {
			t.Fatal(err)
		}
	}
	if code, stdout, stderr := run("wp", "edges", "-corpus", root); code != ExitOK {
		t.Fatalf("edges: exit code %d, stderr %q, stdout %q", code, stderr, stdout)
	}
	code, stdout, stderr := run("wp", "gold", "draw", "l", "-corpus", root)
	if code != ExitOK || !strings.HasPrefix(stdout, "2 articles and 3 draft links in ") {
		t.Fatalf("draw l: exit code %d, stderr %q, stdout %q", code, stderr, stdout)
	}
	path := filepath.Join(root, wpcorpus.GoldPath(wpcorpus.GoldSetL, wpcorpus.GoldVersion, 100))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "to: Sprocket\n") || !strings.Contains(string(data), "the predicate of each link in the items") {
		t.Errorf("the file of Widget:\n%s", data)
	}
	// The person finds that the link to Sprocket is related, not uses.
	s := strings.Replace(string(data), "to: Sprocket\n    predicate: uses\n", "to: Sprocket\n    predicate: related\n", 1)
	if s == string(data) {
		t.Fatalf("no link to Sprocket with the predicate uses:\n%s", data)
	}
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
	mark(t, root, wpcorpus.GoldSetL, 100, "")
	code, stdout, stderr = run("wp", "gold", "measure", "l", "-corpus", root)
	if code != ExitOK || stdout != "1 marked, link-in-item 0.500\n" {
		t.Fatalf("measure l: exit code %d, stderr %q, stdout %q", code, stderr, stdout)
	}
	report, err := os.ReadFile(filepath.Join(root, "reports", "gold-l.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"| link-in-item | 1 | 2 | 0.500 | 0.900 |", "| uses | 1 | 2 | 0.500 |", "gives uses, and the gold predicate is related"} {
		if !strings.Contains(string(report), want) {
			t.Errorf("gold-l.md has no %q:\n%s", want, report)
		}
	}
}
