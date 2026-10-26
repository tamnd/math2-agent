// Package agent runs the agent roles of spec 12 with the common loop of spec 12.2.
//
// A [Role] is a role at its version (spec 12.8). [LoadRole] reads it from the directory of the role: role.yaml with the version, the tool profile, the escalation ladder and the limits, system.md with the system prompt, and the files in style/ and examples/. The run records the hash of each file that it loaded.
//
// A [Job] is the unit of work of spec 11.4: a role, a target, a goal and a [Budget]. A [Runner] runs one job on one rung of the ladder of the role. The model sees only the typed tools of the profile of the role, never a shell. Each turn checks the budget and the progress. The run stops when the goal predicate of the runner holds, when the budget or the turns are used up, when the model makes no progress for some turns, or when the model calls the tool report_stuck with a reason. The goal predicate is the verifier: the run records the own assessment of the model and never uses it.
//
// The [Record] of a run has the job, the role version, the hashes of the prompt files, the models, the full transcript, the outcome with its reason and the spend (spec 11.6). [WriteRecord] writes the record and its summary, a [prov.Run], to the artefact store.
//
// [Permit] checks the changes of a run in its [workspace.Workspace] against the field permission set of its role (spec 12.1). A change outside the set fails, even if it is correct.
package agent

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/prov"
)

// The roles of spec 12.1.
var roles = []string{"formalize", "prove", "explore", "explain", "curate", "align", "bump", "review-assist"}

// Roles returns the names of the eight roles of spec 12.1.
func Roles() []string { return slices.Clone(roles) }

// Rung is one step of the escalation ladder of a role: a model and an effort (spec 11.4).
type Rung struct {
	Model  string `yaml:"model" json:"model"`
	Effort string `yaml:"effort,omitempty" json:"effort,omitempty"`
}

// Duration is a time.Duration that YAML and JSON write as text, for example 2h0m0s.
type Duration time.Duration

// MarshalText writes the duration as text.
func (d Duration) MarshalText() ([]byte, error) { return []byte(time.Duration(d).String()), nil }

// UnmarshalText reads a duration such as 90m or 2h.
func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// Budget is the budget of a job (spec 11.5). A zero field has no limit.
type Budget struct {
	USD          float64  `yaml:"usd,omitempty" json:"usd,omitempty"`
	Wall         Duration `yaml:"wall,omitempty" json:"wall,omitempty"`
	LeanSessionS float64  `yaml:"lean_session_s,omitempty" json:"lean_session_s,omitempty"`
	ComputeCPUS  float64  `yaml:"compute_cpu_s,omitempty" json:"compute_cpu_s,omitempty"`
}

// Role is a role at its version, with its prompts (spec 12.8).
type Role struct {
	Name      string            `yaml:"role"`
	Version   string            `yaml:"version"`    // the role version, semver
	Tools     []string          `yaml:"tools"`      // the tool profile: the names of the tools that the model can call
	Ladder    []Rung            `yaml:"ladder"`     // the escalation ladder, from the cheapest rung
	Budget    Budget            `yaml:"budget"`     // the budget of a job that gives none
	MaxTokens int               `yaml:"max_tokens"` // the limit of the output tokens of one model turn
	Turns     int               `yaml:"turns"`      // the limit of the model turns of a run
	Stuck     int               `yaml:"stuck"`      // the number of turns with no progress after which the run stops
	System    string            `yaml:"-"`          // the system prompt: system.md, then the files of style/ in order
	Examples  []string          `yaml:"-"`          // the worked examples in examples/, in order
	Checks    map[string]string `yaml:"-"`          // the prompts of the checks in checks/ that the goal runs with a model, by the file name without .md
	Files     map[string]string `yaml:"-"`          // the hash of each file that LoadRole read, by its path in the role directory
}

