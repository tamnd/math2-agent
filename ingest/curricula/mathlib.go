package curricula

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/mathlib"
)

// Raw is the root of the raw files of the mathlib4 repository. A file is Raw + ref + "/" + path.
const Raw = "https://raw.githubusercontent.com/leanprover-community/mathlib4/"

// Site is the site of the Mathlib community, which shows the lists.
const Site = "https://leanprover-community.github.io/"

// The two forms of a list of Mathlib.
const (
	Tree  = "tree"  // sections that hold items, each item a name and a declaration
	Items = "items" // entries under a key, each with a title and its fields
)

// List is the adapter of one list of Mathlib.
type List struct {
	ID   string // the source id, for example mathlib_undergrad
	Name string // the name of the source
	File string // the path of the list in mathlib4, for example docs/undergrad.yaml
	Page string // the page of the list on Site
	Form string // Tree or Items
	Key  *regexp.Regexp
}

// Lists are the lists of Mathlib that the package registers.
var Lists = []List{
	{ID: "mathlib_undergrad", Name: "Mathlib, undergraduate mathematics", File: "docs/undergrad.yaml", Page: Site + "undergrad.html", Form: Tree},
	{ID: "mathlib_overview", Name: "Mathlib, overview", File: "docs/overview.yaml", Page: Site + "mathlib-overview.html", Form: Tree},
	{ID: "mathlib_100", Name: "Mathlib, 100 theorems", File: "docs/100.yaml", Page: Site + "100.html", Form: Items, Key: regexp.MustCompile(`^[1-9][0-9]*$`)},
	{ID: "mathlib_1000", Name: "Mathlib, 1000+ theorems", File: "docs/1000.yaml", Page: Site + "1000.html", Form: Items, Key: regexp.MustCompile(`^(Q[1-9][0-9]*)[A-Z]?$`)},
}

func init() {
	for _, l := range Lists {
		ingest.Register(l)
	}
}

// Source returns the catalogue entry of the list.
func (l List) Source() ingest.Source {
	return ingest.Source{
		ID:          l.ID,
		Name:        l.Name,
		Kind:        "curriculum",
		Home:        l.Page,
		Licence:     "Apache-2.0",
		Class:       ingest.Open,
		Attribution: "Mathlib, the Mathlib Community, " + mathlib.Repository + "/blob/master/" + l.File + ", Apache License 2.0",
	}
}

// Version returns the version of the adapter.
func (l List) Version() string { return "curricula/1" }

// Dump returns the raw file of the list on the master branch. Give the URL of the file at a commit to pin the snapshot.
func (l List) Dump() string { return Raw + "master/" + l.File }

// Fetch gets the file at req.URL. It sets req.Commit when the URL names a commit.
func (l List) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	body, err := req.Get(ctx, req.URL)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	if _, err := io.Copy(w, body); err != nil {
		return err
	}
	req.Commit = commitOf(req.URL)
	return nil
}

// Read reads the list and emits one record for each item, in the order of the file.
func (l List) Read(r io.Reader, emit func(ingest.Record) error) error {
	var doc yaml.Node
	if err := yaml.NewDecoder(r).Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%s is empty", l.File)
		}
		return fmt.Errorf("%s: %w", l.File, err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("%s is not a mapping", l.File)
	}
	var n int
	var err error
	if l.Form == Tree {
		n, err = l.readTree(doc.Content[0], emit)
	} else {
		n, err = l.readItems(doc.Content[0], emit)
	}
	if err == nil && n == 0 {
		err = fmt.Errorf("%s has no items", l.File)
	}
	return err
}

// at returns the locator of a line of the list.
func (l List) at(line int) string { return l.File + ":" + strconv.Itoa(line) }

// leanName matches a value of a tree list that is a declaration and not a link.
var leanName = regexp.MustCompile(`^[^\s/:]+$`)

