package marc

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Lines writes a snapshot: one JSON object for each line, compressed with gzip.
type Lines struct {
	zw  *gzip.Writer
	enc *json.Encoder
}

// NewLines gives a Lines that writes to w.
func NewLines(w io.Writer) *Lines {
	zw := gzip.NewWriter(w)
	enc := json.NewEncoder(zw)
	enc.SetEscapeHTML(false)
	return &Lines{zw: zw, enc: enc}
}

// Add writes v as one line.
func (l *Lines) Add(v any) error { return l.enc.Encode(v) }

// Close ends the gzip stream. It does not close the writer below.
func (l *Lines) Close() error { return l.zw.Close() }

// EachBook reads a snapshot of books, one JSON object for each line, and calls fn for each book. A book with no id is an error.
func EachBook(r io.Reader, fn func(Book) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 64<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var b Book
		if err := json.Unmarshal(sc.Bytes(), &b); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if b.ID == "" {
			return fmt.Errorf("line %d: the book has no id", line)
		}
		if err := fn(b); err != nil {
			return err
		}
	}
	return sc.Err()
}
