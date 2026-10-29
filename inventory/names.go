package inventory

import (
	"slices"
	"strings"
	"unicode"
)

// nameResultWords are the words of which the name of a statement needs one to join a row by its name, as "theorem" in "Gershgorin circle theorem". A paper calls many items "Lower bound" or "Stability", and such a name is not the name of one result.
var nameResultWords = map[string]bool{
	"theorem": true, "lemma": true, "inequality": true, "conjecture": true, "formula": true, "identity": true,
	"principle": true, "law": true, "criterion": true, "hypothesis": true, "paradox": true, "problem": true,
}

// NameKey is the form of a name in which two names of one item are equal: lower case, a space for each dash, no leading article, no possessive, and one space between words. "The Perron–Frobenius Theorem" and "Perron-Frobenius theorem" have one key, and so do "Lucas's theorem" and "Lucas' theorem".
func NameKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '\'' || r == '’':
			b.WriteByte('\'')
		default:
			b.WriteByte(' ')
		}
	}
	ws := strings.Fields(b.String())
	for i, w := range ws {
		w = strings.TrimSuffix(w, "'s")
		ws[i] = strings.Trim(w, "'")
	}
	ws = slices.DeleteFunc(ws, func(w string) bool { return w == "" })
	for len(ws) > 0 && (ws[0] == "the" || ws[0] == "a" || ws[0] == "an") {
		ws = ws[1:]
	}
	return strings.Join(ws, " ")
}

// nameJoins reports whether a mention of a paper can join a row by its name, and with which key. Its name has two words or more, and the name of a statement, a conjecture or a problem has a result word. A name with TeX in it is not a name of the paper's item that another source has.
func nameJoins(m *Mention) (string, bool) {
	if m.Attached() || m.Name == "" || strings.ContainsAny(m.Name, `$\{}`) {
		return "", false
	}
	switch m.Kind {
	case "statement", "conjecture", "problem", "definition":
	default:
		return "", false
	}
	k := NameKey(m.Name)
	ws := strings.Fields(k)
	if len(ws) < 2 {
		return "", false
	}
	if m.Kind != "definition" && !slices.ContainsFunc(ws, func(w string) bool { return nameResultWords[w] }) {
		return "", false
	}
	return k, true
}

// joinNames is the name pass of corpus arxiv AX8.2: a mention of the research layer joins the row of a mention of another source when its own name is a name of that mention, the kinds agree, the blocks meet, and the name gives no other group. It is a rule of code: two names that are equal after NameKey, and no judge. The name must be the whole name, so "the Hahn–Banach theorem for modules" does not join "Hahn–Banach theorem", and a name that gives two groups, as "chain rule" for a statement of calculus and one of probability, joins neither. It returns the number of joins.
func joinNames(u unionFind, ms []Mention) int {
	type named struct {
		id     string
		kind   string
		blocks []string
	}
	index := map[string][]named{}
	for i := range ms {
		m := &ms[i]
		if IsResearch(m.Source) || m.Attached() || m.Kind == Unknown {
			continue
		}
		for _, n := range append([]string{m.Name}, m.Aliases...) {
			if k := NameKey(n); k != "" {
				index[k] = append(index[k], named{m.ID, m.Kind, Blocks(m)})
			}
		}
	}
	meet := func(a, b []string) bool {
		return slices.Contains(a, "") || slices.Contains(b, "") || slices.ContainsFunc(a, func(x string) bool { return slices.Contains(b, x) })
	}
	n := 0
	for i := range ms {
		m := &ms[i]
		if !IsResearch(m.Source) {
			continue
		}
		k, ok := nameJoins(m)
		if !ok {
			continue
		}
		blocks := Blocks(m)
		var roots []string
		target := ""
		for _, c := range index[k] {
			if !sameBlockKind(m.Kind, c.kind) || !meet(blocks, c.blocks) {
				continue
			}
			if r := u.find(c.id); !slices.Contains(roots, r) {
				roots, target = append(roots, r), c.id
			}
		}
		if len(roots) == 1 && u.union(m.ID, target) {
			n++
		}
	}
	return n
}
