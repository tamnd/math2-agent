package ingest

import (
	"hash/fnv"
	"slices"
	"strings"
	"unicode"
)

// The parameters of the fingerprints. Two texts that share a passage of Window+Shingle-1 words or more share at least one fingerprint.
const (
	Shingle = 8 // the words in each shingle
	Window  = 4 // the shingles in each window of the winnowing
)

// Fingerprints returns the fingerprints of a text, in increasing order and without repeats. It is the winnowing method of Schleimer, Wilkerson and Aiken (2003): the text becomes lowercase words, each run of Shingle words gets a 32-bit hash, and each window of Window hashes gives its smallest hash. The fingerprints cannot give the text back. A text of fewer than Shingle words has no fingerprints.
func Fingerprints(text string) []uint32 {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(words) < Shingle {
		return nil
	}
	hashes := make([]uint32, len(words)-Shingle+1)
	for i := range hashes {
		h := fnv.New32a()
		for _, w := range words[i : i+Shingle] {
			_, _ = h.Write([]byte(w))
			_, _ = h.Write([]byte{' '})
		}
		hashes[i] = h.Sum32()
	}
	var out []uint32
	last := -1
	for start := 0; start+Window <= len(hashes) || start == 0; start++ {
		end := min(start+Window, len(hashes))
		pick := start
		for i := start; i < end; i++ {
			if hashes[i] <= hashes[pick] {
				pick = i
			}
		}
		if pick != last {
			out = append(out, hashes[pick])
			last = pick
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Shared returns the number of fingerprints that a and b have in common. Each list must be in increasing order without repeats, as Fingerprints returns it.
func Shared(a, b []uint32) int {
	n := 0
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i] < b[j]:
			i++
		case a[i] > b[j]:
			j++
		default:
			n++
			i++
			j++
		}
	}
	return n
}
