// Package erdosproblems is the ingest adapter of the Erdős problems database, teorth/erdosproblems (spec 09.2). It gives one record for each problem in data/problems.yaml, with its status at the commit of the snapshot.
//
// The bulk channel is the git archive of the main branch, and the catalogue records its commit. The data file has no statements, so the records have no text. The meta of a record has these keys:
//
//	status           the status, for example open, proved or disproved (Lean)
//	status_date      the date of the last change of the status
//	informal_status  the status of the informal problem
//	formal_status    the status of the formal solution, for example Lean or unformalized
//	formalized       yes when formal-conjectures has a statement of the problem
//	prize            the prize, for example $500, when the problem has one
//	oeis             the OEIS sequences of the problem, with a space between them
//	tags             the subjects, with "|" between them
//	name             the short name of the problem, for example sunflower conjecture
//	posed_by         Paul Erdős, the person who posed the problems (rule G06)
//
// A key is not in the meta when the data file has no value for it. The data is under the Apache License 2.0.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/erdosproblems"
package erdosproblems

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/ingest"
)

// Repository is the git repository of the database.
const Repository = "https://github.com/teorth/erdosproblems"

// Archive is the git archive of the main branch of the repository.
const Archive = Repository + "/archive/refs/heads/main.tar.gz"

// Data is the path of the data file in the repository.
const Data = "data/problems.yaml"

// Poser is the person who posed the problems.
const Poser = "Paul Erdős"

// Adapter is the adapter of the Erdős problems database.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of the database.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "erdosproblems",
		Name:        "Erdős problems",
		Home:        "https://www.erdosproblems.com",
		Licence:     "Apache-2.0",
		Class:       ingest.Open,
		Attribution: "The contributors of teorth/erdosproblems, the community database of the problems of erdosproblems.com, " + Repository + ", Apache License 2.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "erdosproblems/1" }

// Dump returns the git archive of the main branch.
func (Adapter) Dump() string { return Archive }

// Fetch gets the git archive at req.URL and sets req.Commit to its commit.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	return ingest.FetchArchive(ctx, req, w)
}

// state is a status with the date of its last change.
type state struct {
	State      string `yaml:"state"`
	LastUpdate string `yaml:"last_update"`
}

// problem is one entry of the data file.
type problem struct {
	Number         string   `yaml:"number"`
	Prize          string   `yaml:"prize"`
	Status         state    `yaml:"status"`
	InformalStatus state    `yaml:"informal_status"`
	FormalStatus   state    `yaml:"formal_status"`
	Formalized     state    `yaml:"formalized"`
	OEIS           []string `yaml:"oeis"`
	Tags           []string `yaml:"tags"`
	Comments       string   `yaml:"comments"`
}

// Read reads a git archive of the repository and emits one record for each problem in the data file. Read fails when the archive has no data file, on a number that does not pass crosswalk.Check, on a problem with no status and on a data file with no problems.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	found := false
	_, err := ingest.WalkArchive(r, func(name string, f io.Reader) error {
		if name != Data {
			return nil
		}
		found = true
		return read(f, emit)
	})
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("the archive has no %s", Data)
	}
	return nil
}

func read(r io.Reader, emit func(ingest.Record) error) error {
	var ps []problem
	if err := yaml.NewDecoder(r).Decode(&ps); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s: %w", Data, err)
	}
	if len(ps) == 0 {
		return fmt.Errorf("%s has no problems", Data)
	}
	for i, p := range ps {
		if err := crosswalk.Check("erdos", p.Number); err != nil {
			return fmt.Errorf("%s: problem %d: %w", Data, i+1, err)
		}
		if p.Status.State == "" {
			return fmt.Errorf("%s: problem %s has no status", Data, p.Number)
		}
		if err := emit(record(p)); err != nil {
			return err
		}
	}
	return nil
}

// record makes the record of one problem.
func record(p problem) ingest.Record {
	meta := map[string]string{"posed_by": Poser}
	for k, v := range map[string]string{
		"status":          p.Status.State,
		"status_date":     p.Status.LastUpdate,
		"informal_status": p.InformalStatus.State,
		"formal_status":   p.FormalStatus.State,
		"formalized":      p.Formalized.State,
		"prize":           p.Prize,
		"oeis":            strings.Join(p.OEIS, " "),
		"tags":            strings.Join(p.Tags, "|"),
		"name":            p.Comments,
	} {
		if v != "" && (k != "prize" || v != "no") {
			meta[k] = v
		}
	}
	return ingest.Record{
		ID:    p.Number,
		Kind:  "problem",
		Title: "Erdős Problem #" + p.Number,
		URL:   "https://www.erdosproblems.com/" + p.Number,
		Meta:  meta,
	}
}
