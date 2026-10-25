package prov

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// RunsDir is the directory of the run records in the artefact store, and in the build directory of a working tree that has a copy of them.
const RunsDir = "runs"

// Run is the summary of the record of one agent run (spec 11.6). The full record, with the transcript and the tool calls, is in the artefact store under the same id. The summary is in RunsDir/<id>.json.
type Run struct {
	ID         string            `json:"id"`                   // the run id, for example r-2026-11-02-8812
	Agent      string            `json:"agent"`                // the agent role at its version
	Prompt     string            `json:"prompt"`               // the prompt version
	Models     []string          `json:"models"`               // the model ids that the run used
	Tools      map[string]string `json:"tools,omitempty"`      // the version of each tool
	Images     []string          `json:"images,omitempty"`     // the images as name@sha256:HEX
	OnBehalfOf string            `json:"on_behalf_of"`         // the person, project or schedule that started the run
	Started    string            `json:"started"`              // the start time, RFC 3339
	Outcome    string            `json:"outcome"`              // success, failure or partial
	Transcript string            `json:"transcript,omitempty"` // the hash of the full record in the artefact store, blake3:HEX
}

// Validate reports the first problem of the run record: an id or an agent that is not in its form, no prompt version, no model, no one on whose behalf it acted, a start time that is not RFC 3339 or an outcome that is not known.
func (r *Run) Validate() error {
	switch {
	case !IsRun(r.ID):
		return fmt.Errorf("the run id %q is not in the form r-YYYY-MM-DD-SERIAL", r.ID)
	case !IsAgent(r.Agent):
		return fmt.Errorf("the agent %q is not a role at a version such as m2-agent/formalize@0.9.1", r.Agent)
	case r.Prompt == "":
		return errors.New("the run names no prompt version")
	case len(r.Models) == 0:
		return errors.New("the run names no model")
	case r.OnBehalfOf == "":
		return errors.New("the run names no person, project or schedule on whose behalf it acted")
	case !IsResponsible(r.OnBehalfOf):
		return fmt.Errorf("the run acted on behalf of %q, and that is not a person, a project or a schedule", r.OnBehalfOf)
	case !slices.Contains([]string{"success", "failure", "partial"}, r.Outcome):
		return fmt.Errorf("the outcome %q is not success, failure or partial", r.Outcome)
	}
	if _, err := time.Parse(time.RFC3339, r.Started); err != nil {
		return fmt.Errorf("the start time %q is not RFC 3339", r.Started)
	}
	return nil
}

// ReadRun reads the summary of the run with the id from the directory dir. A run that is not in dir gives an error that wraps fs.ErrNotExist.
func ReadRun(dir, id string) (*Run, error) {
	if !IsRun(id) {
		return nil, fmt.Errorf("prov: the run id %q is not in the form r-YYYY-MM-DD-SERIAL", id)
	}
	path := filepath.Join(dir, id+".json")
	data, err := os.ReadFile(path) // #nosec G304 -- the id is checked above, thus the path is in dir.
	if err != nil {
		return nil, err
	}
	var r Run
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("prov: %s: %w", path, err)
	}
	return &r, nil
}

// ReadRuns reads the summary of each run in the directory dir, by the run id in its file name. A dir that does not exist gives an error that wraps fs.ErrNotExist.
func ReadRuns(dir string) (map[string]*Run, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	out := map[string]*Run{}
	for _, f := range files {
		id := strings.TrimSuffix(filepath.Base(f), ".json")
		if !IsRun(id) {
			continue
		}
		r, err := ReadRun(dir, id)
		if err != nil {
			return nil, err
		}
		out[id] = r
	}
	return out, nil
}

// WriteRun writes the summary of the run to dir/<id>.json. It checks the record first.
func WriteRun(dir string, r *Run) error {
	if err := r.Validate(); err != nil {
		return fmt.Errorf("prov: %w", err)
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- the run summaries are public (spec 11.6).
		return err
	}
	return os.WriteFile(filepath.Join(dir, r.ID+".json"), append(data, '\n'), 0o644) // #nosec G306 -- the run summaries are public (spec 11.6).
}
