package nlab_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/cli"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/nlab"
	"github.com/tamnd/math2-agent/inventory"
)

const commit = "9a8b7c6d5e4f30211203f4e5d6c7b8a9f0e1d2c3"

// The pages of the fixture. The text is written for the test and is not text of the nLab.
var pages = map[string]string{
	"pages/0/0/0/1/1000/name":       "finitary monad\n",
	"pages/0/0/0/1/1000/content.md": "[[!include higher algebra - contents]]\n\nA [[monad]] whose [[endofunctor|functor]] keeps [[filtered colimit#Definition|filtered colimits]].\nSee [[monad]] again.\n\n[[!redirects finitary monads]]\n[[!redirects finitary  monad ]]\n",
	"pages/0/0/0/2/2000/name":       "monad",
	"pages/0/0/0/2/2000/content.md": "A monoid in endofunctors. No links here.\n",
	"pages/0/0/0/3/3000/name":       "C*-algebra",
	"pages/0/0/0/3/3000/content.md": "An [[algebra]].\n",
	"README.md":                     "# nLab content\n",
	"pages/0/0/0/3/3000/notes.txt":  "a file that the adapter does not read\n",
}

// archive makes a git archive like the one GitHub gives, with the files in one directory.
func archive(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": commit}}); err != nil {
		t.Fatal(err)
	}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		body := files[name]
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "nlab-content-master/" + name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write([]byte(body))
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = zw.Close()
	return buf.Bytes()
}

func TestPull(t *testing.T) {
	tgz := archive(t, pages)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tgz) }))
	defer srv.Close()
	root := t.TempDir()
	res, err := ingest.Pull(context.Background(), nlab.Adapter{}, ingest.Options{Root: root, Cache: t.TempDir(), URL: srv.URL + "/master.tar.gz", Date: "2026-10-27"})
	if err != nil {
		t.Fatal(err)
	}
	if m := res.Manifest; m.Records != 3 || m.Classes[ingest.RecordOnly] != 3 || m.Adapter != "nlab/1" {
		t.Errorf("the manifest is %+v, want 3 record-only records from nlab/1", m)
	}
	if res.Snapshot.Commit != commit {
		t.Errorf("the snapshot has the commit %q", res.Snapshot.Commit)
	}
	recs, _, err := ingest.ReadRecords(root, "nlab")
	if err != nil {
		t.Fatal(err)
	}
	var fm *ingest.Record
	for i := range recs {
		if recs[i].Text != "" {
			t.Errorf("the record %s keeps the text of the page", recs[i].ID)
		}
		if recs[i].ID == "finitary monad" {
			fm = &recs[i]
		}
	}
	if fm == nil {
		t.Fatal("no record of the page finitary monad")
	}
	if len(fm.Fingerprints) == 0 {
		t.Error("the record of finitary monad has no fingerprints, thus L02 cannot find its text in nodes/")
	}
	if fm.URL != "https://ncatlab.org/nlab/show/finitary+monad" || fm.Locator != "pages/0/0/0/1/1000" || fm.Title != "finitary monad" {
		t.Errorf("the record is %+v", fm)
	}
	if got := fm.Meta["links"]; got != "endofunctor|filtered colimit|monad" {
		t.Errorf("the links are %q", got)
	}
	if got := fm.Meta["redirects"]; got != "finitary monads" {
		t.Errorf("the redirects are %q", got)
	}

	// The mentions of the import hold no text, and they carry the nLab name as their native id.
	src := &ingest.Source{ID: "nlab", Licence: "LicenseRef-unknown", Class: ingest.RecordOnly}
	for _, m := range inventory.FromRecords(src, res.Snapshot.Hash, "nlab/1", recs) {
		if m.Text != "" || m.Class != string(ingest.RecordOnly) {
			t.Errorf("the mention %s has the class %s and the text %q", m.ID, m.Class, m.Text)
		}
	}
}

func TestRead(t *testing.T) {
	none := func(ingest.Record) error { return nil }
	bad := map[string]map[string]string{
		"no content": {"pages/1/name": "a"},
		"no name":    {"pages/1/content.md": "x"},
		"two names":  {"pages/1/name": "a", "pages/1/content.md": "x", "pages/2/name": "a", "pages/2/content.md": "y"},
		"bad name":   {"pages/1/name": "a|b", "pages/1/content.md": "x"},
		"no pages":   {"README.md": "x"},
	}
	for name, files := range bad {
		tgz := archive(t, files)
		zr, err := gzip.NewReader(bytes.NewReader(tgz))
		if err != nil {
			t.Fatal(err)
		}
		if err := (nlab.Adapter{}).Read(zr, none); err == nil {
			t.Errorf("%s: Read gives no error", name)
		}
	}
}

func TestPageURL(t *testing.T) {
	cases := map[string]string{
		"group":                 "https://ncatlab.org/nlab/show/group",
		"C*-algebra":            "https://ncatlab.org/nlab/show/C%2A-algebra",
		"(infinity,1)-category": "https://ncatlab.org/nlab/show/%28infinity%2C1%29-category",
	}
	for name, want := range cases {
		if got := nlab.PageURL(name); got != want {
			t.Errorf("PageURL(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestIngestMentions(t *testing.T) {
	tgz := archive(t, pages)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tgz) }))
	defer srv.Close()
	root := t.TempDir()
	var out, errs strings.Builder
	if code := cli.Run([]string{"ingest", "-root", root, "-cache", t.TempDir(), "-url", srv.URL + "/master.tar.gz", "-date", "2026-10-27", "-mentions", "nlab"}, "test", &out, &errs); code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, errs.String())
	}
	files, err := filepath.Glob(filepath.Join(root, inventory.MentionsDir, "nlab", "*.jsonl"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no mention files: %v", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f) // #nosec G304 -- a file of the test tree.
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), `"text"`) {
			t.Errorf("%s holds text:\n%s", f, b)
		}
	}
}
