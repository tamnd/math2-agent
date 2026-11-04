package ingest

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

// broken gives the first n bytes of s and then an error.
type broken struct {
	r io.Reader
}

func (b broken) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if errors.Is(err, io.EOF) {
		return n, errors.New("connection reset by peer")
	}
	return n, err
}

func (broken) Close() error { return nil }

func TestResume(t *testing.T) {
	const s = "0123456789abcdefghij"
	ranges := 0
	req := &Request{
		Get: func(context.Context, string) (io.ReadCloser, error) {
			return broken{strings.NewReader(s[:7])}, nil
		},
		GetRange: func(_ context.Context, _ string, start, end int64) (io.ReadCloser, error) {
			ranges++
			if end >= 0 {
				t.Errorf("end %d", end)
			}
			stop := min(int(start)+7, len(s))
			if stop == len(s) {
				return io.NopCloser(strings.NewReader(s[start:])), nil
			}
			return broken{strings.NewReader(s[start:stop])}, nil
		},
	}
	r, err := Resume(context.Background(), req, "https://example.org/dump")
	if err != nil {
		t.Fatal(err)
	}
	r.(*resumer).wait = 0
	b, err := io.ReadAll(r)
	if err != nil || string(b) != s || ranges != 2 {
		t.Fatalf("read %q, %v, %d ranges", b, err, ranges)
	}

	req.GetRange = nil
	r, _ = Resume(context.Background(), req, "https://example.org/dump")
	if _, err := io.ReadAll(r); err == nil || !strings.Contains(err.Error(), "reset") {
		t.Errorf("no range getter: %v", err)
	}
}

func TestSpool(t *testing.T) {
	dir := t.TempDir()
	req := &Request{Cache: dir, Get: func(context.Context, string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("dump")), nil
	}}
	r, err := Spool(context.Background(), req, "https://example.org/dump")
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(r)
	if err != nil || string(b) != "dump" {
		t.Fatalf("read %q, %v", b, err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if es, _ := os.ReadDir(dir); len(es) != 0 {
		t.Errorf("the cache keeps %v", es)
	}
}
