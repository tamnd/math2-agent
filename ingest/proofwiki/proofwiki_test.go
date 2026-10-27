package proofwiki_test

import (
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
	"github.com/tamnd/math2-agent/ingest/proofwiki"
	"github.com/tamnd/math2-agent/inventory"
)

// pull imports the fixture dump, compressed with gzip as the site gives it, into a new root.
func pull(t *testing.T) string {
	b, err := os.ReadFile(filepath.Join("testdata", "dump.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write(b)
	_ = zw.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(gz.Bytes()) }))
	defer srv.Close()
	root := t.TempDir()
	res, err := ingest.Pull(context.Background(), proofwiki.Adapter{}, ingest.Options{Root: root, Cache: t.TempDir(), URL: srv.URL + "/latest.xml.gz", Date: "2026-10-27"})
	if err != nil {
		t.Fatal(err)
	}
	if m := res.Manifest; m.Records != 5 || m.Classes[ingest.Open] != 5 || m.Adapter != "proofwiki/1" {
		t.Errorf("the manifest is %+v, want 5 open records from proofwiki/1", m)
	}
	return root
}

func TestPull(t *testing.T) {
	root := pull(t)
	recs, _, err := ingest.ReadRecords(root, "proofwiki")
	if err != nil {
		t.Fatal(err)
	}
	const theorem = "Order of Subgroup Divides Order of Group"
	type want struct {
		kind, title, url, text string
		meta                   map[string]string
	}
	wants := map[string]want{
		theorem: {"theorem", theorem, "https://proofwiki.org/wiki/Order_of_Subgroup_Divides_Order_of_Group",
			"Let $G$ be a finite group and let $H$ be a subgroup of $G$.\n\nThen the order of $H$ is a divisor of the order of $G$.",
			map[string]string{"categories": "Subgroups|Order of Groups", "revision": "20", "date": "2025-05-27", "contributor": "Alice"}},
		theorem + "/Proof 1": {"proof", theorem + "/Proof 1", "https://proofwiki.org/wiki/Order_of_Subgroup_Divides_Order_of_Group/Proof_1",
			"The left cosets of $H$ in $G$ split $G$ into parts, and each part has as many elements as $H$ has. So the order of $G$ is the number of cosets times the order of $H$.",
			map[string]string{"categories": theorem, "proves": theorem, "revision": "30", "date": "2025-12-23", "contributor": "192.0.2.7"}},
		"Definition:Centre of Group": {"definition", "Centre of Group", "https://proofwiki.org/wiki/Definition:Centre_of_Group",
			"Definition\nThe centre of a group $G$ is the set of the elements of $G$ that commute with all the elements of $G$: $\\set {x \\in G: \\forall g \\in G: x g = g x}$",
			map[string]string{"categories": "Definitions/Group Theory", "revision": "40", "date": "2025-05-28", "contributor": "Bob"}},
		"Axiom:Closure Axiom": {"axiom", "Closure Axiom", "https://proofwiki.org/wiki/Axiom:Closure_Axiom",
			"The product of two elements of the structure is again an element of the structure.",
			map[string]string{"categories": "Axioms/Group Theory", "revision": "50", "date": "2026-01-11", "contributor": "Bob"}},
		"A Puzzle With Groups": {"page", "A Puzzle With Groups", "https://proofwiki.org/wiki/A_Puzzle_With_Groups",
			"Puzzle\nFind all the groups of order four up to isomorphism, and show that there are two of them.",
			map[string]string{"revision": "60", "date": "2026-02-02", "contributor": "Carol"}},
	}
	for _, r := range recs {
		w, ok := wants[r.ID]
		if !ok {
			t.Errorf("the record %s is not a page to read", r.ID)
			continue
		}
		delete(wants, r.ID)
		if r.Kind != w.kind || r.Title != w.title || r.URL != w.url || !strings.HasPrefix(r.Locator, proofwiki.Home+"/w/index.php?title=") || !strings.HasSuffix(r.Locator, "&oldid="+w.meta["revision"]) {
			t.Errorf("%s is %s %q at %s, %s, want %s %q at %s", r.ID, r.Kind, r.Title, r.URL, r.Locator, w.kind, w.title, w.url)
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
}

// TestMentions checks that the mentions keep no text, because the licence is share-alike, and that they have the kinds of the pages.
func TestMentions(t *testing.T) {
	root := pull(t)
	recs, _, err := ingest.ReadRecords(root, "proofwiki")
	if err != nil {
		t.Fatal(err)
	}
	src := proofwiki.Adapter{}.Source()
	kinds := map[string]string{"theorem": "statement", "proof": "proof", "definition": "definition", "axiom": "statement", "page": inventory.Unknown}
	byID := map[string]string{}
	for _, r := range recs {
		byID[r.ID] = r.Kind
	}
	for _, m := range inventory.FromRecords(&src, "blake3:00", "proofwiki/1", recs) {
		if m.Text != "" || m.Kind != kinds[byID[m.NativeID]] || !strings.HasPrefix(m.Locator, proofwiki.Home+"/") {
			t.Errorf("the mention of %s is %+v", m.NativeID, m)
		}
	}
}

// node writes a node with a statement that names the theorem in same_as when cite is true.
func node(t *testing.T, root, statement string, cite bool) {
	same := ""
	if cite {
		same = "same_as:\n  proofwiki: Order of Subgroup Divides Order of Group\n"
	}
	body := "---\ntag: 4M0C1B\nkind: statement\nrole: theorem\nname: Lagrange's theorem\n" + same + "---\n\n## Statement\n\n" + statement + "\n"
	dir := filepath.Join(root, "nodes", "4M0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "4M0C1B.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCopyCheck imports the fixture and checks rules L02 and L03 on a node with the text of a page and on a node in its own words. The text is open, so L02 passes. The licence is share-alike, so L03 fails when the node does not name the page.
func TestCopyCheck(t *testing.T) {
	copied := "Let $G$ be a finite group and let $H$ be a subgroup of $G$. Then the order of $H$ is a divisor of the order of $G$."
	own := "In a finite group, the number of elements of each subgroup divides the number of elements of the group."
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
				if !strings.Contains(f.String(), "proofwiki Order of Subgroup Divides Order of Group under CC-BY-SA-3.0") {
					t.Errorf("the finding is %s", f)
				}
			}
		})
	}
}

