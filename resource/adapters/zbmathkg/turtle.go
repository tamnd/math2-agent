package zbmathkg

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// term is one term of a statement in Turtle: an IRI, a literal or a blank node with its own properties.
type term struct {
	iri   string
	lit   string
	isLit bool
	blank map[string][]term
}

// statement is one subject with its properties, as the object terms of each predicate IRI.
type statement struct {
	subject string
	props   map[string][]term
}

// first gives the first IRI or literal of the predicate p, or an empty string.
func (s statement) first(p string) string {
	for _, t := range s.props[p] {
		if t.isLit {
			return t.lit
		}
		if t.iri != "" {
			return t.iri
		}
	}
	return ""
}

// parser reads the subset of Turtle that the dumps of the zbMATH Open Knowledge Graph use: the prefix directives, IRIs, prefixed names, the keyword a, literals with a datatype or a language, and blank nodes in brackets. It does not read collections.
type parser struct {
	r        *bufio.Reader
	prefixes map[string]string
	peeked   *token
	line     int
}

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIRI
	tokLit
	tokPunct
	tokPrefix
)

type token struct {
	kind tokenKind
	val  string
}

func newParser(r io.Reader) *parser {
	return &parser{r: bufio.NewReaderSize(r, 1<<20), prefixes: map[string]string{}, line: 1}
}

func (p *parser) errorf(format string, args ...any) error {
	return fmt.Errorf("line %d: %s", p.line, fmt.Sprintf(format, args...))
}

// next reads the next statement. It returns io.EOF at the end of the input.
func (p *parser) next() (statement, error) {
	for {
		t, err := p.token()
		if err != nil {
			return statement{}, err
		}
		switch {
		case t.kind == tokEOF:
			return statement{}, io.EOF
		case t.kind == tokPrefix:
			if err := p.prefix(t.val); err != nil {
				return statement{}, err
			}
			continue
		case t.kind == tokPunct && t.val == "[":
			if _, err := p.props("]"); err != nil {
				return statement{}, err
			}
			if err := p.expect("."); err != nil {
				return statement{}, err
			}
			continue
		case t.kind != tokIRI:
			return statement{}, p.errorf("a statement starts with %q", t.val)
		}
		props, err := p.props(".")
		if err != nil {
			return statement{}, err
		}
		return statement{subject: t.val, props: props}, nil
	}
}

// prefix reads the rest of a prefix directive. A directive in the form of SPARQL has no full stop at the end.
func (p *parser) prefix(dir string) error {
	c, err := p.space()
	if err != nil {
		return p.errorf("a bad prefix directive")
	}
	p.unread(c)
	name, err := p.word()
	if err != nil {
		return err
	}
	t, err := p.token()
	if err != nil {
		return err
	}
	if t.kind != tokIRI || !strings.HasSuffix(name, ":") {
		return p.errorf("a bad prefix directive")
	}
	p.prefixes[strings.TrimSuffix(name, ":")] = t.val
	if dir == "@prefix" || dir == "@base" {
		return p.expect(".")
	}
	return nil
}

func (p *parser) expect(punct string) error {
	t, err := p.token()
	if err != nil {
		return err
	}
	if t.kind != tokPunct || t.val != punct {
		return p.errorf("want %q, got %q", punct, t.val)
	}
	return nil
}

// props reads a list of predicates and objects up to the punctuation end.
func (p *parser) props(end string) (map[string][]term, error) {
	out := map[string][]term{}
	for {
		t, err := p.token()
		if err != nil {
			return nil, err
		}
		if t.kind == tokPunct && t.val == end {
			return out, nil
		}
		if t.kind == tokPunct && t.val == ";" {
			continue
		}
		if t.kind != tokIRI {
			return nil, p.errorf("a predicate is %q", t.val)
		}
		pred := t.val
		for {
			o, err := p.object()
			if err != nil {
				return nil, err
			}
			out[pred] = append(out[pred], o)
			t, err := p.token()
			if err != nil {
				return nil, err
			}
			if t.kind == tokPunct && t.val == "," {
				continue
			}
			if t.kind == tokPunct && t.val == end {
				return out, nil
			}
			if t.kind != tokPunct || t.val != ";" {
				return nil, p.errorf("after an object, got %q", t.val)
			}
			break
		}
	}
}

func (p *parser) object() (term, error) {
	t, err := p.token()
	if err != nil {
		return term{}, err
	}
	switch {
	case t.kind == tokIRI:
		return term{iri: t.val}, nil
	case t.kind == tokLit:
		return term{lit: t.val, isLit: true}, nil
	case t.kind == tokPunct && t.val == "[":
		b, err := p.props("]")
		return term{blank: b}, err
	}
	return term{}, p.errorf("an object is %q", t.val)
}

func (p *parser) read() (byte, error) {
	c, err := p.r.ReadByte()
	if c == '\n' {
		p.line++
	}
	return c, err
}

func (p *parser) unread(c byte) {
	_ = p.r.UnreadByte()
	if c == '\n' {
		p.line--
	}
}

