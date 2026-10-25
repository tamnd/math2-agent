package ingest

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var commitID = regexp.MustCompile(`^[0-9a-f]{40}$`)

// FetchArchive gets the git archive at req.URL, a tar file compressed with gzip, and writes its bytes to w. It sets req.Commit to the commit that git archive records in the pax global header of the archive. GitHub gives such an archive for each branch and commit, for example
//
//	https://github.com/teorth/erdosproblems/archive/refs/heads/main.tar.gz
//
// An adapter whose bulk channel is a git archive calls FetchArchive from its Fetch method, and reads the snapshot with [WalkArchive].
func FetchArchive(ctx context.Context, req *Request, w io.Writer) error {
	body, err := req.Get(ctx, req.URL)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	tee := io.TeeReader(body, w)
	zr, err := gzip.NewReader(tee)
	if err != nil {
		return fmt.Errorf("the archive: %w", err)
	}
	h, err := tar.NewReader(zr).Next()
	if err != nil {
		return fmt.Errorf("the archive: %w", err)
	}
	commit := h.PAXRecords["comment"]
	if h.Typeflag != tar.TypeXGlobalHeader || !commitID.MatchString(commit) {
		return errors.New("the archive does not start with the commit of a git archive")
	}
	req.Commit = commit
	_, err = io.Copy(io.Discard, tee)
	return err
}

// WalkArchive calls fn for each regular file of a tar archive, in the order of the archive, and returns the commit of the archive. The name that fn gets is the path of the file without its first directory, because a git archive puts all the files in one directory, such as erdosproblems-main/. The commit is empty when the archive does not record one. WalkArchive returns the first error from fn.
func WalkArchive(r io.Reader, fn func(name string, file io.Reader) error) (commit string, err error) {
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return commit, nil
		}
		if err != nil {
			return "", fmt.Errorf("the archive: %w", err)
		}
		if h.Typeflag == tar.TypeXGlobalHeader && commitID.MatchString(h.PAXRecords["comment"]) {
			commit = h.PAXRecords["comment"]
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		_, name, ok := strings.Cut(h.Name, "/")
		if !ok {
			name = h.Name
		}
		if err := fn(name, tr); err != nil {
			return "", err
		}
	}
}
