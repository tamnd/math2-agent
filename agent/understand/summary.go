package understand

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	axImport "github.com/tamnd/math2-agent/importer/arxiv"
)

// The pass summary (AX7.7) gives a summary of one sentence in plain words of each statement and definition, for search and for the write context. It runs only on a version of the class open, because a summary is an adaptation of the words of the paper. A summary is not a section of a node; the write job can use it as a hint, under the rules of the licence (AX9.3).

// Summary is the pass of the summaries.
const Summary = "summary"

// maxSummary is the most characters of a summary.
const maxSummary = 300

var summaryPass = pass{
	tool:  "give_summaries",
	about: "Give a summary of one sentence of each item of the paper. Give all the items in one call.",
	other: "Give a summary of each item. Only for the pass summary.",
	chars: 2000,
	reads: func(item string) bool {
		k, _, _ := strings.Cut(item, "/")
		return k == "statement" || k == "definition"
	},
	brief: func(b *strings.Builder, p *Paper) {
		fmt.Fprintf(b, "Give a summary of each of the %d items below with one call of give_summaries: one sentence in plain words, at most %d characters, that says what the item states or defines, for a reader who searches for it. Name no item by its number, and use little notation.\n\n", len(p.Items), maxSummary)
	},
	check: checkSummary,
	load: func(_ string, p *Paper, _ []axImport.Object) error {
		if p.Class != "open" {
			return fmt.Errorf("understand: %sv%d is of the class %s, and the pass summary runs only on a version of the class open (AX7.7)", p.ID, p.Version, p.Class)
		}
		return nil
	},
}

func checkSummary(r *Record, p *Paper, it Item, raw json.RawMessage) ([]string, string) {
	var in struct{ Summary string }
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Sprintf("the result of %s: %v", it.ID, err)
	}
	s := clean(in.Summary)
	switch {
	case s == "":
		return nil, fmt.Sprintf("%s has no summary", it.ID)
	case utf8.RuneCountInString(s) > maxSummary:
		return nil, fmt.Sprintf("the summary of %s has more than %d characters", it.ID, maxSummary)
	case len(sentences(s)) > 1:
		return nil, fmt.Sprintf("the summary of %s has more than one sentence", it.ID)
	}
	if m := byNumber.FindString(s); m != "" {
		return nil, fmt.Sprintf("the summary of %s names an item by its number or by a reference, %q", it.ID, m)
	}
	r.Summary = s
	return nil, ""
}
