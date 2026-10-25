// Package oeis is the ingest adapter of the On-Line Encyclopedia of Integer Sequences (spec 09.2). It reads the list of the names of all sequences and gives one record for each sequence, with its A-number and its name. The A-number is the id that a node names in same_as.
//
// The content of the OEIS is under CC BY-SA 4.0. The records keep the name as the title and keep no other text.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/oeis"
package oeis

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/ingest"
)

// Names is the list of the names of all sequences, compressed with gzip. The OEIS makes it again each day.
const Names = "https://oeis.org/names.gz"

// Adapter is the adapter of the OEIS.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of the OEIS.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "oeis",
		Name:        "The On-Line Encyclopedia of Integer Sequences",
		Home:        "https://oeis.org",
		Licence:     "CC-BY-SA-4.0",
		Class:       ingest.Open,
		Attribution: "The OEIS Foundation Inc., The On-Line Encyclopedia of Integer Sequences, https://oeis.org, CC BY-SA 4.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "oeis/1" }

// Dump returns the URL of the list of names.
func (Adapter) Dump() string { return Names }

// Read reads the list of names and emits one record for each sequence. A line that starts with # is a comment. Each other line is an A-number, a space and the name. Read fails on a line that is not in this form, and on a list with no sequences.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 1<<20)
	n := 0
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		id, name, _ := strings.Cut(text, " ")
		if err := crosswalk.Check("oeis", id); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		n++
		if err := emit(Record(id, name)); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if n == 0 {
		return errors.New("the list has no sequences")
	}
	return nil
}

// Record makes the ingest record of the sequence with the A-number id and the name.
func Record(id, name string) ingest.Record {
	return ingest.Record{
		ID:    id,
		Kind:  "sequence",
		Title: strings.TrimSpace(name),
		URL:   "https://oeis.org/" + id,
	}
}
