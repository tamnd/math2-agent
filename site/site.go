// Package site builds the math2 wiki: a static site for GitHub Pages.
//
// The site has one page for each node, each MSC class, each spec document and each kind. People read the HTML pages. Agents read the same content as JSON in api/, as Markdown next to each page and through llms.txt. The site works with no server. Search runs in the browser from api/search.json. Math stays as TeX in the HTML, and KaTeX renders it in the browser with the macros of the content repository.
package site

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/tag"
)

// Milestone is one milestone of the plan, as the home page shows it.
type Milestone struct {
	ID    string
	Title string
	Dates string
	URL   string
}

// Options configure a build.
type Options struct {
	Content    string      // the math2 working tree
	Spec       string      // the spec directory of math2-agent, or "" for no spec pages
	Out        string      // the output directory. Build removes it first.
	URL        string      // the public URL of the site, for example https://tamnd.github.io/math2
	Repo       string      // the content repository, for example tamnd/math2
	AgentRepo  string      // the toolchain repository, for example tamnd/math2-agent
	Commit     string      // the content commit, or ""
	Version    string      // the m2 version
	Date       time.Time   // the build date. A zero date leaves it out, so that two builds give the same bytes.
	Milestones []Milestone // the plan
	Current    string      // the current milestone, for example M0
}

// kinds are the 11 node kinds with a short description, in schema order.
var kinds = []struct{ Name, Plural, Text string }{
	{"concept", "Concepts", "A mathematical idea. It can have more than one definition."},
	{"definition", "Definitions", "One definition of a concept."},
	{"statement", "Statements", "A theorem, a lemma, a proposition, a corollary, a fact or an axiom."},
	{"conjecture", "Conjectures", "A statement that nobody has proved or refuted."},
	{"problem", "Problems", "A question, for example a value to find or a classification to make."},
	{"example", "Examples", "An instance of a concept."},
	{"counterexample", "Counterexamples", "An object that shows that a statement is false."},
	{"object", "Objects", "A specific mathematical object, for example a group, a graph or a sequence."},
	{"proof", "Proofs", "An argument that proves one statement."},
	{"computation", "Computations", "A computation that anybody can run again, with its output and its certificate."},
	{"topic", "Topics", "A group of nodes for navigation."},
}

// refFields are the front matter fields that refer to other nodes, with the label for each. The order is the order on the page.
var refFields = []struct{ Key, Label, Back string }{
	{"proves", "Proves", "Proofs"},
	{"defines", "Defines", "Definitions"},
	{"instance_of", "Instance of", "Instances"},
	{"specializes", "Specializes", "Special cases"},
	{"about", "About", "Results about this"},
	{"topics", "Part of", "Nodes in this topic"},
	{"proofs", "Proofs", "Proved by this"},
	{"uses", "Uses", "Used by"},
	{"methods", "Methods", "Proofs that use this method"},
	{"implies", "Implies", "Implied by"},
	{"equivalent", "Equivalent to", "Equivalent to"},
	{"refutes", "Refutes", "Refuted by"},
	{"resolves", "Resolves", "Resolved by"},
	{"resolved_by", "Resolved by", "Resolves"},
	{"generalizes", "Generalizes", "Generalized by"},
	{"bounds", "Bounds", "Bounded by"},
	{"analogous_to", "Analogous to", "Analogous to"},
	{"successor", "Replaced by", "Replaces"},
}

var specName = regexp.MustCompile(`^([0-9]{2}-[a-z0-9-]+)\.md$`)

type link struct {
	URL, Text, Note string
	Tag             string // the node tag, for a link to a node
	Missing         bool
}

type refGroup struct {
	Field string
	Label string
	Links []link
}

type nodeView struct {
	*node.Node
	Tag, URI, Kind, KindPlural, Role, Name string
	Aliases, MSC                           []string
	Refs, Backlinks                        []refGroup
	Formal                                 []link
	Sources, SameAs                        []link
	Retired                                map[string]any
	HTML                                   template.HTML
	URL, MarkdownURL, JSONURL, EditURL     string
}

type specDoc struct {
	Slug, Title, File, URL string
	Body                   template.HTML
	Source                 []byte
}

type builder struct {
	opt     Options
	base    string // the URL path of the site, for example /math2
	nodes   map[string]*nodeView
	order   []*nodeView
	specs   []*specDoc
	macros  template.JS
	tmpl    *template.Template
	pages   []string // the paths of the HTML pages, for the sitemap
	nodeMD  *markdown
	plainMD *markdown
}

