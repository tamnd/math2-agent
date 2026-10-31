package textbook

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// elements are the kinds of the PreTeXt elements that give items.
var elements = map[string]string{
	"theorem": "statement", "lemma": "statement", "proposition": "statement", "corollary": "statement", "claim": "statement",
	"fact": "statement", "identity": "statement", "axiom": "statement", "principle": "statement", "assumption": "statement",
	"conjecture": "conjecture", "hypothesis": "conjecture",
	"definition": "definition",
	"example":    "example", "question": "example", "problem": "example",
	"exercise": "exercise", "activity": "exercise", "exploration": "exercise", "investigation": "exercise", "project": "exercise",
	"proof": "proof",
}

// divisions are the PreTeXt elements whose title is the section of the items in them.
var divisions = map[string]bool{
	"part": true, "chapter": true, "appendix": true, "section": true, "subsection": true, "subsubsection": true, "worksheet": true,
}

// unseen are the children of an item that are not its text.
var unseen = map[string]bool{"hint": true, "answer": true, "solution": true}

// hidden are the elements that are not read at all: the pictures, the code and the index entries. A picture can hold a PreFigure diagram, whose definition elements are not definitions of mathematics.
var hidden = map[string]bool{
	"image": true, "prefigure": true, "latex-image": true, "sageplot": true, "asymptote": true, "sage": true,
	"program": true, "console": true, "interactive": true, "video": true, "audio": true, "idx": true, "notation": true,
}

const (
	xinclude = "http://www.w3.org/2001/XInclude"
	xmlSpace = "http://www.w3.org/XML/1998/namespace"
	prefig   = "https://prefigure.org"
	para     = "\x00" // a break between paragraphs, in the text that a reader collects
)

// pretext reads the files of a PreTeXt book.
type pretext struct {
	files map[string][]byte
	open  map[string]bool // the files that are being read, against a cycle of includes
	items []item
	divs  []string // the titles of the open divisions
	toc   *toc     // the table of contents, when the reader builds one
}

// readPreTeXt gives the items of the main file of a PreTeXt book and of the files that it includes, in the order of the book.
func readPreTeXt(files map[string][]byte, main string) ([]item, error) {
	if _, ok := files[main]; !ok {
		return nil, fmt.Errorf("the main file %s is not in the archive", main)
	}
	p := &pretext{files: files, open: map[string]bool{}, toc: &toc{keys: map[string]bool{}}}
	if err := p.file(main, nil); err != nil {
		return nil, err
	}
	return p.items, nil
}

// sink collects the text and the references of an item.
type sink struct {
	b    *strings.Builder
	refs *[]string
}

func (w *sink) write(s string) {
	if w != nil && w.b != nil {
		w.b.WriteString(s)
	}
}

// file reads one file. A file that an include names and the archive does not have is not read.
func (p *pretext) file(name string, w *sink) error {
	data, ok := p.files[name]
	if !ok || p.open[name] {
		return nil
	}
	p.open[name] = true
	defer delete(p.open, name)
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Entity = xml.HTMLEntity
	if err := p.children(d, name, w, nil); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// children reads the content of an element up to its end, or a whole file. The function at is called for each child element, and when it is nil, node reads the child.
func (p *pretext) children(d *xml.Decoder, file string, w *sink, at func(xml.StartElement, int) error) error {
	for {
		line, _ := d.InputPos()
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if at != nil {
				err = at(t, line)
			} else {
				err = p.node(d, file, t, line, w)
			}
			if err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		case xml.CharData:
			w.write(string(t))
		}
	}
}

