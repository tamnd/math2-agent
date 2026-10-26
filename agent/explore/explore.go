// Package explore has the goal, the brief and the tools of the explore role (spec 12.5).
//
// An explore run works inside a project that a person defined. The model writes computations, runs them in the sandbox of the computational engine with the tool of [Run], and files what it finds as conjecture and counterexample nodes. It looks up each claim with the tool of [Lookup] before it files it. The role writes no Lean and changes no node that exists.
//
// [Goal] holds when the changes are inside the field permission set of explore, m2 check finds nothing in the changed nodes, the run filed at least one conjecture or counterexample, each such output rests on a computation, each new computation verifies, and the novelty check (spec 17.5) ran on each output. A conjecture links to a computation that checks the pattern, and a counterexample refutes a statement through a computation. A computation verifies when it exits with the code 0 in time, its output has the hash of its node, and the fixed checker accepts its certificate when it has one. An output whose computation has a certificate can reach E4 in the build, and an output with no certificate stays at E1. No verdict of the novelty check says that an output is new, and the goal refuses a node that says so. [Novelty] runs the check and keeps the result of each output for the run record.
package explore

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/curate"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/api"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/compute"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/novelty"
	"github.com/tamnd/math2-agent/tag"
	"github.com/tamnd/math2-agent/workspace"
)

// MaxOutput is the number of bytes of the standard output of a computation that the tool of [Run] gives the model.
const MaxOutput = 4000

// Outputs are the kinds of the nodes that a run files as its outputs.
var Outputs = []string{"conjecture", "counterexample"}

// Filed returns the conjecture and counterexample nodes that the changes add, in the order of their paths, and the computation nodes that they add, by id.
func Filed(changes []workspace.Change) (outputs []*node.Node, computations map[string]*node.Node, err error) {
	computations = map[string]*node.Node{}
	for _, c := range changes {
		if c.Old != nil || c.New == nil || !strings.HasPrefix(c.Path, "nodes/") || !strings.HasSuffix(c.Path, ".md") {
			continue
		}
		n, err := node.Parse(c.Path, c.New)
		if err != nil {
			return nil, nil, err
		}
		switch {
		case slices.Contains(Outputs, n.Kind()):
			outputs = append(outputs, n)
		case n.Kind() == "computation":
			computations[n.ID] = n
		}
	}
	return outputs, computations, nil
}

