package propose

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/schema"
	"github.com/tamnd/math2-agent/tag"
)

// maxText is the largest node file that a write can give, in bytes.
const maxText = 256 << 10

// NewNode returns the change of propose_node: a new node file in nodes/~/ with the text. The node gets a new provisional id, and the mint workflow gives it a tag when the pull request merges. The tag field of the text must be empty, absent or a provisional id.
func NewNode(root string, text []byte) (Change, error) {
	s, err := schema.Load(root)
	if err != nil {
		return Change{}, err
	}
	var id string
	for range 8 {
		if id, err = tag.Provisional(nil); err != nil {
			return Change{}, err
		}
		if _, err := os.Stat(filepath.Join(root, "nodes", "~", id+".md")); errors.Is(err, fs.ErrNotExist) {
			break
		}
	}
	path := "nodes/~/" + id + ".md"
	n, err := parse(path, text)
	if err != nil {
		return Change{}, err
	}
	if t := n.Get("tag"); t != "" && !tag.IsProvisional(t) {
		return Change{}, fmt.Errorf("propose: the text has the tag %s. A new node gets a provisional id, and the mint workflow gives it a tag", t)
	}
	setScalar(n, "tag", id)
	if !slices.Contains(s.Kinds(), n.Kind()) {
		return Change{}, fmt.Errorf("propose: %q is not a kind. The kinds are %s", n.Kind(), strings.Join(s.Kinds(), ", "))
	}
	name := n.Name()
	if name == "" {
		name = id
	}
	return Change{Method: MethodNode, Tag: id, Title: fmt.Sprintf("Add the %s %s", n.Kind(), name), Files: map[string][]byte{path: node.Format(n, s.Order)}}, nil
}

// Edit returns the change of propose_edit: the node file of the tag gets the text. The text must keep the tag and the kind of the node.
func Edit(root, t string, text []byte) (Change, error) {
	old, path, err := read(root, t)
	if err != nil {
		return Change{}, err
	}
	n, err := parse(path, text)
	if err != nil {
		return Change{}, err
	}
	switch {
	case n.Get("tag") != t:
		return Change{}, fmt.Errorf("propose: the text has the tag %q, and an edit keeps the tag %s", n.Get("tag"), t)
	case n.Kind() != old.Kind():
		return Change{}, fmt.Errorf("propose: the text has the kind %q, and an edit keeps the kind %s. To change the kind, retire the node and propose a new one", n.Kind(), old.Kind())
	}
	s, err := schema.Load(root)
	if err != nil {
		return Change{}, err
	}
	data := node.Format(n, s.Order)
	if node.Hash(n) == node.Hash(old) {
		return Change{}, fmt.Errorf("propose: the text does not change %s", t)
	}
	return Change{Method: MethodEdit, Tag: t, Title: "Edit " + t + " " + old.Name(), Files: map[string][]byte{path: data}}, nil
}

// Binding is a binding of a node to a Lean declaration, for Bind.
type Binding struct {
	Library string // the library of the declaration. Empty means mathlib.
	Decl    string // the full name of the declaration
	Match   string // exact, stronger, weaker, variant or partial
	Adapter string // the adapter in Math2/Meta/Match.lean, for a match that is not exact
}

var declRE = regexp.MustCompile(`^[\pL_][\pL\pN_'!?]*(\.[\pL_][\pL\pN_'!?]*)*$`)

// Bind returns the change of propose_binding: the node of the tag gets a lean4 binding in its field formal. The binding has no review yet, so it starts at F1 at most (spec 10).
func Bind(root, t string, b Binding) (Change, error) {
	n, path, err := read(root, t)
	if err != nil {
		return Change{}, err
	}
	if b.Library == "" {
		b.Library = "mathlib"
	}
	switch {
	case !declRE.MatchString(b.Decl):
		return Change{}, fmt.Errorf("propose: %q is not the name of a Lean declaration", b.Decl)
	case !slices.Contains(matches, b.Match):
		return Change{}, fmt.Errorf("propose: the match %q is not %s", b.Match, strings.Join(matches, ", "))
	case b.Match == "exact" && b.Adapter != "":
		return Change{}, errors.New("propose: an exact binding has no adapter")
	case b.Adapter != "" && !declRE.MatchString(b.Adapter):
		return Change{}, fmt.Errorf("propose: %q is not the name of a Lean declaration", b.Adapter)
	}
	formal := field(n, "formal", yaml.SequenceNode)
	for _, f := range formal.Content {
		if f.Kind == yaml.MappingNode && value(f, "system") == "lean4" && value(f, "decl") == b.Decl {
			return Change{}, fmt.Errorf("propose: %s has a binding to %s", t, b.Decl)
		}
	}
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	add(m, "system", "lean4")
	add(m, "library", b.Library)
	add(m, "decl", b.Decl)
	add(m, "match", b.Match)
	if b.Adapter != "" {
		add(m, "adapter", b.Adapter)
	}
	formal.Content = append(formal.Content, m)
	return format(root, MethodBinding, t, "Bind "+t+" to "+b.Decl, path, n)
}

var matches = []string{"exact", "stronger", "weaker", "variant", "partial"}

