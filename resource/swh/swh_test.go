package swh

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
)

func TestOrigin(t *testing.T) {
	cases := map[string]string{
		"https://github.com/sagemath/sage":                    "https://github.com/sagemath/sage",
		"http://www.github.com/sagemath/sage.git":             "https://github.com/sagemath/sage",
		"https://github.com/gap-system/gap/tree/master/src":   "https://github.com/gap-system/gap",
		"https://gitlab.com/group/sub/proj/-/tree/main":       "https://gitlab.com/group/sub/proj",
		"https://codeberg.org/owner/name/":                    "https://codeberg.org/owner/name",
		"https://github.com/sagemath":                         "",
		"https://www.sagemath.org/":                           "",
		"ftp://github.com/a/b":                                "",
		"not a link":                                          "",
		"https://gitlab.com/onlyone":                          "",
		"https://bitbucket.org/owner/repo/src/main/README.md": "https://bitbucket.org/owner/repo",
	}
	for in, want := range cases {
		if got := Origin(in); got != want {
			t.Errorf("Origin(%q) = %q, want %q", in, got, want)
		}
	}
}

const snp = "39b5fe612c3b4abb96e8e50dc120dffa0e3962fc"

func TestBuild(t *testing.T) {
	recs := []cluster.Record{
		{Source: "swmath", NativeID: "825", Kind: "software", Title: "SageMath", IDs: resource.IDs{"swmath": {"825"}}, Links: []string{"https://www.sagemath.org/", "https://github.com/sagemath/sage"}},
		{Source: "swmath", NativeID: "826", Kind: "software", Title: "Sage part", IDs: resource.IDs{"swmath": {"826"}}, Links: []string{"https://github.com/sagemath/sage.git"}},
		{Source: "swmath", NativeID: "900", Kind: "software", Title: "Lost", IDs: resource.IDs{"swmath": {"900"}}, Links: []string{"https://github.com/a/lost"}},
		{Source: "zenodo", NativeID: "10.5281/zenodo.1", Kind: "software", Title: "Tool", IDs: resource.IDs{"doi": {"10.5281/zenodo.1"}, "zenodo": {"1"}}, Links: []string{"https://github.com/b/tool"}},
		{Source: "zenodo", NativeID: "10.5281/zenodo.2", Kind: "dataset", Title: "Data", IDs: resource.IDs{"doi": {"10.5281/zenodo.2"}}, Links: []string{"https://github.com/b/data"}},
		{Source: "swmath", NativeID: "901", Kind: "software", Title: "No forge", IDs: resource.IDs{"swmath": {"901"}}, Links: []string{"https://example.org/x"}},
		{Source: Source, NativeID: "swmath:1", Kind: "software", Title: "Old", IDs: resource.IDs{"swmath": {"1"}}, Links: []string{"https://github.com/c/old"}},
	}
	prev := []cluster.Record{{Source: Source, NativeID: "zenodo:10.5281/zenodo.1", IDs: resource.IDs{"swh": {"swh:1:snp:" + snp}}, Links: []string{"https://github.com/b/tool"}}}
	var asked []string
	visit := func(_ context.Context, origin string) (string, error) {
		asked = append(asked, origin)
		if origin == "https://github.com/sagemath/sage" {
			return snp, nil
		}
		return "", nil
	}
	out, c, err := Build(context.Background(), recs, prev, visit, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 2 || asked[0] != "https://github.com/sagemath/sage" || asked[1] != "https://github.com/a/lost" {
		t.Errorf("asked %q", asked)
	}
	if c.Origins != 4 || c.Reused != 2 || c.Asked != 2 || c.Found != 1 || len(out) != 3 {
		t.Fatalf("counts %+v, records %+v", c, out)
	}
	r := out[0]
	if r.NativeID != "swmath:825" || r.Kind != "software" || r.Title != "SageMath" || r.IDs.Get("swh") != "swh:1:snp:"+snp || r.IDs.Get("swmath") != "825" || r.Links[0] != "https://github.com/sagemath/sage" {
		t.Errorf("record %+v", r)
	}
	if out[1].NativeID != "swmath:826" || out[1].IDs.Get("swmath") != "826" {
		t.Errorf("a second package in the same repository must keep its own record: %+v", out[1])
	}
	if out[2].IDs.Get("doi") != "10.5281/zenodo.1" || out[2].IDs.Get("zenodo") != "" || out[2].IDs.Get("swh") == "" {
		t.Errorf("record %+v", out[2])
	}
	if _, c, _ := Build(context.Background(), recs, nil, visit, 1); c.Asked != 1 || c.Left != 2 {
		t.Errorf("with a limit of 1: %+v", c)
	}
}

func TestID(t *testing.T) {
	if id, err := ID(snp); err != nil || id != "swh:1:snp:"+snp {
		t.Errorf("%q, %v", id, err)
	}
	if _, err := ID("abc"); err == nil {
		t.Error("a short hash must be an error")
	}
}

func TestLatest(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("User-Agent") != "test-agent" || r.URL.Query().Get("require_snapshot") != "true" {
			t.Errorf("request %s %v", r.URL, r.Header)
		}
		w.Header().Set("X-RateLimit-Limit", "700")
		w.Header().Set("X-RateLimit-Reset", "1000060")
		switch r.URL.Path {
		case "/origin/https://github.com/sagemath/sage/visit/latest/":
			w.Header().Set("X-RateLimit-Remaining", "0")
			_, _ = io.WriteString(w, `{"origin":"https://github.com/sagemath/sage","visit":1170,"status":"full","snapshot":"`+snp+`","type":"git"}`)
		case "/origin/https://github.com/a/busy/visit/latest/":
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(2-calls))
			if calls < 3 {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			_, _ = io.WriteString(w, `{"snapshot":"`+snp+`"}`)
		default:
			w.Header().Set("X-RateLimit-Remaining", "10")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"exception":"NotFoundExc","reason":"Origin not found!"}`)
		}
	}))
	defer srv.Close()
	clock := time.Unix(1000000, 0)
	var waits []time.Duration
	c := NewClient("test-agent", time.Second)
	c.Base = srv.URL
	c.now = func() time.Time { return clock }
	c.sleep = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		clock = clock.Add(d)
		return nil
	}

	got, err := c.Latest(context.Background(), "https://github.com/sagemath/sage")
	if err != nil || got != snp {
		t.Fatalf("%q, %v", got, err)
	}
	// The quota is spent, so the next request waits for the reset.
	got, err = c.Latest(context.Background(), "https://github.com/a/none")
	if err != nil || got != "" {
		t.Fatalf("an origin that the archive does not have: %q, %v", got, err)
	}
	if waits[1] != 60*time.Second {
		t.Errorf("waits %v, want 60s before the second request", waits)
	}
	calls = 0
	got, err = c.Latest(context.Background(), "https://github.com/a/busy")
	if err != nil || got != snp || calls != 3 {
		t.Errorf("after two answers 429: %q, %v, %d calls", got, err, calls)
	}
}

func TestBuildError(t *testing.T) {
	recs := []cluster.Record{
		{Source: "swmath", NativeID: "1", Kind: "software", Title: "A", IDs: resource.IDs{"swmath": {"1"}}, Links: []string{"https://github.com/a/a"}},
		{Source: "swmath", NativeID: "2", Kind: "software", Title: "B", IDs: resource.IDs{"swmath": {"2"}}, Links: []string{"https://github.com/b/b"}},
		{Source: "swmath", NativeID: "3", Kind: "software", Title: "C", IDs: resource.IDs{"swmath": {"3"}}, Links: []string{"https://github.com/c/c"}},
	}
	visit := func(_ context.Context, origin string) (string, error) {
		if origin == "https://github.com/b/b" {
			return "", io.ErrUnexpectedEOF
		}
		return snp, nil
	}
	out, c, err := Build(context.Background(), recs, nil, visit, 0)
	if err == nil || len(out) != 1 || out[0].NativeID != "swmath:1" || c.Asked != 2 || c.Left != 2 {
		t.Errorf("records %+v, counts %+v, error %v", out, c, err)
	}
}
