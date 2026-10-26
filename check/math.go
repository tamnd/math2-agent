package check

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/tamnd/math2-agent/node"
)

// math checks each math span of the body (rule B01). It reports the first problem in each span. The scan stops at a span that is not closed, because the rest of the body has no clear meaning after it.
func (r *run) math(n *node.Node, b *body) {
	text := strings.Join(b.masked, "\n")
	place := b.offsets()
	report := func(off int, msg string) {
		line, col := place(off)
		r.add("B01", n.Path, line, col, msg)
	}
	mathSpans(text, func(from, to int) {
		r.checked["B01"]++
		if at, msg := checkTeX(text[from:to], r.macros); msg != "" {
			report(from+at, msg)
		}
	}, func(at int, display bool) {
		r.checked["B01"]++
		if display {
			report(at, "a display math span is not closed")
		} else {
			report(at, "an inline math span is not closed on its line")
		}
	})
}

// mathSpans calls span with the offsets of the TeX source of each math span of text, and unclosed with the offset of each span that is not closed. The scan stops after a display span that is not closed, and it goes on at the next line after an inline span that is not closed.
func mathSpans(text string, span func(from, to int), unclosed func(at int, display bool)) {
	for i := 0; i < len(text); i++ {
		switch {
		case text[i] == '\\':
			i++
		case strings.HasPrefix(text[i:], "$$"):
			end := strings.Index(text[i+2:], "$$")
			if end < 0 {
				unclosed(i, true)
				return
			}
			span(i+2, i+2+end)
			i += end + 3
		case text[i] == '$':
			end := closeInline(text, i+1)
			if end < 0 {
				unclosed(i, false)
				i += strings.IndexByte(text[i:]+"\n", '\n')
				continue
			}
			span(i+1, end)
			i = end
		}
	}
}

// Command is one use of a control word in a math span of a body.
type Command struct {
	Name string // the name with no backslash, for example ip
	Line int    // the line in the file
	Col  int    // the column in the file, in runes
}

// Commands returns the control words in the math spans of the body of n, in body order. Code spans and code blocks are not math, and a span that is not closed gives no commands.
func Commands(n *node.Node) []Command {
	b := parseBody(n)
	text := strings.Join(b.masked, "\n")
	place := b.offsets()
	var out []Command
	mathSpans(text, func(from, to int) {
		for i := from; i < to; i++ {
			if text[i] != '\\' {
				continue
			}
			name, next := command(text, i)
			if name != "" && isLetter(rune(name[0])) {
				line, col := place(i)
				out = append(out, Command{Name: name, Line: line, Col: col})
			}
			i = next - 1
		}
	}, func(int, bool) {})
	return out
}

// closeInline returns the offset of the $ that closes the inline span that starts at from, or -1 when the line has no such $.
func closeInline(text string, from int) int {
	for j := from; j < len(text); j++ {
		switch text[j] {
		case '\\':
			j++
		case '\n':
			return -1
		case '$':
			return j
		}
	}
	return -1
}

// defines are the commands that define or change a macro. A body must not use them, because the macros are in macros/m2.sty.
var defines = map[string]string{
	"newcommand": "defines", "providecommand": "defines", "def": "defines", "gdef": "defines", "edef": "defines", "xdef": "defines", "let": "defines",
	"DeclareMathOperator": "defines", "newenvironment": "defines", "renewcommand": "changes", "renewenvironment": "changes",
}

type openEnv struct {
	name string
	at   int
}

