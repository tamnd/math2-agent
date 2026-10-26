package ingest_test

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
)

// lines is an adapter for tests. Each line of a snapshot is an item: the id, the licence, the title and the text, with a tab between them. An empty licence means the licence of the source.
type lines struct {
	id, licence string
	class       ingest.Class
}

func (l lines) Source() ingest.Source {
	return ingest.Source{ID: l.id, Name: "A test source", Licence: l.licence, Class: l.class, Attribution: "The test authors"}
}

func (l lines) Version() string { return l.id + "/1" }

func (l lines) Dump() string { return "https://example.org/" + l.id + ".tsv" }

func (l lines) Read(r io.Reader, emit func(ingest.Record) error) error {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.SplitN(sc.Text(), "\t", 4)
		if len(f) != 4 {
			return fmt.Errorf("the line %q has %d fields", sc.Text(), len(f))
		}
		if err := emit(ingest.Record{ID: f[0], Licence: f[1], Title: f[2], Text: f[3], URL: "https://example.org/" + f[0]}); err != nil {
			return err
		}
	}
	return sc.Err()
}

const passage = "every finite group of prime order is cyclic and each element other than the identity generates it"

func TestClassOf(t *testing.T) {
	cases := map[string]ingest.Class{
		"CC-BY-SA-4.0":      ingest.Open,
		"CC0-1.0":           ingest.Open,
		"Apache-2.0":        ingest.Open,
		"CC-BY-NC-SA-4.0":   ingest.ShareAlikeNC,
		"CC-BY-NC-ND-4.0":   ingest.Verbatim,
		"GFDL-1.3-or-later": ingest.GFDL,
		"LicenseRef-arXiv":  ingest.RecordOnly,
		"proprietary":       ingest.RecordOnly,
		"":                  ingest.RecordOnly,
	}
	for licence, want := range cases {
		if got := ingest.ClassOf(licence); got != want {
			t.Errorf("ClassOf(%q) = %s, want %s", licence, got, want)
		}
	}
	for _, c := range ingest.Classes() {
		if !c.Valid() {
			t.Errorf("%s is not valid", c)
		}
		if c.Prose() != (c == ingest.Open) || c.StoresText() != (c == ingest.Open) {
			t.Errorf("%s: Prose %v, StoresText %v", c, c.Prose(), c.StoresText())
		}
	}
	if ingest.Class("closed").Valid() {
		t.Error("closed is valid")
	}
	for licence, want := range map[string]bool{"CC-BY-SA-3.0": true, "CC-BY-NC-SA-4.0": true, "GFDL-1.3-only": true, "CC-BY-4.0": false, "CC0-1.0": false} {
		if ingest.ShareAlike(licence) != want {
			t.Errorf("ShareAlike(%q) is not %v", licence, want)
		}
	}
}

func TestFingerprints(t *testing.T) {
	a := ingest.Fingerprints("In the first chapter we show that " + passage + ", and we use it later.")
	b := ingest.Fingerprints("A well known fact: " + strings.ToUpper(passage) + ". The proof uses Lagrange.")
	if ingest.Shared(a, b) == 0 {
		t.Errorf("two texts with a shared passage of %d words share no fingerprint", len(strings.Fields(passage)))
	}
	c := ingest.Fingerprints("A ring is a set with two operations, addition and multiplication, that satisfy the usual laws of arithmetic.")
	if n := ingest.Shared(a, c); n != 0 {
		t.Errorf("two texts with no shared passage share %d fingerprints", n)
	}
	if f := ingest.Fingerprints("too short to have a print"); f != nil {
		t.Errorf("a text of 6 words has the fingerprints %v", f)
	}
	if f := ingest.Fingerprints("one two three four five six seven eight"); len(f) != 1 {
		t.Errorf("a text of 8 words has the fingerprints %v", f)
	}
	for i := 1; i < len(a); i++ {
		if a[i-1] >= a[i] {
			t.Fatalf("the fingerprints are not in increasing order: %v", a)
		}
	}
}

