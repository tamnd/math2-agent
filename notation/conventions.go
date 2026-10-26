package notation

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"go.yaml.in/yaml/v3"
)

// Convention is one entry of the register of conventions (corpus K10.5): a choice where sources differ, such as whether the natural numbers contain 0.
type Convention struct {
	ID      string   `yaml:"id"`
	Concept string   `yaml:"concept,omitempty"` // the tag of the concept that carries the convention
	Default string   `yaml:"default"`           // the option of math2
	Options []Option `yaml:"options"`

	Line int `yaml:"-"` // the line of the entry in notation/conventions.yaml
}

// Option is one option of a convention, with the sources that use it.
type Option struct {
	Option  string   `yaml:"option"`
	Sources []string `yaml:"sources,omitempty"` // a URL, or an id such as isbn:9780471433347 or doi:10.1007/978-1-4612-0979-9
}

// Option returns the option with the name, or nil.
func (c *Convention) Option(name string) *Option {
	for i := range c.Options {
		if c.Options[i].Option == name {
			return &c.Options[i]
		}
	}
	return nil
}

var slugRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// LoadConventions reads notation/conventions.yaml in the working tree at root. It returns an error that wraps fs.ErrNotExist when the file does not exist. An entry whose id is not a slug, an id that is in the file two times, two options with one name or a default that is not an option is an error.
func LoadConventions(root string) ([]*Convention, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ConventionsFile))) // #nosec G304 -- the path is in the working tree, which the caller owns.
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", ConventionsFile, err)
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	list := doc.Content[0]
	if list.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s:%d: the register of conventions is a list of entries", ConventionsFile, list.Line)
	}
	var out []*Convention
	seen := map[string]bool{}
	for _, item := range list.Content {
		c := &Convention{Line: item.Line}
		if err := decodeStrict(item, c); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", ConventionsFile, item.Line, err)
		}
		if err := c.valid(seen); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", ConventionsFile, item.Line, err)
		}
		out = append(out, c)
	}
	return out, nil
}

func (c *Convention) valid(seen map[string]bool) error {
	if !slugRE.MatchString(c.ID) {
		return fmt.Errorf("the id %q is not a slug", c.ID)
	}
	if seen[c.ID] {
		return fmt.Errorf("the convention %s is in the file two times", c.ID)
	}
	seen[c.ID] = true
	var names []string
	for _, o := range c.Options {
		if o.Option == "" || slices.Contains(names, o.Option) {
			return fmt.Errorf("the convention %s has an option with no name or two options named %q", c.ID, o.Option)
		}
		names = append(names, o.Option)
	}
	if c.Option(c.Default) == nil {
		return fmt.Errorf("the default %q of the convention %s is not one of its options", c.Default, c.ID)
	}
	return nil
}
