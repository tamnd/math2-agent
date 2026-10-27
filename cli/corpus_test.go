package cli

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/queue"
)

// treeFiles returns the files of the tree at root, by their paths.
func treeFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path) // #nosec G304 -- a file of the test tree.
		out[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCorpusImportPlan(t *testing.T) {
	body := "w2 two\nw1 one\n"
	var agents []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agents = append(agents, r.UserAgent())
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	root, cache, dir := t.TempDir(), t.TempDir(), t.TempDir()
	plan, summary := filepath.Join(dir, "plan.yaml"), filepath.Join(dir, "summary.md")
	if err := os.WriteFile(plan, []byte("imports:\n  - source: words\n    url: "+srv.URL+"/words.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("corpus", "import", "-plan", plan, "-root", root, "-commit", "c", "-cache", cache, "-summary", summary)
	if code != ExitOK || !strings.Contains(stdout, "words: 2 records from a new snapshot of") || !strings.Contains(stdout, "rows: 2, new 2") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	data, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	if s := string(data); !strings.Contains(s, "| words | new, ") || !strings.Contains(s, "| 2 | 2 | 2 |") || !strings.Contains(s, "The run added 2 mentions and 2 rows. The inventory has 2 rows, and the open count is 2 (it was 0).") {
		t.Errorf("the summary is:\n%s", s)
	}
	if len(agents) != 1 || !strings.HasPrefix(agents[0], "m2/") || !strings.Contains(agents[0], "(+https://github.com/tamnd/math2-agent)") {
		t.Errorf("the User-Agent headers are %q", agents)
	}

	// A run with no change in the source writes no change in the tree.
	first := treeFiles(t, root)
	code, stdout, stderr = run("corpus", "import", "-plan", plan, "-root", root, "-commit", "d", "-cache", cache)
	if code != ExitOK || !strings.Contains(stdout, "from a pinned snapshot") || !strings.Contains(stdout, "The run added 0 mentions and 0 rows.") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	again := treeFiles(t, root)
	if len(again) != len(first) {
		t.Errorf("the tree had %d files and has %d", len(first), len(again))
	}
	for path, data := range first {
		if again[path] != data {
			t.Errorf("the run with no change changed %s", path)
		}
	}

	// A new item in the source gives one new mention and one new row.
	body = "w2 two\nw1 one\nw3 three\n"
	if code, stdout, stderr = run("corpus", "import", "-plan", plan, "-root", root, "-commit", "e", "-cache", cache); code != ExitOK || !strings.Contains(stdout, "| words | new, ") || !strings.Contains(stdout, "The run added 1 mentions and 1 rows. The inventory has 3 rows, and the open count is 3 (it was 2).") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestCorpusImportPlanErrors(t *testing.T) {
	root, dir := t.TempDir(), t.TempDir()
	plan := filepath.Join(dir, "plan.yaml")
	if err := os.WriteFile(plan, []byte("imports:\n  - source: nothing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := run("corpus", "import", "-plan", plan, "-root", root, "-commit", "c"); code != ExitFailure || !strings.Contains(stderr, "the source nothing has no adapter") {
		t.Errorf("a source with no adapter gave %d, %q", code, stderr)
	}
	for _, args := range [][]string{
		{"corpus", "import", "-plan", plan, "-root", root, "words"},
		{"corpus", "import", "-root", root, "-summary", "x.md"},
		{"corpus", "import", "-root", root, "-cache", dir},
	} {
		if code, _, _ := run(args...); code != ExitUsage {
			t.Errorf("%q gave %d, want %d", args, code, ExitUsage)
		}
	}
}

func TestCorpusImport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "w2 two\nw1 one\n")
	}))
	defer srv.Close()
	root, cache := t.TempDir(), t.TempDir()
	if code, _, stderr := run("corpus", "import", "-root", root, "-commit", "c"); code != ExitFailure || !strings.Contains(stderr, "Run m2 ingest first") {
		t.Fatalf("an empty tree gave %d, %q", code, stderr)
	}
	if code, _, stderr := run("ingest", "-root", root, "-cache", cache, "-url", srv.URL+"/words.txt", "-date", "2026-10-25", "words"); code != ExitOK {
		t.Fatalf("m2 ingest: %s", stderr)
	}
	code, stdout, stderr := run("corpus", "import", "-root", root, "-commit", "c")
	if code != ExitOK || !strings.Contains(stdout, "words: 2 mentions") || !strings.Contains(stdout, "rows: 2, new 2") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	// A second import of the same records adds no row.
	if _, stdout, _ = run("corpus", "import", "-root", root, "-commit", "c", "words"); !strings.Contains(stdout, "rows: 2, new 0") {
		t.Fatalf("the second import gave %q", stdout)
	}
	// m2 corpus cluster reads only the mentions, so it gives the same rows.
	if code, stdout, stderr := run("corpus", "cluster", "-root", root, "-commit", "c"); code != ExitOK || !strings.Contains(stdout, "rows: 2, new 0, with new mentions 0, merged 0") {
		t.Fatalf("m2 corpus cluster gave %d, %q, %q", code, stdout, stderr)
	}
	if code, _, stderr := run("corpus", "gold", "-root", root); code != ExitFailure || !strings.Contains(stderr, "no gold set") {
		t.Fatalf("a tree with no gold set gave %d, %q", code, stderr)
	}
	ms, err := inventory.ReadMentions(root)
	if err != nil || len(ms) != 2 {
		t.Fatalf("the mentions are %v, %v", ms, err)
	}
	gold := filepath.Join(root, filepath.FromSlash(inventory.GoldFile))
	if err := os.MkdirAll(filepath.Dir(gold), 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"a":"` + ms[0].ID + `","b":"` + ms[1].ID + `","same":false,"layer":"textbook","by":"tamnd","on":"2026-10-27"}` + "\n"
	if err := os.WriteFile(gold, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := run("corpus", "gold", "-root", root, "-write"); code != ExitOK || !strings.Contains(stdout, "identifiers  textbook  1      -          -") {
		t.Fatalf("m2 corpus gold gave %d, %q, %q", code, stdout, stderr)
	}
	data, err := os.ReadFile(filepath.Join(root, "reports", "inventory", "clustering.json"))
	var report struct {
		Gold   int
		Scores []inventory.Score
	}
	if err != nil || json.Unmarshal(data, &report) != nil || report.Gold != 1 || len(report.Scores) != 6 || report.Scores[5].TrueNeg != 1 {
		t.Fatalf("the report is %s, %v", data, err)
	}
	// The words have no MSC codes, so no area has an estimate.
	if code, stdout, stderr := run("corpus", "recall", "-root", root, "-json", "-write"); code != ExitOK || !strings.Contains(stdout, `"areas": null`) {
		t.Fatalf("m2 corpus recall gave %d, %q, %q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "reports", "inventory", "recall.json")); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = run("corpus", "status", "-root", root, "-json")
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	var s inventory.Summary
	if err := json.Unmarshal([]byte(stdout), &s); err != nil || s.Rows != 2 || s.Open != 2 || s.Sources["words"] != 2 {
		t.Fatalf("the status is %+v, %v", s, err)
	}
	code, stdout, _ = run("corpus", "queue", "-root", root, "decide")
	if code != ExitOK || !strings.Contains(stdout, "decide: 2 items, role curate") {
		t.Fatalf("m2 corpus queue gave %d, %q", code, stdout)
	}
	code, stdout, _ = run("corpus", "plan", "-root", root, "-n", "1", "-json", "decide")
	var p queue.Plan
	if err := json.Unmarshal([]byte(stdout), &p); code != ExitOK || err != nil || p.Jobs != 1 || p.ReviewHigh != 1.0/60 {
		t.Fatalf("m2 corpus plan gave %d, %+v, %v", code, p, err)
	}
	if code, _, _ := run("corpus", "queue", "-root", root, "prove"); code != ExitUsage {
		t.Fatalf("an unknown queue gave %d", code)
	}
	if code, _, _ := run("corpus", "nothing"); code != ExitUsage {
		t.Fatalf("an unknown subcommand gave %d", code)
	}
}

func TestCorpusStatusEmpty(t *testing.T) {
	code, stdout, stderr := run("corpus", "status", "-root", t.TempDir())
	if code != ExitOK || !strings.Contains(stdout, "open      0") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestCorpusImportOfArxivNeedsTheCheckout(t *testing.T) {
	root := t.TempDir()
	if code, _, stderr := run("corpus", "import", "-root", root, "-commit", "c", "-source", "arxiv"); code != ExitUsage || !strings.Contains(stderr, "-corpus") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if code, _, stderr := run("corpus", "import", "-root", root, "-commit", "c", "-source", "stacks", "-corpus", root); code != ExitUsage || !strings.Contains(stderr, "no importer") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestCorpusClusterOfArxiv(t *testing.T) {
	root := t.TempDir()
	body := []inventory.Mention{
		{ID: "ax://paper/2401.00001#T1", Source: "arxiv", NativeID: "2401.00001#T1", Kind: "statement", Hash: "sha256:aa", MSC: []string{"47H10"}, Class: "record", By: "m2/arxiv"},
		{ID: "ax://paper/2401.00002#T1", Source: "arxiv", NativeID: "2401.00002#T1", Kind: "statement", Hash: "sha256:aa", MSC: []string{"47H09"}, Class: "record", By: "m2/arxiv"},
	}
	if err := inventory.WriteMentions(root, "arxiv", body); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := run("corpus", "cluster", "-root", root, "-commit", "c", "-source", "arxiv", "-msc", "47"); code != ExitOK || !strings.Contains(stdout, "joined by equal hashes 1") || !strings.Contains(stdout, "name candidates 0") {
		t.Fatalf("m2 corpus cluster gave %d, %q, %q", code, stdout, stderr)
	}
	data, err := os.ReadFile(filepath.Join(root, "reports", "inventory-arxiv.md"))
	if err != nil || !strings.Contains(string(data), "2 arXiv mentions that are not attached to another item, in 1 row. 1 row holds the mentions of more than one paper") {
		t.Fatalf("the report is %s, %v", data, err)
	}
	if code, _, _ := run("corpus", "cluster", "-root", root, "-commit", "c", "-msc", "47"); code != ExitUsage {
		t.Errorf("-msc with no -source gave %d", code)
	}
}