func TestGate(t *testing.T) {
	gfdl := &ingest.Source{ID: "stacks", Licence: "GFDL-1.3-or-later", Class: ingest.GFDL}
	r := ingest.Gate(gfdl, ingest.Record{ID: "0001", Text: passage})
	if r.Text != "" || r.Class != ingest.GFDL || len(r.Fingerprints) == 0 || r.Licence != "" {
		t.Errorf("the gate gives %+v for a GFDL item", r)
	}
	arxiv := &ingest.Source{ID: "arxiv", Licence: "LicenseRef-arXiv", Class: ingest.RecordOnly}
	r = ingest.Gate(arxiv, ingest.Record{ID: "2610.01234", Licence: "CC-BY-4.0", Text: passage})
	if r.Text != passage || r.Class != ingest.Open || r.Licence != "CC-BY-4.0" {
		t.Errorf("the gate gives %+v for an item with its own open licence", r)
	}
	r = ingest.Gate(arxiv, ingest.Record{ID: "2610.01235", Licence: "LicenseRef-arXiv", Text: passage})
	if r.Text != "" || r.Licence != "" {
		t.Errorf("the gate gives %+v for an item with the licence of the source", r)
	}
	arxiv.PerItemLicence = true
	r = ingest.Gate(arxiv, ingest.Record{ID: "2610.01235", Licence: "LicenseRef-arXiv", Text: passage})
	if r.Text != "" || r.Licence != "LicenseRef-arXiv" {
		t.Errorf("the gate gives %+v for an item of a source with a licence for each item", r)
	}
}

func TestChoices(t *testing.T) {
	for licence, want := range map[string][]string{
		"CC-BY-SA-4.0":                       nil,
		"CC-BY-SA-4.0 OR CC-BY-NC-SA-4.0":    {"CC-BY-SA-4.0", "CC-BY-NC-SA-4.0"},
		"(GFDL-1.3-only OR CC-BY-SA-3.0-US)": {"GFDL-1.3-only", "CC-BY-SA-3.0-US"},
	} {
		if got := ingest.Choices(licence); !slices.Equal(got, want) {
			t.Errorf("Choices(%q) = %q, want %q", licence, got, want)
		}
	}
	s := ingest.Source{ID: "lebl", Name: "Basic Analysis", Licence: "CC-BY-SA-4.0 OR CC-BY-NC-SA-4.0", Election: "CC-BY-SA-4.0", Class: ingest.Open, LicenceRead: &ingest.LicenceRead{On: "2026-10-20"}}
	if s.Elected() != "CC-BY-SA-4.0" {
		t.Errorf("the elected licence is %s", s.Elected())
	}
	if s.ReadOn(&ingest.Snapshot{}) != "2026-10-20" || s.ReadOn(&ingest.Snapshot{LicenceRead: "2026-09-01"}) != "2026-09-01" {
		t.Error("ReadOn does not give the date of the reading of the snapshot")
	}
	if p := s.Problems(); p != nil {
		t.Errorf("an entry with an election has the problems %v", p)
	}
}

