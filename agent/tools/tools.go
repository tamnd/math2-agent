// Package tools has the typed tools of the agent roles (spec 12.2).
//
// A role sees only typed tools, never a shell. [API] gives a tool for each read method of the math2 API, with the same names and parameters as the MCP server. [Read] and [Write] read and write the files of the work tree of the run, and Write refuses each change that the field permission set of the role does not allow. [Check] elaborates a Lean file of the work tree, or a scratch file, in the [Lean] environment of the run. [Lake] is that environment on the Lake project of a work tree of math2.
//
// Each result that has text from a node, a source or a file is in a data block marked with its source and its licence (spec 12.7), thus the model can tell the data from the instructions of its role.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/api"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/tag"
	"github.com/tamnd/math2-agent/workspace"
)

// Data returns the text in a data block marked with its source and its licence (spec 12.7). A closing tag in the text is escaped, thus the text cannot end the block.
func Data(source, licence, text string) string {
	text = strings.ReplaceAll(text, "</data>", `<\/data>`)
	return fmt.Sprintf("<data source=%q licence=%q>\n%s\n</data>", source, licence, strings.TrimSuffix(text, "\n"))
}

// Licence returns the SPDX id of the licence of a file of math2 from its path (the REUSE.toml of math2): CC-BY-SA-4.0 for the prose, Apache-2.0 for the code and CC0-1.0 for the metadata.
func Licence(path string) string {
	first, _, _ := strings.Cut(path, "/")
	switch first {
	case "nodes", "topics", "reviews":
		return "CC-BY-SA-4.0"
	case "lean", "compute", ".github":
		return "Apache-2.0"
	}
	return "CC0-1.0"
}

// Schema returns the JSON Schema of the input of a tool: an object with a property for each parameter.
func Schema(params []api.Param) json.RawMessage {
	props := map[string]any{}
	required := []string{}
	for _, p := range params {
		props[p.Name] = map[string]any{"type": p.Type, "description": p.Summary}
		if p.Required {
			required = append(required, p.Name)
		}
	}
	data, err := json.Marshal(map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false})
	if err != nil {
		panic(err)
	}
	return data
}

// decode reads the input of a call into v and refuses a field that v does not have.
func decode(input json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("the input is not valid: %w", err)
	}
	return nil
}

// API returns a tool for each read method of the service with a name in names, in the order of names. A name that is not a read method panics, because the tool profile of a role is fixed in its code and its role.yaml.
func API(s *api.Service, names ...string) []agent.Tool {
	methods := map[string]api.Method{}
	for _, m := range api.Methods() {
		methods[m.Name] = m
	}
	var out []agent.Tool
	for _, name := range names {
		m, ok := methods[name]
		if !ok {
			panic("tools: " + name + " is not a read method of the API")
		}
		out = append(out, agent.Func{
			Def: llm.Tool{Name: m.Name, Description: m.Summary, Schema: Schema(m.Params)},
			Fn: func(_ context.Context, _ *agent.Job, input json.RawMessage) (agent.Result, error) {
				params := map[string]any{}
				dec := json.NewDecoder(bytes.NewReader(input))
				dec.UseNumber()
				if err := dec.Decode(&params); err != nil {
					return agent.Result{}, fmt.Errorf("the input is not a JSON object: %w", err)
				}
				resp, err := s.Call(m.Name, params)
				if err != nil {
					var e *api.Error
					if errors.As(err, &e) {
						return agent.Result{}, errors.New(e.Code + ": " + e.Message)
					}
					return agent.Result{}, err
				}
				text, err := json.Marshal(resp)
				if err != nil {
					return agent.Result{}, err
				}
				return agent.Result{Text: Data("math2 API "+m.Name, "CC-BY-SA-4.0", string(text))}, nil
			},
		})
	}
	return out
}

var pathParam = api.Param{Name: "path", Type: api.String, Required: true, Summary: "the path of the file in the work tree, for example lean/Math2/Algebra/Lagrange.lean"}

// Read returns the tool read_file, which reads a file of the work tree.
func Read(ws *workspace.Workspace) agent.Tool {
	return agent.Func{
		Def: llm.Tool{Name: "read_file", Description: "Read a file of the work tree of the run: a node file in nodes/ or a Lean file in lean/.", Schema: Schema([]api.Param{pathParam})},
		Fn: func(_ context.Context, _ *agent.Job, input json.RawMessage) (agent.Result, error) {
			var in struct{ Path string }
			if err := decode(input, &in); err != nil {
				return agent.Result{}, err
			}
			data, err := ws.Read(in.Path)
			if err != nil {
				return agent.Result{}, err
			}
			return agent.Result{Text: Data(in.Path, Licence(in.Path), string(data))}, nil
		},
	}
}

// Write returns the tool write_file, which writes a file of the work tree. When the changes of the run after the write are outside the field permission set of the role of the job, Write puts the file back as it was and the call fails with the reason.
func Write(ws *workspace.Workspace) agent.Tool {
	return agent.Func{
		Def: llm.Tool{
			Name:        "write_file",
			Description: "Write the full text of a file of the work tree of the run. The write fails when the role may not make the change, and then the file stays as it was.",
			Schema:      Schema([]api.Param{pathParam, {Name: "text", Type: api.String, Required: true, Summary: "the full text of the file"}}),
		},
		Fn: func(ctx context.Context, job *agent.Job, input json.RawMessage) (agent.Result, error) {
			var in struct{ Path, Text string }
			if err := decode(input, &in); err != nil {
				return agent.Result{}, err
			}
			old, err := ws.Read(in.Path)
			existed := err == nil
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return agent.Result{}, err
			}
			if err := ws.Write(in.Path, []byte(in.Text)); err != nil {
				return agent.Result{}, err
			}
			changes, err := ws.Changes(ctx)
			if err == nil {
				err = agent.Permit(job.Role, job.Target, changes)
			}
			if err != nil {
				undo := ws.Delete(in.Path)
				if existed {
					undo = ws.Write(in.Path, old)
				}
				return agent.Result{}, errors.Join(err, undo)
			}
			return agent.Result{Text: fmt.Sprintf("Wrote %s with %d bytes.", in.Path, len(in.Text))}, nil
		},
	}
}

