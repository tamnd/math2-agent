package audit

import (
	"github.com/tamnd/math2-agent/migrate"
)

// s09 finds the nodes that still have the form of the schema version before schema.Version (rule S09). A migration window is open when a migration in migrate.Migrations goes to schema.Version. A node has the old form when that migration changes it. When no window is open, the rule has nothing to check.
func s09(a *auditor) outcome {
	var o outcome
	m := migrate.Window()
	if m == nil {
		return o
	}
	for _, n := range a.scopeNodes() {
		o.checked++
		next, err := m.Apply(n)
		switch {
		case err != nil:
			o.add(n.Path, 1, 1, "the migration from schema %s to %s fails: %v", m.From, m.To, err)
		case next != nil:
			o.add(n.Path, 1, 1, "the node has the form of schema %s, which is deprecated; migrate it to %s with m2 migrate %s", m.From, m.To, m.To)
		}
	}
	return o
}