// space skips white space and comments. It returns io.EOF at the end of the input.
func (p *parser) space() (byte, error) {
	for {
		c, err := p.read()
		if err != nil {
			return 0, err
		}
		switch c {
		case ' ', '\t', '\r', '\n':
		case '#':
			if _, err := p.r.ReadString('\n'); err != nil {
				return 0, err
			}
			p.line++
		default:
			return c, nil
		}
	}
}

func (p *parser) token() (token, error) {
	if p.peeked != nil {
		t := *p.peeked
		p.peeked = nil
		return t, nil
	}
	c, err := p.space()
	if errors.Is(err, io.EOF) {
		return token{kind: tokEOF}, nil
	}
	if err != nil {
		return token{}, err
	}
	switch c {
	case '<':
		s, err := p.r.ReadString('>')
		if err != nil {
			return token{}, p.errorf("an IRI has no end")
		}
		return token{kind: tokIRI, val: s[:len(s)-1]}, nil
	case '"', '\'':
		s, err := p.literal(c)
		if err != nil {
			return token{}, err
		}
		return token{kind: tokLit, val: s}, p.suffix()
	case '[', ']', ',', ';', '.':
		return token{kind: tokPunct, val: string(c)}, nil
	}
	p.unread(c)
	w, err := p.word()
	if err != nil {
		return token{}, err
	}
	if strings.HasSuffix(w, ".") {
		w = strings.TrimSuffix(w, ".")
		p.peeked = &token{kind: tokPunct, val: "."}
	}
	switch {
	case w == "a":
		return token{kind: tokIRI, val: rdfType}, nil
	case w == "@prefix" || w == "@base" || strings.EqualFold(w, "prefix") || strings.EqualFold(w, "base"):
		if p.peeked != nil {
			return token{}, p.errorf("a bad directive")
		}
		return token{kind: tokPrefix, val: strings.ToLower(w)}, nil
	case w == "true" || w == "false" || (w != "" && (w[0] == '-' || w[0] == '+' || (w[0] >= '0' && w[0] <= '9'))):
		return token{kind: tokLit, val: w}, nil
	}
	i := strings.IndexByte(w, ':')
	if i < 0 {
		return token{}, p.errorf("the word %q is not a prefixed name", w)
	}
	base, ok := p.prefixes[w[:i]]
	if !ok {
		return token{}, p.errorf("the prefix of %q is not declared", w)
	}
	return token{kind: tokIRI, val: base + unescapeLocal(w[i+1:])}, nil
}

// word reads up to white space or punctuation that ends a term.
func (p *parser) word() (string, error) {
	var b strings.Builder
	for {
		c, err := p.read()
		if errors.Is(err, io.EOF) {
			return b.String(), nil
		}
		if err != nil {
			return "", err
		}
		switch c {
		case ' ', '\t', '\r', '\n', ',', ';', '[', ']', '"', '<':
			p.unread(c)
			return b.String(), nil
		case '\\':
			b.WriteByte(c)
			c, err = p.read()
			if err != nil {
				return "", err
			}
		}
		b.WriteByte(c)
	}
}

func unescapeLocal(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// literal reads a string after its first quote q, in the short or the long form.
func (p *parser) literal(q byte) (string, error) {
	long := false
	if c, err := p.read(); err != nil {
		return "", err
	} else if c != q {
		p.unread(c)
	} else if c2, err := p.read(); err == nil && c2 == q {
		long = true
	} else {
		if err == nil {
			p.unread(c2)
		}
		return "", nil
	}
	var b strings.Builder
	quotes := 0
	for {
		c, err := p.read()
		if err != nil {
			return "", p.errorf("a literal has no end")
		}
		if c == q {
			if !long {
				return b.String(), nil
			}
			quotes++
			if quotes == 3 {
				s := b.String()
				return s[:len(s)-2], nil
			}
			b.WriteByte(c)
			continue
		}
		quotes = 0
		if c == '\n' && !long {
			return "", p.errorf("a short literal has a new line")
		}
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		e, err := p.read()
		if err != nil {
			return "", err
		}
		switch e {
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'u', 'U':
			n := 4
			if e == 'U' {
				n = 8
			}
			hex := make([]byte, n)
			for i := range hex {
				if hex[i], err = p.read(); err != nil {
					return "", err
				}
			}
			r, err := strconv.ParseUint(string(hex), 16, 32)
			if err != nil || !utf8.ValidRune(rune(r)) {
				return "", p.errorf("a bad escape \\%c%s", e, hex)
			}
			b.WriteRune(rune(r))
		default:
			b.WriteByte(e)
		}
	}
}

// suffix skips the datatype or the language tag after a literal.
func (p *parser) suffix() error {
	c, err := p.read()
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	switch c {
	case '@':
		_, err := p.word()
		return err
	case '^':
		if c, err := p.read(); err != nil || c != '^' {
			return p.errorf("a bad datatype")
		}
		t, err := p.token()
		if err != nil {
			return err
		}
		if t.kind != tokIRI {
			return p.errorf("a bad datatype")
		}
		return nil
	}
	p.unread(c)
	return nil
}
