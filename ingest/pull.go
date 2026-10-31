package ingest

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zeebo/blake3"
)

// Options controls one import.
type Options struct {
	Root string // the math2 working tree

	// Snapshot is the hash of a snapshot in the catalogue. Pull uses that snapshot again. Empty means a new download of the bulk channel.
	Snapshot string

	URL       string       // the URL of a new download, when it is not the Dump of the adapter
	Date      string       // the date of a new snapshot, YYYY-MM-DD. Empty means today in UTC.
	Cache     string       // the directory of the downloaded snapshots. Empty means m2/snapshots in the user cache directory.
	Client    *http.Client // nil means http.DefaultClient
	UserAgent string       // the User-Agent header of the download
}

// Result is what one import did.
type Result struct {
	Manifest *Manifest
	Snapshot Snapshot
	New      bool // the snapshot is new in the catalogue
}

// Pull imports one snapshot of the source of the adapter into the tree at opt.Root. It records the snapshot in the catalogue, writes the records to build/records/<source>.jsonl and writes the import manifest. When the adapter is a Fetcher, its Fetch method makes a new snapshot. Two pulls of the same snapshot with the same adapter version write the same bytes.
func Pull(ctx context.Context, a Adapter, opt Options) (*Result, error) {
	if opt.Cache == "" {
		dir, err := os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		opt.Cache = filepath.Join(dir, "m2")
	}
	base := opt.Cache
	opt.Cache = filepath.Join(opt.Cache, "snapshots")
	cat, err := ReadCatalogue(opt.Root)
	if err != nil {
		return nil, err
	}
	id := a.Source().ID
	src := cat.Source(id)
	if src == nil {
		s := a.Source()
		s.Snapshots = nil
		cat.Sources = append(cat.Sources, s)
		src = &cat.Sources[len(cat.Sources)-1]
	}
	if p := src.Problems(); len(p) > 0 {
		return nil, fmt.Errorf("ingest %s: the entry in %s is not correct: %s", id, CatalogueFile, strings.Join(p, "; "))
	}

	res := &Result{}
	var path string
	if opt.Snapshot != "" {
		sn := src.Snapshot(opt.Snapshot)
		if sn == nil {
			return nil, fmt.Errorf("ingest %s: %s has no snapshot %s", id, CatalogueFile, opt.Snapshot)
		}
		res.Snapshot = *sn
		if path, err = cached(opt.Cache, sn.Hash); errors.Is(err, fs.ErrNotExist) {
			path, _, _, err = download(ctx, opt, base, a, sn.URL, sn.Hash)
		}
		if err != nil {
			return nil, fmt.Errorf("ingest %s: %w", id, err)
		}
	} else {
		url := opt.URL
		if url == "" {
			url = a.Dump()
		}
		var hash, commit string
		if path, hash, commit, err = download(ctx, opt, base, a, url, ""); err != nil {
			return nil, fmt.Errorf("ingest %s: %w", id, err)
		}
		if sn := src.Snapshot(hash); sn != nil {
			res.Snapshot = *sn
		} else {
			date := opt.Date
			if date == "" {
				date = time.Now().UTC().Format(time.DateOnly)
			}
			res.Snapshot = Snapshot{URL: url, Hash: hash, Date: date, Commit: commit}
			res.New = true
			src.Snapshots = append(src.Snapshots, res.Snapshot)
		}
	}

	f, err := os.Open(path) // #nosec G304 -- the path is in the snapshot cache.
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	recs, err := Import(a, src, f)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	hash, err := WriteRecords(&buf, recs)
	if err != nil {
		return nil, err
	}
	if err := writeFile(filepath.Join(opt.Root, RecordsDir, id+".jsonl"), buf.Bytes()); err != nil {
		return nil, err
	}
	m := &Manifest{Source: id, Adapter: a.Version(), Snapshot: res.Snapshot.Hash, Records: len(recs), Hash: hash, Classes: map[Class]int{}}
	for _, r := range recs {
		m.Classes[r.Class]++
	}
	if err := m.Write(opt.Root); err != nil {
		return nil, err
	}
	if err := cat.Write(opt.Root); err != nil {
		return nil, err
	}
	res.Manifest = m
	return res, nil
}

// SnapshotFile returns the path of the snapshot with the hash in the cache directory of Options. Empty means m2 in the user cache directory, as for Pull.
func SnapshotFile(cache, hash string) (string, error) {
	if cache == "" {
		dir, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		cache = filepath.Join(dir, "m2")
	}
	return cached(filepath.Join(cache, "snapshots"), hash)
}

