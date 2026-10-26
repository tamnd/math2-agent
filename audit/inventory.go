package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/inventory"
)

// invTree is the inventory of the tree, read when a rule needs it.
type invTree struct {
	rows []invRow
	ms   map[string]*inventory.Mention
	byID map[string]*inventory.Row
	err  error
}

// invRow is a row and its place in the tree.
type invRow struct {
	inventory.Row
	file string
	line int
}

func (a *auditor) inventory() *invTree {
	if a.inv != nil {
		return a.inv
	}
	t := &invTree{byID: map[string]*inventory.Row{}, ms: map[string]*inventory.Mention{}}
	a.inv = t
	t.err = inventory.WalkRows(a.root, func(file string, line int, r inventory.Row) error {
		t.rows = append(t.rows, invRow{r, file, line})
		return nil
	})
	if t.err != nil {
		return t
	}
	ms, err := inventory.ReadMentions(a.root)
	if err != nil {
		t.err = err
		return t
	}
	for i := range ms {
		t.ms[ms[i].ID] = &ms[i]
	}
	for i := range t.rows {
		t.byID[t.rows[i].ID] = &t.rows[i].Row
	}
	return t
}

// skip returns the outcome of a rule that reads the rows when the inventory cannot be read or has no rows.
func (t *invTree) skip() (outcome, bool) {
	var o outcome
	if t.err != nil {
		o.add(inventory.RowsDir, 0, 0, "%v", t.err)
		return o, true
	}
	if len(t.rows) == 0 {
		o.reason = "the tree has no inventory"
		return o, true
	}
	return o, false
}

// i01 checks that each inventory value in a node names a row whose outcome is node or member with the tag of the node (rule I01, corpus spec K4.9).
func i01(a *auditor) outcome {
	t := a.inventory()
	var o outcome
	if t.err != nil {
		o.add(inventory.RowsDir, 0, 0, "%v", t.err)
		return o
	}
	for _, n := range a.nodes {
		if !a.inScope(n.Path) {
			continue
		}
		ids, line := listField(n.Front, "inventory")
		for _, id := range ids {
			o.checked++
			r := t.byID[id]
			switch {
			case r == nil:
				o.add(n.Path, line, 1, "the inventory row %s does not exist", id)
			case r.Outcome == nil || r.Outcome.Value != inventory.OutcomeNode && r.Outcome.Value != inventory.OutcomeMember:
				o.add(n.Path, line, 1, "the inventory row %s has the outcome %s, and it must be node or member", id, outcomeValue(r))
			case !slices.Contains(r.Outcome.Target, n.ID):
				o.add(n.Path, line, 1, "the inventory row %s does not name the node %s", id, n.ID)
			}
		}
	}
	if o.checked == 0 && len(o.findings) == 0 {
		o.reason = "no node has an inventory value"
	}
	return o
}

// i02 checks that each row with the outcome node or member names a live node, or a retired node with a successor (rule I02).
func i02(a *auditor) outcome {
	t := a.inventory()
	o, stop := t.skip()
	if stop {
		return o
	}
	live, succ := map[string]bool{}, map[string]bool{}
	for _, n := range a.nodes {
		ret, ok := n.Meta["retired"].(map[string]any)
		if !ok {
			live[n.ID] = true
		} else if s, _ := ret["successor"].([]any); len(s) > 0 {
			succ[n.ID] = true
		}
	}
	for _, r := range t.rows {
		if r.Outcome == nil || r.Outcome.Value != inventory.OutcomeNode && r.Outcome.Value != inventory.OutcomeMember {
			continue
		}
		o.checked++
		if len(r.Outcome.Target) == 0 {
			o.add(r.file, r.line, 1, "the row %s has the outcome %s and names no node", r.ID, r.Outcome.Value)
		}
		for _, tg := range r.Outcome.Target {
			if !live[tg] && !succ[tg] {
				o.add(r.file, r.line, 1, "the row %s names the node %s, which is not live and has no successor", r.ID, tg)
			}
		}
	}
	if o.checked == 0 {
		o.reason = "no row has the outcome node or member"
	}
	return o
}

