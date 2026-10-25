package audit

import (
	"bytes"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/schema"
)

// s09 finds the nodes that still have the form of the schema version before schema.Version (rule S09). A migration window is open when a migration in schema.Migrations goes to schema.Version. A node has the old form when that migration changes its front matter. When no window is open, the rule has nothing to check.
func s09(a *auditor) outcome {
	var o outcome
	var last *schema.Migration
	for i, m := range schema.Migrations {
		if m.To == schema.Version {
			last = &schema.Migrations[i]
		}
	}
	if last == nil {
		return o
	}
	for _, n := range a.scopeNodes() {
		o.checked++
		before, err := yaml.Marshal(n.Front)
		if err != nil {
			continue
		}
		// Apply the migration to a copy, so that the other rules see the file as it is.
		var c yaml.Node
		if err := yaml.Unmarshal(before, &c); err != nil {
			continue
		}
		if err := last.Apply(&c); err != nil {
			o.add(n.Path, 1, 1, "the migration from schema %s to %s fails: %v", last.From, last.To, err)
			continue
		}
		after, err := yaml.Marshal(&c)
		if err == nil && !bytes.Equal(before, after) {
			o.add(n.Path, 1, 1, "the front matter has the form of schema %s, which is deprecated; migrate it to %s", last.From, last.To)
		}
	}
	return o
}
