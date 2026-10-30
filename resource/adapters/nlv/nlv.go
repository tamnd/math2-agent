// Package nlv is the adapter of the catalogue of the National Library of Vietnam (spec corpus resources RS4.1 and RS8.3, phase R3). It reads the books of mathematics in the monthly ISO files of the NLV, and it gives an ingest record and a source record for each book.
//
// The NLV gives its records as monthly files in MARC 21, in the transmission format of ISO 2709, and through Z39.50. The adapter does not use Z39.50. Fetch reads the files at a local path or at a URL: one file, or each file of a directory in the order of the names. A file can be compressed with gzip. The adapter keeps the records whose DDC class is in 510 to 519 or whose UDC class starts with 51, as the test S3 of RS9.2 does, and it writes one JSON object for each book and each line, compressed with gzip. The NLV states no licence (RQ06). Thus math2 holds the records, and the class is record, so that an export gives only the facts until the NLV answers a request. The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/nlv"
package nlv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource/adapters/marc"
	"github.com/tamnd/math2-agent/resource/cluster"
)

// Home is the home page of the NLV. It is not a file of records, so a fetch needs the path or the URL of the files.
const Home = "https://nlv.gov.vn"

// Adapter is the adapter of the monthly ISO files of the NLV.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of the NLV.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "nlv",
		Name:        "National Library of Vietnam, the monthly ISO files of the catalogue",
		Home:        Home,
		Licence:     "LicenseRef-unknown",
		Class:       ingest.RecordOnly,
		Attribution: "National Library of Vietnam, https://nlv.gov.vn",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "nlv/1" }

// Dump returns the home page of the NLV. The NLV has no fixed URL for its files, so the user gives the path or the URL of the files.
func (Adapter) Dump() string { return Home }

// Fetch reads the files at req.URL, a local path or a URL, and writes the books of mathematics as JSON Lines, compressed with gzip.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	src := strings.TrimSpace(req.URL)
	if src == "" || strings.TrimSuffix(src, "/") == Home {
		return errors.New("nlv: give the path or the URL of the monthly ISO files with -url")
	}
	out := marc.NewLines(w)
	keep := func(rec marc.Record) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		b := marc.FromMARC21(rec)
		if b.ID == "" || !marc.Math(b) {
			return nil
		}
		return out.Add(b)
	}
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		body, err := req.Get(ctx, src)
		if err != nil {
			return err
		}
		err = marc.Each(body, keep)
		_ = body.Close()
		if err != nil {
			return fmt.Errorf("nlv: %s: %w", src, err)
		}
		return out.Close()
	}
	files, err := Files(src)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := readFile(f, keep); err != nil {
			return fmt.Errorf("nlv: %s: %w", f, err)
		}
	}
	return out.Close()
}

// Files gives the files at a local path: the path when it is a file, or each regular file of the directory whose name does not start with a period, in the order of the names.
func Files(path string) ([]string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("nlv: %w", err)
	}
	if !fi.IsDir() {
		return []string{path}, nil
	}
	es, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("nlv: %w", err)
	}
	var out []string
	for _, e := range es {
		if e.Type().IsRegular() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, filepath.Join(path, e.Name()))
		}
	}
	slices.Sort(out)
	if len(out) == 0 {
		return nil, fmt.Errorf("nlv: the directory %s has no file", path)
	}
	return out, nil
}

func readFile(path string, fn func(marc.Record) error) error {
	f, err := os.Open(path) // #nosec G304 -- the user gives the path of the files.
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return marc.Each(f, fn)
}

// Read reads the books and emits one ingest record for each book.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return marc.EachBook(r, func(b marc.Book) error { return emit(ingest.Record{ID: b.ID, Kind: marc.Kind(b), Title: b.Title}) })
}

// Records reads the books and emits one source record for each book (RS13.1). The NLV has no stable page for a record, so the record has no link.
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return marc.EachBook(r, func(b marc.Book) error { return emit(marc.SourceRecord(b, "")) })
}
