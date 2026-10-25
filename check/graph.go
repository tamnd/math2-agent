package check

import (
	"fmt"
	"slices"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/tag"
)

// edgeFields are the front matter fields whose values are node references. A field can be at the top level or in a nested mapping, such as via in an item of refutes.
var edgeFields = []string{"topics", "uses", "proofs", "defines", "proves", "specializes", "about", "instance_of", "methods", "resolved_by", "successor", "target", "via"}

// edges checks that each edge target resolves (rule G01). A value that is not a tag or a provisional id is an S01 error, and G01 does not check it.
func (r *run) edges(n *node.Node) {
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
			if !slices.Contains(edgeFields, field) || !tag.IsTag(y.Value) && !tag.IsProvisional(y.Value) {
				return
			}
			r.checked["G01"]++
			if _, ok := r.lookup(y.Value); !ok {
				r.add("G01", n.Path, y.Line, y.Column, fmt.Sprintf("the %s target %s is not a node", field, y.Value))
			}
		}
	}
	walk(n.Front, "")
}