// Evidence returns the nodes that the output names as its evidence, in order and with no repeats: for a conjecture, the nodes that it links to and the nodes of its uses field, and for a counterexample, the via of each refutes edge.
func Evidence(n *node.Node) []string {
	var out []string
	add := func(id string) {
		if id != "" && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	if n.Kind() == "counterexample" {
		list, _ := n.Meta["refutes"].([]any)
		for _, e := range list {
			if m, ok := e.(map[string]any); ok {
				via, _ := m["via"].(string)
				add(via)
			}
		}
		return out
	}
	for _, l := range check.Links(n) {
		add(l.Target)
	}
	for _, id := range n.List("uses") {
		add(id)
	}
	return out
}

var termsRe = regexp.MustCompile(`-?\d+(?:\s*,\s*-?\d+){3,}`)

// ClaimOf returns the claim of an output for the novelty check: its name, the text of its Statement or Counterexample section, and the first run of four or more integers in that text, as the terms of a sequence.
func ClaimOf(n *node.Node) novelty.Claim {
	c := novelty.Claim{Tag: n.ID, Name: n.Name()}
	for _, s := range check.Sections(n) {
		if s.Name == "Statement" || s.Name == "Counterexample" {
			c.Statement = strings.TrimSpace(s.Text)
			break
		}
	}
	if c.Statement == "" {
		c.Statement = strings.TrimSpace(n.Body)
	}
	if m := termsRe.FindString(c.Statement); m != "" {
		for t := range strings.SplitSeq(m, ",") {
			c.Terms = append(c.Terms, strings.TrimSpace(t))
		}
	}
	return c
}

var newRe = regexp.MustCompile(`(?i)\bnovel\b|\bnew\s+(?:result|theorem|conjecture|counterexample|discovery|construction)s?\b|\bfor the first time\b|\bto (?:our|my) knowledge\b`)

// ClaimsNew returns the first words of the text that say that a result is new, such as "a new theorem" or "for the first time", or "" when the text does not say so.
func ClaimsNew(text string) string { return newRe.FindString(text) }

// Novelty runs the novelty check (spec 17.5) on the claims of a run, and keeps the result of each output for the run record. The check runs each search once for each claim, and again only when the claim changes. With no search, the check finds no match. Novelty is not safe for concurrent use.
type Novelty struct {
	Searchers []novelty.Searcher
	Judge     novelty.Judge // nil means that each match is likely-known

	done map[string]*novelty.Result // by the claim
	last map[string]*novelty.Result // by the id of the output
}

// Check runs the novelty check on the claim. A result with an error is a check whose verdict is no-match-found while a search failed. Check runs it again the next time.
func (nv *Novelty) Check(ctx context.Context, c novelty.Claim) (*novelty.Result, error) {
	key := strings.Join(append([]string{c.Tag, c.Name, c.Statement}, c.Terms...), "\x00")
	if r, ok := nv.done[key]; ok {
		return r, nil
	}
	r, err := novelty.Check(ctx, c, nv.Judge, nv.Searchers...)
	if err != nil {
		return r, err
	}
	if nv.done == nil {
		nv.done = map[string]*novelty.Result{}
	}
	nv.done[key] = r
	return r, nil
}

// Results returns the result of the check of each output that the goal checked last, by the id of its node, or nil when the goal checked no output.
func (nv *Novelty) Results() map[string]*novelty.Result { return maps.Clone(nv.last) }

// judgeTool is the tool with which the model of the judge answers.
var judgeTool = llm.Tool{
	Name:        "judge",
	Description: "Give what the claim is to the match: same, generalizes, specializes, related or different, with the reason.",
	Schema: json.RawMessage(`{"type":"object","properties":{` +
		`"verdict":{"type":"string","enum":["same","generalizes","specializes","related","different"]},` +
		`"why":{"type":"string","description":"the reason, with the map between the statements"}},"required":["verdict","why"],"additionalProperties":false}`),
}

// Judge returns a judge of the novelty check that compares the claim with a match in one call of the model m, with the model name and the system prompt. The claim and the match go to the model in data blocks, thus an instruction in them is data. An answer with no verdict is different.
func Judge(m llm.Model, model, system string) novelty.Judge {
	return func(ctx context.Context, c novelty.Claim, match novelty.Match) (string, string, error) {
		var b strings.Builder
		fmt.Fprintf(&b, "Compare the claim with the match, and give the verdict with the tool %s.\n\nThe claim:\n\n%s\n\nThe match:\n\n%s\n", judgeTool.Name,
			tools.Data("the claim", "CC-BY-SA-4.0", c.Name+"\n\n"+c.Statement),
			tools.Data(match.Source+":"+match.ID, "NOASSERTION", strings.TrimSpace(match.Title+"\n\n"+match.Text)))
		resp, err := m.Complete(ctx, &llm.Request{
			Model:     model,
			System:    system,
			Messages:  []llm.Message{{Role: llm.User, Content: []llm.Block{llm.TextBlock(b.String())}}},
			Tools:     []llm.Tool{judgeTool},
			MaxTokens: 2000,
		})
		if err != nil {
			return "", "", fmt.Errorf("explore: the judge: %w", err)
		}
		for _, call := range resp.Calls() {
			if call.Name != judgeTool.Name {
				continue
			}
			var v struct{ Verdict, Why string }
			if err := json.Unmarshal(call.Input, &v); err != nil {
				return "", "", fmt.Errorf("explore: the judge gave a verdict that is not valid: %w", err)
			}
			if slices.Contains([]string{novelty.Same, novelty.Generalizes, novelty.Specializes, novelty.Related, novelty.Different}, v.Verdict) {
				return v.Verdict, v.Why, nil
			}
		}
		return novelty.Different, "the judge gave no verdict", nil
	}
}

// in returns a copy of the runner that runs the code of the work tree.
func in(ws *workspace.Workspace, r *compute.Runner) *compute.Runner {
	c := *r
	c.Root = ws.Dir
	return &c
}

// record reads the record of the computation node id from the work tree.
func record(ws *workspace.Workspace, id string) (*compute.Record, error) {
	p := tools.NodePath(id)
	if p == "" {
		return nil, fmt.Errorf("%q is not a tag or a provisional id", id)
	}
	data, err := ws.Read(p)
	if err != nil {
		return nil, err
	}
	n, err := node.Parse(p, data)
	if err != nil {
		return nil, err
	}
	return compute.FromNode(n)
}

// Run returns the tool run_computation, which runs a computation node of the work tree in the sandbox of the runner and gives the model the result: the exit code, the time, the hash of the output and of the certificate, and the first MaxOutput bytes of the standard output. The output directory of the runner must be outside the work tree, because the runs are not changes of the run.
func Run(ws *workspace.Workspace, r *compute.Runner) agent.Tool {
	return agent.Func{
		Def: llm.Tool{
			Name:        "run_computation",
			Description: "Run a computation node of the work tree in the sandbox, with no network. Write the node, and its code in compute/~/ID/, first. The result gives the hash of the output and of the certificate, which the node must record.",
			Schema: tools.Schema([]api.Param{
				{Name: "computation", Type: api.String, Required: true, Summary: "the tag or the provisional id of the computation node"},
			}),
		},
		Fn: func(ctx context.Context, _ *agent.Job, input json.RawMessage) (agent.Result, error) {
			var args struct{ Computation string }
			if err := json.Unmarshal(input, &args); err != nil {
				return agent.Result{}, err
			}
			if r.Out == "" {
				return agent.Result{}, errors.New("explore: the runner has no output directory")
			}
			rec, err := record(ws, args.Computation)
			if err != nil {
				return agent.Result{}, err
			}
			rr := in(ws, r)
			run, err := rr.Run(ctx, rec)
			if err != nil {
				return agent.Result{}, err
			}
			stdout, err := os.ReadFile(filepath.Join(rr.Dir(rec.Tag), compute.StdoutFile))
			if err != nil {
				return agent.Result{}, err
			}
			var b strings.Builder
			fmt.Fprintf(&b, "The computation %s ran on %s in %.1f s with the exit code %d.", rec.Tag, run.Arch, run.Seconds, run.ExitCode)
			if run.TimedOut {
				b.WriteString(" It took longer than its wall time, and the sandbox stopped it.")
			}
			if run.Truncated {
				b.WriteString(" Its output is longer than the limit of the sandbox.")
			}
			fmt.Fprintf(&b, "\nThe output hash: %s", run.Output)
			if run.Certificate != "" {
				fmt.Fprintf(&b, "\nThe certificate hash: %s", run.Certificate)
			}
			if len(stdout) > MaxOutput {
				fmt.Fprintf(&b, "\nThe first %d of the %d bytes of the output:", MaxOutput, len(stdout))
				stdout = stdout[:MaxOutput]
			} else {
				b.WriteString("\nThe output:")
			}
			fmt.Fprintf(&b, "\n\n%s", tools.Data(rec.Dir()+"/stdout", tools.Licence(rec.Dir()), string(stdout)))
			return agent.Result{Text: b.String(), ComputeCPUS: run.Seconds * float64(max(rec.Resources.CPU, 1))}, nil
		},
	}
}

// Lookup returns the tool lookup, which runs the novelty check on a claim before the model files it, and gives the verdict and the matches.
func Lookup(nv *Novelty) agent.Tool {
	return agent.Func{
		Def: llm.Tool{
			Name:        "lookup",
			Description: "Run the novelty check on a claim: search the graph, the record plane and the external indexes, and compare the best matches with the claim. The verdict is known, likely-known or no-match-found. No verdict says that the claim is new.",
			Schema: tools.Schema([]api.Param{
				{Name: "name", Type: api.String, Required: true, Summary: "a short name of the claim"},
				{Name: "statement", Type: api.String, Required: true, Summary: "the statement in words, with LaTeX in $...$"},
				{Name: "terms", Type: api.String, Summary: "for a claim about an integer sequence, its first terms, separated by commas"},
			}),
		},
		Fn: func(ctx context.Context, _ *agent.Job, input json.RawMessage) (agent.Result, error) {
			var args struct{ Name, Statement, Terms string }
			if err := json.Unmarshal(input, &args); err != nil {
				return agent.Result{}, err
			}
			c := novelty.Claim{Name: strings.TrimSpace(args.Name), Statement: strings.TrimSpace(args.Statement)}
			for t := range strings.SplitSeq(args.Terms, ",") {
				if t = strings.TrimSpace(t); t != "" {
					c.Terms = append(c.Terms, t)
				}
			}
			res, err := nv.Check(ctx, c)
			if res == nil {
				return agent.Result{}, err
			}
			return agent.Result{Text: Report(res, err)}, nil
		},
	}
}

// Report gives the result of a novelty check in words for the model: the verdict, each failed search, and each match with the verdict of the judge. The titles of the matches are in data blocks.
func Report(res *novelty.Result, err error) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The verdict: %s.", res.Verdict)
	if res.Match != nil {
		fmt.Fprintf(&b, " The match %s:%s states the claim or a result that implies it.", res.Match.Source, res.Match.ID)
	}
	if err != nil {
		fmt.Fprintf(&b, " The check is not complete: %v.", err)
	}
	for _, s := range res.Searches {
		if s.Error != "" {
			fmt.Fprintf(&b, "\nThe search %s failed: %s.", s.Source, s.Error)
		}
	}
	for _, m := range res.Matches {
		fmt.Fprintf(&b, "\n\n%s:%s %s", m.Source, m.ID, m.URL)
		if m.Verdict != "" {
			fmt.Fprintf(&b, "\nThe judge: %s. %s", m.Verdict, m.Why)
		}
		if m.Title != "" {
			fmt.Fprintf(&b, "\n%s", tools.Data(m.Source+":"+m.ID, "NOASSERTION", m.Title))
		}
	}
	return b.String()
}