func TestImport(t *testing.T) {
	src := &ingest.Source{ID: "test", Licence: "CC0-1.0", Class: ingest.Open}
	recs, err := ingest.Import(lines{id: "test"}, src, strings.NewReader("b\t\tB\tbee\na\t\tA\tay\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[0].ID != "a" || recs[1].ID != "b" {
		t.Errorf("the records are %+v", recs)
	}
	for _, in := range []string{"a\t\tA\tx\na\t\tA\ty\n", "\t\tA\tx\n", "a\tA\n"} {
		if _, err := ingest.Import(lines{id: "test"}, src, strings.NewReader(in)); err == nil {
			t.Errorf("Import(%q) gives no error", in)
		}
	}
}

func TestProblems(t *testing.T) {
	good := ingest.Source{ID: "oeis", Name: "OEIS", Licence: "CC-BY-SA-4.0", Class: ingest.Open, Snapshots: []ingest.Snapshot{{URL: "https://example.org", Hash: "blake3:" + strings.Repeat("a", 64), Date: "2026-10-25"}}}
	if p := good.Problems(); p != nil {
		t.Errorf("a good entry has the problems %v", p)
	}
	stricter := good
	stricter.Class = ingest.RecordOnly
	if p := stricter.Problems(); p != nil {
		t.Errorf("a class stricter than the licence has the problems %v", p)
	}
	cases := map[string]func(s *ingest.Source){
		"id":       func(s *ingest.Source) { s.ID = "OEIS" },
		"name":     func(s *ingest.Source) { s.Name = "" },
		"licence":  func(s *ingest.Source) { s.Licence = "" },
		"class":    func(s *ingest.Source) { s.Class = "closed" },
		"too open": func(s *ingest.Source) { s.Licence = "GFDL-1.3-only" },
		"url":      func(s *ingest.Source) { s.Snapshots[0].URL = "" },
		"hash":     func(s *ingest.Source) { s.Snapshots[0].Hash = "sha256:00" },
		"date":     func(s *ingest.Source) { s.Snapshots[0].Date = "25 October" },
		"twice":    func(s *ingest.Source) { s.Snapshots = append(s.Snapshots, s.Snapshots[0]) },
	}
	for name, change := range cases {
		s := good
		s.Snapshots = append([]ingest.Snapshot(nil), good.Snapshots...)
		change(&s)
		if len(s.Problems()) != 1 {
			t.Errorf("%s: the problems are %v", name, s.Problems())
		}
	}
}

// server serves body at each path and counts the requests.
func server(t *testing.T, body *string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, *body)
	}))
	t.Cleanup(s.Close)
	return s, &n
}

func read(t *testing.T, root string, rels ...string) string {
	t.Helper()
	var b strings.Builder
	for _, rel := range rels {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
	}
	return b.String()
}

func TestPull(t *testing.T) {
	body := "A1\t\tOne\t" + passage + "\nA2\tCC-BY-NC-4.0\tTwo\t" + passage + "\n"
	srv, requests := server(t, &body)
	root, cache := t.TempDir(), t.TempDir()
	a := lines{id: "seq", licence: "CC-BY-SA-4.0", class: ingest.Open}
	opt := ingest.Options{Root: root, Cache: cache, URL: srv.URL + "/seq.tsv", Date: "2026-10-25"}
	files := []string{ingest.CatalogueFile, ingest.ManifestPath("seq"), ingest.RecordsDir + "/seq.jsonl"}

	res, err := ingest.Pull(context.Background(), a, opt)
	if err != nil {
		t.Fatal(err)
	}
	if !res.New || res.Manifest.Records != 2 || res.Manifest.Classes[ingest.Open] != 1 || res.Manifest.Classes[ingest.ShareAlikeNC] != 1 {
		t.Errorf("the first pull gives %+v and %+v", res, res.Manifest)
	}
	first := read(t, root, files...)

	res, err = ingest.Pull(context.Background(), a, opt)
	if err != nil {
		t.Fatal(err)
	}
	if res.New {
		t.Error("the second pull of the same bytes makes a new snapshot")
	}
	if got := read(t, root, files...); got != first {
		t.Errorf("two pulls of the same snapshot give different files:\n%s\n----\n%s", first, got)
	}

	// The pinned snapshot comes from the cache, also when the source changed.
	hash := res.Snapshot.Hash
	body = "A3\t\tThree\tthree\n"
	before := requests.Load()
	pinned := ingest.Options{Root: root, Cache: cache, Snapshot: hash}
	if _, err := ingest.Pull(context.Background(), a, pinned); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != before {
		t.Error("a pull of a cached snapshot downloads it again")
	}
	if got := read(t, root, files...); got != first {
		t.Errorf("a pull of the pinned snapshot gives different files:\n%s", got)
	}

	// Without the cache, the pinned snapshot is downloaded again, and the hash must agree.
	pinned.Cache = t.TempDir()
	if _, err := ingest.Pull(context.Background(), a, pinned); err == nil || !strings.Contains(err.Error(), "the catalogue has") {
		t.Errorf("a download with a different hash gives %v", err)
	}

	recs, sum, err := ingest.ReadRecords(root, "seq")
	if err != nil {
		t.Fatal(err)
	}
	if sum != res.Manifest.Hash || len(recs) != 2 || recs[0].Text != passage || recs[1].Text != "" || len(recs[1].Fingerprints) == 0 {
		t.Errorf("the records are %+v with the hash %s", recs, sum)
	}
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		t.Fatal(err)
	}
	if s := cat.Source("seq"); s == nil || len(s.Snapshots) != 1 || s.Snapshots[0].Date != "2026-10-25" {
		t.Errorf("the catalogue is %+v", cat)
	}
	m, err := ingest.ReadManifest(filepath.Join(root, ingest.ManifestPath("seq")))
	if err != nil || m.Hash != res.Manifest.Hash || m.Snapshot != hash || m.Adapter != "seq/1" {
		t.Errorf("the manifest is %+v, %v", m, err)
	}
}