// node reads one element and its content.
func (p *pretext) node(d *xml.Decoder, file string, se xml.StartElement, line int, w *sink) error {
	name := se.Name.Local
	if name == "idx" && p.toc != nil && p.toc.index && attr(se, "", "component") == "" {
		return p.pretextTerm(d, file, line)
	}
	if attr(se, "", "component") != "" || hidden[name] || se.Name.Space == prefig {
		return d.Skip() // a variant for one platform, or not text
	}
	switch {
	case name == "include" && (se.Name.Space == xinclude || se.Name.Space == "xi"):
		if err := d.Skip(); err != nil {
			return err
		}
		if href := attr(se, "", "href"); href != "" && attr(se, "", "parse") != "text" {
			return p.file(path.Join(path.Dir(file), href), w)
		}
		return nil
	case divisions[name]:
		p.divs = append(p.divs, "")
		defer func() { p.divs = p.divs[:len(p.divs)-1] }()
		var head *tocNode // the node of the division in the table of contents
		if _, ok := levels[name]; p.toc != nil && (ok || name == "appendix") {
			head = p.toc.add(name, false, "", cmpOr(attr(se, xmlSpace, "id"), attr(se, "", "label")))
			defer p.toc.close(head)
		}
		return p.children(d, file, w, func(c xml.StartElement, line int) error {
			if c.Name.Local == "title" && p.divs[len(p.divs)-1] == "" {
				var b strings.Builder
				err := p.children(d, file, &sink{b: &b, refs: new([]string)}, nil)
				p.divs[len(p.divs)-1] = clean(b.String())
				if head != nil && head.Title == "" {
					head.Title = strings.Join(strings.Fields(p.divs[len(p.divs)-1]), " ")
				}
				return err
			}
			return p.node(d, file, c, line, w)
		})
	case elements[name] != "":
		return p.item(d, file, se, line)
	}
	switch name {
	case "m":
		w.write("$")
		defer w.write("$")
	case "me", "men", "md", "mdn":
		w.write(" $$")
		defer w.write("$$ ")
	case "mrow":
		defer w.write(` \\ `)
	case "p", "li", "statement", "task", "introduction", "conclusion", "blockquote":
		w.write(para)
		defer w.write(para)
	case "xref":
		if w != nil && w.refs != nil {
			for _, a := range []string{"ref", "first", "last"} {
				*w.refs = append(*w.refs, strings.Fields(strings.ReplaceAll(attr(se, "", a), ",", " "))...)
			}
		}
		if ref := attr(se, "", "ref"); ref != "" {
			w.write("[" + ref + "]")
		}
	}
	return p.children(d, file, w, nil)
}

// item reads an element that gives an item. Its text is its statement, or when it has none, all its content but its title, hints, answers and solutions. An item in it is an item of its own.
func (p *pretext) item(d *xml.Decoder, file string, se xml.StartElement, line int) error {
	i := len(p.items)
	p.items = append(p.items, item{
		file:  file,
		env:   se.Name.Local,
		kind:  elements[se.Name.Local],
		label: cmpOr(attr(se, xmlSpace, "id"), attr(se, "", "label")),
		line:  line,
	})
	if p.toc != nil {
		p.items[i].node = p.toc.here()
	}
	for j := len(p.divs) - 1; j >= 0; j-- {
		if p.divs[j] != "" {
			p.items[i].section = p.divs[j]
			break
		}
	}
	var title, statement, all strings.Builder
	var refs []string
	err := p.children(d, file, &sink{b: &all, refs: &refs}, func(c xml.StartElement, line int) error {
		switch name := c.Name.Local; {
		case name == "title":
			return p.children(d, file, &sink{b: &title, refs: &refs}, nil)
		case name == "statement":
			return p.node(d, file, c, line, &sink{b: &statement, refs: &refs})
		case unseen[name]:
			return p.node(d, file, c, line, &sink{refs: new([]string)})
		}
		return p.node(d, file, c, line, &sink{b: &all, refs: &refs})
	})
	it := &p.items[i]
	it.title = clean(title.String())
	it.text = cmpOr(clean(statement.String()), clean(all.String()))
	for _, r := range refs {
		if r != it.label {
			it.refs = append(it.refs, r)
		}
	}
	return err
}

// clean gives the collected text with the spaces of each paragraph collapsed and a blank line between paragraphs.
func clean(s string) string {
	var out []string
	for part := range strings.SplitSeq(s, para) {
		if t := strings.Join(strings.Fields(part), " "); t != "" {
			out = append(out, t)
		}
	}
	return strings.Join(out, "\n\n")
}

// attr gives the value of an attribute of an element, or "".
func attr(se xml.StartElement, space, local string) string {
	for _, a := range se.Attr {
		if a.Name.Local == local && (a.Name.Space == space || space == xmlSpace && a.Name.Space == "xml") {
			return a.Value
		}
	}
	return ""
}
