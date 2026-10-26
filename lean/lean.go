// Package lean drives M2Extract, the Lean program that gives the facts about formal declarations that only Lean can give (spec 07.4 and 07.5).
//
// The source of M2Extract is in this directory as a Lake package. [Install] builds it with the toolchain of a Lake project. [Start] runs it in that project, so that it reads the modules of the project and of the dependencies of the project. A [Session] sends requests to the process and reads the answers. The process loads the modules one time and then answers many requests.
//
// [Elaborate] elaborates one Lean file in a Lake project and gives the messages of Lean. The agent roles use it to check the Lean files that they write (spec 12.4).
//
// The protocol is one line of JSON for each request and one line of JSON for each answer. The comment at the top of Main.lean gives the requests.
package lean

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"

	"github.com/zeebo/blake3"

	"github.com/tamnd/math2-agent/node"
)

// Version is the version of the Lean toolchain that M2Extract runs on.
type Version struct {
	Lean    string `json:"lean"`    // for example "4.35.0"
	Githash string `json:"githash"` // the commit of the toolchain
}

// Record is the extraction record of one declaration (spec 07.4).
type Record struct {
	Decl        string   `json:"decl"`
	Kind        string   `json:"kind"`   // theorem, def, axiom, opaque, inductive, constructor, recursor or quot
	Module      string   `json:"module"` // the module that declares Decl
	LevelParams []string `json:"level_params"`

	// Type and Value are the canonical forms of the type and of the value. Value is empty when the declaration has no value.
	Type  string `json:"type"`
	Value string `json:"value"`

	// TypeHash and ValueHash are the hashes of Type and Value. ValueHash is empty when Value is empty.
	TypeHash  string `json:"type_hash"`
	ValueHash string `json:"value_hash"`

	StatementPP    string `json:"statement_pp"`     // the type as Lean prints it
	StatementPPAll string `json:"statement_pp_all"` // the type with pp.all, thus with full elaboration and no notation

	Axioms             []string `json:"axioms"`               // the axioms, as #print axioms gives them, without the axioms of native_decide
	NativeDecideAxioms []string `json:"native_decide_axioms"` // the auxiliary axioms that native_decide added

	UsesDecls                []string `json:"uses_decls"`                  // the constants in the value
	UsesBound                []string `json:"uses_bound"`                  // the bound declarations that the declaration reaches through declarations that are not bound
	UsesConstantsInStatement []string `json:"uses_constants_in_statement"` // the constants in the type

	Deprecated *Deprecation `json:"deprecated,omitempty"` // the entry of the @[deprecated] attribute. It is nil when the declaration is not deprecated.

	// Checks is the result of the degenerate-statement checks. It is nil when no check ran, for example on a declaration that is not a theorem.
	Checks *Checks `json:"checks,omitempty"`

	Error string `json:"error,omitempty"` // the reason when Lean has no record, for example "unknown declaration"
}

// Statement reports whether the declaration is a statement: a theorem, or a definition whose type is Prop. The library states a result that has no proof yet as a definition of a proposition, because rule F06 refuses sorry in it (spec 07.2). The degenerate-statement checks and the F1 battery run on statements.
func (r *Record) Statement() bool {
	return r.Error == "" && (r.Kind == "theorem" || r.Kind == "def" && r.Type == "s 0\n")
}

// Deprecation is the entry of the @[deprecated] attribute of a declaration.
type Deprecation struct {
	NewName string `json:"new_name,omitempty"` // the replacement, for example Nat.add_comm. It is empty when the entry names no replacement.
	Since   string `json:"since,omitempty"`    // the date of the deprecation, as the entry gives it
	Text    string `json:"text,omitempty"`     // the message of the entry
}

// Checks is the result of the degenerate-statement checks 4, 5 and 6 of spec 10.4 on one theorem. Each field is the first tactic that closed the goal, or empty when no tactic closed it.
type Checks struct {
	Vacuous string `json:"vacuous,omitempty"` // the tactic that proves False from the hypotheses
	Trivial string `json:"trivial,omitempty"` // the tactic that proves the statement without the theorem itself
	Refuted string `json:"refuted,omitempty"` // the tactic that proves the negation of the statement
}

// CheckHeartbeats is the limit of each tactic in [Session.Checks], in thousands of heartbeats. It is 5 percent of the default limit of Lean.
const CheckHeartbeats = 10000

