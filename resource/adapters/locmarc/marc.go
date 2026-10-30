package locmarc

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Field is one field of a MARC record. A control field has only Value. A data field has the indicators and the subfields.
type Field struct {
	Tag       string
	Value     string
	Ind       string
	Subfields [][2]string // the code and the value of each subfield, in order
}

// Sub gives the values of the subfields with the code c.
func (f Field) Sub(c string) []string {
	var out []string
	for _, s := range f.Subfields {
		if s[0] == c {
			out = append(out, s[1])
		}
	}
	return out
}

// First gives the first value of the subfield with the code c, or empty.
func (f Field) First(c string) string {
	for _, s := range f.Subfields {
		if s[0] == c {
			return s[1]
		}
	}
	return ""
}

// Record is one MARC record.
type Record struct {
	Leader string
	Fields []Field
}

// All gives the fields with the tag.
func (r Record) All(tag string) []Field {
	var out []Field
	for _, f := range r.Fields {
		if f.Tag == tag {
			out = append(out, f)
		}
	}
	return out
}

// Control gives the value of the first control field with the tag, or empty.
func (r Record) Control(tag string) string {
	for _, f := range r.Fields {
		if f.Tag == tag {
			return f.Value
		}
	}
	return ""
}

// subfield starts each subfield of a data field.
const subfield = 0x1f

// Reader reads the records of a file of MARC 21 in the transmission format of ISO 2709, encoded in UTF-8.
type Reader struct{ br *bufio.Reader }

// NewReader gives a Reader of r.
func NewReader(r io.Reader) *Reader { return &Reader{br: bufio.NewReaderSize(r, 1<<20)} }

// Read reads the next record. It gives io.EOF after the last record.
func (rd *Reader) Read() (Record, error) {
	head, err := rd.br.Peek(5)
	if errors.Is(err, io.EOF) && len(head) == 0 {
		return Record{}, io.EOF
	}
	if err != nil {
		return Record{}, fmt.Errorf("marc: %w", err)
	}
	n, err := strconv.Atoi(string(head))
	if err != nil || n < 25 {
		return Record{}, fmt.Errorf("marc: the record length %q is not valid", head)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(rd.br, buf); err != nil {
		return Record{}, fmt.Errorf("marc: %w", err)
	}
	return Parse(buf)
}

// Parse parses one record.
func Parse(b []byte) (Record, error) {
	if len(b) < 25 {
		return Record{}, errors.New("marc: the record is too short")
	}
	base, err := strconv.Atoi(string(b[12:17]))
	if err != nil || base < 25 || base > len(b) {
		return Record{}, fmt.Errorf("marc: the base address %q is not valid", b[12:17])
	}
	rec := Record{Leader: string(b[:24])}
	dir := b[24 : base-1]
	for i := 0; i+12 <= len(dir); i += 12 {
		tag := string(dir[i : i+3])
		length, err1 := strconv.Atoi(string(dir[i+3 : i+7]))
		start, err2 := strconv.Atoi(string(dir[i+7 : i+12]))
		if err1 != nil || err2 != nil || base+start+length > len(b) {
			return Record{}, fmt.Errorf("marc: the directory entry %q is not valid", dir[i:i+12])
		}
		data := bytes.TrimRight(b[base+start:base+start+length], "\x1e\x1d")
		f := Field{Tag: tag}
		if tag < "010" {
			f.Value = string(data)
		} else {
			if len(data) >= 2 {
				f.Ind, data = string(data[:2]), data[2:]
			}
			for _, part := range bytes.Split(data, []byte{subfield})[1:] {
				if len(part) == 0 {
					continue
				}
				f.Subfields = append(f.Subfields, [2]string{string(part[:1]), strings.TrimSpace(string(part[1:]))})
			}
		}
		rec.Fields = append(rec.Fields, f)
	}
	return rec, nil
}
