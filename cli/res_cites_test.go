package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/resource"
)

func TestResResolveCrossref(t *testing.T) {
	root := showTree(t)
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Query().Get("mailto") != "" {
			t.Error("the request has a mailto")
		}
		_, _ = io.WriteString(w, `{"message":{"reference":[{"key":"r1","unstructured":"W. Rudin, Principles of Mathematical Analysis, 3rd ed., McGraw-Hill, 1976."},{"key":"r2","author":"Nobody","article-title":"A lost paper","year":"1950"}]}}`)
	}))
	defer srv.Close()
	code, _, stderr := run("res", "resolve", "-root", root, "-gap", "0", "-source", "crossref", "-crossref", srv.URL)
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if len(paths) != 1 || paths[0] != "/works/10.1007/978-3-642-13797-4" || !strings.Contains(stderr, "crossref: 1 works, 2 references, 1 matched") {
		t.Errorf("paths %q, stderr %q", paths, stderr)
	}
	cs, err := resource.ReadCites(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 || cs[0].From != "8F2K1QW7ZJ/e3" || cs[0].To != "8F2K1QW7ZJ/e3" || cs[0].Text != "" || cs[1].To != "" || cs[1].Parsed.Title != "A lost paper" {
		t.Errorf("cites %+v", cs)
	}
	if code, _, _ := run("res", "resolve", "-root", root, "-source", "nothing"); code != ExitUsage {
		t.Errorf("exit code %d for an unknown source", code)
	}
	if _, err := readResQueue(filepath.Join(root, ResResolveQueue)); err == nil {
		t.Error("a run with no candidates writes the queue")
	}
}
