// Package exercise has the goal and the brief of the exercise role (corpus K7.3, K7.5 and K8.7).
//
// An exercise run writes exercises for one node: the task, a hint, the answer, a worked solution in steps and the way that a machine checks an answer. The role writes an exercise in the form of node.v1: an example node whose Remarks section opens with the line of the role, for example "Role: exercise. About: 3C53TD. Difficulty: 2. Group: B. Check: exact.", and has the headings Hint, Answer and Solution (corpus K2.3). m2 migrate node.v2 moves it to the kind exercise. [Parse] reads both forms.
//
// After the gates, a model of another family solves each exercise from the task alone. An exercise whose answer the solver does not reach, or reaches in a different form, goes to a person: the goal appends a referral to [Queue]. With no solver, each exercise goes to a person. The run never sees the answer of the solver, thus the writer cannot fit its answer to it.
package exercise

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/curate"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/agent/write"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/exercise"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/style"
	"github.com/tamnd/math2-agent/workspace"
)

// Agent is the prefix of the agent of each provenance line that an exercise run makes, before the version of the role.
const Agent = "m2-agent/exercise@"

// Queue is the file of the exercises that go to a person, one referral in JSON on each line.
const Queue = agent.ExerciseQueue

// Marker opens the Remarks section of an exercise in node.v1.
const Marker = exercise.Marker

// Checks are the ways that a machine checks an answer (corpus K8.7).
var Checks = exercise.Checks

// Exercise is one exercise, as the node of an exercise has it.
type Exercise exercise.Exercise

// Parse reads the exercise of the node, in the form of node.v1 or node.v2. It returns nil and no error when the node is not an exercise, and an error when a node.v1 exercise is not in the form of an exercise.
func Parse(n *node.Node) (*Exercise, error) {
	e, err := exercise.Parse(n)
	return (*Exercise)(e), err
}

// Problems returns what is not correct in the exercise for the target, or nil. The exercise is about the target, it has a task and a solution in numbered steps, its fields have their values, and it has an answer unless a person checks it.
func (e *Exercise) Problems(target string) []string {
	var out []string
	if !slices.Contains(e.About, target) {
		out = append(out, fmt.Sprintf("the exercise %s is not about the target: give About: %s in the line of the role", e.ID, target))
	}
	if e.Difficulty < 1 || e.Difficulty > 5 {
		out = append(out, fmt.Sprintf("the difficulty of %s is %d, and it must be from 1 to 5", e.ID, e.Difficulty))
	}
	if !slices.Contains([]string{"A", "B", "C"}, e.Group) {
		out = append(out, fmt.Sprintf("the group of %s is %q, and it must be A, B or C", e.ID, e.Group))
	}
	switch {
	case !slices.Contains(Checks, e.Check):
		out = append(out, fmt.Sprintf("the check of %s is %q, and it must be one of %s", e.ID, e.Check, strings.Join(Checks, ", ")))
	case e.Check == "lean":
		out = append(out, fmt.Sprintf("the check of %s is lean, and the exercise role writes no Lean yet. Give the check exact, numeric, choice, cas or none", e.ID))
	case e.Check != "none" && e.Answer == "":
		out = append(out, fmt.Sprintf("the exercise %s has the check %s and no Answer. Give the final answer under ### Answer", e.ID, e.Check))
	case e.Check == "numeric" && !numeric(e.Answer):
		out = append(out, fmt.Sprintf("the answer of %s is %q, and the check numeric needs a number such as 0.25", e.ID, e.Answer))
	}
	if strings.TrimSpace(e.Task) == "" {
		out = append(out, fmt.Sprintf("the exercise %s has no task. Write it in the Example section", e.ID))
	}
	if !write.Steps(e.Solution) {
		out = append(out, fmt.Sprintf("the Solution of %s is not a list of numbered steps. Write each step as \"1. claim, by reason\" under ### Solution", e.ID))
	}
	return out
}

