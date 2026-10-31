package understand

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tamnd/math2-agent/agent/tools"
	axImport "github.com/tamnd/math2-agent/importer/arxiv"
	"github.com/tamnd/math2-agent/llm"
)

// The pass prose (AX7.4) gives the items that a paper states in running text: a definition, a conjecture, a problem, a claim or a standing hypothesis that no environment marks. It reads the text that ax prose of arxiv-reader prints, the running text of each section as numbered sentences with the cues of step 1, and gives each prose item as a range of sentence numbers with a kind. A check by a model of another family reads each span again (step 3). An item is accepted only when the check gives the same kind and finds the span complete; otherwise it goes to the queue prose-review (rule D11).

// Prose is the pass of the prose items.
const Prose = "prose"

// ProseReview is the queue of the prose items that the check did not accept.
const ProseReview = "prose-review"

// ProseKinds are the kinds of a prose item.
var ProseKinds = []string{"definition", "conjecture", "problem", "claim", "hypothesis"}

// maxProse is the most sentences that one prose item spans.
const maxProse = 5

// Text is the running text of a version, as ax prose prints it. It has the words of the paper, thus it is never stored: it goes only to the models of one run.
type Text struct {
	Paper    string    `json:"paper"`
	Version  int       `json:"version"`
	Text     bool      `json:"text"`
	Sections []Section `json:"sections"`
}

// Section is the running text of one section.
type Section struct {
	Section   string     `json:"section"`
	Sentences []Sentence `json:"sentences"`
}

// Sentence is one sentence of the running text, with its number in the paper and its cues.
type Sentence struct {
	N    int    `json:"n"`
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
	Cues []struct {
		Kind string `json:"kind"`
		Cue  string `json:"cue"`
	} `json:"cues,omitempty"`
}

// ProseItem is one prose item of a section: the range of its sentences, its kind, the term of a definition, and the answer of the check.
type ProseItem struct {
	From  int    `json:"from"` // the first sentence, by its number in the paper
	To    int    `json:"to"`   // the last sentence
	Line  int    `json:"line"` // the line of the first sentence in its file
	File  string `json:"file"`
	Kind  string `json:"kind"`
	Term  string `json:"term,omitempty"` // the defined term, for a definition of a version whose class permits its words
	Check *Check `json:"check,omitempty"`
	Queue string `json:"queue,omitempty"`
}

// Check is the answer of the check of a prose item by a model of another family. It has no text, so that a record of a closed class has none (AX7.9).
type Check struct {
	Model    string `json:"model"`
	Kind     string `json:"kind"` // one of ProseKinds, or none
	Complete bool   `json:"complete"`
}

// ProseCheck reads the sentences of one prose item, with the sentence before and after it, and gives its kind, or none, and whether the span is complete. The model of the check is of a family other than the family of the model of the pass (rule X09).
type ProseCheck func(ctx context.Context, p *Paper, before string, span []string, after string) (c *Check, err error)

// SetText gives the paper of a prose job its running text. Each section becomes one item of the pass, with the id ax://paper/ID#SECTION, so that each section gets a record, with no prose item when it has none. The text must be of the version of the paper.
func (p *Paper) SetText(t *Text) error {
	if t.Paper != p.ID || t.Version != p.Version {
		return fmt.Errorf("understand: the text is of %sv%d, and the paper is %sv%d", t.Paper, t.Version, p.ID, p.Version)
	}
	p.Items, p.Sections = nil, map[string]Section{}
	for _, s := range t.Sections {
		name := cmp.Or(s.Section, "front")
		id := axImport.MentionID(p.ID, name)
		if _, ok := p.Sections[id]; ok {
			return fmt.Errorf("understand: the text has the section %s twice", name)
		}
		p.Sections[id] = s
		var b strings.Builder
		for _, x := range s.Sentences {
			fmt.Fprintf(&b, "[%d]", x.N)
			for _, c := range x.Cues {
				fmt.Fprintf(&b, " (cue %s: %s)", c.Kind, c.Cue)
			}
			fmt.Fprintf(&b, " %s\n", x.Text)
		}
		p.Items = append(p.Items, Item{ID: id, Ref: fmt.Sprintf("%sv%d#%s", p.ID, p.Version, name), Item: "section", Title: name, Body: strings.TrimSuffix(b.String(), "\n")})
	}
	return nil
}

