package site

import (
	"embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/schema"
)

//go:embed assets templates
var files embed.FS

func parseTemplates(b *builder) (*template.Template, error) {
	funcs := template.FuncMap{
		"url":          b.url,
		"schemaFile":   b.schemaFile,
		"join":         strings.Join,
		"list":         list,
		"repo":         func() string { return "https://github.com/" + b.opt.Repo },
		"agentRepo":    func() string { return "https://github.com/" + b.opt.AgentRepo },
		"mscName":      func(code string) string { return mscTop[code] },
		"mscCodes":     mscCodes,
		"subjectCount": subjectCount,
		"hue":          hue,
		"steps":        b.Steps,
		"area":         b.Area,
		"skeleton":     func(code string) *learnPath { return b.skeletons[code] },
		"top": func(code string) string {
			if len(code) < 2 {
				return code
			}
			return code[:2]
		},
	}
	return template.New("site").Funcs(funcs).ParseFS(files, "templates/*.html")
}

// list joins the items with commas and an "and" before the last item.
func list(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// kindCount is one row of the table of kinds.
type kindCount struct {
	Name, Plural, Text, URL string
	Count                   int
	Rows                    int // the rows of the inventory of this kind
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
		rows := 0
		if b.invSummary != nil {
			rows = b.invSummary.Kinds[k.Name]
		}
		out = append(out, kindCount{k.Name, k.Plural, k.Text, b.url("kind/" + k.Name + "/"), n, rows})
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
		if err := b.writePage("wiki/"+v.Slug, "node", v, p); err != nil {
			return err
		}
		if !b.opt.Cloudflare {
			if err := b.write("node/"+v.Tag+"/index.html", b.shortLink(v)); err != nil {
				return err
			}
		}
		src, err := os.ReadFile(filepath.Join(b.opt.Content, filepath.FromSlash(v.Path))) // #nosec G304 -- the path comes from node.Load.
		if err != nil {
			return err
		}
		if v.source != nil {
			src = v.source(src)
		}
		if err := b.write("node/"+v.Tag+".md", src); err != nil {
			return err
		}
		if err := b.writeJSON("api/node/"+v.Tag+".json", b.nodeJSON(v)); err != nil {
			return err
		}
	}
	if b.opt.Cloudflare {
		return b.writeWorker()
	}
	return nil
}