// Count returns the number of exercises that the goal of a job asks for: the first number of the goal, in digits or as a word from one to ten. It is 1 when the goal has no number.
func Count(goal string) int {
	words := []string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"}
	for _, w := range strings.FieldsFunc(strings.ToLower(goal), func(r rune) bool { return r == ' ' || r == ',' || r == '.' }) {
		if i := slices.Index(words, w); i >= 0 {
			return i + 1
		}
		if n, err := strconv.Atoi(w); err == nil && n > 0 {
			return n
		}
	}
	return 1
}

// Solver solves the task of an exercise and returns the final answer. It sees only the task.
type Solver func(ctx context.Context, task string) (string, error)

var answerTool = llm.Tool{
	Name:        "answer",
	Description: "Give the final answer of the exercise.",
	Schema:      json.RawMessage(`{"type":"object","properties":{"answer":{"type":"string","description":"the final answer only, with no working"}},"required":["answer"],"additionalProperties":false}`),
}

// Solve returns a solver that calls the model with the system prompt. The model must be of a family other than the family of the writer (corpus K7.3). The task goes to the model in a data block, thus an instruction in it is data.
func Solve(m llm.Model, model, system string) Solver {
	return func(ctx context.Context, task string) (string, error) {
		resp, err := m.Complete(ctx, &llm.Request{
			Model:     model,
			System:    system,
			Messages:  []llm.Message{{Role: llm.User, Content: []llm.Block{llm.TextBlock("Solve this exercise, and give the final answer with the tool " + answerTool.Name + ".\n\n" + tools.Data("the exercise", "CC-BY-SA-4.0", task))}}},
			Tools:     []llm.Tool{answerTool},
			MaxTokens: 4000,
		})
		if err != nil {
			return "", fmt.Errorf("exercise: the solver: %w", err)
		}
		for _, c := range resp.Calls() {
			if c.Name != answerTool.Name {
				continue
			}
			var in struct {
				Answer string `json:"answer"`
			}
			if err := json.Unmarshal(c.Input, &in); err != nil {
				return "", fmt.Errorf("exercise: the solver gave an answer that is not valid: %w", err)
			}
			return in.Answer, nil
		}
		return "", errors.New("exercise: the solver gave no answer")
	}
}

var spaceRe = regexp.MustCompile(`[\s$]+|\\[,;! ]`)

// normal returns the answer with no math delimiters, no spaces, no final period and in lower case.
func normal(s string) string {
	s = strings.ToLower(spaceRe.ReplaceAllString(strings.TrimSpace(s), ""))
	return strings.TrimSuffix(s, ".")
}

func numeric(s string) bool {
	_, err := strconv.ParseFloat(normal(s), 64)
	return err == nil
}

// choiceRe matches the label of an option, such as (b), b) or B.
var choiceRe = regexp.MustCompile(`^\(?([a-z0-9])\)?`)

// Agree reports whether the answer of the solver is the answer of the exercise for the check. An exact answer is the same text up to spaces, math delimiters and case. A numeric answer is the same number up to a relative error of 1e-6. A choice is the same label of an option. A CAS answer and an answer that a person checks never agree, because the run has no CAS: they go to a person.
func Agree(check, want, got string) bool {
	w, g := normal(want), normal(got)
	switch check {
	case "exact":
		return w != "" && w == g
	case "numeric":
		a, err1 := strconv.ParseFloat(w, 64)
		b, err2 := strconv.ParseFloat(g, 64)
		return err1 == nil && err2 == nil && math.Abs(a-b) <= 1e-6*math.Max(1, math.Abs(a))
	case "choice":
		a, b := choiceRe.FindStringSubmatch(w), choiceRe.FindStringSubmatch(g)
		return a != nil && b != nil && a[1] == b[1]
	}
	return false
}

