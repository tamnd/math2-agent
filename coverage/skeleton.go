package coverage

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/msc"
)

// SkeletonDir is the directory of the skeletons in the math2 tree.
const SkeletonDir = "topics/skeletons"

// Skeleton is the concept skeleton of one top-level MSC2020 class: the main concepts and definitions of the area, and the theorems that a survey or a graduate text names (spec 18.1 and 18.3). It is the file topics/skeletons/NN.yaml, where NN is the first two characters of the code. C2 of the class is the share of its items that have a live node with at least one source.
type Skeleton struct {
	Code    string            `yaml:"skeleton"` // the top-level code, for example 20-XX
	Title   string            `yaml:"title"`
	Sources map[string]Source `yaml:"sources"` // the sources that the items refer to, by their key
	Items   []Item            `yaml:"items"`
}

// Source is a source that the items of a skeleton refer to.
type Source struct {
	Title     string `yaml:"title"`
	URL       string `yaml:"url"`
	Retrieved string `yaml:"retrieved,omitempty"` // YYYY-MM-DD
}

// Item is one concept, definition or result of a skeleton. It has the fields of an item of a curriculum.
type Item struct {
	Name      string   `yaml:"name"`
	Kind      string   `yaml:"kind"`                // a kind of node, for example concept or statement
	Decl      string   `yaml:"decl,omitempty"`      // the Mathlib declaration that states or defines the item
	Wikipedia string   `yaml:"wikipedia,omitempty"` // the title of the English Wikipedia article about the item
	Wikidata  string   `yaml:"wikidata,omitempty"`  // the Wikidata item of the article
	Mathlib   []string `yaml:"mathlib,omitempty"`   // the Mathlib lists that give the item
	Node      string   `yaml:"node,omitempty"`      // the tag of the node for the item, when the node exists
}

// ReadSkeletons reads the skeletons of the math2 tree at root, in the order of their files. A tree with no skeleton directory has no skeletons.
func ReadSkeletons(root string) ([]Skeleton, error) {
	paths, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(SkeletonDir), "*.yaml"))
	if err != nil {
		return nil, err
	}
	out := make([]Skeleton, 0, len(paths))
	for _, p := range paths {
		rel := SkeletonDir + "/" + filepath.Base(p)
		data, err := os.ReadFile(p) // #nosec G304 -- a file of the tree that the caller names.
		if err != nil {
			return nil, err
		}
		var s Skeleton
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(&s); err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		if err := s.check(strings.TrimSuffix(filepath.Base(p), ".yaml")); err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		out = append(out, s)
	}
	return out, nil
}

// check returns the errors of a skeleton in the file with the name file.
func (s *Skeleton) check(file string) error {
	var errs []error
	if msc.Level(s.Code) != 1 || !strings.HasPrefix(s.Code, file) || len(file) != 2 {
		errs = append(errs, fmt.Errorf("the skeleton %q is not the top-level code of the file %s.yaml", s.Code, file))
	}
	if len(s.Items) == 0 {
		errs = append(errs, errors.New("the skeleton has no items"))
	}
	names := make(map[string]bool, len(s.Items))
	for i, it := range s.Items {
		key := strings.ToLower(it.Name)
		if it.Name == "" || it.Kind == "" {
			errs = append(errs, fmt.Errorf("item %d has no name or no kind", i+1))
		} else if names[key] {
			errs = append(errs, fmt.Errorf("item %d: the name %q occurs two times", i+1, it.Name))
		}
		names[key] = true
	}
	return errors.Join(errs...)
}
