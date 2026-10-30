package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	rescluster "github.com/tamnd/math2-agent/resource/cluster"
)

func TestResImportSWH(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "m2/") {
			t.Errorf("User-Agent %q", r.Header.Get("User-Agent"))
		}
		if r.URL.Path != "/origin/https://github.com/sagemath/sage/visit/latest/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `{"snapshot":"39b5fe612c3b4abb96e8e50dc120dffa0e3962fc"}`)
	}))
	defer srv.Close()
	saved := swhAPI
	t.Cleanup(func() { swhAPI = saved })
	swhAPI = srv.URL

	root := t.TempDir()
	if err := rescluster.WriteRecords(root, "swmath", rescluster.SourceFile, []rescluster.Record{
		{Source: "swmath", Snapshot: "blake3:01", NativeID: "825", Level: "work", Kind: "software", Title: "SageMath", IDs: resource.IDs{"swmath": {"825"}}, Links: []string{"https://github.com/sagemath/sage"}},
		{Source: "swmath", Snapshot: "blake3:01", NativeID: "900", Level: "work", Kind: "software", Title: "Lost", IDs: resource.IDs{"swmath": {"900"}}, Links: []string{"https://github.com/a/lost"}},
	}); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("res", "import", "-root", root, "-gap", "0", "swh")
	if code != ExitOK || !strings.Contains(stdout, "swh: origins 2, reused 0, asked 2, found 1, left 0, records 1, rejected 0") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	recs, err := rescluster.ReadRecords(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []rescluster.Record
	for _, r := range recs {
		if r.Source == "swh" {
			got = append(got, r)
		}
	}
	if len(got) != 1 || got[0].NativeID != "swmath:825" || got[0].IDs.Get("swh") != "swh:1:snp:39b5fe612c3b4abb96e8e50dc120dffa0e3962fc" || got[0].IDs.Get("swmath") != "825" {
		t.Fatalf("records %+v", got)
	}
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		t.Fatal(err)
	}
	if src := cat.Source("swh"); src == nil || len(src.Snapshots) != 1 || src.Class != ingest.RecordOnly {
		t.Fatalf("source %+v", src)
	}

	// A second run reuses the SWHID and asks only for the origin with none.
	calls = 0
	code, stdout, stderr = run("res", "import", "-root", root, "-gap", "0", "-n", "5", "swh")
	if code != ExitOK || calls != 1 || !strings.Contains(stdout, "reused 1, asked 1, found 0") {
		t.Fatalf("exit code %d, %d calls, stdout %q, stderr %q", code, calls, stdout, stderr)
	}
}

func TestResImportSourceWorks(t *testing.T) {
	root := t.TempDir()
	code, stdout, stderr := run("res", "import", "-root", root, "source_works")
	if code != ExitOK || !strings.Contains(stdout, "source_works: ") || !strings.Contains(stdout, "rejected 0") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	recs, err := rescluster.ReadRecords(root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range recs {
		if r.Source == "source_works" && r.NativeID == "oeis" {
			found = r.Kind == "dataset" && r.IDs.Get("source") == "oeis" && r.IDs.Get("wikidata") == "Q728415"
		}
	}
	if !found {
		t.Fatalf("no record of the OEIS in %+v", recs)
	}
}
