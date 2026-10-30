package cli

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
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
		zw := gzip.NewWriter(w)
		_, _ = io.WriteString(zw, "b2 Topology\nb1 Algebra\n")
		_ = zw.Close()
	}))
	defer srv.Close()
	root, cache := t.TempDir(), t.TempDir()
	if code, _, stderr := run("res", "import", "-root", root, "-cache", cache, "words"); code != ExitFailure || !strings.Contains(stderr, "gives no source records") {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if code, _, _ := run("res", "import", "-root", root); code != ExitUsage {
		t.Fatalf("exit code %d with no source", code)
	}
	// m2 res import has no -url flag, so the test pins the snapshot with m2 ingest first. The snapshot is compressed, as the snapshot of a Fetcher is.
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

// tocShelf is a shelf that also gives a table of contents: one chapter for each line of a snapshot.
type tocShelf struct{ shelf }

func (tocShelf) Source() ingest.Source {
	return ingest.Source{ID: "tocshelf", Name: "A test catalogue with tables of contents", Licence: "CC0-1.0", Class: ingest.Open}
}
func (tocShelf) Structure(r io.Reader) ([]resource.Node, string, error) {
	var ns []resource.Node
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		n := strconv.Itoa(len(ns) + 1)
		_, title, _ := strings.Cut(sc.Text(), " ")
		ns = append(ns, resource.Node{Key: n, Kind: "chapter", Number: n, Title: title})
	}
	return ns, "abc", sc.Err()
}

func init() { ingest.Register(tocShelf{}) }

