package ingest_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
)

// gitArchive makes a git archive of the files, with the commit in the pax global header when it is not empty.
func gitArchive(commit string, files ...string) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if commit != "" {
		_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": commit}})
	}
	_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: "repo-main/", Mode: 0o755})
	for i := 0; i+1 < len(files); i += 2 {
		_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "repo-main/" + files[i], Mode: 0o644, Size: int64(len(files[i+1]))})
		_, _ = tw.Write([]byte(files[i+1]))
	}
	_ = tw.Close()
	_ = zw.Close()
	return buf.Bytes()
}

func serve(data []byte) ingest.Getter {
	return func(context.Context, string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
}

func TestFetchArchive(t *testing.T) {
	const commit = "4d22d408cbafe9ecf5b2b1223e689783eaf76803"
	data := gitArchive(commit, "data/problems.yaml", "- number: \"1\"\n")
	var out bytes.Buffer
	req := &ingest.Request{URL: "https://github.com/teorth/erdosproblems/archive/refs/heads/main.tar.gz", Get: serve(data)}
	if err := ingest.FetchArchive(context.Background(), req, &out); err != nil {
		t.Fatal(err)
	}
	if req.Commit != commit || !bytes.Equal(out.Bytes(), data) {
		t.Errorf("FetchArchive gives the commit %q and %d bytes, want %q and %d bytes", req.Commit, out.Len(), commit, len(data))
	}
	for name, bad := range map[string][]byte{
		"no commit":  gitArchive("", "a", "b"),
		"bad commit": gitArchive("main", "a", "b"),
		"no gzip":    []byte("<html>"),
	} {
		req := &ingest.Request{URL: "x", Get: serve(bad)}
		if err := ingest.FetchArchive(context.Background(), req, io.Discard); err == nil {
			t.Errorf("FetchArchive of an archive with %s gives no error", name)
		}
	}
}

func ExampleWalkArchive() {
	data := gitArchive("4d22d408cbafe9ecf5b2b1223e689783eaf76803", "README.md", "# Erdős problem database\n", "data/problems.yaml", "- number: \"1\"\n")
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	err = ingest.WalkArchive(zr, func(name string, f io.Reader) error {
		b, err := io.ReadAll(f)
		fmt.Printf("%s: %q\n", name, strings.TrimSpace(string(b)))
		return err
	})
	if err != nil {
		panic(err)
	}
	// Output:
	// README.md: "# Erdős problem database"
	// data/problems.yaml: "- number: \"1\""
}
