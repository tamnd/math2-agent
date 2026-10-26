package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
)

// rebuild computes the status of the tree in root again, with the reviews and the registers, and writes it to build/, as m2 build does.
func rebuild(t *testing.T, root string) {
	t.Helper()
	nodes, err := node.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	reviews, err := status.LoadReviews(root)
	if err != nil {
		t.Fatal(err)
	}
	reviewers, err := status.LoadReviewers(root)
	if err != nil {
		t.Fatal(err)
	}
	authors, err := status.LoadAuthors(root)
	if err != nil {
		t.Fatal(err)
	}
	rec := lean.Record{Decl: "Nat.zero_add", Kind: "theorem", TypeHash: "blake3:zeroadd"}
	st := status.Compute(status.Input{Nodes: nodes, Records: map[string]lean.Record{rec.Decl: rec}, Reviews: reviews, Reviewers: reviewers, Authors: authors})
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "build", build.StatusFile), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReviewSample(t *testing.T) {
	root := reviewTree(t)
	if code, stdout, stderr := run("review", "sample", "-root", root, "-month", "2026-11"); code != ExitOK || !strings.Contains(stdout, "the sample of 2026-11 is empty") {
		t.Errorf("no binding at F2: exit code %d, %s%s", code, stdout, stderr)
	}
	if code, _, stderr := run("review", "-root", root, "-reviewer", "github:reviewer", "-verdict", "faithful", "-date", "2026-10-01", "1B0000"); code != ExitOK {
		t.Fatalf("m2 review: %s", stderr)
	}
	reg := filepath.Join(root, "registers", "reviewers.reg")
	if err := os.WriteFile(reg, []byte("github:reviewer 2026-01-01 03 -\ngithub:second 2026-02-01 03 -\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rebuild(t, root)

	code, stdout, stderr := run("review", "sample", "-root", root, "-month", "2026-11", "-seed", "s1")
	if code != ExitOK || !strings.Contains(stdout, "The sample of 2026-11 with the seed s1 has 1 binding.\n\nTAG     LEVEL  DECLARATION   REVIEWER\n1B0000  F2     Nat.zero_add  github:second\n") || !strings.Contains(stdout, "Give -write") {
		t.Fatalf("exit code %d, %s%s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "registers", "samples.reg")); err == nil {
		t.Error("m2 review sample with no -write wrote the register")
	}
	if code, stdout, stderr := run("review", "sample", "-root", root, "-month", "2026-11", "-seed", "s1", "-write"); code != ExitOK || !strings.Contains(stdout, "m2 review -blind TAG") {
		t.Fatalf("-write: exit code %d, %s%s", code, stdout, stderr)
	}
	if code, _, stderr := run("review", "sample", "-root", root, "-month", "2026-11", "-write"); code != ExitFailure || !strings.Contains(stderr, "has the sample of 2026-11") {
		t.Errorf("a second sample of the month: exit code %d, %s", code, stderr)
	}
	if code, stdout, _ := run("review", "measure", "-root", root); code != ExitOK || !strings.Contains(stdout, "1 sample, 0 re-reviews done, 0 disagree.") || !strings.Contains(stdout, "Open\n  2026-11 1B0000 Nat.zero_add: github:second\n") {
		t.Errorf("an open re-review: exit code %d, %s", code, stdout)
	}

	// The packet of a blind re-review has no reviews.
	code, stdout, _ = run("review", "-root", root, "-blind", "-json", "1B0000")
	var p struct {
		Bindings []struct {
			Reviews []any `json:"reviews"`
		} `json:"bindings"`
	}
	if err := json.Unmarshal([]byte(stdout), &p); code != ExitOK || err != nil || len(p.Bindings) != 1 || len(p.Bindings[0].Reviews) != 0 {
		t.Errorf("-blind: exit code %d, %v, %s", code, err, stdout)
	}
	if code, _, stderr := run("review", "-root", root, "-reviewer", "github:second", "-verdict", "not-faithful", "-class", "M-junk", "-date", "2026-11-03", "1B0000"); code != ExitOK {
		t.Fatalf("the re-review: %s", stderr)
	}
	code, stdout, _ = run("review", "measure", "-root", root, "2026-11")
	if code != ExitOK || !strings.Contains(stdout, "1 sample, 1 re-review done, 1 disagrees.") || !strings.Contains(stdout, "F2     all   1      1     1         100.0%") || !strings.Contains(stdout, "Disagree\n  2026-11 1B0000 Nat.zero_add: r-1B0000-02 by github:second, M-junk\n") {
		t.Errorf("a re-review that disagrees: exit code %d, %s", code, stdout)
	}
	if _, stdout, _ := run("review", "measure", "-root", root, "2026-12"); stdout != "registers/samples.reg has no sample of 2026-12.\n" {
		t.Errorf("a month with no sample: %s", stdout)
	}
	code, stdout, _ = run("review", "measure", "-root", root, "-json")
	var m struct {
		Results []struct {
			State string `json:"state"`
		} `json:"results"`
		Rates []struct {
			Area string `json:"area"`
		} `json:"rates"`
	}
	if err := json.Unmarshal([]byte(stdout), &m); code != ExitOK || err != nil || len(m.Results) != 1 || m.Results[0].State != "disagrees" || len(m.Rates) != 2 {
		t.Errorf("-json: exit code %d, %v, %s", code, err, stdout)
	}
	if code, _, _ := run("review", "measure", "a", "b"); code != ExitUsage {
		t.Errorf("two months: exit code %d", code)
	}
	if code, _, _ := run("review", "sample", "-root", root, "-month", "2026-13"); code != ExitFailure {
		t.Errorf("a month that does not exist: exit code %d", code)
	}
}
