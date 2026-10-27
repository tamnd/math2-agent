package pack

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"

	"github.com/tamnd/math2-agent/markup"
)

// LaTeX is the format of the LaTeX output (corpus K11.4).
const LaTeX = "latex"

// TeXLive is the year of the TeX Live that the 00README.json of an article names for arXiv.
const TeXLive = 2025

// The files of the LaTeX output.
const (
	MainTeX     = "main.tex"
	MacroSty    = "m2.sty"
	MainBib     = "main.bib"
	ArxivReadme = "00README.json"
)

// LaTeX returns the files of the LaTeX output, by slash path: main.tex, a copy of m2.sty, and main.bib when the items cite sources. An article also has 00README.json, so that the files are an arXiv source tree. Each item is an amsthm environment with its number from the document and its tag as the label. main.tex builds with pdflatex and the packages of TeX Live. It needs no BibTeX run, because the list of sources is in main.tex too. The output is the same each time for the same document.
func (d *Document) LaTeX() (map[string][]byte, error) {
	w := &texWriter{d: d, b: &strings.Builder{}}
	main := w.main()
	files := map[string][]byte{MainTeX: []byte(main), MacroSty: d.sty()}
	if len(d.Sources) > 0 {
		files[MainBib] = []byte(d.bib())
	}
	if d.Volume.Kind == "article" {
		readme, err := d.arxivReadme(files)
		if err != nil {
			return nil, err
		}
		files[ArxivReadme] = readme
	}
	return files, nil
}

// arxivReadme returns the 00README.json of arXiv: the compiler, the TeX Live year, and the use of each file.
func (d *Document) arxivReadme(files map[string][]byte) ([]byte, error) {
	type source struct {
		Filename string `json:"filename"`
		Usage    string `json:"usage"`
	}
	var sources []source
	for _, name := range slices.Sorted(maps.Keys(files)) {
		usage := "include"
		switch name {
		case MainTeX:
			usage = "toplevel"
		case MainBib:
			usage = "ignore"
		}
		sources = append(sources, source{name, usage})
	}
	slices.SortStableFunc(sources, func(a, b source) int { return boolInt(b.Usage == "toplevel") - boolInt(a.Usage == "toplevel") })
	data, err := json.MarshalIndent(map[string]any{
		"spec_version":    1,
		"process":         map[string]string{"compiler": "pdflatex"},
		"sources":         sources,
		"texlive_version": TeXLive,
	}, "", "  ")
	return append(data, '\n'), err
}

// sty returns the m2.sty of the output: the copy in the tree, else the one that the notation register gives, else one from the KaTeX macros.
func (d *Document) sty() []byte {
	if d.tree.sty != nil {
		return d.tree.sty
	}
	if d.tree.reg != nil {
		return d.tree.reg.Sty()
	}
	var b strings.Builder
	b.WriteString("% The LaTeX macros that a node body can use, from macros/m2.katex.json.\n\\NeedsTeXFormat{LaTeX2e}\n\\ProvidesPackage{m2}\n\\RequirePackage{amsmath}\n\\RequirePackage{amssymb}\n")
	for _, m := range slices.Sorted(maps.Keys(d.tree.macros)) {
		b.WriteString(`\providecommand{` + m + `}` + arity(d.tree.macros[m]) + `{` + d.tree.macros[m] + "}\n")
	}
	return []byte(b.String())
}

var argRef = regexp.MustCompile(`#([1-9])`)

// arity returns the argument count of a macro body for \newcommand, for example [2], or "".
func arity(body string) string {
	n := 0
	for _, m := range argRef.FindAllStringSubmatch(body, -1) {
		n = max(n, int(m[1][0]-'0'))
	}
	if n == 0 {
		return ""
	}
	return "[" + strconv.Itoa(n) + "]"
}

// bib returns main.bib: one entry for each source, with only what the identifier gives.
func (d *Document) bib() string {
	var b strings.Builder
	b.WriteString("% m2 pack wrote this file from the sources of the nodes. Do not change it by hand.\n")
	for _, s := range d.Sources {
		fmt.Fprintf(&b, "\n@misc{%s,\n  title = {{%s}},\n", s.Key, texText(s.Title))
		switch s.System {
		case "doi":
			fmt.Fprintf(&b, "  doi = {%s},\n", texURL(s.ID))
		case "arxiv":
			fmt.Fprintf(&b, "  eprint = {%s},\n  archiveprefix = {arXiv},\n", texText(s.ID))
		case "book":
			if isbn, ok := strings.CutPrefix(s.ID, "isbn:"); ok {
				fmt.Fprintf(&b, "  isbn = {%s},\n", texText(isbn))
			}
		}
		if s.URL != "" {
			fmt.Fprintf(&b, "  howpublished = {\\url{%s}},\n", texURL(s.URL))
		}
		b.WriteString("}\n")
	}
	return b.String()
}

