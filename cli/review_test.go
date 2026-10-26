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

// reviewTree writes a tree with one bound statement, and the files of a build of it in build/. It returns the root.
func reviewTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"nodes/1B/1B0000.md":      "---\ntag: 1B0000\nkind: statement\nname: Zero add\nformal:\n  - {system: lean4, library: mathlib, decl: Nat.zero_add, match: exact}\n---\n\n## Statement\n\nFor each natural number n, 0 + n = n.\n",
		"registers/reviewers.reg": "github:reviewer 2026-01-01 03 -\n",
		"registers/bindings.reg":  "1B0000 lean4 - Nat.zero_add bind abc github:author\n",
	}
	for name, text := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rec := lean.Record{Decl: "Nat.zero_add", Kind: "theorem", TypeHash: "blake3:zeroadd", StatementPP: "∀ (n : ℕ), 0 + n = n", StatementPPAll: "∀ (n : Nat), @Eq.{1} Nat (@HAdd.hAdd.{0, 0, 0} Nat Nat Nat (@instHAdd.{0} Nat instAddNat) (@OfNat.ofNat.{0} Nat 0 (instOfNatNat 0)) n) n", UsesConstantsInStatement: []string{"Eq", "HAdd.hAdd", "instHAdd", "instAddNat", "OfNat.ofNat", "instOfNatNat"}}
	nodes, err := node.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	st := status.Compute(status.Input{Nodes: nodes, Records: map[string]lean.Record{rec.Decl: rec}})
	for name, v := range map[string]any{build.StatusFile: st, build.ExtractFile: build.Extraction{Records: []lean.Record{rec}}} {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "build", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestReviewPacket(t *testing.T) {
	root := reviewTree(t)
	code, stdout, stderr := run("review", "-root", root, "-width", "80", "1B0000")
	if code != ExitOK {
		t.Fatalf("exit code %d: %s", code, stderr)
	}
	for _, want := range []string{"1B0000 statement: Zero add", "you review      Nat.zero_add", "For each natural number n, 0 + n = n.", "| ∀ (n : ℕ), 0 + n = n", "Battery         not run"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the packet has no %q:\n%s", want, stdout)
		}
	}
	code, stdout, _ = run("review", "-root", root, "-json", "1B0000")
	var p struct {
		Tag      string `json:"tag"`
		Bindings []struct {
			TypeHash string `json:"type_hash"`
		} `json:"bindings"`
	}
	if err := json.Unmarshal([]byte(stdout), &p); code != ExitOK || err != nil || p.Tag != "1B0000" || len(p.Bindings) != 1 || p.Bindings[0].TypeHash != "blake3:zeroadd" {
		t.Errorf("m2 review -json: exit code %d, %v, %s", code, err, stdout)
	}
}

func TestReviewWrite(t *testing.T) {
	root := reviewTree(t)
	code, stdout, stderr := run("review", "-root", root, "-reviewer", "github:reviewer", "-verdict", "faithful", "-date", "2026-11-01", "-dry-run", "1B0000")
	if code != ExitOK || !strings.Contains(stdout, "The review is valid") {
		t.Fatalf("dry run: exit code %d, %s %s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "reviews")); err == nil {
		t.Error("a dry run wrote a review")
	}
	code, stdout, stderr = run("review", "-root", root, "-reviewer", "github:reviewer", "-verdict", "faithful-with-conventions", "-convention", "ℕ starts at 0", "-date", "2026-11-01", "-json", "1B0000")
	var r status.Review
	if err := json.Unmarshal([]byte(stdout), &r); code != ExitOK || err != nil || r.ID != "r-1B0000-01" || r.Decl != "Nat.zero_add" || len(r.Conventions) != 1 {
		t.Fatalf("exit code %d, %v, %s %s", code, err, stdout, stderr)
	}
	reviews, err := status.LoadReviews(root)
	if err != nil || reviews["r-1B0000-01"].TypeHash != "blake3:zeroadd" {
		t.Errorf("the tree has %+v, %v", reviews, err)
	}
}

func TestReviewErrors(t *testing.T) {
	root := reviewTree(t)
	tests := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"review"}, ExitUsage, "give one tag"},
		{[]string{"review", "-root", root, "-dry-run", "1B0000"}, ExitUsage, "need a verdict"},
		{[]string{"review", "-root", root, "-out", t.TempDir(), "1B0000"}, ExitFailure, "Run m2 build first"},
		{[]string{"review", "-root", root, "ZZZZZZ"}, ExitFailure, "no node has the tag ZZZZZZ"},
		{[]string{"review", "-root", root, "-decl", "Nat.add_zero", "1B0000"}, ExitFailure, "no Lean binding of Nat.add_zero"},
		{[]string{"review", "-root", root, "-reviewer", "github:author", "-verdict", "faithful", "1B0000"}, ExitFailure, "cannot review it"},
		{[]string{"review", "-root", root, "-reviewer", "m2-agent", "-verdict", "faithful", "1B0000"}, ExitFailure, "not a person"},
		{[]string{"review", "-root", root, "-reviewer", "github:reviewer", "-verdict", "not-faithful", "1B0000"}, ExitFailure, "needs one class"},
	}
	for _, tt := range tests {
		code, _, stderr := run(tt.args...)
		if code != tt.code || !strings.Contains(stderr, tt.want) {
			t.Errorf("m2 %s: exit code %d, stderr %q, want %d and %q", strings.Join(tt.args, " "), code, stderr, tt.code, tt.want)
		}
	}
}

