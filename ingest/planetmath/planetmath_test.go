package planetmath_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/audit"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/planetmath"
	"github.com/tamnd/math2-agent/inventory"
)

const commit = "5a1e2d3c4b5a69788796a5b4c3d2e1f0a9b8c7d6"

// archive makes a git archive like the one GitHub gives, with the files in one directory.
func archive(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": commit}}); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: planetmath.Pilot + "-master/" + name, Mode: 0o644, Size: int64(len(body))}); err != nil {
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

// fixtures gives the files of testdata, a README and an entry in a directory, which the adapter does not read.
func fixtures(t *testing.T) map[string]string {
	files := map[string]string{"README.md": "# PlanetMath\n", "html/20-00-Other.tex": "\\pmcanonicalname{Other}\n\\pmtitle{other}\n\\endmetadata\n"}
	names, err := filepath.Glob(filepath.Join("testdata", "*.tex"))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		b, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		files[filepath.Base(n)] = string(b)
	}
	return files
}

// pull imports the fixtures into a new root.
func pull(t *testing.T) string {
	tgz := archive(t, fixtures(t))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tgz) }))
	defer srv.Close()
	root := t.TempDir()
	res, err := ingest.Pull(context.Background(), planetmath.Adapter{}, ingest.Options{Root: root, Cache: t.TempDir(), URL: srv.URL + "/master.tar.gz", Date: "2026-10-27"})
	if err != nil {
		t.Fatal(err)
	}
	if m := res.Manifest; m.Records != 4 || m.Classes[ingest.Open] != 4 || m.Adapter != "planetmath/1" {
		t.Errorf("the manifest is %+v, want 4 open records from planetmath/1", m)
	}
	return root
}

func TestPull(t *testing.T) {
	root := pull(t)
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		t.Fatal(err)
	}
	if s := cat.Source("planetmath"); s == nil || len(s.Snapshots) != 1 || s.Snapshots[0].Commit != commit {
		t.Errorf("the catalogue entry is %+v, want one snapshot at %s", s, commit)
	}
	recs, _, err := ingest.ReadRecords(root, "planetmath")
	if err != nil {
		t.Fatal(err)
	}
	type want struct {
		kind, title, locator, text string
		meta                       map[string]string
	}
	wants := map[string]want{
		"CentreOfAGroup": {"definition", "centre of a group", "20-00-CentreOfAGroup.tex", "This file is a test fixture",
			map[string]string{"msc": "20-00 20A05", "authors": "alice", "defines": "centre|central element", "synonyms": "center of a group", "related": "SylowCount", "modified": "2013-03-22"}},
		"SylowCount": {"theorem", `the number of Sylow $p$-subgroups is $1 \bmod{p}$`, "20D20-SylowCount.tex", "Let $G$ be a finite group",
			map[string]string{"msc": "20D20", "authors": "bob|carol", "modified": "2014-01-05"}},
		"ProofOfSylowCount": {"proof", "proof of the count of Sylow subgroups", "20D20-ProofOfSylowCount.tex", "Let a Sylow subgroup act",
			map[string]string{"msc": "20D20", "authors": "bob", "related": "SylowCount", "modified": "2013-03-22"}},
		"NoType": {"entry", "an entry with no type", "20A05-NoType.tex", "An entry with no document environment",
			map[string]string{"authors": "dave"}},
	}
	for _, r := range recs {
		w, ok := wants[r.ID]
		if !ok {
			t.Errorf("the record %s is not an entry of the fixtures", r.ID)
			continue
		}
		delete(wants, r.ID)
		if r.Kind != w.kind || r.Title != w.title || r.Locator != w.locator || r.URL != planetmath.Home+"/"+strings.ToLower(r.ID) {
			t.Errorf("%s is %s %q at %s, %s, want %s %q at %s", r.ID, r.Kind, r.Title, r.Locator, r.URL, w.kind, w.title, w.locator)
		}
		if !strings.HasPrefix(r.Text, w.text) || strings.Contains(r.Text, `\end{document}`) || r.Class != ingest.Open || len(r.Fingerprints) == 0 {
			t.Errorf("%s has the class %s, %d fingerprints and the text %q", r.ID, r.Class, len(r.Fingerprints), r.Text)
		}
		if len(r.Meta) != len(w.meta) {
			t.Errorf("the meta of %s is %v, want %v", r.ID, r.Meta, w.meta)
		}
		for k, v := range w.meta {
			if r.Meta[k] != v {
				t.Errorf("the meta %s of %s is %q, want %q", k, r.ID, r.Meta[k], v)
			}
		}
	}
	for id := range wants {
		t.Errorf("the records have no %s", id)
	}
}

