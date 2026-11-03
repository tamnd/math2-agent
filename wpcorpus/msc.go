package wpcorpus

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
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
	leadLink     = regexp.MustCompile(`\]\(wikipedia://en/([0-9]+)\)`)
	leadContinue = regexp.MustCompile(`(?i)^(more |most )?(specifically|particularly|especially|in particular|notably|namely|in |and in |or in |within )`)
)

// LeadParagraph gives the first paragraph of the lead of a Markdown
// rendering (WP3.12), with the links replaced by their text. It gives the
// empty string when the rendering has no lead block.
func LeadParagraph(markdown string) string {
	return strings.ReplaceAll(mdLink.ReplaceAllString(rawLead(markdown), "$1"), "**", "")
}

// LeadLinks gives the page ids of the articles that the first paragraph of
// the lead links to, in the order of the text, with no page twice.
func LeadLinks(markdown string) []int {
	var out []int
	for _, m := range leadLink.FindAllStringSubmatch(rawLead(markdown), -1) {
		if id, err := strconv.Atoi(m[1]); err == nil && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// rawLead gives the first paragraph of the lead block as it is written.
func rawLead(markdown string) string {
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
				return p
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
	// Links are the votes of the articles that the lead links to. They count
	// only when the categories and the context phrase give no code or a tie.
	Links map[string]int
}

// ClassifyMSC gives the MSC of an article from its categories, the context
// phrase of its lead and the links of its lead (WP7.3). Each category that a
// rule maps gives one vote, and the context phrase gives two votes, because
// it names the area of the article and a category can name a side topic.
// When these give no code or a tie, each code in linked gives one vote. The
// caller gives the codes of the articles that the lead links to, one code
// for each link. With a tie, only the codes of the tie get the votes of the
// links, so a link cannot add a new area to a tie.
func ClassifyMSC(t *CategoryTree, md *Metadata, lead string, linked []string) MSCResult {
	res := MSCResult{Categories: map[string]int{}, Links: map[string]int{}}
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
	res.Codes = topCodes(votes)
	if len(res.Codes) == 1 {
		return res
	}
	tie := len(res.Codes) > 1
	for _, code := range linked {
		if !tie || slices.Contains(res.Codes, code) {
			res.Links[code]++
			votes[code]++
		}
	}
	if len(res.Links) > 0 {
		res.Codes = topCodes(votes)
	}
	return res
}

// topCodes gives the codes with the most votes, in the order of the
// classification.
func topCodes(votes map[string]int) []string {
	top := 0
	for _, n := range votes {
		top = max(top, n)
	}
	var out []string
	for code, n := range votes {
		if n == top {
			out = append(out, code)
		}
	}
	slices.SortFunc(out, msc.Compare)
	return out
}

// ReadLead gives the lead paragraph of the Markdown rendering of a page and
// the pages that it links to. It gives nothing when the page has no
// rendering.
func ReadLead(root string, page int) (string, []int, error) {
	data, err := os.ReadFile(filepath.Join(root, MarkdownPath(page))) // #nosec G304 -- the path is in the corpus that the user names
	if os.IsNotExist(err) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	return LeadParagraph(string(data)), LeadLinks(string(data)), nil
}

// MSCDecision is the class that the model pass of WP7.3 gave to an article
// with no code or a tie.
type MSCDecision struct {
	Page       int     `yaml:"page"`
	Code       string  `yaml:"code"`
	Confidence float64 `yaml:"confidence"`
}

// mscModel is the file manifests/wikipedia/msc-model.yaml.
type mscModel struct {
	Version   int           `yaml:"version"`
	Model     string        `yaml:"model"`
	Decisions []MSCDecision `yaml:"decisions"`
}

// MSCModelFile is the name of the file of the model decisions in the
// manifest directory.
const MSCModelFile = "msc-model.yaml"

// LoadMSCModel reads the decisions of the model pass, by page id. It gives an
// empty map when the corpus has no file of decisions.
func LoadMSCModel(root string) (map[int]MSCDecision, error) {
	var m mscModel
	err := readYAML(filepath.Join(root, ManifestDir, MSCModelFile), &m)
	if os.IsNotExist(err) {
		return map[int]MSCDecision{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make(map[int]MSCDecision, len(m.Decisions))
	for i, d := range m.Decisions {
		if d.Page <= 0 || !msc.Valid(d.Code) || d.Confidence < 0 || d.Confidence > 1 {
			return nil, fmt.Errorf("%s: decision %d needs a page id, a valid MSC code and a confidence from 0 to 1", MSCModelFile, i+1)
		}
		if _, ok := out[d.Page]; ok {
			return nil, fmt.Errorf("%s: page %d has two decisions", MSCModelFile, d.Page)
		}
		out[d.Page] = d
	}
	return out, nil
}
