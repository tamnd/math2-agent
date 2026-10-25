// Package tag mints node tags and changes provisional ids to tags.
//
// A tag has 6 characters of Crockford base32: the digits and the capital letters without I, L, O and U. Tags that start with 0 are for the M1 seed only. Mint draws tags at random, so two branches can mint tags without coordination. Rule T02 finds a collision at merge.
//
// Mint does not give a tag that YAML reads as a number, such as 123456 or 12E345. It does not give a tag that starts with an MSC code, such as 20D60X. A bare tag then resolves to one type of thing only (spec 04.3).
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
)

// Alphabet is Crockford base32 in value order.
const Alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Len is the number of characters in a tag.
const Len = 6

var (
	tagRE         = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{6}$`)
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

// IsTag reports whether s is a well-formed tag.
func IsTag(s string) bool { return tagRE.MatchString(s) }

// IsProvisional reports whether s is a well-formed provisional id.
func IsProvisional(s string) bool { return provisionalRE.MatchString(s) }

// IsSeed reports whether s is a tag of the M1 seed.
func IsSeed(s string) bool { return IsTag(s) && s[0] == '0' }

// Excluded reports whether Mint must not give s. The schema accepts such a tag, but no new node gets one.
func Excluded(s string) bool { return numberRE.MatchString(s) || mscRE.MatchString(s) }

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
	var buf [4]byte
	for range maxTries {
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return "", fmt.Errorf("tag: read random bytes: %w", err)
		}
		v := binary.BigEndian.Uint32(buf[:])
		if seed {
			v &= 1<<25 - 1 // The first character is then 0.
		}
		t := encode(v)
		if (t[0] == '0') != seed || Excluded(t) || (used != nil && used(t)) {
			continue
		}
		return t, nil
	}
	return "", ErrExhausted
}

// encode writes the low 30 bits of v as 6 characters.
func encode(v uint32) string {
	var b [Len]byte
	for i := Len - 1; i >= 0; i-- {
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

// Shard returns the directory below nodes/ for a tag or a provisional id.
func Shard(id string) string {
	if IsProvisional(id) {
		return "~"
	}
	return id[:2]
}