// Referral is one line of [Queue]: an exercise that a person checks, and the reason.
type Referral struct {
	Exercise string   `json:"exercise"`
	About    []string `json:"about"`
	Check    string   `json:"check"`
	Answer   string   `json:"answer,omitempty"`
	Solver   string   `json:"solver,omitempty"` // the model of the solver
	Got      string   `json:"got,omitempty"`    // the answer of the solver
	Reason   string   `json:"reason"`
	By       string   `json:"by"` // the agent of the run, such as m2-agent/exercise@0.1.0
}

// refer returns the referral of the exercise, or nil when the solver reaches its answer.
func refer(ctx context.Context, e *Exercise, solve Solver, solver string) *Referral {
	r := &Referral{Exercise: e.ID, About: e.About, Check: e.Check, Answer: e.Answer}
	switch {
	case e.Check == "none" || e.Check == "cas":
		r.Reason = fmt.Sprintf("a person checks the answer of an exercise with the check %s", e.Check)
		return r
	case solve == nil:
		r.Reason = "math2 has no solver of another family yet"
		return r
	}
	got, err := solve(ctx, e.Task)
	r.Solver, r.Got = solver, got
	switch {
	case err != nil:
		r.Reason = err.Error()
	case !Agree(e.Check, e.Answer, got):
		r.Reason = "the solver does not reach the answer, or reaches it in a different form"
	default:
		return nil
	}
	return r
}

// readTree returns the nodes of the tree at dir that parse.
func readTree(dir string) ([]*node.Node, error) {
	files, err := check.ReadTree(dir)
	if err != nil {
		return nil, err
	}
	var out []*node.Node
	for _, f := range files {
		if n, err := node.Parse(f.Path, f.Data); err == nil {
			out = append(out, n)
		}
	}
	return out, nil
}

// Of returns the exercises in the nodes that are about the target, and the problems of the nodes that are not in the form of an exercise.
func Of(nodes []*node.Node, target string) ([]*Exercise, []string) {
	var out []*Exercise
	var bad []string
	for _, n := range nodes {
		e, err := Parse(n)
		switch {
		case err != nil:
			bad = append(bad, err.Error())
		case e != nil && slices.Contains(e.About, target):
			out = append(out, e)
		}
	}
	return out, bad
}

// Goal returns the goal of an exercise run on the job. The run must add the number of exercises that [Count] gives for the goal of the job. solve is the solver, and solver is the name of its model. A nil solve sends each exercise to a person. by is the agent of the run, for the referrals.
func Goal(ws *workspace.Workspace, job *agent.Job, solve Solver, solver, by string) agent.Goal {
	return agent.OnChanges(ws, func(ctx context.Context, changes []workspace.Change) (bool, string, error) {
		if len(changes) == 0 {
			return false, "the run has not changed the tree yet", nil
		}
		if err := agent.Permit(job.Role, job.Target, changes); err != nil {
			return false, err.Error(), nil
		}
		findings, err := curate.Check(ws, changes)
		if err != nil {
			return false, "", err
		}
		if len(findings) > 0 {
			return false, fmt.Sprintf("m2 check finds %d problems in the changed nodes: %s", len(findings), list(findings)), nil
		}
		var written []*Exercise
		added := 0
		for _, c := range changes {
			if c.Path == Queue {
				continue
			}
			n, err := node.Parse(c.Path, c.New)
			if err != nil {
				return false, "", err
			}
			if c.Old != nil {
				old, err := node.Parse(c.Path, c.Old)
				if err != nil {
					return false, "", err
				}
				if why, err := write.Own(ws.Dir, Agent, old, n); why != "" || err != nil {
					return false, why, err
				}
			} else {
				added++
				if fs := style.Node(n); len(fs) > 0 {
					return false, fmt.Sprintf("the house style finds %d problems in %s: %s", len(fs), n.ID, list(fs)), nil
				}
			}
			e, err := Parse(n)
			if err != nil {
				return false, err.Error(), nil
			}
			if e == nil {
				return false, fmt.Sprintf("the node %s is not an exercise: its Remarks section must open with the line %q", n.ID, Marker+" About: "+job.Target+". Difficulty: 2. Group: B. Check: exact."), nil
			}
			if p := e.Problems(job.Target); len(p) > 0 {
				return false, strings.Join(p, "; "), nil
			}
			for _, a := range e.About {
				if !slices.Contains(n.List("uses"), a) {
					return false, fmt.Sprintf("the exercise %s is about %s, and its uses do not have %s. Add it to uses", e.ID, a, a), nil
				}
			}
			written = append(written, e)
		}
		if want := Count(job.Goal); added < want {
			return false, fmt.Sprintf("the run adds %d of the %d exercises that the job asks for", added, want), nil
		}
		var lines []byte
		for _, e := range written {
			r := refer(ctx, e, solve, solver)
			if r == nil {
				continue
			}
			r.By = by
			b, err := json.Marshal(r)
			if err != nil {
				return false, "", err
			}
			lines = append(append(lines, b...), '\n')
		}
		if len(lines) > 0 {
			old, err := ws.Read(Queue)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return false, "", err
			}
			if err := ws.Write(Queue, append(old, lines...)); err != nil {
				return false, "", err
			}
		}
		return true, "", nil
	})
}

