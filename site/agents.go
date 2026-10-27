package site

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/tamnd/math2-agent/schema"
)

// schemaFile returns the path of the node schema of the content tree: schema.File, or schema.Previous for a tree that has not moved to schema.Version.
func (b *builder) schemaFile() string {
	if p := schema.Path(b.opt.Content); p != "" {
		return p
	}
	return schema.File
}

// abs returns the absolute URL for the site path p.
func (b *builder) abs(p string) string {
	if b.opt.URL == "" {
		return b.url(p)
	}
	return strings.TrimSuffix(b.opt.URL, "/") + "/" + strings.TrimPrefix(p, "/")
}

type jsonLink struct {
	Field  string `json:"field"`
	Target string `json:"target"`
	Note   string `json:"note,omitempty"`
}

// nodeJSON is the agent view of a node: the front matter as it is in the file, the body as Markdown, and the links in both directions. A field in quarantine is not in the view. The key prov has the URL of the provenance log, the fields that an agent wrote and that no person checked, and the fields in quarantine.
func (b *builder) nodeJSON(v *nodeView) map[string]any {
	var out, in []jsonLink
	for _, g := range v.Refs {
		for _, l := range g.Links {
			out = append(out, jsonLink{Field: g.Field, Target: l.Tag, Note: l.Note})
		}
	}
	for _, g := range v.Backlinks {
		for _, l := range g.Links {
			in = append(in, jsonLink{Field: g.Field, Target: l.Tag})
		}
	}
	var pv any
	if p := v.Prov; p != nil {
		pv = map[string]any{"log": p.LogURL, "unchecked": p.unchecked, "quarantine": p.quarantined}
	}
	return map[string]any{
		"tag":       v.Tag,
		"uri":       v.URI,
		"kind":      v.Kind,
		"role":      v.Role,
		"name":      v.Name,
		"aliases":   v.Aliases,
		"msc":       v.MSC,
		"url":       b.abs(strings.TrimPrefix(v.URL, b.base)),
		"link":      b.abs("node/" + v.Tag + "/"),
		"markdown":  b.abs("node/" + v.Tag + ".md"),
		"source":    "https://github.com/" + b.opt.Repo + "/blob/main/" + v.Path,
		"front":     v.Meta,
		"body":      v.Body,
		"links":     out,
		"backlinks": in,
		"status":    nil,
		"writing":   v.Writing,
		"prov":      pv,
		"commit":    b.opt.Commit,
	}
}

type nodeEntry struct {
	Tag     string   `json:"tag"`
	Kind    string   `json:"kind"`
	Role    string   `json:"role,omitempty"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases,omitempty"`
	MSC     []string `json:"msc,omitempty"`
	Level   string   `json:"level,omitempty"` // the writing level
	URL     string   `json:"url"`
	JSON    string   `json:"json"`
}

type agentsData struct {
	URL, Nodes, Search, Spec, Schema, Macros, LLMs, LLMsFull, Example string
	Count                                                             int
}

func (b *builder) writeAgents() error {
	entries := []nodeEntry{}
	for _, v := range b.order {
		level := ""
		if v.Writing != nil {
			level = v.Writing.Level.String()
		}
		entries = append(entries, nodeEntry{v.Tag, v.Kind, v.Role, v.Name, v.Aliases, v.MSC, level, b.abs(strings.TrimPrefix(v.URL, b.base)), b.abs("api/node/" + v.Tag + ".json")})
	}
	index := map[string]any{
		"name":    "Math 2.0",
		"version": b.opt.Version,
		"commit":  b.opt.Commit,
		"schema":  b.abs(b.schemaFile()),
		"count":   len(entries),
		"nodes":   entries,
	}
	if !b.opt.Date.IsZero() {
		index["built"] = b.opt.Date.UTC().Format("2006-01-02T15:04:05Z")
	}
	if err := b.writeJSON("api/nodes.json", index); err != nil {
		return err
	}
	type searchEntry struct {
		T string   `json:"t"`
		N string   `json:"n"`
		A []string `json:"a,omitempty"`
		K string   `json:"k"`
		M []string `json:"m,omitempty"`
		U string   `json:"u"`
	}
	search := []searchEntry{}
	for _, v := range b.order {
		search = append(search, searchEntry{v.Tag, v.Name, v.Aliases, v.Kind, v.MSC, v.URL})
	}
	for _, d := range b.specs {
		search = append(search, searchEntry{T: d.Slug, N: d.Title, K: "spec", U: d.URL})
	}
	if err := b.writeJSON("api/search.json", search); err != nil {
		return err
	}
	type specEntry struct {
		Slug     string `json:"slug"`
		Title    string `json:"title"`
		URL      string `json:"url"`
		Markdown string `json:"markdown"`
	}
	specs := []specEntry{}
	for _, d := range b.specs {
		specs = append(specs, specEntry{d.Slug, d.Title, b.abs("spec/" + d.Slug + "/"), b.abs("spec/" + d.Slug + ".md")})
	}
	if err := b.writeJSON("api/spec.json", specs); err != nil {
		return err
	}
	if err := b.write("llms.txt", b.llms(false)); err != nil {
		return err
	}
	if err := b.write("llms-full.txt", b.llms(true)); err != nil {
		return err
	}
	example := "7K3QX2"
	if len(b.order) > 0 {
		example = b.order[0].Tag
	}
	d := agentsData{
		URL: b.abs(""), Nodes: b.abs("api/nodes.json"), Search: b.abs("api/search.json"), Spec: b.abs("api/spec.json"),
		Schema: b.abs(b.schemaFile()), Macros: b.abs("macros/m2.katex.json"), LLMs: b.abs("llms.txt"),
		LLMsFull: b.abs("llms-full.txt"), Example: example, Count: len(b.order),
	}
	if err := b.writePage("agents", "agents", d, page{Title: "For agents", Description: "How a program reads Math 2.0: JSON, Markdown, llms.txt and m2 URIs.", Section: "agents", MarkdownURL: b.url("llms.txt")}); err != nil {
		return err
	}
	if err := b.write("robots.txt", []byte("User-agent: *\nAllow: /\n\nSitemap: "+b.abs("sitemap.xml")+"\n")); err != nil {
		return err
	}
	return b.write(".nojekyll", nil)
}

