// Package format writes node files in the form that m2 fmt gives (spec 04.4 and 04.5).
//
// A Formatter does two things to a node file. First, it replaces each alias in a stored reference with the tag that the alias names (rule T04). A stored reference is the value of a reference field of the schema, at any depth of the front matter, or the target of a [[text|target]] link in the body. Then it writes the file with node.Format, which gives the canonical form of the front matter. The Formatter does not change other text of the body, and it does not change a link in code.
//
// An alias that is not in the table stays as it is. m2 check reports it as S01 in the front matter and as B02 in the body.
package format

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/schema"
	"github.com/tamnd/math2-agent/tag"
)

// Aliases maps each alias to the tag or the provisional id that it names now.
type Aliases map[string]string

// AliasRegister is the path of the alias register in a math2 working tree.
const AliasRegister = "registers/aliases.reg"

// ReadAliases returns the aliases that are in effect in the working tree at root. They come from the assignments in registers/aliases.reg that have no end date, and from the aliases field of each of the nodes. It returns an error if an alias names two nodes.
func ReadAliases(root string, nodes []*node.Node) (Aliases, error) {
	reg, err := tag.ReadAliases(filepath.Join(root, AliasRegister))
	if err != nil {
		return nil, err
	}
	a := Aliases{}
	src := map[string]string{}
	add := func(alias, id, where string) error {
		if old, ok := a[alias]; ok && old != id {
			return fmt.Errorf("the alias %s names %s in %s and %s in %s", alias, old, src[alias], id, where)
		}
		a[alias], src[alias] = id, where
		return nil
	}
	for _, r := range reg {
		if r.Until != tag.Current {
			continue
		}
		if err := add(r.Name, r.Tag, AliasRegister); err != nil {
			return nil, err
		}
	}
	for _, n := range nodes {
		for _, alias := range n.List("aliases") {
			if !tag.IsAlias(alias) {
				continue
			}
			if err := add(alias, n.ID, n.Path); err != nil {
				return nil, err
			}
		}
	}
	return a, nil
}

// Formatter writes node files in canonical form.
type Formatter struct {
	// Schema gives the key order and the reference fields. Nil means schema.Embedded.
	Schema *schema.Schema
	// Aliases gives the tag of each alias. Nil means that the Formatter does not replace aliases.
	Aliases Aliases
}

// Source returns the node file data in the form that m2 fmt gives. The path is the path of the file in the working tree. It goes into the errors, and its base name gives the id of the node. Source returns an error if the file does not parse as a node.
func (f *Formatter) Source(path string, data []byte) ([]byte, error) {
	n, err := node.Parse(path, data)
	if err != nil {
		return nil, err
	}
	s := cmp.Or(f.Schema, schema.Embedded())
	if len(f.Aliases) > 0 {
		f.front(n.Front, "", s.Refs)
		n.Body = f.body(n.Body)
	}
	return node.Format(n, s.Order), nil
}

// front replaces each alias in a reference field of the YAML node y. The field is the name of the mapping key that y is the value of.
func (f *Formatter) front(y *yaml.Node, field string, refs []string) {
	switch y.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, c := range y.Content {
			f.front(c, field, refs)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(y.Content); i += 2 {
			f.front(y.Content[i+1], y.Content[i].Value, refs)
		}
	case yaml.ScalarNode:
		if id, ok := f.Aliases[y.Value]; ok && slices.Contains(refs, field) {
			y.Value, y.Tag, y.Style = id, "!!str", 0
		}
	}
}

// body replaces each alias that is the target of a [[text|target]] link outside code.
func (f *Formatter) body(text string) string {
	lines := strings.Split(text, "\n")
	inCode := codeLines(lines)
	for i, l := range lines {
		if inCode[i] || !strings.Contains(l, "[[") {
			continue
		}
		masked := maskSpans(l)
		var b strings.Builder
		last := 0
		for _, m := range wikiLink.FindAllStringSubmatchIndex(masked, -1) {
			inner := l[m[2]:m[3]]
			bar := strings.LastIndex(inner, "|")
			if bar < 0 {
				continue
			}
			target := strings.TrimSpace(inner[bar+1:])
			id, ok := f.Aliases[target]
			if !ok {
				continue
			}
			start := m[2] + bar + 1 + strings.Index(inner[bar+1:], target)
			b.WriteString(l[last:start])
			b.WriteString(id)
			last = start + len(target)
		}
		if last > 0 {
			b.WriteString(l[last:])
			lines[i] = b.String()
		}
	}
	return strings.Join(lines, "\n")
}