// StandardAxioms are the axioms that E5 allows (spec 07.5).
var StandardAxioms = []string{"Classical.choice", "Quot.sound", "propext"}

// Hash returns the hash of a canonical form. It is "blake3:" and 64 lowercase hex characters.
func Hash(canonical string) string {
	sum := blake3.Sum256([]byte(canonical))
	return node.HashPrefix + hex.EncodeToString(sum[:])
}

// A Session is a running M2Extract process. A Session is not safe for concurrent use.
type Session struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    *bufio.Reader
	stderr bytes.Buffer
}

// Start runs the M2Extract program exe with "lake env" in the Lake project directory project. The process stops when ctx is done or when Close is called.
func Start(ctx context.Context, exe, project string) (*Session, error) {
	s := &Session{}
	s.cmd = exec.CommandContext(ctx, "lake", "env", exe) // #nosec G204 -- the caller names the program, and no shell reads the arguments.
	s.cmd.Dir = project
	s.cmd.Stderr = &s.stderr
	in, err := s.cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := s.cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := s.cmd.Start(); err != nil {
		return nil, fmt.Errorf("start m2extract: %w", err)
	}
	s.in, s.out = in, bufio.NewReader(out)
	return s, nil
}

// Close stops the process.
func (s *Session) Close() error {
	_ = s.in.Close()
	err := s.cmd.Wait()
	if err != nil && s.stderr.Len() > 0 {
		return fmt.Errorf("m2extract: %w: %s", err, strings.TrimSpace(s.stderr.String()))
	}
	return err
}

// call sends one request and decodes the answer into resp.
func (s *Session) call(req, resp any) error {
	line, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if _, err := s.in.Write(append(line, '\n')); err != nil {
		return s.died(err)
	}
	answer, err := s.out.ReadBytes('\n')
	if err != nil {
		return s.died(err)
	}
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(answer, &e); err != nil {
		return fmt.Errorf("m2extract: %w", err)
	}
	if e.Error != "" {
		return errors.New("m2extract: " + e.Error)
	}
	return json.Unmarshal(answer, resp)
}

func (s *Session) died(err error) error {
	if msg := strings.TrimSpace(s.stderr.String()); msg != "" {
		return fmt.Errorf("m2extract stopped: %w: %s", err, msg)
	}
	return fmt.Errorf("m2extract stopped: %w", err)
}

// Version returns the version of the Lean toolchain of the process.
func (s *Session) Version() (Version, error) {
	var v Version
	err := s.call(map[string]any{"cmd": "version"}, &v)
	return v, err
}

// Import loads the modules from the search path of the project. It replaces the modules that an earlier Import loaded.
func (s *Session) Import(modules ...string) error {
	var resp struct{}
	return s.call(map[string]any{"cmd": "import", "modules": modules}, &resp)
}

// Extract returns one record for each declaration in decls, in the same order. bound is the list of all bound declarations. It sets the boundary of UsesBound. A declaration that Lean does not know has a record with an Error.
func (s *Session) Extract(decls, bound []string) ([]Record, error) {
	var resp struct {
		Records []Record `json:"records"`
	}
	if err := s.call(map[string]any{"cmd": "extract", "decls": nonNil(decls), "bound": nonNil(bound)}, &resp); err != nil {
		return nil, err
	}
	for i := range resp.Records {
		r := &resp.Records[i]
		if r.Error != "" {
			continue
		}
		r.TypeHash = Hash(r.Type)
		if r.Value != "" {
			r.ValueHash = Hash(r.Value)
		}
	}
	return resp.Records, nil
}

// Checks runs the degenerate-statement checks on each statement in decls (spec 10.4, checks 4 to 6): each theorem, and each definition of a proposition, whose value is the statement. Each tactic stops after heartbeats thousand heartbeats. The map has a result for each statement that Lean knows. A declaration that is not a statement is not in the map.
func (s *Session) Checks(heartbeats int, decls ...string) (map[string]Checks, error) {
	var resp struct {
		Checks []struct {
			Checks
			Decl  string `json:"decl"`
			Error string `json:"error"`
		} `json:"checks"`
	}
	if err := s.call(map[string]any{"cmd": "checks", "decls": nonNil(decls), "heartbeats": heartbeats}, &resp); err != nil {
		return nil, err
	}
	out := map[string]Checks{}
	for _, c := range resp.Checks {
		if c.Error == "" {
			out[c.Decl] = c.Checks
		}
	}
	return out, nil
}