var prosePass = pass{
	tool:  "give_prose",
	about: "Give the prose items of each section of the paper, as ranges of sentence numbers with a kind. Give all the sections in one call, and a section with no prose item with an empty list.",
	other: "Give the prose items of each section. Only for the pass prose.",
	reads: func(string) bool { return false }, // the items are the sections of the text, from SetText
	brief: func(b *strings.Builder, p *Paper) {
		fmt.Fprintf(b, "Give the prose items of each of the %d sections below with one call of give_prose. A prose item is a definition, a conjecture, a problem, a claim or a standing hypothesis that the running text states, outside the theorem environments. Give each item as the numbers of its first and last sentence, at most %d sentences, with its kind: %s. For a definition, give the term that it defines. A cue marks a sentence that a pattern found; a cue can be wrong, and an item can have no cue. Give a section with no prose item with an empty list.\n\n", len(p.Items), maxProse, strings.Join(ProseKinds, ", "))
	},
	check: checkProse,
	after: afterProse,
}

func checkProse(r *Record, p *Paper, it Item, raw json.RawMessage) ([]string, string) {
	var in struct {
		Prose *[]struct {
			From, To   int
			Kind, Term string
		}
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Sprintf("the result of %s: %v", it.ID, err)
	}
	if in.Prose == nil {
		return nil, fmt.Sprintf("%s has no list prose. Give an empty list for a section with no prose item", it.ID)
	}
	sec := p.Sections[it.ID]
	at := map[int]Sentence{}
	for _, x := range sec.Sentences {
		at[x.N] = x
	}
	r.Prose = []ProseItem{}
	var left []string
	for _, x := range *in.Prose {
		name := fmt.Sprintf("the item %d to %d", x.From, x.To)
		first, ok1 := at[x.From]
		_, ok2 := at[x.To]
		switch {
		case !ok1 || !ok2 || x.To < x.From:
			left = append(left, name+", whose sentences are not of the section")
			continue
		case x.To-x.From >= maxProse:
			left = append(left, fmt.Sprintf("%s, which spans more than %d sentences", name, maxProse))
			continue
		case !slices.Contains(ProseKinds, x.Kind):
			left = append(left, fmt.Sprintf("%s, whose kind %q is not one of %s", name, x.Kind, strings.Join(ProseKinds, ", ")))
			continue
		case slices.ContainsFunc(r.Prose, func(y ProseItem) bool { return x.From <= y.To && y.From <= x.To }):
			left = append(left, name+", which overlaps another item")
			continue
		}
		pi := ProseItem{From: x.From, To: x.To, File: first.File, Line: first.Line, Kind: x.Kind}
		if t := clean(x.Term); t != "" {
			switch {
			case x.Kind != "definition":
				left = append(left, "the term of "+name+", which is not a definition")
			case p.Class == "record" || p.Class == "verbatim":
				left = append(left, "the term of "+name+", because math2 stores no words of a paper of the class "+p.Class)
			case utf8.RuneCountInString(t) > maxName || !strings.Contains(strings.ToLower(spanText(at, x.From, x.To)), strings.ToLower(t)):
				left = append(left, fmt.Sprintf("the term %q of %s, which is not in its sentences", t, name))
			default:
				pi.Term = t
			}
		}
		r.Prose = append(r.Prose, pi)
	}
	slices.SortFunc(r.Prose, func(a, b ProseItem) int { return a.From - b.From })
	return left, ""
}

