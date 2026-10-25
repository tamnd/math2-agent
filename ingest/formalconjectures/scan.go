package formalconjectures

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// decl is a theorem or lemma of a Lean file, with the attributes and the docstring before it.
type decl struct {
	name      string // the full name, with the namespaces
	line      int    // the line of the keyword theorem or lemma
	attrs     string // the text of the attribute blocks, without @[ and ]
	doc       string // the docstring, without /-- and -/
	statement string // the text after the name up to :=, on one line
}

// file is what scan finds in one Lean file.
type file struct {
	moduleDoc string // the first module docstring, without /-! and -/
	decls     []decl
}

// modifiers can come between the attributes of a declaration and its keyword.
var modifiers = map[string]bool{
	"private": true, "protected": true, "public": true, "noncomputable": true,
	"nonrec": true, "unsafe": true, "partial": true, "meta": true,
}

// scan reads the source of a Lean file. It does not elaborate the file. It skips comments and strings, follows namespace, section and end, and gives each theorem and lemma with its full name. A keyword counts only as the first word of a line, as the declarations of formal-conjectures are written.
func scan(src string) file {
	var (
		f        file
		ns       [][]string // the stack of namespaces and sections; a section has no components
		doc      string
		attrs    []string
		line     = 1
		lineHead = true // no word on this line yet
		afterMod bool   // the last word was a modifier at the head of a line
	)
	reset := func() { doc, attrs = "", nil }
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '\n':
			line++
			lineHead = true
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case strings.HasPrefix(src[i:], "/-"):
			end := blockEnd(src, i)
			text := src[i:end]
			line += strings.Count(text, "\n")
			switch {
			case strings.HasPrefix(text, "/-!") && f.moduleDoc == "":
				f.moduleDoc = strings.TrimSpace(strings.TrimSuffix(text[3:], "-/"))
			case strings.HasPrefix(text, "/--"):
				doc = strings.TrimSpace(strings.TrimSuffix(text[3:], "-/"))
			}
			i = end
		case strings.HasPrefix(src[i:], "--"):
			i = lineEnd(src, i)
		case c == '"':
			i = stringEnd(src, i)
			lineHead = false
		case strings.HasPrefix(src[i:], "@["):
			end := bracketEnd(src, i+1)
			text := src[i+2 : end-1]
			line += strings.Count(text, "\n")
			attrs = append(attrs, text)
			i = end
		case isIdentStart(src, i):
			end := identEnd(src, i)
			word := src[i:end]
			lead := lineHead || afterMod
			lineHead, afterMod = false, false
			i = end
			if !lead {
				continue
			}
			if modifiers[word] {
				afterMod = true
				continue
			}
			switch word {
			case "namespace":
				name, next := nextWord(src, i)
				ns = append(ns, strings.Split(name, "."))
				i = next
				reset()
			case "section":
				ns = append(ns, nil)
				reset()
			case "end":
				if len(ns) > 0 {
					ns = ns[:len(ns)-1]
				}
				reset()
			case "theorem", "lemma":
				name, next := nextWord(src, i)
				stmt, end := statement(src, next)
				if len(attrs) > 0 {
					f.decls = append(f.decls, decl{
						name:      fullName(ns, name),
						line:      line,
						attrs:     strings.Join(attrs, " "),
						doc:       doc,
						statement: stmt,
					})
				}
				line += strings.Count(src[i:end], "\n")
				i = end
				reset()
			default:
				reset()
			}
		default:
			_, size := utf8.DecodeRuneInString(src[i:])
			i += size
			lineHead = false
		}
	}
	return f
}

// fullName puts the namespaces before the name of a declaration.
func fullName(ns [][]string, name string) string {
	if n, ok := strings.CutPrefix(name, "_root_."); ok {
		return n
	}
	var parts []string
	for _, n := range ns {
		parts = append(parts, n...)
	}
	return strings.Join(append(parts, name), ".")
}

// blockEnd returns the end of the block comment at i. Block comments nest.
func blockEnd(src string, i int) int {
	depth := 0
	for j := i; j < len(src)-1; j++ {
		switch {
		case src[j] == '/' && src[j+1] == '-':
			depth++
			j++
		case src[j] == '-' && src[j+1] == '/':
			depth--
			j++
			if depth == 0 {
				return j + 1
			}
		}
	}
	return len(src)
}

// lineEnd returns the place of the next newline after i, or the end of src.
func lineEnd(src string, i int) int {
	if n := strings.IndexByte(src[i:], '\n'); n >= 0 {
		return i + n
	}
	return len(src)
}

// stringEnd returns the end of the string literal at i.
func stringEnd(src string, i int) int {
	for j := i + 1; j < len(src); j++ {
		switch src[j] {
		case '\\':
			j++
		case '"':
			return j + 1
		}
	}
	return len(src)
}

// bracketEnd returns the end of the bracket at i, with the brackets and strings in it.
func bracketEnd(src string, i int) int {
	depth := 0
	for j := i; j < len(src); j++ {
		switch src[j] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return j + 1
			}
		case '"':
			j = stringEnd(src, j) - 1
		}
	}
	return len(src)
}

func isIdentRune(r rune) bool {
	return r == '_' || r == '\'' || r == '!' || r == '?' || r == '.' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.No, r)
}

func isIdentStart(src string, i int) bool {
	r, _ := utf8.DecodeRuneInString(src[i:])
	return r == '_' || r == '«' || unicode.IsLetter(r)
}

// identEnd returns the end of the identifier at i. A part in « » can have any character.
func identEnd(src string, i int) int {
	for j := i; j < len(src); {
		r, size := utf8.DecodeRuneInString(src[j:])
		switch {
		case r == '«':
			if n := strings.IndexRune(src[j:], '»'); n >= 0 {
				j += n + len("»")
				continue
			}
			return len(src)
		case isIdentRune(r):
			j += size
		default:
			return j
		}
	}
	return len(src)
}

// nextWord returns the identifier after the spaces at i, and its end.
func nextWord(src string, i int) (string, int) {
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	end := identEnd(src, i)
	return src[i:end], end
}

var spaces = regexp.MustCompile(`\s+`)

// statement returns the text from i to the first := that is not in brackets, with each run of spaces as one space, and the place of the :=.
func statement(src string, i int) (string, int) {
	var b strings.Builder
	depth := 0
	for j := i; j < len(src); j++ {
		c := src[j]
		switch {
		case strings.HasPrefix(src[j:], "--"):
			j = lineEnd(src, j) - 1
			continue
		case strings.HasPrefix(src[j:], "/-"):
			j = blockEnd(src, j) - 1
			continue
		case c == '"':
			end := stringEnd(src, j)
			b.WriteString(src[j:end])
			j = end - 1
			continue
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
		case depth <= 0 && strings.HasPrefix(src[j:], ":="):
			return strings.TrimSpace(spaces.ReplaceAllString(b.String(), " ")), j
		}
		b.WriteByte(c)
	}
	return strings.TrimSpace(spaces.ReplaceAllString(b.String(), " ")), len(src)
}
