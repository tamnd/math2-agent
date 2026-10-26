// Package write has the goal and the brief of the write role (corpus K7.3 and K7.5).
//
// A write run takes one node to the next writing level (corpus K2.4). It writes the sections that the card of the kind of the node asks for at that level, such as the statement, the definition and the proof in steps, and it adds example nodes and informal proof nodes of the target. The write role does not write the explanations, which are the work of explain, or the History section, which needs sources that a person checks. The other items of a card, such as the sources, the bindings and the crosswalk edges, are the work of other roles and of people.
//
// The provenance log marks each section that the run writes as the work of an agent (spec 04.8). The run can change a section of the target only when the log shows that write wrote it and that no person confirmed it.
//
// [Goal] runs the gates of corpus K9.2 that need no model in the loop: m2 check (G1), the card (G2), the house style (G3, hard only for a W3 job) and the copy check (G4, when the run has a reference corpus). It also checks the form of two sections: a proof is a list of numbered steps (corpus K8.5), and a definition names the defined term in bold. [Brief] gives the first message of a run.
package write

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/curate"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/card"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/copycheck"
	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/site"
	"github.com/tamnd/math2-agent/style"
	"github.com/tamnd/math2-agent/workspace"
)

// Agent is the prefix of the agent of each provenance line that a write run makes, before the version of the role.
const Agent = "m2-agent/write@"

// madeIn are the edges into the target that a new node of write makes: an example, a non-example, an illustration and a proof.
var madeIn = []string{graph.InstanceOf, graph.NotInstance, graph.Illustrates, graph.Proves}

// Mine reports whether the write role makes the item of a card: a section other than Explanations and History, an edge into the node that a new example or proof makes, or the uses of the node.
func Mine(it card.Item) bool {
	switch {
	case it.Section != "":
		return it.Section != "Explanations" && it.Section != "History"
	case it.Dir == "in":
		return len(it.Edge) > 0 && !slices.ContainsFunc(it.Edge, func(t string) bool { return !slices.Contains(madeIn, t) })
	case it.Dir == "out":
		return slices.Equal(it.Edge, []string{graph.Uses})
	}
	return false
}

// Need is an item of the card that the node lacks, with the level that needs it.
type Need struct {
	card.Item
	Level      card.Level
	Have, Want int
}

// Needs returns the items of the card of the node that write makes, that the level l or a level below needs, and that the node lacks, in the order of the card. g is the graph of the tree.
func Needs(cards *card.Cards, n *node.Node, g *graph.Graph, l card.Level) []Need {
	items, _ := cards.Card(n.Kind())
	var out []Need
	for _, it := range items {
		at, ok := it.Needs()
		if !ok || at > l || !Mine(it) {
			continue
		}
		if have, want := card.Count(n, card.Env{Graph: g}, it); have < want {
			out = append(out, Need{Item: it, Level: at, Have: have, Want: want})
		}
	}
	return out
}

var levelRe = regexp.MustCompile(`\bW([1-3])\b`)

// Level returns the writing level of a job on the node: the level that the goal names, such as W2, or else the lowest level that needs an item of the card that write makes and that the node lacks. A node that lacks no such item gives W3.
func Level(cards *card.Cards, n *node.Node, g *graph.Graph, goal string) card.Level {
	if m := levelRe.FindStringSubmatch(goal); m != nil {
		return card.Level(m[1][0] - '0')
	}
	for l := card.W1; l < card.W3; l++ {
		if len(Needs(cards, n, g, l)) > 0 {
			return l
		}
	}
	return card.W3
}

// How returns what a write run does for the need of the target.
func How(nd Need, target string) string {
	switch {
	case nd.Section != "":
		return fmt.Sprintf("write the section %s", nd.Section)
	case slices.Equal(nd.Edge, []string{graph.Uses}):
		return "add to uses each node that the text needs"
	case slices.Equal(nd.Edge, []string{graph.Proves}):
		return fmt.Sprintf("add a proof node in nodes/~/ with proves: %s and a Proof section in numbered steps", target)
	case slices.Equal(nd.Edge, []string{graph.NotInstance}):
		return fmt.Sprintf("add an example node in nodes/~/ with not_instance_of: [{target: %s, fails: [N]}], where N is the number of the clause that fails", target)
	}
	return fmt.Sprintf("add an example node in nodes/~/ with %s: [%s]", nd.Edge[0], target)
}

