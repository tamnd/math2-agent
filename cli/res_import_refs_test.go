package cli

import (
	"net/http"
	"net/http/httptest"
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