// texWriter writes main.tex.
type texWriter struct {
	d   *Document
	b   *strings.Builder
	src []byte // the source of the Markdown that the writer is in
}

// class returns the document class of the kind of volume, and the commands of a chapter and of a section.
func (w *texWriter) class() (class, chapter, section string) {
	switch w.d.Volume.Kind {
	case "article", "survey":
		return "article", "section", "subsection"
	case "notes":
		return "report", "chapter", "section"
	}
	return "book", "chapter", "section"
}

func (w *texWriter) printf(format string, args ...any) { fmt.Fprintf(w.b, format, args...) }

func (w *texWriter) main() string {
	d, v := w.d, w.d.Volume
	class, chapter, section := w.class()
	w.printf("%% m2 pack wrote this file from the volume %s of math2. Do not change it by hand.\n", v.ID)
	w.printf("\\documentclass[11pt]{%s}\n", class)
	w.printf("\\usepackage[utf8]{inputenc}\n\\usepackage[T1]{fontenc}\n\\usepackage{amsmath}\n\\usepackage{amssymb}\n\\usepackage{amsthm}\n\\usepackage{m2}\n")
	for _, u := range d.Notation {
		if u.Variant != "" {
			args := ""
			if u.Args > 0 {
				args = "[" + strconv.Itoa(u.Args) + "]"
			}
			w.printf("\\providecommand{%s}{}\\renewcommand{%s}%s{%s}\n", u.Macro, u.Macro, args, u.LaTeX)
		}
	}
	w.printf("\\usepackage[hidelinks]{hyperref}\n")
	w.printf("\\newcommand{\\mtwohead}{}\n")
	for _, style := range []string{"plain", "definition", "remark"} {
		w.printf("\\theoremstyle{%s}\\newtheorem*{mtwo%s}{\\mtwohead}\n", style, style)
	}
	w.printf("\\title{%s}\n\\author{%s}\n\\date{math2 release %s}\n", texText(v.Title), texList(v.Editors, ` \and `), texText(d.Release))
	w.printf("\\begin{document}\n\\maketitle\n\\tableofcontents\n")
	if len(d.Preface) > 0 {
		w.unnumbered(chapter, "Preface", "preface")
		for _, g := range d.Preface {
			w.markdown(g.Text)
		}
	}
	if len(d.Assumed) > 0 {
		w.printf("\n\\noindent The reader knows these nodes:\n\\begin{itemize}\n")
		for _, r := range d.Assumed {
			w.printf("\\item \\href{%s}{%s}, \\texttt{%s}\n", texURL(r.URL), texText(r.Name), texText(r.URI))
		}
		w.printf("\\end{itemize}\n")
	}
	for _, c := range d.Chapters {
		w.printf("\n\\setcounter{%s}{%d}\n\\%s{%s}\\label{%s}\n", chapter, c.Number-1, chapter, texText(c.Title), texLabel(c.ID))
		for _, g := range c.Glue {
			w.markdown(g.Text)
		}
		for si, s := range c.Sections {
			w.printf("\n\\setcounter{%s}{%d}\n\\%s{%s}\\label{%s}\n", section, si, section, texText(s.Title), texLabel(s.ID))
			for _, g := range s.Glue {
				w.markdown(g.Text)
			}
			for _, it := range s.Items {
				w.item(it)
			}
		}
	}
	w.back(chapter)
	w.printf("\n\\end{document}\n")
	return w.b.String()
}

// unnumbered starts a part with no number that the contents list.
func (w *texWriter) unnumbered(cmd, title, label string) {
	w.printf("\n\\%s*{%s}\\label{%s}\n\\addcontentsline{toc}{%s}{%s}\n", cmd, title, label, cmd, title)
}

// envStyle returns the amsthm style of an item.
func envStyle(it *Item) string {
	switch it.Kind {
	case "statement", "conjecture":
		return "plain"
	}
	return "definition"
}

