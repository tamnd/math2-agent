package audit

import (
	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/node"
)

// a01 checks that each same_as id has the syntax of its external system (rule A01). A null value means that somebody checked and the system has no entry, and it passes.
func a01(a *auditor) outcome {
	var o outcome
	for _, n := range a.scopeNodes() {
		m := sameAs(n)
		if m == nil {
			continue
		}
		o.checked++
		for i := 0; i+1 < len(m.Content); i += 2 {
			k, v := m.Content[i], m.Content[i+1]
			if v.Tag == "!!null" {
				continue
			}
			if v.Kind != yaml.ScalarNode {
				o.add(n.Path, v.Line, v.Column, "the same_as value of %s is not a string", k.Value)
				continue
			}
			if err := crosswalk.Check(k.Value, v.Value); err != nil {
				o.add(n.Path, v.Line, v.Column, "%v", err)
			}
		}
	}
	return o
}

// sameAs returns the same_as mapping of the front matter of n, or nil when n has none.
func sameAs(n *node.Node) *yaml.Node {
	if n.Front == nil || len(n.Front.Content) == 0 {
		return nil
	}
	m := n.Front.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == "same_as" && m.Content[i+1].Kind == yaml.MappingNode {
			return m.Content[i+1]
		}
	}
	return nil
}
