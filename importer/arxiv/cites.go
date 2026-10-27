package arxiv

import (
	"regexp"
	"strings"

	"github.com/tamnd/math2-agent/inventory"
)

// Bib is the part of a bib record of tamnd/arxiv that the importer reads: one entry of the bibliography of one version.
type Bib struct {
	Paper   string `json:"paper"`
	Version int    `json:"version"`
	Key     string `json:"key"`
	ArXiv   string `json:"arxiv"`
	DOI     string `json:"doi"`
	Year    int    `json:"year"`
}

// bibOf returns the entries of each version of a paper by key.
func bibOf(recs []Bib) map[int]map[string]Bib {
	out := map[int]map[string]Bib{}
	for _, r := range recs {
		if out[r.Version] == nil {
			out[r.Version] = map[string]Bib{}
		}
		out[r.Version][r.Key] = r
	}
	return out
}

var subclass = regexp.MustCompile(`^([a-z-]+)\.[A-Z]{2}/`)

// Work returns the record id of the work of a bib entry: arxiv:<id> from the arXiv id, else doi:<doi>, else "" (AX5.7 step 1). An old id loses its subject class, as math.AG/0003117 gives arxiv:math/0003117, because the corpus names the paper so.
func Work(b Bib) string {
	switch {
	case b.ArXiv != "":
		return "arxiv:" + subclass.ReplaceAllString(b.ArXiv, "$1/")
	case b.DOI != "":
		return "doi:" + b.DOI
	}
	return ""
}

// work returns the record id of the work with the bib key in this version, or "" when the bibliography does not resolve the key.
func (vp *versionIn) work(key string) string {
	if b, ok := vp.bib[vp.v][key]; ok {
		return Work(b)
	}
	return ""
}

// locatorIn reads a locator as "Theorem 4.1", "Thm. 4.1", "Prop 2", "Lemma A.3 (ii)" or "4.1": an optional kind word and a number.
var locatorIn = regexp.MustCompile(`^(?:([A-Za-z]+)\.?\s*)?((?:[A-Z]\.?)?\d+(?:\.\d+)*[a-z]?|[A-Z](?:\.\d+)*)(?:\s*\([^)]*\))?$`)

// kindWords is the kind words of a locator, with the mention kind and the role that they name.
var kindWords = map[string][2]string{
	"theorem": {"statement", "theorem"}, "thm": {"statement", "theorem"}, "th": {"statement", "theorem"},
	"lemma": {"statement", "lemma"}, "lem": {"statement", "lemma"}, "lm": {"statement", "lemma"},
	"claim": {"statement", "lemma"},
	"proposition": {"statement", "proposition"}, "prop": {"statement", "proposition"},
	"corollary": {"statement", "corollary"}, "cor": {"statement", "corollary"}, "coro": {"statement", "corollary"},
	"fact": {"statement", "fact"}, "axiom": {"statement", "axiom"},
	"definition": {"definition", ""}, "def": {"definition", ""}, "defn": {"definition", ""},
	"conjecture": {"conjecture", ""}, "conj": {"conjecture", ""},
	"problem": {"problem", ""}, "question": {"problem", ""},
	"example": {"example", ""}, "ex": {"example", ""},
}

// ParseLocator reads a locator into the kind word and the number (AX5.7 step 2). The kind word is "" when the locator is only a number. It reports false for a locator that names no item, as "§3", "p. 12", "(3.2)", "Section 2" or "Main Theorem".
func ParseLocator(loc string) (word, number string, ok bool) {
	m := locatorIn.FindStringSubmatch(strings.Join(strings.Fields(strings.ReplaceAll(loc, "~", " ")), " "))
	if m == nil {
		return "", "", false
	}
	word = strings.ToLower(m[1])
	if word != "" {
		if _, known := kindWords[word]; !known {
			return "", "", false
		}
	} else if !strings.ContainsAny(m[2], "0123456789") {
		return "", "", false
	}
	return word, m[2], true
}

// resolveCites changes each cited work that is a paper of the import into the mention of the cited item, when the locator resolves to one (AX5.7 step 3). It returns the number of cited results that now go to a mention and the number that go to the record id of a work.
func resolveCites(ms []inventory.Mention) (toMention, toWork int) {
	byPaper := map[string][]int{}
	for i := range ms {
		p, _, _ := strings.Cut(ms[i].NativeID, "#")
		byPaper[p] = append(byPaper[p], i)
	}
	for i := range ms {
		for j := range ms[i].CitesResult {
			c := &ms[i].CitesResult[j]
			if !strings.HasPrefix(c.To, "arxiv:") {
				if !strings.HasPrefix(c.To, "bib:") {
					toWork++
				}
				continue
			}
			if t, conf := cited(ms, byPaper[strings.TrimPrefix(c.To, "arxiv:")], c.Locator); t != "" && t != ms[i].ID {
				c.To, c.Confidence = t, conf
				toMention++
				continue
			}
			toWork++
		}
	}
	return toMention, toWork
}

// cited returns the mention among the candidates that the locator names, and the confidence: high when the number resolves and the kinds agree, medium when the number resolves to one item and the locator gives no kind or another kind. The number of the latest version comes first, then the numbers of the other versions.
func cited(ms []inventory.Mention, cands []int, loc string) (string, string) {
	word, number, ok := ParseLocator(loc)
	if !ok || len(cands) == 0 {
		return "", ""
	}
	kr, hasKind := kindWords[word]
	pick := func(numbered func(m *inventory.Mention) bool) (string, string) {
		var all, agree []string
		for _, i := range cands {
			m := &ms[i]
			if m.Attached() || !numbered(m) {
				continue
			}
			all = append(all, m.ID)
			if hasKind && m.Kind == kr[0] && (kr[1] == "" || m.Role == kr[1]) {
				agree = append(agree, m.ID)
			}
		}
		switch {
		case len(agree) == 1:
			return agree[0], "high"
		case len(agree) == 0 && len(all) == 1:
			return all[0], "medium"
		}
		return "", ""
	}
	if t, conf := pick(func(m *inventory.Mention) bool { return m.Number == number }); t != "" {
		return t, conf
	}
	return pick(func(m *inventory.Mention) bool {
		for _, v := range m.Versions {
			if v.Number == number {
				return true
			}
		}
		return false
	})
}
