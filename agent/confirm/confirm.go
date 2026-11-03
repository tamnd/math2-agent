// Package confirm is the source confirmation of corpus K9.5 for the research nodes (corpus arxiv AX9.5). For an item of an open version, a model of another family than the writer compares the main section of the node with the item text of the paper, and says whether they state the same thing. An item of a version that is not open has no text in the context, thus a person confirms it.
package confirm

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/tamnd/math2-agent/agent/write"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/node"
)

// Path is the report of m2 corpus confirm -source arxiv, relative to the root of the tree.
const Path = "reports/confirm/arxiv.md"

// The verdicts. Same confirms the source. The others go to a person with the reason.
const (
	Same      = "same"      // the node states the item, with the same hypotheses and the same conclusion
	Weaker    = "weaker"    // the node states less than the item: a hypothesis more or a conclusion less
	Stronger  = "stronger"  // the node states more than the item: a hypothesis less or a conclusion more
	Different = "different" // the node states another thing
	Unclear   = "unclear"   // the model cannot tell, or its answer has no verdict
	Person    = "person"    // the version is not open, thus the check has no text and a person confirms the item
)

var verdicts = []string{Same, Weaker, Stronger, Different, Unclear}

// Result is the verdict on one arXiv source of a node.
type Result struct {
	Node    string
	Kind    string
	Name    string
	Mention string
	Locator string
	Version string // the versioned id, as 2610.13556v1
	Class   string
	Licence string
	Writer  string // the model that wrote the node, from its provenance
	Model   string // the model of the check. Empty for Person.
	Verdict string
	Reason  string
}

// System is the system prompt of the check.
const System = `You check one node of math2, an open encyclopedia of mathematics, against the item of a paper that is its source (math2 corpus spec K9.5).
The node is in math2's own words, and it may use other notation, other names and more or fewer explanations than the paper. That is fine.
Compare only the mathematics: the objects, the hypotheses, the conclusion, and for a definition each condition.
Answer with exactly two lines and nothing else:
VERDICT: one of same, weaker, stronger, different, unclear
REASON: one or two sentences. For any verdict but same, name the hypothesis, the condition or the conclusion that differs.
Use same when the node states the item faithfully. Use weaker when the node states less (an extra hypothesis, a weaker conclusion, a missing case). Use stronger when the node states more (a dropped hypothesis, a stronger conclusion). Use different when it states another thing. Use unclear when the item text is not enough to decide.`

// MainSection gives the text of the first level 2 section of a node body, the section that states the node.
func MainSection(body string) string {
	_, rest, ok := strings.Cut("\n"+body, "\n## ")
	if !ok {
		return ""
	}
	if i := strings.Index(rest, "\n## "); i >= 0 {
		rest = rest[:i]
	}
	return strings.TrimSpace("## " + rest)
}

// Check confirms each arXiv source of the node n. items are the items of its sources, with the text of the open ones (write.SourceItems). model is the check model, and id its model id. writer is the model that wrote the node.
func Check(ctx context.Context, m llm.Model, id, writer string, n *node.Node, items []write.SourceItem) ([]Result, error) {
	section := MainSection(n.Body)
	var out []Result
	for _, it := range items {
		r := Result{Node: n.ID, Kind: n.Kind(), Name: n.Name(), Mention: it.ID, Locator: it.Locator, Version: fmt.Sprintf("%sv%d", it.Paper, it.Version), Class: it.Class, Licence: it.Licence, Writer: writer}
		if it.Class != "open" || it.Text == "" {
			r.Verdict, r.Reason = Person, "the version is not open, or the checkout has no text of the item, thus no model sees it"
			out = append(out, r)
			continue
		}
		if section == "" {
			r.Verdict, r.Reason = Unclear, "the node has no main section"
			out = append(out, r)
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "The node %s, of the kind %s, named %q:\n\n%s\n\n", n.ID, n.Kind(), n.Name(), section)
		fmt.Fprintf(&b, "The item %s of arXiv:%s (%s), licence %s:\n\n%s\n", it.Locator, r.Version, it.Title, it.Licence, it.Text)
		resp, err := m.Complete(ctx, &llm.Request{Model: id, System: System, Messages: []llm.Message{{Role: llm.User, Content: []llm.Block{llm.TextBlock(b.String())}}}, MaxTokens: 2000})
		if err != nil {
			return out, fmt.Errorf("the check of %s against %s: %w", n.ID, it.ID, err)
		}
		r.Model = id
		r.Verdict, r.Reason = Parse(resp.Text())
		out = append(out, r)
	}
	return out, nil
}

// Parse reads the verdict and the reason of an answer. An answer with no known verdict is unclear.
func Parse(s string) (string, string) {
	verdict, reason := Unclear, ""
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "*"))
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(strings.Trim(strings.TrimSpace(v), "*"))
		switch strings.ToUpper(strings.TrimSpace(strings.Trim(k, "*"))) {
		case "VERDICT":
			w := strings.ToLower(strings.Trim(strings.Fields(v + " x")[0], ".,"))
			for _, x := range verdicts {
				if w == x {
					verdict = x
				}
			}
		case "REASON":
			reason = v
		}
	}
	if reason == "" {
		reason = "the answer gives no reason"
	}
	return verdict, reason
}

// Report gives the report of the check: a YAML header with the counts, and a table of the results.
func Report(rs []Result, day string) []byte {
	count := map[string]int{}
	for _, r := range rs {
		count[r.Verdict]++
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	fmt.Fprintf(&b, "source: arxiv\ndate: %s\nchecked: %d\n", day, len(rs))
	for _, v := range append(verdicts, Person) {
		fmt.Fprintf(&b, "%s: %d\n", v, count[v])
	}
	b.WriteString("---\n\n# The source confirmation of the research nodes\n\n")
	fmt.Fprintf(&b, "This report gives the check of corpus K9.5 that m2 corpus confirm ran on %s (corpus arxiv AX9.5). For each arXiv source of a node whose version is open, a model of another family than the writer of the node (rule X09) compared the main section of the node with the item text, and gave a verdict. The verdict same confirms the source. The other verdicts and the sources of a version that is not open go to a person. A verdict of a model is evidence for triage. It does not raise a level (settled decision 3).\n\n", day)
	if len(rs) == 0 {
		return b.Bytes()
	}
	b.WriteString("| Node | Kind | Name | Source | Class | Writer | Check | Verdict | Reason |\n|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range rs {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n", r.Node, r.Kind, cell(r.Name), cell(strings.TrimSpace(r.Version+" "+r.Locator)), cell(r.Class+" "+r.Licence), r.Writer, r.Model, r.Verdict, cell(r.Reason))
	}
	return b.Bytes()
}

func cell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), "|", `\|`), "\n", " ")
}
