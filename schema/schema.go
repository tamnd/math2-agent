// Package schema checks node front matter against the node schema, schema/node.v2.json in tamnd/math2 (spec 04.5 and 04.9).
//
// Schema node.v2 adds the kind exercise to node.v1 (corpus K8.9). Each node.v1 node is also a node.v2 node, thus this release accepts both: a tree that has only schema/node.v1.json is still a valid tree, and m2 migrate node.v2 moves it to node.v2.
//
// Load reads the schema from a math2 working tree. Outside a working tree, Embedded gives the copy in this package. Validate gives every error in a node, each with a rule id from spec 15.3, a line and a column.
package schema

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"go.yaml.in/yaml/v3"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/tag"
)

// File is the path of the schema in a math2 working tree.
const File = "schema/node.v2.json"

// Version is the schema version that this toolchain reads and writes.
const Version = "node.v2"

// Previous is the path of the schema of the version before Version. A tree that has not moved to Version has this file and not File.
const Previous = "schema/node.v1.json"

//go:embed node.v2.json
var embedded []byte

// Raw returns the bytes of the copy of the schema in this package, which m2 migrate writes to File.
func Raw() []byte { return slices.Clone(embedded) }

// Schema is a compiled node schema.
type Schema struct {
	Source    string     // the file that the schema came from, or "embedded"
	Order     node.Order // the key order of the canonical form
	Forbidden []string   // the field names that state a status (rule S03)
	Refs      []string   // the field names whose values are references to other nodes, in byte order
	sections  map[string]Sections
	fields    map[string][]string // the fields that the branch of each kind adds
	required  []string            // the fields that each node needs
	needs     map[string][]string // the fields that the branch of each kind needs
	compiled  *jsonschema.Schema
}

// Sections gives the body sections of a kind, from the x-m2-sections keyword.
type Sections struct {
	Required []string `json:"required"`
	Allowed  []string `json:"allowed"`
}

var (
	embeddedOnce   sync.Once
	embeddedSchema *Schema
)

// Embedded returns the copy of the schema in this package.
func Embedded() *Schema {
	embeddedOnce.Do(func() {
		s, err := Parse(embedded, "embedded")
		if err != nil {
			panic("schema: the embedded schema does not compile: " + err.Error())
		}
		embeddedSchema = s
	})
	return embeddedSchema
}

// Path returns the path of the schema file of the working tree at root: File, or Previous when the tree has not moved to Version. It returns "" when the tree has neither file.
func Path(root string) string {
	for _, f := range []string{File, Previous} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(f))); err == nil {
			return f
		}
	}
	return ""
}

// Load reads the schema from the working tree at root, from the file that [Path] gives. When the tree has no schema file, Load returns the embedded copy.
func Load(root string) (*Schema, error) {
	f := Path(root)
	if f == "" {
		return Embedded(), nil
	}
	path := filepath.Join(root, filepath.FromSlash(f))
	data, err := os.ReadFile(path) // #nosec G304 -- the path is in the working tree, which the caller owns.
	if errors.Is(err, fs.ErrNotExist) {
		return Embedded(), nil
	}
	if err != nil {
		return nil, err
	}
	return Parse(data, filepath.ToSlash(path))
}

// Parse compiles a schema. The source names the schema in errors.
func Parse(data []byte, source string) (*Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	root, ok := doc.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: the schema is not an object", source)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(File, doc); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	compiled, err := c.Compile(File)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	order, err := node.ParseOrder(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	s := &Schema{Source: source, Order: order, sections: map[string]Sections{}, fields: map[string][]string{}, required: strs(root["required"]), needs: map[string][]string{}, compiled: compiled}
	if names, ok := dig(root, "propertyNames", "not", "enum").([]any); ok {
		for _, name := range names {
			if n, ok := name.(string); ok {
				s.Forbidden = append(s.Forbidden, n)
			}
		}
	}
	branches, _ := root["allOf"].([]any)
	for _, b := range branches {
		k, _ := dig(b, "if", "properties", "kind", "const").(string)
		sec, _ := dig(b, "then", "x-m2-sections").(map[string]any)
		if k == "" || sec == nil {
			continue
		}
		s.sections[k] = Sections{Required: strs(sec["required"]), Allowed: strs(sec["allowed"])}
		s.needs[k] = strs(dig(b, "then", "required"))
		if props, ok := dig(b, "then", "properties").(map[string]any); ok {
			for f := range props {
				s.fields[k] = append(s.fields[k], f)
			}
		}
	}
	if len(s.sections) == 0 {
		return nil, fmt.Errorf("%s: the schema gives no sections for any kind", source)
	}
	s.Refs = refs(root)
	return s, nil
}

// refDef is the definition of a node reference in the schema.
const refDef = "#/$defs/ref"

