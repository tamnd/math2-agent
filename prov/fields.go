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

// span is the text of one body field: the lines from to to of the body, with to not included. When head is true, the first line is the heading of an explanation.
type span struct {
	field    string
	from, to int
	head     bool
}

// spans returns the body fields of the node in the order of the body. The opening text of the Explanations section has a span only when it has text.
func spans(n *node.Node) []span {
	lines := strings.Split(n.Body, "\n")
	sections := check.Sections(n)
	var out []span
	first := len(lines)
	if len(sections) > 0 {
		first = min(len(lines), max(0, sections[0].Line-n.BodyLine))
	}
	if strings.TrimSpace(strings.Join(lines[:first], "\n")) != "" {
		out = append(out, span{field: "body", to: first})
	}
	for _, s := range sections {
		f := "body." + strings.ReplaceAll(strings.ToLower(s.Name), " ", "_")
		start := s.Line - n.BodyLine + 1
		end := start + strings.Count(s.Text, "\n") + 1
		if s.Text == "" {
			end = start
		}
		if s.Name != "Explanations" {
			out = append(out, span{field: f, from: start, to: end})
			continue
		}
		at, cur := start, f
		flush := func(to int) {
			if cur != f || strings.TrimSpace(strings.Join(lines[at:to], "\n")) != "" {
				out = append(out, span{field: cur, from: at, to: to, head: cur != f})
			}
		}
		for _, h := range s.Headings {
			m := explanationRe.FindStringSubmatch(h.Name)
			if h.Level != 3 || m == nil {
				continue
			}
			flush(h.Line - n.BodyLine)
			at, cur = h.Line-n.BodyLine, f+"."+m[1]
		}
		flush(end)
	}
	return out
}

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
	for _, s := range spans(n) {
		out[s.field] += strings.TrimSpace(strings.Join(lines[s.from:s.to], "\n"))
	}
	return out
}

// Hide returns the body of the node with the text of each body field that hidden reports replaced by the paragraph notice. The headings stay, so that a reader sees that the text exists. hidden gets the name of each body field, as [Fields] gives it.
func Hide(n *node.Node, hidden func(field string) bool, notice string) string {
	lines := strings.Split(n.Body, "\n")
	var out []string
	at := 0
	for _, s := range spans(n) {
		if !hidden(s.field) {
			continue
		}
		from := s.from
		if s.head {
			from++
		}
		out = append(out, lines[at:from]...)
		out = append(out, "", notice)
		if at = s.to; at < len(lines) && lines[at] != "" {
			out = append(out, "")
		}
	}
	if at == 0 {
		return n.Body
	}
	return strings.Join(append(out, lines[at:]...), "\n")
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
