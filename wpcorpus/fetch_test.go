package wpcorpus

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchErrors(t *testing.T) {
	for _, c := range []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusOK, `{"error":{"code":"maxlag","info":"Waiting for a database server"}}`, "the error maxlag"},
		{http.StatusOK, `{"query":{"pages":[{"ns":0,"title":"X","missing":true}]}}`, "does not exist"},
		{http.StatusOK, `{"query":{"pages":[{"pageid":1,"ns":14,"title":"Category:X","revisions":[{"revid":2}]}]}}`, "namespace 14"},
		{http.StatusOK, `{"query":{"pages":[{"pageid":1,"ns":0,"title":"X"}]}}`, "no revision"},
		{http.StatusOK, `{"query":{"pages":[]}}`, "0 pages"},
		{http.StatusOK, `not json`, "X: "},
		{http.StatusServiceUnavailable, ``, "status 503"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(c.status)
			_, _ = io.WriteString(w, c.body)
		}))
		_, err := (&Fetcher{API: srv.URL}).Fetch(context.Background(), "X")
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: the error is %v, want %q", c.body, err, c.want)
		}
	}
}
