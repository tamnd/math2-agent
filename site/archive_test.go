package site

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/schema"
)

// unzip writes the files of the archive to a new directory and returns it.
func unzip(t *testing.T, data []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatal(err)
		}
		_ = rc.Close()
		writeFile(t, dir, f.Name, buf.String())
	}
	return dir
}

func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

func TestArchiveFixtures(t *testing.T) {
	out := buildFixtures(t, true)
	var a, b bytes.Buffer
	if err := Archive(&a, out, "/math2", "math2-test"); err != nil {
		t.Fatal(err)
	}
	if err := Archive(&b, out, "/math2/", "math2-test"); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Error("two archives of the same site are not the same bytes")
	}
	dir := unzip(t, a.Bytes())
	if got := listFiles(t, dir); len(got) == 0 || !strings.HasPrefix(got[0], "math2-test/") {
		t.Fatalf("the archive has no folder math2-test: %v", got[:min(3, len(got))])
	}
	root := filepath.Join(dir, "math2-test")

	// The archive has the same pages as the site.
	want := append(listFiles(t, out), "README.txt", "assets/search.js")
	slices.Sort(want)
	if got := listFiles(t, root); !slices.Equal(got, want) {
		t.Errorf("the archive has %d files, and the site and the two added files are %d", len(got), len(want))
	}

	// The archive meets each target, with no base path.
	r, err := Check(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if !r.OK() {
		t.Errorf("the archive misses a target: %v", r.Problems)
	}

	// Each link into the site is relative and names a file.
	attr := regexp.MustCompile(`\s(href|src|action)="([^"]*)"`)
	for _, rel := range listFiles(t, root) {
		if path.Ext(rel) != ".html" {
			continue
		}
		page := readFile(t, root, rel)
		for _, m := range attr.FindAllStringSubmatch(page, -1) {
			ref := m[2]
			if ref == "" || strings.HasPrefix(ref, "#") || strings.Contains(ref, ":") {
				continue
			}
			if strings.HasPrefix(ref, "/") {
				t.Errorf("%s: the link %s is not relative", rel, ref)
				continue
			}
			file, ok := resolve("", rel, ref)
			if !ok {
				t.Errorf("%s: the link %s is outside the archive", rel, ref)
				continue
			}
			// The fixtures have no schema file, so a link to it is broken on the site too.
			if st, err := os.Stat(filepath.Join(root, filepath.FromSlash(file))); (err != nil || st.IsDir()) && file != schema.File {
				t.Errorf("%s: the link %s names no file", rel, ref)
			}
		}
	}

	node := readNode(t, root, "Q529ES")
	for _, s := range []string{`<body data-base="../../" `, `href="../../assets/site.css"`, `href="../../nodes/index.html"`, `href="../../index.html"`, `action="../../search/index.html"`} {
		if !strings.Contains(node, s) {
			t.Errorf("the node page has no %s", s)
		}
	}
	if search := readFile(t, root, "search/index.html"); !strings.Contains(search, `<script defer src="../assets/search.js"></script>`+"\n"+`<script defer src="../assets/site.js"></script>`) {
		t.Error("the search page does not load assets/search.js before assets/site.js")
	}
	index := readFile(t, root, "assets/search.js")
	if !strings.Contains(index, "window.m2Search = [") || !strings.Contains(index, `"u":"wiki/lagranges-theorem/index.html"`) || strings.Contains(index, "/math2/") {
		t.Errorf("the search index of the archive is wrong: %.300s", index)
	}
}

func TestArchiveErrors(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	if err := Archive(&buf, dir, "", "x"); err == nil {
		t.Error("a site with no api/search.json gives no error")
	}
	writeFile(t, dir, "api/search.json", "[]")
	if err := Archive(&buf, dir, "", ""); err == nil {
		t.Error("an empty folder name gives no error")
	}
	writeFile(t, dir, "README.txt", "x")
	if err := Archive(&buf, dir, "", "x"); err == nil {
		t.Error("a site with a README.txt gives no error")
	}
}

func TestRelative(t *testing.T) {
	for _, c := range []struct {
		dir, p string
		prefix bool
		want   string
	}{
		{".", "", false, "index.html"},
		{".", "", true, "./"},
		{".", "nodes/", false, "nodes/index.html"},
		{"node/A", "node/A/", false, "index.html"},
		{"node/A", "node/B/#x", false, "../B/index.html#x"},
		{"node/A", "", true, "../../"},
		{"node/A", "assets/site.css", false, "../../assets/site.css"},
		{"search", "search/?q=a", false, "index.html?q=a"},
		{"msc/20", "msc/", true, "../"},
	} {
		if got := relative(c.dir, c.p, c.prefix); got != c.want {
			t.Errorf("relative(%q, %q, %v) = %q, want %q", c.dir, c.p, c.prefix, got, c.want)
		}
	}
	for _, c := range []struct {
		base, u, want string
		ok            bool
	}{
		{"/math2", "/math2/node/A/", "node/A/", true},
		{"/math2", "/math2", "", true},
		{"/math2", "/math2#x", "#x", true},
		{"/math2", "/math20/", "", false},
		{"/math2", "//cdn.example.org/a.js", "", false},
		{"/math2", "https://example.org/math2/", "", false},
		{"", "/a/b", "a/b", true},
	} {
		got, ok := sitePath(c.base, c.u)
		if got != c.want || ok != c.ok {
			t.Errorf("sitePath(%q, %q) = %q, %v, want %q, %v", c.base, c.u, got, ok, c.want, c.ok)
		}
	}
}
