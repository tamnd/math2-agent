package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// Resumes is the number of times that a body of Resume gets the rest of the file after a read error.
const Resumes = 8

// Resume gets the body of url and returns a reader that continues after a read error. After an error, the reader gets the rest of the file with GetRange, from the first byte that it did not read, and tries again up to Resumes times. A large dump takes long to read, and a server can close the connection before the end. When GetRange is nil, the reader gives the first error.
func Resume(ctx context.Context, req *Request, url string) (io.ReadCloser, error) {
	body, err := req.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	return &resumer{ctx: ctx, get: req.GetRange, url: url, body: body, wait: 10 * time.Second}, nil
}

type resumer struct {
	ctx   context.Context
	get   RangeGetter
	url   string
	body  io.ReadCloser
	off   int64
	tries int
	wait  time.Duration
}

func (r *resumer) Read(p []byte) (int, error) {
	for {
		n, err := r.body.Read(p)
		r.off += int64(n)
		if err == nil || errors.Is(err, io.EOF) || n > 0 {
			if err != nil && !errors.Is(err, io.EOF) {
				// Give the bytes now, and meet the error on the next read.
				err = nil
			}
			return n, err
		}
		if r.get == nil || r.tries >= Resumes || r.ctx.Err() != nil {
			return 0, err
		}
		r.tries++
		_ = r.body.Close()
		select {
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
		case <-time.After(r.wait):
		}
		body, gerr := r.get(r.ctx, r.url, r.off, -1)
		if gerr != nil {
			return 0, fmt.Errorf("%w; the resume at byte %d failed: %v", err, r.off, gerr)
		}
		r.body = body
	}
}

func (r *resumer) Close() error { return r.body.Close() }

// Spool gets the file at url into a temporary file in req.Cache, with Resume, and returns the file from its start. Close removes the file. A fetch that reads a large dump slowly, as it parses it, keeps the connection open for a long time, and a server can reset it. Spool reads the dump at the speed of the network first.
func Spool(ctx context.Context, req *Request, url string) (io.ReadCloser, error) {
	body, err := Resume(ctx, req, url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()
	f, err := os.CreateTemp(req.Cache, "spool-*")
	if err != nil {
		return nil, err
	}
	s := &spool{f}
	if _, err := io.Copy(f, body); err != nil {
		_ = s.Close()
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

type spool struct{ *os.File }

func (s *spool) Close() error {
	err := s.File.Close()
	if rerr := os.Remove(s.Name()); err == nil {
		err = rerr
	}
	return err
}
