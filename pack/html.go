package pack

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"path"
	"strings"

	"github.com/tamnd/math2-agent/katex"
	"github.com/tamnd/math2-agent/markup"
)

//go:embed volume.css
var styleCSS []byte

//go:embed volume.html.tmpl
var pageTemplate string

// Style is the name of the style sheet of the HTML output.
const Style = "volume.css"

// HTML returns the files of the HTML output, by slash path: index.html, the style sheet, and the style sheet and the fonts of KaTeX in katex/. The volume is one page, and each item has the tag as its anchor. The output is the same each time for the same document.
func (d *Document) HTML() (map[string][]byte, error) {
	r, err := newHTMLRenderer(d)
	if err != nil {
		return nil, err
	}
	page, err := r.page()
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{"index.html": page, Style: styleCSS}
	err = fs.WalkDir(katex.Assets(), ".", func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		data, err := fs.ReadFile(katex.Assets(), p)
		files[path.Join("katex", p)] = data
		return err
	})
	return files, err
}

// htmlRenderer renders one document.
type htmlRenderer struct {
	d     *Document
	katex *katex.Renderer
	md    *markup.Markdown
	err   error
}

func newHTMLRenderer(d *Document) (*htmlRenderer, error) {
	k, err := katex.New(d.tree.macros)
	if err != nil {
		return nil, err
	}
	r := &htmlRenderer{d: d, katex: k}
	r.md = markup.New(r.math)
	return r, nil
}

// math returns the HTML of a formula. A formula that KaTeX cannot parse shows as its TeX source, with the error as the title.
func (r *htmlRenderer) math(tex string, display bool) string {
	out, err := r.katex.Render(tex, display)
	if err != nil {
		return `<code class="tex-error" title="` + html.EscapeString(err.Error()) + `">` + html.EscapeString(tex) + `</code>`
	}
	return out
}

// link resolves a link of a body. A link to a node in the volume goes to its item, with the number of the item as the title. A link to a node outside the volume goes to its page on the site, as a citation by name and URI (corpus K11.2, step 4).
func (r *htmlRenderer) link(dest string) (markup.Target, bool) {
	t, ok := strings.CutPrefix(dest, markup.NodeScheme)
	if !ok {
		t, ok = strings.CutPrefix(dest, "m2://")
	}
	if !ok {
		return markup.Target{Dest: dest}, true
	}
	if it := r.d.homes[t]; it != nil {
		title := it.Label + " " + it.Number
		if it.Proof {
			title = "Proof of " + it.Name
		}
		return markup.Target{Dest: "#" + it.anchor(), Title: title, Class: "ref"}, true
	}
	if n := r.d.tree.nodes[t]; n != nil {
		ref := r.d.ref(t)
		return markup.Target{Dest: ref.URL, Title: ref.Name + ", " + ref.URI, Class: "cite"}, true
	}
	return markup.Target{Dest: "#" + t}, false
}

// ref returns the reference to a node.
func (d *Document) ref(t string) Ref {
	r := Ref{Tag: t, URI: "m2://" + t, URL: d.site + "/node/" + t + "/"}
	if n := d.tree.nodes[t]; n != nil {
		r.Name = n.Name()
	}
	return r
}

// anchor returns the id of the item in the page.
func (it *Item) anchor() string { return it.Tag }

// render renders Markdown. The first error stops the page.
func (r *htmlRenderer) render(src string) template.HTML {
	if src == "" || r.err != nil {
		return ""
	}
	out, err := r.md.Render(src, r.link)
	if err != nil {
		r.err = err
		return ""
	}
	return template.HTML(out) // #nosec G203 -- the Markdown is reviewed content of the math2 repository and reviewed glue.
}

// tex renders an inline formula.
func (r *htmlRenderer) tex(src string) template.HTML {
	return template.HTML(r.math(src, false)) // #nosec G203 -- KaTeX escapes its output.
}

// notationTeX returns the TeX that shows a macro with its arguments as letters, for example \ip{a}{b}.
func notationTeX(u NotationUse) string {
	var b strings.Builder
	b.WriteString(u.Macro)
	for i := range u.Args {
		fmt.Fprintf(&b, "{%c}", 'a'+i)
	}
	return b.String()
}

// jsonLD returns the JSON-LD of the volume (corpus K11.4a): the volume as a book, and each item as a part of it with its URI.
func (r *htmlRenderer) jsonLD() (template.HTML, error) {
	v := r.d.Volume
	var editors []map[string]string
	for _, e := range v.Editors {
		editors = append(editors, map[string]string{"@type": "Person", "identifier": e})
	}
	graph := []any{map[string]any{
		"@type":   "Book",
		"@id":     "#volume",
		"name":    v.Title,
		"editor":  editors,
		"version": r.d.Release,
		"genre":   v.Kind,
	}}
	for _, it := range r.d.Items() {
		if it.Proof {
			continue
		}
		graph = append(graph, map[string]any{
			"@type":      "CreativeWork",
			"@id":        "#" + it.anchor(),
			"identifier": "m2://" + it.Tag,
			"name":       strings.TrimSpace(it.Label + " " + it.Number + " " + it.Name),
			"url":        r.d.site + "/node/" + it.Tag + "/",
			"isPartOf":   map[string]string{"@id": "#volume"},
		})
	}
	data, err := json.MarshalIndent(map[string]any{"@context": "https://schema.org", "@graph": graph}, "", "  ")
	if err != nil {
		return "", err
	}
	// json.Marshal escapes <, > and &, so the data cannot close the script element.
	return template.HTML(`<script type="application/ld+json">` + "\n" + string(data) + "\n</script>"), nil // #nosec G203 -- see above.
}

func (r *htmlRenderer) page() ([]byte, error) {
	ld, err := r.jsonLD()
	if err != nil {
		return nil, err
	}
	t, err := template.New("volume").Funcs(template.FuncMap{
		"md":       r.render,
		"tex":      r.tex,
		"notation": notationTeX,
	}).Parse(pageTemplate)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, struct {
		*Document
		LD    template.HTML
		Style string
	}{r.d, ld, Style}); err != nil {
		return nil, err
	}
	if r.err != nil {
		return nil, r.err
	}
	return buf.Bytes(), nil
}
