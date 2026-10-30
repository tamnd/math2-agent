package nlv

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource/adapters/marc"
	"github.com/tamnd/math2-agent/resource/cluster"
)

// book gives one record in MARC 21 with the class field cls (080 or 082) and the class c.
func book(id, title, cls, c string) []byte {
	return marc.Encode(marc.Record{Fields: []marc.Field{
		{Tag: "001", Value: id},
		{Tag: "008", Value: "200101s2020    vm            000 0 vie  "},
		{Tag: "020", Ind: "  ", Subfields: [][2]string{{"a", "9786040000002"}}},
		{Tag: cls, Ind: "04", Subfields: [][2]string{{"a", c}}},
		{Tag: "100", Ind: "1 ", Subfields: [][2]string{{"a", "Nguyễn, Văn A"}}},
		{Tag: "245", Ind: "10", Subfields: [][2]string{{"a", title}}},
		{Tag: "260", Ind: "  ", Subfields: [][2]string{{"b", "Giáo dục,"}}},
	}})
}

func records(t *testing.T, snap io.Reader) []cluster.Record {
	t.Helper()
	r, err := ingest.Uncompressed(snap)
	if err != nil {
		t.Fatal(err)
	}
	var rs []cluster.Record
	if err := (Adapter{}).Records(r, func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestFetchDirectory(t *testing.T) {
	dir := t.TempDir()
	first := append(book("1", "Giải tích", "082", "515"), book("2", "Lịch sử", "082", "959.7")...)
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write(book("3", "Đại số", "080", "512.5"))
	_ = zw.Close()
	if err := os.WriteFile(filepath.Join(dir, "2024-01.iso"), first, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2024-02.iso.gz"), gz.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hidden"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: dir}, &buf); err != nil {
		t.Fatal(err)
	}
	rs := records(t, &buf)
	if len(rs) != 2 || rs[0].NativeID != "1" || rs[1].NativeID != "3" {
		t.Fatalf("records %+v", rs)
	}
	r := rs[0]
	if r.Title != "Giải tích" || r.Language != "vi" || r.Date != "2020" || r.Publisher != "Giáo dục" || r.IDs["isbn"][0] != "9786040000002" || len(r.Links) != 0 || r.Subjects[0].System != "ddc" {
		t.Errorf("record %+v", r)
	}
	if rs[1].Subjects[0].System != "udc" || rs[1].Subjects[0].Code != "512.5" {
		t.Errorf("record %+v", rs[1])
	}
}

func TestFetchURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(book("9", "Xác suất", "082", "519.2"))
	}))
	defer srv.Close()
	get := func(ctx context.Context, u string) (io.ReadCloser, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := srv.Client().Do(req)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
	var buf bytes.Buffer
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: srv.URL + "/2024-03.iso", Get: get}, &buf); err != nil {
		t.Fatal(err)
	}
	if rs := records(t, &buf); len(rs) != 1 || rs[0].NativeID != "9" {
		t.Errorf("records %+v", rs)
	}
}

func TestFetchNeedsPath(t *testing.T) {
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: Home}, io.Discard); err == nil {
		t.Error("the home page gives no error")
	}
}