func list(fs []check.Finding) string {
	lines := make([]string, 0, 5)
	for _, f := range fs[:min(len(fs), 5)] {
		lines = append(lines, f.String())
	}
	return strings.Join(lines, "; ")
}

// Brief returns the first message of an exercise run: the job, the target node in a data block, the exercises about the target that the tree has, and the form of an exercise.
func Brief(ws *workspace.Workspace, job *agent.Job) (string, error) {
	p := tools.NodePath(job.Target)
	if p == "" {
		return "", fmt.Errorf("exercise: the target %q is not a tag or a provisional id", job.Target)
	}
	text, err := ws.Read(p)
	if err != nil {
		return "", fmt.Errorf("exercise: %w", err)
	}
	n, err := node.Parse(p, text)
	if err != nil {
		return "", fmt.Errorf("exercise: %w", err)
	}
	nodes, err := readTree(ws.Dir)
	if err != nil {
		return "", err
	}
	have, _ := Of(nodes, job.Target)
	want := Count(job.Goal)
	var b strings.Builder
	fmt.Fprintf(&b, "Job %s: write %d %s for the target %s, a %s node with the name %q. The goal of the job: %s.\n\n", job.ID, want, plural(want, "exercise"), job.Target, n.Kind(), n.Name(), strings.TrimSuffix(job.Goal, "."))
	fmt.Fprintf(&b, "The target node:\n\n%s\n\n", tools.Data(p, tools.Licence(p), string(text)))
	if len(have) > 0 {
		b.WriteString("The tree has these exercises about the target. Do not write the same task again, and prefer a group that they do not have:\n\n")
		for _, e := range have {
			fmt.Fprintf(&b, "- %s: group %s, difficulty %d, check %s\n", e.ID, e.Group, e.Difficulty, e.Check)
		}
		b.WriteString("\n")
	}
	if want >= 3 {
		b.WriteString("Write at least one exercise of group C: a proof, a conjecture or an evaluation.\n\n")
	}
	fmt.Fprintf(&b, "Add each exercise in nodes/~/ID.md, where ID is a new provisional id such as ~1a2b3c4d, in this form:\n\n---\ntag: ~ID\nkind: example\nname: the name of the exercise in words\nuses: [%s]\n---\n\n## Example\n\nThe task, which a reader can do with no other text.\n\n## Remarks\n\n%s About: %s. Difficulty: 2. Group: B. Check: exact.\n\n### Hint\n\nOne or two sentences.\n\n### Answer\n\nThe final answer.\n\n### Solution\n\n1. The first step, by its reason.\n2. The next step.\n", job.Target, Marker, job.Target)
	return b.String(), nil
}

func plural(n int, noun string) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}
