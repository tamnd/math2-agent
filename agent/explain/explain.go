// Package explain has the goal and the brief of the explain role (spec 12.6).
//
// An explain run writes levelled explanations into the Explanations section of the target node, each under a heading such as "### L2 agent", and it can add example nodes. L1 is for a person who has not met the concept, L2 for an undergraduate, L3 for a graduate and L4 for a researcher (spec 04.5).
//
// [Goal] holds when the changes are inside the field permission set of explain, the run wrote an explanation at each level that the goal of the job names, no person wrote or confirmed an explanation that the run changed, each explanation cites nodes that exist, the cited nodes of an L1 or L2 explanation are in the curricula of its level, and the grounding check finds no sentence that the cited nodes do not support. A sentence that the cited nodes contradict fails the run. [Ground] is the grounding check: a second model call that labels each sentence. [Brief] gives the first message of a run.
package explain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/tag"
	"github.com/tamnd/math2-agent/workspace"
)

// The labels of the grounding check.
const (
	Supported    = "supported"
	Unsupported  = "unsupported"
	Contradicted = "contradicted"
)

// Explanation is an explanation that a run added or changed.
type Explanation struct {
	Level string // L1 to L4
	Text  string // the text after the heading
}

var errNoExplanation = errors.New("the run has not written an explanation of the target yet. Write it in the Explanations section under a heading such as \"### L2 agent\"")

var fieldRe = regexp.MustCompile(`^body\.explanations\.(L[1-4])$`)

// Written returns the explanations of the target that the changes add or change, in the order of their levels. An explanation that the changes remove is not in the list.
func Written(target string, changes []workspace.Change) ([]Explanation, error) {
	p := tools.NodePath(target)
	i := slices.IndexFunc(changes, func(c workspace.Change) bool { return c.Path == p })
	if i < 0 || changes[i].Old == nil || changes[i].New == nil {
		return nil, nil
	}
	o, err := node.Parse(p, changes[i].Old)
	if err != nil {
		return nil, err
	}
	n, err := node.Parse(p, changes[i].New)
	if err != nil {
		return nil, err
	}
	before, after := prov.Fields(o), prov.Fields(n)
	var out []Explanation
	for _, f := range slices.Sorted(maps.Keys(after)) {
		m := fieldRe.FindStringSubmatch(f)
		if m == nil || before[f] == after[f] {
			continue
		}
		text := strings.TrimSpace(strings.ReplaceAll(after[f], "### "+m[1]+" agent", "\n\n"))
		out = append(out, Explanation{Level: m[1], Text: text})
	}
	return out, nil
}

var levelRe = regexp.MustCompile(`\bL[1-4]\b`)

// Levels returns the levels that the goal of a job names, such as L1 and L2, in order and with no repeats.
func Levels(goal string) []string {
	out := levelRe.FindAllString(goal, -1)
	slices.Sort(out)
	return slices.Compact(out)
}

// Cited returns the targets of the links [[text|TAG]] of the text, in order and with no repeats. A link in code is text and is not a citation.
func Cited(text string) []string {
	var out []string
	for _, l := range check.Links(&node.Node{Body: text}) {
		if !slices.Contains(out, l.Target) {
			out = append(out, l.Target)
		}
	}
	return out
}

var paraRe = regexp.MustCompile(`\n\s*\n`)

