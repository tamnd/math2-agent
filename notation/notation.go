// Package notation reads the notation register of a math2 working tree, and writes the macro files from it (corpus K10.4).
//
// A symbol is an identity, and a notation is a way to print it. The register says what each macro means: the node that it denotes, how to print it and where a body can use it. The register is the set of files notation/<topic>.yaml, one for each topic, and notation/global.yaml for the symbols of all areas. Each file is a YAML list of entries, and each entry is one macro. The macro files macros/m2.sty and macros/m2.katex.json are the output of the register.
//
// The file notation/conventions.yaml is the register of conventions (corpus K10.5). It is in the same directory, and it is not a file of macros.
package notation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	// Dir is the directory of the register in a math2 working tree.
	Dir = "notation"
	// Global is the name of the register file and of the scope of the symbols of all areas.
	Global = "global"
	// ConventionsFile is the register of conventions. It has no macros.
	ConventionsFile = "notation/conventions.yaml"
	// StyFile is the LaTeX macro file that the register gives.
	StyFile = "macros/m2.sty"
	// KaTeXFile is the KaTeX macro table that the register gives.
	KaTeXFile = "macros/m2.katex.json"
)

// Variant is another way to print the symbol of an entry. A volume or a reader can choose it (corpus K11.1).
type Variant struct {
	Name    string   `yaml:"name"`
	LaTeX   string   `yaml:"latex"`
	Sources []string `yaml:"sources,omitempty"`
}

// Entry is one macro of the register.
type Entry struct {
	Macro        string    `yaml:"macro"`                   // the only form that a node body uses, for example \totient
	Denotes      string    `yaml:"denotes"`                 // the tag of the concept or definition that the macro means
	Args         int       `yaml:"args"`                    // the number of arguments
	Prec         *int      `yaml:"prec,omitempty"`          // for an operator, the binding strength
	LaTeX        string    `yaml:"latex"`                   // the expansion for m2.sty and KaTeX
	Typst        string    `yaml:"typst,omitempty"`         // the expansion for the Typst output
	Intent       string    `yaml:"intent,omitempty"`        // a MathML intent concept name
	OpenMath     string    `yaml:"openmath,omitempty"`      // an OpenMath symbol, for alignment
	Reads        string    `yaml:"reads"`                   // how a person reads it, for screen readers
	Lean         string    `yaml:"lean,omitempty"`          // the Lean declaration or notation
	Scope        []string  `yaml:"scope,omitempty"`         // the topics where a body can use the macro
	IntroducedBy string    `yaml:"introduced_by,omitempty"` // the definition node that introduces the symbol
	Convention   string    `yaml:"convention,omitempty"`    // one line for the list of notation
	Variants     []Variant `yaml:"variants,omitempty"`
	Conflicts    []string  `yaml:"conflicts,omitempty"` // the ids of entries in other scopes that print the same glyph

	File string `yaml:"-"` // the register file, as a slash path in the working tree
	Line int    `yaml:"-"` // the line of the entry in File
}

// Name returns the macro with no backslash.
func (e *Entry) Name() string { return strings.TrimPrefix(e.Macro, `\`) }

// ID returns the id of the entry: the name of its register file with no extension, a dot and the name of the macro, for example global.ip. The conflicts field uses these ids.
func (e *Entry) ID() string {
	return strings.TrimSuffix(path.Base(e.File), ".yaml") + "." + e.Name()
}

// IsGlobal reports whether a body of each topic can use the macro. The entries of notation/global.yaml and the entries with the scope global are global.
func (e *Entry) IsGlobal() bool {
	return path.Base(e.File) == Global+".yaml" || slices.Contains(e.Scope, Global)
}

// Scopes returns the topics where a body can use the macro. An entry of a topic file with no scope field has the topic of its file.
func (e *Entry) Scopes() []string {
	if e.IsGlobal() {
		return []string{Global}
	}
	if len(e.Scope) == 0 {
		return []string{strings.TrimSuffix(path.Base(e.File), ".yaml")}
	}
	return e.Scope
}

// Register is the notation register of a working tree.
type Register struct {
	Files   []string // the register files, as slash paths: global.yaml first, then the topic files in name order
	Entries []*Entry // in file order
}

// Lookup returns the entries of the macro, which has a backslash.
func (r *Register) Lookup(macro string) []*Entry {
	var out []*Entry
	for _, e := range r.Entries {
		if e.Macro == macro {
			out = append(out, e)
		}
	}
	return out
}

var (
	macroRE = regexp.MustCompile(`^\\[A-Za-z]+$`)
	paramRE = regexp.MustCompile(`#([1-9])`)
)

// Load reads the register of the working tree at root. It returns an error that wraps fs.ErrNotExist when the tree has no register files. An entry with a field of the wrong type, a field that the register does not have, a macro that is not a backslash and letters, or an expansion whose highest #n is not its number of arguments is an error. Two entries for one macro are not an error here: rule N06 reports them.
func Load(root string) (*Register, error) {
	paths, err := filepath.Glob(filepath.Join(root, Dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	var files []string
	for _, p := range paths {
		rel := Dir + "/" + filepath.Base(p)
		if rel != ConventionsFile {
			files = append(files, rel)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("notation: %s has no register files: %w", Dir, fs.ErrNotExist)
	}
	slices.SortFunc(files, func(a, b string) int {
		ga, gb := path.Base(a) == Global+".yaml", path.Base(b) == Global+".yaml"
		if ga != gb {
			if ga {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})
	r := &Register{Files: files}
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f))) // #nosec G304 -- the path is in the working tree, which the caller owns.
		if err != nil {
			return nil, err
		}
		entries, err := Parse(f, data)
		if err != nil {
			return nil, err
		}
		r.Entries = append(r.Entries, entries...)
	}
	return r, nil
}

// Parse parses one register file. The name is the slash path of the file in the working tree, and it goes into the entries and the errors.
func Parse(name string, data []byte) ([]*Entry, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	list := doc.Content[0]
	if list.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s:%d: a register file is a list of entries", name, list.Line)
	}
	var out []*Entry
	for _, item := range list.Content {
		e := &Entry{File: name, Line: item.Line}
		if err := decodeStrict(item, e); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", name, item.Line, err)
		}
		if err := e.valid(); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", name, item.Line, err)
		}
		out = append(out, e)
	}
	return out, nil
}

