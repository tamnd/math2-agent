package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/queue"
)

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
	if err != nil || json.Unmarshal(data, &report) != nil || report.Gold != 1 || len(report.Scores) != 4 || report.Scores[3].TrueNeg != 1 {
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