var semverRe = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// LoadRole reads the role with the name from fsys. The files are name/role.yaml, name/system.md, name/CHANGELOG.md and the files in name/style/, name/examples/ and name/checks/. The role in role.yaml must be the name, and the version must be semver.
func LoadRole(fsys fs.FS, name string) (*Role, error) {
	if !slices.Contains(roles, name) {
		return nil, fmt.Errorf("agent: %q is not a role of spec 12.1", name)
	}
	r := &Role{Files: map[string]string{}, Checks: map[string]string{}}
	read := func(p string) (string, error) {
		data, err := fs.ReadFile(fsys, path.Join(name, p))
		if err != nil {
			return "", fmt.Errorf("agent: %w", err)
		}
		r.Files[p] = lean.Hash(string(data))
		return string(data), nil
	}
	spec, err := read("role.yaml")
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(strings.NewReader(spec))
	dec.KnownFields(true)
	if err := dec.Decode(r); err != nil {
		return nil, fmt.Errorf("agent: %s/role.yaml: %w", name, err)
	}
	if r.System, err = read("system.md"); err != nil {
		return nil, err
	}
	if _, err := read("CHANGELOG.md"); err != nil {
		return nil, err
	}
	for _, dir := range []string{"style", "examples", "checks"} {
		entries, err := fs.ReadDir(fsys, path.Join(name, dir))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("agent: %w", err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			text, err := read(path.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			switch dir {
			case "style":
				r.System += "\n\n" + text
			case "examples":
				r.Examples = append(r.Examples, text)
			default:
				r.Checks[strings.TrimSuffix(e.Name(), ".md")] = text
			}
		}
	}
	if err := r.validate(name); err != nil {
		return nil, fmt.Errorf("agent: %s/role.yaml: %w", name, err)
	}
	return r, nil
}

func (r *Role) validate(name string) error {
	switch {
	case r.Name != name:
		return fmt.Errorf("the role is %q in the directory %s", r.Name, name)
	case !semverRe.MatchString(r.Version):
		return fmt.Errorf("the version %q is not semver", r.Version)
	case len(r.Ladder) == 0:
		return errors.New("the ladder has no rung")
	case r.MaxTokens <= 0 || r.Turns <= 0 || r.Stuck <= 0:
		return errors.New("max_tokens, turns and stuck must be more than 0")
	case slices.Contains(r.Tools, StuckTool):
		return fmt.Errorf("%s is in each profile, thus the profile does not name it", StuckTool)
	}
	for _, g := range r.Ladder {
		req := llm.Request{Model: g.Model, Effort: g.Effort, MaxTokens: 1, Messages: []llm.Message{{Role: llm.User}}}
		if err := req.Validate(); err != nil {
			return fmt.Errorf("the rung %+v: %w", g, err)
		}
	}
	return nil
}

// Agent returns the id of the role at its version, for example m2-agent/prove@0.1.0.
func (r *Role) Agent() string { return "m2-agent/" + r.Name + "@" + r.Version }

// Prompt returns the prompt version of the role, for example prove/0.1.0.
func (r *Role) Prompt() string { return r.Name + "/" + r.Version }

// Constraints are the limits that a job sets on its run.
type Constraints struct {
	Models       []string `yaml:"models,omitempty" json:"models,omitempty"`               // the models that the run can use. Empty means each model of the ladder.
	NativeDecide bool     `yaml:"native_decide,omitempty" json:"native_decide,omitempty"` // the job permits a proof that uses native_decide (spec 12.4). Rule F04 records each use.
	TypeChange   bool     `yaml:"type_change,omitempty" json:"type_change,omitempty"`     // the job permits a bump rebind to a declaration with another type hash (spec 17.3.4).
}

// Job is a unit of work with a goal, a target and a budget (spec 11.4).
type Job struct {
	ID          string      `yaml:"job" json:"job"`
	Role        string      `yaml:"role" json:"role"`
	Target      string      `yaml:"target" json:"target"`
	Goal        string      `yaml:"goal" json:"goal"`
	Priority    float64     `yaml:"priority,omitempty" json:"priority,omitempty"`
	Budget      Budget      `yaml:"budget,omitempty" json:"budget,omitzero"`
	OnBehalfOf  string      `yaml:"on_behalf_of" json:"on_behalf_of"`
	RequestedBy string      `yaml:"requested_by,omitempty" json:"requested_by,omitempty"`
	Constraints Constraints `yaml:"constraints,omitempty" json:"constraints,omitzero"`
}

var jobRe = regexp.MustCompile(`^j-[0-9]{4}-[0-9]{2}-[0-9]{2}-[0-9a-z]{1,16}$`)

// Validate reports the first problem of the job: an id that is not in the form j-YYYY-MM-DD-SERIAL, a role that is not known, no target or goal, or no person, project or schedule on whose behalf it runs.
func (j *Job) Validate() error {
	switch {
	case !jobRe.MatchString(j.ID):
		return fmt.Errorf("agent: the job id %q is not in the form j-YYYY-MM-DD-SERIAL", j.ID)
	case !slices.Contains(roles, j.Role):
		return fmt.Errorf("agent: the job has the role %q", j.Role)
	case j.Target == "" || j.Goal == "":
		return errors.New("agent: the job has no target or no goal")
	case !prov.IsResponsible(j.OnBehalfOf):
		return fmt.Errorf("agent: the job runs on behalf of %q, and that is not a person, a project or a schedule", j.OnBehalfOf)
	case j.RequestedBy != "" && !prov.IsPerson(j.RequestedBy):
		return fmt.Errorf("agent: %q requested the job, and that is not a person", j.RequestedBy)
	}
	return nil
}

// budget returns the budget of the job, with the budget of the role for each field that the job does not set.
func (j *Job) budget(r *Role) Budget {
	b := j.Budget
	b.USD = cmp.Or(b.USD, r.Budget.USD)
	b.Wall = cmp.Or(b.Wall, r.Budget.Wall)
	b.LeanSessionS = cmp.Or(b.LeanSessionS, r.Budget.LeanSessionS)
	b.ComputeCPUS = cmp.Or(b.ComputeCPUS, r.Budget.ComputeCPUS)
	return b
}

// Result is the result of one tool call.
type Result struct {
	Text         string  // the text that the model reads. Text from a source must be in a data block marked with its source and licence (spec 12.7).
	IsError      bool    // whether the call failed
	LeanSessionS float64 // the Lean session seconds that the call used
	ComputeCPUS  float64 // the compute CPU seconds that the call used
}

// Tool is a typed tool that the model can call. Call gets the job of the run and the JSON input of the call. An error is a failed call, and the model reads the error.
type Tool interface {
	Spec() llm.Tool
	Call(ctx context.Context, job *Job, input json.RawMessage) (Result, error)
}

// Func is a [Tool] from a function.
type Func struct {
	Def llm.Tool
	Fn  func(ctx context.Context, job *Job, input json.RawMessage) (Result, error)
}

// Spec returns the definition of the tool.
func (f Func) Spec() llm.Tool { return f.Def }

// Call calls the function.
func (f Func) Call(ctx context.Context, job *Job, input json.RawMessage) (Result, error) {
	return f.Fn(ctx, job, input)
}
