// Package models reads the model register of corpus K7.7.
//
// The register has an entry for each model that the orchestrator can call: the exact id, the family, the provider and the terms that apply to it. A terms document has its URL, the date that a person read it, and the clauses on outputs, for example a duty to name the model or a ban on training other models with the outputs. The register is data, in models.yaml in this package, and Default gives it.
//
// The orchestrator refuses a model with no entry. Rule X08 of the audit refuses a field from a model with no entry, and rule X09 compares the families of a writer and its judges.
package models

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"
)

// File is the path of the register in tamnd/math2-agent.
const File = "models/models.yaml"

// Version is the version of the register that this package reads.
const Version = "models.v1"

//go:embed models.yaml
var embedded []byte

// The duties of a clause on outputs.
const (
	Owns        = "owns"         // the customer owns the outputs
	Name        = "name"         // a published output, or a model trained with outputs, must name the model
	NoTraining  = "no_training"  // outputs must not train an AI model without the authorization of the provider
	NoCompeting = "no_competing" // outputs must not build a product or a model that competes with the provider
	Other       = "other"        // a clause of another kind; says gives it
)

// Duties lists the duties of a clause.
var Duties = []string{Owns, Name, NoTraining, NoCompeting, Other}

// Register is the model register.
type Register struct {
	Version string  `yaml:"version"`
	Terms   []Terms `yaml:"terms"`
	Models  []Model `yaml:"models"`
	Source  string  `yaml:"-"` // the file that the register came from, or "embedded"
}

// Terms is a licence or a terms document of a provider.
type Terms struct {
	ID        string   `yaml:"id"`
	Name      string   `yaml:"name"`
	URL       string   `yaml:"url"`
	Effective string   `yaml:"effective,omitempty"` // the date that the version that the person read takes effect
	Read      string   `yaml:"read"`                // the date that a person read the document
	By        string   `yaml:"by"`                  // who read it
	Outputs   []Clause `yaml:"outputs,omitempty"`
}

// Clause is a clause of a terms document on outputs.
type Clause struct {
	Duty    string `yaml:"duty"`              // one of Duties
	Section string `yaml:"section,omitempty"` // the section of the document
	From    string `yaml:"from,omitempty"`    // the date that the clause applies from, when the document gives one
	Says    string `yaml:"says"`              // the clause in short words
}