// Build writes the site to opt.Out.
func Build(opt Options) error {
	b := &builder{opt: opt, nodes: map[string]*nodeView{}, nodeMD: newMarkdown(true), plainMD: newMarkdown(false)}
	if opt.URL != "" {
		u := strings.TrimSuffix(opt.URL, "/")
		if i := strings.Index(u, "://"); i >= 0 {
			if j := strings.Index(u[i+3:], "/"); j >= 0 {
				b.base = u[i+3+j:]
			}
		}
	}
	var err error
	if b.tmpl, err = parseTemplates(b); err != nil {
		return err
	}
	if err := b.load(); err != nil {
		return err
	}
	if err := os.RemoveAll(opt.Out); err != nil {
		return err
	}
	steps := []func() error{b.writeAssets, b.writeNodes, b.writeLists, b.writeSpec, b.writeDocs, b.writeHome, b.writeAgents}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return b.writeSitemap()
}

// url returns the site URL path for p.
func (b *builder) url(p string) string { return b.base + "/" + strings.TrimPrefix(p, "/") }

func (b *builder) nodeURL(id string) string { return b.url("node/" + id + "/") }

func (b *builder) load() error {
	nodes, err := node.Load(b.opt.Content)
	if err != nil {
		return fmt.Errorf("site: read the node files: %w", err)
	}
	for _, n := range nodes {
		v := &nodeView{Node: n, Tag: n.ID, Kind: n.Kind(), Role: n.Get("role"), Name: n.Name(), Aliases: n.List("aliases"), MSC: n.List("msc")}
		v.URI = "m2://" + n.ID
		if tag.IsProvisional(n.ID) {
			v.URI = ""
		}
		v.URL = b.nodeURL(n.ID)
		v.MarkdownURL = b.url("node/" + n.ID + ".md")
		v.JSONURL = b.url("api/node/" + n.ID + ".json")
		v.EditURL = "https://github.com/" + b.opt.Repo + "/edit/main/" + n.Path
		v.KindPlural = v.Kind
		for _, k := range kinds {
			if k.Name == v.Kind {
				v.KindPlural = k.Plural
			}
		}
		if r, ok := n.Meta["retired"].(map[string]any); ok {
			v.Retired = r
		}
		b.nodes[n.ID] = v
		b.order = append(b.order, v)
	}
	if data, err := os.ReadFile(filepath.Join(b.opt.Content, "macros", "m2.katex.json")); err == nil {
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("site: macros/m2.katex.json: %w", err)
		}
		js, _ := json.Marshal(m)
		b.macros = template.JS(js) // #nosec G203 -- json.Marshal escapes <, > and & for a script element.
	} else {
		b.macros = template.JS("{}")
	}
	for _, v := range b.order {
		if err := b.resolve(v); err != nil {
			return err
		}
	}
	return b.loadSpec()
}

// refsOf returns the node ids in a front matter value: a string, a list of strings, or maps with a target.
func refsOf(v any) []link {
	switch x := v.(type) {
	case string:
		return []link{{Text: x}}
	case []any:
		var out []link
		for _, item := range x {
			out = append(out, refsOf(item)...)
		}
		return out
	case map[string]any:
		t, _ := x["target"].(string)
		if t == "" {
			return nil
		}
		l := link{Text: t}
		for _, k := range []string{"via", "side", "value", "outcome", "map", "note"} {
			if s, ok := x[k]; ok {
				l.Note += fmt.Sprintf("%s: %v. ", k, s)
			}
		}
		l.Note = strings.TrimSpace(l.Note)
		return []link{l}
	}
	return nil
}

func (b *builder) nodeLink(id string) link {
	if t, ok := b.nodes[id]; ok {
		return link{URL: t.URL, Text: t.Name, Tag: id}
	}
	return link{URL: b.nodeURL(id), Text: id, Tag: id, Missing: true}
}

func (b *builder) resolve(v *nodeView) error {
	meta := v.Meta
	for _, f := range refFields {
		val := meta[f.Key]
		if f.Key == "successor" && v.Retired != nil {
			val = v.Retired["successor"]
		}
		g := refGroup{Field: f.Key, Label: f.Label}
		for _, r := range refsOf(val) {
			l := b.nodeLink(r.Text)
			l.Note = r.Note
			g.Links = append(g.Links, l)
			if t, ok := b.nodes[r.Text]; ok {
				t.addBacklink(f.Key, f.Back, link{URL: v.URL, Text: v.Name, Tag: v.Tag})
			}
		}
		if len(g.Links) > 0 {
			v.Refs = append(v.Refs, g)
		}
	}
	v.Formal = formalLinks(meta["formal"])
	v.Sources = sourceLinks(meta["sources"])
	v.SameAs = sameAsLinks(meta["same_as"])
	body, err := b.nodeMD.render(v.Body, b.contentLinks)
	if err != nil {
		return fmt.Errorf("site: %s: %w", v.Path, err)
	}
	v.HTML = template.HTML(body) // #nosec G203 -- the body is content from the math2 repository, which review gates.
	return nil
}

