package copycheck

import (
	"encoding/base64"
	"encoding/binary"
	"hash/fnv"
	"maps"
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Stage 3 compares the body with the items of the arXiv corpus that the reference corpus has only as a MinHash (corpus arxiv AX9.5): the items of a closed version, whose text math2 does not keep. The MinHash is the fingerprint of AX3.8 that arxiv-reader computes: 128 values of 16 bits over the character 5-grams of the normalized body. Stage 3 computes the same fingerprint of the whole section and of each paragraph of the section with [ItemMin] characters or more, and a similarity of [ItemJaccard] or more to an item is a match.

// The thresholds of stage 3.
const (
	ItemJaccard = 0.5 // the estimated Jaccard similarity to an item that is a match (AX9.5)
	ItemMin     = 100 // the normalized characters of a part of the body that stage 3 checks
)

const (
	itemSize    = 128 // the values of a fingerprint
	itemShingle = 5   // the characters of a shingle
	itemRows    = 3   // the values of a band of the index of stage 3
)

var (
	itemLabel   = regexp.MustCompile(`\\label\s*\{[^{}]*\}`)
	itemRef     = regexp.MustCompile(`\\(?:ref|eqref|cref|Cref|autoref|pageref|nameref|vref|cpageref|hyperref)\*?\s*(?:\[[^\]]*\])?\s*\{[^{}]*\}`)
	itemCite    = regexp.MustCompile(`\\(?:cite[a-zA-Z]*|nocite)\*?\s*(?:\[[^\]]*\]\s*){0,2}\{[^{}]*\}`)
	itemSpacing = regexp.MustCompile(`\\(?:,|;|:|!|quad|qquad|enspace|thinspace|medspace|thickspace|hfill|hspace\*?\s*\{[^{}]*\}|vspace\*?\s*\{[^{}]*\}|noindent|medskip|smallskip|bigskip|par\b|newline|\\)|~`)
	itemBlank   = regexp.MustCompile(`\s+`)
)

// itemNormalize is the normalization of the fingerprint of arxiv-reader, with no macros to expand: a body that an agent wrote has no macros of a paper.
func itemNormalize(s string) string {
	s = norm.NFC.String(s)
	s = itemLabel.ReplaceAllString(s, " ")
	s = itemRef.ReplaceAllString(s, " \\REF ")
	s = itemCite.ReplaceAllString(s, " \\CITE ")
	s = itemSpacing.ReplaceAllString(s, " ")
	s = itemBlank.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// itemSignature is the fingerprint of the normalized text, as arxiv-reader computes it.
func itemSignature(normalized string) [itemSize]uint16 {
	var mins [itemSize]uint64
	for i := range mins {
		mins[i] = math.MaxUint64
	}
	r := []rune(normalized)
	n := max(len(r)-itemShingle+1, 1)
	for i := range n {
		f := fnv.New64a()
		_, _ = f.Write([]byte(string(r[i:min(i+itemShingle, len(r))])))
		x := f.Sum64()
		for k := range mins {
			if v := itemMix(x, uint64(k)); v < mins[k] {
				mins[k] = v
			}
		}
	}
	var out [itemSize]uint16
	for k, v := range mins {
		out[k] = uint16(v) // #nosec G115 -- the fingerprint keeps the low 16 bits of each minimum
	}
	return out
}

func itemMix(x, k uint64) uint64 {
	x ^= (k + 1) * 0x9e3779b97f4a7c15
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

// itemMinHash is the fingerprint in the form of the object records: the 256 bytes in base64.
func itemMinHash(sig [itemSize]uint16) string {
	b := make([]byte, 2*itemSize)
	for i, v := range sig {
		binary.BigEndian.PutUint16(b[2*i:], v)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// parseItem reads a fingerprint in the form of the object records.
func parseItem(s string) ([itemSize]uint16, bool) {
	var out [itemSize]uint16
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != 2*itemSize {
		return out, false
	}
	for i := range out {
		out[i] = binary.BigEndian.Uint16(b[2*i:])
	}
	return out, true
}

// itemSimilarity estimates the Jaccard similarity of two fingerprints, with the correction for the chance agreement of values of 16 bits.
func itemSimilarity(a, b *[itemSize]uint16) float64 {
	same := 0
	for i := range a {
		if a[i] == b[i] {
			same++
		}
	}
	const c = 1.0 / 65536
	return max((float64(same)/itemSize-c)/(1-c), 0)
}

// itemBand is the key of the band b of a fingerprint in the index of stage 3.
func itemBand(b int, sig *[itemSize]uint16) uint64 {
	k := uint64(b) << 48 // #nosec G115 -- a band is not negative
	for _, v := range sig[b*itemRows : (b+1)*itemRows] {
		k = k<<16 ^ uint64(v)
	}
	return k ^ uint64(b)<<56 // #nosec G115 -- a band is not negative
}

// addItem adds the fingerprint of the text t to the index of stage 3.
func (c *Corpus) addItem(t int, sig [itemSize]uint16) {
	k := len(c.items)
	c.items = append(c.items, item{text: t, sig: sig})
	for b := range itemSize / itemRows {
		key := itemBand(b, &sig)
		c.itemIx[key] = append(c.itemIx[key], k)
	}
}

// item is the fingerprint of a reference that stage 3 reads.
type item struct {
	text int
	sig  [itemSize]uint16
}

// fingerprints is stage 3. It finds each part of the section, the whole section or a paragraph, whose fingerprint is near the fingerprint of an item, and gives the best part for each item.
func (c *Corpus) fingerprints(s Section) []Finding {
	if len(c.items) == 0 {
		return nil
	}
	parts := []paragraph{{text: s.Text, start: 0, end: len(s.Text)}}
	if ps := paragraphs(s.Text); len(ps) > 1 {
		parts = append(parts, ps...)
	}
	best := map[int]Finding{}
	for _, p := range parts {
		n := itemNormalize(p.text)
		if utf8.RuneCountInString(n) < ItemMin {
			continue
		}
		sig := itemSignature(n)
		cands := map[int]bool{}
		for b := range itemSize / itemRows {
			for _, k := range c.itemIx[itemBand(b, &sig)] {
				cands[k] = true
			}
		}
		for _, k := range slices.Sorted(maps.Keys(cands)) {
			it := &c.items[k]
			j := itemSimilarity(&sig, &it.sig)
			if j < ItemJaccard {
				continue
			}
			f := c.finding(s.Field, p.start, p.end, 3, it.text)
			f.Match, f.Similarity = true, j
			if old, ok := best[it.text]; !ok || j > old.Similarity {
				best[it.text] = f
			}
		}
	}
	var out []Finding
	for _, t := range slices.Sorted(maps.Keys(best)) {
		out = append(out, best[t])
	}
	return out
}
