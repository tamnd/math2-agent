package locmarc

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource/cluster"
)

// marc encodes one record in ISO 2709. Each field is a tag and its data: the value of a control field, or the indicators and the subfields with $ for the delimiter.
func iso(fields ...[2]string) []byte {
	var dir, data bytes.Buffer
	for _, f := range fields {
		d := strings.ReplaceAll(f[1], "$", "\x1f") + "\x1e"
		fmt.Fprintf(&dir, "%s%04d%05d", f[0], len(d), data.Len())
		data.WriteString(d)
	}
	dir.WriteByte(0x1e)
	base := 24 + dir.Len()
	n := base + data.Len() + 1
	leader := fmt.Sprintf("%05dcam a22%05d a 4500", n, base)
	return append([]byte(leader+dir.String()+data.String()), 0x1d)
}

func TestInScope(t *testing.T) {
	for c, want := range map[string]bool{"QA1": true, "QA300 .H3": true, "QA76.73.J38": false, "QA75.5": false, "QA76.95": false, "QA76.96": true, "QA939": true, "QA940": false, "QB43": false, "Q172": false} {
		if InScope(c) != want {
			t.Errorf("InScope(%q) = %v", c, !want)
		}
	}
}

func TestFetchAndRecords(t *testing.T) {
	var part bytes.Buffer
	zw := gzip.NewWriter(&part)
	_, _ = zw.Write(iso([2]string{"001", "  12345 "}, [2]string{"008", "850101s1908    enk           000 0 eng  "}, [2]string{"010", "  $a   08012345 "},
		[2]string{"020", "  $a0521720559 (pbk.)"}, [2]string{"035", "  $a(OCoLC)ocm2779601"}, [2]string{"050", "00$aQA303$b.H3"}, [2]string{"082", "00$a515/.1"},
		[2]string{"100", "1 $aHardy, G. H.$q(Godfrey Harold),$d1877-1947."}, [2]string{"245", "12$aA course of pure mathematics /$cby G. H. Hardy."}, [2]string{"250", "  $a10th ed."},
		[2]string{"260", "  $aCambridge :$bUniversity Press,$c1908."}, [2]string{"490", "1 $aCambridge mathematical library ;$v1"}, [2]string{"650", " 0$aCalculus.$xTextbooks."},
		[2]string{"505", "0 $aReal variables -- Functions."}, [2]string{"700", "1 $aKörner, T. W.,$eeditor."}))
	_, _ = zw.Write(iso([2]string{"001", "999"}, [2]string{"050", "00$aQA76.73$b.J38"}, [2]string{"245", "10$aJava."}))
	_, _ = zw.Write(iso([2]string{"001", "777"}, [2]string{"082", "04$a519.2"}, [2]string{"245", "10$aProbability :$bessays."}, [2]string{"700", "1 $aDoob, J. L.,$eed."}))
	_ = zw.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cds/products/page.html":
			_, _ = io.WriteString(w, `<a href="/d/BooksAll.2014.part01.utf8.gz">old</a><a href="/d/BooksAll.2016.part01.utf8.gz">new</a><a href="/d/BooksAll.2016.part01.xml.gz">xml</a>`)
		case "/d/BooksAll.2016.part01.utf8.gz":
			_, _ = w.Write(part.Bytes())
		default:
			t.Errorf("path %s", r.URL.Path)
			http.NotFound(w, r)
		}
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
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: srv.URL + "/cds/products/page.html", Get: get}, &buf); err != nil {
		t.Fatal(err)
	}
	snap, err := ingest.Uncompressed(&buf)
	if err != nil {
		t.Fatal(err)
	}
	var rs []cluster.Record
	if err := (Adapter{}).Records(snap, func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("records %+v", rs)
	}
	r := rs[0]
	if r.NativeID != "12345" || r.Kind != "textbook" || r.Title != "A course of pure mathematics" || r.Date != "1908" || r.Language != "en" || r.Edition != "10th ed." ||
		r.Publisher != "University Press" || r.Links[0] != "https://lccn.loc.gov/08012345" {
		t.Errorf("record %+v", r)
	}
	if r.IDs["lccn"][0] != "08012345" || r.IDs["isbn"][0] != "9780521720557" || r.IDs["oclc"][0] != "2779601" {
		t.Errorf("ids %+v", r.IDs)
	}
	if len(r.Subjects) != 2 || r.Subjects[0].Code != "QA303 .H3" || r.Subjects[1].Code != "515.1" {
		t.Errorf("subjects %+v", r.Subjects)
	}
	if len(r.Creators) != 2 || r.Creators[0].Name != "Hardy, G. H." || r.Creators[0].Role != "author" || r.Creators[1].Role != "editor" {
		t.Errorf("creators %+v", r.Creators)
	}
	if r := rs[1]; r.NativeID != "777" || r.Kind != "edited-volume" || r.Title != "Probability: essays" {
		t.Errorf("record %+v", r)
	}
}
