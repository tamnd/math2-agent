package ingest

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchRetry(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch n.Add(1) {
		case 1:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
		case 2:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			_, _ = io.WriteString(w, "ok")
		}
	}))
	defer srv.Close()
	body, err := fetch(context.Background(), Options{}, srv.URL, "", http.StatusOK)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = body.Close() }()
	if b, _ := io.ReadAll(body); string(b) != "ok" || n.Load() != 3 {
		t.Fatalf("body %q after %d requests", b, n.Load())
	}
}

func TestFetchNoRetry(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	_, err := fetch(context.Background(), Options{}, srv.URL, "", http.StatusOK)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusBadRequest || n.Load() != 1 {
		t.Fatalf("error %v after %d requests", err, n.Load())
	}
}

func TestFetchRetryLimit(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	_, err := fetch(context.Background(), Options{}, srv.URL, "", http.StatusOK)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusTooManyRequests || n.Load() != Retries+1 {
		t.Fatalf("error %v after %d requests", err, n.Load())
	}
}

func TestRetryWait(t *testing.T) {
	for _, c := range []struct {
		after string
		try   int
		want  time.Duration
	}{
		{"", 0, 2 * time.Second},
		{"", 2, 8 * time.Second},
		{"", 20, MaxWait},
		{"5", 3, 5 * time.Second},
		{"9999", 0, MaxWait},
		{"Wed, 21 Oct 2015 07:28:00 GMT", 1, 4 * time.Second},
	} {
		if got := retryWait(c.after, c.try); got != c.want {
			t.Errorf("retryWait(%q, %d) = %v, want %v", c.after, c.try, got, c.want)
		}
	}
}
