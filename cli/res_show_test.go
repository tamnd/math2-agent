package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/queue"
	"github.com/tamnd/math2-agent/resource"
	resscope "github.com/tamnd/math2-agent/resource/scope"
)

func showTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	rs := []resource.Resource{
		{
			ID: "8F2K1QW7ZJ", Kind: "textbook", Title: "Principles of Mathematical Analysis", Date: "1953",
			Creators: []resource.Creator{{Name: "Walter Rudin", Role: "author"}},
			IDs:      resource.IDs{"openlibrary_work": {"OL100W"}},
			Expressions: []resource.Expression{{Key: "e3", Kind: "edition", Label: "3rd edition", Language: "en", Date: "1976",
				IDs:            resource.IDs{"doi": {"10.1007/978-3-642-13797-4"}},
				Manifestations: []resource.Manifestation{{Key: "m1", Kind: "print", Publisher: "McGraw-Hill", IDs: resource.IDs{"isbn": {"9780070542358"}}, Access: &resource.Access{Class: "record", Text: "closed"}}},
			}},
			Scope:     &resource.Scope{Value: resource.In, Rule: "S1", By: "m2/test"},
			Catalogue: []resource.CatalogueRecord{{Source: "openlibrary", Snapshot: "blake3:00", NativeID: "OL100W", Level: "work"}},
			History:   []resource.Event{{Op: "add", At: "2026-10-30", By: "m2/test"}},
		},
		{
			ID: "8F2AAAAAA3", Kind: "textbook", Title: "Principles of Mathematical Analysis",
			Scope:     &resource.Scope{Value: resource.Duplicate, Target: "8F2K1QW7ZJ", By: "m2/test"},
			Catalogue: []resource.CatalogueRecord{{Source: "openlibrary", Snapshot: "blake3:00", NativeID: "OL9W", Level: "work"}},
			History:   []resource.Event{{Op: "merge", At: "2026-10-30", By: "m2/test"}},
		},
	}
	if err := resource.WriteResources(root, rs); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestResShow(t *testing.T) {
	root := showTree(t)
	code, stdout, stderr := run("res", "show", "-root", root, "8F2K1QW7ZJ")
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	for _, want := range []string{"8F2K1QW7ZJ m2://res/8F2K1QW7ZJ\n", "author: Walter Rudin\n", "scope: in, rule S1, by m2/test\n", "ids: openlibrary_work:OL100W\n", "e3 edition, 3rd edition, en, 1976 [doi:10.1007/978-3-642-13797-4]\n", "  e3/m1 print, McGraw-Hill, access record [isbn:9780070542358]\n", "catalogue: openlibrary OL100W at the level work"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the output has no %q:\n%s", want, stdout)
		}
	}
	if _, stdout, _ := run("res", "show", "-root", root, "isbn:978-0-07-054235-8"); !strings.Contains(stdout, "[isbn:9780070542358] <- the id of the query") {
		t.Errorf("the ISBN does not mark its manifestation:\n%s", stdout)
	}
	if _, stdout, _ := run("res", "show", "-root", root, "doi:10.1007/978-3-642-13797-4"); !strings.Contains(stdout, "[doi:10.1007/978-3-642-13797-4] <- the id of the query") {
		t.Errorf("the DOI does not mark its expression:\n%s", stdout)
	}
	if _, stdout, _ := run("res", "show", "-root", root, "8F2AAAAAA3"); !strings.Contains(stdout, "scope: duplicate of 8F2K1QW7ZJ, by m2/test") {
		t.Errorf("the duplicate does not name its target:\n%s", stdout)
	}
	if _, stdout, _ := run("res", "show", "-root", root, "-json", "8F2K1QW7ZJ"); !strings.Contains(stdout, `"id": "8F2K1QW7ZJ"`) {
		t.Errorf("the JSON form has no id:\n%s", stdout)
	}
	if code, _, stderr := run("res", "show", "-root", root, "doi:10.1000/none"); code != ExitFailure || !strings.Contains(stderr, "no resource has the id") {
		t.Errorf("exit code %d, stderr %q for an unknown id", code, stderr)
	}
	if code, _, _ := run("res", "show", "-root", root); code != ExitUsage {
		t.Errorf("exit code %d with no id", code)
	}
}

func TestResQueue(t *testing.T) {
	root := t.TempDir()
	if code, stdout, _ := run("res", "queue", "-root", root, "res-scope"); code != ExitOK || !strings.Contains(stdout, "is empty") {
		t.Fatalf("exit code %d, stdout %q for no queue file", code, stdout)
	}
	recs, err := mdrec.Marshal([]queue.Item{
		{Target: "8F2AAAAAA3", Name: "the classifier score 0.500 is in the middle band", Goal: "g", Priority: 0.5},
		{Target: "8F2K1QW7ZJ", Name: "the record is deferred past its date", Goal: "g", Priority: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ResScopeQueue)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := mdrec.Write(path, recs); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("res", "queue", "-root", root, "-n", "1", "res-scope")
	if code != ExitOK || stdout != "res-scope: 2 items\n1.000 8F2K1QW7ZJ the record is deferred past its date\nand 1 more\n" {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if code, _, stderr := run("res", "queue", "-root", root, "-decide", "8F2K1QW7ZJ", "-value", "in", "-reason", "a textbook of analysis", "-by", "github:octocat", "res-scope"); code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	ds, err := resscope.ReadDecisions(root)
	if err != nil || len(ds) != 1 || ds[0].ID != "8F2K1QW7ZJ" || ds[0].Kind != "scope" || ds[0].Value != "in" {
		t.Fatalf("decisions %+v, %v", ds, err)
	}
	if _, stdout, _ := run("res", "queue", "-root", root, "res-scope"); !strings.Contains(stdout, "8F2K1QW7ZJ the record is deferred past its date (decided") {
		t.Errorf("the decided item has no mark:\n%s", stdout)
	}
	if code, _, _ := run("res", "queue", "-root", root, "-decide", "8F2K1QW7ZJ 8F2AAAAAA3", "-value", "merge", "-reason", "one book", "-by", "github:octocat", "res-cluster"); code != ExitOK {
		t.Fatalf("exit code %d for a merge", code)
	}
	if ds, _ := resscope.ReadDecisions(root); len(ds) != 2 || ds[1].Kind != "merge" || ds[1].Target != "8F2AAAAAA3" {
		t.Fatalf("decisions %+v", ds)
	}
	for name, args := range map[string][]string{
		"deferred":         {"-decide", "8F2K1QW7ZJ", "-value", "deferred", "-reason", "r", "-by", "p", "res-scope"},
		"duplicate, no to": {"-decide", "8F2K1QW7ZJ", "-value", "duplicate", "-reason", "r", "-by", "p", "res-scope"},
		"one id to merge":  {"-decide", "8F2K1QW7ZJ", "-value", "merge", "-reason", "r", "-by", "p", "res-cluster"},
		"structure":        {"-decide", "8F2K1QW7ZJ", "-value", "in", "-reason", "r", "-by", "p", "res-structure"},
		"unknown queue":    {"res-nothing"},
	} {
		if code, _, _ := run(append([]string{"res", "queue", "-root", root}, args...)...); code != ExitUsage {
			t.Errorf("%s: exit code %d", name, code)
		}
	}
	if code, _, _ := run("res", "queue", "-root", root, "-decide", "8F2K1QW7ZJ", "-value", "out", "-by", "p", "res-scope"); code != ExitFailure {
		t.Errorf("exit code %d for a decision with no reason", code)
	}
}