// TestMentions checks that the mentions of the entries keep no text, because the licence is share-alike, and that they have the kinds and the MSC codes of the entries.
func TestMentions(t *testing.T) {
	root := pull(t)
	recs, _, err := ingest.ReadRecords(root, "planetmath")
	if err != nil {
		t.Fatal(err)
	}
	src := planetmath.Adapter{}.Source()
	kinds := map[string]string{"CentreOfAGroup": "definition", "SylowCount": "statement", "ProofOfSylowCount": "proof", "NoType": inventory.Unknown}
	for _, m := range inventory.FromRecords(&src, "blake3:00", "planetmath/1", recs) {
		if m.Text != "" || m.Kind != kinds[m.NativeID] || !strings.HasPrefix(m.Locator, planetmath.Home+"/") {
			t.Errorf("the mention of %s is %+v", m.NativeID, m)
		}
		if m.NativeID == "CentreOfAGroup" && strings.Join(m.MSC, " ") != "20-00 20A05" {
			t.Errorf("the mention of CentreOfAGroup has the MSC codes %v", m.MSC)
		}
	}
}

// node writes a node with a statement that names the Sylow entry in same_as when cite is true.
func node(t *testing.T, root, statement string, cite bool) {
	same := ""
	if cite {
		same = "same_as:\n  planetmath: SylowCount\n"
	}
	body := "---\ntag: 4M0C1B\nkind: statement\nrole: theorem\nname: The number of Sylow subgroups\n" + same + "---\n\n## Statement\n\n" + statement + "\n"
	dir := filepath.Join(root, "nodes", "4M0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "4M0C1B.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCopyCheck imports the fixtures and checks the rules L02 and L03 on a node with the text of an entry and on a node in its own words. The text is open, so L02 passes. The licence is share-alike, so L03 fails when the node does not name the entry.
func TestCopyCheck(t *testing.T) {
	copied := "Let $G$ be a finite group and let $p$ be a prime. The count of the Sylow $p$-subgroups of $G$ divides the order of $G$ and leaves the remainder one on division by $p$."
	own := "In a finite group, the Sylow subgroups for one prime come in a number that is one more than a multiple of that prime."
	for _, tc := range []struct {
		name, statement string
		cite            bool
		l03             check.State
	}{
		{"copied text with no attribution", copied, false, check.Fail},
		{"copied text with attribution", copied, true, check.Pass},
		{"own words", own, false, check.Pass},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := pull(t)
			node(t, root, tc.statement, tc.cite)
			rep, err := audit.Run(root, audit.Options{Rules: []string{"L02", "L03"}})
			if err != nil {
				t.Fatal(err)
			}
			states := map[string]check.State{}
			for _, r := range rep.Results {
				states[r.Rule] = r.State
			}
			if states["L02"] != check.Pass || states["L03"] != tc.l03 {
				t.Fatalf("the results are %+v, want L02 pass and L03 %s", rep.Results, tc.l03)
			}
			for _, f := range rep.Findings {
				if !strings.Contains(f.String(), "planetmath SylowCount under CC-BY-SA-3.0") {
					t.Errorf("the finding is %s", f)
				}
			}
		})
	}
}

func TestRead(t *testing.T) {
	none := func(ingest.Record) error { return nil }
	entry := func(name, title string) string {
		return "\\pmcanonicalname{" + name + "}\n\\pmtitle{" + title + "}\n\\endmetadata\ntext\n"
	}
	for name, files := range map[string]map[string]string{
		"no entries":         {"preamble.tex": "\\usepackage{amsmath}\n", "html/a.tex": entry("A", "a")},
		"a bad name":         {"a.tex": entry("A-B", "a")},
		"a name two times":   {"a.tex": entry("A", "a"), "b.tex": entry("A", "b")},
		"no title":           {"a.tex": entry("A", " ")},
		"no end of metadata": {"a.tex": "\\pmcanonicalname{A}\n\\pmtitle{a}\n"},
	} {
		zr, err := gzip.NewReader(bytes.NewReader(archive(t, files)))
		if err != nil {
			t.Fatal(err)
		}
		if err := (planetmath.Adapter{}).Read(zr, none); err == nil {
			t.Errorf("Read of an archive with %s gives no error", name)
		}
	}
	if err := (planetmath.Adapter{}).Read(strings.NewReader("not a tar file"), none); err == nil {
		t.Error("Read of bytes that are not a tar file gives no error")
	}
}

func TestSource(t *testing.T) {
	s := planetmath.Adapter{}.Source()
	if p := s.Problems(); len(p) > 0 {
		t.Errorf("the catalogue entry has problems: %v", p)
	}
	if !ingest.ShareAlike(s.Licence) || ingest.ClassOf(s.Licence) != ingest.Open || s.Attribution == "" {
		t.Errorf("the licence %s is not share-alike, not of the class open, or has no attribution", s.Licence)
	}
	if a, ok := ingest.Lookup("planetmath"); !ok || a.Version() != "planetmath/1" || a.Dump() != planetmath.Archive(planetmath.Pilot) {
		t.Error("the adapter is not registered")
	}
}
