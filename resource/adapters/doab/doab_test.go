package doab

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource/cluster"
)

const export = "\uFEFFid,collection,dc.contributor.author,dc.contributor.editor,dc.date.issued,dc.identifier,dc.identifier.isbn,dc.identifier.uri,dc.language,dc.publisher,dc.relation.ispartofseries,oapen.series.number,dc.subject.classification,dc.subject.other,dc.title,dc.type,oapen.identifier.doi,BITSTREAM License\n" +
	"a1,c,,\"Marchau, Vincent||Walker, Warren\",2019,1007261||OCN: 1099789127,9783030052515,http://library.oapen.org/handle/20.500.12657/22900,English[eng],Springer,Lecture Notes,7," +
	"\"thema EDItEUR::P Mathematics and Science::PB Mathematics::PBT Probability and statistics||bic Book Industry Communication::K Economics::KJ Business\",Probabilities,\"Decision Making\n under Uncertainty\",book,10.1007/978-3-030-05252-2,CC BY\n" +
	"a2,c,\"Bell, Currer\",,1847,,,,French,P,,,thema EDItEUR::D Literature::DC Poetry,Poetry,Poems,book,,\n" +
	"a3,c,\"Noether, Emmy\",,1921,,,,German,P,,,,Algebra -- Rings,Idealtheorie,chapter,,\n"

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

func TestFetchAndRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, export) }))
	defer srv.Close()
	var buf bytes.Buffer
	if err := OAPEN.Fetch(context.Background(), &ingest.Request{URL: srv.URL, Get: get(srv)}, &buf); err != nil {
		t.Fatal(err)
	}
	snap, err := ingest.Uncompressed(&buf)
	if err != nil {
		t.Fatal(err)
	}
	var rs []cluster.Record
	if err := OAPEN.Records(snap, func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("records %+v", rs)
	}
	r := rs[0]
	if r.NativeID != "a1" || r.Kind != "edited-volume" || r.Level != "manifestation" || r.Title != "Decision Making under Uncertainty" || r.Language != "en" || r.Date != "2019" ||
		r.Publisher != "Springer" || r.Volume != "7" || r.Links[0] != "https://library.oapen.org/handle/20.500.12657/22900" {
		t.Errorf("record %+v", r)
	}
	if r.IDs["doi"][0] != "10.1007/978-3-030-05252-2" || r.IDs["isbn"][0] != "9783030052515" || r.IDs["oclc"][0] != "1099789127" || r.IDs["handle"][0] != "20.500.12657/22900" {
		t.Errorf("ids %+v", r.IDs)
	}
	if len(r.Subjects) != 2 || r.Subjects[0].System != "thema" || r.Subjects[0].Code != "PBT" || r.Subjects[1].Code != "KJ" {
		t.Errorf("subjects %+v", r.Subjects)
	}
	if len(r.Creators) != 2 || r.Creators[0].Role != "editor" || r.Creators[0].Name != "Marchau, Vincent" {
		t.Errorf("creators %+v", r.Creators)
	}
	if r := rs[1]; r.NativeID != "a3" || r.Kind != "chapter" || r.Language != "de" || r.Links[0] != "https://library.oapen.org/items/a3" {
		t.Errorf("record %+v", r)
	}
}

func TestColumns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "id,dc.title\n1,A\n") }))
	defer srv.Close()
	if err := DOAB.Fetch(context.Background(), &ingest.Request{URL: srv.URL, Get: get(srv)}, io.Discard); err == nil {
		t.Error("an export with no column dc.type gives no error")
	}
}

func TestSources(t *testing.T) {
	if DOAB.Source().ID != "doab" || OAPEN.Source().ID != "oapen" || OAPEN.Version() != "oapen/1" {
		t.Error("the sources have the wrong ids")
	}
}
