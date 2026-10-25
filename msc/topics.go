package msc

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/tag"
)

// File is the path of the topic map in the math2 tree.
const File = "topics/msc2020.yaml"

// MaxWords is the largest number of words in a label.
const MaxWords = 8

// Entry is one code of the topic map.
type Entry struct {
	Code  string // the MSC2020 code
	Label string // a short label in our own words
	Topic string // the tag of the topic node of the code, or empty
}

// Map is the topic map: an entry for each MSC2020 code.
type Map struct {
	entries  map[string]Entry
	codes    []string            // in the order of Compare
	children map[string][]string // in the order of Compare
}

// Read reads the topic map of the math2 tree at root.
func Read(root string) (*Map, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(File))) // #nosec G304 -- the caller names the tree.
	if err != nil {
		return nil, err
	}
	m, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	return m, nil
}

// Parse parses a topic map. The map is a YAML mapping from each code to its entry, for example:
//
//	"11-XX": {label: number theory}
//	"11Axx": {label: elementary number theory, topic: 7K3QX2}
//
// Parse returns all the errors that it finds, joined. The map must have the parent of each code, a label of 1 to MaxWords words for each code, and no two codes with the same label.
func Parse(data []byte) (*Map, error) {
	var raw map[string]struct {
		Label string `yaml:"label"`
		Topic string `yaml:"topic"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	m := &Map{entries: make(map[string]Entry, len(raw)), children: map[string][]string{}}
	for c, e := range raw {
		m.entries[c] = Entry{Code: c, Label: e.Label, Topic: e.Topic}
		m.codes = append(m.codes, c)
	}
	slices.SortFunc(m.codes, Compare)
	var errs []error
	labels := map[string]string{}
	for _, c := range m.codes {
		e := m.entries[c]
		if !Valid(c) {
			errs = append(errs, fmt.Errorf("%q is not an MSC2020 code", c))
			continue
		}
		if p := Parent(c); p != "" {
			if _, ok := m.entries[p]; !ok {
				errs = append(errs, fmt.Errorf("%s has no parent %s", c, p))
			}
			m.children[p] = append(m.children[p], c)
		}
		if n := len(strings.Fields(e.Label)); n == 0 || n > MaxWords || strings.ContainsAny(e.Label, "\n\\$") {
			errs = append(errs, fmt.Errorf("%s: the label %q must be plain text of 1 to %d words", c, e.Label, MaxWords))
		}
		key := strings.ToLower(e.Label)
		if other, ok := labels[key]; ok {
			errs = append(errs, fmt.Errorf("%s has the label of %s", c, other))
		}
		labels[key] = c
		if e.Topic != "" && !tag.IsTag(e.Topic) {
			errs = append(errs, fmt.Errorf("%s: the topic %q is not a tag", c, e.Topic))
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return m, nil
}

// Len returns the number of codes in the map.
func (m *Map) Len() int {
	return len(m.codes)
}

// Lookup returns the entry of the code.
func (m *Map) Lookup(c string) (Entry, bool) {
	e, ok := m.entries[c]
	return e, ok
}

// Codes returns the codes of the map in the order of Compare.
func (m *Map) Codes() []string {
	return slices.Clone(m.codes)
}

// Children returns the codes one level below the code, in the order of Compare. The children of the empty string are the top-level codes.
func (m *Map) Children(c string) []string {
	if c == "" {
		return slices.DeleteFunc(m.Codes(), func(c string) bool { return Level(c) != 1 })
	}
	return slices.Clone(m.children[c])
}
