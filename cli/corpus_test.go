package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