// llms writes llms.txt in the form of llmstxt.org. The full form adds the text of each spec document and each node.
func (b *builder) llms(full bool) []byte {
	var w bytes.Buffer
	w.WriteString("# Math 2.0\n\n> Math 2.0 is a versioned knowledge graph of mathematics. Each node is a concept, a definition, a statement, a conjecture, a problem, an example, a counterexample, an object, a proof, a computation, a topic or an exercise. Each node has a permanent tag, sources, typed links to other nodes and a status that the build computes from the evidence.\n\n")
	fmt.Fprintf(&w, "Each node page has a Markdown copy at node/<TAG>.md and a JSON copy at api/node/<TAG>.json. The URI of a node is m2://<TAG>. The front matter follows the JSON Schema at %s. The site has %d nodes now.\n\n", b.abs(b.schemaFile()), len(b.order))
	w.WriteString("## Data\n\n")
	fmt.Fprintf(&w, "- [All nodes](%s): the tag, kind, name, aliases and MSC classes of each node\n", b.abs("api/nodes.json"))
	fmt.Fprintf(&w, "- [Reviewers](%s): the reviewers and their fidelity reviews\n", b.abs("api/reviewers.json"))
	fmt.Fprintf(&w, "- [Node schema](%s): JSON Schema 2020-12 for the front matter\n", b.abs(b.schemaFile()))
	fmt.Fprintf(&w, "- [Edges](%s): the 25 link types between nodes\n", b.abs("schema/edges.md"))
	fmt.Fprintf(&w, "- [Macros](%s): the TeX macros that node bodies use\n", b.abs("macros/m2.katex.json"))
	fmt.Fprintf(&w, "- [Contribute](%s): how to add a node\n", b.abs("contribute.md"))
	if b.hasDoc(contentDocs[1]) {
		fmt.Fprintf(&w, "- [Curation](%s): how to decide on the alignment proposals of m2 align\n", b.abs("curate.md"))
	}
	w.WriteString("\n")
	if len(b.specs) > 0 {
		w.WriteString("## Specification\n\n")
		for _, d := range b.specs {
			fmt.Fprintf(&w, "- [%s](%s)\n", d.Title, b.abs("spec/"+d.Slug+".md"))
		}
		w.WriteString("\n")
	}
	if len(b.order) > 0 {
		w.WriteString("## Nodes\n\n")
		for _, v := range b.order {
			fmt.Fprintf(&w, "- [%s](%s): %s %s\n", v.Name, b.abs("node/"+v.Tag+".md"), v.Kind, v.Tag)
		}
		w.WriteString("\n")
	}
	if !full {
		fmt.Fprintf(&w, "## Optional\n\n- [Full text](%s): the text of each spec document and each node in one file\n", b.abs("llms-full.txt"))
		return w.Bytes()
	}
	for _, d := range b.specs {
		fmt.Fprintf(&w, "<document source=%q>\n%s\n</document>\n\n", b.abs("spec/"+d.Slug+".md"), strings.TrimSpace(string(d.Source)))
	}
	for _, v := range b.order {
		fmt.Fprintf(&w, "<document source=%q tag=%q kind=%q>\n# %s\n\n%s\n</document>\n\n", b.abs("node/"+v.Tag+".md"), v.Tag, v.Kind, v.Name, strings.TrimSpace(v.Body))
	}
	return w.Bytes()
}

func (b *builder) writeSitemap() error {
	type loc struct {
		Loc string `xml:"loc"`
	}
	type urlset struct {
		XMLName xml.Name `xml:"urlset"`
		NS      string   `xml:"xmlns,attr"`
		URLs    []loc    `xml:"url"`
	}
	set := urlset{NS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	for _, p := range b.pages {
		u := b.abs(p)
		if p != "" {
			u += "/"
		}
		set.URLs = append(set.URLs, loc{u})
	}
	data, err := xml.MarshalIndent(set, "", "  ")
	if err != nil {
		return err
	}
	return b.write("sitemap.xml", append([]byte(xml.Header), append(data, '\n')...))
}
