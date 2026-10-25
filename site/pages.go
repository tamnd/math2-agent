package site

import (
	"embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

//go:embed assets templates
var files embed.FS

func parseTemplates(b *builder) (*template.Template, error) {
	funcs := template.FuncMap{
		"url":          b.url,
		"join":         strings.Join,
		"repo":         func() string { return "https://github.com/" + b.opt.Repo },
		"agentRepo":    func() string { return "https://github.com/" + b.opt.AgentRepo },
		"mscName":      func(code string) string { return mscTop[code] },
		"mscCodes":     mscCodes,
		"subjectCount": subjectCount,
		"top": func(code string) string {
			if len(code) < 2 {
				return code
			}
			return code[:2]
		},
	}
	return template.New("site").Funcs(funcs).ParseFS(files, "templates/*.html")
}

// kindCount is one row of the table of kinds.
type kindCount struct {
	Name, Plural, Text, URL string
	Count                   int
}

func (b *builder) kindCounts() []kindCount {
	var out []kindCount
	for _, k := range kinds {
		n := 0
		for _, v := range b.order {
			if v.Kind == k.Name {
				n++
			}
		}
		out = append(out, kindCount{k.Name, k.Plural, k.Text, b.url("kind/" + k.Name + "/"), n})
	}
	return out
}

// mscGroup is the nodes of one top level MSC class.
type mscGroup struct {
	Code, Name, URL string
	Nodes           []*nodeView
}

func (b *builder) mscGroups() []*mscGroup {
	groups := map[string]*mscGroup{}
	for _, v := range b.order {
		seen := map[string]bool{}
		for _, code := range v.MSC {
			if len(code) < 2 || seen[code[:2]] {
				continue
			}
			top := code[:2]
			seen[top] = true
			g, ok := groups[top]
			if !ok {
				g = &mscGroup{Code: top, Name: mscTop[top], URL: b.url("msc/" + top + "/")}
				groups[top] = g
			}
			g.Nodes = append(g.Nodes, v)
		}
	}
	var out []*mscGroup
	for _, g := range groups {
		out = append(out, g)
	}
	slices.SortFunc(out, func(x, y *mscGroup) int { return strings.Compare(x.Code, y.Code) })
	return out
}

func (b *builder) writeNodes() error {
	for _, v := range b.order {
		desc := v.Name + ". A " + v.Kind + " in Math 2.0."
		p := page{Title: v.Name, Description: desc, Section: "nodes", MarkdownURL: v.MarkdownURL, JSONURL: v.JSONURL, Math: true}
		if err := b.writePage("node/"+v.Tag, "node", v, p); err != nil {
			return err
		}
		src, err := os.ReadFile(filepath.Join(b.opt.Content, filepath.FromSlash(v.Path))) // #nosec G304 -- the path comes from node.Load.
		if err != nil {
			return err
		}
		if err := b.write("node/"+v.Tag+".md", src); err != nil {
			return err
		}
		if err := b.writeJSON("api/node/"+v.Tag+".json", b.nodeJSON(v)); err != nil {
			return err
		}
	}
	return nil
}

func (b *builder) writeLists() error {
	type listData struct {
		Title, Text string
		Kinds       []kindCount
		Nodes       []*nodeView
		Groups      []*mscGroup
	}
	all := listData{Title: "All nodes", Kinds: b.kindCounts(), Nodes: b.order}
	if err := b.writePage("nodes", "list", all, page{Title: "All nodes", Description: "Each node of Math 2.0.", Section: "nodes", JSONURL: b.url("api/nodes.json")}); err != nil {
		return err
	}
	for _, k := range kinds {
		var nodes []*nodeView
		for _, v := range b.order {
			if v.Kind == k.Name {
				nodes = append(nodes, v)
			}
		}
		d := listData{Title: k.Plural, Text: k.Text, Kinds: b.kindCounts(), Nodes: nodes}
		if err := b.writePage("kind/"+k.Name, "list", d, page{Title: k.Plural, Description: k.Text, Section: "nodes"}); err != nil {
			return err
		}
	}
	groups := b.mscGroups()
	if err := b.writePage("msc", "subjects", groups, page{Title: "Subjects", Description: "The nodes of Math 2.0 by Mathematics Subject Classification.", Section: "msc"}); err != nil {
		return err
	}
	for _, code := range mscCodes() {
		g := &mscGroup{Code: code, Name: mscTop[code], URL: b.url("msc/" + code + "/")}
		for _, x := range groups {
			if x.Code == code {
				g = x
			}
		}
		title := code + " " + g.Name
		if err := b.writePage("msc/"+code, "subject", g, page{Title: title, Description: "The nodes of Math 2.0 in MSC class " + title + ".", Section: "msc"}); err != nil {
			return err
		}
	}
	return nil
}

type specPage struct {
	Doc        *specDoc
	Docs       []*specDoc
	Prev, Next *specDoc
}

func (b *builder) writeSpec() error {
	if err := b.writePage("spec", "specindex", b.specs, page{Title: "Specification", Description: fmt.Sprintf("The design of Math 2.0 in %d documents.", len(b.specs)), Section: "spec"}); err != nil {
		return err
	}
	for i, d := range b.specs {
		sp := specPage{Doc: d, Docs: b.specs}
		if i > 0 {
			sp.Prev = b.specs[i-1]
		}
		if i < len(b.specs)-1 {
			sp.Next = b.specs[i+1]
		}
		p := page{Title: d.Title, Description: d.Title + ". A part of the Math 2.0 specification.", Section: "spec", MarkdownURL: b.url("spec/" + d.Slug + ".md")}
		if err := b.writePage("spec/"+d.Slug, "spec", sp, p); err != nil {
			return err
		}
		if err := b.write("spec/"+d.Slug+".md", d.Source); err != nil {
			return err
		}
	}
	return nil
}

// A contentDoc is a document of the content repository that the site shows as a page.
type contentDoc struct {
	File, Slug, Title, Description string
}

// contentDocs are the documents of the content repository that the site shows. The site always has the first one, Contribute.
var contentDocs = []contentDoc{
	{"CONTRIBUTING.md", "contribute", "Contribute", "How to add and change nodes in Math 2.0."},
	{"CURATION.md", "curate", "Curation", "How to decide on the alignment proposals of m2 align."},
}

// hasDoc tells if the content repository has the document d. The site always has a Contribute page.
func (b *builder) hasDoc(d contentDoc) bool {
	if d.Slug == "contribute" {
		return true
	}
	_, err := os.Stat(filepath.Join(b.opt.Content, d.File))
	return err == nil
}

// writeDocs writes the pages from the documents of the content repository and copies the schema and the macros.
func (b *builder) writeDocs() error {
	for _, d := range contentDocs {
		data, err := os.ReadFile(filepath.Join(b.opt.Content, d.File))
		switch {
		case os.IsNotExist(err) && d.Slug == "contribute":
			data = []byte("# Contribute\n\nRead CONTRIBUTING.md in [the content repository](https://github.com/" + b.opt.Repo + ").\n")
		case os.IsNotExist(err):
			continue
		case err != nil:
			return err
		}
		body, err := b.plainMD.render(string(data), b.contentLinks)
		if err != nil {
			return fmt.Errorf("site: %s: %w", d.File, err)
		}
		doc := &specDoc{Title: d.Title, Body: template.HTML(body)} // #nosec G203 -- the document is in the math2 repository, which review gates.
		if err := b.writePage(d.Slug, "doc", doc, page{Title: d.Title, Description: d.Description, Section: "contribute", MarkdownURL: b.url(d.Slug + ".md")}); err != nil {
			return err
		}
		if err := b.write(d.Slug+".md", data); err != nil {
			return err
		}
	}
	for _, f := range []string{"schema/node.v1.json", "schema/edges.md", "macros/m2.katex.json", "macros/m2.sty"} {
		data, err := os.ReadFile(filepath.Join(b.opt.Content, filepath.FromSlash(f)))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if err := b.write(f, data); err != nil {
			return err
		}
	}
	if err := b.writePage("search", "search", nil, page{Title: "Search", Description: "Search the nodes of Math 2.0.", Section: "search"}); err != nil {
		return err
	}
	return b.writePage("404", "notfound", nil, page{Title: "Page not found", Section: ""})
}

type homeData struct {
	Kinds      []kindCount
	Total      int
	Recent     []*nodeView
	Groups     []*mscGroup
	Milestones []Milestone
	Current    string
	Specs      []*specDoc
}

func (b *builder) writeHome() error {
	d := homeData{Kinds: b.kindCounts(), Total: len(b.order), Groups: b.mscGroups(), Milestones: b.opt.Milestones, Current: b.opt.Current, Specs: b.specs}
	d.Recent = b.order[:min(len(b.order), 12)]
	p := page{Title: "", Description: "Math 2.0 is a versioned knowledge graph of mathematics for people and for agents.", Section: "home", JSONURL: b.url("api/nodes.json")}
	return b.writePage("", "home", d, p)
}

// The methods below are for the templates.

// Canonical returns the absolute URL of the page in dir.
func (b *builder) Canonical(dir string) string {
	if dir == "" || dir == "404" {
		return b.abs("")
	}
	return b.abs(dir + "/")
}

// Macros returns the KaTeX macros as JSON.
func (b *builder) Macros() template.JS { return b.macros }

// KindCounts returns the number of nodes of each kind.
func (b *builder) KindCounts() []kindCount { return b.kindCounts() }

// Version returns the m2 version.
func (b *builder) Version() string { return b.opt.Version }

// Commit returns the content commit.
func (b *builder) Commit() string { return b.opt.Commit }

// Built returns the build date, or "" when the build has no date.
func (b *builder) Built() string {
	if b.opt.Date.IsZero() {
		return ""
	}
	return b.opt.Date.UTC().Format("2 January 2006")
}

func subjectCount(groups []*mscGroup, code string) int {
	for _, g := range groups {
		if g.Code == code {
			return len(g.Nodes)
		}
	}
	return 0
}
