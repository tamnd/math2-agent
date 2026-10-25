package site

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/prov"
)

// quarantineNotice replaces the text of a body field in quarantine, on the page and in the copies for agents.
const quarantineNotice = "*This text is in quarantine. An agent wrote it with a model or a prompt that is under review, and no person checked it since.*"

// provView is the provenance of a node on its page (spec 04.8 and 06.5).
type provView struct {
	History     []historyLine
	Unchecked   []string // the fields that an agent wrote and that no person checked, as labels
	Quarantined []string // the fields in quarantine, as labels
	LogURL      string
	unchecked   []string // the same fields as Unchecked, by name
	quarantined []string // the same fields as Quarantined, by name
}

type historyLine struct {
	Day, Op, Field string
	By             link
}

// loadProv reads the provenance logs of the tree. For each node with a log, it sets the provenance of the page, and it hides each field in quarantine. It runs before the pages resolve their links, so that a field in quarantine gives no link.
func (b *builder) loadProv() error {
	logs, err := prov.ReadAll(b.opt.Content)
	if err != nil {
		return fmt.Errorf("site: %w", err)
	}
	for _, v := range b.order {
		log := logs[v.Tag]
		if len(log) == 0 {
			continue
		}
		p := &provView{LogURL: "https://github.com/" + b.opt.Repo + "/blob/main/" + prov.Path(v.Tag)}
		for _, l := range log {
			p.History = append(p.History, historyLine{Day: day(l.At), Op: l.Op, Field: fieldLabel(l.Field), By: byLink(l)})
		}
		q := prov.Quarantined(log)
		p.quarantined = slices.Sorted(maps.Keys(q))
		for f := range prov.Unconfirmed(log) {
			if _, ok := q[f]; !ok {
				p.unchecked = append(p.unchecked, f)
			}
		}
		slices.Sort(p.unchecked)
		for _, f := range p.quarantined {
			p.Quarantined = append(p.Quarantined, fieldLabel(f))
		}
		for _, f := range p.unchecked {
			p.Unchecked = append(p.Unchecked, fieldLabel(f))
		}
		v.Prov = p
		if len(q) > 0 {
			b.hide(v, p.quarantined)
		}
	}
	return nil
}

// hide removes the fields in quarantine from the node of the page. A body field becomes the quarantine notice. A front matter key goes when a field in quarantine is the key, a part of it or the whole node, but the tag, the kind and the name stay. The node of the page is a copy, so that the graph keeps the file as it is.
func (b *builder) hide(v *nodeView, fields []string) {
	body := func(f string) bool {
		return slices.ContainsFunc(fields, func(q string) bool { return prov.Under(f, q) })
	}
	drop := func(k string) bool {
		return k != "tag" && k != "kind" && k != "name" && slices.ContainsFunc(fields, func(q string) bool { return prov.Covers(k, q) })
	}
	n := *v.Node
	n.Body = prov.Hide(v.Node, body, quarantineNotice)
	n.Meta = maps.Clone(v.Meta)
	maps.DeleteFunc(n.Meta, func(k string, _ any) bool { return drop(k) })
	front := ""
	if len(n.Meta) < len(v.Meta) && v.Front != nil && len(v.Front.Content) == 1 {
		m := *v.Front.Content[0]
		m.Content = nil
		for i := 0; i+1 < len(v.Front.Content[0].Content); i += 2 {
			if kv := v.Front.Content[0].Content[i : i+2]; !drop(kv[0].Value) {
				m.Content = append(m.Content, kv...)
			}
		}
		if data, err := yaml.Marshal(&m); err == nil {
			front = "---\n" + string(data) + "---\n"
		}
	}
	old := v.Body
	v.source = func(src []byte) []byte {
		s := string(src)
		if front == "" && strings.HasSuffix(s, old) {
			front = s[:len(s)-len(old)]
		}
		return []byte(front + n.Body)
	}
	v.Node = &n
}

// fieldLabel names a field for a reader.
func fieldLabel(f string) string {
	switch {
	case f == prov.Whole:
		return "the whole node"
	case f == "body":
		return "the opening text"
	case strings.HasPrefix(f, "body.explanations."):
		return "the explanation " + strings.TrimPrefix(f, "body.explanations.")
	case strings.HasPrefix(f, "body."):
		name := strings.ReplaceAll(strings.TrimPrefix(f, "body."), "_", " ")
		return "the section " + strings.ToUpper(name[:1]) + name[1:]
	}
	return "the field " + f
}

// byLink names the person or the agent of a line, with a link to the person.
func byLink(l prov.Line) link {
	switch {
	case strings.HasPrefix(l.Human, "github:"):
		login := strings.TrimPrefix(l.Human, "github:")
		return link{URL: "https://github.com/" + login, Text: login}
	case strings.HasPrefix(l.Human, "orcid:"):
		id := strings.TrimPrefix(l.Human, "orcid:")
		return link{URL: "https://orcid.org/" + id, Text: "ORCID " + id}
	case l.Human != "":
		return link{Text: l.Human}
	}
	return link{Text: l.Agent, Note: "agent run " + l.Run}
}

// day returns the date of a time in RFC 3339.
func day(at string) string {
	d, _, _ := strings.Cut(at, "T")
	return d
}
