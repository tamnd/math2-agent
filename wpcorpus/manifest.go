// Package wpcorpus reads and writes the Wikipedia corpus of Math 2.0.
//
// The corpus is the repository tamnd/wikipedia-math. It has the manifests of
// WP3 (the template table, the heading table and the waivers), the content
// plane (one wikitext file for each page) and the analysis plane (the objects
// and the census of each page revision). The package loads the manifests,
// extracts the objects and the census of one page, and reads and writes the
// files of the corpus.
package wpcorpus

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tamnd/wikipedia-cli/wiki/parse"
	"go.yaml.in/yaml/v3"
)

// ManifestDir is the directory of the manifests in the corpus.
const ManifestDir = "manifests/wikipedia"

// The template classes of WP3.2.
const (
	ClassItem        = "item"
	ClassTerm        = "term"
	ClassAnchor      = "anchor"
	ClassMath        = "math"
	ClassCrosswalk   = "crosswalk"
	ClassCitation    = "citation"
	ClassHatnote     = "hatnote"
	ClassNav         = "nav"
	ClassMaintenance = "maintenance"
	ClassLayout      = "layout"
	ClassMeta        = "meta"
	ClassIgnore      = "ignore"
)

var classes = map[string]bool{
	ClassItem: true, ClassTerm: true, ClassAnchor: true, ClassMath: true, ClassCrosswalk: true, ClassCitation: true,
	ClassHatnote: true, ClassNav: true, ClassMaintenance: true, ClassLayout: true, ClassMeta: true, ClassIgnore: true,
}

// Template is one entry of the template table.
type Template struct {
	Key     string            `yaml:"-"`
	Class   string            `yaml:"class"`
	Kind    string            `yaml:"kind"`
	Cue     string            `yaml:"cue"`
	Params  map[string]string `yaml:"params"`
	Aliases []string          `yaml:"aliases"`
}

// Param returns the first parameter name that has the role, in the order of
// names, or the first one in key order when names is empty. It returns "" when
// no parameter has the role.
func (t *Template) Param(role string, names ...string) string {
	for _, n := range names {
		if t.Params[n] == role {
			return n
		}
	}
	if len(names) > 0 {
		return ""
	}
	best := ""
	for n, r := range t.Params {
		if r == role && (best == "" || n < best) {
			best = n
		}
	}
	return best
}

// Waiver is one entry of waivers.yaml (WP4.8).
type Waiver struct {
	ID      string `yaml:"id"`
	Pattern struct {
		Template string `yaml:"template"`
		Heading  string `yaml:"heading"`
		Type     string `yaml:"type"`
	} `yaml:"pattern"`
	Reason string `yaml:"reason"`
	Author string `yaml:"author"`
	Date   string `yaml:"date"`
}

type templatePattern struct {
	Prefix string `yaml:"prefix"`
	Suffix string `yaml:"suffix"`
	Class  string `yaml:"class"`
}

type headingPattern struct {
	re   *regexp.Regexp
	role string
}

// Manifests holds the loaded manifests of one corpus.
type Manifests struct {
	TemplatesVersion int
	HeadingsVersion  int
	WaiversVersion   int
	Waivers          []Waiver

	templates map[string]*Template
	patterns  []templatePattern
	headings  map[string]string
	hpatterns []headingPattern
}

// Load reads the manifests of the corpus at root.
func Load(root string) (*Manifests, error) {
	dir := filepath.Join(root, ManifestDir)
	m := &Manifests{templates: map[string]*Template{}, headings: map[string]string{}}

	var tf struct {
		Version   int                  `yaml:"version"`
		Templates map[string]*Template `yaml:"templates"`
		Patterns  []templatePattern    `yaml:"patterns"`
	}
	if err := readYAML(filepath.Join(dir, "templates.yaml"), &tf); err != nil {
		return nil, err
	}
	m.TemplatesVersion = tf.Version
	for name, t := range tf.Templates {
		if t == nil {
			return nil, fmt.Errorf("templates.yaml: %s: empty entry", name)
		}
		if !classes[t.Class] {
			return nil, fmt.Errorf("templates.yaml: %s: unknown class %q", name, t.Class)
		}
		if t.Cue != "" && t.Cue != StrengthStrong && t.Cue != StrengthNormal {
			return nil, fmt.Errorf("templates.yaml: %s: cue must be strong or normal, not %q", name, t.Cue)
		}
		t.Key = parse.TemplateKey(name)
		for _, k := range append([]string{name}, t.Aliases...) {
			k = parse.TemplateKey(k)
			if prev, ok := m.templates[k]; ok {
				return nil, fmt.Errorf("templates.yaml: %s: key %q is also in %s", name, k, prev.Key)
			}
			m.templates[k] = t
		}
	}
	for i, p := range tf.Patterns {
		if (p.Prefix == "") == (p.Suffix == "") {
			return nil, fmt.Errorf("templates.yaml: pattern %d: give one of prefix and suffix", i+1)
		}
		if !classes[p.Class] {
			return nil, fmt.Errorf("templates.yaml: pattern %d: unknown class %q", i+1, p.Class)
		}
	}
	m.patterns = tf.Patterns

	var hf struct {
		Version  int                 `yaml:"version"`
		Roles    map[string][]string `yaml:"roles"`
		Patterns []struct {
			Match string `yaml:"match"`
			Role  string `yaml:"role"`
		} `yaml:"patterns"`
	}
	if err := readYAML(filepath.Join(dir, "headings.yaml"), &hf); err != nil {
		return nil, err
	}
	m.HeadingsVersion = hf.Version
	for role, hs := range hf.Roles {
		if !validRole(role) {
			return nil, fmt.Errorf("headings.yaml: role %q must start with item/ or section/", role)
		}
		for _, h := range hs {
			k := fold(normalHeading(h, ""))
			if prev, ok := m.headings[k]; ok && prev != role {
				return nil, fmt.Errorf("headings.yaml: %q is in %s and in %s", h, prev, role)
			}
			m.headings[k] = role
		}
	}
	for i, p := range hf.Patterns {
		if !validRole(p.Role) {
			return nil, fmt.Errorf("headings.yaml: pattern %d: role %q must start with item/ or section/", i+1, p.Role)
		}
		re, err := regexp.Compile(p.Match)
		if err != nil {
			return nil, fmt.Errorf("headings.yaml: pattern %d: %w", i+1, err)
		}
		m.hpatterns = append(m.hpatterns, headingPattern{re, p.Role})
	}

	var wf struct {
		Version int      `yaml:"version"`
		Waivers []Waiver `yaml:"waivers"`
	}
	err := readYAML(filepath.Join(dir, "waivers.yaml"), &wf)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	m.WaiversVersion = wf.Version
	seen := map[string]bool{}
	for i, w := range wf.Waivers {
		switch {
		case w.ID == "":
			return nil, fmt.Errorf("waivers.yaml: waiver %d has no id", i+1)
		case seen[w.ID]:
			return nil, fmt.Errorf("waivers.yaml: id %s is used twice", w.ID)
		case w.Pattern.Template == "" && w.Pattern.Heading == "":
			return nil, fmt.Errorf("waivers.yaml: %s: the pattern has no template and no heading", w.ID)
		case w.Reason == "" || w.Author == "":
			return nil, fmt.Errorf("waivers.yaml: %s: a waiver needs a reason and an author", w.ID)
		}
		seen[w.ID] = true
		if w.Pattern.Template != "" {
			w.Pattern.Template = parse.TemplateKey(w.Pattern.Template)
		}
		m.Waivers = append(m.Waivers, w)
	}
	return m, nil
}

