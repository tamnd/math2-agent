package check

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// MacroFile is the path of the KaTeX macro table in a math2 working tree. macros/m2.sty declares the same macros for LaTeX.
const MacroFile = "macros/m2.katex.json"

// Macros maps the name of each declared macro, with no backslash, to its number of arguments.
type Macros map[string]int

//go:embed m2.katex.json
var embeddedMacros []byte

var (
	macrosOnce sync.Once
	macros     Macros
)

// EmbeddedMacros returns the copy of the macro table in this package.
func EmbeddedMacros() Macros {
	macrosOnce.Do(func() {
		m, err := ParseMacros(embeddedMacros)
		if err != nil {
			panic("check: the embedded macro table is not valid: " + err.Error())
		}
		macros = m
	})
	return macros
}

// LoadMacros reads the macro table from the working tree at root. When the tree has no macro table, LoadMacros returns the embedded copy.
func LoadMacros(root string) (Macros, error) {
	path := filepath.Join(root, MacroFile)
	data, err := os.ReadFile(path) // #nosec G304 -- the path is in the working tree, which the caller owns.
	if errors.Is(err, fs.ErrNotExist) {
		return EmbeddedMacros(), nil
	}
	if err != nil {
		return nil, err
	}
	m, err := ParseMacros(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.ToSlash(path), err)
	}
	return m, nil
}

var param = regexp.MustCompile(`#([1-9])`)

// ParseMacros parses a macro table in the KaTeX form: a JSON object that maps each macro, such as "\\ip", to its expansion. The highest #n in the expansion gives the number of arguments.
func ParseMacros(data []byte) (Macros, error) {
	var table map[string]string
	if err := json.Unmarshal(data, &table); err != nil {
		return nil, err
	}
	m := Macros{}
	for k, v := range table {
		name, ok := strings.CutPrefix(k, `\`)
		if !ok || name == "" || strings.IndexFunc(name, func(r rune) bool { return !isLetter(r) }) >= 0 {
			return nil, fmt.Errorf("the macro name %q must be a backslash and letters", k)
		}
		n := 0
		for _, p := range param.FindAllStringSubmatch(v, -1) {
			n = max(n, int(p[1][0]-'0'))
		}
		m[name] = n
	}
	return m, nil
}
