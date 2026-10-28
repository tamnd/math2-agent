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
