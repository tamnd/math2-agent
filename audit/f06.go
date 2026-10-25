package audit

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// library is the directory of the math2 Lean library, and hypotheses is the directory of the named hypotheses in it.
const (
	library    = "lean/Math2"
	hypotheses = "lean/Math2/Hypotheses/"
)

// unsound gives, for each word that rule F06 refuses, what to do instead.
var unsound = map[string]string{
	"sorry":          "Complete the proof",
	"admit":          "Complete the proof",
	"axiom":          "Prove it, or state it as a named hypothesis in Math2/Hypotheses/",
	"implemented_by": "Remove the attribute. The kernel cannot check the code that it names",
}

// f06 checks that the math2 Lean library has no sorry, admit, axiom or implemented_by outside Math2/Hypotheses/ (rule F06). It reads the source files and ignores comments and string literals.
func f06(a *auditor) outcome {
	var o outcome
	var files []string
	if data, err := a.readFile(library + ".lean"); err == nil && data != nil {
		files = append(files, library+".lean")
	}
	err := filepath.WalkDir(filepath.Join(a.root, library), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.SkipDir
			}
			return err
		}
		rel, _ := filepath.Rel(a.root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() && rel+"/" == hypotheses {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(rel, ".lean") {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		o.reason = err.Error()
		return o
	}
	for _, rel := range files {
		if !a.inScope(rel) {
			continue
		}
		data, err := a.readFile(rel)
		if err != nil {
			o.reason = err.Error()
			return o
		}
		o.checked++
		code := scrub(data)
		for i := 0; i < len(code); {
			r, size := utf8.DecodeRune(code[i:])
			if !identStart(r) {
				i += size
				continue
			}
			j := i
			for j < len(code) {
				r, size := utf8.DecodeRune(code[j:])
				if !identRest(r) {
					break
				}
				j += size
			}
			if fix, ok := unsound[string(code[i:j])]; ok {
				line, col := lineCol(data, i)
				o.add(rel, line, col, "the library uses %s outside Math2/Hypotheses/. %s", code[i:j], fix)
			}
			i = j
		}
	}
	return o
}

func identStart(r rune) bool { return r == '_' || unicode.IsLetter(r) }

// identRest reports whether r can be in a Lean name after the first rune. The dot is in the set, so that a word in a dotted name, for example Foo.sorry, is not a word of its own.
func identRest(r rune) bool {
	return identStart(r) || unicode.IsDigit(r) || r == '\'' || r == '!' || r == '?' || r == '.'
}

// scrub returns a copy of the Lean source src with spaces for the comments, the string and char literals and the quoted names. Each newline stays, so that the offsets in the copy are the offsets in src.
func scrub(src []byte) []byte {
	out := []byte(string(src))
	blank := func(from, to int) {
		for k := from; k < to && k < len(out); k++ {
			if out[k] != '\n' {
				out[k] = ' '
			}
		}
	}
	prevIdent := func(i int) bool {
		r, _ := utf8.DecodeLastRune(src[:i])
		return i > 0 && identRest(r)
	}
	for i := 0; i < len(src); {
		switch {
		case strings.HasPrefix(string(src[i:]), "--"):
			j := i
			for j < len(src) && src[j] != '\n' {
				j++
			}
			blank(i, j)
			i = j
		case strings.HasPrefix(string(src[i:]), "/-"):
			depth, j := 1, i+2
			for j < len(src) && depth > 0 {
				switch {
				case strings.HasPrefix(string(src[j:]), "/-"):
					depth, j = depth+1, j+2
				case strings.HasPrefix(string(src[j:]), "-/"):
					depth, j = depth-1, j+2
				default:
					j++
				}
			}
			blank(i, j)
			i = j
		case src[i] == 'r' && !prevIdent(i) && rawString(src[i+1:]) > 0:
			hashes := rawString(src[i+1:]) - 1
			end := `"` + strings.Repeat("#", hashes)
			j := i + 1 + hashes + 1
			if k := strings.Index(string(src[j:]), end); k >= 0 {
				j += k + len(end)
			} else {
				j = len(src)
			}
			blank(i, j)
			i = j
		case src[i] == '"':
			j := i + 1
			for j < len(src) && src[j] != '"' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			blank(i, j+1)
			i = j + 1
		case src[i] == '\'' && !prevIdent(i):
			j := i + 1
			if j < len(src) && src[j] == '\\' {
				j++
			}
			_, size := utf8.DecodeRune(src[min(j, len(src)):])
			j += size
			if k := strings.IndexByte(string(src[j:min(j+8, len(src))]), '\''); k >= 0 {
				blank(i, j+k+1)
				i = j + k + 1
			} else {
				i++
			}
		case strings.HasPrefix(string(src[i:]), "«"):
			j := i + len("«")
			if k := strings.Index(string(src[j:]), "»"); k >= 0 {
				j += k + len("»")
			}
			blank(i, j)
			i = j
		default:
			i++
		}
	}
	return out
}

// rawString returns 1 plus the number of hashes when s starts the body of a raw string literal, for example #"" after the r of r#"..."#, and 0 when it does not.
func rawString(s []byte) int {
	k := 0
	for k < len(s) && s[k] == '#' {
		k++
	}
	if k < len(s) && s[k] == '"' {
		return k + 1
	}
	return 0
}