// readTree reads a list of sections. A key whose value is a mapping is a section, and a key whose value is a scalar is an item. The value of an item is its declaration, a link to a page about it, or empty when Mathlib does not have it yet. The id of an item is its sections and its name, with " > " between them.
func (l List) readTree(root *yaml.Node, emit func(ingest.Record) error) (int, error) {
	n := 0
	seen := map[string]int{}
	var walk func(path []string, m *yaml.Node) error
	walk = func(path []string, m *yaml.Node) error {
		for i := 0; i+1 < len(m.Content); i += 2 {
			k, v := m.Content[i], m.Content[i+1]
			name := clean(k.Value)
			if name == "" {
				return fmt.Errorf("%s: a key has no name", l.at(k.Line))
			}
			p := append(slices.Clip(path), name)
			switch v.Kind {
			case yaml.MappingNode:
				if err := walk(p, v); err != nil {
					return err
				}
				continue
			case yaml.ScalarNode:
			default:
				return fmt.Errorf("%s: the item %q is not a declaration or a section", l.at(k.Line), name)
			}
			id := strings.Join(p, " > ")
			if first, dup := seen[id]; dup {
				return fmt.Errorf("%s: the item %q is also on line %d", l.at(k.Line), id, first)
			}
			seen[id] = k.Line
			n++
			m := map[string]string{"section": strings.Join(path, " > "), "order": strconv.Itoa(n)}
			switch val := strings.TrimSpace(v.Value); {
			case val == "":
			case strings.Contains(val, "://"):
				m["link"] = val
			case strings.HasPrefix(val, "Mathlib/"):
				m["link"] = mathlib.Docs + val
			case leanName.MatchString(val):
				m["mathlib"] = val
			default:
				return fmt.Errorf("%s: the item %q has the value %q, which is not a declaration or a link", l.at(k.Line), name, val)
			}
			rec := ingest.Record{ID: id, Title: name, URL: l.Page, Locator: l.at(k.Line), Meta: meta(m)}
			if err := emit(rec); err != nil {
				return err
			}
		}
		return nil
	}
	return n, walk(nil, root)
}

// entry is one entry of a list of items.
type entry struct {
	Title     string            `yaml:"title"`
	Decl      string            `yaml:"decl"`
	Decls     []string          `yaml:"decls"`
	Statement string            `yaml:"statement"`
	Authors   string            `yaml:"authors"`
	Date      string            `yaml:"date"`
	URL       string            `yaml:"url"`
	Links     map[string]string `yaml:"links"`
	Note      string            `yaml:"note"`
	Comment   string            `yaml:"comment"`
}

// readItems reads a list of entries. The key of an entry is its id: the number of the theorem in the 100 theorems, and the Wikidata item with an optional letter in the 1000+ theorems.
func (l List) readItems(root *yaml.Node, emit func(ingest.Record) error) (int, error) {
	n := 0
	seen := map[string]int{}
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		id := k.Value
		km := l.Key.FindStringSubmatch(id)
		if km == nil {
			return n, fmt.Errorf("%s: the key %q is not an id of %s", l.at(k.Line), id, l.ID)
		}
		if first, dup := seen[id]; dup {
			return n, fmt.Errorf("%s: the key %s is also on line %d", l.at(k.Line), id, first)
		}
		seen[id] = k.Line
		var e entry
		if err := v.Decode(&e); err != nil {
			return n, fmt.Errorf("%s: %s: %w", l.at(k.Line), id, err)
		}
		if clean(e.Title) == "" {
			return n, fmt.Errorf("%s: %s has no title", l.at(k.Line), id)
		}
		n++
		decls := slices.DeleteFunc(append([]string{e.Decl}, e.Decls...), func(s string) bool { return s == "" })
		status := ""
		switch {
		case len(decls) > 0:
			status = "formalized"
		case e.Statement != "":
			status = "statement"
		}
		if e.Statement != "" {
			decls = append(decls, e.Statement)
		}
		links := []string{e.URL}
		for _, u := range e.Links {
			links = append(links, u)
		}
		slices.Sort(links)
		links = slices.DeleteFunc(slices.Compact(links), func(s string) bool { return s == "" })
		m := map[string]string{
			"mathlib": strings.Join(decls, "|"),
			"status":  status,
			"authors": clean(e.Authors),
			"date":    e.Date,
			"links":   strings.Join(links, "|"),
			"note":    clean(strings.Join([]string{e.Note, e.Comment}, " ")),
			"order":   strconv.Itoa(n),
		}
		if len(km) > 1 {
			m["wikidata"] = km[1]
		}
		rec := ingest.Record{ID: id, Kind: "statement", Title: clean(e.Title), URL: l.Page + "#" + id, Locator: l.at(k.Line), Meta: meta(m)}
		if err := emit(rec); err != nil {
			return n, err
		}
	}
	return n, nil
}
