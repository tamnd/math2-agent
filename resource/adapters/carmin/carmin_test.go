package carmin

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource/cluster"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name) // #nosec G304 -- the file is a fixture of the test.
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func get(srv *httptest.Server) ingest.Getter {
	return func(ctx context.Context, u string) (io.ReadCloser, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := srv.Client().Do(req)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
}

// server answers with the fixtures, and records the queries.
func server(t *testing.T, queries *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		*queries = append(*queries, r.URL.RawQuery)
		if q.Get("mailto") != "" || r.Header.Get("From") != "" {
			t.Error("the request has an email")
		}
		switch {
		case q.Get("verb") == "ListSets":
			_, _ = w.Write(fixture(t, "sets.xml"))
		case q.Get("resumptionToken") == "eyJwYWdlIjoxfQ==":
			_, _ = w.Write(fixture(t, "page2.xml"))
		case q.Get("verb") == "ListRecords" && q.Get("set") == "videos:institution:ihes" && q.Get("metadataPrefix") == Prefix:
			_, _ = w.Write(fixture(t, "page1.xml"))
		default:
			t.Errorf("query %s", r.URL.RawQuery)
			http.NotFound(w, r)
		}
	}))
}

func TestFetchAndRecords(t *testing.T) {
	var queries []string
	srv := server(t, &queries)
	defer srv.Close()
	a := IHES
	a.Gap = 0
	var buf bytes.Buffer
	if err := a.Fetch(context.Background(), &ingest.Request{URL: srv.URL, Get: get(srv)}, &buf); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 3 {
		t.Errorf("queries %q", queries)
	}
	snap, err := ingest.Uncompressed(&buf)
	if err != nil {
		t.Fatal(err)
	}
	var rs []cluster.Record
	if err := a.Records(snap, func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 3 {
		t.Fatalf("records %+v", rs)
	}
	s := rs[0]
	if s.NativeID != "videos:collection:the-pentagram-map-and-ramifications" || s.Kind != "video-series" || s.Level != "work" || s.Title != "The Pentagram Map and Ramifications" ||
		s.IDs.Get("carmin_collection") != "the-pentagram-map-and-ramifications" {
		t.Errorf("series %+v", s)
	}
	l := rs[1]
	if l.NativeID != "complexity-of-lasso-with-normalized-data" || l.Kind != "course-material" || l.Level != "work" || l.Title != "Complexity of Lasso with Normalized Data" ||
		l.Date != "2026-10-15" || l.Language != "en" || l.Publisher != "IHES" || l.IDs.Get("carmin") != l.NativeID ||
		len(l.Links) != 1 || l.Links[0] != "https://www.carmin.tv/fr/video/complexity-of-lasso-with-normalized-data" {
		t.Errorf("lecture %+v", l)
	}
	if len(l.Creators) != 1 || l.Creators[0].Name != "Vladimir Dragovich" || l.Creators[0].Role != "author" {
		t.Errorf("creators %+v", l.Creators)
	}
	if len(l.Subjects) != 4 || l.Subjects[0].System != "keyword" || l.Subjects[0].Code != "lasso regularization paths" || l.Subjects[3].Code != "symbolic" {
		t.Errorf("subjects %+v", l.Subjects)
	}
	if len(l.Relations) != 1 || l.Relations[0].Kind != "part_of" || l.Relations[0].To != "carmin_collection:the-pentagram-map-and-ramifications" || l.Relations[0].Source != "ihes" {
		t.Errorf("edges %+v", l.Relations)
	}
	p := rs[2]
	if p.NativeID != "pentagram-map-and-poncelet-grid" || p.Title != "Théorème de Poncelet & réseaux" || p.Language != "fr" || len(p.Creators) != 1 || len(p.Subjects) != 2 || len(p.Relations) != 0 {
		t.Errorf("lecture %+v", p)
	}
	for _, r := range rs {
		if why := cluster.Check(&r); why != "" {
			t.Errorf("%s: %s", r.NativeID, why)
		}
	}
}

func TestSnapshotHasNoText(t *testing.T) {
	var queries []string
	srv := server(t, &queries)
	defer srv.Close()
	a := IHES
	a.Gap = 0
	var buf bytes.Buffer
	if err := a.Fetch(context.Background(), &ingest.Request{URL: srv.URL, Get: get(srv)}, &buf); err != nil {
		t.Fatal(err)
	}
	snap, err := ingest.Uncompressed(&buf)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(snap)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"abstract", "workshop explores", "uploads/video", "Not Only Scalar Curvature", "a-removed-video"} {
		if strings.Contains(string(data), s) {
			t.Errorf("the snapshot has %q", s)
		}
	}
}

func TestRead(t *testing.T) {
	in := `{"collection":{"slug":"s","title":"S"}}` + "\n" + `{"lecture":{"slug":"l","title":"L","url":"https://www.carmin.tv/fr/video/l"}}` + "\n"
	var got []ingest.Record
	if err := CIRM.Read(strings.NewReader(in), func(r ingest.Record) error { got = append(got, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "videos:collection:s" || got[0].Kind != "video-series" || got[1].Kind != "course-material" || got[1].URL != "https://www.carmin.tv/fr/video/l" {
		t.Errorf("records %+v", got)
	}
	for _, bad := range []string{`{"collection":{"title":"S"}}`, `{"lecture":{"title":"L"}}`, `{}`, `{"lecture":`} {
		if err := CIRM.Read(strings.NewReader(bad), func(ingest.Record) error { return nil }); err == nil {
			t.Errorf("%s gives no error", bad)
		}
	}
}

func TestError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<OAI-PMH><error code="badVerb">Illegal verb</error></OAI-PMH>`)
	}))
	defer srv.Close()
	a := CIRM
	a.Gap = 0
	if err := a.Fetch(context.Background(), &ingest.Request{URL: srv.URL, Get: get(srv)}, io.Discard); err == nil {
		t.Error("an error of the endpoint gives no error")
	}
}

func TestSources(t *testing.T) {
	if CIRM.Source().ID != "cirm" || IHES.Source().ID != "ihes" || IHES.Version() != "ihes/1" || CIRM.Dump() != Endpoint {
		t.Error("the sources have the wrong ids")
	}
	if CIRM.Source().Class != ingest.RecordOnly {
		t.Error("the class is not record")
	}
}