// Lean is the Lean environment of the work tree of a run. The paths are paths in the work tree.
type Lean interface {
	// Check elaborates the Lean file at the path. It builds the modules of the tree that the file imports first.
	Check(ctx context.Context, file string) (*lean.Elaboration, error)
	// Extract builds the Lean library of the tree and returns the extraction record of each declaration, in order. With checks, each record of a statement (see [lean.Record.Statement]) has the result of the degenerate-statement checks of spec 10.4.
	Extract(ctx context.Context, decls []string, checks bool) ([]lean.Record, error)
}

// Scratch is the path of the file of the text that Check elaborates when a call gives text and no path. Git ignores it, thus it is not a change of the run.
const Scratch = "lean/.lake/m2/Scratch.lean"

// Check returns the tool lean_check, which elaborates a Lean file of the work tree, or Lean text in a scratch file. The time of Lean counts in the Lean session seconds of the run.
func Check(ws *workspace.Workspace, env Lean) agent.Tool {
	return agent.Func{
		Def: llm.Tool{
			Name:        "lean_check",
			Description: "Elaborate a Lean file of the work tree, or Lean text in a scratch file, with the pinned toolchain, Mathlib and the Math2 library. The result has each error and warning with its line and column, and says if a declaration uses sorry.",
			Schema: Schema([]api.Param{
				{Name: "path", Type: api.String, Summary: "the path of a Lean file in the work tree"},
				{Name: "text", Type: api.String, Summary: "Lean text to elaborate in a scratch file that is not part of the change, when there is no path"},
			}),
		},
		Fn: func(ctx context.Context, _ *agent.Job, input json.RawMessage) (agent.Result, error) {
			var in struct{ Path, Text string }
			if err := decode(input, &in); err != nil {
				return agent.Result{}, err
			}
			switch {
			case (in.Path == "") == (in.Text == ""):
				return agent.Result{}, errors.New("give a path or a text, not both")
			case in.Text != "":
				if err := ws.Write(Scratch, []byte(in.Text)); err != nil {
					return agent.Result{}, err
				}
				in.Path = Scratch
			case !strings.HasPrefix(in.Path, "lean/") || !strings.HasSuffix(in.Path, ".lean"):
				return agent.Result{}, fmt.Errorf("%s is not a Lean file in lean/", in.Path)
			}
			e, err := env.Check(ctx, in.Path)
			if err != nil {
				return agent.Result{}, err
			}
			return agent.Result{Text: Messages(in.Path, e), IsError: len(e.Errors()) > 0, LeanSessionS: e.Seconds}, nil
		},
	}
}

// Messages returns the messages of the elaboration of the file as text, one line for each message, with a last line that sums them up.
func Messages(file string, e *lean.Elaboration) string {
	var b strings.Builder
	for _, m := range e.Messages {
		fmt.Fprintf(&b, "%s:%d:%d: %s: %s\n", file, m.Line, m.Column, m.Severity, m.Text)
	}
	switch n := len(e.Errors()); {
	case n == 1:
		fmt.Fprintf(&b, "%s does not elaborate: 1 error.", file)
	case n > 1:
		fmt.Fprintf(&b, "%s does not elaborate: %d errors.", file, n)
	case e.Sorry():
		fmt.Fprintf(&b, "%s elaborates, and a declaration uses sorry.", file)
	default:
		fmt.Fprintf(&b, "%s elaborates with no errors and no sorry.", file)
	}
	return b.String()
}

// NodePath returns the path of the file of the node with the id in a tree of math2: nodes/XX/TAG.md for a tag, where XX is the first two characters of the tag, and nodes/~/ID.md for a provisional id. It returns "" for an id that is neither.
func NodePath(id string) string {
	switch {
	case tag.IsTag(id):
		return "nodes/" + id[:2] + "/" + id + ".md"
	case tag.IsProvisional(id):
		return "nodes/~/" + id + ".md"
	}
	return ""
}

// Profile returns the tools of the profile of the role, in the order of the profile: a tool from [API] for each read method of the API, and [Read], [Write] and [Check] for read_file, write_file and lean_check. The extra tools are the tools of one role, for example the record tool of curate, and a name in the profile can name one of them. A name that is none of these is an error.
func Profile(r *agent.Role, s *api.Service, ws *workspace.Workspace, env Lean, extra ...agent.Tool) ([]agent.Tool, error) {
	read := map[string]bool{}
	for _, m := range api.Methods() {
		read[m.Name] = true
	}
	own := map[string]agent.Tool{}
	for _, t := range extra {
		own[t.Spec().Name] = t
	}
	var out []agent.Tool
	for _, name := range r.Tools {
		switch {
		case own[name] != nil:
			out = append(out, own[name])
		case read[name]:
			out = append(out, API(s, name)...)
		case name == "read_file":
			out = append(out, Read(ws))
		case name == "write_file":
			out = append(out, Write(ws))
		case name == "lean_check":
			out = append(out, Check(ws, env))
		default:
			return nil, fmt.Errorf("tools: the profile of %s has the tool %s, and this package does not have it", r.Name, name)
		}
	}
	return out, nil
}