func spanText(at map[int]Sentence, from, to int) string {
	var parts []string
	for n := from; n <= to; n++ {
		parts = append(parts, at[n].Text)
	}
	return strings.Join(parts, " ")
}

// afterProse runs the check of each prose item of the record. With no check, each item goes to prose-review, because no model of another family has read it.
func afterProse(ctx context.Context, r *Record, p *Paper) (string, error) {
	sec := p.Sections[r.ID]
	at := map[int]Sentence{}
	for _, x := range sec.Sentences {
		at[x.N] = x
	}
	for i := range r.Prose {
		pi := &r.Prose[i]
		if p.Check == nil {
			pi.Queue = ProseReview
			continue
		}
		var span []string
		for n := pi.From; n <= pi.To; n++ {
			span = append(span, at[n].Text)
		}
		c, err := p.Check(ctx, p, at[pi.From-1].Text, span, at[pi.To+1].Text)
		if err != nil {
			return "", err
		}
		pi.Check = c
		if c == nil || c.Kind != pi.Kind || !c.Complete {
			pi.Queue = ProseReview
		}
	}
	return "", nil
}

// checkTool is the tool of the check of a prose item.
var checkTool = llm.Tool{Name: "check_prose", Description: "Give the kind of the span, or none, and whether the span is the whole item.", Schema: json.RawMessage(`{"type":"object","properties":{` +
	`"kind":{"type":"string","enum":["definition","conjecture","problem","claim","hypothesis","none"]},` +
	`"complete":{"type":"boolean","description":"true when the span holds the whole item, and no sentence of the item is outside it"}},"required":["kind","complete"],"additionalProperties":false}`)}

// CheckWith returns the check of the prose items with the model, which must be of another family than the model of the pass. The system prompt is checks/prose.md of the role.
func CheckWith(m llm.Model, model, system string) ProseCheck {
	return func(ctx context.Context, p *Paper, before string, span []string, after string) (*Check, error) {
		var b strings.Builder
		fmt.Fprintf(&b, "Read the span, with the sentence before it and the sentence after it, and give your answer with the tool %s.\n\n", checkTool.Name)
		if before != "" {
			fmt.Fprintf(&b, "Before the span:\n%s\n\n", tools.Data("arxiv:"+p.ID, licenceLabel(p), before))
		}
		fmt.Fprintf(&b, "The span:\n%s\n\n", tools.Data("arxiv:"+p.ID, licenceLabel(p), strings.Join(span, " ")))
		if after != "" {
			fmt.Fprintf(&b, "After the span:\n%s\n", tools.Data("arxiv:"+p.ID, licenceLabel(p), after))
		}
		resp, err := m.Complete(ctx, &llm.Request{
			Model:     model,
			System:    system,
			Messages:  []llm.Message{{Role: llm.User, Content: []llm.Block{llm.TextBlock(b.String())}}},
			Tools:     []llm.Tool{checkTool},
			MaxTokens: 500,
		})
		if err != nil {
			return nil, fmt.Errorf("understand: the check of the prose: %w", err)
		}
		for _, call := range resp.Calls() {
			if call.Name != checkTool.Name {
				continue
			}
			var v struct {
				Kind     string
				Complete bool
			}
			if json.Unmarshal(call.Input, &v) == nil && (v.Kind == "none" || slices.Contains(ProseKinds, v.Kind)) {
				return &Check{Model: model, Kind: v.Kind, Complete: v.Complete}, nil
			}
		}
		return &Check{Model: model, Kind: "none"}, nil
	}
}

// Accepted reports whether the check accepted the prose item.
func (pi ProseItem) Accepted() bool { return pi.Check != nil && pi.Queue == "" }

// String gives the item as sec-3 [12-13] definition.
func (pi ProseItem) String() string {
	return "[" + strconv.Itoa(pi.From) + "-" + strconv.Itoa(pi.To) + "] " + pi.Kind
}
