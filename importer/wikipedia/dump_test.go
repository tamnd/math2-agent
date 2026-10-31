package wikipedia_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/audit"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/importer/wikipedia"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/inventory"
)

const (
	dumpName = "enwiki-test-pages-articles-multistream.xml.bz2"
	lagrange = "Lagrange's theorem (group theory)"
)

// server serves the fixture dump and its index, with ranges, and records the Range header of each request.
type server struct {
	*httptest.Server
	mu     sync.Mutex
	ranges []string
}

func serve(t *testing.T) *server {
	s := &server{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(filepath.Join("testdata", filepath.Base(r.URL.Path)))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		s.mu.Lock()
		s.ranges = append(s.ranges, filepath.Base(r.URL.Path)+" "+r.Header.Get("Range"))
		s.mu.Unlock()
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(s.Close)
	return s
}

// tree writes a math2 tree whose node names the Lagrange article in same_as when cite is true, and whose mentions name more titles. A node has the statement.
func tree(t *testing.T, statement string, cite bool) string {
	root := t.TempDir()
	same := ""
	if cite {
		same = "same_as:\n  wikipedia: " + lagrange + "\n"
	}
	body := "---\ntag: 4M0C1B\nkind: statement\nrole: theorem\nname: Lagrange's theorem\n" + same + "---\n\n## Statement\n\n" + statement + "\n"
	dir := filepath.Join(root, "nodes", "4M0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "4M0C1B.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ms := []inventory.Mention{{
		ID: inventory.MentionID("thousand_plus", "Q1", 1), Source: "thousand_plus", Snapshot: "blake3:00", NativeID: "Q1", Kind: "statement", Class: "open", By: "thousand_plus/1",
		Same: map[string][]string{"wikipedia": {"sylow_theorem", "Group", "Lagrange theorem", "Nonexistent page", "Group (mathematics)#Notes"}},
	}}
	if err := inventory.WriteMentions(root, "thousand_plus", ms); err != nil {
		t.Fatal(err)
	}
	return root
}

func pull(t *testing.T, srv *server, root string) *ingest.Result {
	res, err := ingest.Pull(context.Background(), wikipedia.Adapter{}, ingest.Options{Root: root, Cache: t.TempDir(), URL: srv.URL + "/" + dumpName, Date: "2026-10-27", UserAgent: "m2/test"})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestTitles(t *testing.T) {
	got, err := wikipedia.Titles(tree(t, "A statement.", true))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Group", "Lagrange theorem", lagrange, "Nonexistent page", "Sylow theorem"}
	if !slices.Equal(got, want) {
		t.Errorf("Titles gives %q, want %q", got, want)
	}
}

func TestPull(t *testing.T) {
	srv := serve(t)
	root := tree(t, "A statement.", true)
	res := pull(t, srv, root)
	if m := res.Manifest; m.Records != 2 || m.Classes[ingest.Open] != 2 || m.Adapter != "wikipedia/1" {
		t.Errorf("the manifest is %+v, want 2 open records from wikipedia/1", m)
	}
	// The fetch gets the index, the header and the streams at 303, 948 and 1408. The stream at 1408 has the target of a redirect, so it comes in the second pass. No title of the stream at 1947 is in the tree.
	index := "enwiki-test-pages-articles-multistream-index.txt.bz2 "
	want := []string{index, dumpName + " bytes=0-302", dumpName + " bytes=303-947", dumpName + " bytes=948-1407", dumpName + " bytes=1408-1946"}
	if !slices.Equal(srv.ranges, want) {
		t.Errorf("the requests are\n%q, want\n%q", srv.ranges, want)
	}

	recs, _, err := ingest.ReadRecords(root, "wikipedia")
	if err != nil {
		t.Fatal(err)
	}
	type rec struct {
		url, text string
		meta      map[string]string
	}
	wants := map[string]rec{
		lagrange: {"https://en.wikipedia.org/wiki/Lagrange%27s_theorem_%28group_theory%29",
			"Lagrange's theorem is a result of group theory.\n\nStatement\nIn a finite group, the order of each subgroup divides the order of the group. The quotient of the two orders is the number of the cosets of the subgroup, and this number is the index of the subgroup.",
			map[string]string{"wikipedia": lagrange + "|Lagrange theorem", "description": "Theorem on the order of a subgroup", "categories": "Theorems in group theory|Joseph-Louis Lagrange", "revision": "111", "date": "2026-09-30", "contributor": "Alice"}},
		"Sylow theorems": {"https://en.wikipedia.org/wiki/Sylow_theorems",
			"The Sylow theorems say that a finite group has subgroups of each largest prime power order that divides its order, and that these subgroups are conjugate.\n\nHistory\nThey are named after a mathematician of the nineteenth century.",
			map[string]string{"wikipedia": "Sylow theorems|Sylow theorem", "description": "Theorems on the subgroups of prime power order", "categories": "Theorems in group theory", "revision": "321", "date": "2026-05-01", "contributor": "Carol"}},
	}
	for _, r := range recs {
		w, ok := wants[r.ID]
		if !ok {
			t.Errorf("the record %s is not an article that the tree names", r.ID)
			continue
		}
		delete(wants, r.ID)
		if r.Kind != "article" || r.Title != r.ID || r.URL != w.url || !strings.HasPrefix(r.Locator, wikipedia.Home+"/w/index.php?title=") || !strings.HasSuffix(r.Locator, "&oldid="+w.meta["revision"]) {
			t.Errorf("%s is %s %q at %s, %s, want article at %s", r.ID, r.Kind, r.Title, r.URL, r.Locator, w.url)
		}
		if r.Text != w.text || r.Class != ingest.Open || len(r.Fingerprints) == 0 {
			t.Errorf("%s has the class %s, %d fingerprints and the text\n%q, want\n%q", r.ID, r.Class, len(r.Fingerprints), r.Text, w.text)
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

	// The mentions keep no text, because the licence is share-alike, and they name the redirects, so that the inventory joins them with the mentions of the other sources.
	src := wikipedia.Adapter{}.Source()
	for _, m := range inventory.FromRecords(&src, res.Snapshot.Hash, "wikipedia/1", recs) {
		if m.Text != "" || m.Kind != inventory.Unknown || len(m.Same["wikipedia"]) != 2 || m.Same["wikipedia"][0] != m.NativeID {
			t.Errorf("the mention of %s is %+v", m.NativeID, m)
		}
	}
}

// TestCopyCheck imports the fixture and checks the rules L02 and L03 on a node with the text of an article and on a node in its own words. The text is open, so L02 passes. The licence is share-alike, so L03 fails when the node does not name the article.
func TestCopyCheck(t *testing.T) {
	srv := serve(t)
	copied := "In a finite group, the order of each subgroup divides the order of the group. The quotient of the two orders is the number of the cosets of the subgroup, and this number is the index of the subgroup."
	own := "The number of elements of a subgroup of a finite group is a factor of the number of elements of the group."
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
			root := tree(t, tc.statement, true)
			pull(t, srv, root)
			path := filepath.Join(root, "nodes", "4M0", "4M0C1B.md")
			body := "---\ntag: 4M0C1B\nkind: statement\nrole: theorem\nname: Lagrange's theorem\n"
			if tc.cite {
				body += "same_as:\n  wikipedia: " + lagrange + "\n"
			}
			if err := os.WriteFile(path, []byte(body+"---\n\n## Statement\n\n"+tc.statement+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
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
				if !strings.Contains(f.String(), "wikipedia "+lagrange+" under CC-BY-SA-4.0") {
					t.Errorf("the finding is %s", f)
				}
			}
		})
	}
}

func TestFetch(t *testing.T) {
	srv := serve(t)
	ctx := context.Background()
	pullURL := func(root, url string) error {
		_, err := ingest.Pull(ctx, wikipedia.Adapter{}, ingest.Options{Root: root, Cache: t.TempDir(), URL: url, Date: "2026-10-27"})
		return err
	}
	if err := pullURL(tree(t, "A.", true), srv.URL+"/enwiki-test-pages-articles.xml.bz2"); err == nil || !strings.Contains(err.Error(), "not a multistream dump") {
		t.Errorf("a dump that is not multistream gives %v", err)
	}
	if err := pullURL(t.TempDir(), srv.URL+"/"+dumpName); err == nil || !strings.Contains(err.Error(), "names no Wikipedia titles") {
		t.Errorf("a tree with no titles gives %v", err)
	}
	none := t.TempDir()
	ms := []inventory.Mention{{ID: inventory.MentionID("thousand_plus", "Q1", 1), Source: "thousand_plus", Snapshot: "blake3:00", NativeID: "Q1", Kind: "statement", Class: "open", By: "thousand_plus/1", Same: map[string][]string{"wikipedia": {"Nonexistent page"}}}}
	if err := inventory.WriteMentions(none, "thousand_plus", ms); err != nil {
		t.Fatal(err)
	}
	if err := pullURL(none, srv.URL+"/"+dumpName); err == nil || !strings.Contains(err.Error(), "no title that the tree names is in the dump") {
		t.Errorf("a tree with no title in the dump gives %v", err)
	}
	if err := (wikipedia.Adapter{}).Fetch(ctx, &ingest.Request{URL: srv.URL + "/" + dumpName}, new(bytes.Buffer)); err == nil {
		t.Error("a request with no range getter gives no error")
	}
}

func TestRead(t *testing.T) {
	none := func(ingest.Record) error { return nil }
	page := func(title, text string) string {
		return `<mediawiki><page><title>` + title + `</title><ns>0</ns><id>1</id><revision><id>2</id><text>` + text + `</text></revision></page></mediawiki>`
	}
	for name, dump := range map[string]string{
		"no pages":                 `<mediawiki></mediawiki>`,
		"only a redirect":          `<mediawiki><page><title>A</title><ns>0</ns><id>1</id><redirect title="B"/><revision><id>2</id><text>#REDIRECT [[B]]</text></revision></page></mediawiki>`,
		"only a disambiguation":    page("A", "{{dab}}"),
		"only a template":          `<mediawiki><page><title>Template:A</title><ns>10</ns><id>1</id><revision><id>2</id><text>a</text></revision></page></mediawiki>`,
		"a bad title":              page("A {b}", "text"),
		"not a dump":               `<html><page><title>A</title></page></html>`,
		"a dump that does not end": `<mediawiki><page>`,
	} {
		if err := (wikipedia.Adapter{}).Read(strings.NewReader(dump), none); err == nil {
			t.Errorf("Read of an export with %s gives no error", name)
		}
	}
}

func TestIndex(t *testing.T) {
	got, err := wikipedia.Index(wikipedia.Dump)
	if want := "https://dumps.wikimedia.org/enwiki/latest/enwiki-latest-pages-articles-multistream-index.txt.bz2"; err != nil || got != want {
		t.Errorf("Index gives %q, %v, want %q", got, err, want)
	}
}

func TestSource(t *testing.T) {
	s := wikipedia.Adapter{}.Source()
	if p := s.Problems(); len(p) > 0 {
		t.Errorf("the catalogue entry has problems: %v", p)
	}
	if !ingest.ShareAlike(s.Licence) || ingest.ClassOf(s.Licence) != ingest.Open || s.Attribution == "" {
		t.Errorf("the licence %s is not share-alike, not of the class open, or has no attribution", s.Licence)
	}
	a, ok := ingest.Lookup("wikipedia")
	if !ok || a.Version() != "wikipedia/1" || a.Dump() != wikipedia.Dump {
		t.Fatal("the adapter is not registered")
	}
	if w, ok := a.(wikipedia.Adapter); !ok || w.Pause != time.Second {
		t.Errorf("the registered adapter is %+v, want a pause of one second", a)
	}
}