// Implication is the result of [Session.Implies] on one pair of theorems. Each tactic field is the first tactic that proved the goal, or empty when no tactic proved it.
type Implication struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Implies     string `json:"implies,omitempty"`     // the tactic that proves that the statement of From implies the statement of To
	Contradicts string `json:"contradicts,omitempty"` // the tactic that proves that the statement of From implies the negation of the statement of To
	Error       string `json:"error,omitempty"`       // the reason when Lean cannot compare the two, for example "not a statement"
}

// Implies compares the statements of the theorems of each pair, for the checks 7 and 8 of spec 10.4. The first element of a pair is the premise. The proofs must not use the two theorems themselves, and each tactic stops after heartbeats thousand heartbeats. The result has one [Implication] for each pair, in the same order.
func (s *Session) Implies(heartbeats int, pairs ...[2]string) ([]Implication, error) {
	ps := make([][]string, len(pairs))
	for i, p := range pairs {
		ps[i] = []string{p[0], p[1]}
	}
	var resp struct {
		Implies []struct {
			Implication
			Implies     *string `json:"implies"`
			Contradicts *string `json:"contradicts"`
		} `json:"implies"`
	}
	if err := s.call(map[string]any{"cmd": "implies", "pairs": ps, "heartbeats": heartbeats}, &resp); err != nil {
		return nil, err
	}
	out := make([]Implication, len(resp.Implies))
	for i, r := range resp.Implies {
		out[i] = r.Implication
		if r.Implies != nil {
			out[i].Implies = *r.Implies
		}
		if r.Contradicts != nil {
			out[i].Contradicts = *r.Contradicts
		}
	}
	return out, nil
}

// Binding is a declaration with the attribute @[m2 "TAG"] of the Math2 package.
type Binding struct {
	Decl string `json:"decl"`
	Tag  string `json:"tag"`
}

// Bindings returns the declarations with the attribute @[m2 "TAG"] in the loaded modules, in order of module. It is empty when no loaded module defines the attribute.
func (s *Session) Bindings() ([]Binding, error) {
	var resp struct {
		Bindings []Binding `json:"bindings"`
	}
	err := s.call(map[string]any{"cmd": "bindings"}, &resp)
	return resp.Bindings, err
}

// Axioms returns the axioms of each declaration in decls, as "#print axioms" gives them, in order of name. A declaration that Lean does not know is not in the map.
func (s *Session) Axioms(decls ...string) (map[string][]string, error) {
	var resp struct {
		Axioms map[string]*[]string `json:"axioms"`
	}
	if err := s.call(map[string]any{"cmd": "axioms", "decls": nonNil(decls)}, &resp); err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for d, axs := range resp.Axioms {
		if axs != nil {
			out[d] = slices.Clone(*axs)
		}
	}
	return out, nil
}

// Decl is the library record of one declaration: the facts that the record plane keeps about each declaration of a library such as Mathlib (spec 09.2).
type Decl struct {
	Decl     string   `json:"decl"`
	Kind     string   `json:"kind"`      // as in [Record]
	Module   string   `json:"module"`    // the module that declares Decl
	TypeHash string   `json:"type_hash"` // the hash of the canonical form of the type
	Uses     []string `json:"uses"`      // the constants in the value, in order of name
	Doc      string   `json:"doc,omitempty"`
	Error    string   `json:"error,omitempty"` // the reason when Lean has no record
}

// Library returns the library records of the declarations of the loaded modules whose first component is root, for example Mathlib. The declarations are in the order of their names, and Library returns at most limit of them from the position offset. It also returns the number of declarations under root. Internal names, such as auxiliary definitions, are not in the list.
func (s *Session) Library(root string, offset, limit int) ([]Decl, int, error) {
	var resp struct {
		Total   int `json:"total"`
		Records []struct {
			Decl
			Type string  `json:"type"`
			Doc  *string `json:"doc"`
		} `json:"records"`
	}
	if err := s.call(map[string]any{"cmd": "library", "root": root, "offset": offset, "limit": limit}, &resp); err != nil {
		return nil, 0, err
	}
	out := make([]Decl, len(resp.Records))
	for i, r := range resp.Records {
		out[i] = r.Decl
		if r.Doc != nil {
			out[i].Doc = *r.Doc
		}
		if r.Error == "" {
			out[i].TypeHash = Hash(r.Type)
			out[i].Uses = nonNil(r.Uses)
		}
	}
	return out, resp.Total, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
