package align

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// stop are the words that do not help to tell two names apart.
var stop = map[string]bool{
	"a": true, "an": true, "the": true, "of": true, "on": true, "in": true, "for": true,
	"and": true, "to": true, "with": true, "by": true, "is": true, "at": true,
}

// possessive matches the s of a possessive at the end of a word, as in Lagrange's theorem.
var possessive = regexp.MustCompile(`['’]s\b`)

// words returns the words of s in lowercase, without accents, without the s of a possessive and in the singular, with each run of letters and digits as one word. It drops a word in stop only when the word stands alone, thus the a of A-group stays.
func words(s string) []string {
	var out []string
	for chunk := range strings.FieldsSeq(possessive.ReplaceAllString(fold(s), "")) {
		parts := strings.FieldsFunc(chunk, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
		if len(parts) == 1 && stop[parts[0]] {
			continue
		}
		for _, w := range parts {
			out = append(out, singular(w))
		}
	}
	return out
}

// singular returns the word without the s of a plural: numbers is number, and primes is prime. It keeps the s after s, u or i, as in Gauss, torus and basis, and it keeps a word of three letters or less. Both the item and the record go through singular, thus a word that is not a plural, such as series, changes on both sides and still matches.
func singular(w string) string {
	if len(w) <= 3 || !strings.HasSuffix(w, "s") {
		return w
	}
	switch w[len(w)-2] {
	case 's', 'u', 'i':
		return w
	}
	return w[:len(w)-1]
}

// fold returns s in lowercase and without the marks of accents, thus Erdős and erdos are the same.
func fold(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// Key returns the normal form of a name: its words with one space between them. Two names with the same key are the same name. The inventory uses it to match the index terms of a book with the rows (corpus K4.7).
func Key(name string) string { return strings.Join(words(name), " ") }

// names returns the keys of the names, and of the part of each name before a colon, for a title such as "Lagrange's theorem: the order of a subgroup". It drops empty keys and keys that are there two times.
func names(list ...string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if k := Key(s); k != "" && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for _, n := range list {
		add(n)
		if before, _, ok := strings.Cut(n, ":"); ok {
			add(before)
		}
	}
	return out
}

// mathSpan matches the math of Markdown with LaTeX: $...$ and $$...$$.
var mathSpan = regexp.MustCompile(`\$\$[^$]+\$\$|\$[^$\n]+\$`)

// space matches the space and the commands that do not change a formula.
var space = regexp.MustCompile(`\s+|\\[,;:! ]|\\left|\\right|\\displaystyle|\\mathrm|\\operatorname`)

// formulas returns the normal forms of the formulas in the text. A formula of one or two characters, such as $G$, says nothing, and formulas drops it.
func formulas(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range mathSpan.FindAllString(text, -1) {
		f := strings.Trim(m, "$")
		f = space.ReplaceAllString(f, "")
		f = strings.NewReplacer("{", "", "}", "").Replace(f)
		if len([]rune(f)) <= 2 || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// link matches a link to a node, [[text|target]], and keeps the text.
var link = regexp.MustCompile(`\[\[([^|\]]*)\|[^\]]*\]\]`)

// prose returns the body of a node without its headings, without the sections that do not state what the node is, and with the text of each link in place of the link.
func prose(body string) string {
	var b strings.Builder
	skip := false
	for line := range strings.Lines(body) {
		if h, ok := strings.CutPrefix(line, "## "); ok {
			h = strings.TrimSpace(h)
			skip = h == "Remarks" || h == "Proof" || h == "History" || h == "References"
			continue
		}
		if !skip {
			b.WriteString(link.ReplaceAllString(line, "$1"))
		}
	}
	return strings.TrimSpace(b.String())
}
