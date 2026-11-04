package cluster

import (
	"container/heap"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource/spill"
)

// Sorter collects the encoded records of one source and writes the same files as WriteEncoded, with less memory. It keeps at most ShardSize+1 records in memory. When it has more than ShardSize, it sorts them by the native id and writes them as one run to a spill file, and Write merges the runs.
type Sorter struct {
	dir  string
	buf  []Encoded
	runs []string
	n    int
}

// spilled is one record of a run.
type spilled struct {
	ID string          `json:"id"`
	R  json.RawMessage `json:"r"`
}

// NewSorter gives a Sorter that writes its runs to a new directory in dir. Write removes the directory.
func NewSorter(dir string) (*Sorter, error) {
	d, err := os.MkdirTemp(dir, "sort-")
	if err != nil {
		return nil, err
	}
	return &Sorter{dir: d}, nil
}

// Add adds one record.
func (s *Sorter) Add(e Encoded) error {
	s.buf = append(s.buf, e)
	s.n++
	if len(s.buf) <= ShardSize {
		return nil
	}
	return s.spill()
}

// Remove removes the spill directory, for a run that stops before Write.
func (s *Sorter) Remove() error { return os.RemoveAll(s.dir) }

// Len gives the count of the records.
func (s *Sorter) Len() int { return s.n }

func (s *Sorter) spill() error {
	slices.SortStableFunc(s.buf, func(a, b Encoded) int { return strings.Compare(a.NativeID, b.NativeID) })
	path := filepath.Join(s.dir, fmt.Sprintf("run-%04d.gz", len(s.runs)))
	w, err := spill.Create(path)
	if err != nil {
		return err
	}
	for _, e := range s.buf {
		if err := w.Write(spilled{e.NativeID, e.JSON}); err != nil {
			_ = w.Close()
			return err
		}
	}
	if err := w.Close(); err != nil {
		return err
	}
	s.runs = append(s.runs, path)
	s.buf = s.buf[:0]
	return nil
}

// Write writes the records to RecordsDir/<source>/<name>.md in the tree at root, as WriteEncoded does, and removes the spill directory.
func (s *Sorter) Write(root, source, name string) error {
	defer func() { _ = s.Remove() }()
	if len(s.runs) == 0 {
		return WriteEncoded(root, source, name, s.buf)
	}
	if len(s.buf) > 0 {
		if err := s.spill(); err != nil {
			return err
		}
	}
	dir, err := recordsDir(root, source, name)
	if err != nil {
		return err
	}
	m := &merge{}
	defer m.close()
	for i, path := range s.runs {
		r, err := spill.Open(path)
		if err != nil {
			return err
		}
		m.rs = append(m.rs, r)
		h, ok, err := m.read(i)
		if err != nil {
			return err
		}
		if ok {
			m.heads = append(m.heads, h)
		}
	}
	heap.Init(m)
	var out [][]byte
	file := 0
	flush := func() error {
		file++
		err := mdrec.Write(filepath.Join(dir, fmt.Sprintf("%s-%04d%s", name, file, mdrec.Ext)), out)
		out = out[:0]
		return err
	}
	for m.Len() > 0 {
		out = append(out, m.heads[0].rec.R)
		if len(out) == ShardSize {
			if err := flush(); err != nil {
				return err
			}
		}
		h, ok, err := m.read(m.heads[0].run)
		if err != nil {
			return err
		}
		if ok {
			m.heads[0] = h
			heap.Fix(m, 0)
		} else {
			heap.Pop(m)
		}
	}
	if len(out) > 0 {
		return flush()
	}
	return nil
}

type head struct {
	run int
	rec spilled
}

// merge is a heap of the next record of each run. Two records with one native id come in the order of their runs, so that the merge is stable, as the sort of WriteEncoded.
type merge struct {
	rs    []*spill.Reader
	heads []head
}

// read gives the next record of the run i, and false at the end of the run.
func (m *merge) read(i int) (head, bool, error) {
	line, err := m.rs[i].Next()
	if errors.Is(err, io.EOF) {
		return head{}, false, nil
	}
	if err != nil {
		return head{}, false, err
	}
	var r spilled
	err = json.Unmarshal(line, &r)
	return head{run: i, rec: r}, err == nil, err
}

func (m *merge) close() {
	for _, r := range m.rs {
		_ = r.Close()
	}
}

func (m *merge) Len() int { return len(m.heads) }
func (m *merge) Less(i, j int) bool {
	a, b := m.heads[i], m.heads[j]
	return a.rec.ID < b.rec.ID || (a.rec.ID == b.rec.ID && a.run < b.run)
}
func (m *merge) Swap(i, j int) { m.heads[i], m.heads[j] = m.heads[j], m.heads[i] }
func (m *merge) Push(x any)    { m.heads = append(m.heads, x.(head)) }
func (m *merge) Pop() any {
	h := m.heads[len(m.heads)-1]
	m.heads = m.heads[:len(m.heads)-1]
	return h
}
