// Package prov reads and writes the provenance of math2 (spec 04.8 and 06.5).
//
// Each node has a provenance log in prov/<shard>/<tag>.jsonl. The log has one [Line] for each change to each field of the node. The line says who changed the field, when, with what and from which source: a person, or an agent at a version with its model and its run. [Changes] makes the lines for a change to a node file, and [Append] adds them to the log.
//
// The full record of an agent run is in the artefact store, keyed by run id (spec 11.6). A [Run] is the summary of that record: the agent, the prompt version, the models and the person, project or schedule on whose behalf the agent acted. [Credit] lists the work of a person, and [Produced] lists the fields that a model and a prompt version made and that no person has replaced or confirmed since. A person quarantines such a field with a line of the operation Quarantine, and clears it with a later change or a line of the operation Confirm.
package prov

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/tag"
)

// Dir is the directory of the provenance logs in a math2 working tree.
const Dir = "prov"

// The operations of a line.
const (
	Add        = "add"        // the field is new
	Change     = "change"     // the field has a new value
	Remove     = "remove"     // the field is gone
	Confirm    = "confirm"    // a person read the value of an agent and approved it
	Quarantine = "quarantine" // a person hid the value of an agent until a person clears it
)

// Whole is the field of a line that is about the whole node: a node file that is new or that is gone.
const Whole = "*"

// Line is one line of a provenance log. A line has an agent, a person or both. A line of a person with no agent is a change that the person made. A line with both is a change of an agent that a person made through a tool that records the agent, for example a review of a proposal.
type Line struct {
	Field  string // the field, for example formal[0], same_as.wikidata or body.explanations.L2
	Op     string // Add, Change, Remove, Confirm or Quarantine
	Agent  string // the agent role at its version, for example m2-agent/formalize@0.9.1, or ""
	Model  string // the model of the agent, or ""
	Prompt string // the prompt version of the agent, or ""
	Run    string // the id of the agent run, or ""
	Human  string // the person, as orcid:ID or github:LOGIN, or ""
	Source string // the external source of the value, for example arxiv:2401.01234, or ""
	Commit string // the commit of the change, or "" before the change is committed
	At     string // the time of the change, RFC 3339 in UTC
}

// wire is the JSON form of a line. The agent and the person are always there, and null when they are empty.
type wire struct {
	Field  string  `json:"field"`
	Op     string  `json:"op"`
	Agent  *string `json:"agent"`
	Model  string  `json:"model,omitempty"`
	Prompt string  `json:"prompt,omitempty"`
	Run    string  `json:"run,omitempty"`
	Human  *string `json:"human"`
	Source string  `json:"source,omitempty"`
	Commit string  `json:"commit,omitempty"`
	At     string  `json:"at"`
}

