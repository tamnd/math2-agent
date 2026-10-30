package notes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readRepos(t *testing.T) []Repo {
	t.Helper()
	data, err := os.ReadFile("testdata/search.json")
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Items []Repo `json:"items"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		t.Fatal(err)
	}
	return res.Items
}

func TestWhy(t *testing.T) {
	want := map[string]string{
		"dalcde/cam-notes":               "no licence",
		"boazbk/crypto":                  "no subject",
		"alice/algebraic-topology-notes": "",
		"bob/algebraic-topology-notes":   "fork",
		"carol/pde":                      "no notes",
		"dave/other-notes":               "",
	}
	for _, r := range readRepos(t) {
		if got := Why(r); got != want[r.FullName] {
			t.Errorf("Why(%s) = %q, want %q", r.FullName, got, want[r.FullName])
		}
	}
}

func TestKeep(t *testing.T) {
	repos := readRepos(t)
	c, ok := Keep(repos[2], Queries[0], "2026-10-31")
	if !ok {
		t.Fatal("the scan drops alice/algebraic-topology-notes")
	}
	want := Candidate{Repo: "alice/algebraic-topology-notes", URL: "https://github.com/alice/algebraic-topology-notes", Licence: "CC-BY-4.0", Language: "TeX",
		Description: "Lecture notes for Math 231, algebraic topology", Topics: "homology", Stars: 12, Date: "2025-03-02", Subject: "math",
		Query: Queries[0], Found: "2026-10-31", Rules: Rules, Status: "candidate"}
	if c != want {
		t.Errorf("candidate = %+v\nwant %+v", c, want)
	}
	if c, _ := Keep(repos[5], Queries[0], "2026-10-31"); c.Licence != "NOASSERTION" || c.Subject != "probability" {
		t.Errorf("candidate = %+v, want the licence NOASSERTION and the subject probability", c)
	}
	if _, ok := Keep(repos[0], Queries[0], "2026-10-31"); ok {
		t.Error("the scan keeps a repository with no licence")
	}
}

func TestScan(t *testing.T) {
	page, err := os.ReadFile("testdata/search.json")
	if err != nil {
		t.Fatal(err)
	}
	var queries []string
	limited := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); ua != "m2/test (+https://github.com/tamnd/math2-agent)" || r.Header.Get("Authorization") != "" {
			t.Errorf("User-Agent = %q, Authorization = %q", ua, r.Header.Get("Authorization"))
		}
		q := r.URL.Query()
		if q.Get("per_page") != "100" || q.Get("page") != "1" {
			t.Errorf("query = %v", q)
		}
		if !limited {
			limited = true
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		queries = append(queries, q.Get("q"))
		_, _ = w.Write(page)
	}))
	defer srv.Close()
	var waits []time.Duration
	s := &Scanner{Base: srv.URL, UserAgent: "m2/test (+https://github.com/tamnd/math2-agent)", Gap: Gap,
		wait: func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }}
	cs, counts, err := s.Scan(context.Background(), Queries[:2], "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(queries, "|") != strings.Join(Queries[:2], "|") {
		t.Errorf("queries = %q", queries)
	}
	if len(cs) != 2 || cs[0].Repo != "alice/algebraic-topology-notes" || cs[1].Repo != "dave/other-notes" || cs[0].Query != Queries[0] {
		t.Errorf("candidates = %+v", cs)
	}
	if counts.Requests != 2 || counts.Seen != 12 || counts.Kept != 2 || counts.Dropped["no licence"] != 2 || counts.Dropped["fork"] != 2 {
		t.Errorf("counts = %+v", counts)
	}
	// The first wait is the Retry-After of the rate limit. Each later request waits about Gap.
	if len(waits) != 3 || waits[0] != 30*time.Second || waits[1] < Gap-time.Second || waits[2] < Gap-time.Second {
		t.Errorf("waits = %v", waits)
	}
}

func TestScanError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer srv.Close()
	s := &Scanner{Base: srv.URL, UserAgent: "m2/test"}
	if _, _, err := s.Scan(context.Background(), Queries[:1], "2026-10-31"); err == nil {
		t.Error("a status 422 gives no error")
	}
}

func TestMergeWrite(t *testing.T) {
	old := []Candidate{
		{Repo: "b/notes", Stars: 1, Found: "2026-01-01", Query: "q1", Rules: Rules, Status: "confirmed"},
		{Repo: "c/notes", Stars: 5, Found: "2026-01-01", Query: "q1", Rules: Rules, Status: "rejected"},
	}
	scan := []Candidate{
		{Repo: "a/notes", Stars: 2, Found: "2026-10-31", Query: "q2", Rules: Rules, Status: "candidate"},
		{Repo: "b/notes", Stars: 9, Found: "2026-10-31", Query: "q2", Rules: Rules, Status: "candidate"},
	}
	cs, added := Merge(old, scan)
	if added != 1 || len(cs) != 3 || cs[0].Repo != "a/notes" || cs[2].Repo != "c/notes" {
		t.Fatalf("merge = %+v, %d", cs, added)
	}
	if b := cs[1]; b.Stars != 9 || b.Status != "confirmed" || b.Found != "2026-01-01" || b.Query != "q1" {
		t.Errorf("b/notes = %+v", b)
	}
	path := filepath.Join(t.TempDir(), "candidates", "github-notes.md")
	if got, err := Read(path); err != nil || got != nil {
		t.Errorf("Read of a missing file = %v, %v", got, err)
	}
	if err := Write(path, cs); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "---\nrecords:\n") {
		t.Errorf("the file is not a file of records:\n%s", data)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[1] != cs[1] {
		t.Errorf("read back = %+v", got)
	}
}

func TestReset(t *testing.T) {
	h := http.Header{}
	if d := reset(h); d != time.Minute {
		t.Errorf("no header: %v", d)
	}
	h.Set("X-RateLimit-Reset", "1")
	if d := reset(h); d != 0 {
		t.Errorf("a reset in the past: %v", d)
	}
	h.Set("Retry-After", "600")
	if d := reset(h); d != 2*time.Minute {
		t.Errorf("Retry-After 600: %v", d)
	}
}