func (v *nodeView) addBacklink(field, label string, l link) {
	for i := range v.Backlinks {
		if v.Backlinks[i].Field == field {
			v.Backlinks[i].Links = append(v.Backlinks[i].Links, l)
			return
		}
	}
	v.Backlinks = append(v.Backlinks, refGroup{Field: field, Label: label, Links: []link{l}})
}

// contentLinks resolves a link in a node body or in a document of the content repository.
func (b *builder) contentLinks(dest string) (string, bool) {
	if id, ok := strings.CutPrefix(dest, nodeScheme); ok {
		l := b.nodeLink(id)
		return l.URL, !l.Missing
	}
	if strings.Contains(dest, "://") || strings.HasPrefix(dest, "#") || strings.HasPrefix(dest, "mailto:") {
		return dest, true
	}
	if dest == "CONTRIBUTING.md" {
		return b.url("contribute/"), true
	}
	return "https://github.com/" + b.opt.Repo + "/blob/main/" + strings.TrimPrefix(dest, "./"), true
}

// specLinks resolves a link in a spec document.
func (b *builder) specLinks(dest string) (string, bool) {
	file, frag, _ := strings.Cut(dest, "#")
	if m := specName.FindStringSubmatch(file); m != nil {
		u := b.url("spec/" + m[1] + "/")
		if frag != "" {
			u += "#" + frag
		}
		return u, true
	}
	if strings.Contains(dest, "://") || strings.HasPrefix(dest, "#") || strings.HasPrefix(dest, "mailto:") {
		return dest, true
	}
	return "https://github.com/" + b.opt.AgentRepo + "/blob/main/spec/" + dest, true
}

func (b *builder) loadSpec() error {
	if b.opt.Spec == "" {
		return nil
	}
	files, err := filepath.Glob(filepath.Join(b.opt.Spec, "[0-9][0-9]-*.md"))
	if err != nil {
		return err
	}
	slices.Sort(files)
	for _, f := range files {
		data, err := os.ReadFile(f) // #nosec G304 -- the path comes from a glob of the spec directory.
		if err != nil {
			return err
		}
		slug := strings.TrimSuffix(filepath.Base(f), ".md")
		title := slug
		for line := range strings.Lines(string(data)) {
			if t, ok := strings.CutPrefix(line, "# "); ok {
				title = strings.TrimSpace(t)
				break
			}
		}
		body, err := b.plainMD.render(string(data), b.specLinks)
		if err != nil {
			return fmt.Errorf("site: %s: %w", f, err)
		}
		b.specs = append(b.specs, &specDoc{
			Slug: slug, Title: title, File: filepath.Base(f), URL: b.url("spec/" + slug + "/"),
			Body: template.HTML(body), Source: data, // #nosec G203 -- the spec is in the math2-agent repository, which review gates.
		})
	}
	return nil
}

// write writes data to the output path p.
func (b *builder) write(p string, data []byte) error {
	full := filepath.Join(b.opt.Out, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil { // #nosec G301 -- the site is public.
		return err
	}
	return os.WriteFile(full, data, 0o644) // #nosec G306 G703 -- the site is public, and the builder makes each path.
}

func (b *builder) writeJSON(p string, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	return b.write(p, buf.Bytes())
}

// page is the data for the layout template.
type page struct {
	Title, Description, Path, Section string
	Content                           template.HTML
	Site                              *builder
	MarkdownURL, JSONURL              string
	Math                              bool
}

// writePage renders the content template name with data, puts it in the layout and writes it to dir/index.html.
func (b *builder) writePage(dir, name string, data any, p page) error {
	var content bytes.Buffer
	if err := b.tmpl.ExecuteTemplate(&content, name, data); err != nil {
		return fmt.Errorf("site: template %s: %w", name, err)
	}
	p.Content = template.HTML(content.String()) // #nosec G203 -- html/template escaped the content.
	p.Site = b
	p.Path = dir
	var out bytes.Buffer
	if err := b.tmpl.ExecuteTemplate(&out, "layout", p); err != nil {
		return fmt.Errorf("site: layout for %s: %w", dir, err)
	}
	file := path.Join(dir, "index.html")
	if dir == "404" {
		file = "404.html"
	} else {
		b.pages = append(b.pages, dir)
	}
	return b.write(file, out.Bytes())
}

func (b *builder) writeAssets() error {
	return fs.WalkDir(files, "assets", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		return b.write(p, data)
	})
}
