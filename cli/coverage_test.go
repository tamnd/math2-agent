package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/coverage"
)

func TestCoverage(t *testing.T) {
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(filepath.Join("..", "release", "testdata", "seed"))); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"topics/msc2020.yaml":        "\"11-XX\": {label: number theory}\n\"20-XX\": {label: groups}\n\"20Axx\": {label: foundations of group theory}\n\"20A05\": {label: axioms of groups}\n\"20Dxx\": {label: finite groups}\n\"20D60\": {label: arithmetic problems on finite groups}\n",
		"build/records/zbmath.jsonl": `{"id":"1","class":"open","meta":{"msc":"20D60 11A41"}}` + "\n" + `{"id":"2","class":"open","meta":{"msc":"20A05"}}` + "\n",
	}
	for p, data := range files {
		path := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if code, stdout, stderr := run("coverage", "-root", root); code != ExitFailure || !strings.Contains(stderr, "Run m2 build first") {
		t.Errorf("m2 coverage before m2 build: exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if code, _, stderr := run("build", "-root", root); code != ExitOK {
		t.Fatalf("m2 build: exit code %d, stderr %q", code, stderr)
	}

	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Query().Get("class"))
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "m2/") {
			http.Error(w, "no agent", http.StatusForbidden)
			return
		}
		fmt.Fprintf(w, `{"status": {"nr_total_results": %d}}`, map[string]int{"11": 10, "20": 4}[r.URL.Query().Get("class")])
	}))
	defer srv.Close()
	defer func(f func(string) string) { countURL = f }(countURL)
	countURL = func(prefix string) string { return srv.URL + "?class=" + prefix }

	code, stdout, stderr := run("coverage", "-root", root, "-count", "-delay", "0")
	if code != ExitOK || !strings.Contains(stdout, "the count of 2 classes") || !strings.Contains(stdout, "6 codes, 2 records from zbmath, 2 nodes, C0 21.4% of 14 documents") {
		t.Fatalf("m2 coverage -count: exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if strings.Join(asked, " ") != "11 20" {
		t.Errorf("m2 coverage asked for the classes %v", asked)
	}
	e, err := coverage.ReadExpected(root)
	if err != nil || e.Counts["20-XX"] != 4 || e.Source != "zbmath" {
		t.Errorf("the count is %+v, %v", e, err)
	}
	md, err := os.ReadFile(filepath.Join(root, "reports", "coverage", "next", "20.md"))
	if err != nil || !strings.Contains(string(md), "| 20-XX | groups | 2 | 2 | 4 | 50.0% | 0.0% | - | - | - | 2 |") || !strings.Contains(string(md), "| 20Axx | foundations of group theory | 1 | 1 | - |") {
		t.Errorf("the summary of 20 is\n%s, %v", md, err)
	}

	// A second run with no -count uses the pinned count and makes no request.
	asked = nil
	if code, stdout, _ := run("coverage", "-root", root, "-name", "2027.04"); code != ExitOK || len(asked) != 0 || !strings.Contains(stdout, "reports/coverage/2027.04.parquet") {
		t.Errorf("m2 coverage -name 2027.04: exit code %d, stdout %q, requests %v", code, stdout, asked)
	}
	if code, _, _ := run("coverage", "-root", root, "-name", "../x"); code != ExitUsage {
		t.Errorf("m2 coverage -name ../x: exit code %d", code)
	}
}
