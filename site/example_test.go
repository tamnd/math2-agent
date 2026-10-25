package site_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/tamnd/math2-agent/site"
)

func ExampleCheck() {
	out, err := os.MkdirTemp("", "m2site")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(out) }()
	opt := site.Options{Content: "../testdata/nodes/valid", Out: filepath.Join(out, "site"), URL: "https://example.org/math2"}
	if err := site.Build(opt); err != nil {
		panic(err)
	}
	r, err := site.Check(opt.Out, "/math2")
	if err != nil {
		panic(err)
	}
	for _, t := range site.Targets() {
		fmt.Println(t, r.Passed(t))
	}
	// Output:
	// weight true
	// mathml true
	// nojs true
	// lists true
	// offline true
}

func ExampleHandler() {
	dir, err := os.MkdirTemp("", "m2site")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<p>Math 2.0</p>"), 0o600); err != nil {
		panic(err)
	}
	srv := httptest.NewServer(site.Handler(dir, "/math2"))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/math2/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		panic(err)
	}
	defer func() { _ = resp.Body.Close() }()
	fmt.Println(resp.StatusCode, resp.Header.Get("Content-Encoding"))
	// Output:
	// 200 gzip
}
