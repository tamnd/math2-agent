package audit

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/workspace"
)

// The rules X01 to X04 limit what agents change (spec 15.3). A merge is an agent pull request when one of the new lines of its provenance logs is of an agent with no person. The runs of the pull request are the runs that those lines name, and the audit reads their summaries in build/runs/.

// agentPR is a merge that is an agent pull request.
type agentPR struct {
	from, to string
	runs     []string           // the ids of the runs, sorted
	changes  []workspace.Change // the changed files, except the provenance logs and build/
}

// agentMerge returns the merge when it is an agent pull request, or nil. The reason is not empty when the rule cannot read the merge.
func (a *auditor) agentMerge() (pr *agentPR, reason string) {
	if !isRepo(a.root) {
		return nil, "the tree is not a git repository"
	}
	from, to, err := a.mergeRange()
	if err != nil {
		return nil, err.Error()
	}
	paths, err := a.changedFiles(from, to)
	if err != nil {
		return nil, err.Error()
	}
	pr = &agentPR{from: from, to: to}
	ids := map[string]bool{}
	for _, p := range paths {
		if strings.HasPrefix(p, prov.Dir+"/") && strings.HasSuffix(p, ".jsonl") {
			old, now := readLog(a.blob(from, p)), readLog(a.blob(to, p))
			for _, l := range now[min(len(old), len(now)):] {
				if l.err == nil && l.line.Agent != "" && l.line.Human == "" && l.line.Run != "" {
					ids[l.line.Run] = true
				}
			}
			continue
		}
		if strings.HasPrefix(p, "build/") {
			continue
		}
		pr.changes = append(pr.changes, workspace.Change{Path: p, Old: a.blob(from, p), New: a.blob(to, p)})
	}
	if len(ids) == 0 {
		return nil, ""
	}
	pr.runs = slices.Sorted(maps.Keys(ids))
	return pr, ""
}

// role returns the role of an agent id such as m2-agent/prove@0.1.1.
func role(agentID string) string {
	_, r, _ := strings.Cut(agentID, "/")
	r, _, _ = strings.Cut(r, "@")
	return r
}

// runPath is the path of the summary of a run in the tree.
func runPath(id string) string { return filepath.ToSlash(filepath.Join(runsDir, id+".json")) }

// x01 checks each change of an agent pull request against the field permission set of the role of its run (rule X01, spec 12.1). A change passes when the set of one run of the pull request allows it. A change outside the sets fails even if it is correct. The provenance logs are not checked here: rule P01 checks that they only grow.
func x01(a *auditor) outcome {
	var o outcome
	pr, reason := a.agentMerge()
	if pr == nil {
		o.reason = reason
		return o
	}
	runs, reason := a.runs()
	if reason != "" {
		o.reason = reason
		return o
	}
	type grant struct{ run, role, target string }
	var grants []grant
	for _, id := range pr.runs {
		r := runs[id]
		if r == nil {
			continue // P02 reports it
		}
		if len(r.Targets) == 0 {
			o.add(runPath(id), 1, 1, "the run %s names no target, thus its changes cannot be checked against the permissions of %s", id, role(r.Agent))
			continue
		}
		grants = append(grants, grant{id, role(r.Agent), r.Targets[0]})
	}
	if len(grants) == 0 {
		return o
	}
	for _, c := range pr.changes {
		o.checked++
		var first error
		for _, g := range grants {
			err := agent.Permit(g.role, g.target, []workspace.Change{c})
			if err == nil {
				first = nil
				break
			}
			if first == nil {
				why := strings.TrimPrefix(err.Error(), "agent: the changes are outside the permissions of "+g.role+": "+c.Path+": ")
				first = fmt.Errorf("the run %s of %s: %s", g.run, g.role, strings.TrimPrefix(why, "agent: "))
			}
		}
		if first != nil {
			o.add(c.Path, 1, 1, "the agent pull request changes this file outside the field permissions of its role. For %v", first)
		}
	}
	return o
}

// x02 checks that each run of an agent pull request records its spend and its budget, and spent no more than the budget (rule X02).
func x02(a *auditor) outcome {
	var o outcome
	pr, reason := a.agentMerge()
	if pr == nil {
		o.reason = reason
		return o
	}
	runs, reason := a.runs()
	if reason != "" {
		o.reason = reason
		return o
	}
	for _, id := range pr.runs {
		r := runs[id]
		if r == nil {
			continue // P02 reports it
		}
		o.checked++
		p := runPath(id)
		switch {
		case r.SpendUSD <= 0:
			o.add(p, 1, 1, "the summary of the run %s does not show the cost of the run", id)
		case r.BudgetUSD <= 0:
			o.add(p, 1, 1, "the summary of the run %s does not show the budget of the run", id)
		case r.SpendUSD > r.BudgetUSD:
			o.add(p, jsonKeyLine(a, p, "spend_usd"), 1, "the run %s spent $%.4f, more than its budget of $%.2f", id, r.SpendUSD, r.BudgetUSD)
		}
	}
	return o
}

// The limits of rule X03. A run counts for the acceptance rate in the week that ends acceptGrace before the date of the HEAD commit, thus each pull request of a run had a week for review.
const (
	acceptWindow = 7 * 24 * time.Hour
	acceptGrace  = 7 * 24 * time.Hour
	acceptMin    = 5    // the fewest runs of a role and a model for a rate
	acceptRate   = 0.25 // the rate below which the rule asks for a review of the prompt or the model
)

