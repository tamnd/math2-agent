package textbook

import (
	"regexp"
	"sort"
	"strings"
)

// The patterns of the LaTeX reader.
var (
	newTheorem     = regexp.MustCompile(`\\newtheorem\*?\s*\{([^{}]+)\}\s*(?:\[[^\]]*\]\s*)?\{([^{}]*)\}`)
	declareTheorem = regexp.MustCompile(`\\declaretheorem\s*(?:\[((?:[^\[\]]|\[[^\]]*\])*)\])?\s*\{([^{}]+)\}`)
	crefName       = regexp.MustCompile(`\\[cC]refname\s*\{([^{}#]+)\}\s*\{([^{}]*)\}`)
	alias          = regexp.MustCompile(`\\newenvironment\s*\{([^{}]+)\}\s*(?:\[(\d)\]\s*)?(?:\[[^\]]*\]\s*)?\{\s*\\begin\{([^{}]+)\}`)
	nameKey        = regexp.MustCompile(`(?:^|,)\s*name\s*=\s*(\{[^{}]*\}|[^,]*)`)
	environment    = regexp.MustCompile(`\\(begin|end)\s*\{([^{}]+)\}`)
	labelCmd       = regexp.MustCompile(`\\(?:ol)?label\s*\{([^{}]+)\}`)
	refCmd         = regexp.MustCompile(`\\(?:hyperref\s*\[([^\]]+)\]|[a-zA-Z]*ref\*?\s*(?:\[[^\]]*\]\s*)*\{([^{}]+)\})`)
	sectionCmd     = regexp.MustCompile(`\\(?:part|chapter|section|subsection|subsubsection|olchapter|olsection|olsubsection)\*?\s*(?:\[[^\]]*\]\s*)?\{`)
	commentEnv     = regexp.MustCompile(`(?s)\\begin\{comment\}.*?\\end\{comment\}`)
)

// shortNames are the kinds of the usual short names of environments, for an environment that no file of the book defines.
var shortNames = map[string]string{
	"thm": "statement", "lem": "statement", "prop": "statement", "cor": "statement",
	"defn": "definition", "defi": "definition", "dfn": "definition", "conj": "conjecture",
}

// readLaTeX gives the items of the files of order that files matches and skip does not match, in that order. It takes the kinds of the environments from the definitions in all the files, in the order of the paths, so that a definition in a .sty file counts for all files. The macros are more commands that define an environment as \newtheorem does, with the name and the display name as their first two arguments.
func readLaTeX(files map[string][]byte, order []string, match, skip *regexp.Regexp, macros []string) []item {
	clean := map[string]string{}
	for _, name := range order {
		clean[name] = stripComments(string(files[name]))
	}
	names := sorted(order)
	defs, args := envKinds(clean, names, macros)
	var items []item
	for _, name := range order {
		if !match.MatchString(name) || skip != nil && skip.MatchString(name) {
			continue
		}
		items = append(items, latexItems(name, clean[name], defs, args)...)
	}
	return items
}

func sorted(s []string) []string {
	s = append([]string(nil), s...)
	sort.Strings(s)
	return s
}

// stripComments removes the comments of LaTeX source, and the comment environments, and keeps the line breaks, so that the lines keep their numbers.
func stripComments(s string) string {
	s = commentEnv.ReplaceAllStringFunc(s, func(m string) string { return strings.Repeat("\n", strings.Count(m, "\n")) })
	var b strings.Builder
	for line := range strings.SplitAfterSeq(s, "\n") {
		end := ""
		if strings.HasSuffix(line, "\n") {
			line, end = line[:len(line)-1], "\n"
		}
		if i := commentAt(line); i >= 0 {
			line = strings.TrimRight(line[:i], " \t")
		}
		b.WriteString(line + end)
	}
	return b.String()
}

