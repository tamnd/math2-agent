// Package marc reads bibliographic records in MARC 21 and UNIMARC for the adapters of the catalogues of resources (spec corpus resources RS4.1 and RS8.3). It reads the transmission format of ISO 2709 and the XML forms MARCXML and MarcXchange, and it maps a record of MARC 21 or of UNIMARC to a book.
package marc

import (
	"bufio"
	"bytes"
	"compress/gzip"
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

// Encode writes one record in the transmission format of ISO 2709. It sets the record length and the base address in the leader. A record with no leader of 24 characters gets the leader of a book in MARC 21 with UTF-8. The tests of the adapters use Encode to make their fixtures.
func Encode(rec Record) []byte {
	var dir, data bytes.Buffer
	for _, f := range rec.Fields {
		var d []byte
		if f.Tag < "010" {
			d = []byte(f.Value)
		} else {
			ind := f.Ind
			if len(ind) != 2 {
				ind = "  "
			}
			d = []byte(ind)
			for _, s := range f.Subfields {
				d = append(d, subfield)
				d = append(d, s[0]+s[1]...)
			}
		}
		d = append(d, 0x1e)
		fmt.Fprintf(&dir, "%-3.3s%04d%05d", f.Tag, len(d), data.Len())
		data.Write(d)
	}
	dir.WriteByte(0x1e)
	base := 24 + dir.Len()
	n := base + data.Len() + 1
	leader := rec.Leader
	if len(leader) != 24 {
		leader = "00000cam a2200000 a 4500"
	}
	leader = fmt.Sprintf("%05d", n) + leader[5:12] + fmt.Sprintf("%05d", base) + leader[17:]
	return append([]byte(leader+dir.String()+data.String()), 0x1d)
}

// Each reads a file of records and calls fn for each record. The file is in ISO 2709 or in MARCXML, and it can be compressed with gzip. Each finds the form from the first bytes.
func Each(r io.Reader, fn func(Record) error) error {
	br := bufio.NewReaderSize(r, 1<<20)
	if head, _ := br.Peek(2); len(head) == 2 && head[0] == 0x1f && head[1] == 0x8b {
		zr, err := gzip.NewReader(br)
		if err != nil {
			return fmt.Errorf("marc: %w", err)
		}
		defer func() { _ = zr.Close() }()
		br = bufio.NewReaderSize(zr, 1<<20)
	}
	if xmlStart(br) {
		return EachXML(br, fn)
	}
	rd := &Reader{br: br}
	for {
		rec, err := rd.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := fn(rec); err != nil {
			return err
		}
	}
}

// xmlStart reports whether the first byte after the spaces and the byte order mark is <.
func xmlStart(br *bufio.Reader) bool {
	for i := 1; i <= 4096; i++ {
		b, _ := br.Peek(i)
		if len(b) < i {
			return false
		}
		switch b[i-1] {
		case '<':
			return true
		case ' ', '\t', '\r', '\n', 0xef, 0xbb, 0xbf:
			continue
		default:
			return false
		}
	}
	return false
}
