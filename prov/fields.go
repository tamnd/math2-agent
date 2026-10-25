package prov

import (
	"maps"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/node"
)

// explanationRe matches the heading of one explanation, such as "L2 agent", and gives the level.
var explanationRe = regexp.MustCompile(`^(L[1-4])\b`)

// Fields returns the fields of the node with their values as text. Each key of the front matter is a field. Each level 2 section of the body is the field body. and the name of the section in lower case with _ for a space, for example body.statement. Each explanation in the Explanations section is a field of its own, for example body.explanations.L2. The text before the first section is the field body.
func Fields(n *node.Node) map[string]string {
	out := map[string]string{}
	for k, v := range n.Meta {
		b, err := yaml.Marshal(v)
		if err != nil {
			b = []byte("?")
		}
		out[k] = string(b)
	}
	lines := strings.Split(n.Body, "\n")
	sections := check.Sections(n)
	first := len(lines)
	if len(sections) > 0 {
		first = sections[0].Line - n.BodyLine
	}
	if pre := strings.TrimSpace(strings.Join(lines[:max(0, min(first, len(lines)))], "\n")); pre != "" {
		out["body"] = pre
	}
	for _, s := range sections {
		f := "body." + strings.ReplaceAll(strings.ToLower(s.Name), " ", "_")
		if s.Name != "Explanations" {
			out[f] = strings.TrimSpace(s.Text)
			continue
		}
		text := strings.Split(s.Text, "\n")
		at, cur := 0, f
		flush := func(end int) {
			if t := strings.TrimSpace(strings.Join(text[at:end], "\n")); t != "" || cur != f {
				out[cur] += t
			}
		}
		for _, h := range s.Headings {
			m := explanationRe.FindStringSubmatch(h.Name)
			if h.Level != 3 || m == nil {
				continue
			}
			end := h.Line - s.Line - 1
			flush(end)
			at, cur = end, f+"."+m[1]
		}
		flush(len(text))
	}
	return out
}

// Changes returns a line for each field that differs between the node before and the node after, in the order of the fields. The lines take the agent, the person, the run, the commit and the time from who. A field that is only in after is Add, a field that is only in before is Remove, and another field that differs is Change. A node that is new, with before nil, gives one line on [Whole] with Add. A node that is gone, with after nil, gives one line on [Whole] with Remove.
func Changes(before, after *node.Node, who Line) []Line {
	line := func(field, op string) Line {
		l := who
		l.Field, l.Op = field, op
		return l
	}
	switch {
	case before == nil && after == nil:
		return nil
	case before == nil:
		return []Line{line(Whole, Add)}
	case after == nil:
		return []Line{line(Whole, Remove)}
	}
	a, b := Fields(before), Fields(after)
	var out []Line
	keys := slices.Collect(maps.Keys(a))
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	for _, f := range keys {
		x, inA := a[f]
		y, inB := b[f]
		switch {
		case !inA:
			out = append(out, line(f, Add))
		case !inB:
			out = append(out, line(f, Remove))
		case x != y:
			out = append(out, line(f, Change))
		}
	}
	return out
}