func (b *builder) writeLists() error {
	type listData struct {
		Title, Text string
		Kinds       []kindCount
		Levels      []levelCount
		Nodes       []*nodeView
		Groups      []*mscGroup
	}
	all := listData{Title: "Every idea", Kinds: b.kindCounts(), Nodes: b.order}
	if err := b.writePage("nodes", "list", all, page{Title: "Every idea", Description: "Each idea in Math 2.0.", Section: "nodes", JSONURL: b.url("api/nodes.json")}); err != nil {
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
	idx := subjectsData{Full: slices.Clone(groups)}
	b.depthOf = b.depths()
	slices.SortStableFunc(idx.Full, func(x, y *mscGroup) int { return len(y.Nodes) - len(x.Nodes) })
	for _, code := range mscCodes() {
		if subjectCount(groups, code) == 0 {
			idx.Empty = append(idx.Empty, &mscGroup{Code: code, Name: mscTop[code], URL: b.url("msc/" + code + "/")})
		}
	}
	if err := b.writePage("msc", "subjects", idx, page{Title: "Subjects", Description: "The nodes of Math 2.0 by Mathematics Subject Classification.", Section: "msc"}); err != nil {
		return err
	}
	for _, code := range mscCodes() {
		g := &mscGroup{Code: code, Name: mscTop[code], URL: b.url("msc/" + code + "/")}
		for _, x := range groups {
			if x.Code == code {
				g = x
			}
		}
		title := g.Name
		if err := b.writePage("msc/"+code, "subject", b.subjectView(g), page{Title: title, Description: "The ideas of " + title + " in Math 2.0, the foundations first.", Section: "msc", Math: true}); err != nil {
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

// specGroup is the main spec or one of its parts, in the spec index.
type specGroup struct {
	Title string
	Docs  []*specDoc
}

// specGroups splits the spec documents by part, in the order of loadSpec.
func (b *builder) specGroups() []specGroup {
	var gs []specGroup
	for _, d := range b.specs {
		if len(gs) == 0 || gs[len(gs)-1].Docs[0].Part != d.Part {
			title := ""
			if d.Part != "" {
				title = d.Title
			}
			gs = append(gs, specGroup{Title: title})
		}
		gs[len(gs)-1].Docs = append(gs[len(gs)-1].Docs, d)
	}
	return gs
}

func (b *builder) writeSpec() error {
	groups := b.specGroups()
	if err := b.writePage("spec", "specindex", groups, page{Title: "Specification", Description: fmt.Sprintf("The design of Math 2.0 in %d documents.", len(b.specs)), Section: "spec"}); err != nil {
		return err
	}
	for _, g := range groups {
		for i, d := range g.Docs {
			sp := specPage{Doc: d, Docs: g.Docs}
			if i > 0 {
				sp.Prev = g.Docs[i-1]
			}
			if i < len(g.Docs)-1 {
				sp.Next = g.Docs[i+1]
			}
			p := page{Title: d.Title, Description: d.Title + ". A part of the Math 2.0 specification.", Section: "spec", MarkdownURL: b.url("spec/" + d.Slug + ".md")}
			if err := b.writePage("spec/"+d.Slug, "spec", sp, p); err != nil {
				return err
			}
			if err := b.write("spec/"+d.Slug+".md", d.Source); err != nil {
				return err
			}
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
		body, err := b.plainMD.Render(string(data), dests(b.contentLinks))
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
	for _, f := range []string{schema.File, schema.Previous, "schema/edges.md", "macros/m2.katex.json", "macros/m2.sty"} {
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
	Levels     []levelCount
	Total      int
	Recent     []*nodeView
	Groups     []*mscGroup
	Milestones []Milestone
	Current    string
	Specs      []*specDoc
	Inventory  *inventory.Summary
	Letters    []*invLetter
	Top        []*mscGroup // the subjects with the most nodes
	Latest     []feedCard  // the newest nodes
}

func (b *builder) writeHome() error {
	d := homeData{Kinds: b.kindCounts(), Levels: b.levelCounts(), Total: len(b.order), Groups: b.mscGroups(), Milestones: b.opt.Milestones, Current: b.opt.Current, Specs: b.specs, Letters: b.invLetters}
	if b.invSummary != nil && b.invSummary.Rows > 0 {
		d.Inventory = b.invSummary
	}
	d.Recent = b.order[:min(len(b.order), 12)]
	d.Top = slices.Clone(d.Groups)
	slices.SortStableFunc(d.Top, func(x, y *mscGroup) int { return len(y.Nodes) - len(x.Nodes) })
	d.Top = d.Top[:min(len(d.Top), 12)]
	latest := b.feedNodes()
	if b.history {
		slices.SortStableFunc(latest, func(x, y *nodeView) int { return y.Updated.Compare(x.Updated) })
	}
	for _, v := range latest[:min(len(latest), 6)] {
		d.Latest = append(d.Latest, b.card(v, false))
	}
	p := page{Title: "", Description: "Math 2.0 is a free, open map of mathematics, one idea at a time, for people and for agents.", Section: "home", JSONURL: b.url("api/nodes.json"), Math: true}
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

// shortLink returns the page at node/TAG/, the short link of a node that does not change when the name changes. It sends the reader to the page of the node.
func (b *builder) shortLink(v *nodeView) []byte {
	to := template.HTMLEscapeString(v.URL)
	rel := template.HTMLEscapeString("../../wiki/" + v.Slug + "/")
	name := template.HTMLEscapeString(v.Name)
	return []byte(`<!doctype html>
<html lang="en">
<meta charset="utf-8">
<title>` + name + `</title>
<link rel="canonical" href="` + to + `">
<meta name="robots" content="noindex">
<meta http-equiv="refresh" content="0; url=` + rel + `">
<p>The page of this node is <a href="` + to + `">` + name + `</a>.</p>
</html>
`)
}

type subjectsData struct {
	Full  []*mscGroup // the subjects with nodes, the most nodes first
	Empty []*mscGroup // the subjects with no node
}

// kindGroup is the nodes of one kind on the page of a subject.
type kindGroup struct {
	Title string
	Nodes []*nodeView
}

type subjectView struct {
	*mscGroup
	Start []*nodeView // the first nodes to read: the first items of the map with a node, or the concepts with the shortest reading paths
	Map   *learnPath  // the skeleton of the subject, or nil
	Kinds []kindGroup // the nodes that are not on the map, by kind
}

// kindTitles are the kinds in the order of the page of a subject, with the title of each for a reader. A definition comes late, because the page of its concept comes first.
var kindTitles = []struct{ Kind, Title string }{
	{"concept", "Concepts"},
	{"statement", "Theorems and results"},
	{"example", "Examples"},
	{"counterexample", "Counterexamples"},
	{"object", "Objects"},
	{"problem", "Problems"},
	{"conjecture", "Conjectures"},
	{"exercise", "Exercises"},
	{"computation", "Computations"},
	{"proof", "Proofs"},
	{"definition", "Definitions"},
	{"topic", "Topics"},
}

// depths gives the length of the reading path of each node: the number of nodes that it needs, with itself. A concept has no uses of its own, so its depth is the least depth of the definitions that define it.
func (b *builder) depths() map[*nodeView]int {
	d := map[*nodeView]int{}
	for _, v := range b.order {
		d[v] = 1
		if v.Closure != nil {
			d[v] = len(v.Closure.Items)
		}
	}
	for _, v := range b.order {
		c, ok := v.Meta["defines"].(string)
		if !ok || v.Kind != "definition" || b.nodes[c] == nil || b.nodes[c].Closure != nil {
			continue
		}
		if k := b.nodes[c]; d[k] == 1 || d[v] < d[k] {
			d[k] = d[v]
		}
	}
	return d
}

// subjectView orders the nodes of a subject for a learner: the foundations first, by the length of their reading paths, and by kind.
func (b *builder) subjectView(g *mscGroup) subjectView {
	sv := subjectView{mscGroup: g, Map: b.skeletons[g.Code]}
	onMap := map[*nodeView]bool{}
	if sv.Map != nil {
		for _, it := range sv.Map.Items {
			if it.Node != nil {
				onMap[it.Node] = true
			}
		}
		sv.Start = sv.Map.Start(6)
	}
	depth := b.depthOf
	nodes := slices.Clone(g.Nodes)
	slices.SortStableFunc(nodes, func(x, y *nodeView) int {
		if d := depth[x] - depth[y]; d != 0 {
			return d
		}
		return strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
	})
	for _, v := range nodes {
		if sv.Map == nil && v.Kind == "concept" && v.Excerpt != "" && v.Retired == nil && len(sv.Start) < 6 {
			sv.Start = append(sv.Start, v)
		}
	}
	for _, k := range kindTitles {
		kg := kindGroup{Title: k.Title}
		for _, v := range nodes {
			if v.Kind == k.Kind && !onMap[v] {
				kg.Nodes = append(kg.Nodes, v)
			}
		}
		if len(kg.Nodes) > 0 {
			sv.Kinds = append(sv.Kinds, kg)
		}
	}
	return sv
}

// hue gives a colour hue in degrees for a subject code, so that each subject keeps its colour on each page.
func hue(code string) int {
	if code == "" {
		return 250
	}
	h := 0
	for _, r := range code {
		h = h*31 + int(r)
	}
	return (h*47 + 200) % 360
}
