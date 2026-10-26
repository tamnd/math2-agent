// Package tag mints node tags and changes provisional ids to tags.
//
// A tag has 10 characters of Crockford base32: the digits and the capital letters without I, L, O and U. The first 9 characters are random, and the last one is a check character (corpus spec K3.3). The tags of 6 characters that math2 minted before the change are legacy tags. They have no check character, and they stay valid. Tags that start with 0 are for the seed only. Mint draws tags at random, so two branches can mint tags without coordination. Rule T02 finds a collision at merge.
//
// Mint does not give a tag that YAML reads as a number, such as 1234567890 or 12E3456789. It does not give a tag that starts with an MSC code, such as 20D60XXXXX, or a tag with fewer than two letters. A bare tag then resolves to one type of thing only (spec 04.3).
//
// A provisional id is ~ and 8 lowercase hex characters, for example ~3fa94c01. A node has a provisional id until it merges.
package tag

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Alphabet is Crockford base32 in value order.
const Alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Len is the number of characters in a tag that Mint gives.
const Len = 10

// LegacyLen is the number of characters in a legacy tag.
const LegacyLen = 6

var (
	longRE        = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{10}$`)
	legacyRE      = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{6}$`)
	provisionalRE = regexp.MustCompile(`^~[0-9a-f]{8}$`)
	// ProvisionalInText finds provisional ids in a file.
	ProvisionalInText = regexp.MustCompile(`~[0-9a-f]{8}`)

	numberRE = regexp.MustCompile(`^[0-9]+(E[0-9]+)?$`)
	mscRE    = regexp.MustCompile(`^[0-9]{2}[A-Z][0-9]{2}`)
)

// maxTries is the number of draws before Mint stops. With fewer than 100 million tags in use, 64 draws fail with a chance below 1 in 10^24.
const maxTries = 64

// ErrExhausted means that Mint drew maxTries tags and each one was in use or excluded.
var ErrExhausted = errors.New("tag: no free tag found")

// ErrCheck means that the check character of a tag of 10 characters does not match the other characters.
var ErrCheck = errors.New("not a valid tag: the check character does not match")

// IsTag reports whether s is a well-formed tag: a tag of 10 characters with a valid check character, or a legacy tag.
func IsTag(s string) bool { return IsLegacy(s) || longRE.MatchString(s) && valid(s) }

// IsLegacy reports whether s has the form of a legacy tag of 6 characters. Only the legacy set of registers/tags.reg holds such tags, and rule T10 checks that no new one is minted.
func IsLegacy(s string) bool { return legacyRE.MatchString(s) }

// Check returns the check character of a payload of 9 characters, by the Luhn mod 32 algorithm (corpus spec K3.3). Check returns 0 when the payload has a character that is not in the alphabet.
func Check(payload string) byte {
	sum, ok := luhn(payload, 2)
	if !ok {
		return 0
	}
	return Alphabet[(32-sum%32)%32]
}

// valid reports whether the last character of s is the check character of the others.
func valid(s string) bool {
	sum, ok := luhn(s, 1)
	return ok && sum%32 == 0
}

// luhn returns the Luhn mod 32 sum of s. The factor of the last character is f, and the factors alternate between 2 and 1 to the left.
func luhn(s string, f int) (int, bool) {
	sum := 0
	for i := len(s) - 1; i >= 0; i-- {
		v := strings.IndexByte(Alphabet, s[i])
		if v < 0 {
			return 0, false
		}
		p := v * f
		sum += p/32 + p%32
		f = 3 - f
	}
	return sum, true
}

// Normalize changes s to the canonical form of a tag (corpus spec K3.4). It removes hyphens and spaces, changes lower case to upper case, and reads I and L as 1 and O as 0. Then it validates the result. A tag of 10 characters with a wrong check character gives ErrCheck. Normalize never gives the nearest valid tag, because that can be another node.
func Normalize(s string) (string, error) {
	t := strings.Map(func(r rune) rune {
		switch r {
		case '-', ' ':
			return -1
		case 'I', 'i', 'L', 'l':
			return '1'
		case 'O', 'o':
			return '0'
		}
		if 'a' <= r && r <= 'z' {
			return r - 'a' + 'A'
		}
		return r
	}, s)
	switch {
	case IsLegacy(t):
		return t, nil
	case longRE.MatchString(t) && !valid(t):
		return "", ErrCheck
	case longRE.MatchString(t):
		return t, nil
	}
	return "", fmt.Errorf("not a valid tag: %q", s)
}

// Display returns the tag in two groups of five characters, as printed works show it, for example 7K3QX-2M9R2. A legacy tag does not change. The hyphen is not part of the tag.
func Display(t string) string {
	if len(t) != Len {
		return t
	}
	return t[:5] + "-" + t[5:]
}

// IsProvisional reports whether s is a well-formed provisional id.
func IsProvisional(s string) bool { return provisionalRE.MatchString(s) }

// IsSeed reports whether s is a tag of the M1 seed.
func IsSeed(s string) bool { return IsTag(s) && s[0] == '0' }

// Excluded reports whether Mint must not give s. The schema accepts such a tag, but no new node gets one. A tag of 10 characters is also excluded when it has fewer than two letters, because it could then read as an ISBN or another numeric id.
func Excluded(s string) bool {
	if numberRE.MatchString(s) || mscRE.MatchString(s) {
		return true
	}
	letters := 0
	for i := range len(s) {
		if 'A' <= s[i] && s[i] <= 'Z' {
			letters++
		}
	}
	return len(s) == Len && letters < 2
}

// Mint draws a new tag that does not start with 0. The used function reports whether a tag exists or existed. If r is nil, Mint uses crypto/rand.
func Mint(r io.Reader, used func(string) bool) (string, error) {
	return mint(r, used, false)
}

// MintSeed draws a new tag that starts with 0. Use it only for the M1 seed.
func MintSeed(r io.Reader, used func(string) bool) (string, error) {
	return mint(r, used, true)
}

func mint(r io.Reader, used func(string) bool, seed bool) (string, error) {
	if r == nil {
		r = rand.Reader
	}
	var buf [8]byte
	for range maxTries {
		if _, err := io.ReadFull(r, buf[2:]); err != nil {
			return "", fmt.Errorf("tag: read random bytes: %w", err)
		}
		v := binary.BigEndian.Uint64(buf[:])
		if seed {
			v &= 1<<40 - 1 // The first character is then 0.
		}
		p := encode(v)
		t := p + string(Check(p))
		if (t[0] == '0') != seed || Excluded(t) || (used != nil && used(t)) {
			continue
		}
		return t, nil
	}
	return "", ErrExhausted
}

// encode writes the low 45 bits of v as 9 characters.
func encode(v uint64) string {
	var b [Len - 1]byte
	for i := len(b) - 1; i >= 0; i-- {
		b[i] = Alphabet[v&31]
		v >>= 5
	}
	return string(b[:])
}

// Provisional draws a new provisional id. If r is nil, Provisional uses crypto/rand.
func Provisional(r io.Reader) (string, error) {
	if r == nil {
		r = rand.Reader
	}
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return "", fmt.Errorf("tag: read random bytes: %w", err)
	}
	return "~" + hex.EncodeToString(buf[:]), nil
}

// ShardLen is the number of characters of a tag that name its shard (corpus spec K3.5).
const ShardLen = 3

// Shard returns the directory below nodes/, prov/ and reviews/ for a tag or a provisional id: the first three characters of a tag, for long and legacy tags, or ~ for a provisional id.
func Shard(id string) string {
	if IsProvisional(id) {
		return "~"
	}
	return id[:ShardLen]
}
