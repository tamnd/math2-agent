package cli

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	rescluster "github.com/tamnd/math2-agent/resource/cluster"
)

// shelf is an adapter for the tests that gives source records: each line of a snapshot is an id and a title.
type shelf struct{ words }

func (shelf) Source() ingest.Source {
	return ingest.Source{ID: "shelf", Name: "A test catalogue", Licence: "CC0-1.0", Class: ingest.Open}
}
func (shelf) Version() string { return "shelf/1" }
func (shelf) Records(r io.Reader, emit func(rescluster.Record) error) error {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		id, title, _ := strings.Cut(sc.Text(), " ")
		if err := emit(rescluster.Record{NativeID: id, Level: "work", Kind: "monograph", Title: title}); err != nil {
			return err
		}
	}
	return sc.Err()
}

func init() { ingest.Register(shelf{}) }

func TestResImport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "b2 Topology\nb1 Algebra\n")
	}))
	defer srv.Close()
	root, cache := t.TempDir(), t.TempDir()
	if code, _, stderr := run("res", "import", "-root", root, "-cache", cache, "words"); code != ExitFailure || !strings.Contains(stderr, "gives no source records") {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if code, _, _ := run("res", "import", "-root", root); code != ExitUsage {
		t.Fatalf("exit code %d with no source", code)
	}
	// m2 res import has no -url flag, so the test pins the snapshot with m2 ingest first.
	if code, _, stderr := run("ingest", "-root", root, "-cache", cache, "-url", srv.URL+"/shelf.txt", "shelf"); code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		t.Fatal(err)
	}
	hash := cat.Source("shelf").Snapshots[0].Hash
	srv.Close()
	code, stdout, stderr := run("res", "import", "shelf", "-root", root, "-cache", cache, "-snapshot", hash)
	if code != ExitOK || !strings.Contains(stdout, "shelf: 2 source records") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	rs, err := rescluster.ReadRecords(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || rs[0].NativeID != "b1" || rs[0].Source != "shelf" || rs[0].Snapshot != hash || rs[1].Title != "Topology" {
		t.Fatalf("the source records are %+v", rs)
	}
}