func TestReviewExport(t *testing.T) {
	root := reviewTree(t)
	if code, stdout, _ := run("review", "export", "-root", root); code != ExitOK || stdout != "The tree has no reviews.\n" {
		t.Errorf("a tree with no reviews: exit code %d, %s", code, stdout)
	}
	if code, _, stderr := run("review", "-root", root, "-reviewer", "github:reviewer", "-verdict", "faithful", "-date", "2026-11-01", "1B0000"); code != ExitOK {
		t.Fatalf("m2 review: %s", stderr)
	}
	code, stdout, stderr := run("review", "export", "-root", root, "github:reviewer")
	want := "github:reviewer: 1 fidelity review of Lean formalizations for math2, on 2026-11-01.\nhttps://tamnd.github.io/math2/reviewers/github-reviewer/\n\n2026-11-01  r-1B0000-01  Zero add (m2://1B0000), Nat.zero_add: faithful\n"
	if code != ExitOK || stdout != want {
		t.Errorf("exit code %d, %s%s", code, stdout, stderr)
	}
	code, stdout, _ = run("review", "export", "-root", root, "-json")
	var exports []struct {
		Reviewer string `json:"reviewer"`
		First    string `json:"first"`
		Reviews  []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"reviews"`
	}
	if err := json.Unmarshal([]byte(stdout), &exports); code != ExitOK || err != nil || len(exports) != 1 || exports[0].First != "2026-01-01" || exports[0].Reviews[0].Name != "Zero add" {
		t.Errorf("-json: exit code %d, %v, %s", code, err, stdout)
	}
	if code, _, stderr := run("review", "export", "-root", root, "github:other"); code != ExitFailure || !strings.Contains(stderr, "no review by github:other") {
		t.Errorf("a reviewer with no review: exit code %d, %s", code, stderr)
	}
	if code, _, _ := run("review", "export", "a", "b"); code != ExitUsage {
		t.Errorf("two reviewers: exit code %d", code)
	}
}

func TestReviewQueue(t *testing.T) {
	root := reviewTree(t)
	if code, stdout, stderr := run("review", "queue", "-root", root); code != ExitOK || stdout != "No binding waits for a first review.\n" {
		t.Errorf("a binding at F0: exit code %d, %s%s", code, stdout, stderr)
	}
	// The battery passes, thus the binding is at F1.
	nodes, err := node.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	rec := lean.Record{Decl: "Nat.zero_add", Kind: "theorem", TypeHash: "blake3:zeroadd"}
	st := status.Compute(status.Input{Nodes: nodes, Records: map[string]lean.Record{rec.Decl: rec}, Battery: map[status.Key]bool{{Tag: "1B0000", Decl: "Nat.zero_add"}: true}})
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "build", build.StatusFile), data, 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("review", "queue", "-root", root, "-reviewer", "github:reviewer")
	if code != ExitOK || !strings.HasPrefix(stdout, "1 binding waits for a first review.\n\nTAG     USED BY  DECLARATION   NAME\n1B0000  0        Nat.zero_add  Zero add\n") {
		t.Errorf("exit code %d, %s%s", code, stdout, stderr)
	}
	if _, stdout, _ := run("review", "queue", "-root", root, "-reviewer", "github:author"); !strings.HasPrefix(stdout, "No binding") {
		t.Errorf("the author of the binding: %s", stdout)
	}
	if _, stdout, _ := run("review", "queue", "-root", root, "-second"); stdout != "No binding waits for a second review.\n" {
		t.Errorf("-second: %s", stdout)
	}
	code, stdout, _ = run("review", "queue", "-root", root, "-json")
	var tasks []struct {
		Tag      string `json:"tag"`
		Fidelity string `json:"fidelity"`
	}
	if err := json.Unmarshal([]byte(stdout), &tasks); code != ExitOK || err != nil || len(tasks) != 1 || tasks[0].Fidelity != "F1" {
		t.Errorf("-json: exit code %d, %v, %s", code, err, stdout)
	}
	if _, stdout, _ := run("review", "queue", "-root", root, "-area", "20", "-json"); stdout != "[]\n" {
		t.Errorf("an area with no binding: %s", stdout)
	}
	if code, _, _ := run("review", "queue", "-root", root, "x"); code != ExitUsage {
		t.Errorf("an argument: exit code %d", code)
	}
	if code, _, stderr := run("review", "queue", "-root", t.TempDir()); code != ExitFailure || !strings.Contains(stderr, "Run m2 build first") {
		t.Errorf("no build: exit code %d, %s", code, stderr)
	}
}
