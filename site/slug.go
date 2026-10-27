package site

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// slugKinds is the order in which nodes with the same name get a plain slug. The first kind keeps the slug of the name, and a later kind gets the kind as a suffix.
var slugKinds = []string{"topic", "concept", "definition", "statement", "conjecture", "problem", "object", "example", "counterexample", "proof", "computation", "exercise"}

// Slug returns the part of a page URL for a node name, in lower case with hyphens, for example "lagranges-theorem" for "Lagrange's theorem". It removes accents and apostrophes, and it changes each other run of characters that are not letters or digits to one hyphen. A name with no letter or digit gives "".
func Slug(name string) string {
	var sb strings.Builder
	hyphen := false
	for _, r := range norm.NFD.String(name) {
		switch {
		case unicode.Is(unicode.Mn, r), r == '\'', r == '’':
			continue
		case 'a' <= r && r <= 'z', '0' <= r && r <= '9':
		case 'A' <= r && r <= 'Z':
			r = unicode.ToLower(r)
		default:
			hyphen = sb.Len() > 0
			continue
		}
		if hyphen {
			sb.WriteByte('-')
			hyphen = false
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// assignSlugs gives each node a unique slug. The nodes are in the order of slugKinds and then of their tags. A node gets the slug of its name when no node before it has that slug. Else it gets the slug with its kind as a suffix, and else the slug with its tag as a suffix. A name with no letter or digit gives the tag in lower case.
func assignSlugs(views []*nodeView) {
	sorted := slices.Clone(views)
	rank := func(k string) int {
		if i := slices.Index(slugKinds, k); i >= 0 {
			return i
		}
		return len(slugKinds)
	}
	slices.SortFunc(sorted, func(a, b *nodeView) int {
		return cmp.Or(cmp.Compare(rank(a.Kind), rank(b.Kind)), cmp.Compare(a.Tag, b.Tag))
	})
	used := map[string]bool{}
	for _, v := range sorted {
		v.Slug = strings.ToLower(v.Tag)
		if base := Slug(v.Name); base != "" {
			for _, s := range []string{base, base + "-" + v.Kind, base + "-" + v.Slug} {
				if !used[s] {
					v.Slug = s
					break
				}
			}
		}
		used[v.Slug] = true
	}
}
