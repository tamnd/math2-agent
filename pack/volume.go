package pack

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/card"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/status"
)

// Dir is the directory of the volume files in a math2 working tree.
const Dir = "volumes"

// Kinds are the kinds of volumes (corpus K11.5).
var Kinds = []string{"course", "book", "notes", "survey", "article", "reference", "problems"}

// The values of Volume.Proofs.
const (
	AllProofs  = "all"  // each proof of each statement
	MainProofs = "main" // the proof that the closure takes for each statement
	NoProofs   = "none" // no proofs
)

// HTML is the format of the HTML output (corpus K11.4).
const HTML = "html"

// Formats are the output formats of corpus K11.4.
var Formats = []string{HTML, LaTeX, PreTeXt, "jats", "cartridge", "blueprint", "myst", "typst", "slides"}

// Backends are the formats that have a back end now (corpus K14.2).
var Backends = []string{HTML, LaTeX, PreTeXt}

// Preface is the place of the glue that comes before the first chapter.
const Preface = "preface"

// Volume is a volume file, volumes/<id>.yaml (corpus K11.1).
type Volume struct {
	ID          string            `yaml:"id"`
	Title       string            `yaml:"title"`
	Editors     []string          `yaml:"editors"`            // the people who are responsible for the volume, as github:LOGIN or orcid:ID
	Kind        string            `yaml:"kind"`               // one of Kinds
	Audience    string            `yaml:"audience,omitempty"` // the level of the reader, as in corpus K8.3
	Targets     []string          `yaml:"targets"`            // tags, topic tags, curriculum:ID, curriculum:ID/COURSE, or msc:CODE
	Assumes     []string          `yaml:"assumes,omitempty"`  // the tags of the nodes that the reader knows
	Release     string            `yaml:"release,omitempty"`  // the math2 release that the edition pins, or "" for a draft
	Levels      Levels            `yaml:"levels,omitempty"`
	Proofs      string            `yaml:"proofs,omitempty"` // AllProofs, MainProofs or NoProofs. The default depends on the kind
	Exercises   Exercises         `yaml:"exercises,omitempty"`
	Choose      map[string]string `yaml:"choose,omitempty"` // the definition that the volume uses for a concept with two or more, by the tag of the concept
	Conventions []Convention      `yaml:"conventions,omitempty"`
	Notation    map[string]string `yaml:"notation,omitempty"` // the variant that the volume uses, by the id of the register entry
	Outline     []OutlineChapter  `yaml:"outline,omitempty"`
	Glue        []Glue            `yaml:"glue,omitempty"`
	Formats     []string          `yaml:"formats,omitempty"` // the outputs. The default is html

	File string         `yaml:"-"` // the volume file, as a slash path in the working tree
	Line map[string]int `yaml:"-"` // the line of each top-level key
}

// Levels are the lowest levels that a node must have to go in the volume (corpus K11.6).
type Levels struct {
	Writing  card.Level   `yaml:"writing,omitempty"`
	Evidence status.Level `yaml:"evidence,omitempty"` // for the kinds that have an evidence level
}

// Exercises says how many exercises each section needs.
type Exercises struct {
	PerSection int   `yaml:"per_section,omitempty"`
	Difficulty []int `yaml:"difficulty,omitempty"` // the lowest and the highest difficulty, 1 to 5. Empty means each difficulty
}

// Convention is the option of a convention of the register (corpus K10.5) that the volume uses.
type Convention struct {
	Convention string            `yaml:"convention"`
	Option     string            `yaml:"option"`
	Differ     map[string]string `yaml:"differ,omitempty"` // by tag, a node that uses another option, and the difference in one sentence
}

// OutlineChapter is a chapter of an outline.
type OutlineChapter struct {
	ID       string           `yaml:"id,omitempty"` // the place of its glue. The default is the slug of the title
	Title    string           `yaml:"title"`
	Sections []OutlineSection `yaml:"sections"`
}

// OutlineSection is a section of an outline.
type OutlineSection struct {
	ID    string   `yaml:"id,omitempty"`
	Title string   `yaml:"title"`
	Nodes []string `yaml:"nodes"` // in reading order. A proof comes after its statement, and it is not in the list
}

// Glue is one part of the editorial text (corpus K11.3).
type Glue struct {
	At        string `yaml:"at"`                  // Preface, or the id of a chapter or a section
	Text      string `yaml:"text,omitempty"`      // Markdown
	File      string `yaml:"file,omitempty"`      // a Markdown file in volumes/<id>/, instead of Text
	Writer    string `yaml:"writer"`              // the person or the agent that wrote the text
	Run       string `yaml:"run,omitempty"`       // the agent run, when an agent wrote the text
	Confirmed string `yaml:"confirmed,omitempty"` // the editor who confirmed the text of an agent (rule P04)
}

// Agent reports whether an agent wrote the glue.
func (g Glue) Agent() bool { return prov.IsAgent(g.Writer) || g.Run != "" }

var idRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Load reads the volume with the id from the working tree at root. The id can also be the path of the volume file.
func Load(root, id string) (*Volume, error) {
	rel := Dir + "/" + id + ".yaml"
	if strings.HasSuffix(id, ".yaml") {
		rel = filepath.ToSlash(id)
		if filepath.IsAbs(id) {
			r, err := filepath.Rel(root, id)
			if err != nil {
				return nil, err
			}
			rel = filepath.ToSlash(r)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) // #nosec G304 -- the path is in the working tree, which the caller owns.
	if err != nil {
		return nil, err
	}
	v, err := Parse(rel, data)
	if err != nil {
		return nil, err
	}
	for i := range v.Glue {
		g := &v.Glue[i]
		if g.File == "" {
			continue
		}
		if strings.Contains(g.File, "..") || strings.HasPrefix(g.File, "/") {
			return nil, fmt.Errorf("%s: the glue file %q is not in %s/%s/", rel, g.File, Dir, v.ID)
		}
		text, err := os.ReadFile(filepath.Join(root, Dir, v.ID, filepath.FromSlash(g.File))) // #nosec G304 -- the path is in the directory of the volume.
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		g.Text = string(text)
	}
	return v, nil
}

// Parse parses a volume file. The path names the file in errors, and its base name must be the id of the volume.
func Parse(path string, data []byte) (*Volume, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: a volume file is a mapping", path)
	}
	v := &Volume{File: path, Line: map[string]int{}}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	m := doc.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		v.Line[m.Content[i].Value] = m.Content[i].Line
	}
	if err := v.valid(path); err != nil {
		return nil, fmt.Errorf("%s:%d: %w", path, v.Line[keyOf(err)], err)
	}
	return v, nil
}

// fieldError is an error in one field of the volume file.
type fieldError struct {
	key string
	msg string
}

func (e *fieldError) Error() string { return e.msg }

func keyOf(err error) string {
	var fe *fieldError
	if errors.As(err, &fe) {
		return fe.key
	}
	return ""
}

func bad(key, format string, args ...any) error {
	return &fieldError{key, fmt.Sprintf(format, args...)}
}

func (v *Volume) valid(path string) error {
	switch {
	case !idRE.MatchString(v.ID):
		return bad("id", "the id %q is not a slug", v.ID)
	case strings.TrimSuffix(filepath.Base(path), ".yaml") != v.ID:
		return bad("id", "the id %s is not the name of the file", v.ID)
	case strings.TrimSpace(v.Title) == "":
		return bad("title", "the volume has no title")
	case len(v.Editors) == 0:
		return bad("editors", "the volume has no editor")
	case !slices.Contains(Kinds, v.Kind):
		return bad("kind", "the kind %q is not one of %s", v.Kind, strings.Join(Kinds, ", "))
	case len(v.Targets) == 0:
		return bad("targets", "the volume has no targets")
	case v.Proofs != "" && v.Proofs != AllProofs && v.Proofs != MainProofs && v.Proofs != NoProofs:
		return bad("proofs", "proofs is %q, and it must be all, main or none", v.Proofs)
	case v.Exercises.PerSection < 0:
		return bad("exercises", "per_section is less than 0")
	}
	for _, e := range v.Editors {
		if !prov.IsPerson(e) {
			return bad("editors", "the editor %q is not a person: use github:LOGIN or orcid:ID", e)
		}
	}
	if d := v.Exercises.Difficulty; len(d) > 0 && (len(d) != 2 || d[0] < 1 || d[1] > 5 || d[0] > d[1]) {
		return bad("exercises", "difficulty is %v, and it must be the lowest and the highest difficulty from 1 to 5", d)
	}
	for _, f := range v.Formats {
		if !slices.Contains(Formats, f) {
			return bad("formats", "the format %q is not one of %s", f, strings.Join(Formats, ", "))
		}
	}
	seen := map[string]bool{}
	for _, c := range v.Outline {
		if strings.TrimSpace(c.Title) == "" {
			return bad("outline", "a chapter of the outline has no title")
		}
		for _, s := range c.Sections {
			if strings.TrimSpace(s.Title) == "" {
				return bad("outline", "a section of the chapter %q has no title", c.Title)
			}
		}
		for _, id := range append([]string{c.place()}, sectionPlaces(c)...) {
			if seen[id] {
				return bad("outline", "two parts of the outline have the id %s", id)
			}
			seen[id] = true
		}
	}
	for _, g := range v.Glue {
		switch {
		case g.At == "":
			return bad("glue", "a part of the glue has no place in at")
		case (g.Text == "") == (g.File == ""):
			return bad("glue", "the glue at %s must have exactly one of text and file", g.At)
		}
	}
	for i, c := range v.Conventions {
		if c.Convention == "" || c.Option == "" {
			return bad("conventions", "the convention %d has no convention or no option", i+1)
		}
	}
	return nil
}

// place returns the id of the chapter.
func (c OutlineChapter) place() string {
	if c.ID != "" {
		return c.ID
	}
	return slug(c.Title)
}

func sectionPlaces(c OutlineChapter) []string {
	var out []string
	for _, s := range c.Sections {
		out = append(out, s.place())
	}
	return out
}

func (s OutlineSection) place() string {
	if s.ID != "" {
		return s.ID
	}
	return slug(s.Title)
}

// slug returns the words of s in lower case, joined by hyphens.
func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		default:
			dash = true
		}
	}
	return b.String()
}

// proofs returns the proofs that the volume shows. The default is NoProofs for a survey, a reference and problems, MainProofs for notes and a course, and AllProofs for a book and an article.
func (v *Volume) proofs() string {
	if v.Proofs != "" {
		return v.Proofs
	}
	switch v.Kind {
	case "survey", "reference", "problems":
		return NoProofs
	case "notes", "course":
		return MainProofs
	}
	return AllProofs
}

// Outputs returns the output formats of the volume.
func (v *Volume) Outputs() []string {
	if len(v.Formats) == 0 {
		return []string{HTML}
	}
	return v.Formats
}
