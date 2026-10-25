package cli

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
)

// words is an adapter for the tests: each line of a snapshot is an id and a title.
type words struct{}

func (words) Source() ingest.Source {
	return ingest.Source{ID: "words", Name: "A test source", Licence: "CC0-1.0", Class: ingest.Open}
}
func (words) Version() string { return "words/1" }
func (words) Dump() string    { return "http://127.0.0.1:1/words.txt" }
func (words) Read(r io.Reader, emit func(ingest.Record) error) error {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		id, title, _ := strings.Cut(sc.Text(), " ")
		if err := emit(ingest.Record{ID: id, Title: title}); err != nil {
			return err
		}
	}
	return sc.Err()
}

func init() { ingest.Register(words{}) }

func TestIngest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "w2 two\nw1 one\n")
	}))
	defer srv.Close()
	root, cache := t.TempDir(), t.TempDir()

	code, stdout, stderr := run("ingest", "-root", root, "-cache", cache, "-url", srv.URL+"/words.txt", "-date", "2026-10-25", "words")
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "words: 2 records from a new snapshot of 2026-10-25") || !strings.Contains(stdout, "classes  open 2") {
		t.Errorf("stdout is:\n%s", stdout)
	}
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		t.Fatal(err)
	}
	hash := cat.Source("words").Snapshots[0].Hash
	records := filepath.Join(root, ingest.RecordsDir, "words.jsonl")
	first, err := os.ReadFile(records)
	if err != nil {
		t.Fatal(err)
	}

	// The source can come first, and the pinned snapshot comes from the cache.
	srv.Close()
	code, stdout, stderr = run("ingest", "words", "-root", root, "-cache", cache, "-snapshot", hash)
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "from a pinned snapshot") {
		t.Errorf("stdout is:\n%s", stdout)
	}
	if again, _ := os.ReadFile(records); string(again) != string(first) {
		t.Errorf("two imports of the same snapshot give\n%s\nand\n%s", first, again)
	}

	if code, _, stderr := run("audit", "-root", root, "-plane", "licence", "-q"); code != ExitOK {
		t.Errorf("the licence rules fail on the import: %s", stderr)
	}
}

func TestIngestErrors(t *testing.T) {
	cases := map[string][]string{
		"no source":     {"ingest"},
		"two sources":   {"ingest", "words", "other"},
		"bad flag":      {"ingest", "-zz"},
		"unknown":       {"ingest", "-root", t.TempDir(), "other"},
		"not available": {"ingest", "-root", t.TempDir(), "-cache", t.TempDir(), "words"},
	}
	for name, args := range cases {
		if code, _, stderr := run(args...); code == ExitOK || stderr == "" {
			t.Errorf("%s: exit code %d, stderr %q", name, code, stderr)
		}
	}
	if _, _, stderr := run("ingest", "-root", t.TempDir(), "other"); !strings.Contains(stderr, "The sources with an adapter: words") {
		t.Errorf("stderr is %q", stderr)
	}
}