// Model is an entry of the register.
type Model struct {
	ID       string   `yaml:"id"`                // the exact id that the provider gives
	Family   string   `yaml:"family"`            // the family, for example claude
	Provider string   `yaml:"provider"`          // the provider, for example anthropic
	Terms    []string `yaml:"terms"`             // the ids of the terms documents that apply
	Retires  string   `yaml:"retires,omitempty"` // the retirement date, when one is known
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// Parse reads a register and checks it. source names the data in the errors.
func Parse(data []byte, source string) (*Register, error) {
	r := &Register{Source: source}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(r); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	if problems := r.Problems(); len(problems) > 0 {
		return nil, fmt.Errorf("%s: %s", source, strings.Join(problems, "; "))
	}
	return r, nil
}

// Load reads the register at path.
func Load(path string) (*Register, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the caller names the register.
	if err != nil {
		return nil, err
	}
	return Parse(data, path)
}

var (
	defaultOnce sync.Once
	defaultReg  *Register
)

// Default returns the register in this package. It panics when the register does not parse, which a test of the package prevents.
func Default() *Register {
	defaultOnce.Do(func() {
		r, err := Parse(embedded, "embedded")
		if err != nil {
			panic("models: the embedded register does not parse: " + err.Error())
		}
		defaultReg = r
	})
	return defaultReg
}

// Problems returns what is not correct in the register, or nil.
func (r *Register) Problems() []string {
	var out []string
	if r.Version != Version {
		out = append(out, fmt.Sprintf("the version is %q, want %s", r.Version, Version))
	}
	terms := map[string]bool{}
	for i, t := range r.Terms {
		at := fmt.Sprintf("terms %d", i+1)
		if t.ID != "" {
			at = "the terms " + t.ID
		}
		switch {
		case !idPattern.MatchString(t.ID):
			out = append(out, fmt.Sprintf("%s: the id %q is not lower case letters, digits, dots, hyphens and underscores", at, t.ID))
		case terms[t.ID]:
			out = append(out, fmt.Sprintf("%s: two terms documents have this id", at))
		}
		terms[t.ID] = true
		if t.Name == "" {
			out = append(out, at+": the name is empty")
		}
		if !strings.HasPrefix(t.URL, "https://") {
			out = append(out, fmt.Sprintf("%s: the URL %q is not an HTTPS URL", at, t.URL))
		}
		if !isDate(t.Read) {
			out = append(out, fmt.Sprintf("%s: the date of the reading %q is not a date YYYY-MM-DD", at, t.Read))
		}
		if t.Effective != "" && !isDate(t.Effective) {
			out = append(out, fmt.Sprintf("%s: the date %q that the terms take effect is not a date YYYY-MM-DD", at, t.Effective))
		}
		if strings.TrimSpace(t.By) == "" {
			out = append(out, at+": the terms do not say who read them")
		}
		for j, c := range t.Outputs {
			if !slices.Contains(Duties, c.Duty) {
				out = append(out, fmt.Sprintf("%s: clause %d has the duty %q. Give one of %s", at, j+1, c.Duty, strings.Join(Duties, ", ")))
			}
			if strings.TrimSpace(c.Says) == "" {
				out = append(out, fmt.Sprintf("%s: clause %d does not say what the clause is", at, j+1))
			}
			if c.From != "" && !isDate(c.From) {
				out = append(out, fmt.Sprintf("%s: clause %d has the date %q, which is not a date YYYY-MM-DD", at, j+1, c.From))
			}
		}
	}
	seen := map[string]bool{}
	for i, m := range r.Models {
		at := fmt.Sprintf("model %d", i+1)
		if m.ID != "" {
			at = "the model " + m.ID
		}
		switch {
		case !idPattern.MatchString(m.ID):
			out = append(out, fmt.Sprintf("%s: the id %q is not lower case letters, digits, dots, hyphens and underscores", at, m.ID))
		case seen[m.ID]:
			out = append(out, fmt.Sprintf("%s: two entries have this id", at))
		}
		seen[m.ID] = true
		if !idPattern.MatchString(m.Family) {
			out = append(out, fmt.Sprintf("%s: the family %q is not lower case letters, digits, dots, hyphens and underscores", at, m.Family))
		}
		if !idPattern.MatchString(m.Provider) {
			out = append(out, fmt.Sprintf("%s: the provider %q is not lower case letters, digits, dots, hyphens and underscores", at, m.Provider))
		}
		if len(m.Terms) == 0 {
			out = append(out, at+": the entry names no terms")
		}
		for _, t := range m.Terms {
			if !terms[t] {
				out = append(out, fmt.Sprintf("%s: the register has no terms %q", at, t))
			}
		}
		if m.Retires != "" && !isDate(m.Retires) {
			out = append(out, fmt.Sprintf("%s: the retirement date %q is not a date YYYY-MM-DD", at, m.Retires))
		}
	}
	return out
}

func isDate(s string) bool {
	_, err := time.Parse(time.DateOnly, s)
	return err == nil
}

// Lookup returns the entry of the model id, or nil.
func (r *Register) Lookup(id string) *Model {
	i := slices.IndexFunc(r.Models, func(m Model) bool { return m.ID == id })
	if i < 0 {
		return nil
	}
	return &r.Models[i]
}

// Family returns the family of the model id, or "" when the register has no entry for it.
func (r *Register) Family(id string) string {
	if m := r.Lookup(id); m != nil {
		return m.Family
	}
	return ""
}

// Check returns an error when the register has no entry for the model id. The orchestrator calls it before it calls a model.
func (r *Register) Check(id string) error {
	if r.Lookup(id) == nil {
		return fmt.Errorf("models: the model %s has no entry in the model register %s (corpus K7.7)", id, File)
	}
	return nil
}

// TermsOf returns the terms documents that apply to the model id, in the order of its entry.
func (r *Register) TermsOf(id string) []*Terms {
	m := r.Lookup(id)
	if m == nil {
		return nil
	}
	var out []*Terms
	for _, t := range m.Terms {
		if i := slices.IndexFunc(r.Terms, func(x Terms) bool { return x.ID == t }); i >= 0 {
			out = append(out, &r.Terms[i])
		}
	}
	return out
}