// refs returns the names of the properties whose schema is a node reference or a list of node references. It looks in each properties keyword of the schema, also in $defs and in the branches of the kinds. A name is in the list once, also when more than one kind has it. The tag field is not in the list, because it names the node itself and not another node.
func refs(root map[string]any) []string {
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch v := v.(type) {
		case []any:
			for _, x := range v {
				walk(x)
			}
		case map[string]any:
			for k, x := range v {
				props, ok := x.(map[string]any)
				if k != "properties" || !ok {
					walk(x)
					continue
				}
				for name, p := range props {
					if dig(p, "$ref") == refDef || dig(p, "items", "$ref") == refDef {
						out = append(out, name)
					}
					walk(p)
				}
			}
		}
	}
	walk(root)
	slices.Sort(out)
	return slices.DeleteFunc(slices.Compact(out), func(name string) bool { return name == "tag" })
}

// dig returns the value at a path of object keys, or nil.
func dig(v any, keys ...string) any {
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

func strs(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// Kinds returns the kinds that the schema gives sections for, in byte order.
func (s *Schema) Kinds() []string {
	out := make([]string, 0, len(s.sections))
	for k := range s.sections {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// Sections returns the body sections of a kind. It returns false for a kind that the schema does not know.
func (s *Schema) Sections(kind string) (Sections, bool) {
	sec, ok := s.sections[kind]
	return sec, ok
}

// Required returns the front matter fields that a node of the kind needs, in the key order of the canonical form. It returns false for a kind that the schema does not know.
func (s *Schema) Required(kind string) ([]string, bool) {
	if _, ok := s.sections[kind]; !ok {
		return nil, false
	}
	out := slices.Concat(s.required, s.needs[kind])
	known := s.Order[""]
	rank := func(f string) int {
		if i := slices.Index(known, f); i >= 0 {
			return i
		}
		return len(known)
	}
	slices.SortStableFunc(out, func(a, b string) int { return rank(a) - rank(b) })
	return slices.Compact(out), true
}

// Template returns a new node file of the kind, with id as its tag. The file is in canonical form. Each field that the kind needs, other than tag and kind, has the placeholder null. The body has the headings of the required sections. A kind with no required sections gets the heading of its first allowed section.
//
// m2 check reports each placeholder as S01, so a new file fails until each placeholder has a value. Template returns an error for a kind that the schema does not know.
func (s *Schema) Template(kind, id string) ([]byte, error) {
	fields, ok := s.Required(kind)
	if !ok {
		return nil, fmt.Errorf("schema: %q is not a kind. The kinds are %s", kind, strings.Join(s.Kinds(), ", "))
	}
	var b strings.Builder
	b.WriteString("---\n")
	for _, f := range fields {
		switch f {
		case "tag":
			fmt.Fprintf(&b, "tag: %s\n", id)
		case "kind":
			fmt.Fprintf(&b, "kind: %s\n", kind)
		default:
			fmt.Fprintf(&b, "%s: null\n", f)
		}
	}
	b.WriteString("---\n")
	sec := s.sections[kind]
	heads := sec.Required
	if len(heads) == 0 && len(sec.Allowed) > 0 {
		heads = sec.Allowed[:1]
	}
	for _, h := range heads {
		fmt.Fprintf(&b, "\n## %s\n", h)
	}
	n, err := node.Parse("nodes/"+tag.Shard(id)+"/"+id+".md", []byte(b.String()))
	if err != nil {
		return nil, err
	}
	return node.Format(n, s.Order), nil
}

// Finding is one schema error in a node.
type Finding struct {
	Rule  string // S01, S02 or S03
	Path  string // the node file
	Line  int
	Col   int
	Field string // the place in the front matter, for example formal.0.match
	Msg   string
}

// String returns the finding in the form path:line:col: RULE: message.
func (f Finding) String() string {
	return fmt.Sprintf("%s:%d:%d: %s: %s", f.Path, f.Line, f.Col, f.Rule, f.Msg)
}

// Validate checks the front matter of n. It returns every finding, in line order.
//
// The rules come from spec 15.3. S03 is a field that states a status. S02 is a missing field that the kind of the node needs. S01 is every other schema error.
func (s *Schema) Validate(n *node.Node) []Finding {
	front := n.Front
	if front.Kind == yaml.DocumentNode && len(front.Content) > 0 {
		front = front.Content[0]
	}
	v := &walker{s: s, n: n, front: front, kind: n.Kind()}
	err := s.compiled.Validate(value(front))
	var ve *jsonschema.ValidationError
	if errors.As(err, &ve) {
		v.collect(ve)
	}
	slices.SortFunc(v.out, func(a, b Finding) int {
		if a.Line != b.Line {
			return a.Line - b.Line
		}
		if a.Col != b.Col {
			return a.Col - b.Col
		}
		return strings.Compare(a.Rule+a.Msg, b.Rule+b.Msg)
	})
	return slices.Compact(v.out)
}

var printer = message.NewPrinter(language.English)

type walker struct {
	s     *Schema
	n     *node.Node
	front *yaml.Node
	kind  string
	out   []Finding
}

// collect adds a finding for each leaf error below e.
func (w *walker) collect(e *jsonschema.ValidationError) {
	loc := e.InstanceLocation
	switch k := e.ErrorKind.(type) {
	case *kind.PropertyNames:
		w.add("S03", append(slices.Clone(loc), k.Property), fmt.Sprintf("the field %q states a status, but the build computes the status of each node", k.Property))
		return
	case *kind.Required:
		rule := "S01"
		if strings.Contains(e.SchemaURL, "/allOf/") {
			rule = "S02"
		}
		for _, m := range k.Missing {
			msg := fmt.Sprintf("the field %q is missing", m)
			if rule == "S02" {
				msg = fmt.Sprintf("the field %q is missing, and a node of kind %s needs it", m, w.kind)
			}
			w.add(rule, loc, msg)
		}
		return
	case *kind.FalseSchema:
		field := loc[len(loc)-1]
		msg := fmt.Sprintf("the field %q is not allowed here", field)
		if len(loc) == 1 {
			// A status field is S03 only. A field of this kind is unevaluated only because another error is in the branch of the kind, and that error is enough.
			if slices.Contains(w.s.Forbidden, field) || slices.Contains(w.s.fields[w.kind], field) {
				return
			}
			msg = fmt.Sprintf("the field %q is not allowed in a node of kind %s", field, w.kind)
		}
		w.add("S01", loc, msg)
		return
	case *kind.AdditionalProperties:
		for _, p := range k.Properties {
			w.add("S01", append(slices.Clone(loc), p), fmt.Sprintf("the field %q is not allowed here", p))
		}
		return
	}
	if y := at(w.front, loc); len(loc) > 0 && y != nil && y.Kind == yaml.ScalarNode && y.Tag == "!!null" {
		w.add("S01", loc, fmt.Sprintf("the field %q has no value", strings.Join(loc, ".")))
		return
	}
	switch e.ErrorKind.(type) {
	case *kind.AnyOf, *kind.OneOf:
		switch {
		case strings.HasSuffix(e.SchemaURL, "/$defs/ref"):
			w.add("S01", loc, "the value is not a tag or a provisional id")
			return
		case strings.HasSuffix(e.SchemaURL, "/$defs/source"):
			w.add("S01", loc, "a source must have exactly one identifier, for example doi or arxiv")
			return
		}
	}
	if len(e.Causes) == 0 {
		w.add("S01", loc, e.ErrorKind.LocalizedString(printer))
		return
	}
	for _, c := range e.Causes {
		w.collect(c)
	}
}

// add adds a finding at the place of loc in the front matter.
func (w *walker) add(rule string, loc []string, msg string) {
	line, col := locate(w.front, loc)
	field := strings.Join(loc, ".")
	if field != "" && !strings.HasPrefix(msg, "the field") {
		msg = field + ": " + msg
	}
	w.out = append(w.out, Finding{Rule: rule, Path: w.n.Path, Line: line, Col: col, Field: field, Msg: msg})
}

// locate returns the line and the column of the place loc in the front matter. For a mapping key, it gives the place of the key. For a place that does not exist, it gives the place of the nearest parent.
func locate(n *yaml.Node, loc []string) (line, col int) {
	line, col = n.Line, n.Column
	for _, tok := range loc {
		var next *yaml.Node
		switch n.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				if n.Content[i].Value == tok {
					line, col = n.Content[i].Line, n.Content[i].Column
					next = n.Content[i+1]
					break
				}
			}
		case yaml.SequenceNode:
			if i, err := strconv.Atoi(tok); err == nil && i >= 0 && i < len(n.Content) {
				next = n.Content[i]
				line, col = next.Line, next.Column
			}
		}
		if next == nil {
			return line, col
		}
		n = next
	}
	return line, col
}

// at returns the YAML node at the place loc in the front matter, or nil when the place does not exist.
func at(n *yaml.Node, loc []string) *yaml.Node {
	for _, tok := range loc {
		var next *yaml.Node
		switch n.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				if n.Content[i].Value == tok {
					next = n.Content[i+1]
					break
				}
			}
		case yaml.SequenceNode:
			if i, err := strconv.Atoi(tok); err == nil && i >= 0 && i < len(n.Content) {
				next = n.Content[i]
			}
		}
		if next == nil {
			return nil
		}
		n = next
	}
	return n
}

// value changes a YAML node to the value that the validator reads. A date stays a string, as in the Python check of tamnd/math2. A number that JSON cannot hold, such as .inf, stays a string.
func value(n *yaml.Node) any {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil
		}
		return value(n.Content[0])
	case yaml.MappingNode:
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			m[n.Content[i].Value] = value(n.Content[i+1])
		}
		return m
	case yaml.SequenceNode:
		list := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			list = append(list, value(c))
		}
		return list
	case yaml.AliasNode:
		return value(n.Alias)
	}
	switch n.Tag {
	case "!!null":
		return nil
	case "!!bool":
		var b bool
		if n.Decode(&b) == nil {
			return b
		}
	case "!!int":
		var i int64
		if n.Decode(&i) == nil {
			return i
		}
		var f float64
		if n.Decode(&f) == nil {
			return f
		}
	case "!!float":
		var f float64
		if n.Decode(&f) == nil && !math.IsInf(f, 0) && !math.IsNaN(f) {
			return f
		}
	}
	return n.Value
}