func (w *texWriter) item(it *Item) {
	env := "mtwo" + envStyle(it)
	note := it.Name
	if it.Claim != "" {
		note = strings.TrimPrefix(note+", "+it.Claim, ", ")
	}
	if note != "" {
		note = "[{" + texText(note) + "}]"
	}
	w.printf("\n\\phantomsection\\label{%s}\n\\renewcommand{\\mtwohead}{%s %s}\n\\begin{%s}%s\n", texLabel(it.anchor()), texText(it.Label), it.Number, env, note)
	if it.Characterizes != "" {
		w.printf("An equivalent characterization of the definition in %s.\n\n", w.ref(it.Characterizes, it.Characterizes))
	}
	w.markdown(it.Main)
	w.printf("\\end{%s}\n", env)
	if it.Remarks != "" {
		w.printf("\n\\noindent\\textit{Remarks.}\n")
		w.markdown(it.Remarks)
	}
	for _, p := range it.Proofs {
		w.printf("\n\\begin{proof}\\phantomsection\\label{%s}\n", texLabel(p.anchor()))
		w.markdown(p.Main)
		w.printf("\\end{proof}\n")
	}
	if len(it.Sources) > 0 {
		w.printf("\n\\noindent{\\small Sources: ")
		for i, c := range it.Sources {
			if i > 0 {
				w.printf("; ")
			}
			if c.Locator != "" {
				w.printf("\\cite[%s]{%s}", texText(c.Locator), c.Key)
			} else {
				w.printf("\\cite{%s}", c.Key)
			}
		}
		w.printf(".}\n")
	}
	w.printf("\n\\noindent{\\footnotesize\\href{%s}{\\texttt{m2://%s}}}\n", texURL(w.d.ref(it.Tag).URL), it.Tag)
}

// ref returns a link to a node: to its item when the volume has it, else to its page on the site.
func (w *texWriter) ref(tag, text string) string {
	if it := w.d.homes[tag]; it != nil {
		return `\hyperref[` + texLabel(it.anchor()) + `]{` + text + `}`
	}
	if w.d.tree.nodes[tag] != nil {
		return `\href{` + texURL(w.d.ref(tag).URL) + `}{` + text + `}`
	}
	return text
}

// back writes the conventions, the notation, the cited nodes, the sources and the attribution.
func (w *texWriter) back(chapter string) {
	d := w.d
	if len(d.Conventions) > 0 {
		w.unnumbered(chapter, "Conventions", "conventions")
		w.printf("\\begin{description}\n")
		for _, c := range d.Conventions {
			w.printf("\\item[%s] This volume uses %s", texText(c.ID), texText(c.Option))
			if c.Default {
				w.printf(", the default of math2")
			}
			w.printf(".")
			for _, x := range c.Differ {
				w.printf(" %s uses %s", w.ref(x.Tag, texText(x.Name)), texText(x.Option))
				if x.Note != "" {
					w.printf(": %s", texText(x.Note))
				} else {
					w.printf(".")
				}
			}
			w.printf("\n")
		}
		w.printf("\\end{description}\n")
	}
	if len(d.Notation) > 0 {
		w.unnumbered(chapter, "Notation", "notation")
		w.printf("\\begin{tabular}{l p{0.55\\textwidth} l}\n\\textbf{Symbol} & \\textbf{Reads} & \\textbf{Introduced} \\\\\n\\hline\n")
		for _, u := range d.Notation {
			w.printf("$%s$ & %s & %s \\\\\n", notationTeX(u), texText(u.Reads), texText(u.Introduced))
		}
		w.printf("\\end{tabular}\n")
	}
	if len(d.Cited) > 0 {
		w.unnumbered(chapter, "Cited nodes", "cited")
		w.printf("\\begin{itemize}\n")
		for _, r := range d.Cited {
			w.printf("\\item \\href{%s}{%s}, \\texttt{%s}\n", texURL(r.URL), texText(r.Name), texText(r.URI))
		}
		w.printf("\\end{itemize}\n")
	}
	if len(d.Sources) > 0 {
		name := `\bibname`
		if chapter == "section" {
			name = `\refname`
		}
		w.printf("\n\\renewcommand{%s}{Sources}\n\\begin{thebibliography}{%d}\n", name, len(d.Sources))
		for _, s := range d.Sources {
			w.printf("\\bibitem{%s} %s", s.Key, texText(s.Title))
			if s.URL != "" {
				w.printf(", \\url{%s}", texURL(s.URL))
			}
			w.printf(".\n")
		}
		w.printf("\\end{thebibliography}\n")
	}
	w.unnumbered(chapter, "Attribution", "attribution")
	w.printf("%s\n", texText(licence))
	a := d.Attribution
	if len(a.People) > 0 {
		w.printf("\n\\begin{itemize}\n")
		for _, c := range a.People {
			w.printf("\\item %s\n", texText(c.String()))
		}
		w.printf("\\end{itemize}\n")
	}
	if len(a.Agents) > 0 {
		w.printf("\n\\noindent Agent runs that helped:\n\\begin{itemize}\n")
		for _, r := range a.Agents {
			w.printf("\\item %s\n", texText(r))
		}
		w.printf("\\end{itemize}\n")
	}
	if len(a.NoLog) > 0 {
		var refs []string
		for _, t := range a.NoLog {
			refs = append(refs, w.ref(t, t))
		}
		w.printf("\n\\noindent These nodes have no provenance log: %s.\n", strings.Join(refs, ", "))
	}
}