// Goal returns the goal predicate of an explore run on the work tree. The runner runs the new computations, and nv runs the novelty check on each output.
func Goal(ws *workspace.Workspace, job *agent.Job, r *compute.Runner, nv *Novelty) agent.Goal {
	return agent.OnChanges(ws, func(ctx context.Context, changes []workspace.Change) (bool, string, error) {
		if err := agent.Permit(job.Role, job.Target, changes); err != nil {
			return false, err.Error(), nil
		}
		findings, err := curate.Check(ws, changes)
		if err != nil {
			return false, "", err
		}
		if len(findings) > 0 {
			var lines []string
			for _, f := range findings[:min(len(findings), 5)] {
				lines = append(lines, f.String())
			}
			return false, "m2 check finds problems in the changed nodes: " + strings.Join(lines, "; "), nil
		}
		outputs, computations, err := Filed(changes)
		if err != nil {
			return false, err.Error(), nil
		}
		if len(outputs) == 0 {
			return false, "the run has filed no conjecture or counterexample yet. File what the computations show as a conjecture or a counterexample node", nil
		}
		for _, c := range changes {
			if id, ok := tag.CodeID(c.Path); ok && computations[id] == nil {
				return false, fmt.Sprintf("the run adds %s, and it adds no computation node %s", c.Path, id), nil
			}
		}
		for _, n := range outputs {
			if w := ClaimsNew(n.Name() + "\n" + n.Body); w != "" {
				return false, fmt.Sprintf("%s says %q. Only a person states that a result is new, after the novelty check. Remove the words", n.ID, w), nil
			}
			var evidence []string
			for _, id := range Evidence(n) {
				if computations[id] != nil {
					evidence = append(evidence, id)
				} else if rec, err := record(ws, id); err == nil && rec != nil {
					evidence = append(evidence, id)
				}
			}
			if len(evidence) == 0 {
				if n.Kind() == "counterexample" {
					return false, fmt.Sprintf("the counterexample %s refutes no statement through a computation. Give refutes: [{target: TAG, via: ID}], where ID is the computation that checks the counterexample", n.ID), nil
				}
				return false, fmt.Sprintf("the conjecture %s links to no computation. Falsify before filing: run a computation that checks the pattern to a larger bound than the one where you found it, and link the conjecture to it", n.ID), nil
			}
		}
		for _, id := range slices.Sorted(maps.Keys(computations)) {
			rec, err := compute.FromNode(computations[id])
			if err != nil {
				return false, err.Error(), nil
			}
			run, err := in(ws, r).Verify(ctx, rec)
			if err != nil {
				return false, err.Error(), nil
			}
			if !run.Verified {
				return false, fmt.Sprintf("the computation %s does not verify: %s", id, run.Problem), nil
			}
		}
		last := map[string]*novelty.Result{}
		defer func() { nv.last = last }()
		for _, n := range outputs {
			res, err := nv.Check(ctx, ClaimOf(n))
			if res != nil {
				last[n.ID] = res
			}
			if err != nil {
				return false, "", fmt.Errorf("explore: the novelty check of %s: %w", n.ID, err)
			}
		}
		return true, "", nil
	})
}

