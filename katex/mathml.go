package katex

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// MathML returns the MathML element of a rendering of Render, for a browser that implements MathML Core. MathML Core draws mathvariant only as normal, so each letter and digit of a math alphabet, for example the R of \mathbb{R}, becomes its character in the Mathematical Alphanumeric Symbols block. A prime in a superscript, as in f', follows its base, because the prime glyph of a math font is already raised. Text in a math alphabet keeps its letters and gets the weight and the slant in a style. The result is "" when out has no MathML element.
func MathML(out string) string {
	i := strings.Index(out, "<math")
	j := strings.LastIndex(out, "</math>")
	if i < 0 || j < i {
		return ""
	}
	src := out[i : j+len("</math>")]
	root, err := parseXML(src)
	if err != nil {
		return src
	}
	modern(root)
	var b strings.Builder
	root.write(&b)
	return b.String()
}

// elem is an element of a MathML tree. A text node has no name.
type elem struct {
	name  string
	attrs []xml.Attr
	kids  []*elem
	text  string
}

func parseXML(src string) (*elem, error) {
	d := xml.NewDecoder(strings.NewReader(src))
	d.Strict = false
	d.Entity = xml.HTMLEntity
	top := &elem{name: "#root"}
	stack := []*elem{top}
	for {
		tok, err := d.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		cur := stack[len(stack)-1]
		switch t := tok.(type) {
		case xml.StartElement:
			e := &elem{name: t.Name.Local, attrs: t.Attr}
			cur.kids = append(cur.kids, e)
			stack = append(stack, e)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			cur.kids = append(cur.kids, &elem{text: string(t)})
		}
	}
	if len(top.kids) != 1 || top.kids[0].name != "math" {
		return nil, errors.New("katex: no math element")
	}
	return top.kids[0], nil
}

func (e *elem) write(b *strings.Builder) {
	if e.name == "" {
		_ = xml.EscapeText(b, []byte(e.text))
		return
	}
	b.WriteString("<" + e.name)
	for _, a := range e.attrs {
		name := a.Name.Local
		if a.Name.Space == "xmlns" {
			name = "xmlns:" + name
		}
		b.WriteString(" " + name + `="`)
		_ = xml.EscapeText(b, []byte(a.Value))
		b.WriteString(`"`)
	}
	b.WriteString(">")
	for _, k := range e.kids {
		k.write(b)
	}
	b.WriteString("</" + e.name + ">")
}

func (e *elem) attr(name string) (string, int) {
	for i, a := range e.attrs {
		if a.Name.Local == name && a.Name.Space == "" {
			return a.Value, i
		}
	}
	return "", -1
}

// elems returns the element children of e, without the text nodes.
func (e *elem) elems() []*elem {
	var out []*elem
	for _, k := range e.kids {
		if k.name != "" {
			out = append(out, k)
		}
	}
	return out
}

// textOf returns the text of e and its children.
func (e *elem) textOf() string {
	if e.name == "" {
		return e.text
	}
	var s strings.Builder
	for _, k := range e.kids {
		s.WriteString(k.textOf())
	}
	return s.String()
}

func modern(e *elem) {
	for _, k := range e.kids {
		modern(k)
	}
	switch e.name {
	case "mi", "mn", "mo":
		v, i := e.attr("mathvariant")
		if i < 0 || v == "normal" {
			return
		}
		ok := true
		for _, k := range e.kids {
			if k.name != "" {
				ok = false
				continue
			}
			s, all := alphabet(k.text, v)
			k.text = s
			ok = ok && all
		}
		if ok {
			e.attrs = append(e.attrs[:i], e.attrs[i+1:]...)
		}
	case "mtext":
		v, i := e.attr("mathvariant")
		if i < 0 {
			return
		}
		style := ""
		if strings.Contains(v, "bold") {
			style += "font-weight:bold;"
		}
		if strings.Contains(v, "italic") {
			style += "font-style:italic;"
		}
		if strings.Contains(v, "monospace") {
			style += "font-family:monospace;"
		}
		e.attrs = append(e.attrs[:i], e.attrs[i+1:]...)
		if style != "" {
			e.attrs = append(e.attrs, xml.Attr{Name: xml.Name{Local: "style"}, Value: style})
		}
	case "msup":
		k := e.elems()
		if len(k) == 2 && primes(k[1]) {
			e.name = "mrow"
		}
	}
}