// tree returns the nodes of the work tree before the changes and after them. A file that the changes add is not in the tree before them.
func tree(ws *workspace.Workspace, changes []workspace.Change) (before, after []*node.Node, err error) {
	files, err := check.ReadTree(ws.Dir)
	if err != nil {
		return nil, nil, err
	}
	changed := map[string]workspace.Change{}
	for _, c := range changes {
		changed[c.Path] = c
	}
	for _, f := range files {
		n, err := node.Parse(f.Path, f.Data)
		if err != nil {
			continue
		}
		after = append(after, n)
		c, ok := changed[f.Path]
		switch {
		case !ok:
			before = append(before, n)
		case c.Old != nil:
			if o, err := node.Parse(f.Path, c.Old); err == nil {
				before = append(before, o)
			}
		}
	}
	return before, after, nil
}

func find(nodes []*node.Node, id string) *node.Node {
	i := slices.IndexFunc(nodes, func(n *node.Node) bool { return n.ID == id })
	if i < 0 {
		return nil
	}
	return nodes[i]
}

// Steps reports whether the text of a proof is a list of numbered steps: its first line with text is the step 1.
func Steps(text string) bool {
	for l := range strings.Lines(text) {
		if l = strings.TrimSpace(l); l != "" {
			return strings.HasPrefix(l, "1. ")
		}
	}
	return false
}

var boldRe = regexp.MustCompile(`\*\*([^*]+)\*\*`)

// Bold reports whether the text names the node in bold: a bold term whose slug is the slug of the name or an alias of the node.
func Bold(n *node.Node, text string) bool {
	names := []string{site.Slug(n.Name())}
	for _, a := range n.List("aliases") {
		names = append(names, site.Slug(a))
	}
	for _, m := range boldRe.FindAllStringSubmatch(text, -1) {
		if s := site.Slug(m[1]); s != "" && slices.Contains(names, s) {
			return true
		}
	}
	return false
}

// written returns the body fields of the node after the changes that differ from the node before, with their text. A nil before gives each body field.
func written(before, after *node.Node) map[string]string {
	var old map[string]string
	if before != nil {
		old = prov.Fields(before)
	}
	out := map[string]string{}
	for f, v := range prov.Fields(after) {
		if strings.HasPrefix(f, "body.") && old[f] != v {
			out[f] = v
		}
	}
	return out
}

// form checks the form of the sections that the run wrote in the node.
func form(n *node.Node, fields map[string]string) string {
	if text, ok := fields["body.proof"]; ok && !Steps(text) {
		return fmt.Sprintf("the Proof section of %s is not a list of numbered steps. Write each step as \"1. claim, by reason\", with substeps such as 3.1 when they help", n.ID)
	}
	if text, ok := fields["body.definition"]; ok && !Bold(n, text) {
		return fmt.Sprintf("the Definition section of %s does not name the defined term in bold. Write the name of the node, or an alias, as **name** the first time", n.ID)
	}
	return ""
}

// newStyle returns the findings of the house style in the node after that the node before does not have. A nil before gives each finding of after.
func newStyle(before, after *node.Node) []check.Finding {
	seen := map[string]int{}
	if before != nil {
		for _, f := range style.Node(before) {
			seen[f.Rule+" "+f.Msg]++
		}
	}
	var out []check.Finding
	for _, f := range style.Node(after) {
		if k := f.Rule + " " + f.Msg; seen[k] > 0 {
			seen[k]--
			continue
		}
		out = append(out, f)
	}
	return out
}

func list(fs []check.Finding) string {
	lines := make([]string, 0, 5)
	for _, f := range fs[:min(len(fs), 5)] {
		lines = append(lines, f.String())
	}
	return strings.Join(lines, "; ")
}

