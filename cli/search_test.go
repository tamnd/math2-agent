package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/search"
)

func TestSearch(t *testing.T) {
	out := t.TempDir()
	if _, err := build.Run(context.Background(), build.Options{Root: valid, Out: out}); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("search", "-root", valid, "-out", out, "Lagrange's theorem")
	if code != ExitOK || !strings.HasPrefix(strings.Join(strings.Fields(stdout), " "), "1 Q529ES statement Lagrange's theorem") || !strings.Contains(stdout, "lexical#1") {
		t.Errorf("m2 search exits with code %d and prints:\n%s%s", code, stdout, stderr)
	}
	code, stdout, stderr = run("search", "-root", valid, "-out", out, "-json", "-k", "2", "-kind", "proof", "subgroup")
	var rs []search.Result
	if code != ExitOK || json.Unmarshal([]byte(stdout), &rs) != nil || len(rs) == 0 || len(rs) > 2 {
		t.Fatalf("m2 search -json exits with code %d and prints:\n%s%s", code, stdout, stderr)
	}
	for _, r := range rs {
		if r.Kind != "proof" {
			t.Errorf("m2 search -kind proof gives %+v", r)
		}
	}
	if code, stdout, _ = run("search", "-root", valid, "-out", out, "Q180967"); !strings.HasPrefix(strings.Join(strings.Fields(stdout), " "), "1 Q529ES") || !strings.Contains(stdout, "identifier#1") {
		t.Errorf("m2 search Q180967 exits with code %d and prints:\n%s", code, stdout)
	}
	if code, _, stderr = run("search", "-root", valid, "-out", out, "-channel", "nope", "x"); code != ExitUsage || !strings.Contains(stderr, "nope") {
		t.Errorf("an unknown channel exits with code %d: %s", code, stderr)
	}
	if code, _, _ = run("search", "-root", valid, "-out", out); code != ExitUsage {
		t.Errorf("a search with no query exits with code %d", code)
	}
	if code, _, stderr = run("search", "-root", valid, "-out", t.TempDir(), "x"); code != ExitFailure || !strings.Contains(stderr, "m2 build") {
		t.Errorf("a search with no index exits with code %d: %s", code, stderr)
	}
	// The tree has no lean/ directory, thus the index has no declarations, and the premises come only from the nodes that the goal uses.
	if code, stdout, stderr = run("search", "-root", valid, "-out", out, "-premises", "-json", "-tag", "M8XBZ4"); code != ExitOK || !strings.HasPrefix(stdout, "[") {
		t.Errorf("m2 search -premises exits with code %d and prints:\n%s%s", code, stdout, stderr)
	}
}