// primes tells if e is one or more primes and nothing else.
func primes(e *elem) bool {
	t := strings.TrimSpace(e.textOf())
	return t != "" && strings.Trim(t, "′″‴") == ""
}

// alphabet returns s in the math alphabet v, and whether each letter and digit of s has a character in it.
func alphabet(s, v string) (string, bool) {
	a, ok := alphabets[v]
	if !ok {
		return s, false
	}
	all := true
	var b strings.Builder
	for _, r := range s {
		m := a.letter(r)
		if m == 0 {
			if isAlnum(r) {
				all = false
			}
			b.WriteRune(r)
			continue
		}
		b.WriteRune(m)
	}
	return b.String(), all
}

func isAlnum(r rune) bool {
	return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r >= 0x391 && r <= 0x3C9
}

// alpha is a math alphabet: the first capital Latin letter, the first digit and the first capital and small Greek letters, with 0 for none, and the letters that are elsewhere in Unicode.
type alpha struct {
	latin, digit, greek, greekSmall rune
	holes                           map[rune]rune
}

func (a alpha) letter(r rune) rune {
	if m, ok := a.holes[r]; ok {
		return m
	}
	switch {
	case r >= 'A' && r <= 'Z' && a.latin != 0:
		return a.latin + r - 'A'
	case r >= 'a' && r <= 'z' && a.latin != 0:
		return a.latin + 26 + r - 'a'
	case r >= '0' && r <= '9' && a.digit != 0:
		return a.digit + r - '0'
	case r >= 0x391 && r <= 0x3A9 && a.greek != 0:
		return a.greek + r - 0x391
	case r >= 0x3B1 && r <= 0x3C9 && a.greekSmall != 0:
		return a.greekSmall + r - 0x3B1
	}
	return 0
}

var alphabets = map[string]alpha{
	"bold":                   {latin: 0x1D400, digit: 0x1D7CE, greek: 0x1D6A8, greekSmall: 0x1D6C2},
	"italic":                 {latin: 0x1D434, greek: 0x1D6E2, greekSmall: 0x1D6FC, holes: map[rune]rune{'h': 0x210E}},
	"bold-italic":            {latin: 0x1D468, greek: 0x1D71C, greekSmall: 0x1D736},
	"script":                 {latin: 0x1D49C, holes: map[rune]rune{'B': 0x212C, 'E': 0x2130, 'F': 0x2131, 'H': 0x210B, 'I': 0x2110, 'L': 0x2112, 'M': 0x2133, 'R': 0x211B, 'e': 0x212F, 'g': 0x210A, 'o': 0x2134}},
	"bold-script":            {latin: 0x1D4D0},
	"fraktur":                {latin: 0x1D504, holes: map[rune]rune{'C': 0x212D, 'H': 0x210C, 'I': 0x2111, 'R': 0x211C, 'Z': 0x2128}},
	"double-struck":          {latin: 0x1D538, digit: 0x1D7D8, holes: map[rune]rune{'C': 0x2102, 'H': 0x210D, 'N': 0x2115, 'P': 0x2119, 'Q': 0x211A, 'R': 0x211D, 'Z': 0x2124}},
	"bold-fraktur":           {latin: 0x1D56C},
	"sans-serif":             {latin: 0x1D5A0, digit: 0x1D7E2},
	"bold-sans-serif":        {latin: 0x1D5D4, digit: 0x1D7EC, greek: 0x1D756, greekSmall: 0x1D770},
	"sans-serif-italic":      {latin: 0x1D608},
	"sans-serif-bold-italic": {latin: 0x1D63C, greek: 0x1D790, greekSmall: 0x1D7AA},
	"monospace":              {latin: 0x1D670, digit: 0x1D7F6},
}