func TestResImportStructure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		zw := gzip.NewWriter(w)
		_, _ = io.WriteString(zw, "b1 Algebra\n")
		_ = zw.Close()
	}))
	defer srv.Close()
	root, cache := t.TempDir(), t.TempDir()
	for _, id := range []string{"shelf", "tocshelf"} {
		if code, _, stderr := run("ingest", "-root", root, "-cache", cache, "-url", srv.URL+"/"+id+".txt", id); code != ExitOK {
			t.Fatalf("exit code %d, stderr %q", code, stderr)
		}
	}
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		t.Fatal(err)
	}
	hash := cat.Source("tocshelf").Snapshots[0].Hash
	if code, _, stderr := run("res", "import", "-root", root, "-cache", cache, "-snapshot", cat.Source("shelf").Snapshots[0].Hash, "-structure", "shelf"); code != ExitFailure || !strings.Contains(stderr, "gives no table of contents") {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	args := []string{"res", "import", "-root", root, "-cache", cache, "-snapshot", hash, "-structure", "tocshelf"}
	if code, _, stderr := run(args...); code != ExitFailure || !strings.Contains(stderr, "0 live resources group the source records of tocshelf") {
		t.Fatalf("exit code %d with no resource, stderr %q", code, stderr)
	}
	rs := []resource.Resource{
		{ID: "8F2K1QW7ZJ", Kind: "textbook", Title: "Algebra", Expressions: []resource.Expression{{Key: "e1", Kind: "edition"}, {Key: "e2", Kind: "edition", Label: "2nd edition"}},
			Catalogue: []resource.CatalogueRecord{{Source: "tocshelf", Snapshot: hash, NativeID: "b1", Level: "work"}}},
		{ID: "8F2K1QW7ZK", Kind: "textbook", Title: "Other", Catalogue: []resource.CatalogueRecord{{Source: "shelf", NativeID: "b1", Level: "work"}}},
	}
	if err := resource.WriteResources(root, rs); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run(args...)
	if code != ExitOK || !strings.Contains(stdout, "tocshelf: 1 top nodes of the table of contents in records/resources/structure/8F2K1QW7ZJ/e1.json") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	s, err := resource.ReadStructure(root, "8F2K1QW7ZJ", "e1")
	if err != nil || s == nil {
		t.Fatalf("the structure is %+v, %v", s, err)
	}
	if s.Resource != "8F2K1QW7ZJ/e1" || s.Source != "tocshelf" || s.Snapshot != hash || len(s.Nodes) != 1 || s.Nodes[0].Title != "Algebra" {
		t.Errorf("the structure is %+v", s)
	}
	got, err := resource.ReadResources(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(got[0].Expressions) != 2 || got[0].Expressions[0].Structure != "8F2K1QW7ZJ/e1.json" || got[0].Expressions[1].Structure != "" || got[1].Expressions != nil {
		t.Errorf("the resources are %+v", got)
	}
}

// courseShelf is a shelf that also gives the sessions of two offerings, as a course platform does. The snapshot does not change them.
type courseShelf struct{ shelf }

func (courseShelf) Source() ingest.Source {
	return ingest.Source{ID: "courseshelf", Name: "A test course platform", Licence: "CC0-1.0", Class: ingest.Open}
}
func (courseShelf) Structures(r io.Reader, emit func(string, []resource.Node) error) error {
	if _, err := io.Copy(io.Discard, r); err != nil {
		return err
	}
	ns := []resource.Node{{Key: "L01", Kind: "session", Number: "1", Title: "Vectors", Materials: []string{"ocw:c/resources/m1", "ocw:c/resources/m2"},
		Readings: []resource.Reading{{Resource: "isbn:9780980232714", At: map[string]string{"section": "1.1"}, Text: "1.1"}, {Resource: "isbn:9780000000002", At: map[string]string{"section": "2.1"}, Text: "Other book 2.1"}, {Text: "Notes"}}}}
	if err := emit("offering:042nb2s44:18.06:2010-spring", ns); err != nil {
		return err
	}
	return emit("offering:042nb2s44:18.06:2011-fall", ns)
}

func init() { ingest.Register(courseShelf{}) }

func TestResImportStructures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "c Linear Algebra\n") }))
	defer srv.Close()
	root, cache := t.TempDir(), t.TempDir()
	if code, _, stderr := run("ingest", "-root", root, "-cache", cache, "-url", srv.URL+"/c.txt", "courseshelf"); code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"res", "import", "-root", root, "-cache", cache, "-snapshot", cat.Source("courseshelf").Snapshots[0].Hash, "-structure", "courseshelf"}
	if code, _, stderr := run(args...); code != ExitFailure || !strings.Contains(stderr, "Run m2 res cluster first") {
		t.Fatalf("exit code %d with no resource, stderr %q", code, stderr)
	}
	rs := []resource.Resource{
		{ID: "8F2K1QW7ZA", Kind: "course", Title: "Linear Algebra", IDs: resource.IDs{"course": {"042nb2s44:18.06"}},
			Expressions: []resource.Expression{{Key: "e1", Kind: "offering", Label: "Spring 2010", IDs: resource.IDs{"offering": {"042nb2s44:18.06:2010-spring"}}}}},
		{ID: "8F2K1QW7ZB", Kind: "course-material", Title: "Lecture 1", Expressions: []resource.Expression{{Key: "e1", Kind: "edition", Manifestations: []resource.Manifestation{{Key: "m1", Kind: "page", IDs: resource.IDs{"ocw": {"c/resources/m1"}}}}}}},
		{ID: "8F2K1QW7ZC", Kind: "textbook", Title: "Introduction to Linear Algebra", Expressions: []resource.Expression{{Key: "e1", Kind: "edition"}, {Key: "e2", Kind: "edition", Manifestations: []resource.Manifestation{{Key: "m1", Kind: "print", IDs: resource.IDs{"isbn": {"9780980232714"}}}}}}},
	}
	if err := resource.WriteResources(root, rs); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run(args...)
	if code != ExitOK || !strings.Contains(stdout, "courseshelf: 1 structures in records/resources/structure, 1 with no resource") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	s, err := resource.ReadStructure(root, "8F2K1QW7ZA", "e1")
	if err != nil || s == nil || len(s.Nodes) != 1 {
		t.Fatalf("the structure is %+v, %v", s, err)
	}
	n := s.Nodes[0]
	if len(n.Materials) != 2 || n.Materials[0] != "8F2K1QW7ZB" || n.Materials[1] != "ocw:c/resources/m2" {
		t.Errorf("the materials are %v. A material that the tree does not have keeps its id", n.Materials)
	}
	want := []resource.Reading{{Resource: "8F2K1QW7ZC/e2", At: map[string]string{"section": "1.1"}}, {At: map[string]string{"section": "2.1"}, Text: "Other book 2.1"}, {Text: "Notes"}}
	if len(n.Readings) != len(want) {
		t.Fatalf("the readings are %+v", n.Readings)
	}
	for i, r := range n.Readings {
		if r.Resource != want[i].Resource || r.Text != want[i].Text || r.At["section"] != want[i].At["section"] {
			t.Errorf("reading %d is %+v, want %+v", i, r, want[i])
		}
	}
	got, err := resource.ReadResources(root)
	if err != nil {
		t.Fatal(err)
	}
	e := got[0].Expressions[0]
	if e.Structure != "8F2K1QW7ZA/e1.json" || len(got[0].Relations) != 1 || got[0].Relations[0] != (resource.Relation{Kind: "uses_text", From: "e1", To: "8F2K1QW7ZC/e2", Source: "courseshelf"}) {
		t.Errorf("the course is %+v", got[0])
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := resource.Validate(resource.StructureSchema, data); err != nil {
		t.Errorf("the structure does not match its schema: %v", err)
	}
}