// Brief returns the first message of an explore run: the job and its project, the rules of the role, the images of the sandbox, and the target node from the work tree in a data block. The job must run on behalf of a project, because a person decides what is worth exploring (spec 12.5).
func Brief(ws *workspace.Workspace, job *agent.Job, r *compute.Runner) (string, error) {
	if !strings.HasPrefix(job.OnBehalfOf, "project:") {
		return "", fmt.Errorf("explore: a job of explore runs inside a project that a person defined, and this job runs on behalf of %q", job.OnBehalfOf)
	}
	p := tools.NodePath(job.Target)
	if p == "" {
		return "", fmt.Errorf("explore: the target %q is not a tag or a provisional id", job.Target)
	}
	text, err := ws.Read(p)
	if err != nil {
		return "", fmt.Errorf("explore: %w", err)
	}
	n, err := node.Parse(p, text)
	if err != nil {
		return "", fmt.Errorf("explore: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Job %s of %s: explore from the target %s, a %s node. The goal of the job: %s.\n\n", job.ID, job.OnBehalfOf, job.Target, n.Kind(), strings.TrimSuffix(job.Goal, "."))
	b.WriteString("Write each computation as a node of kind computation in nodes/~/ID.md, where ID is a new provisional id such as ~1a2b3c4d, with its code in compute/~/ID/. Run it with run_computation, and record the output hash in the node. File what you find as a conjecture or a counterexample node. A conjecture links to the computation that checks it, and a counterexample gives refutes: [{target: TAG, via: ID}]. Check each claim with lookup before you file it.\n\n")
	if r != nil && r.Images != nil {
		arch := cmp.Or(r.Arch, runtime.GOARCH)
		var lines []string
		for _, img := range r.Images.Images {
			if d := img.Digests[arch]; d != "" {
				lines = append(lines, fmt.Sprintf("- %s@%s: %s", img.Name, d, cmp.Or(img.Contents, img.Name)))
			}
		}
		if len(lines) > 0 {
			fmt.Fprintf(&b, "The images of the sandbox:\n\n%s\n\n", strings.Join(lines, "\n"))
		}
	}
	fmt.Fprintf(&b, "The target node:\n\n%s\n", tools.Data(p, tools.Licence(p), string(text)))
	return b.String(), nil
}
