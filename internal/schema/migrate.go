package schema

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// Migration changes the front matter of a node from one schema version to the next (spec 04.9). A breaking change of the schema adds a script in migrate/ and a Migration here. An additive change needs no Migration.
type Migration struct {
	From  string
	To    string
	Apply func(front *yaml.Node) error
}

// Migrations is the list of migrations, in order. The schema has had no breaking change, so the list is empty.
var Migrations []Migration

// Migrate changes front from the schema version from to Version. It returns the versions that it went through.
func Migrate(front *yaml.Node, from string) ([]string, error) {
	var path []string
	for v := from; v != Version; {
		i := -1
		for j, m := range Migrations {
			if m.From == v {
				i = j
				break
			}
		}
		if i < 0 {
			return path, fmt.Errorf("no migration goes from schema %s to %s", v, Version)
		}
		if err := Migrations[i].Apply(front); err != nil {
			return path, fmt.Errorf("migrate from %s to %s: %w", v, Migrations[i].To, err)
		}
		v = Migrations[i].To
		path = append(path, v)
		if len(path) > len(Migrations) {
			return path, fmt.Errorf("the migrations have a cycle at %s", v)
		}
	}
	return path, nil
}