func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func val(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// MarshalJSON writes the line in the form of spec 04.8.
func (l Line) MarshalJSON() ([]byte, error) {
	return json.Marshal(wire{l.Field, l.Op, ptr(l.Agent), l.Model, l.Prompt, l.Run, ptr(l.Human), l.Source, l.Commit, l.At})
}

// UnmarshalJSON reads a line in the form of spec 04.8. A model that is null is the empty string.
func (l *Line) UnmarshalJSON(data []byte) error {
	var w struct {
		wire
		Model *string `json:"model"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*l = Line{w.Field, w.Op, val(w.Agent), val(w.Model), w.Prompt, w.Run, val(w.Human), w.Source, w.Commit, w.At}
	return nil
}

var (
	orcidRe    = regexp.MustCompile(`^orcid:[0-9]{4}-[0-9]{4}-[0-9]{4}-[0-9]{3}[0-9X]$`)
	loginRe    = regexp.MustCompile(`^github:[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)
	scheduleRe = regexp.MustCompile(`^(?:schedule|project):[a-z0-9][a-z0-9-]{0,62}$`)
	agentRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*/[a-z0-9][a-z0-9-]*@[0-9][0-9A-Za-z.+-]*$`)
	runRe      = regexp.MustCompile(`^r-[0-9]{4}-[0-9]{2}-[0-9]{2}-[0-9a-z]{1,16}$`)
)

// IsPerson reports whether id names a person: orcid: and an ORCID iD, or github: and a GitHub login.
func IsPerson(id string) bool { return orcidRe.MatchString(id) || loginRe.MatchString(id) }

// IsResponsible reports whether id names someone who can start an agent run: a person, or schedule: or project: and a name such as schedule:nightly (spec 16.5). An agent is never responsible.
func IsResponsible(id string) bool { return IsPerson(id) || scheduleRe.MatchString(id) }

// IsAgent reports whether id names an agent role at a version, for example m2-agent/formalize@0.9.1.
func IsAgent(id string) bool { return agentRe.MatchString(id) }

// IsRun reports whether id is a run id such as r-2026-11-02-8812: r-, the date the run started and a short serial.
func IsRun(id string) bool { return runRe.MatchString(id) }

// Validate reports the first problem of the line: a field or an operation that is missing or unknown, no agent and no person, an agent with no run, an id that is not in its form, or a time that is not RFC 3339.
func (l Line) Validate() error {
	switch {
	case l.Field == "":
		return errors.New("the line has no field")
	case !slices.Contains([]string{Add, Change, Remove, Confirm, Quarantine}, l.Op):
		return fmt.Errorf("the operation %q is not add, change, remove, confirm or quarantine", l.Op)
	case l.Agent == "" && l.Human == "":
		return errors.New("the line has no agent and no person")
	case (l.Op == Confirm || l.Op == Quarantine) && l.Human == "":
		return fmt.Errorf("only a person can %s a field", l.Op)
	case l.Agent != "" && !IsAgent(l.Agent):
		return fmt.Errorf("the agent %q is not a role at a version such as m2-agent/formalize@0.9.1", l.Agent)
	case l.Agent != "" && l.Run == "":
		return fmt.Errorf("the line of %s names no run", l.Agent)
	case l.Run != "" && !IsRun(l.Run):
		return fmt.Errorf("the run id %q is not in the form r-YYYY-MM-DD-SERIAL", l.Run)
	case l.Human != "" && !IsPerson(l.Human):
		return fmt.Errorf("%q is not a person. Give orcid: and an ORCID iD, or github: and a GitHub login", l.Human)
	}
	if _, err := time.Parse(time.RFC3339, l.At); err != nil {
		return fmt.Errorf("the time %q is not RFC 3339", l.At)
	}
	return nil
}

// Path returns the path of the provenance log of the tag from the tree root, for example prov/7K/7K3QX2.jsonl.
func Path(t string) string { return Dir + "/" + tag.Shard(t) + "/" + t + ".jsonl" }

// Parse reads a provenance log. An empty line is skipped. A line that is not JSON gives an error with its line number.
func Parse(r io.Reader) ([]Line, error) {
	var out []Line
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 1<<20)
	for i := 1; sc.Scan(); i++ {
		b := bytes.TrimSpace(sc.Bytes())
		if len(b) == 0 {
			continue
		}
		var l Line
		if err := json.Unmarshal(b, &l); err != nil {
			return out, fmt.Errorf("line %d: %w", i, err)
		}
		out = append(out, l)
	}
	return out, sc.Err()
}

// Read reads the provenance log of the tag in the tree at root. A tag with no log has no lines and no error.
func Read(root, t string) ([]Line, error) {
	p := Path(t)
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p))) // #nosec G304 -- the path is a provenance log of the working tree that the caller names.
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lines, err := Parse(bytes.NewReader(data))
	if err != nil {
		return lines, fmt.Errorf("prov: %s: %w", p, err)
	}
	return lines, nil
}

// ReadAll reads each provenance log in the tree at root, by tag. A tree with no prov/ directory has no logs.
func ReadAll(root string) (map[string][]Line, error) {
	files, err := filepath.Glob(filepath.Join(root, Dir, "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	out := map[string][]Line{}
	for _, f := range files {
		t := strings.TrimSuffix(filepath.Base(f), ".jsonl")
		if Path(t) != Dir+"/"+filepath.Base(filepath.Dir(f))+"/"+filepath.Base(f) {
			continue // a file in the wrong shard is not the log of a tag
		}
		lines, err := Read(root, t)
		if err != nil {
			return nil, err
		}
		out[t] = lines
	}
	return out, nil
}

// Append checks the lines and adds them to the end of the provenance log of the tag in the tree at root. It makes the log and its shard directory when they do not exist. A log is never rewritten: a correction is a new line.
func Append(root, t string, lines ...Line) error {
	if len(lines) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for _, l := range lines {
		if err := l.Validate(); err != nil {
			return fmt.Errorf("prov: %s %s: %w", t, l.Field, err)
		}
		b, err := json.Marshal(l)
		if err != nil {
			return err
		}
		buf.Write(append(b, '\n'))
	}
	path := filepath.Join(root, filepath.FromSlash(Path(t)))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- the log is part of the working tree, which others read.
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644) // #nosec G302 G304 -- the log is part of the working tree, which others read.
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// Covers reports whether a line on the field a is about the field b: a and b are the same field, one is a part of the other, or one of them is [Whole]. For example a line on same_as.wikidata covers a change to same_as, and a line on formal covers formal[0].
func Covers(a, b string) bool {
	return a == Whole || b == Whole || within(a, b) || within(b, a)
}

// within reports whether the field a is b or a part of b.
func within(a, b string) bool {
	return a == b || strings.HasPrefix(a, b) && (a[len(b)] == '.' || a[len(b)] == '[')
}

// under reports whether a change to the field b changes the field a: a is b or a part of b, or b is the whole node.
func under(a, b string) bool { return b == Whole || within(a, b) }

// Unconfirmed returns the fields of the log whose value is from an agent, and that no person changed or confirmed after it, with the line of the agent. A line of a person who quarantined the field does not confirm it.
func Unconfirmed(lines []Line) map[string]Line {
	out := map[string]Line{}
	for _, l := range lines {
		switch {
		case l.Op == Quarantine:
		case l.Agent != "" && l.Human == "" && l.Op != Remove:
			out[l.Field] = l
		default:
			for f := range out {
				if under(f, l.Field) {
					delete(out, f)
				}
			}
		}
	}
	return out
}

// Quarantined returns the fields of the log that a person quarantined and that no person cleared after it, with the line of the quarantine. A person clears a field when they change it, remove it or confirm it.
func Quarantined(lines []Line) map[string]Line {
	out := map[string]Line{}
	for _, l := range lines {
		switch {
		case l.Op == Quarantine:
			out[l.Field] = l
		case l.Human != "":
			for f := range out {
				if under(f, l.Field) {
					delete(out, f)
				}
			}
		}
	}
	return out
}

// Author returns the last line of the log that changed or confirmed the field: a line on the field, on a field that contains it, on a part of it or on the whole node. A line of the operation Quarantine changes nothing, and Author skips it. It returns false when no line is about the field.
func Author(lines []Line, field string) (Line, bool) {
	for _, l := range slices.Backward(lines) {
		if l.Op != Quarantine && Covers(l.Field, field) {
			return l, true
		}
	}
	return Line{}, false
}