// markdown writes a body or a part of the glue.
func (w *texWriter) markdown(src string) {
	if strings.TrimSpace(src) == "" {
		return
	}
	doc, source := markup.Parse(src)
	w.src = source
	w.blocks(doc)
}

func (w *texWriter) blocks(parent ast.Node) {
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		w.block(n)
	}
}

func (w *texWriter) block(n ast.Node) {
	switch n := n.(type) {
	case *ast.Paragraph:
		if m, ok := n.FirstChild().(*markup.Math); ok && n.ChildCount() == 1 && m.Display {
			w.printf("\\[\n%s\n\\]\n", strings.TrimSpace(string(m.TeX)))
			return
		}
		w.inlines(n)
		w.printf("\n\n")
	case *ast.TextBlock:
		w.inlines(n)
		w.printf("\n")
	case *ast.Heading:
		w.printf("\\par\\medskip\\noindent\\textbf{")
		w.inlines(n)
		w.printf("}\\par\n")
	case *ast.ThematicBreak:
		w.printf("\\par\\medskip\\noindent\\hrulefill\\par\n")
	case *ast.CodeBlock, *ast.FencedCodeBlock:
		w.printf("\\begin{verbatim}\n")
		lines := n.Lines()
		for i := range lines.Len() {
			seg := lines.At(i)
			w.printf("%s", strings.ReplaceAll(string(seg.Value(w.src)), `\end{verbatim}`, `\end {verbatim}`))
		}
		w.printf("\\end{verbatim}\n")
	case *ast.Blockquote:
		w.printf("\\begin{quote}\n")
		w.blocks(n)
		w.printf("\\end{quote}\n")
	case *ast.List:
		env := "itemize"
		if n.IsOrdered() {
			env = "enumerate"
		}
		w.printf("\\begin{%s}\n", env)
		if n.IsOrdered() && n.Start > 1 {
			w.printf("\\setcounter{enumi}{%d}\n", n.Start-1)
		}
		for li := n.FirstChild(); li != nil; li = li.NextSibling() {
			w.printf("\\item ")
			w.blocks(li)
		}
		w.printf("\\end{%s}\n", env)
	case *east.Table:
		w.printf("\\begin{tabular}{%s}\n", strings.Repeat("l", len(n.Alignments)))
		for row := n.FirstChild(); row != nil; row = row.NextSibling() {
			for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
				if cell != row.FirstChild() {
					w.printf(" & ")
				}
				w.inlines(cell)
			}
			w.printf(" \\\\\n")
			if _, ok := row.(*east.TableHeader); ok {
				w.printf("\\hline\n")
			}
		}
		w.printf("\\end{tabular}\n\n")
	case *east.FootnoteList, *ast.HTMLBlock:
		// The footnotes are at their links. The reviewed HTML of a body has no LaTeX form.
	default:
		w.blocks(n)
	}
}

func (w *texWriter) inlines(parent ast.Node) {
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		w.inline(n)
	}
}

func (w *texWriter) inline(n ast.Node) {
	switch n := n.(type) {
	case *ast.Text:
		w.printf("%s", texText(string(n.Segment.Value(w.src))))
		switch {
		case n.HardLineBreak():
			w.printf("\\\\\n")
		case n.SoftLineBreak():
			w.printf("\n")
		}
	case *ast.String:
		w.printf("%s", texText(string(n.Value)))
	case *markup.Math:
		if n.Display {
			w.printf("\\[%s\\]", n.TeX)
		} else {
			w.printf("$%s$", n.TeX)
		}
	case *ast.CodeSpan:
		w.printf("\\texttt{")
		w.inlines(n)
		w.printf("}")
	case *ast.Emphasis:
		cmd := `\emph{`
		if n.Level > 1 {
			cmd = `\textbf{`
		}
		w.printf("%s", cmd)
		w.inlines(n)
		w.printf("}")
	case *ast.Link:
		saved := w.b
		w.b = &strings.Builder{}
		w.inlines(n)
		text := w.b.String()
		w.b = saved
		w.printf("%s", w.link(string(n.Destination), text))
	case *ast.AutoLink:
		u := string(n.URL(w.src))
		w.printf("\\href{%s}{%s}", texURL(u), texText(string(n.Label(w.src))))
	case *east.FootnoteLink:
		w.printf("\\footnote{")
		if fn := footnote(n); fn != nil {
			for p := fn.FirstChild(); p != nil; p = p.NextSibling() {
				if p != fn.FirstChild() {
					w.printf("\\par ")
				}
				w.inlines(p)
			}
		}
		w.printf("}")
	case *ast.RawHTML, *east.FootnoteBacklink:
	default:
		w.inlines(n)
	}
}

