package cli

import (
	"bufio"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/inventory"
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

	code, stdout, stderr := run("ingest", "-root", root, "-cache", cache, "-url", srv.URL+"/words.txt", "-date", "2026-10-25", "-mentions", "words")
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "words: 2 records from a new snapshot of 2026-10-25") || !strings.Contains(stdout, "classes  open 2") || !strings.Contains(stdout, "mentions 2 in records/mentions/words/") {
		t.Errorf("stdout is:\n%s", stdout)
	}
	mentions, err := inventory.ReadMentions(root)
	if err != nil || len(mentions) != 2 || mentions[0].NativeID != "w1" || mentions[0].By != "words/1" {
		t.Fatalf("the mentions are %+v, %v", mentions, err)
	}
	firstMentions := mentionFiles(t, root)
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
	code, stdout, stderr = run("ingest", "words", "-root", root, "-cache", cache, "-snapshot", hash, "-mentions")
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "from a pinned snapshot") {
		t.Errorf("stdout is:\n%s", stdout)
	}
	if again, _ := os.ReadFile(records); string(again) != string(first) {
		t.Errorf("two imports of the same snapshot give\n%s\nand\n%s", first, again)
	}
	if again := mentionFiles(t, root); !maps.Equal(again, firstMentions) {
		t.Errorf("two imports of the same snapshot give the mentions\n%v\nand\n%v", firstMentions, again)
	}

	// m2 ingest makes the entry of a new source, and L08 fails until a person completes the source document and approves it.
	if code, _, _ := run("audit", "-root", root, "-rule", "L08", "-q"); code == ExitOK {
		t.Error("L08 passes on a source document that no person approved")
	}
	if cat, err = ingest.ReadCatalogue(root); err != nil {
		t.Fatal(err)
	}
	s := cat.Source("words")
	s.LicenceRead = &ingest.LicenceRead{At: "https://example.org/LICENSE", On: "2026-10-25", By: "a-reader"}
	s.Authored, s.Channel, s.ApprovedBy = "human", "bulk", "a-maintainer"
	if s.Attribution == "" {
		s.Attribution = "The words of the test"
	}
	if err := cat.Write(root); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := run("audit", "-root", root, "-plane", "licence", "-q"); code != ExitOK {
		t.Errorf("the licence rules fail on the import: %s", stderr)
	}
}

// mentionFiles returns the files of records/mentions/ under root and their contents.
func mentionFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(filepath.Join(root, inventory.MentionsDir), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path) // #nosec G304 -- a file of the test tree.
		out[path] = string(data)
		return err
	})
	if err != nil || len(out) == 0 {
		t.Fatalf("records/mentions/ has %d files: %v", len(out), err)
	}
	return out
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
	if _, _, stderr := run("ingest", "-root", t.TempDir(), "other"); !strings.Contains(stderr, "The sources with an adapter: ") || !strings.Contains(stderr, "words") {
		t.Errorf("stderr is %q", stderr)
	}
}