// i03 checks that no row has only mentions from models (rule I03). Such mentions wait until a source that people wrote confirms them.
func i03(a *auditor) outcome {
	t := a.inventory()
	o, stop := t.skip()
	if stop {
		return o
	}
	for _, r := range t.rows {
		if len(r.Mentions) == 0 {
			continue // a merged row gave its mentions to another row
		}
		o.checked++
		models := 0
		for _, id := range r.Mentions {
			if m := t.ms[id]; m != nil && inventory.FromModel(m) {
				models++
			}
		}
		if models == len(r.Mentions) {
			o.add(r.file, r.line, 1, "the row %s has only mentions from models", r.ID)
		}
	}
	return o
}

// i04 checks that each outcome that a rule gave names a rule in schema/outcomes.v1.yaml (rule I04).
func i04(a *auditor) outcome {
	t := a.inventory()
	o, stop := t.skip()
	if stop {
		return o
	}
	const file = "schema/outcomes.v1.yaml"
	rules, err := outcomeRules(filepath.Join(a.root, filepath.FromSlash(file)))
	if err != nil {
		o.add(file, 0, 0, "%v", err)
		return o
	}
	for _, r := range t.rows {
		if r.Outcome == nil || r.Outcome.Rule == "" {
			continue
		}
		o.checked++
		if want, ok := rules[r.Outcome.Rule]; !ok {
			o.add(r.file, r.line, 1, "the row %s names the rule %s, which is not in %s", r.ID, r.Outcome.Rule, file)
		} else if want != r.Outcome.Value {
			o.add(r.file, r.line, 1, "the row %s has the outcome %s from the rule %s, which gives %s", r.ID, r.Outcome.Value, r.Outcome.Rule, want)
		}
	}
	if o.checked == 0 {
		o.reason = "no outcome came from a rule"
	}
	return o
}

// i05 reports each deferred row whose date has passed (rule I05). Such a row is open again.
func i05(a *auditor) outcome {
	t := a.inventory()
	o, stop := t.skip()
	if stop {
		return o
	}
	today := time.Now().UTC()
	for _, r := range t.rows {
		if r.Outcome == nil || r.Outcome.Value != inventory.OutcomeDeferred {
			continue
		}
		o.checked++
		if r.IsOpen(today) {
			if r.Outcome.Until == "" {
				o.add(r.file, r.line, 1, "the row %s is deferred and has no date", r.ID)
			} else {
				o.add(r.file, r.line, 1, "the row %s was deferred until %s, and the date has passed", r.ID, r.Outcome.Until)
			}
		}
	}
	if o.checked == 0 {
		o.reason = "no row is deferred"
	}
	return o
}

func outcomeValue(r *inventory.Row) string {
	if r.Outcome == nil {
		return "open"
	}
	return r.Outcome.Value
}

// outcomeRules reads the rules of schema/outcomes.v1.yaml and gives the outcome of each rule.
func outcomeRules(path string) (map[string]string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the path is in the tree that the caller audits.
	if err != nil {
		return nil, err
	}
	var doc struct {
		Rules []struct {
			ID      string `yaml:"id"`
			Outcome string `yaml:"outcome"`
		} `yaml:"rules"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, r := range doc.Rules {
		if r.ID == "" || !slices.Contains(inventory.Outcomes(), r.Outcome) {
			return nil, fmt.Errorf("the rule %q has the outcome %q, which is not an outcome of corpus spec K4.4", r.ID, r.Outcome)
		}
		out[r.ID] = r.Outcome
	}
	return out, nil
}

// listField returns the string values of a list field of the front matter, and the line of the field.
func listField(front *yaml.Node, key string) ([]string, int) {
	if front == nil || len(front.Content) == 0 {
		return nil, 0
	}
	m := front.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != key {
			continue
		}
		v := m.Content[i+1]
		if v.Kind == yaml.ScalarNode {
			return []string{v.Value}, v.Line
		}
		var out []string
		for _, c := range v.Content {
			out = append(out, c.Value)
		}
		return out, v.Line
	}
	return nil, 0
}
