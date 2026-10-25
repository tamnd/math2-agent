// Package node reads node files.
//
// A node file is nodes/<shard>/<id>.md. It starts with YAML front matter between two lines of ---, and a Markdown body follows. The id is a tag or a provisional id (spec 04.5).
package node

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

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
	if err := yaml.Unmarshal([]byte(head), &doc); err != nil {
		return nil, &Error{Path: path, Line: 2, Col: 1, Msg: "the front matter is not valid YAML: " + err.Error()}
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, &Error{Path: path, Line: 2, Col: 1, Msg: "the front matter must be a mapping"}
	}
	shiftLines(&doc, 1)
	var meta map[string]any
	if err := doc.Decode(&meta); err != nil {
		return nil, &Error{Path: path, Line: 2, Col: 1, Msg: err.Error()}
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
