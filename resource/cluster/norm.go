package cluster

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var fold = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// Fold gives the text in lower case, without diacritics, with each run of characters that are not letters or digits as one space.
func Fold(s string) string {
	s, _, _ = transform.String(fold, s)
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			space = false
		} else {
			space = true
		}
	}
	return b.String()
}

// ordinals are the words of the edition numbers in the languages of RS4.9.
var ordinals = map[string]int{
	"first": 1, "second": 2, "third": 3, "fourth": 4, "fifth": 5, "sixth": 6, "seventh": 7, "eighth": 8, "ninth": 9, "tenth": 10,
	"erste": 1, "zweite": 2, "dritte": 3, "vierte": 4, "funfte": 5, "sechste": 6, "siebte": 7, "achte": 8,
	"premiere": 1, "deuxieme": 2, "seconde": 2, "troisieme": 3, "quatrieme": 4, "cinquieme": 5,
	"prima": 1, "seconda": 2, "terza": 3, "quarta": 4, "primera": 1, "segunda": 2, "tercera": 3, "cuarta": 4,
	"первое": 1, "второе": 2, "третье": 3, "четвертое": 4, "пятое": 5, "шестое": 6,
	"一": 1, "二": 2, "三": 3, "四": 4, "五": 5, "六": 6, "七": 7, "八": 8, "九": 9, "十": 10,
}

var (
	// 2nd ed., 2. Auflage, 3e édition, 2-е издание, edition 4
	editionNum = regexp.MustCompile(`(?i)\b([0-9]{1,2})\s*(?:st|nd|rd|th|e|er|eme|ème|re|a|o|\.|-?е|-?e)?\s*(?:rev(?:ised)?\.?\s*|enl(?:arged)?\.?\s*|corr(?:ected)?\.?\s*|aufl\.?|ed\b|edn\b|edition|ed\.|auflage|édition|edizione|edición|издание|изд\.?|wydanie|wyd\.?|vydání)`)
	editionPre = regexp.MustCompile(`(?i)(?:edition|auflage|издание|ed\.)\s+([0-9]{1,2})\b`)
	// 第二版, 第2版
	editionCJK     = regexp.MustCompile(`第\s*([0-9一二三四五六七八九十]+)\s*版`)
	editionWord    = regexp.MustCompile(`(?i)\b(\pL+)\s+(?:revised\s+|enlarged\s+)?(?:edition|ed\.|auflage|édition|edizione|edición|издание)`)
	editionWordPre = regexp.MustCompile(`(?i)(?:издание|edition)\s+(\pL+)`)
)

// Edition gives the number of the edition that a statement or a title names, or 0. It reads "2nd ed.", "Second edition", "2. Auflage", "3e édition", "Издание второе" and "第二版".
func Edition(s string) int {
	if m := editionCJK.FindStringSubmatch(s); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
		return ordinals[m[1]]
	}
	for _, re := range []*regexp.Regexp{editionNum, editionPre} {
		if m := re.FindStringSubmatch(s); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
				return n
			}
		}
	}
	for _, re := range []*regexp.Regexp{editionWord, editionWordPre} {
		for _, m := range re.FindAllStringSubmatch(s, -1) {
			if n := ordinals[Fold(m[1])]; n > 0 {
				return n
			}
		}
	}
	return 0
}

var (
	editionPart = regexp.MustCompile(`(?i)[\s,.;:(\[]*(?:` + editionNum.String() + `|` + editionWord.String() + `|` + editionCJK.String() + `)[^,;:)\]]*[)\]]?`)
	subtitle    = regexp.MustCompile(`\s*[:;=]\s.*$|\s+[-–—]\s.*$|\s*[.]\s+[A-Z].*$`)
)

// WorkTitle gives the normal form of a title for the grouping of works (pass C5): no edition statement, no subtitle, folded.
func WorkTitle(t string) string {
	t = editionPart.ReplaceAllString(t, "")
	t = subtitle.ReplaceAllString(t, "")
	return Fold(t)
}

// Family gives the folded family name of a creator name, as "Erdős, Paul" or "Paul Erdős".
func Family(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.IndexByte(name, ','); i > 0 {
		return Fold(name[:i])
	}
	f := strings.Fields(Fold(name))
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
}

// Similarity gives the Dice coefficient of the sets of the 3-grams of the folded titles, from 0 to 1.
func Similarity(a, b string) float64 {
	a, b = Fold(a), Fold(b)
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	ga, gb := grams(a), grams(b)
	n := 0
	for g := range ga {
		if gb[g] {
			n++
		}
	}
	return 2 * float64(n) / float64(len(ga)+len(gb))
}

func grams(s string) map[string]bool {
	r := []rune(" " + s + " ")
	out := map[string]bool{}
	for i := 0; i+3 <= len(r); i++ {
		out[string(r[i:i+3])] = true
	}
	return out
}