// checkTeX checks the TeX source s of one math span. It returns the offset of the first problem and a message, or an empty message.
func checkTeX(s string, m Macros) (int, string) {
	var braces []int
	var envs []openEnv
	for i := 0; i < len(s); {
		switch s[i] {
		case '%':
			i += strings.IndexByte(s[i:]+"\n", '\n')
		case '{':
			braces = append(braces, i)
			i++
		case '}':
			if len(braces) == 0 {
				return i, "a closing brace has no opening brace"
			}
			braces = braces[:len(braces)-1]
			i++
		case '\\':
			name, next := command(s, i)
			args, macro := m[name]
			switch {
			case name == "":
				return i, "the math ends with a backslash"
			case !isLetter(rune(name[0])):
				if !strings.Contains(controlSymbols, name) {
					return i, fmt.Sprintf(`the command \%s is not in KaTeX and MathJax`, name)
				}
			case defines[name] != "":
				return i, fmt.Sprintf(`the body %s a macro with \%s. Declare each macro in macros/m2.sty and macros/m2.katex.json`, defines[name], name)
			case name == "begin" || name == "end":
				env, after, ok := braceName(s, next)
				if !ok {
					return i, fmt.Sprintf(`\%s needs the name of an environment in braces`, name)
				}
				next = after
				if name == "begin" {
					if !environments[env] {
						return i, fmt.Sprintf("the environment %s is not in both KaTeX and MathJax", env)
					}
					envs = append(envs, openEnv{env, i})
					break
				}
				if len(envs) == 0 || envs[len(envs)-1].name != env {
					return i, fmt.Sprintf(`\end{%s} has no \begin{%s}`, env, env)
				}
				envs = envs[:len(envs)-1]
			case macro:
				if got := countArgs(s, next, args); got < args {
					return i, fmt.Sprintf(`the macro \%s needs %s, and it has %d`, name, plural(args, "argument"), got)
				}
			case commands[name]:
			case mathJaxOnly[name]:
				return i, fmt.Sprintf(`the command \%s is in MathJax but not in KaTeX`, name)
			case katexOnly[name]:
				return i, fmt.Sprintf(`the command \%s is in KaTeX but not in MathJax`, name)
			default:
				return i, fmt.Sprintf(`the command \%s is not in KaTeX and MathJax, and the macros do not declare it`, name)
			}
			i = next
		default:
			i++
		}
	}
	if len(braces) > 0 {
		return braces[len(braces)-1], "a brace is not closed"
	}
	if len(envs) > 0 {
		e := envs[len(envs)-1]
		return e.at, fmt.Sprintf(`the environment %s has no \end`, e.name)
	}
	return 0, ""
}

func plural(n int, word string) string {
	if n == 1 {
		return "one " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// command returns the name of the command at s[i], which is a backslash, and the offset after it. A control word is letters. A control symbol is one character. The name is "" when s ends after the backslash.
func command(s string, i int) (string, int) {
	j := i + 1
	if j >= len(s) {
		return "", j
	}
	if !isLetter(rune(s[j])) {
		_, size := utf8.DecodeRuneInString(s[j:])
		return s[j : j+size], j + size
	}
	for j < len(s) && isLetter(rune(s[j])) {
		j++
	}
	return s[i+1 : j], j
}

func isLetter(r rune) bool { return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' }

// braceName reads {name} after spaces at s[i]. It returns the name and the offset after the closing brace.
func braceName(s string, i int) (string, int, bool) {
	for i < len(s) && s[i] == ' ' {
		i++
	}
	if i >= len(s) || s[i] != '{' {
		return "", i, false
	}
	end := strings.IndexByte(s[i:], '}')
	if end < 0 {
		return "", i, false
	}
	return s[i+1 : i+end], i + end + 1, true
}

// countArgs counts the arguments after s[i], up to want. An argument is a group in braces, a command or one character.
func countArgs(s string, i, want int) int {
	got := 0
	for got < want {
		for i < len(s) && strings.IndexByte(" \t\n", s[i]) >= 0 {
			i++
		}
		if i >= len(s) {
			return got
		}
		switch s[i] {
		case '}', '%', '&':
			return got
		case '{':
			end := matchBrace(s, i)
			if end < 0 {
				return want
			}
			i = end + 1
		case '\\':
			_, i = command(s, i)
		default:
			_, size := utf8.DecodeRuneInString(s[i:])
			i += size
		}
		got++
	}
	return got
}

// matchBrace returns the offset of the brace that closes the brace at s[i], or -1.
func matchBrace(s string, i int) int {
	depth := 0
	for ; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
