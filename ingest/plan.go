package ingest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// PlanFile is the import plan of the pipeline, relative to the root of the math2 working tree (corpus K12.1).
const PlanFile = "corpus/plan.yaml"

// Plan is the list of the importers that one run of the pipeline runs, one after the other.
type Plan struct {
	Imports []Step `yaml:"imports"`
}

// Step is one importer of a plan.
type Step struct {
	Source string `yaml:"source"` // the id of the source, which must have an adapter

	// URL is the pinned bulk channel, for example a raw file at a commit. Empty means the Dump of the adapter.
	URL string `yaml:"url,omitempty"`

	// Snapshot is the hash of a snapshot in the catalogue that the step uses again. Empty means a new download.
	Snapshot string `yaml:"snapshot,omitempty"`
}

var (
	sourceID     = regexp.MustCompile(`^[a-z0-9_]+$`)
	snapshotHash = regexp.MustCompile(`^blake3:[0-9a-f]{64}$`)
)

// ReadPlan reads the import plan at path and checks it. A plan has at least one step, each source has one step, and a step has a URL or a snapshot, not both.
func ReadPlan(path string) (*Plan, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the caller names the plan.
	if err != nil {
		return nil, err
	}
	var p Plan
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if problems := p.Problems(); len(problems) > 0 {
		return nil, fmt.Errorf("%s: %s", path, strings.Join(problems, "; "))
	}
	return &p, nil
}

// Problems returns what is not correct in the plan, or nil.
func (p *Plan) Problems() []string {
	var out []string
	if len(p.Imports) == 0 {
		out = append(out, "the plan has no imports")
	}
	seen := map[string]bool{}
	for i, s := range p.Imports {
		switch {
		case !sourceID.MatchString(s.Source):
			out = append(out, fmt.Sprintf("import %d: the source %q is not the id of a source", i+1, s.Source))
		case seen[s.Source]:
			out = append(out, fmt.Sprintf("import %d: the source %s has two imports", i+1, s.Source))
		}
		seen[s.Source] = true
		if s.URL != "" && s.Snapshot != "" {
			out = append(out, fmt.Sprintf("import %d: the source %s has a URL and a snapshot", i+1, s.Source))
		}
		if s.URL != "" && !strings.HasPrefix(s.URL, "https://") && !strings.HasPrefix(s.URL, "http://") {
			out = append(out, fmt.Sprintf("import %d: the URL %q is not an HTTP URL", i+1, s.URL))
		}
		if s.Snapshot != "" && !snapshotHash.MatchString(s.Snapshot) {
			out = append(out, fmt.Sprintf("import %d: the snapshot %q is not a BLAKE3 hash", i+1, s.Snapshot))
		}
	}
	return out
}
