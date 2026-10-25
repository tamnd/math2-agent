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

// Pull imports one snapshot of the source of the adapter into the tree at opt.Root. It records the snapshot in the catalogue, writes the records to build/records/<source>.jsonl and writes the import manifest. Two pulls of the same snapshot with the same adapter version write the same bytes.
func Pull(ctx context.Context, a Adapter, opt Options) (*Result, error) {
	if opt.Cache == "" {
		dir, err := os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		opt.Cache = filepath.Join(dir, "m2")
	}
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
			path, _, err = download(ctx, opt, sn.URL, sn.Hash)
		}
		if err != nil {
			return nil, fmt.Errorf("ingest %s: %w", id, err)
		}
	} else {
		url := opt.URL
		if url == "" {
			url = a.Dump()
		}
		var hash string
		if path, hash, err = download(ctx, opt, url, ""); err != nil {
			return nil, fmt.Errorf("ingest %s: %w", id, err)
		}
		if sn := src.Snapshot(hash); sn != nil {
			res.Snapshot = *sn
		} else {
			date := opt.Date
			if date == "" {
				date = time.Now().UTC().Format(time.DateOnly)
			}
			res.Snapshot = Snapshot{URL: url, Hash: hash, Date: date}
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

// cached returns the path of the snapshot with the hash in the cache.
func cached(cache, hash string) (string, error) {
	path := filepath.Join(cache, strings.TrimPrefix(hash, "blake3:"))
	_, err := os.Stat(path)
	return path, err
}

// download gets url into the cache and returns the path and the hash of the bytes. When want is not empty, the hash must be want.
func download(ctx context.Context, opt Options, url, want string) (path, hash string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	if opt.UserAgent != "" {
		req.Header.Set("User-Agent", opt.UserAgent)
	}
	client := opt.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req) // #nosec G107 -- the URL is the bulk channel of the source, from the adapter, the catalogue or the user.
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	if err := os.MkdirAll(opt.Cache, 0o750); err != nil {
		return "", "", err
	}
	tmp, err := os.CreateTemp(opt.Cache, ".download-*")
	if err != nil {
		return "", "", err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	h := blake3.New()
	_, err = io.Copy(io.MultiWriter(tmp, h), resp.Body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", "", fmt.Errorf("GET %s: %w", url, err)
	}
	hash = "blake3:" + hex.EncodeToString(h.Sum(nil))
	if want != "" && hash != want {
		return "", "", fmt.Errorf("GET %s: the bytes have the hash %s, and the catalogue has %s", url, hash, want)
	}
	path = filepath.Join(opt.Cache, strings.TrimPrefix(hash, "blake3:"))
	return path, hash, os.Rename(tmp.Name(), path)
}
