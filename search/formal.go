package search

import (
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// shape is what the formal channel knows of the type of a declaration (spec 14.3): the head constant of the conclusion, the head constants of the hypotheses and the constants of the type.
type shape struct {
	concl  string
	hyps   []string
	consts []string
}

// typeShape reads the canonical form of a type, as M2Extract writes it, and returns its shape. The form has one node on each line, and the last line is the root. A line that the function does not know gives no constant.
func typeShape(canon string, consts []string) shape {
	lines := strings.Split(strings.TrimRight(canon, "\n"), "\n")
	sh := shape{consts: slices.Clone(consts)}
	if len(lines) == 0 || lines[0] == "" {
		return sh
	}
	ref := func(s string) int {
		i, err := strconv.Atoi(s)
		if err != nil || i < 0 || i >= len(lines) {
			return -1
		}
		return i
	}
	var head func(i, depth int) string
	head = func(i, depth int) string {
		if i < 0 || depth > 1000 {
			return ""
		}
		f := strings.Fields(lines[i])
		if len(f) == 0 {
			return ""
		}
		switch f[0] {
		case "c":
			if len(f) > 1 {
				return f[1]
			}
		case "a":
			if len(f) > 1 {
				return head(ref(f[1]), depth+1)
			}
		case "p":
			return "→"
		case "s":
			return "Sort"
		}
		return ""
	}
	i := len(lines) - 1
	for steps := 0; steps < len(lines); steps++ {
		f := strings.Fields(lines[i])
		if len(f) != 4 || f[0] != "p" {
			break
		}
		if h := head(ref(f[2]), 0); h != "" && h != "Sort" && h != "→" {
			sh.hyps = append(sh.hyps, h)
		}
		i = ref(f[3])
		if i < 0 {
			return sh
		}
	}
	sh.concl = head(i, 0)
	slices.Sort(sh.hyps)
	sh.hyps = slices.Compact(sh.hyps)
	return sh
}

// The notation of Lean and the constants that it stands for. The formal channel reads a query with them.
var notation = map[string]string{
	"=": "Eq", "≠": "Ne", "∣": "Dvd.dvd", "≤": "LE.le", "<": "LT.lt", "≥": "GE.ge", ">": "GT.gt", "∈": "Membership.mem", "∉": "Membership.mem",
	"+": "HAdd.hAdd", "-": "HSub.hSub", "*": "HMul.hMul", "/": "HDiv.hDiv", "%": "HMod.hMod", "^": "HPow.hPow", "•": "HSMul.hSMul",
	"∑": "Finset.sum", "∏": "Finset.prod", "¬": "Not", "∧": "And", "∨": "Or", "↔": "Iff", "∃": "Exists", "⊆": "HasSubset.Subset", "⊂": "HasSSubset.SSubset",
	"∩": "Inter.inter", "∪": "Union.union", "ℕ": "Nat", "ℤ": "Int", "ℚ": "Rat", "ℝ": "Real", "ℂ": "Complex", "φ": "Nat.totient", "∫": "MeasureTheory.integral",
	"⁻¹": "Inv.inv", "‖": "Norm.norm", "∘": "Function.comp", "×": "Prod", "≃": "Equiv", "≅": "CategoryTheory.Iso", "⧸": "HasQuotient.Quotient", "↑": "",
}

// The infix operators, from the weakest to the strongest binding. The head of a part of a query is its weakest operator at the top level.
var infixLevels = [][]string{{"↔"}, {"∨"}, {"∧"}, {"=", "≠", "∣", "≤", "<", "≥", ">", "∈", "∉", "⊆", "⊂", "≃", "≅"}, {"+", "-", "∪"}, {"*", "/", "%", "•", "∩", "∘"}, {"^"}}

// leanQuery is a parsed Lean query: a type pattern, a goal or a set of constants.
type leanQuery struct {
	concl  string   // the head constant of the conclusion, or "" when the query has none
	hyps   []string // the head constants of the hypotheses
	consts []string // the constants that the query names
}

// isLean reports whether a query looks like a Lean type, a goal or a list of Lean names.
func isLean(q string) bool {
	if strings.ContainsAny(q, "→∀∃⊢ℕℤℚℝℂ∣≤≥≠∑∏↔∧∨¬") {
		return true
	}
	if strings.Contains(q, "$") || strings.Contains(q, `\`) {
		return false
	}
	// A list of names, such as "IsCyclic, Units, Field", or one name with a dot, such as Nat.Prime.
	parts := strings.Split(q, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || strings.ContainsRune(p, ' ') {
			return false
		}
		r, _ := utf8.DecodeRuneInString(p)
		if !unicode.IsUpper(r) && !strings.Contains(p, ".") {
			return false
		}
	}
	return len(parts) > 1 || strings.Contains(q, ".")
}

// parseLean reads a Lean query. A goal has ⊢ before the conclusion. A pattern has → between the hypotheses and the conclusion. A query of a binder ∀ x : T, P has P as its conclusion. A list of names separated by commas names only constants.
func parseLean(q string) leanQuery {
	var out leanQuery
	q = strings.TrimSpace(q)
	var hyps []string
	bound := map[string]bool{} // the names of the binders, which are not constants
	concl := q
	if before, after, ok := strings.Cut(q, "⊢"); ok {
		for _, l := range strings.FieldsFunc(before, func(r rune) bool { return r == '\n' || r == ';' }) {
			if _, t, ok := strings.Cut(l, ":"); ok {
				hyps = append(hyps, t)
			} else if strings.TrimSpace(l) != "" {
				hyps = append(hyps, l)
			}
		}
		concl = after
	} else if !strings.ContainsAny(q, "→∀∃=≠∣≤<≥>∈↔") && strings.Contains(q, ",") {
		for _, p := range strings.Split(q, ",") {
			out.consts = append(out.consts, strings.TrimSpace(p))
		}
		return out
	}
	// Remove the binders ∀ x : T, and keep their types as hypotheses.
	for {
		concl = strings.TrimSpace(concl)
		if !strings.HasPrefix(concl, "∀") {
			break
		}
		comma := topIndex(concl, ",")
		if comma < 0 {
			break
		}
		binder := strings.TrimPrefix(concl[:comma], "∀")
		for _, group := range splitBinders(binder) {
			if vars, t, ok := strings.Cut(group, ":"); ok {
				hyps = append(hyps, t)
				for _, v := range strings.Fields(vars) {
					bound[v] = true
				}
			}
		}
		concl = concl[comma+1:]
	}
	parts := splitTop(concl, "→")
	concl = parts[len(parts)-1]
	hyps = append(hyps, parts[:len(parts)-1]...)
	out.concl = headOf(concl)
	for _, h := range hyps {
		if c := headOf(h); c != "" && !sorts[c] && !bound[c] {
			out.hyps = append(out.hyps, c)
		}
	}
	for _, c := range constantsOf(q) {
		if !sorts[c] && !bound[c] {
			out.consts = append(out.consts, c)
		}
	}
	slices.Sort(out.hyps)
	out.hyps = slices.Compact(out.hyps)
	return out
}

// The universes. A binder of a type, such as (G : Type), gives no hypothesis.
var sorts = map[string]bool{"Type": true, "Sort": true, "Prop": true}

// splitBinders splits the binders of a ∀, such as "(n : ℕ) [Fact p]" or "n : ℕ", into groups.
func splitBinders(s string) []string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "(") && !strings.HasPrefix(s, "{") && !strings.HasPrefix(s, "[") {
		return []string{s}
	}
	var out []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '(', '{', '[', '⦃':
			if depth == 0 {
				start = i + utf8.RuneLen(r)
			}
			depth++
		case ')', '}', ']', '⦄':
			depth--
			if depth == 0 {
				g := s[start:i]
				if r == ']' && !strings.Contains(g, ":") {
					g = ":" + g // an instance binder without a name
				}
				out = append(out, g)
			}
		}
	}
	return out
}

// topIndex returns the index of the first sep in s that is not in brackets, or -1.
func topIndex(s, sep string) int {
	depth := 0
	for i, r := range s {
		switch r {
		case '(', '{', '[', '⦃', '⟨':
			depth++
		case ')', '}', ']', '⦄', '⟩':
			depth--
		}
		if depth == 0 && strings.HasPrefix(s[i:], sep) {
			return i
		}
	}
	return -1
}

// splitTop splits s at each sep that is not in brackets.
func splitTop(s, sep string) []string {
	var out []string
	for {
		i := topIndex(s, sep)
		if i < 0 {
			return append(out, s)
		}
		out = append(out, s[:i])
		s = s[i+len(sep):]
	}
}

// headOf returns the head constant of a part of a query: its weakest infix operator at the top level, or else its first name. A wildcard _ or a variable gives "".
func headOf(s string) string {
	s = strings.TrimSpace(s)
	for len(s) > 1 && s[0] == '(' && topIndex(s[1:], ")") == len(s)-2 {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	for _, level := range infixLevels {
		for _, op := range level {
			if topIndex(s, op) > 0 {
				return notation[op]
			}
		}
	}
	for _, op := range []string{"¬", "∃", "∑", "∏"} {
		if strings.HasPrefix(s, op) {
			return notation[op]
		}
	}
	names := names(s)
	if len(names) == 0 {
		return ""
	}
	return constName(names[0])
}

// names returns the names in s, in order: the runs of letters, digits, _, ' and dots.
func names(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !inName(r) })
}

// inName reports whether a rune can be part of a name.
func inName(r rune) bool {
	if unicode.IsLetter(r) {
		return true
	}
	return (unicode.IsDigit(r) || r == '_' || r == '.' || r == '\'' || r >= '₀' && r <= '₉') && notation[string(r)] == ""
}

// constName returns the constant that a name in a query stands for, or "" when the name looks like a variable. A name that starts with a capital letter or has a dot is a constant. With field notation, as in n.divisors, the constant is the last part.
func constName(n string) string {
	if c, ok := notation[n]; ok {
		return c
	}
	if n == "_" || n == "" {
		return ""
	}
	r, _ := utf8.DecodeRuneInString(n)
	if unicode.IsUpper(r) {
		return n
	}
	if i := strings.LastIndex(n, "."); i >= 0 {
		first := n[:strings.Index(n, ".")]
		if len([]rune(first)) <= 2 { // a variable, as in n.divisors
			return n[i+1:]
		}
		return n
	}
	return ""
}

// The operators that are in most statements. They are not constants of a query, because they do not tell the statements apart.
var common = map[string]bool{"-": true, "+": true, "*": true, "/": true, "^": true, "%": true, "<": true, ">": true, "=": true}

// constantsOf returns the constants of a query: its names that look like constants, and the constants of its notation.
func constantsOf(q string) []string {
	var out []string
	for _, n := range names(q) {
		if c := constName(n); c != "" {
			out = append(out, c)
		}
	}
	for sym, c := range notation {
		if c != "" && strings.Contains(q, sym) && !common[sym] {
			out = append(out, c)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// matchName reports whether the constant c of a declaration is the constant q of a query. A query can leave out the namespaces: Prime matches Nat.Prime.
func matchName(c, q string) bool {
	return c == q || strings.HasSuffix(c, "."+q)
}

// score returns how well a declaration with the shape fits the query, or 0 when it does not fit. A constant of the query that no declaration has is not part of the score, thus resolved gives the constants that some declaration has.
func (q leanQuery) score(s shape, resolved map[string]bool) float64 {
	var total, found float64
	for _, c := range q.consts {
		if !resolved[c] {
			continue
		}
		total++
		if slices.ContainsFunc(s.consts, func(x string) bool { return matchName(x, c) }) || matchName(s.concl, c) || slices.ContainsFunc(s.hyps, func(x string) bool { return matchName(x, c) }) {
			found++
		}
	}
	var sc float64
	if total > 0 {
		sc = found / total
	}
	if q.concl != "" && s.concl != "" && matchName(s.concl, q.concl) {
		sc++
	}
	if len(q.hyps) > 0 {
		var h float64
		for _, x := range q.hyps {
			if slices.ContainsFunc(s.hyps, func(y string) bool { return matchName(y, x) }) {
				h++
			}
		}
		sc += 0.5 * h / float64(len(q.hyps))
	}
	return sc
}