// Sentences splits the text of an explanation into sentences. A sentence ends at ".", "?" or "!" before a space or the end of a paragraph, outside $…$ math and not after the abbreviations e.g., i.e. and cf. The lines of a paragraph join with a space.
func Sentences(text string) []string {
	var out []string
	for _, para := range paraRe.Split(strings.TrimSpace(text), -1) {
		p := strings.Join(strings.Fields(para), " ")
		start, math := 0, false
		for i := 0; i < len(p); i++ {
			switch c := p[i]; {
			case c == '$':
				math = !math
			case !math && (c == '.' || c == '?' || c == '!') && (i+1 == len(p) || p[i+1] == ' '):
				w := p[start : i+1]
				if strings.HasSuffix(w, "e.g.") || strings.HasSuffix(w, "i.e.") || strings.HasSuffix(w, "cf.") {
					continue
				}
				if s := strings.TrimSpace(w); s != "" {
					out = append(out, s)
				}
				start = i + 1
			}
		}
		if s := strings.TrimSpace(p[start:]); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Label is the label that the grounding check gives one sentence.
type Label struct {
	Sentence string
	Label    string // Supported, Unsupported or Contradicted
	Why      string // the reason, in one sentence
}

// labelTool is the tool with which the model of the grounding check answers.
var labelTool = llm.Tool{
	Name:        "label",
	Description: "Give each sentence of the explanation one label: supported, unsupported or contradicted, with the reason in one sentence.",
	Schema: json.RawMessage(`{"type":"object","properties":{"labels":{"type":"array","items":{"type":"object","properties":{` +
		`"sentence":{"type":"integer","description":"the number of the sentence, from 1"},` +
		`"label":{"type":"string","enum":["supported","unsupported","contradicted"]},` +
		`"why":{"type":"string","description":"the reason, in one sentence"}},"required":["sentence","label","why"],"additionalProperties":false}}},` +
		`"required":["labels"],"additionalProperties":false}`),
}

// Ground runs the grounding check of the explanation with the model (spec 12.6): one call of the model with the system prompt, the sentences of the explanation and the text of each cited node, which gives each sentence a label. The cited nodes map a tag to the text of its node file. A sentence that the model does not label is unsupported. The explanation and the nodes go to the model in data blocks, thus an instruction in them is data.
func Ground(ctx context.Context, m llm.Model, model, system string, e Explanation, cited map[string]string) ([]Label, error) {
	sentences := Sentences(e.Text)
	if len(sentences) == 0 {
		return nil, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Label each sentence of this %s explanation with the tool %s.\n\n", e.Level, labelTool.Name)
	var numbered strings.Builder
	for i, s := range sentences {
		fmt.Fprintf(&numbered, "%d. %s\n", i+1, s)
	}
	b.WriteString(tools.Data("the explanation "+e.Level, "CC-BY-SA-4.0", numbered.String()))
	b.WriteString("\n\nThe cited nodes:\n")
	for _, t := range slices.Sorted(maps.Keys(cited)) {
		p := tools.NodePath(t)
		fmt.Fprintf(&b, "\n%s\n", tools.Data(p, tools.Licence(p), cited[t]))
	}
	resp, err := m.Complete(ctx, &llm.Request{
		Model:     model,
		System:    system,
		Messages:  []llm.Message{{Role: llm.User, Content: []llm.Block{llm.TextBlock(b.String())}}},
		Tools:     []llm.Tool{labelTool},
		MaxTokens: 4000,
	})
	if err != nil {
		return nil, fmt.Errorf("explain: the grounding check: %w", err)
	}
	out := make([]Label, len(sentences))
	for i, s := range sentences {
		out[i] = Label{Sentence: s, Label: Unsupported, Why: "the grounding check gave the sentence no label"}
	}
	for _, c := range resp.Calls() {
		if c.Name != labelTool.Name {
			continue
		}
		var in struct {
			Labels []struct {
				Sentence int    `json:"sentence"`
				Label    string `json:"label"`
				Why      string `json:"why"`
			} `json:"labels"`
		}
		if err := json.Unmarshal(c.Input, &in); err != nil {
			return nil, fmt.Errorf("explain: the grounding check gave labels that are not valid: %w", err)
		}
		for _, l := range in.Labels {
			if l.Sentence < 1 || l.Sentence > len(sentences) || !slices.Contains([]string{Supported, Unsupported, Contradicted}, l.Label) {
				continue
			}
			out[l.Sentence-1].Label, out[l.Sentence-1].Why = l.Label, l.Why
		}
	}
	return out, nil
}

// Curricula returns the nodes that an explanation at each level may cite, from the curricula in topics/curricula/ of the tree (spec 12.6). The first version maps L1 to the courses of year 1 and L2 to the courses of years 1 and 2. L3 and L4 have no set. A level with no node in its courses has no set, thus its explanations are not checked.
func Curricula(fsys fs.FS) (map[string][]string, error) {
	files, err := fs.Glob(fsys, "topics/curricula/*.yaml")
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, f := range files {
		data, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		var c struct {
			Courses []struct {
				Year  int `yaml:"year"`
				Items []struct {
					Node string `yaml:"node"`
				} `yaml:"items"`
			} `yaml:"courses"`
		}
		if err := yaml.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("explain: %s: %w", f, err)
		}
		for _, course := range c.Courses {
			for _, it := range course.Items {
				if it.Node == "" {
					continue
				}
				if course.Year == 1 {
					out["L1"] = append(out["L1"], it.Node)
				}
				if course.Year == 1 || course.Year == 2 {
					out["L2"] = append(out["L2"], it.Node)
				}
			}
		}
	}
	for l, tags := range out {
		slices.Sort(tags)
		out[l] = slices.Compact(tags)
	}
	return out, nil
}

// Goal returns the goal predicate of an explain run on the work tree. The model m with the name model and the system prompt runs the grounding check, and m is the meter of the run, thus the check counts in its spend.
func Goal(ws *workspace.Workspace, job *agent.Job, m llm.Model, model, system string) agent.Goal {
	return agent.OnChanges(ws, func(ctx context.Context, changes []workspace.Change) (bool, string, error) {
		if err := agent.Permit(job.Role, job.Target, changes); err != nil {
			return false, err.Error(), nil
		}
		written, err := Written(job.Target, changes)
		if err != nil {
			return false, err.Error(), nil
		}
		if len(written) == 0 {
			return false, errNoExplanation.Error(), nil
		}
		for _, l := range Levels(job.Goal) {
			if !slices.ContainsFunc(written, func(e Explanation) bool { return e.Level == l }) {
				return false, fmt.Sprintf("the run has not written the %s explanation yet. Write it under the heading \"### %s agent\"", l, l), nil
			}
		}
		log, err := prov.Read(ws.Dir, job.Target)
		if err != nil {
			return false, "", err
		}
		for _, e := range written {
			if l, ok := prov.Author(log, "body.explanations."+e.Level); ok && l.Human != "" {
				return false, fmt.Sprintf("%s wrote or confirmed the %s explanation, and explain does not change it. Put the text back as it was", l.Human, e.Level), nil
			}
		}
		allowed, err := Curricula(os.DirFS(ws.Dir))
		if err != nil {
			return false, err.Error(), nil
		}
		var added []string
		for _, c := range changes {
			if c.Old == nil {
				if n, err := node.Parse(c.Path, c.New); err == nil {
					added = append(added, n.ID)
				}
			}
		}
		var unsupported []string
		for _, e := range written {
			cited := Cited(e.Text)
			if len(cited) == 0 {
				return false, fmt.Sprintf("the %s explanation cites no node. Cite the node that supports each claim with a link [[text|TAG]]", e.Level), nil
			}
			texts := map[string]string{}
			if text, err := ws.Read(tools.NodePath(job.Target)); err == nil {
				texts[job.Target] = string(text)
			}
			for _, t := range cited {
				if !tag.IsTag(t) && !tag.IsProvisional(t) {
					return false, fmt.Sprintf("the %s explanation links to %q, which is not a tag", e.Level, t), nil
				}
				text, err := ws.Read(tools.NodePath(t))
				if err != nil {
					return false, fmt.Sprintf("the %s explanation links to %s, which is not a node of the tree", e.Level, t), nil
				}
				texts[t] = string(text)
				if set := allowed[e.Level]; len(set) > 0 && t != job.Target && !slices.Contains(added, t) && !slices.Contains(set, t) {
					return false, fmt.Sprintf("the %s explanation links to %s, which is not in the curricula of %s. Explain with the nodes of the curricula", e.Level, t, e.Level), nil
				}
			}
			labels, err := Ground(ctx, m, model, system, e, texts)
			if err != nil {
				return false, "", err
			}
			for i, l := range labels {
				switch l.Label {
				case Contradicted:
					return false, "", fmt.Errorf("sentence %d of the %s explanation is contradicted by the cited nodes: %s", i+1, e.Level, strings.TrimSuffix(l.Why, "."))
				case Unsupported:
					unsupported = append(unsupported, fmt.Sprintf("sentence %d of the %s explanation (%q): %s", i+1, e.Level, l.Sentence, strings.TrimSuffix(l.Why, ".")))
				}
			}
		}
		if len(unsupported) > 0 {
			return false, "the cited nodes do not support " + strings.Join(unsupported, "; ") + ". Cite a node that supports each such sentence, or remove the sentence", nil
		}
		return true, "", nil
	})
}

// Brief returns the first message of an explain run: the job, how to write and mark an explanation, the nodes that the curricula allow for each level that the goal names, and the target node from the work tree in a data block.
func Brief(ws *workspace.Workspace, job *agent.Job) (string, error) {
	p := tools.NodePath(job.Target)
	if p == "" {
		return "", fmt.Errorf("explain: the target %q is not a tag or a provisional id", job.Target)
	}
	text, err := ws.Read(p)
	if err != nil {
		return "", fmt.Errorf("explain: %w", err)
	}
	n, err := node.Parse(p, text)
	if err != nil {
		return "", fmt.Errorf("explain: %w", err)
	}
	allowed, err := Curricula(os.DirFS(ws.Dir))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Job %s: explain the target %s, a %s node. The goal of the job: %s.\n\n", job.ID, job.Target, n.Kind(), strings.TrimSuffix(job.Goal, "."))
	fmt.Fprintf(&b, "Write each explanation in the Explanations section of %s, under a heading such as \"### L2 agent\". Change no other part of the node.\n\n", p)
	for _, l := range Levels(job.Goal) {
		if set := allowed[l]; len(set) > 0 {
			fmt.Fprintf(&b, "An %s explanation may link only to the target, to the example nodes that you add, and to these nodes of the curricula: %s.\n\n", l, strings.Join(set, ", "))
		}
	}
	fmt.Fprintf(&b, "The target node:\n\n%s\n", tools.Data(p, tools.Licence(p), string(text)))
	return b.String(), nil
}
