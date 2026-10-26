// Package copycheck finds the parts of a body that copy a reference text (corpus K9.4).
//
// Models reproduce text that they saw in training, even when nobody asks them to. Thus the copy check compares each body that an agent wrote with a reference corpus, and not only with the sources of the node: a model that writes from memory cites nothing. The check has two stages. Stage 1 finds the exact runs. A run of [FlagRun] normalized characters or more is a flag. A run of [MatchRun] characters or more in one source, or flags that cover [MatchCover] of a section or more, is a match. Stage 2 finds the near copies of each paragraph with MinHash on word 5-grams. A pair with an estimated Jaccard similarity of [NearJaccard] or more is a match when it also has a common block of [NearBlock] words or more, or a ROUGE-L of [NearRouge] or more. Stage 2 skips a paragraph of the body with fewer than [NearBlock] words, because the ROUGE-L of a short paragraph says little. Stage 1 checks it.
//
// The check normalizes each text first: lower case, one space for each run of white space, no Markdown marks, and one placeholder for each formula, thus a standard formula does not count as a copy. A standard phrase, such as "let $G$ be a group", does not count either. Only a curator adds a phrase to the list.
//
// A match with a source of the class record, verbatim, share-alike-nc or gfdl blocks the merge, and the agent gets the places to write again. A match with an open source is allowed, and the node then needs the attribution of rule L03. The check uses no network and no model, and the same input gives the same report.
//
// The thresholds are a start. Before rule L05 blocks a merge, the calibration of corpus K9.4 sets them on text that people wrote.
package copycheck

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"index/suffixarray"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/prov"
)

// The thresholds of corpus K9.4.
const (
	FlagRun     = 50   // the normalized characters of an exact run that is a flag
	MatchRun    = 100  // the normalized characters of an exact run in one source that is a match
	MatchCover  = 0.10 // the share of a section that flags must cover to be matches
	NearJaccard = 0.5  // the estimated Jaccard similarity of a candidate near copy
	NearBlock   = 20   // the words of a common block that make a candidate a match
	NearRouge   = 0.8  // the ROUGE-L that makes a candidate a match
	Shingle     = 5    // the words of a shingle of stage 2
)

// Version is the version of the check. The run record names it with each result.
const Version = "copycheck/1"

// The MinHash signature has hashes values, in bands of rows values each. Two paragraphs are candidates when they agree on one band. With 32 bands of 2 values, a pair at the Jaccard similarity 0.5 is a candidate with a probability above 0.9999.
const (
	hashes = 64
	rows   = 2
	bands  = hashes / rows
)

// The bytes that the normalized text uses for itself. The normalized text of a body or a reference never has them otherwise.
const (
	sep     = 0x00 // between two reference texts
	formula = 0x02 // a formula
	ownMask = 0x03 // a standard phrase in a body
	refMask = 0x04 // a standard phrase in a reference text. It differs from ownMask, thus no run goes through a phrase
)

// Text is one text of the reference corpus.
type Text struct {
	Source  string       `json:"source"`            // the source, for example wikipedia
	URI     string       `json:"uri,omitempty"`     // the page or the item in the source
	Licence string       `json:"licence,omitempty"` // the SPDX identifier of the licence
	Class   ingest.Class `json:"class,omitempty"`   // the access class. Empty means the class of the licence
	Text    string       `json:"text"`
}

// Section is one body field of a node, for example body.proof, with its text.
type Section struct {
	Field string
	Text  string
}

// Finding is a flag or a match.
type Finding struct {
	prov.CopyMatch
	Match      bool    // the finding is a match. A flag that is not a match is false
	Length     int     // the normalized characters of an exact run, or the words of the longest common block of a near copy
	Similarity float64 // the estimated Jaccard similarity of a near copy, or 0 for an exact run
}

// Report is the result of a check.
type Report struct {
	Findings []Finding // in the order of the field, the place, the stage and the source
}

// Matches returns the matches of the report.
func (r *Report) Matches() []prov.CopyMatch {
	var out []prov.CopyMatch
	for _, f := range r.Findings {
		if f.Match {
			out = append(out, f.CopyMatch)
		}
	}
	return out
}

