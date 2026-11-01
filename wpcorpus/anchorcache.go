package wpcorpus

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The anchor cache keeps the anchor links of all the articles of one dump, so
// that m2 wp pointers reads the dump one time for each dump date. The file is
// gzip text. The first line is "# anchor links of DUMP, N pages", and each
// other line is the title of the page, the link title and the fragment,
// separated by tabs. The lines of one page are together and in order.

// AnchorCache writes an anchor cache.
type AnchorCache struct {
	f     *os.File
	gz    *gzip.Writer
	w     *bufio.Writer
	tmp   string
	path  string
	dump  string
	pages int
}

// CreateAnchorCache starts an anchor cache at path for the dump with the name dump. Close writes the file.
func CreateAnchorCache(path, dump string) (*AnchorCache, error) {
	f, err := os.CreateTemp(filepath.Dir(path), ".anchors-*")
	if err != nil {
		return nil, err
	}
	gz := gzip.NewWriter(f)
	return &AnchorCache{f: f, gz: gz, w: bufio.NewWriterSize(gz, 1<<20), tmp: f.Name(), path: path, dump: dump}, nil
}

// Add adds the anchor links of one page.
func (c *AnchorCache) Add(from string, links []AnchorLink) error {
	c.pages++
	for _, l := range links {
		if _, err := fmt.Fprintf(c.w, "%s\t%s\t%s\n", cacheField(from), cacheField(l.Title), cacheField(l.Frag)); err != nil {
			return err
		}
	}
	return nil
}

// Close writes the header and moves the file to its path.
func (c *AnchorCache) Close() error {
	if err := c.w.Flush(); err != nil {
		return c.abort(err)
	}
	if err := c.gz.Close(); err != nil {
		return c.abort(err)
	}
	if err := c.f.Close(); err != nil {
		return c.abort(err)
	}
	// The header needs the count of pages, so the body is copied after it.
	body, err := os.Open(c.tmp)
	if err != nil {
		return c.abort(err)
	}
	defer func() { _ = body.Close() }()
	out, err := os.CreateTemp(filepath.Dir(c.path), ".anchors-*")
	if err != nil {
		return c.abort(err)
	}
	gz := gzip.NewWriter(out)
	if _, err := fmt.Fprintf(gz, "# anchor links of %s, %d pages\n", c.dump, c.pages); err != nil {
		return c.abort(err)
	}
	if err := gz.Close(); err != nil {
		return c.abort(err)
	}
	// A gzip file can have more than one member, and a reader reads them in order.
	if _, err := io.Copy(out, body); err != nil {
		return c.abort(err)
	}
	if err := out.Close(); err != nil {
		return c.abort(err)
	}
	_ = os.Remove(c.tmp)
	return os.Rename(out.Name(), c.path)
}

// Abort removes the files of an anchor cache that is not complete.
func (c *AnchorCache) Abort() { _ = c.abort(nil) }

func (c *AnchorCache) abort(err error) error {
	_ = c.f.Close()
	_ = os.Remove(c.tmp)
	return err
}

func cacheField(s string) string {
	return strings.NewReplacer("\t", " ", "\n", " ").Replace(s)
}

// ReadAnchorCache reads the anchor cache at path. It calls fn one time for each page with anchor links, and it returns the name of the dump and the count of pages in the header.
func ReadAnchorCache(path string, fn func(from string, links []AnchorLink) error) (dump string, pages int, err error) {
	f, err := os.Open(path) // #nosec G304 -- the path is the anchor cache that the user names
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", 0, fmt.Errorf("%s: %w", path, err)
	}
	sc := bufio.NewScanner(gz)
	sc.Buffer(nil, 1<<20)
	if !sc.Scan() {
		return "", 0, fmt.Errorf("%s: the file has no header", path)
	}
	if _, err := fmt.Sscanf(strings.TrimPrefix(sc.Text(), "# anchor links of "), "%s %d pages", &dump, &pages); err != nil {
		return "", 0, fmt.Errorf("%s: the header %q is not correct", path, sc.Text())
	}
	dump = strings.TrimSuffix(dump, ",")
	var from string
	var links []AnchorLink
	flush := func() error {
		if len(links) == 0 {
			return nil
		}
		err := fn(from, links)
		links = nil
		return err
	}
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), "\t", 3)
		if len(parts) != 3 {
			return "", 0, fmt.Errorf("%s: the line %q does not have three fields", path, sc.Text())
		}
		if parts[0] != from {
			if err := flush(); err != nil {
				return "", 0, err
			}
			from = parts[0]
		}
		links = append(links, AnchorLink{parts[1], parts[2]})
	}
	if err := sc.Err(); err != nil {
		return "", 0, fmt.Errorf("%s: %w", path, err)
	}
	return dump, pages, flush()
}