// Goal returns the goal of a write run on the job, with the cards of the tree. A nil corpus skips the copy check, which then runs only in CI.
func Goal(ws *workspace.Workspace, job *agent.Job, cards *card.Cards, corpus *copycheck.Corpus) agent.Goal {
	return agent.OnChanges(ws, func(_ context.Context, changes []workspace.Change) (bool, string, error) {
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
		before, after, err := tree(ws, changes)
		if err != nil {
			return false, "", err
		}
		old, n := find(before, job.Target), find(after, job.Target)
		if old == nil || n == nil {
			return false, fmt.Sprintf("the tree has no node %s", job.Target), nil
		}
		log, err := prov.Read(ws.Dir, job.Target)
		if err != nil {
			return false, "", err
		}
		was := prov.Fields(old)
		mine := written(old, n)
		for _, f := range slices.Sorted(maps.Keys(union(was, prov.Fields(n)))) {
			if !strings.HasPrefix(f, "body.") || was[f] == "" || was[f] == prov.Fields(n)[f] {
				continue
			}
			l, ok := prov.Author(log, f)
			switch {
			case !ok:
				return false, fmt.Sprintf("the provenance log of %s does not show who wrote %s, thus write does not change it. Put the text back as it was", job.Target, f), nil
			case l.Human != "":
				return false, fmt.Sprintf("%s wrote or confirmed %s, and write does not change it. Put the text back as it was", l.Human, f), nil
			case !strings.HasPrefix(l.Agent, Agent):
				return false, fmt.Sprintf("%s wrote %s, and write changes only its own sections. Put the text back as it was", l.Agent, f), nil
			}
		}
		level := Level(cards, old, graph.Build(before), job.Goal)
		if needs := Needs(cards, n, graph.Build(after), level); len(needs) > 0 {
			var what []string
			for _, nd := range needs {
				what = append(what, How(nd, job.Target))
			}
			return false, fmt.Sprintf("the card of a %s at %s still needs this work: %s", n.Kind(), level, strings.Join(what, "; ")), nil
		}
		added := []*node.Node{}
		for _, c := range changes {
			if c.Old == nil {
				if m, err := node.Parse(c.Path, c.New); err == nil {
					added = append(added, m)
				}
			}
		}
		if why := form(n, mine); why != "" {
			return false, why, nil
		}
		for _, m := range added {
			if why := form(m, written(nil, m)); why != "" {
				return false, why, nil
			}
		}
		if level == card.W3 {
			fs := newStyle(old, n)
			for _, m := range added {
				fs = append(fs, newStyle(nil, m)...)
			}
			if len(fs) > 0 {
				return false, fmt.Sprintf("the house style finds %d problems in the text of the run: %s", len(fs), list(fs)), nil
			}
		}
		if corpus != nil {
			nodes := append([]*node.Node{n}, added...)
			for i, m := range nodes {
				fields := mine
				if i > 0 {
					fields = written(nil, m)
				}
				var sections []copycheck.Section
				for _, f := range slices.Sorted(maps.Keys(fields)) {
					sections = append(sections, copycheck.Section{Field: f, Text: fields[f]})
				}
				if r := corpus.Check(sections...); r.Blocks() {
					var at []string
					for _, x := range r.Matches() {
						if x.Blocks() {
							at = append(at, fmt.Sprintf("%s copies %s", x.Field, x.Source))
						}
					}
					return false, fmt.Sprintf("the copy check finds text of %s that copies a source whose licence math2 cannot take: %s. Write these parts again in your own words", m.ID, strings.Join(slices.Compact(at), "; ")), nil
				}
			}
		}
		return true, "", nil
	})
}

func union[V any](a, b map[string]V) map[string]V {
	u := maps.Clone(a)
	maps.Copy(u, b)
	return u
}

// Brief returns the first message of a write run: the job, the level that it takes the target to, the work that the card asks for, and the target node from the work tree in a data block.
func Brief(ws *workspace.Workspace, job *agent.Job, cards *card.Cards) (string, error) {
	p := tools.NodePath(job.Target)
	if p == "" {
		return "", fmt.Errorf("write: the target %q is not a tag or a provisional id", job.Target)
	}
	text, err := ws.Read(p)
	if err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	nodes, _, err := tree(ws, nil)
	if err != nil {
		return "", err
	}
	n := find(nodes, job.Target)
	if n == nil {
		return "", fmt.Errorf("write: %s does not parse", p)
	}
	g := graph.Build(nodes)
	level := Level(cards, n, g, job.Goal)
	var b strings.Builder
	fmt.Fprintf(&b, "Job %s: write the target %s, a %s node, at the writing level %s (%s). The goal of the job: %s.\n\n", job.ID, job.Target, n.Kind(), level, level.Word(), strings.TrimSuffix(job.Goal, "."))
	needs := Needs(cards, n, g, level)
	if len(needs) == 0 {
		b.WriteString("The node has each item of its card at this level that write makes. If you find no section to improve, call report_stuck.\n\n")
	} else {
		fmt.Fprintf(&b, "The card of a %s asks for this work at %s:\n\n", n.Kind(), level)
		for _, nd := range needs {
			fmt.Fprintf(&b, "- %s", How(nd, job.Target))
			if nd.Note != "" {
				fmt.Fprintf(&b, ": %s", nd.Note)
			}
			if nd.Want > 1 {
				fmt.Fprintf(&b, " (the node has %d of %d)", nd.Have, nd.Want)
			}
			b.WriteString(".\n")
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Write the sections in %s. Do not change the sections that a person wrote, the explanations or the front matter other than notation and uses.\n\n", p)
	fmt.Fprintf(&b, "The target node:\n\n%s\n", tools.Data(p, tools.Licence(p), string(text)))
	return b.String(), nil
}