func TestPullErrors(t *testing.T) {
	body := "A1\t\tOne\tone\n"
	srv, _ := server(t, &body)
	a := lines{id: "seq", licence: "CC-BY-SA-4.0", class: ingest.Open}
	cases := map[string]ingest.Options{
		"not found":   {URL: srv.URL + "/missing"},
		"no snapshot": {Snapshot: "blake3:" + strings.Repeat("0", 64)},
	}
	for name, opt := range cases {
		opt.Root, opt.Cache = t.TempDir(), t.TempDir()
		if _, err := ingest.Pull(context.Background(), a, opt); err == nil {
			t.Errorf("%s: Pull gives no error", name)
		}
	}
	_, err := ingest.Pull(context.Background(), a, ingest.Options{Root: t.TempDir(), Cache: t.TempDir(), URL: srv.URL + "/missing"})
	if se, ok := errors.AsType[*ingest.StatusError](err); !ok || se.Code != http.StatusNotFound || se.URL != srv.URL+"/missing" {
		t.Errorf("a missing snapshot gives %v, want a StatusError with the code 404", err)
	}
	bad := lines{id: "seq", licence: "GFDL-1.3-only", class: ingest.Open}
	if _, err := ingest.Pull(context.Background(), bad, ingest.Options{Root: t.TempDir(), Cache: t.TempDir(), URL: srv.URL}); err == nil || !strings.Contains(err.Error(), "gives the class gfdl") {
		t.Errorf("a source with a class that is too open gives %v", err)
	}
}

func TestRegister(t *testing.T) {
	ingest.Register(lines{id: "registered"})
	if a, ok := ingest.Lookup("registered"); !ok || a.Version() != "registered/1" {
		t.Errorf("Lookup gives %v, %v", a, ok)
	}
	if _, ok := ingest.Lookup("other"); ok {
		t.Error("Lookup finds a source that has no adapter")
	}
	found := false
	for _, id := range ingest.Adapters() {
		found = found || id == "registered"
	}
	if !found {
		t.Errorf("Adapters gives %v", ingest.Adapters())
	}
	defer func() {
		if recover() == nil {
			t.Error("a second Register does not panic")
		}
	}()
	ingest.Register(lines{id: "registered"})
}

func TestImportGzip(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte("a\t\tA\t" + passage + "\n"))
	_ = zw.Close()
	src := &ingest.Source{ID: "test", Licence: "CC0-1.0", Class: ingest.Open}
	recs, err := ingest.Import(lines{id: "test"}, src, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Text != passage {
		t.Errorf("the gzip snapshot gives %+v", recs)
	}
}
