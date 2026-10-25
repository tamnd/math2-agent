package audit

import (
	"fmt"
	"path/filepath"
	"slices"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/tag"
)

// t04 checks that each stored reference is a tag or a provisional id, and not an alias or a slug (rule T04). The references are the values of the reference fields of the schema and the targets of the links in the body. A link with no target is a B02 problem, and T04 does not check it.
func t04(a *auditor) outcome {
	var o outcome
	aliases, err := tag.ReadAliases(filepath.Join(a.root, "registers", "aliases.reg"))
	if err != nil {
		o.reason = err.Error()
		return o
	}
	names := map[string]string{}
	for _, al := range aliases {
		if al.Until == tag.Current {
			names[al.Name] = al.Tag
		}
	}
	hint := func(v string) string {
		if t, ok := names[v]; ok {
			return fmt.Sprintf(". The alias %s names %s, and m2 fmt changes it to the tag", v, t)
		}
		return ""
	}
	for _, n := range a.scopeNodes() {
		walkRefs(n, a.schema.Refs, func(field string, y *yaml.Node) {
			o.checked++
			if !isRef(y.Value) {
				o.add(n.Path, y.Line, y.Column, "the %s value %q is not a tag%s", field, y.Value, hint(y.Value))
			}
		})
		for _, l := range check.Links(n) {
			if l.Target == "" {
				continue
			}
			o.checked++
			if !isRef(l.Target) {
				o.add(n.Path, l.Line, l.Col, "the link target %q is not a tag%s", l.Target, hint(l.Target))
			}
		}
	}
	return o
}

// walkRefs calls fn for each scalar value of a reference field in the front matter of n. A field can be at the top level or in a nested mapping, such as via in an item of refutes.
func walkRefs(n *node.Node, refs []string, fn func(field string, y *yaml.Node)) {
	var walk func(y *yaml.Node, field string)
	walk = func(y *yaml.Node, field string) {
		switch y.Kind {
		case yaml.DocumentNode, yaml.SequenceNode:
			for _, c := range y.Content {
				walk(c, field)
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(y.Content); i += 2 {
				walk(y.Content[i+1], y.Content[i].Value)
			}
		case yaml.ScalarNode:
			if slices.Contains(refs, field) {
				fn(field, y)
			}
		}
	}
	walk(n.Front, "")
}
