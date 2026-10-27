// Package migrate moves a math2 working tree from one version of the node schema to the next (spec 04.9).
//
// A breaking change of the schema adds a [Migration] to [Migrations]. During the window of the change, the toolchain accepts both versions, and rule S09 flags each node that still has the old form. [Tree] moves each node of a tree to the new form, adds the lines of the change to the provenance logs and writes the new schema file.
package migrate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/tamnd/math2-agent/exercise"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/schema"
)

// Migration changes a node from one schema version to the next.
type Migration struct {
	From string
	To   string

	// Apply returns the new form of the node, or nil when the node does not change. It does not change n.
	Apply func(n *node.Node) (*node.Node, error)

	// Fields gives, for each field of the old form, the fields of the new form that take its value. A field that is not in Fields keeps its name.
	Fields map[string][]string
}

// Migrations is the list of the migrations, in order.
var Migrations = []Migration{
	{From: "node.v1", To: "node.v2", Apply: exercise.Migrate, Fields: exercise.Moves},
}

// Window returns the migration to schema.Version, or nil when no migration window is open.
func Window() *Migration {
	for i, m := range Migrations {
		if m.To == schema.Version {
			return &Migrations[i]
		}
	}
	return nil
}

// Node changes n from the schema version from to schema.Version. It returns the new form of the node, or nil when no migration changes it, and the versions that it went through.
func Node(n *node.Node, from string) (*node.Node, []string, error) {
	var path []string
	var out *node.Node
	for v := from; v != schema.Version; {
		i := slices.IndexFunc(Migrations, func(m Migration) bool { return m.From == v })
		if i < 0 {
			return nil, path, fmt.Errorf("no migration goes from schema %s to %s", v, schema.Version)
		}
		m := Migrations[i]
		next, err := m.Apply(n)
		if err != nil {
			return nil, path, fmt.Errorf("migrate %s from %s to %s: %w", n.ID, v, m.To, err)
		}
		if next != nil {
			n, out = next, next
		}
		v = m.To
		path = append(path, v)
		if len(path) > len(Migrations) {
			return nil, path, fmt.Errorf("the migrations have a cycle at %s", v)
		}
	}
	return out, path, nil
}

// Lines returns the provenance lines of the change from before to after, at the time at. Each line takes the agent, the model, the prompt, the run and the person of the last line in log that is about the old field, so that a moved value keeps its author. A field that has no author in log gets no line.
func (m *Migration) Lines(before, after *node.Node, log []prov.Line, at string) []prov.Line {
	from := map[string]string{}
	for old, fields := range m.Fields {
		for _, f := range fields {
			from[f] = old
		}
	}
	var out []prov.Line
	for _, c := range prov.Changes(before, after, prov.Line{}) {
		old := c.Field
		if f, ok := from[c.Field]; ok {
			old = f
		}
		a, ok := prov.Author(log, old)
		if !ok {
			continue
		}
		a.Field, a.Op, a.Commit, a.At = c.Field, c.Op, "", at
		out = append(out, a)
	}
	return out
}

// Change is one node that a migration changed.
type Change struct {
	Path  string // the path of the node file from the root of the tree
	Lines int    // the count of the lines that the migration added to the provenance log
}

// Tree moves each node of the working tree at root that has the old form of the open window to schema.Version. It writes each changed node in canonical form, adds its lines to its provenance log at the time at, writes schema.File and removes schema.Previous. It does nothing when no window is open. It returns the changed nodes in tag order, and it stops at the first error.
func Tree(root, at string) ([]Change, error) {
	m := Window()
	if m == nil {
		return nil, nil
	}
	nodes, err := node.Load(root)
	if err != nil {
		return nil, err
	}
	order := schema.Embedded().Order
	var out []Change
	for _, n := range nodes {
		next, err := m.Apply(n)
		if err != nil {
			return out, err
		}
		if next == nil {
			continue
		}
		log, err := prov.Read(root, n.ID)
		if err != nil {
			return out, err
		}
		lines := m.Lines(n, next, log, at)
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(n.Path)), node.Format(next, order), 0o644); err != nil { // #nosec G306 -- the node is part of the working tree, which others read.
			return out, err
		}
		if err := prov.Append(root, n.ID, lines...); err != nil {
			return out, err
		}
		out = append(out, Change{Path: n.Path, Lines: len(lines)})
	}
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(schema.File)), 0o755); err != nil { // #nosec G301 -- the schema is part of the working tree.
		return out, err
	}
	if err := os.WriteFile(filepath.Join(root, schema.File), schema.Raw(), 0o644); err != nil { // #nosec G306 -- the schema is part of the working tree.
		return out, err
	}
	if err := os.Remove(filepath.Join(root, schema.Previous)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return out, err
	}
	return out, nil
}