func readYAML(path string, v any) error {
	data, err := os.ReadFile(path) // #nosec G304 -- the path is in the corpus that the user names
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return nil
}

func validRole(r string) bool {
	return strings.HasPrefix(r, "item/") || strings.HasPrefix(r, "section/")
}

// Template returns the entry of a template key, from the table or from the
// first pattern that matches. It returns nil when the key is not mapped.
func (m *Manifests) Template(key string) *Template {
	if t, ok := m.templates[key]; ok {
		return t
	}
	for _, p := range m.patterns {
		if (p.Prefix != "" && strings.HasPrefix(key, p.Prefix)) || (p.Suffix != "" && strings.HasSuffix(key, p.Suffix)) {
			return &Template{Key: key, Class: p.Class}
		}
	}
	return nil
}

// HeadingRole returns the role of a heading in the article with the given
// title, or "" when the heading is not mapped.
func (m *Manifests) HeadingRole(heading, title string) string {
	h := normalHeading(heading, title)
	if r, ok := m.headings[fold(h)]; ok {
		return r
	}
	for _, p := range m.hpatterns {
		if p.re.MatchString(h) {
			return p.role
		}
	}
	return ""
}

// Waive returns the first waiver that matches a cue, or nil. template is the
// template key of a template cue, heading is the role of a heading cue, and
// typ is the article type.
func (m *Manifests) Waive(template, heading, typ string) *Waiver {
	for i := range m.Waivers {
		w := &m.Waivers[i]
		if w.Pattern.Template != "" && w.Pattern.Template != template {
			continue
		}
		if w.Pattern.Heading != "" && w.Pattern.Heading != heading {
			continue
		}
		if w.Pattern.Type != "" && w.Pattern.Type != typ {
			continue
		}
		return w
	}
	return nil
}

var edgeWords = map[string]bool{"the": true, "a": true, "an": true, "of": true, "for": true, "and": true, "in": true, "on": true}

// normalHeading gives the normal form of a heading of WP3.5: lower case, the
// base title of the article removed, and the small words at the two ends
// removed.
func normalHeading(h, title string) string {
	s := strings.ToLower(strings.Join(strings.Fields(h), " "))
	if t := strings.ToLower(baseTitle(title)); t != "" {
		s = strings.ReplaceAll(s, t, " ")
	}
	s = strings.Trim(s, " .:")
	w := strings.Fields(s)
	for len(w) > 0 && edgeWords[w[0]] {
		w = w[1:]
	}
	for len(w) > 0 && edgeWords[w[len(w)-1]] {
		w = w[:len(w)-1]
	}
	return strings.Join(w, " ")
}

// fold folds the plural of each word, so that "proofs" and "proof" give the
// same key.
func fold(s string) string {
	w := strings.Fields(s)
	for i, x := range w {
		switch {
		case len(x) > 4 && strings.HasSuffix(x, "ies"):
			w[i] = x[:len(x)-3] + "y"
		case len(x) > 3 && strings.HasSuffix(x, "s") && !strings.HasSuffix(x, "ss") && !strings.HasSuffix(x, "is") && !strings.HasSuffix(x, "us"):
			w[i] = x[:len(x)-1]
		}
	}
	return strings.Join(w, " ")
}

// baseTitle removes the disambiguation part " (...)" from a title.
func baseTitle(title string) string {
	if i := strings.LastIndex(title, " ("); i > 0 && strings.HasSuffix(title, ")") {
		return title[:i]
	}
	return title
}
