package stacks_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/audit"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/stacks"
)

const commit = "4f0e2d1c3b5a69788796a5b4c3d2e1f0a9b8c7d6"

// archive makes a git archive like the one GitHub gives, with the files in one directory.
func archive(t *testing.T, withCommit bool, files map[string]string) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if withCommit {
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": commit}}); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "stacks-project-master/" + name, Mode: 0o644, Size: int64(len(body))}); err != nil {
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

// fixtures gives the files of testdata at their paths in the repository, and a LaTeX file in a directory, which the adapter does not read.
func fixtures(t *testing.T) map[string]string {
	files := map[string]string{"README.md": "# The Stacks project\n", "documentation/notes.tex": "\\begin{lemma}\n\\label{lemma-x}\nx\n\\end{lemma}\n"}
	err := fs.WalkDir(os.DirFS("testdata"), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(filepath.Join("testdata", name))
		files[name] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// pull imports the fixtures into a new root.
func pull(t *testing.T) string {
	tgz := archive(t, true, fixtures(t))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tgz) }))
	defer srv.Close()
	root := t.TempDir()
	res, err := ingest.Pull(context.Background(), stacks.Adapter{}, ingest.Options{Root: root, Cache: t.TempDir(), URL: srv.URL + "/master.tar.gz", Date: "2026-10-26"})
	if err != nil {
		t.Fatal(err)
	}
	if m := res.Manifest; m.Records != 14 || m.Classes[ingest.GFDL] != 14 || m.Adapter != "stacks/1" {
		t.Errorf("the manifest is %+v, want 14 gfdl records from stacks/1", m)
	}
	return root
}

func TestPull(t *testing.T) {
	root := pull(t)
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		t.Fatal(err)
	}
	if s := cat.Source("stacks"); s == nil || len(s.Snapshots) != 1 || s.Snapshots[0].Commit != commit {
		t.Errorf("the catalogue entry is %+v, want one snapshot at %s", s, commit)
	}
	recs, _, err := ingest.ReadRecords(root, "stacks")
	if err != nil {
		t.Fatal(err)
	}
	type want struct {
		kind, title, locator, label, chapter, section string
		prints                                        bool
	}
	wants := map[string]want{
		"T001": {"part", "Preliminaries", "chapters.tex:3", "book-part-preliminaries", "", "", false},
		"T002": {"chapter", "Sets", "sets.tex:11", "sets-section-phantom", "Sets", "", false},
		"T003": {"section", "Introduction", "sets.tex:16", "sets-section-introduction", "Sets", "", true},
		"T004": {"section", "Finite sets", "sets.tex:23", "sets-section-finite", "Sets", "", true},
		"T005": {"definition", "", "sets.tex:29", "sets-definition-finite", "Sets", "Finite sets", true},
		"T006": {"lemma", "Pigeonhole", "sets.tex:35", "sets-lemma-pigeonhole", "Sets", "Finite sets", true},
		"T007": {"equation", "", "sets.tex:52", "sets-equation-count", "Sets", "Finite sets", false},
		"T008": {"subsection", "Unions", "sets.tex:58", "sets-subsection-unions", "Sets", "Finite sets", true},
		"T009": {"remark", "", "sets.tex:63", "sets-lemma-wrong-prefix", "Sets", "Finite sets", true},
		"T00A": {"lemma", "", "sets.tex:68", "sets-lemma-items", "Sets", "Finite sets", true},
		"T00B": {"item", "", "sets.tex:73", "sets-item-union", "Sets", "Finite sets", false},
		"T00C": {"part", "Schemes", "chapters.tex:7", "book-part-schemes", "", "", false},
		"T00D": {"chapter", "Spaces", "spaces.tex:8", "spaces-section-phantom", "Spaces", "", false},
		"T00E": {"theorem", "A theorem with a name", "spaces.tex:13", "spaces-theorem-named", "Spaces", "Points", true},
	}
	for _, r := range recs {
		w, ok := wants[r.ID]
		if !ok {
			t.Errorf("the record %s is not in the tags", r.ID)
			continue
		}
		delete(wants, r.ID)
		if r.Kind != w.kind || r.Title != w.title || r.Locator != w.locator || r.URL != stacks.Home+"/tag/"+r.ID {
			t.Errorf("%s is %s %q at %s, %s, want %s %q at %s", r.ID, r.Kind, r.Title, r.Locator, r.URL, w.kind, w.title, w.locator)
		}
		if r.Meta["label"] != w.label || r.Meta["chapter"] != w.chapter || r.Meta["section"] != w.section {
			t.Errorf("the meta of %s is %v, want the label %s, the chapter %q and the section %q", r.ID, r.Meta, w.label, w.chapter, w.section)
		}
		if r.Text != "" || r.Class != ingest.GFDL {
			t.Errorf("%s has the class %s and stores text: %q", r.ID, r.Class, r.Text)
		}
		if got := len(r.Fingerprints) > 0; got != w.prints {
			t.Errorf("%s has %d fingerprints", r.ID, len(r.Fingerprints))
		}
	}
	for id := range wants {
		t.Errorf("the records have no %s", id)
	}
}