// decodeStrict decodes the node into v, and it returns an error for a field that v does not have.
func decodeStrict(n *yaml.Node, v any) error {
	data, err := yaml.Marshal(n)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	return dec.Decode(v)
}

func (e *Entry) valid() error {
	if !macroRE.MatchString(e.Macro) {
		return fmt.Errorf("the macro %q is not a backslash and letters", e.Macro)
	}
	if e.LaTeX == "" {
		return fmt.Errorf("the macro %s has no latex expansion", e.Macro)
	}
	if n := highestParam(e.LaTeX); n != e.Args {
		return fmt.Errorf("the macro %s has args %d, and its expansion uses %d", e.Macro, e.Args, n)
	}
	return nil
}

// highestParam returns the highest #n in the expansion. KaTeX takes the number of arguments of a macro from it.
func highestParam(s string) int {
	n := 0
	for _, m := range paramRE.FindAllStringSubmatch(s, -1) {
		n = max(n, int(m[1][0]-'0'))
	}
	return n
}

// styHead is the start of macros/m2.sty.
const styHead = `% The LaTeX macros that a node body can use.
% m2 notation build writes this file from the notation register in notation/. Do not change it by hand.
% Write one \newcommand on each line. test_macros.py reads this file line by line.
\NeedsTeXFormat{LaTeX2e}
\ProvidesPackage{m2}[2026/10/26 math2 node macros v2]
\RequirePackage{amsmath}
\RequirePackage{amssymb}
`

// macros returns the first entry of each macro, in register order. The output files have one expansion for each macro.
func (r *Register) macros() []*Entry {
	seen := map[string]bool{}
	var out []*Entry
	for _, e := range r.Entries {
		if !seen[e.Macro] {
			seen[e.Macro] = true
			out = append(out, e)
		}
	}
	return out
}

// Sty returns the text of macros/m2.sty: one \newcommand for each macro, under a comment line with the name of its register file.
func (r *Register) Sty() []byte {
	var b bytes.Buffer
	b.WriteString(styHead)
	file := ""
	for _, e := range r.macros() {
		if e.File != file {
			file = e.File
			fmt.Fprintf(&b, "%% %s\n", file)
		}
		b.WriteString(`\newcommand{` + e.Macro + `}`)
		if e.Args > 0 {
			b.WriteString("[" + strconv.Itoa(e.Args) + "]")
		}
		b.WriteString("{" + e.LaTeX + "}\n")
	}
	return b.Bytes()
}

// KaTeX returns the text of macros/m2.katex.json: a JSON object from each macro to its expansion, in register order.
func (r *Register) KaTeX() []byte {
	var b bytes.Buffer
	b.WriteString("{\n")
	ms := r.macros()
	for i, e := range ms {
		fmt.Fprintf(&b, "  %s: %s", jsonString(e.Macro), jsonString(e.LaTeX))
		if i < len(ms)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	return b.Bytes()
}

func jsonString(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // a string always encodes
	return strings.TrimSuffix(b.String(), "\n")
}

// Outputs returns the macro files that the register gives, keyed by their slash paths in the working tree.
func (r *Register) Outputs() map[string][]byte {
	return map[string][]byte{StyFile: r.Sty(), KaTeXFile: r.KaTeX()}
}

var newcommandRE = regexp.MustCompile(`^\\newcommand\{(\\[A-Za-z]+)\}(?:\[([1-9])\])?\{(.*)\}$`)

// Seed reads a macro file in the form of macros/m2.sty, and returns an entry for each \newcommand line in file order. The entries say how to print each macro, but not what it means: Denotes, Reads and the other fields are empty, and a curator fills them in. Seed returns an error for a \newcommand that is not on one line in the form \newcommand{\name}[n]{body}.
func Seed(sty []byte) ([]*Entry, error) {
	var out []*Entry
	for i, line := range strings.Split(string(sty), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, `\newcommand`) {
			continue
		}
		m := newcommandRE.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("line %d: write each \\newcommand on one line in the form \\newcommand{\\name}[n]{body}", i+1)
		}
		e := &Entry{Macro: m[1], LaTeX: m[3]}
		if m[2] != "" {
			e.Args = int(m[2][0] - '0')
		}
		if err := e.valid(); err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		out = append(out, e)
	}
	if len(out) == 0 {
		return nil, errors.New("the file has no \\newcommand")
	}
	return out, nil
}

// Marshal returns the YAML text of a register file with the entries.
func Marshal(entries []*Entry) ([]byte, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(entries); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Glyph returns what an expansion prints with no arguments: the expansion with no #n, no \left and \right, no spaces and no brackets or commas. For example \varphi(#1) and \varphi give \varphi. An expansion that is only brackets, such as (#1, #2), gives itself with no spaces.
func Glyph(latex string) string {
	s := paramRE.ReplaceAllString(latex, "")
	s = strings.NewReplacer(`\left`, "", `\right`, "", " ", "").Replace(s)
	if g := strings.Map(func(r rune) rune {
		if strings.ContainsRune("()[],", r) {
			return -1
		}
		return r
	}, s); g != "" {
		return g
	}
	return s
}