func TestRead(t *testing.T) {
	none := func(ingest.Record) error { return nil }
	page := func(title, text string) string {
		return `<mediawiki><page><title>` + title + `</title><ns>0</ns><id>1</id><revision><id>2</id><text>` + text + `</text></revision></page></mediawiki>`
	}
	for name, dump := range map[string]string{
		"no pages":             `<mediawiki></mediawiki>`,
		"only a redirect":      `<mediawiki><page><title>A</title><ns>0</ns><id>1</id><redirect title="B"/><revision><id>2</id><text>#REDIRECT [[B]]</text></revision></page></mediawiki>`,
		"a bad title":          page("A [b]", "text"),
		"a page with no title": page("", "text"),
		"not a dump":           `<html><page><title>A</title></page></html>`,
		"bad XML":              `<mediawiki><page>`,
	} {
		if err := (proofwiki.Adapter{}).Read(strings.NewReader(dump), none); err == nil {
			t.Errorf("Read of a dump with %s gives no error", name)
		}
	}
}

func TestSource(t *testing.T) {
	s := proofwiki.Adapter{}.Source()
	if p := s.Problems(); len(p) > 0 {
		t.Errorf("the catalogue entry has problems: %v", p)
	}
	if !ingest.ShareAlike(s.Elected()) || ingest.ClassOf(s.Elected()) != ingest.Open || len(ingest.Choices(s.Licence)) != 2 {
		t.Errorf("the licence %s with the election %s is not a share-alike choice of the class open", s.Licence, s.Election)
	}
	if a, ok := ingest.Lookup("proofwiki"); !ok || a.Version() != "proofwiki/1" || a.Dump() != proofwiki.Dump {
		t.Error("the adapter is not registered")
	}
}