// Blocks reports whether a match of the report blocks the merge.
func (r *Report) Blocks() bool {
	return slices.ContainsFunc(r.Findings, func(f Finding) bool { return f.Match && f.Blocks() })
}

// Attribution returns the URIs of the open sources that the body matches, or their sources when they have no URI, sorted. The node must give each one the attribution of rule L03.
func (r *Report) Attribution() []string {
	var out []string
	for _, f := range r.Findings {
		if f.Match && !f.Blocks() {
			out = append(out, cmp.Or(f.URI, f.Source))
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Record returns the result of the check as the run record keeps it.
func (r *Report) Record(node string) prov.CopyCheck {
	return prov.CopyCheck{Node: node, Version: Version, Matches: r.Matches()}
}

// Corpus is a reference corpus with its index. A corpus is safe for use by more than one goroutine.
type Corpus struct {
	texts   []Text
	phrases [][]byte // the normalized standard phrases, the longest first
	norm    []byte   // the normalized texts, each one after a sep byte
	starts  []int    // the start of each text in norm
	index   *suffixarray.Index
	paras   []para
	bandIx  map[uint64][]int // the paragraphs with a band, by the hash of the band
}

// para is a paragraph of a reference text, for stage 2.
type para struct {
	text  int
	words []string
	sig   *[hashes]uint64
}

// New makes the corpus of the texts, with the standard phrases. A text with no class gets the class of its licence, and a text with a class that is not valid gets the class record, the most closed one.
func New(texts []Text, phrases []string) *Corpus {
	c := &Corpus{texts: slices.Clone(texts), bandIx: map[uint64][]int{}}
	for _, p := range phrases {
		if n := normalize(p).b; len(bytes.TrimSpace(n)) > 0 {
			c.phrases = append(c.phrases, n)
		}
	}
	slices.SortStableFunc(c.phrases, func(a, b []byte) int { return cmp.Compare(len(b), len(a)) })
	for i := range c.texts {
		t := &c.texts[i]
		if t.Class == "" {
			t.Class = ingest.ClassOf(t.Licence)
		}
		if !t.Class.Valid() {
			t.Class = ingest.RecordOnly
		}
		n := c.mask(normalize(t.Text), refMask)
		c.norm = append(c.norm, sep)
		c.starts = append(c.starts, len(c.norm))
		c.norm = append(c.norm, n.b...)
		for _, p := range paragraphs(t.Text) {
			w := c.words(p.text)
			sig := signature(w)
			if sig == nil {
				continue
			}
			k := len(c.paras)
			c.paras = append(c.paras, para{text: i, words: w, sig: sig})
			for b := range bands {
				key := bandKey(b, sig)
				c.bandIx[key] = append(c.bandIx[key], k)
			}
		}
	}
	c.norm = append(c.norm, sep)
	c.index = suffixarray.New(c.norm)
	return c
}

// FromRecords returns the texts of the records of a source that have text. The licence gate keeps the text of open records only, thus the texts of the other classes come from the files of ReadDir.
func FromRecords(source string, records []ingest.Record) []Text {
	var out []Text
	for _, r := range records {
		if r.Text != "" {
			out = append(out, Text{Source: source, URI: cmp.Or(r.URL, source+":"+r.ID), Licence: r.Licence, Class: r.Class, Text: r.Text})
		}
	}
	return out
}

// ReadDir reads the reference texts in the files *.jsonl of the directory, one Text in JSON on each line, in the order of the files and the lines.
func ReadDir(dir string) ([]Text, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	var out []Text
	for _, f := range files {
		data, err := os.ReadFile(f) // #nosec G304 -- the file is in the directory that the caller gives.
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(nil, 64<<20)
		for line := 1; sc.Scan(); line++ {
			if len(bytes.TrimSpace(sc.Bytes())) == 0 {
				continue
			}
			var t Text
			if err := json.Unmarshal(sc.Bytes(), &t); err != nil {
				return nil, fmt.Errorf("copycheck: %s:%d: %w", f, line, err)
			}
			out = append(out, t)
		}
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("copycheck: %s: %w", f, err)
		}
	}
	return out, nil
}

// ReadPhrases reads a list of standard phrases: one phrase on each line. An empty line and a line that starts with # are skipped.
func ReadPhrases(path string) ([]string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the caller gives the path of the list.
	if err != nil {
		return nil, err
	}
	var out []string
	for l := range strings.SplitSeq(string(data), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out, nil
}

// Check checks the sections of a body against the corpus.
func (c *Corpus) Check(sections ...Section) *Report {
	r := &Report{}
	for _, s := range sections {
		r.Findings = append(r.Findings, c.exact(s)...)
		r.Findings = append(r.Findings, c.near(s)...)
	}
	slices.SortFunc(r.Findings, func(a, b Finding) int {
		return cmp.Or(strings.Compare(a.Field, b.Field), cmp.Compare(a.Start, b.Start), cmp.Compare(a.Stage, b.Stage), cmp.Compare(a.End, b.End), strings.Compare(a.Source, b.Source), strings.Compare(a.URI, b.URI))
	})
	return r
}

// textAt returns the text whose normalized form has the byte at offset in c.norm.
func (c *Corpus) textAt(offset int) int {
	return sort.SearchInts(c.starts, offset+1) - 1
}

// finding returns a finding on the text t.
func (c *Corpus) finding(field string, start, end, stage, t int) Finding {
	x := c.texts[t]
	return Finding{CopyMatch: prov.CopyMatch{Field: field, Start: start, End: end, Stage: stage, Source: x.Source, URI: x.URI, Licence: x.Licence, Class: string(x.Class)}}
}

// span is a part of a normalized body, from start to end.
type span struct{ start, end int }

// exact is stage 1. It finds each run of FlagRun characters or more that the section has in common with a reference text. The runs of one text that overlap are one finding.
func (c *Corpus) exact(s Section) []Finding {
	n := c.mask(normalize(s.Text), ownMask)
	b := n.b
	runs := map[int][]span{} // by text
	done := map[[2]int]int{} // the end of the run of each text and diagonal that is found
	for i := 0; i+FlagRun <= len(b); i++ {
		for _, o := range c.index.Lookup(b[i:i+FlagRun], -1) {
			t := c.textAt(o)
			key := [2]int{t, o - i}
			if done[key] > i {
				continue
			}
			k := FlagRun
			for i+k < len(b) && o+k < len(c.norm) && b[i+k] == c.norm[o+k] {
				k++
			}
			done[key] = i + k
			runs[t] = append(runs[t], span{i, i + k})
		}
	}
	var all []span
	merged := map[int][]span{}
	for t, rs := range runs {
		merged[t] = union(rs)
		all = append(all, rs...)
	}
	covered := 0
	for _, x := range union(all) {
		covered += x.end - x.start
	}
	cover := len(b) > 0 && float64(covered) >= MatchCover*float64(len(b))
	var out []Finding
	for _, t := range slices.Sorted(maps.Keys(merged)) {
		for _, x := range merged[t] {
			f := c.finding(s.Field, n.off[x.start], n.off[x.end], 1, t)
			f.Length = x.end - x.start
			f.Match = cover || f.Length >= MatchRun
			out = append(out, f)
		}
	}
	return out
}

// near is stage 2. It finds each paragraph of the section that is a near copy of a paragraph of a reference text.
func (c *Corpus) near(s Section) []Finding {
	var out []Finding
	for _, p := range paragraphs(s.Text) {
		w := c.words(p.text)
		if len(w) < NearBlock {
			continue // a short paragraph is for stage 1
		}
		sig := signature(w)
		cands := map[int]bool{}
		for b := range bands {
			for _, k := range c.bandIx[bandKey(b, sig)] {
				cands[k] = true
			}
		}
		best := map[int]Finding{} // the best near copy of each text
		for _, k := range slices.Sorted(maps.Keys(cands)) {
			q := c.paras[k]
			j := similarity(sig, q.sig)
			if j < NearJaccard {
				continue
			}
			block, lcs := common(w, q.words)
			rouge := 0.0
			if lcs > 0 {
				prec, rec := float64(lcs)/float64(len(w)), float64(lcs)/float64(len(q.words))
				rouge = 2 * prec * rec / (prec + rec)
			}
			if block < NearBlock && rouge < NearRouge {
				continue
			}
			f := c.finding(s.Field, p.start, p.end, 2, q.text)
			f.Match, f.Length, f.Similarity = true, block, j
			if old, ok := best[q.text]; !ok || f.Similarity > old.Similarity {
				best[q.text] = f
			}
		}
		for _, t := range slices.Sorted(maps.Keys(best)) {
			out = append(out, best[t])
		}
	}
	return out
}

// union returns the union of the spans, sorted, with the spans that overlap or touch as one.
func union(xs []span) []span {
	xs = slices.Clone(xs)
	slices.SortFunc(xs, func(a, b span) int { return cmp.Or(cmp.Compare(a.start, b.start), cmp.Compare(a.end, b.end)) })
	var out []span
	for _, x := range xs {
		if len(out) > 0 && x.start <= out[len(out)-1].end {
			out[len(out)-1].end = max(out[len(out)-1].end, x.end)
			continue
		}
		out = append(out, x)
	}
	return out
}

// normalized is a normalized text, with the offset in the original text of each byte.
type normalized struct {
	b   []byte
	off []int // off[i] is the offset in the original text of b[i]. off has one more entry: the end of the original text
}

// normalize normalizes the text s: each formula becomes one placeholder, the letters go to lower case, each run of white space becomes one space, and the marks *, _ and ` of Markdown go away. A link keeps its text and loses its target.
func normalize(s string) normalized {
	var n normalized
	space := true
	put := func(at int, bs ...byte) {
		for _, c := range bs {
			n.b = append(n.b, c)
			n.off = append(n.off, at)
		}
	}
	for i := 0; i < len(s); {
		if end, ok := formulaEnd(s, i); ok {
			put(i, formula)
			space, i = false, end
			continue
		}
		r, w := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == ']' && strings.HasPrefix(s[i+1:], "("):
			if j := strings.IndexByte(s[i:], ')'); j >= 0 {
				w = j + 1
			}
		case unicode.IsSpace(r):
			if !space {
				put(i, ' ')
				space = true
			}
		case r == '*' || r == '_' || r == '`' || r == '[' || r < 0x20 || r == utf8.RuneError && w == 1:
		default:
			put(i, utf8.AppendRune(nil, unicode.ToLower(r))...)
			space = false
		}
		i += w
	}
	if len(n.b) > 0 && n.b[len(n.b)-1] == ' ' {
		n.b, n.off = n.b[:len(n.b)-1], n.off[:len(n.off)-1]
	}
	n.off = append(n.off, len(s))
	return n
}

// formulaEnd returns the end of the formula that starts at i in s: $$...$$, $...$, \(...\) or \[...\].
func formulaEnd(s string, i int) (int, bool) {
	rest := s[i:]
	for _, d := range [][2]string{{"$$", "$$"}, {`\(`, `\)`}, {`\[`, `\]`}, {"$", "$"}} {
		if !strings.HasPrefix(rest, d[0]) || d[0] == "$" && i > 0 && s[i-1] == '\\' {
			continue
		}
		if j := strings.Index(rest[len(d[0]):], d[1]); j >= 0 {
			return i + len(d[0]) + j + len(d[1]), true
		}
		return 0, false
	}
	return 0, false
}

// mask changes each standard phrase of the normalized text to the byte m. A phrase counts only between word boundaries.
func (c *Corpus) mask(n normalized, m byte) normalized {
	if len(c.phrases) == 0 {
		return n
	}
	var out normalized
	for i := 0; i < len(n.b); {
		hit := 0
		if i == 0 || !wordByte(n.b[i-1]) {
			for _, p := range c.phrases {
				if bytes.HasPrefix(n.b[i:], p) && (i+len(p) == len(n.b) || !wordByte(n.b[i+len(p)])) {
					hit = len(p)
					break
				}
			}
		}
		if hit > 0 {
			out.b, out.off = append(out.b, m), append(out.off, n.off[i])
			i += hit
			continue
		}
		out.b, out.off = append(out.b, n.b[i]), append(out.off, n.off[i])
		i++
	}
	out.off = append(out.off, n.off[len(n.b)])
	return out
}

// wordByte reports whether the byte can be in a word: a letter, a digit or a byte of a rune that is not ASCII.
func wordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c >= 0x80
}

// paragraph is a paragraph of a text, with its place in the text.
type paragraph struct {
	text       string
	start, end int
}

// paragraphs splits a text at the empty lines.
func paragraphs(s string) []paragraph {
	var out []paragraph
	start, end := -1, 0
	for at := 0; at <= len(s); {
		next := strings.IndexByte(s[at:], '\n')
		if next < 0 {
			next = len(s)
		} else {
			next += at
		}
		switch {
		case strings.TrimSpace(s[at:next]) != "":
			if start < 0 {
				start = at
			}
			end = next
		case start >= 0:
			out = append(out, paragraph{s[start:end], start, end})
			start = -1
		}
		at = next + 1
	}
	if start >= 0 {
		out = append(out, paragraph{s[start:end], start, end})
	}
	return out
}

// words returns the normalized words of a paragraph for stage 2. A standard phrase is not in it.
func (c *Corpus) words(s string) []string {
	var out []string
	for w := range strings.FieldsSeq(string(c.mask(normalize(s), ownMask).b)) {
		if !strings.ContainsRune(w, ownMask) {
			out = append(out, w)
		}
	}
	return out
}

// signature returns the MinHash signature of the word shingles, or nil when the words are fewer than a shingle.
func signature(words []string) *[hashes]uint64 {
	if len(words) < Shingle {
		return nil
	}
	var sig [hashes]uint64
	for i := range sig {
		sig[i] = ^uint64(0)
	}
	for i := 0; i+Shingle <= len(words); i++ {
		h := fnv64(strings.Join(words[i:i+Shingle], " "))
		for k := range sig {
			if v := mix(h ^ seeds[k]); v < sig[k] {
				sig[k] = v
			}
		}
	}
	return &sig
}

// similarity returns the share of the values that two signatures have in common, the estimate of the Jaccard similarity.
func similarity(a, b *[hashes]uint64) float64 {
	same := 0
	for i := range a {
		if a[i] == b[i] {
			same++
		}
	}
	return float64(same) / hashes
}

// bandKey returns the hash of the band b of a signature.
func bandKey(b int, sig *[hashes]uint64) uint64 {
	h := mix(seeds[b] + 1)
	for _, v := range sig[b*rows : (b+1)*rows] {
		h = mix(h ^ v)
	}
	return h
}

// common returns the length of the longest common block of words of a and b, and the length of their longest common subsequence.
func common(a, b []string) (block, lcs int) {
	const most = 4000
	a, b = a[:min(len(a), most)], b[:min(len(b), most)]
	run := make([]int, len(b)+1)
	sub := make([]int, len(b)+1)
	for i := range a {
		prevRun, prevSub := 0, 0
		for j := range b {
			r, s := run[j+1], sub[j+1]
			if a[i] == b[j] {
				run[j+1] = prevRun + 1
				sub[j+1] = prevSub + 1
				block = max(block, run[j+1])
			} else {
				run[j+1] = 0
				sub[j+1] = max(sub[j+1], sub[j])
			}
			prevRun, prevSub = r, s
		}
	}
	return block, sub[len(b)]
}

// seeds are the seeds of the hash functions of the signature.
var seeds = func() [hashes]uint64 {
	var s [hashes]uint64
	for i := range s {
		s[i] = mix(uint64(i)*0x9e3779b97f4a7c15 + 0x632be59bd9b4e019)
	}
	return s
}()

// fnv64 returns the FNV-1a hash of s.
func fnv64(s string) uint64 {
	h := uint64(14695981039346656037)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

// mix is the finalizer of SplitMix64.
func mix(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}