// node writes a node with a statement that cites the pigeonhole lemma.
func node(t *testing.T, root, statement string) {
	body := "---\ntag: 4M0C1B\nkind: statement\nrole: lemma\nname: The pigeonhole principle\nsources:\n  - stacks: T006\n---\n\n## Statement\n\n" + statement + "\n"
	dir := filepath.Join(root, "nodes", "4M0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "4M0C1B.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestL02 imports the fixtures and checks that rule L02 fails on a node with the text of a lemma, of its proof or of a section, and passes on a node in its own words.
func TestL02(t *testing.T) {
	for _, tc := range []struct {
		name, statement string
		fail            bool
	}{
		{"the statement", "Let $S$ and $T$ be finite sets with more elements in $S$ than in $T$. Then each map from $S$ to $T$ sends two different elements to one element.", true},
		{"the proof", "Use induction on the number of elements of $T$. When $T$ is empty, the set $S$ has an element and there is no map at all.", true},
		{"the section", "This chapter is a test file for the adapter of math2. It is in the style of the Stacks project.", true},
		{"own words", "If a finite set has more elements than a second finite set, no map from the first set to the second is injective.", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := pull(t)
			node(t, root, tc.statement)
			rep, err := audit.Run(root, audit.Options{Rules: []string{"L02"}})
			if err != nil {
				t.Fatal(err)
			}
			want := check.Pass
			if tc.fail {
				want = check.Fail
			}
			if len(rep.Results) != 1 || rep.Results[0].State != want {
				t.Fatalf("the results are %+v, want %s", rep.Results, want)
			}
			for _, f := range rep.Findings {
				if !strings.Contains(f.String(), "with stacks T0") {
					t.Errorf("the finding is %s", f)
				}
			}
		})
	}
}

func TestRead(t *testing.T) {
	none := func(ingest.Record) error { return nil }
	tags := "T001,a-lemma-x\n"
	lemma := "\\begin{lemma}\n\\label{lemma-x}\nx\n\\end{lemma}\n"
	for name, files := range map[string]map[string]string{
		"no tags":           {"a.tex": lemma},
		"an empty list":     {"a.tex": lemma, stacks.Tags: "# no tags\n"},
		"a bad tag":         {"a.tex": lemma, stacks.Tags: "t001,a-lemma-x\n"},
		"a tag two times":   {"a.tex": lemma, stacks.Tags: tags + tags},
		"a missing label":   {"a.tex": lemma, stacks.Tags: "T001,a-lemma-y\n"},
		"a label two times": {"a.tex": lemma + lemma, stacks.Tags: tags},
		"no label":          {"a.tex": "\\begin{lemma}\nx\n\\end{lemma}\n", stacks.Tags: tags},
		"an open lemma":     {"a.tex": "\\begin{lemma}\n\\label{lemma-x}\nx\n", stacks.Tags: tags},
	} {
		zr, err := gzip.NewReader(bytes.NewReader(archive(t, true, files)))
		if err != nil {
			t.Fatal(err)
		}
		if err := (stacks.Adapter{}).Read(zr, none); err == nil {
			t.Errorf("Read of an archive with %s gives no error", name)
		}
	}
	if err := (stacks.Adapter{}).Read(strings.NewReader("not a tar file"), none); err == nil {
		t.Error("Read of bytes that are not a tar file gives no error")
	}
}

func TestSource(t *testing.T) {
	s := stacks.Adapter{}.Source()
	if p := s.Problems(); len(p) > 0 {
		t.Errorf("the catalogue entry has problems: %v", p)
	}
	if !ingest.ShareAlike(s.Licence) || ingest.ClassOf(s.Licence) != ingest.GFDL {
		t.Errorf("the licence %s is not share-alike or not of the class gfdl", s.Licence)
	}
	if a, ok := ingest.Lookup("stacks"); !ok || a.Version() != "stacks/1" {
		t.Error("the adapter is not registered")
	}
}