// cached returns the path of the snapshot with the hash in the cache.
func cached(cache, hash string) (string, error) {
	path := filepath.Join(cache, strings.TrimPrefix(hash, "blake3:"))
	_, err := os.Stat(path)
	return path, err
}

// getter returns the Getter of the options. It sends the User-Agent of the options and fails on a status other than 200 OK.
func getter(opt Options) Getter {
	return func(ctx context.Context, url string) (io.ReadCloser, error) {
		return fetch(ctx, opt, url, "", http.StatusOK)
	}
}

// rangeGetter returns the RangeGetter of the options. It sends the User-Agent of the options and fails on a status other than 206 Partial Content.
func rangeGetter(opt Options) RangeGetter {
	return func(ctx context.Context, url string, start, end int64) (io.ReadCloser, error) {
		if start < 0 || (end >= 0 && end < start) {
			return nil, fmt.Errorf("GET %s: the range %d-%d is not valid", url, start, end)
		}
		r := fmt.Sprintf("bytes=%d-", start)
		if end >= 0 {
			r += strconv.FormatInt(end, 10)
		}
		return fetch(ctx, opt, url, r, http.StatusPartialContent)
	}
}

// Retries is the number of times that fetch tries a URL again when the server answers 429 Too Many Requests or 503 Service Unavailable.
const Retries = 6

// MaxWait is the longest wait of fetch before it tries a URL again.
const MaxWait = 2 * time.Minute

// fetch gets one URL with the Range header r when r is not empty, and fails when the status is not want. When the server answers 429 or 503, fetch waits and tries again, at most Retries times. The wait is the Retry-After of the answer, or 2, 4, 8 and more seconds when the answer has no Retry-After, and at most MaxWait.
func fetch(ctx context.Context, opt Options, url, r string, want int) (io.ReadCloser, error) {
	client := opt.Client
	if client == nil {
		client = http.DefaultClient
	}
	for try := 0; ; try++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		if opt.UserAgent != "" {
			req.Header.Set("User-Agent", opt.UserAgent)
		}
		if r != "" {
			req.Header.Set("Range", r)
		}
		resp, err := client.Do(req) // #nosec G107 -- the URL is the bulk channel of the source, from the adapter, the catalogue or the user.
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == want {
			return resp.Body, nil
		}
		_ = resp.Body.Close()
		busy := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable
		if !busy || try >= Retries {
			return nil, &StatusError{URL: url, Code: resp.StatusCode, Status: resp.Status}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryWait(resp.Header.Get("Retry-After"), try)):
		}
	}
}

// retryWait is the wait before the next try: the seconds of the Retry-After header when it has them, or else 2 seconds doubled for each try. It is at most MaxWait.
func retryWait(after string, try int) time.Duration {
	d := time.Duration(2<<min(try, 10)) * time.Second
	if n, err := strconv.Atoi(strings.TrimSpace(after)); err == nil && n >= 0 {
		d = time.Duration(n) * time.Second
	}
	return min(d, MaxWait)
}

// download gets the bulk channel at url into the cache and returns the path, the hash of the bytes and the commit of the source when the adapter gives one. When the adapter is a Fetcher, Fetch makes the snapshot, and base is the m2 cache directory of the request. When want is not empty, the hash must be want.
func download(ctx context.Context, opt Options, base string, a Adapter, url, want string) (path, hash, commit string, err error) {
	if err := os.MkdirAll(opt.Cache, 0o750); err != nil {
		return "", "", "", err
	}
	tmp, err := os.CreateTemp(opt.Cache, ".download-*")
	if err != nil {
		return "", "", "", err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	h := blake3.New()
	w := io.MultiWriter(tmp, h)
	get := getter(opt)
	if f, ok := a.(Fetcher); ok {
		req := &Request{URL: url, Root: opt.Root, Cache: base, Get: get, GetRange: rangeGetter(opt)}
		err = f.Fetch(ctx, req, w)
		commit = req.Commit
	} else {
		var body io.ReadCloser
		if body, err = get(ctx, url); err == nil {
			_, err = io.Copy(w, body)
			_ = body.Close()
		}
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", "", "", fmt.Errorf("fetch %s: %w", url, err)
	}
	hash = "blake3:" + hex.EncodeToString(h.Sum(nil))
	if want != "" && hash != want {
		return "", "", "", fmt.Errorf("fetch %s: the bytes have the hash %s, and the catalogue has %s", url, hash, want)
	}
	path = filepath.Join(opt.Cache, strings.TrimPrefix(hash, "blake3:"))
	return path, hash, commit, os.Rename(tmp.Name(), path)
}
