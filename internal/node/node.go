// Package node reads node files, writes them in canonical form and computes their content hash.
//
// A node file is nodes/<shard>/<id>.md. It starts with YAML front matter between two lines of ---, and a Markdown body follows. The id is a tag or a provisional id (spec 04.5). Format gives the canonical form, and Hash gives the content hash (spec 06.3). Each error from this package has the file, the line and the column.
package node

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/internal/tag"
)

// Node is one parsed node file.
type Node struct {
	Path     string         // the path from the tree root, with forward slashes
	ID       string         // the tag or the provisional id from the file name
	Front    *yaml.Node     // the front matter as a YAML document, with lines and columns
	Meta     map[string]any // the front matter as plain values
	Body     string         // the body bytes, as they are in the file
	BodyLine int            // the line number of the first body line
}

// Error is an error at a place in a file.
type Error struct {
	Path string
	Line int
	Col  int
	Msg  string
}

func (e *Error) Error() string {
	if e.Line == 0 {
		return e.Path + ": " + e.Msg
	}
	return fmt.Sprintf("%s:%d:%d: %s", e.Path, e.Line, e.Col, e.Msg)
}

// Parse parses the node file data. The path gives the id and goes into each error.
func Parse(path string, data []byte) (*Node, error) {
	path = filepath.ToSlash(path)
	id := strings.TrimSuffix(filepath.Base(path), ".md")
	if !tag.IsTag(id) && !tag.IsProvisional(id) {
		return nil, &Error{Path: path, Msg: fmt.Sprintf("%q is not a tag or a provisional id", id)}
	}
	if !utf8.Valid(data) {
		line, col := badUTF8(data)
		return nil, &Error{Path: path, Line: line, Col: col, Msg: "the file is not valid UTF-8"}
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return nil, &Error{Path: path, Line: 1, Col: 1, Msg: "the file must start with a line of ---"}
	}
	head, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		if h, found := strings.CutSuffix(rest, "\n---"); found {
			head, body = h, ""
		} else {
			return nil, &Error{Path: path, Line: 1, Col: 1, Msg: "the front matter has no closing line of ---"}
		}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(head+"\n"), &doc); err != nil {
		return nil, yamlError(path, err, 1)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, &Error{Path: path, Line: 2, Col: 1, Msg: "the front matter must be a mapping"}
	}
	shiftLines(&doc, 1)
	if a := findAnchor(&doc); a != nil {
		return nil, &Error{Path: path, Line: a.Line, Col: a.Column, Msg: "the front matter must not use YAML anchors, aliases or merge keys"}
	}
	var meta map[string]any
	if err := doc.Decode(&meta); err != nil {
		return nil, yamlError(path, err, 0)
	}
	return &Node{
		Path:     path,
		ID:       id,
		Front:    &doc,
		Meta:     meta,
		Body:     body,
		BodyLine: strings.Count(head, "\n") + 4,
	}, nil
}

// badUTF8 returns the line and the column of the first byte that is not valid UTF-8. The column counts runes.
func badUTF8(data []byte) (line, col int) {
	line, col = 1, 1
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		switch {
		case r == utf8.RuneError && size <= 1:
			return line, col
		case r == '\n':
			line, col = line+1, 1
		default:
			col++
		}
		data = data[size:]
	}
	return line, col
}

// yamlLine finds the line and the column in an error from the YAML parser.
var yamlLine = regexp.MustCompile(`line (\d+)(?:, column (\d+))?`)

// yamlError changes an error from the YAML parser to an Error. The line numbers in the error get shift added. The parser counts lines from the start of the front matter, which is line 2 of the file, so a parse error needs a shift of 1. Some parser errors have no line. Such an error gets line 2, the first line of the front matter.
func yamlError(path string, err error, shift int) *Error {
	msg := err.Error()
	var te *yaml.TypeError
	if errors.As(err, &te) && len(te.Errors) > 0 {
		msg = te.Errors[0]
	}
	msg = strings.TrimPrefix(msg, "yaml: ")
	e := &Error{Path: path, Line: 2, Col: 1}
	if m := yamlLine.FindStringSubmatchIndex(msg); m != nil && m[0] == 0 {
		e.Line, _ = strconv.Atoi(msg[m[2]:m[3]])
		e.Line += shift
		if m[4] >= 0 {
			e.Col, _ = strconv.Atoi(msg[m[4]:m[5]])
		}
		msg = strings.TrimPrefix(msg[m[1]:], ": ")
	}
	msg = yamlLine.ReplaceAllStringFunc(msg, func(s string) string {
		n, _ := strconv.Atoi(yamlLine.FindStringSubmatch(s)[1])
		return "line " + strconv.Itoa(n+shift)
	})
	e.Msg = "the front matter is not valid YAML: " + msg
	return e
}

// findAnchor returns the first node that is an alias, has an anchor or is a merge key.
func findAnchor(n *yaml.Node) *yaml.Node {
	if n.Kind == yaml.AliasNode || n.Anchor != "" || n.Tag == "!!merge" {
		return n
	}
	for _, c := range n.Content {
		if a := findAnchor(c); a != nil {
			return a
		}
	}
	return nil
}

// shiftLines adds n to each line number, because the front matter starts on line 2 of the file.
func shiftLines(n *yaml.Node, by int) {
	if n.Line > 0 {
		n.Line += by
	}
	for _, c := range n.Content {
		shiftLines(c, by)
	}
}

// Load parses each node file below root/nodes. It returns the nodes in id order and every error that it found.
func Load(root string) ([]*Node, error) {
	dir := filepath.Join(root, "nodes")
	var nodes []*Node
	var errs []error
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && path == dir {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G304 G122 -- the path comes from a walk of the working tree, which the caller owns.
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		n, err := Parse(rel, data)
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		nodes = append(nodes, n)
		return nil
	})
	if err != nil {
		errs = append(errs, err)
	}
	slices.SortFunc(nodes, func(a, b *Node) int { return strings.Compare(a.ID, b.ID) })
	return nodes, errors.Join(errs...)
}

// String returns the string value of a top-level key, or "".
func (n *Node) String(key string) string {
	s, _ := n.Meta[key].(string)
	return s
}

// Kind returns the kind of the node.
func (n *Node) Kind() string { return n.String("kind") }

// Name returns the name of the node, or its id when it has no name.
func (n *Node) Name() string {
	if s := n.String("name"); s != "" {
		return s
	}
	return n.ID
}

// Strings returns the string items of a top-level list, or a top-level string as one item.
func (n *Node) Strings(key string) []string {
	switch v := n.Meta[key].(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
