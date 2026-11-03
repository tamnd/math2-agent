package wpcorpus

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/tamnd/math2-agent/msc"
)

// MSCRule maps the words of a category title or of a context phrase to an
// MSC code (WP7.3).
type MSCRule struct {
	Words string `yaml:"words"`
	Code  string `yaml:"code"`
}

// checkMSC checks the rules of the msc list and normalizes their words.
func (t *CategoryTree) checkMSC() error {
	seen := map[string]bool{}
	for i, r := range t.MSC {
		w := mscWords(r.Words)
		if w == "" || !msc.Valid(r.Code) {
			return fmt.Errorf("categories.yaml: msc rule %d needs words and a valid MSC code", i+1)
		}
		if seen[w] {
			return fmt.Errorf("categories.yaml: msc rule %d: the words %q are used twice", i+1, r.Words)
		}
		seen[w] = true
		t.MSC[i].Words = w
	}
	return nil
}

// mscWords lowers the text and keeps the letters, the digits and the
// hyphens, with one space between the words.
func mscWords(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-'
	}), " ")
}

// MSCOf gives the code of the rule with the longest words that the text has
// as whole words. It gives the empty string when no rule matches. When two
// rules have words of the same length, the first rule wins.
func (t *CategoryTree) MSCOf(text string) string {
	s := " " + mscWords(text) + " "
	code, best := "", 0
	for _, r := range t.MSC {
		if len(r.Words) > best && strings.Contains(s, " "+r.Words+" ") {
			code, best = r.Code, len(r.Words)
		}
	}
	return code
}

var (
	mdLink       = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	leadContinue = regexp.MustCompile(`(?i)^(more |most )?(specifically|particularly|especially|in particular|notably|namely|in |and in |or in |within )`)
)

// LeadParagraph gives the first paragraph of the lead of a Markdown
// rendering (WP3.12), with the links replaced by their text. It gives the
// empty string when the rendering has no lead block.
func LeadParagraph(markdown string) string {
	lines := strings.Split(markdown, "\n")
	for i, l := range lines {
		if !strings.HasPrefix(l, ":::") || !strings.Contains(l, "cue=lead") {
			continue
		}
		for _, p := range lines[i+1:] {
			if p = strings.TrimSpace(p); p != "" {
				if strings.HasPrefix(p, ":::") {
					return ""
				}
				return strings.ReplaceAll(mdLink.ReplaceAllString(p, "$1"), "**", "")
			}
		}
	}
	return ""
}

// ContextPhrase gives the context phrase of a lead (WP3.6): the words from
// "In" to the first comma, with the parts that narrow it, as in "In
// mathematics, specifically in algebraic topology". It gives the empty
// string when the lead does not start with "In ".
func ContextPhrase(lead string) string {
	if !strings.HasPrefix(lead, "In ") {
		return ""
	}
	s := lead
	var parts []string
	for len(parts) < 4 {
		j := strings.IndexAny(s, ",.;:")
		if j < 0 {
			break
		}
		parts = append(parts, strings.TrimSpace(s[:j]))
		if s[j] != ',' {
			break
		}
		s = strings.TrimSpace(s[j+1:])
		if !leadContinue.MatchString(s) {
			break
		}
	}
	return strings.Join(parts, ", ")
}

// MSCResult is the MSC of one article from the signals that need no model.
type MSCResult struct {
	// Codes are the codes with the most votes. More than one code is a tie
	// that the model pass decides.
	Codes []string
	// Categories and Lead are the codes that the categories and the
	// context phrase gave.
	Categories map[string]int
	Lead       string
}

// ClassifyMSC gives the MSC of an article from its categories and the
// context phrase of its lead (WP7.3). Each category that a rule maps gives
// one vote, and the context phrase gives two votes, because it names the
// area of the article and a category can name a side topic.
func ClassifyMSC(t *CategoryTree, md *Metadata, lead string) MSCResult {
	res := MSCResult{Categories: map[string]int{}}
	votes := map[string]int{}
	for _, c := range md.Categories {
		if code := t.MSCOf(c); code != "" {
			res.Categories[code]++
			votes[code]++
		}
	}
	if p := ContextPhrase(lead); p != "" {
		if res.Lead = t.MSCOf(p); res.Lead != "" {
			votes[res.Lead] += 2
		}
	}
	top := 0
	for _, n := range votes {
		top = max(top, n)
	}
	for code, n := range votes {
		if n == top {
			res.Codes = append(res.Codes, code)
		}
	}
	slices.SortFunc(res.Codes, msc.Compare)
	return res
}

// ReadLead gives the lead paragraph of the Markdown rendering of a page, or
// the empty string when the page has no rendering.
func ReadLead(root string, page int) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, MarkdownPath(page))) // #nosec G304 -- the path is in the corpus that the user names
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return LeadParagraph(string(data)), nil
}
