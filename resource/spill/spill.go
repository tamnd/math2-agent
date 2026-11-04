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

// Read calls fn with each line of a file of a Writer, with no newline.
func Read(path string, fn func([]byte) error) error {
	f, err := os.Open(path) // #nosec G304 -- the path is in the spill directory of the run.
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	z, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	r := bufio.NewReaderSize(z, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 1 {
			if err := fn(line[:len(line)-1]); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