// commentAt gives the index of the % that starts a comment in a line, or -1. A % after an odd number of backslashes is a percent sign.
func commentAt(line string) int {
	for i := 0; i < len(line); i++ {
		if line[i] != '%' {
			continue
		}
		n := 0
		for j := i - 1; j >= 0 && line[j] == '\\'; j-- {
			n++
		}
		if n%2 == 0 {
			return i
		}
	}
	return -1
}

// envKinds gives the kind of each environment that the files define, by its display name. An alias that \newenvironment defines as another environment takes the kind of that environment, and args gives the number of its arguments.
func envKinds(files map[string]string, names, macros []string) (kinds map[string]string, args map[string]int) {
	display := map[string]string{} // the display name of each environment
	weak := map[string]string{}    // the display names from \crefname, for an environment with no other
	var macro *regexp.Regexp
	if len(macros) > 0 {
		quoted := make([]string, len(macros))
		for i, m := range macros {
			quoted[i] = regexp.QuoteMeta(m)
		}
		macro = regexp.MustCompile(`\\(?:` + strings.Join(quoted, "|") + `)\s*\{([^{}#]+)\}\s*\{([^{}#]*)\}`)
	}
	aliases := map[string]string{}
	args = map[string]int{}
	for _, name := range names {
		s := files[name]
		for _, m := range newTheorem.FindAllStringSubmatch(s, -1) {
			display[strings.TrimSpace(m[1])] = m[2]
		}
		for _, m := range declareTheorem.FindAllStringSubmatch(s, -1) {
			d := ""
			if k := nameKey.FindStringSubmatch(m[1]); k != nil {
				d = strings.Trim(strings.TrimSpace(k[1]), "{}")
			}
			for env := range strings.SplitSeq(m[2], ",") {
				env = strings.TrimSpace(env)
				if d == "" {
					display[env] = env
				} else {
					display[env] = d
				}
			}
		}
		if macro != nil {
			for _, m := range macro.FindAllStringSubmatch(s, -1) {
				display[strings.TrimSpace(m[1])] = m[2]
			}
		}
		for _, m := range crefName.FindAllStringSubmatch(s, -1) {
			weak[strings.TrimSpace(m[1])] = m[2]
		}
		for _, m := range alias.FindAllStringSubmatch(s, -1) {
			a := strings.TrimSpace(m[1])
			aliases[a] = strings.TrimSpace(m[3])
			if m[2] != "" {
				args[a] = int(m[2][0] - '0')
			}
		}
	}
	kinds = map[string]string{}
	for env, d := range weak {
		if _, ok := display[env]; !ok {
			kinds[env] = kindOf(d)
		}
	}
	for env, d := range display {
		// A name such as \theoremname is the macro of the word theorem.
		if strings.HasPrefix(d, `\`) && strings.HasSuffix(d, "name") {
			d = strings.TrimSuffix(d[1:], "name")
		}
		kinds[env] = kindOf(d)
	}
	for a, env := range aliases {
		if k, ok := kinds[env]; ok {
			kinds[a] = k
		}
	}
	return kinds, args
}

// kindOfEnv gives the kind of an environment: the kind of its definition, else the kind of its usual name. A starred environment that no file defines has the kind of the environment without the star.
func kindOfEnv(env string, defs map[string]string) string {
	if k, ok := defs[env]; ok {
		return k
	}
	if base, ok := strings.CutSuffix(env, "*"); ok {
		return kindOfEnv(base, defs)
	}
	if k, ok := shortNames[env]; ok {
		return k
	}
	return kinds[env]
}

// span is an environment of an item in a file.
type span struct {
	env, kind               string
	begin, body, end, after int // the offsets of \begin, of the body, of \end and after \end{...}
	children                []int
}

// latexItems gives the items of one file.
func latexItems(file, s string, defs map[string]string, args map[string]int) []item {
	var spans []span
	var open []int // the indexes in spans of the open items
	for _, m := range environment.FindAllStringSubmatchIndex(s, -1) {
		env := strings.TrimSpace(s[m[4]:m[5]])
		kind := kindOfEnv(env, defs)
		if kind == "" {
			continue
		}
		if s[m[2]:m[3]] == "begin" {
			if len(open) > 0 {
				p := open[len(open)-1]
				spans[p].children = append(spans[p].children, len(spans))
			}
			open = append(open, len(spans))
			spans = append(spans, span{env: env, kind: kind, begin: m[0], body: m[1], end: -1})
			continue
		}
		// Close the innermost open item of this environment, and the items in it that did not close.
		for i := len(open) - 1; i >= 0; i-- {
			if spans[open[i]].env == env {
				spans[open[i]].end, spans[open[i]].after = m[0], m[1]
				open = open[:i]
				break
			}
		}
	}
	sections := sectionsOf(s)
	var items []item
	for _, sp := range spans {
		if sp.end < 0 {
			continue
		}
		body := s[sp.body:sp.end]
		for range args[sp.env] {
			t := strings.TrimLeft(body, " \t\n")
			g, ok := braced(t)
			if !ok {
				break
			}
			sp.body += len(body) - len(t) + len(g) + 2
			body = s[sp.body:sp.end]
		}
		title, rest := optional(body)
		own := rest // the body without the items in it
		for i := len(sp.children) - 1; i >= 0; i-- {
			c := spans[sp.children[i]]
			if c.end < 0 {
				continue
			}
			at := sp.body + len(body) - len(rest) // the offset of rest in s
			from, to := c.begin-at, c.after-at
			if from >= 0 && to <= len(own) {
				own = own[:from] + own[to:]
			}
		}
		it := item{
			file:    file,
			env:     sp.env,
			kind:    sp.kind,
			title:   strings.Join(strings.Fields(title), " "),
			section: sectionAt(sections, sp.begin),
			text:    strings.TrimSpace(rest),
			line:    1 + strings.Count(s[:sp.begin], "\n"),
		}
		// The label of the item comes before the first environment in it. A later label is of an equation or a list.
		head := own
		if i := strings.Index(own, `\begin`); i >= 0 {
			head = own[:i]
		}
		if m := labelCmd.FindStringSubmatchIndex(head); m != nil {
			// head is also the start of rest, so the label is at the same place in rest.
			it.label = strings.TrimSpace(head[m[2]:m[3]])
			it.text = strings.TrimSpace(rest[:m[0]] + rest[m[1]:])
		}
		for _, m := range refCmd.FindAllStringSubmatch(own, -1) {
			for l := range strings.SplitSeq(m[1]+m[2], ",") {
				if l = strings.TrimSpace(l); l != "" && l != it.label {
					it.refs = append(it.refs, l)
				}
			}
		}
		items = append(items, it)
	}
	return items
}

// optional splits the optional argument [title] from the start of the body of an environment. The argument can hold brackets in braces.
func optional(body string) (title, rest string) {
	t := strings.TrimLeft(body, " \t")
	if !strings.HasPrefix(t, "[") {
		return "", body
	}
	depth := 0
	for i := 1; i < len(t); i++ {
		switch t[i] {
		case '{':
			depth++
		case '}':
			depth--
		case ']':
			if depth == 0 {
				return t[1:i], t[i+1:]
			}
		}
	}
	return "", body
}

// heading is a division of a file and its offset.
type heading struct {
	at    int
	title string
}

// sectionsOf gives the headings of a file, in order.
func sectionsOf(s string) []heading {
	var out []heading
	for _, m := range sectionCmd.FindAllStringIndex(s, -1) {
		if t, ok := braced(s[m[1]-1:]); ok {
			out = append(out, heading{m[0], strings.Join(strings.Fields(t), " ")})
		}
	}
	return out
}

// sectionAt gives the title of the last heading before an offset.
func sectionAt(hs []heading, at int) string {
	i := sort.Search(len(hs), func(i int) bool { return hs[i].at > at })
	if i == 0 {
		return ""
	}
	return hs[i-1].title
}

// braced gives the text of the group that starts at s[0], which is {.
func braced(s string) (string, bool) {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[1:i], true
			}
		}
	}
	return "", false
}
