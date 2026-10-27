package arxiv

import (
	"slices"
	"strings"
	"unicode"

	"github.com/tamnd/math2-agent/inventory"
)

// resultWords are the words of which a name of a result has one, as in "Hahn-Banach theorem" or "Zorn's lemma". A row name with none of them, as "Compactness", is too common in text to name a result. The value is true for a strong word, which names a result also in a row of the kind topic or unknown. A weak word, as in "identity matrix" or "universal property", names a result only in a row of the kind statement or problem.
var resultWords = map[string]bool{
	"theorem": true, "theorems": true, "lemma": true, "inequality": true, "inequalities": true, "conjecture": true,
	"principle": true, "criterion": true, "law": true, "paradox": true,
	"formula": false, "identity": false, "property": false, "construction": false, "hypothesis": false, "trick": false,
}

// plainWords are the words that do not make a name specific, as in "Main theorem", "Key lemma" or "Existence theorem". A name needs a word that is not one of them and not a result word.
var plainWords = map[string]bool{
	"a": true, "an": true, "the": true, "this": true, "our": true, "main": true, "key": true, "technical": true, "basic": true,
	"first": true, "second": true, "third": true, "general": true, "auxiliary": true, "central": true, "crucial": true, "important": true,
	"useful": true, "simple": true, "standard": true, "classical": true, "new": true, "following": true, "previous": true, "next": true,
	"above": true, "below": true, "same": true, "existence": true, "uniqueness": true, "comparison": true, "extension": true,
	"approximation": true, "representation": true, "decomposition": true, "classification": true, "regularity": true, "stability": true,
	"compactness": true, "embedding": true, "duality": true, "density": true, "vanishing": true, "fixed": true, "point": true,
	"convergence": true, "continuity": true, "product": true, "local": true, "global": true, "fundamental": true, "structure": true, "of": true, "for": true, "on": true, "in": true,
}

// Names is the index of the named results of the inventory, for the name method of AX6.2. It holds each row of the kind statement, problem, topic or unknown that has a name of two or more words with a result word and a specific word, that has a mention from a source other than arXiv, and that is not merged into another row. A row with only arXiv mentions has the name that one paper gives, which is not a known name.
type Names struct {
	byFirst map[string][]namedRow
}

type namedRow struct {
	words []string
	row   string
	areas []string // the MSC codes of the row at two digits
}

// NewNames indexes the names of the rows.
func NewNames(rows []inventory.Row) *Names {
	n := &Names{byFirst: map[string][]namedRow{}}
	for i := range rows {
		r := &rows[i]
		if r.Name == "" || (r.Outcome != nil && r.Outcome.Value == inventory.OutcomeMerged) {
			continue
		}
		if !slices.ContainsFunc(r.Mentions, func(m string) bool { return !strings.HasPrefix(m, "ax://") }) {
			continue
		}
		weak := false
		switch r.Kind {
		case "statement", "problem":
			weak = true
		case "topic", "unknown":
		default:
			continue
		}
		ws := trimArticles(nameWords(r.Name))
		result := slices.ContainsFunc(ws, func(w string) bool {
			strong, ok := resultWords[w]
			return ok && (strong || weak)
		})
		specific := slices.ContainsFunc(ws, func(w string) bool {
			_, ok := resultWords[w]
			return !ok && !plainWords[w]
		})
		if len(ws) < 2 || !result || !specific {
			continue
		}
		var areas []string
		for _, c := range r.MSC {
			if a := c[:min(2, len(c))]; !slices.Contains(areas, a) {
				areas = append(areas, a)
			}
		}
		n.byFirst[ws[0]] = append(n.byFirst[ws[0]], namedRow{words: ws, row: r.ID, areas: areas})
	}
	return n
}

// Find returns the rows that the text names, sorted. A name counts only when one row has it in the MSC areas of the paper. A row with no MSC code is in each area. A paper with no MSC code has no area, so each row with the name counts, and the name must still give one row. The names in skip, as the name of the item itself, do not count.
func (n *Names) Find(text string, msc []string, skip ...string) []string {
	if n == nil || len(n.byFirst) == 0 || text == "" {
		return nil
	}
	var areas []string
	for _, c := range msc {
		areas = append(areas, c[:min(2, len(c))])
	}
	skipped := map[string]bool{}
	for _, s := range skip {
		skipped[strings.Join(trimArticles(nameWords(s)), " ")] = true
	}
	ws := nameWords(text)
	var out []string
	// At each place the longest name wins, so "strong law of large numbers" does not also give "law of large numbers". A name that gives more than one row gives no edge, and still covers its words.
	for i := 0; i < len(ws); i++ {
		best, rows := 0, []string(nil)
		for _, c := range n.byFirst[ws[i]] {
			k := len(c.words)
			if k < best || i+k > len(ws) || !slices.Equal(ws[i:i+k], c.words) {
				continue
			}
			if len(areas) > 0 && len(c.areas) > 0 && !slices.ContainsFunc(c.areas, func(a string) bool { return slices.Contains(areas, a) }) {
				continue
			}
			if k > best {
				best, rows = k, nil
			}
			if !slices.Contains(rows, c.row) {
				rows = append(rows, c.row)
			}
		}
		if best == 0 {
			continue
		}
		if len(rows) == 1 && !skipped[strings.Join(ws[i:i+best], " ")] && !slices.Contains(out, rows[0]) {
			out = append(out, rows[0])
		}
		i += best - 1
	}
	slices.Sort(out)
	return out
}

// trimArticles removes a leading article, which is not part of a name: "The Triangle Inequality" is "triangle inequality".
func trimArticles(ws []string) []string {
	for len(ws) > 0 && (ws[0] == "the" || ws[0] == "a" || ws[0] == "an") {
		ws = ws[1:]
	}
	return ws
}

// nameWords splits a name or a text into lower-case words. A hyphen or a dash joins two words of a name, as in "Hahn-Banach" and "Hahn–Banach", so both give the same word. A possessive stays on its word, as "zorn's".
func nameWords(s string) []string {
	var ws []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			ws = append(ws, strings.Trim(b.String(), "-'"))
			b.Reset()
		}
	}
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		case r == '-' || r == '‐' || r == '‑' || r == '–':
			if b.Len() > 0 {
				b.WriteByte('-')
			}
		case r == '\'' || r == '’':
			if b.Len() > 0 {
				b.WriteByte('\'')
			}
		default:
			flush()
		}
	}
	flush()
	return slices.DeleteFunc(ws, func(w string) bool { return w == "" })
}