// Alignment is an edge from a node to an external system or a paper result, for Align.
type Alignment struct {
	Edge   string // same_as, close_to or aligned
	System string // the external system of same_as and close_to, for example wikidata
	ID     string // the id in the system, or the m2:// URI of the paper result of aligned
	Note   string // how the two differ, for close_to
	Match  string // the match of aligned: exact, stronger, weaker, variant or partial
}

// Align returns the change of propose_alignment: the node of the tag gets the edge. A same_as edge replaces the value of the system that the node has.
func Align(root, t string, a Alignment) (Change, error) {
	n, path, err := read(root, t)
	if err != nil {
		return Change{}, err
	}
	if a.ID == "" {
		return Change{}, errors.New("propose: the alignment has no id")
	}
	switch a.Edge {
	case "same_as":
		if a.System == "" || a.Note != "" || a.Match != "" {
			return Change{}, errors.New("propose: a same_as edge has a system and an id, and no note and no match")
		}
		m := field(n, "same_as", yaml.MappingNode)
		set(m, a.System, a.ID)
	case "close_to":
		if a.System == "" || a.Note == "" || a.Match != "" {
			return Change{}, errors.New("propose: a close_to edge has a system, an id and a note that says how the two differ, and no match")
		}
		m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		add(m, "system", a.System)
		add(m, "id", a.ID)
		add(m, "note", a.Note)
		l := field(n, "close_to", yaml.SequenceNode)
		l.Content = append(l.Content, m)
	case "aligned":
		if a.System != "" || a.Note != "" || !slices.Contains(matches, a.Match) {
			return Change{}, fmt.Errorf("propose: an aligned edge has the URI of a paper result and a match of %s, and no system and no note", strings.Join(matches, ", "))
		}
		m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		add(m, "record", a.ID)
		add(m, "match", a.Match)
		l := field(n, "aligned", yaml.SequenceNode)
		l.Content = append(l.Content, m)
	default:
		return Change{}, fmt.Errorf("propose: the edge %q is not same_as, close_to or aligned", a.Edge)
	}
	what := a.System
	if a.Edge == "aligned" {
		what = "the paper result"
	}
	return format(root, MethodAlignment, t, fmt.Sprintf("Align %s with %s %s", t, what, a.ID), path, n)
}

// read reads the node file of the tag t from the tree. It returns the node and its path.
func read(root, t string) (*node.Node, string, error) {
	if !tag.IsTag(t) {
		return nil, "", fmt.Errorf("propose: %q is not a tag", t)
	}
	path := "nodes/" + tag.Shard(t) + "/" + t + ".md"
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path))) // #nosec G304 -- the path is made from a checked tag.
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "", fmt.Errorf("propose: no node has the tag %s", t)
	}
	if err != nil {
		return nil, "", err
	}
	n, err := node.Parse(path, data)
	if err != nil {
		return nil, "", fmt.Errorf("propose: the tree has a node file that does not parse: %w", err)
	}
	return n, path, nil
}

// parse parses the text of a node file at the path.
func parse(path string, text []byte) (*node.Node, error) {
	if len(text) > maxText {
		return nil, fmt.Errorf("propose: the text has %d bytes, and a node file can have %d", len(text), maxText)
	}
	n, err := node.Parse(path, text)
	if err != nil {
		return nil, fmt.Errorf("propose: the text is not a node file: %w", err)
	}
	return n, nil
}

// format returns the change of the node n after an edit of its front matter. The edit must change the node.
func format(root, method, t, title, path string, n *node.Node) (Change, error) {
	s, err := schema.Load(root)
	if err != nil {
		return Change{}, err
	}
	data := node.Format(n, s.Order)
	if _, err := node.Parse(path, data); err != nil {
		return Change{}, fmt.Errorf("propose: the new node file does not parse: %w", err)
	}
	if old, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path))); err == nil && bytes.Equal(old, data) { // #nosec G304 -- the path is made from a checked tag.
		return Change{}, fmt.Errorf("propose: the change does not change %s", t)
	}
	return Change{Method: method, Tag: t, Title: title, Files: map[string][]byte{path: data}}, nil
}

// front returns the mapping of the front matter of n.
func front(n *node.Node) *yaml.Node {
	return n.Front.Content[0]
}

// field returns the value of the key in the front matter of n, of the kind. It adds the key with an empty value of the kind when n has no such key.
func field(n *node.Node, key string, kind yaml.Kind) *yaml.Node {
	m := front(n)
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key && m.Content[i+1].Kind == kind {
			return m.Content[i+1]
		}
	}
	tags := map[yaml.Kind]string{yaml.MappingNode: "!!map", yaml.SequenceNode: "!!seq"}
	v := &yaml.Node{Kind: kind, Tag: tags[kind]}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
	return v
}

// setScalar sets the key of the front matter of n to the string value.
func setScalar(n *node.Node, key, v string) {
	set(front(n), key, v)
	n.Meta[key] = v
}

// set sets the key of the mapping m to the string value.
func set(m *yaml.Node, key, v string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
			return
		}
	}
	add(m, key, v)
}

// add adds the key with the string value at the end of the mapping m.
func add(m *yaml.Node, key, v string) {
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v})
}

// value returns the value of a key of a mapping, or "".
func value(m *yaml.Node, key string) string {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1].Value
		}
	}
	return ""
}
