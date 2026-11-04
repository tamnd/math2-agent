// Package spill writes and reads the temporary files of a run that does not keep all its data in memory, as the partitioned cluster run (RS10.8). A file holds one JSON value on each line and is compressed with gzip.
package spill

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
)

// Writer writes one JSON value on each line of a file.
type Writer struct {
	f *os.File
	z *gzip.Writer
	w *bufio.Writer
}

// Create creates the file at path for a Writer.
func Create(path string) (*Writer, error) {
	f, err := os.Create(path) // #nosec G304 -- the path is in the spill directory of the run.
	if err != nil {
		return nil, err
	}
	z, err := gzip.NewWriterLevel(f, gzip.BestSpeed)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Writer{f: f, z: z, w: bufio.NewWriter(z)}, nil
}

// Write writes v as one line.
func (s *Writer) Write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.w.Write(append(b, '\n'))
	return err
}

// Close writes the rest of the data and closes the file.
func (s *Writer) Close() error {
	return errors.Join(s.w.Flush(), s.z.Close(), s.f.Close())
}

// Reader reads the lines of a file of a Writer one at a time.
type Reader struct {
	f *os.File
	z *gzip.Reader
	r *bufio.Reader
}

// Open opens the file at path for a Reader.
func Open(path string) (*Reader, error) {
	f, err := os.Open(path) // #nosec G304 -- the path is in the spill directory of the run.
	if err != nil {
		return nil, err
	}
	z, err := gzip.NewReader(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Reader{f: f, z: z, r: bufio.NewReaderSize(z, 1<<20)}, nil
}

// Next gives the next line, with no newline. At the end of the file it gives io.EOF.
func (s *Reader) Next() ([]byte, error) {
	for {
		line, err := s.r.ReadBytes('\n')
		if n := len(line); n > 0 && line[n-1] == '\n' {
			line = line[:n-1]
		}
		if len(line) > 0 {
			return line, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// Close closes the file.
func (s *Reader) Close() error {
	return errors.Join(s.z.Close(), s.f.Close())
}

// Read calls fn with each line of a file of a Writer, with no newline.
func Read(path string, fn func([]byte) error) error {
	r, err := Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	for {
		line, err := r.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := fn(line); err != nil {
			return err
		}
	}
}
