package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	rescluster "github.com/tamnd/math2-agent/resource/cluster"
)

func TestResImportRefs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/10.4007/annals.2001.1") {
			_, _ = w.Write([]byte(`{"message":{}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	saved := refsBases
	t.Cleanup(func() { refsBases = saved })
	refsBases = map[string]string{"doi": srv.URL + "/works/"}

	root := t.TempDir()
	in := &resource.Scope{Value: resource.In}
	if err := resource.WriteResources(root, []resource.Resource{{ID: "8F2AAAAAA3", Kind: "article", Title: "A", Scope: in}, {ID: "8F2BBBBBB6", Kind: "article", Title: "B", Scope: in}}); err != nil {
		t.Fatal(err)
	}
	weil := &resource.Parsed{Authors: []string{"André Weil"}, Title: "Basic Number Theory", Year: "1967"}
	if err := resource.WriteCites(root, []resource.Cite{
		{From: "8F2AAAAAA3/e1", N: 1, Parsed: weil},
		{From: "8F2BBBBBB6/e1", N: 1, Parsed: weil},
		{From: "8F2AAAAAA3/e1", N: 2, Parsed: &resource.Parsed{Title: "Lone paper", Authors: []string{"B. Lee"}, Year: "2001", IDs: resource.IDs{"doi": {"10.4007/annals.2001.1"}}}},
		{From: "8F2AAAAAA3/e1", N: 3, Parsed: &resource.Parsed{Title: "Gone", Authors: []string{"C. Kim"}, Year: "2001", IDs: resource.IDs{"doi": {"10.4007/gone"}}}},
	}); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("res", "import", "-root", root, "-lookup", "-gap", "0", "refs")
	if code != ExitOK || !strings.Contains(stdout, "refs: 4 unresolved references, 2 records (1 cited by two or more, 1 by id, 0 by a person), 2 ids looked up, 0 rejected") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	recs, err := rescluster.ReadRecords(root)
	if err != nil || len(recs) != 2 {
		t.Fatalf("records %+v, %v", recs, err)
	}
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		t.Fatal(err)
	}
	src := cat.Source("refs")
	if src == nil || len(src.Snapshots) != 1 || src.Snapshots[0].Hash != recs[0].Snapshot {
		t.Fatalf("source %+v", src)
	}
	// The same records give the same snapshot, and other records give a new one.
	if code, _, _ := run("res", "import", "-root", root, "-lookup", "-gap", "0", "refs"); code != ExitOK {
		t.Fatalf("exit code %d", code)
	}
	if cat, _ := ingest.ReadCatalogue(root); len(cat.Source("refs").Snapshots) != 1 {
		t.Errorf("the same records have a new snapshot: %+v", cat.Source("refs").Snapshots)
	}
	if code, _, _ := run("res", "import", "-root", root, "refs"); code != ExitOK {
		t.Fatalf("exit code %d", code)
	}
	if cat, _ := ingest.ReadCatalogue(root); len(cat.Source("refs").Snapshots) != 2 {
		t.Errorf("a build with no lookup has other records, and it needs its own snapshot: %+v", cat.Source("refs").Snapshots)
	}
}

func TestResImportS2(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%s %s", r.Method, r.Header.Get("Content-Type"))
		}
		_, _ = w.Write([]byte(`[{"corpusId":42,"title":"A","year":2001,"externalIds":{"DOI":"10.1/a"}},null]`))
	}))
	defer srv.Close()
	saved := s2URL
	t.Cleanup(func() { s2URL = saved })
	s2URL = srv.URL

	root := t.TempDir()
	in := &resource.Scope{Value: resource.In}
	if err := resource.WriteResources(root, []resource.Resource{
		{ID: "8F2AAAAAA3", Kind: "article", Title: "A", Scope: in, IDs: resource.IDs{"doi": {"10.1/a"}}},
		{ID: "8F2BBBBBB6", Kind: "article", Title: "B", Scope: in, IDs: resource.IDs{"doi": {"10.1/b"}}},
	}); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("res", "import", "-root", root, "-gap", "0", "s2")
	if code != ExitOK || !strings.Contains(stdout, "s2: asked 2, found 1, records 1, rejected 0, requests 1") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	recs, err := rescluster.ReadRecords(root)
	if err != nil || len(recs) != 1 || recs[0].IDs["s2"][0] != "42" || recs[0].Source != "s2" {
		t.Fatalf("records %+v, %v", recs, err)
	}
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		t.Fatal(err)
	}
	if src := cat.Source("s2"); src == nil || len(src.Snapshots) != 1 || src.Licence != "ODC-By-1.0" {
		t.Fatalf("source %+v", src)
	}
}

func TestResImportZbsearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch q := r.URL.Query().Get("search_string"); q {
		case "ti:random walk and the heat equation & py:2010 & au:lawler":
			_, _ = w.Write([]byte(`{"result":[{"id":5826084,"identifier":"1213.60001","title":{"title":"Random walk and the heat equation"},"year":"2010","language":{"languages":["English"]},"msc":[{"code":"60-01","scheme":"msc2020"}],"contributors":{"authors":[{"name":"Lawler, Gregory F."}]}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	saved := zbsearchURL
	t.Cleanup(func() { zbsearchURL = saved })
	zbsearchURL = srv.URL

	root := t.TempDir()
	in := &resource.Scope{Value: resource.In}
	if err := resource.WriteResources(root, []resource.Resource{
		{ID: "8F2AAAAAA3", Kind: "monograph", Title: "Random Walk and the Heat Equation", Date: "2010", Creators: []resource.Creator{{Name: "Gregory F. Lawler", Role: "author"}}, Scope: in, IDs: resource.IDs{"doi": {"10.1090/stml/055"}}},
		{ID: "8F2BBBBBB6", Kind: "monograph", Title: "B", Date: "2001", Scope: in, IDs: resource.IDs{"isbn": {"9780000000002"}}},
	}); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("res", "import", "-root", root, "-gap", "0", "zbmath_search")
	if code != ExitOK || !strings.Contains(stdout, "zbmath_search: asked 2, found 1, records 1, rejected 0") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	recs, err := rescluster.ReadRecords(root)
	if err != nil || len(recs) != 1 || recs[0].Source != "zbmath_search" || recs[0].IDs["doi"][0] != "10.1090/stml/055" || recs[0].Language != "en" || recs[0].MSC[0] != "60-01" {
		t.Fatalf("records %+v, %v", recs, err)
	}
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		t.Fatal(err)
	}
	if src := cat.Source("zbmath_search"); src == nil || len(src.Snapshots) != 1 || src.Licence != "CC-BY-SA-4.0" {
		t.Fatalf("source %+v", src)
	}
}

func TestResImportCrlang(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/10.1090%2Fstml%2F055":
			_, _ = w.Write([]byte(`{"message":{"DOI":"10.1090/stml/055","language":"en"}}`))
		case "/10.1007%2Fbfb0059573":
			_, _ = w.Write([]byte(`{"message":{"DOI":"10.1007/bfb0059573"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	saved := crlangURL
	t.Cleanup(func() { crlangURL = saved })
	crlangURL = srv.URL + "/"

	root := t.TempDir()
	in := &resource.Scope{Value: resource.In}
	if err := resource.WriteResources(root, []resource.Resource{
		{ID: "8F2AAAAAA3", Kind: "monograph", Title: "A", Scope: in, IDs: resource.IDs{"doi": {"10.1090/stml/055"}}},
		{ID: "8F2BBBBBB6", Kind: "monograph", Title: "B", Scope: in, IDs: resource.IDs{"doi": {"10.1007/bfb0059573"}}},
		{ID: "8F2CCCCCC9", Kind: "monograph", Title: "C", Scope: in, IDs: resource.IDs{"doi": {"10.1007/none"}}},
	}); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("res", "import", "-root", root, "-gap", "0", "crossref_language")
	if code != ExitOK || !strings.Contains(stdout, "crossref_language: asked 3, found 1, records 1, rejected 0") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	recs, err := rescluster.ReadRecords(root)
	if err != nil || len(recs) != 1 || recs[0].Source != "crossref_language" || recs[0].IDs["doi"][0] != "10.1090/stml/055" || recs[0].Language != "en" {
		t.Fatalf("records %+v, %v", recs, err)
	}
}

func TestResImportArxivText(t *testing.T) {
	checkout := t.TempDir()
	p := filepath.Join(checkout, "analysis", "objects", "2101", "2101.00001.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.Repeat("Let the map be open. Then we have that this is the claim of the theorem, and it is true for each point. ", 5)), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	in := &resource.Scope{Value: resource.In}
	if err := resource.WriteResources(root, []resource.Resource{
		{ID: "8F2AAAAAA3", Kind: "preprint", Title: "A", Date: "2021", Scope: in, IDs: resource.IDs{"arxiv": {"2101.00001"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := run("res", "import", "-root", root, "arxiv_text"); code != ExitFailure {
		t.Errorf("no -url: exit code %d", code)
	}
	code, stdout, stderr := run("res", "import", "-root", root, "-url", checkout, "arxiv_text")
	if code != ExitOK || !strings.Contains(stdout, "arxiv_text: read 1, found 1, records 1, rejected 0") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	recs, err := rescluster.ReadRecords(root)
	if err != nil || len(recs) != 1 || recs[0].Language != "en" || recs[0].IDs["arxiv"][0] != "2101.00001" {
		t.Fatalf("records %+v, %v", recs, err)
	}
}
