package coverage

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/msc"
)

// ExpectedFile is the path of the pinned count in the math2 tree.
const ExpectedFile = "manifests/coverage/expected.yaml"

// Expected is the pinned count of the documents of each MSC2020 class in one source. It is the denominator of C0.
type Expected struct {
	Source string         `yaml:"source"` // the id of the source in the catalogue, for example zbmath
	URL    string         `yaml:"url"`    // the URL of the count of the class 11, as an example of the requests
	Date   string         `yaml:"date"`   // the date of the count, YYYY-MM-DD
	Counts map[string]int `yaml:"counts"` // the number of documents with a code in each top-level class, by the top-level code
}

// ReadExpected reads the pinned count of the math2 tree at root.
func ReadExpected(root string) (*Expected, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ExpectedFile))) // #nosec G304 -- the caller names the tree.
	if err != nil {
		return nil, err
	}
	var e Expected
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&e); err != nil {
		return nil, fmt.Errorf("%s: %w", ExpectedFile, err)
	}
	var errs []error
	if e.Source == "" || e.Date == "" {
		errs = append(errs, errors.New("the count has no source or no date"))
	}
	for _, c := range slices.Sorted(maps.Keys(e.Counts)) {
		if msc.Level(c) != 1 || e.Counts[c] < 0 {
			errs = append(errs, fmt.Errorf("the count %s: %d is not the count of a top-level code", c, e.Counts[c]))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("%s: %w", ExpectedFile, err)
	}
	return &e, nil
}

// Write writes the count to the math2 tree at root. The same count gives the same bytes.
func (e *Expected) Write(root string) error {
	var b bytes.Buffer
	b.WriteString("# The number of documents in each MSC2020 class, which is the denominator of C0 (spec 18.1).\n# m2 coverage -count writes this file. Do not edit it.\n")
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(e); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	path := filepath.Join(root, filepath.FromSlash(ExpectedFile))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o600)
}
