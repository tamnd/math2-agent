package cli

import (
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/resource"
)

func TestResRefs(t *testing.T) {
	root := t.TempDir()
	rs := []resource.Resource{
		{ID: "8F2K1QW7ZJ", Kind: "textbook", Title: "Principles of Mathematical Analysis", Expressions: []resource.Expression{{Key: "e3", Kind: "edition"}}},
		{ID: "8F2AAAAAA3", Kind: "article", Title: "On primes", Expressions: []resource.Expression{{Key: "e1", Kind: "version"}}},
	}
	if err := resource.WriteResources(root, rs); err != nil {
		t.Fatal(err)
	}
	cs := []resource.Cite{
		{From: "8F2AAAAAA3/e1", N: 1, To: "8F2K1QW7ZJ/e3", Score: 1, Matcher: "doi"},
		{From: "8F2AAAAAA3/e1", N: 2, Parsed: &resource.Parsed{Authors: []string{"A. Weil"}, Title: "Basic number theory", Year: "1967"}},
	}
	if err := resource.WriteCites(root, cs); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("res", "refs", "-root", root, "8F2AAAAAA3")
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	for _, want := range []string{"references: 2, resolved: 1", "8F2AAAAAA3/e1 #1 -> 8F2K1QW7ZJ/e3 Principles of Mathematical Analysis (score 1.000, doi)", "#2 not resolved: A. Weil, Basic number theory, 1967", "cited by: 0"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("no %q in %q", want, stdout)
		}
	}
	code, stdout, _ = run("res", "refs", "-root", root, "8F2K1QW7ZJ/e3")
	if code != ExitOK || !strings.Contains(stdout, "references: 0, resolved: 0") || !strings.Contains(stdout, "cited by: 1\n  8F2AAAAAA3/e1 #1 On primes") {
		t.Errorf("exit code %d, stdout %q", code, stdout)
	}
	code, stdout, _ = run("res", "refs", "-root", root, "-json", "8F2K1QW7ZJ")
	if code != ExitOK || !strings.Contains(stdout, `"cited_by": [`) {
		t.Errorf("exit code %d, stdout %q", code, stdout)
	}
	if code, _, _ := run("res", "refs", "-root", root, "8F2BBBBBB6"); code != ExitFailure {
		t.Errorf("exit code %d for an unknown id", code)
	}
}