// link returns a link of a body in LaTeX: to an item of the volume, to the page of a node outside it, or to a URL.
func (w *texWriter) link(dest, text string) string {
	t, ok := strings.CutPrefix(dest, markup.NodeScheme)
	if !ok {
		t, ok = strings.CutPrefix(dest, "m2://")
	}
	if ok {
		return w.ref(t, text)
	}
	return `\href{` + texURL(dest) + `}{` + text + `}`
}

// footnote returns the footnote that a footnote link goes to.
func footnote(l *east.FootnoteLink) *east.Footnote {
	root := ast.Node(l)
	for root.Parent() != nil {
		root = root.Parent()
	}
	var out *east.Footnote
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if fn, ok := n.(*east.Footnote); ok && entering && fn.Index == l.Index {
			out = fn
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return out
}

// texChars are the LaTeX forms of the characters that are not text in LaTeX, and of the characters that the T1 fonts of pdflatex do not have.
var texChars = map[rune]string{
	'\\': `\textbackslash{}`, '{': `\{`, '}': `\}`, '$': `\$`, '&': `\&`, '#': `\#`, '^': `\textasciicircum{}`, '_': `\_`, '%': `\%`, '~': `\textasciitilde{}`,
	'<': `\textless{}`, '>': `\textgreater{}`, '|': `\textbar{}`,
	'−': `\ensuremath{-}`, '×': `\ensuremath{\times}`, '·': `\ensuremath{\cdot}`, '→': `\ensuremath{\to}`, '←': `\ensuremath{\leftarrow}`, '↔': `\ensuremath{\leftrightarrow}`,
	'⇒': `\ensuremath{\Rightarrow}`, '⇔': `\ensuremath{\Leftrightarrow}`, '≤': `\ensuremath{\le}`, '≥': `\ensuremath{\ge}`, '≠': `\ensuremath{\ne}`, '≈': `\ensuremath{\approx}`,
	'∈': `\ensuremath{\in}`, '∉': `\ensuremath{\notin}`, '⊂': `\ensuremath{\subset}`, '⊆': `\ensuremath{\subseteq}`, '∪': `\ensuremath{\cup}`, '∩': `\ensuremath{\cap}`,
	'∞': `\ensuremath{\infty}`, '∑': `\ensuremath{\sum}`, '∏': `\ensuremath{\prod}`, '√': `\ensuremath{\surd}`, '∀': `\ensuremath{\forall}`, '∃': `\ensuremath{\exists}`,
	'∅': `\ensuremath{\emptyset}`, '∘': `\ensuremath{\circ}`, '∎': `\ensuremath{\blacksquare}`, '□': `\ensuremath{\square}`,
}

// greek are the Greek letters, in the order of the code points from α.
var greek = []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta", "iota", "kappa", "lambda", "mu", "nu", "xi", "o", "pi", "rho", "varsigma", "sigma", "tau", "upsilon", "phi", "chi", "psi", "omega"}

// texText returns text in LaTeX. A character that pdflatex cannot set shows as its code point.
func texText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case texChars[r] != "":
			b.WriteString(texChars[r])
		case r < utf8.RuneSelf, r >= 0xA0 && r <= 0x17F, strings.ContainsRune("–—‘’“”…•†‡€", r):
			b.WriteRune(r)
		case r == 'ο':
			b.WriteRune('o')
		case r >= 'α' && r <= 'ω':
			b.WriteString(`\ensuremath{\` + greek[r-'α'] + `}`)
		default:
			fmt.Fprintf(&b, `\texttt{U+%04X}`, r)
		}
	}
	return b.String()
}

// texList returns the items in LaTeX, joined by sep.
func texList(items []string, sep string) string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = texText(s)
	}
	return strings.Join(out, sep)
}

// texURL returns a URL for \href and \url, with the characters that LaTeX reads escaped.
func texURL(u string) string {
	return strings.NewReplacer(`%`, `\%`, `#`, `\#`, `\`, `\%5C`, `{`, `\%7B`, `}`, `\%7D`).Replace(u)
}

// texLabel returns an id as a LaTeX label.
func texLabel(id string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.' || r == ':' {
			return r
		}
		return '-'
	}, id)
}
