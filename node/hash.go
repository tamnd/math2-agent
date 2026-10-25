package node

import (
	"encoding/hex"
	"strings"

	"github.com/zeebo/blake3"
	"golang.org/x/text/unicode/norm"
)

// HashPrefix starts each content hash.
const HashPrefix = "blake3:"

// Hash returns the content hash of the node (spec 06.3). The hash is "blake3:" and 64 lowercase hex characters. It is BLAKE3 with a 32-byte output over these bytes:
//
//  1. The line ---.
//  2. The front matter in YAML. The keys of each mapping are in byte order. There are no comments. Each list uses block style. The indent is two spaces. The scalars follow the rules of Format.
//  3. The line ---.
//  4. The body in Unicode NFC, with LF line endings. Each line has no trailing spaces or tabs. The body has no empty lines at the start or at the end. A body that is not empty ends with one newline.
//
// Thus the hash does not change when only the key order, the comments, the list style, the quotes or the trailing whitespace change. The hash changes when a value or a word of the body changes.
func Hash(n *Node) string {
	front := canonical(n.Front, nil, "", style{bare: true, block: true})
	h := blake3.New()
	_, _ = h.WriteString("---\n")
	_, _ = h.Write(encode(front))
	_, _ = h.WriteString("---\n")
	_, _ = h.WriteString(hashBody(n.Body))
	return HashPrefix + hex.EncodeToString(h.Sum(nil))
}

// hashBody returns the body in the form that the hash uses.
func hashBody(body string) string {
	lines := strings.Split(norm.NFC.String(lf(body)), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	out := strings.Trim(strings.Join(lines, "\n"), "\n")
	if out == "" {
		return ""
	}
	return out + "\n"
}

// HashText returns the hash of a text, for example one section of a body. The text has the form that Hash uses for the body: Unicode NFC, LF line endings, no trailing spaces or tabs on a line, and no empty lines at the start or at the end.
func HashText(text string) string {
	sum := blake3.Sum256([]byte(hashBody(text)))
	return HashPrefix + hex.EncodeToString(sum[:])
}
