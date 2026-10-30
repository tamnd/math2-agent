package resource

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"go.yaml.in/yaml/v3"
)

// CurriculaDir holds the curricula (corpus K10.3).
const CurriculaDir = "topics/curricula"

// TextKinds are the kinds of a text that a curriculum names (RS5.6).
var TextKinds = []string{"book", "notes", "course", "paper"}

// Text is one work that a curriculum names: a book, lecture notes, a course or a paper. A curriculum names it by resource id (change to K5.8). The name is the free text for display, and Resource is empty until the resolver or a person finds the resource.
type Text struct {
	Name     string `yaml:"name"`
	Resource string `yaml:"resource,omitempty"`
	Kind     string `yaml:"kind,omitempty"`

	File   string `yaml:"-"` // the file of the curriculum, relative to the root, with slashes
	Line   int    `yaml:"-"`
	Course string `yaml:"-"` // the course that names the text, or empty for the curriculum
}

// ReadTexts reads the texts that the curricula of the tree at root name: the texts of each curriculum and of each of its courses, in the order of the files. A tree with no curricula has no texts.
func ReadTexts(root string) ([]Text, error) {
	paths, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(CurriculaDir), "*.yaml"))
	if err != nil {
		return nil, err
	}
	slices.Sort(paths)
	var out []Text
	for _, p := range paths {
		rel := CurriculaDir + "/" + filepath.Base(p)
		data, err := os.ReadFile(p) // #nosec G304 -- the path is in the tree, which the caller owns.
		if err != nil {
			return nil, err
		}
		var c struct {
			Texts   []yaml.Node `yaml:"texts"`
			Courses []struct {
				Course string      `yaml:"course"`
				Texts  []yaml.Node `yaml:"texts"`
			} `yaml:"courses"`
		}
		if err := yaml.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		add := func(nodes []yaml.Node, course string) error {
			for i := range nodes {
				var t Text
				if err := nodes[i].Decode(&t); err != nil {
					return fmt.Errorf("%s:%d: %w", rel, nodes[i].Line, err)
				}
				if t.Name == "" {
					return fmt.Errorf("%s:%d: a text needs a name", rel, nodes[i].Line)
				}
				if t.Kind != "" && !slices.Contains(TextKinds, t.Kind) {
					return fmt.Errorf("%s:%d: the kind %q of a text is not one of %v", rel, nodes[i].Line, t.Kind, TextKinds)
				}
				t.File, t.Line, t.Course = rel, nodes[i].Line, course
				out = append(out, t)
			}
			return nil
		}
		if err := add(c.Texts, ""); err != nil {
			return nil, err
		}
		for _, course := range c.Courses {
			if err := add(course.Texts, course.Course); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}
