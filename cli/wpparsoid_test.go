package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/wpcorpus"
)

func TestWPParsoid(t *testing.T) {
	root := t.TempDir()
	for _, m := range []*wpcorpus.Metadata{
		{Page: 12345, Title: "Widget", Revision: 678, Timestamp: "2026-01-02T03:04:05Z", Type: "article", Importance: "Low"},
		{Page: 300, Title: "Small widget", Revision: 30, Timestamp: "2026-01-02T03:04:05Z", Type: "article", Importance: "Top"},
	} {
		if err := wpcorpus.WriteMetadata(root, m); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.UserAgent(), "m2/") {
			http.Error(w, "no user agent", http.StatusForbidden)
			return
		}
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		_, _ = w.Write([]byte("<html>" + r.URL.Path + "</html>"))
	}))
	defer srv.Close()
	saved := parsoidBase
	parsoidBase = srv.URL
	t.Cleanup(func() { parsoidBase = saved })

	code, stdout, stderr := run("wp", "parsoid", "-corpus", root)
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	// The order of WP2.9 puts the article of importance Top first.
	if len(paths) != 2 || paths[0] != "/page/html/Small_widget/30" || paths[1] != "/page/html/Widget/678" {
		t.Errorf("the paths are %v", paths)
	}
	if !strings.Contains(stdout, "2 articles, 2 fetched, 0 failed, 2 requests") {
		t.Errorf("stdout is:\n%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(root, wpcorpus.ParsoidPath(12345, 678))); err != nil {
		t.Error(err)
	}
	runs, err := filepath.Glob(filepath.Join(root, wpcorpus.RunsDir, "*-wp-parsoid.md"))
	if err != nil || len(runs) != 1 {
		t.Fatalf("the run records are %v, %v", runs, err)
	}
	data, err := os.ReadFile(runs[0])
	if err != nil || !strings.Contains(string(data), "requests: 2\n") || !strings.Contains(string(data), "command: m2 wp parsoid\n") {
		t.Errorf("the run record is:\n%s", data)
	}

	// A revision does not change, so a second run fetches nothing.
	if code, stdout, _ := run("wp", "parsoid", "-corpus", root); code != ExitOK || len(paths) != 2 || !strings.Contains(stdout, "0 articles, 0 fetched") {
		t.Errorf("the second run gives %d and fetches %v:\n%s", code, paths, stdout)
	}
}

// TestWPParsoidWorkers checks that the workers fetch at the same time and
// that the starts of the requests stay at least 300 ms apart.
func TestWPParsoidWorkers(t *testing.T) {
	root := t.TempDir()
	for i := range 3 {
		m := &wpcorpus.Metadata{Page: 100 + i, Title: fmt.Sprintf("Widget %d", i), Revision: 5, Timestamp: "2026-01-02T03:04:05Z", Type: "article"}
		if err := wpcorpus.WriteMetadata(root, m); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	var starts []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		starts = append(starts, time.Now())
		mu.Unlock()
		time.Sleep(time.Second)
		_, _ = w.Write([]byte("<html></html>"))
	}))
	defer srv.Close()
	saved := parsoidBase
	parsoidBase = srv.URL
	t.Cleanup(func() { parsoidBase = saved })

	begin := time.Now()
	code, stdout, stderr := run("wp", "parsoid", "-corpus", root, "-workers", "3")
	if code != ExitOK || !strings.Contains(stdout, "3 articles, 3 fetched, 0 failed") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	// One worker needs 3 s. Three workers need about 1.6 s.
	if d := time.Since(begin); d > 2500*time.Millisecond {
		t.Errorf("the run took %v", d)
	}
	slices.SortFunc(starts, func(a, b time.Time) int { return a.Compare(b) })
	for i := 1; i < len(starts); i++ {
		if gap := starts[i].Sub(starts[i-1]); gap < 290*time.Millisecond {
			t.Errorf("request %d started %v after the last one", i, gap)
		}
	}
}