// x03 finds each role and model with an acceptance rate below 25% (rule X03). The rate is the part of the runs whose output is on the branch: a provenance line names the run. The runs are those that started in the week that ends a week before the date of the HEAD commit. A role and a model with fewer than five runs in the week are not checked.
func x03(a *auditor) outcome {
	var o outcome
	if !isRepo(a.root) {
		o.reason = "the tree is not a git repository"
		return o
	}
	runs, reason := a.runs()
	if reason != "" {
		o.reason = reason
		return o
	}
	head, err := headDate(a.root)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	logs, err := prov.ReadAll(a.root)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	accepted := map[string]bool{}
	for _, ls := range logs {
		for _, l := range ls {
			if l.Agent != "" && l.Run != "" {
				accepted[l.Run] = true
			}
		}
	}
	end := head.Add(-acceptGrace)
	start := end.Add(-acceptWindow)
	type group struct {
		ids []string
		ok  int
	}
	groups := map[string]*group{}
	for _, id := range slices.Sorted(maps.Keys(runs)) {
		r := runs[id]
		t, err := time.Parse(time.RFC3339, r.Started)
		if err != nil || t.Before(start) || !t.Before(end) {
			continue
		}
		for _, m := range r.Models {
			k := role(r.Agent) + " on " + m
			g := groups[k]
			if g == nil {
				g = &group{}
				groups[k] = g
			}
			g.ids = append(g.ids, id)
			if accepted[id] {
				g.ok++
			}
		}
	}
	for _, k := range slices.Sorted(maps.Keys(groups)) {
		g := groups[k]
		if len(g.ids) < acceptMin {
			continue
		}
		o.checked++
		if rate := float64(g.ok) / float64(len(g.ids)); rate < acceptRate {
			o.add(runPath(g.ids[len(g.ids)-1]), 1, 1, "the acceptance rate of %s is %.0f%%: %d of the %d runs from %s to %s are on the branch. The rate is below %.0f%%, thus review the prompt or the model", k, 100*rate, g.ok, len(g.ids), start.Format(time.DateOnly), end.Add(-time.Second).Format(time.DateOnly), 100*acceptRate)
		}
	}
	return o
}

// statusParts are the parts of a field name that state a status or a review. Rule S03 forbids the first seven at the top of a node file.
var statusParts = []string{"status", "verified", "proved", "evidence", "fidelity", "level", "checked", "fidelity_reviews", "reviews"}

// statusField reports whether a part of the field, such as formal[0].fidelity_reviews, states a status or a review.
func statusField(field string) bool {
	for part := range strings.FieldsFuncSeq(field, func(r rune) bool { return r == '.' || r == '[' || r == ']' }) {
		if slices.Contains(statusParts, part) {
			return true
		}
	}
	return false
}

// statusValues returns the value of each field of the front matter, at any depth, whose last part states a status or a review. A nil node has none.
func statusValues(n *node.Node) map[string]string {
	out := map[string]string{}
	if n == nil {
		return out
	}
	var walk func(path string, v any)
	walk = func(path string, v any) {
		switch v := v.(type) {
		case map[string]any:
			for k, x := range v {
				p := k
				if path != "" {
					p = path + "." + k
				}
				if slices.Contains(statusParts, k) {
					out[p] = fmt.Sprint(x)
				}
				walk(p, x)
			}
		case []any:
			for i, x := range v {
				walk(fmt.Sprintf("%s[%d]", path, i), x)
			}
		}
	}
	walk("", n.Meta)
	return out
}

// union returns the keys of the two maps.
func union(a, b map[string]string) map[string]string {
	out := maps.Clone(a)
	maps.Copy(out, b)
	return out
}

// x04 checks that no agent wrote a status field or a review (rule X04). No provenance line of an agent with no person may be on such a field. An agent pull request may not change a file below reviews/ or a status field of a node.
func x04(a *auditor) outcome {
	var o outcome
	logs, err := a.agentLines()
	if err != nil {
		o.reason = err.Error()
		return o
	}
	for _, p := range slices.Sorted(maps.Keys(logs)) {
		for _, l := range logs[p] {
			if l.line.Human != "" {
				continue
			}
			o.checked++
			if statusField(l.line.Field) {
				o.add(p, l.n, 1, "the agent %s wrote the field %s, which states a status or a review. The build computes the status, and only a person writes a review", l.line.Agent, l.line.Field)
			}
		}
	}
	pr, reason := a.agentMerge()
	if pr == nil {
		if o.checked == 0 {
			o.reason = reason
		}
		return o
	}
	for _, c := range pr.changes {
		o.checked++
		switch {
		case strings.HasPrefix(c.Path, "reviews/"):
			o.add(c.Path, 1, 1, "the agent pull request changes a review file. Only a person writes a review")
		case strings.HasPrefix(c.Path, "nodes/") && strings.HasSuffix(c.Path, ".md"):
			before, after := statusValues(a.nodeAt(pr.from, c.Path)), statusValues(a.nodeAt(pr.to, c.Path))
			at := a.nodeAt(pr.to, c.Path)
			for _, f := range slices.Sorted(maps.Keys(union(before, after))) {
				if before[f] != after[f] {
					line, col := 1, 1
					if at != nil {
						line, col = fieldPos(at, f)
					}
					o.add(c.Path, line, col, "the agent pull request changes the field %s, which states a status or a review", f)
				}
			}
		}
	}
	return o
}
