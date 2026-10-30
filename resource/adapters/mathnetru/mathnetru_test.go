package mathnetru

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource/cluster"
)

const issuePage = `<a class="ry" href="/eng/im/authornotes">Guidelines</a>
<td colspan=2 width=90% valign=top align=left>
<a class=SLink href="/eng/im8810">Linear forms of a given Diophantine type</a><br>O.&nbsp;N.&nbsp;German
<div style='font-size: small'><i>Izvestiya: Mathematics, 2020, Pages 3&ndash;22<br/>DOI:&nbsp;<a class=SLink href='https://doi.org/10.1070/IM8810'>https://doi.org/10.1070/IM8810</a></i></div>
<div><i>Izv. RAN, DOI:&nbsp;<a class=SLink href='https://doi.org/10.4213/im8810'>x</a></i></div></td>
<a class=SLink href="/eng/im8811">О смешанной динамике</a><br>С. В. Гонченко, Д. Тураев
<div style='font-size: small'><i>Pages 23&ndash;40</i></div>`

func TestFetchAndRecords(t *testing.T) {
	var mu sync.Mutex
	var times []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/ej.phtml":
			_, _ = io.WriteString(w, `<A href=/php/journal.phtml?jrnid=im&option_lang=eng>x</a><A href=/php/journal.phtml?jrnid=im&option_lang=eng>y</a>`)
		case r.URL.Path == "/php/archive.phtml" && q.Get("wshow") == "contents" && q.Get("jrnid") == "im":
			_, _ = io.WriteString(w, `<a class=SLink href=/php/archive.phtml?jrnid=im&amp;wshow=issue&year=2020&volume=84&issue=1&option_lang=eng>1</a>
<a class=SLink href=/php/archive.phtml?jrnid=im&wshow=issue&year=2020&volume=84&issue=1&option_lang=eng>1</a>
<a href=/php/archive.phtml?jrnid=sm&wshow=issue&year=2020&volume=1&issue=1&option_lang=eng>other</a>`)
		case r.URL.Path == "/php/archive.phtml" && q.Get("wshow") == "issue" && q.Get("jrnid") == "im" && q.Get("issue") == "1":
			b, _ := charmap.Windows1251.NewEncoder().String(issuePage)
			_, _ = io.WriteString(w, b)
		default:
			t.Errorf("request %v", r.URL)
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
	gap := 30 * time.Millisecond
	var buf bytes.Buffer
	if err := (Adapter{Gap: gap}).Fetch(context.Background(), &ingest.Request{URL: srv.URL, Get: get}, &buf); err != nil {
		t.Fatal(err)
	}
	if len(times) != 3 {
		t.Fatalf("%d requests", len(times))
	}
	for i := 1; i < len(times); i++ {
		if d := times[i].Sub(times[i-1]); d < gap*8/10 { // the server sees each request a little after the adapter sends it
			t.Errorf("request %d came %v after the request before it", i+1, d)
		}
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
	if r.NativeID != "im8810" || r.Kind != "article" || r.Level != "work" || r.Title != "Linear forms of a given Diophantine type" || r.Date != "2020" || r.Volume != "84" || r.Page != "3" ||
		r.Links[0] != "https://www.mathnet.ru/eng/im8810" {
		t.Errorf("record %+v", r)
	}
	if r.IDs["mathnetru"][0] != "im8810" || len(r.IDs["doi"]) != 2 || r.IDs["doi"][0] != "10.1070/im8810" || r.IDs["doi"][1] != "10.4213/im8810" {
		t.Errorf("ids %+v", r.IDs)
	}
	if len(r.Creators) != 1 || r.Creators[0].Name != "O. N. German" {
		t.Errorf("creators %+v", r.Creators)
	}
	if r := rs[1]; r.Title != "О смешанной динамике" || len(r.Creators) != 2 || r.Creators[0].Name != "С. В. Гонченко" || r.Page != "23" || len(r.IDs["doi"]) != 0 {
		t.Errorf("record %+v", r)
	}
}

func TestFetchStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	get := func(context.Context, string) (io.ReadCloser, error) {
		cancel()
		return io.NopCloser(bytes.NewReader([]byte(`<a href=/php/journal.phtml?jrnid=im>x</a>`))), nil
	}
	if err := (Adapter{Gap: time.Hour}).Fetch(ctx, &ingest.Request{URL: "http://x", Get: get}, io.Discard); err == nil {
		t.Error("a cancelled fetch gives no error")
	}
}
