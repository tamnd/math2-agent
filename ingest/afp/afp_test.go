package afp_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/afp"
)

// archive makes a git archive of the files, like the one GitHub gives, at the commit.
func archive(t *testing.T, commit string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": commit}}); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		h := &tar.Header{Name: "mirror-afp-devel-" + commit + "/" + name, Mode: 0o644, Size: int64(len(body)), ModTime: time.Unix(0, 0)}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fixtures gives the files under testdata and the extra files.
func fixtures(t *testing.T, extra map[string]string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := os.ReadFile(p) // #nosec G304 -- a fixture of the test.
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("testdata", p)
		files[filepath.ToSlash(rel)] = string(body)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	maps.Copy(files, extra)
	return files
}

// unzip gives the tar archive in a git archive, which Import unzips for Read.
func unzip(t *testing.T, body []byte) io.Reader {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return zr
}

func read(t *testing.T, body []byte) map[string]ingest.Record {
	t.Helper()
	a := afp.Adapter{}
	src := a.Source()
	recs, err := ingest.Import(a, &src, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]ingest.Record{}
	for _, r := range recs {
		out[r.ID] = r
	}
	return out
}

func TestRead(t *testing.T) {
	recs := read(t, archive(t, afp.Commit, fixtures(t, nil)))
	const toy = "Toy_Entry.Toy."
	want := map[string][2]string{
		"Toy_Entry":                    {"entry", ""},
		"Toy_Lgpl":                     {"entry", ""},
		toy + "double":                 {"definition", "definition"},
		toy + "double_zero":            {"statement", "lemma"},
		toy + "double_add":             {"statement", "theorem"},
		toy + "half":                   {"definition", "fun"},
		toy + "count":                  {"definition", "function"},
		toy + "tree":                   {"definition", "datatype"},
		toy + "pair2":                  {"definition", "datatype"},
		toy + "point":                  {"definition", "record"},
		toy + "monoid_toy":             {"definition", "locale"},
		toy + "monoid_toy.assoc_again": {"statement", "lemma"},
		toy + "monoid_toy.assoc_twice": {"statement", "lemma"},
		toy + "monoid_toy.in_context":  {"statement", "corollary"},
		toy + "anonymous_context":      {"statement", "proposition"},
		toy + "even_toy":               {"definition", "inductive"},
		"Toy_Entry.Toy_More.sum_to":    {"definition", "primrec"},
		"Toy_Lgpl.Lg.lg_fact":          {"statement", "lemma"},
	}
	for id, w := range want {
		r, ok := recs[id]
		if !ok {
			t.Errorf("no record %s", id)
			continue
		}
		if r.Kind != w[0] || r.Meta["keyword"] != w[1] {
			t.Errorf("the record %s is %+v", id, r)
		}
	}
	if len(recs) != len(want) {
		var ids []string
		for id := range recs {
			if _, ok := want[id]; !ok {
				ids = append(ids, id)
			}
		}
		t.Errorf("the archive gives %d records, want %d. The others are %v", len(recs), len(want), ids)
	}
	e := recs["Toy_Entry"]
	if e.Title != "A Toy Entry" || e.URL != afp.Site+"Toy_Entry.html" || e.Licence != afp.BSD || e.Class != ingest.Open {
		t.Errorf("the record of the BSD entry is %+v", e)
	}
	if e.Text != "This entry is a fixture of the tests. It proves that “x + 0 = x” in a toy setting. The text is ours." {
		t.Errorf("the text of the BSD entry is %q", e.Text)
	}
	if e.Meta["topics"] != "Mathematics/Algebra|Logic/Proof theory" || e.Meta["date"] != "2024-05-06" || e.Meta["authors"] != "Ada Tester|Bo Sample" {
		t.Errorf("the meta of the BSD entry is %v", e.Meta)
	}
	l := recs["Toy_Lgpl"]
	if l.Licence != afp.LGPL || l.Class != ingest.RecordOnly || l.Text != "" || l.Meta["authors"] != "unknown_person" {
		t.Errorf("the record of the LGPL entry is %+v", l)
	}
	if f := recs["Toy_Lgpl.Lg.lg_fact"]; f.Licence != afp.LGPL || f.Class != ingest.RecordOnly {
		t.Errorf("a lemma of the LGPL entry is %+v", f)
	}
	d := recs[toy+"double_zero"]
	if d.Title != "double_zero" || d.Locator != "thys/Toy_Entry/Toy.thy:15" || d.URL != afp.Repository+"/blob/"+afp.Commit+"/thys/Toy_Entry/Toy.thy#L15" || d.Text != "" || d.Class != ingest.Open {
		t.Errorf("the record of double_zero is %+v", d)
	}
	if m := recs[toy+"monoid_toy.in_context"].Meta; m["locale"] != "monoid_toy" || m["entry"] != "Toy_Entry" || m["theory"] != "Toy" {
		t.Errorf("the meta of in_context is %v", m)
	}
}

func TestReadErrors(t *testing.T) {
	cases := map[string][]byte{
		"no commit":   archive(t, "", fixtures(t, nil)),
		"bad licence": archive(t, afp.Commit, fixtures(t, map[string]string{"metadata/entries/Toy_Lgpl.toml": "title = \"x\"\nlicense = \"gpl\"\n"})),
		"no entries":  archive(t, afp.Commit, map[string]string{"thys/No_Meta/X.thy": "theory X begin end\n"}),
	}
	for name, body := range cases {
		err := (afp.Adapter{}).Read(unzip(t, body), func(ingest.Record) error { return nil })
		if err == nil {
			t.Errorf("%s: Read gives no error", name)
		} else {
			t.Logf("%s: %v", name, err)
		}
	}
}

func TestReadBrokenTheory(t *testing.T) {
	for _, body := range []string{"theory Bad (* no end\nlemma x: \"True\"\n", "lemma x: \"True\n", "text \\<open>no end\nlemma y: \"True\"\n"} {
		recs := read(t, archive(t, afp.Commit, fixtures(t, map[string]string{"thys/Toy_Entry/Bad.thy": body})))
		for id := range recs {
			if strings.HasPrefix(id, "Toy_Entry.Bad.") {
				t.Errorf("a theory that does not lex gives %s", id)
			}
		}
		if _, ok := recs["Toy_Entry.Toy.double"]; !ok {
			t.Errorf("a theory that does not lex stops the read")
		}
	}
}

func TestPull(t *testing.T) {
	body := archive(t, afp.Commit, fixtures(t, nil))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer srv.Close()
	res, err := ingest.Pull(context.Background(), afp.Adapter{}, ingest.Options{Root: t.TempDir(), Cache: t.TempDir(), URL: srv.URL + "/a.tar.gz", Date: "2026-10-27"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Snapshot.Commit != afp.Commit || res.Manifest.Records != 18 || res.Manifest.Classes[ingest.Open] != 16 || res.Manifest.Classes[ingest.RecordOnly] != 2 {
		t.Errorf("the pull gives the commit %s and the manifest %+v", res.Snapshot.Commit, res.Manifest)
	}
	other := archive(t, strings.Repeat("a", 40), fixtures(t, nil))
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(other) }))
	defer srv2.Close()
	if _, err := ingest.Pull(context.Background(), afp.Adapter{}, ingest.Options{Root: t.TempDir(), Cache: t.TempDir(), URL: srv2.URL + "/a.tar.gz", Date: "2026-10-27"}); err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Errorf("a pull of another commit gives %v", err)
	}
	if got := (afp.Adapter{}).Dump(); got != afp.Repository+"/archive/"+afp.Commit+".tar.gz" {
		t.Errorf("Dump gives %s", got)
	}
}
