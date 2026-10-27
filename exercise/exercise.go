// Package exercise reads the exercise nodes of a math2 tree (corpus K8.7).
//
// Schema node.v2 has the kind exercise. Its sections are Exercise, Hint, Answer and Solution, and its fields are about, difficulty, math_group, check and external. In node.v1, an exercise is an example node whose Remarks section opens with the line of the role, for example "Role: exercise. About: 3C53TD. Difficulty: 2. Group: B. Check: exact.", and has the headings Hint, Answer and Solution (corpus K2.3). [Parse] reads both forms. [Migrate] gives the node.v2 form of a node.v1 exercise, with the same tag, and the migration from node.v1 to node.v2 uses it.
package exercise

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prov"
)

// Kind is the kind of an exercise node in node.v2.
const Kind = "exercise"

// Marker opens the Remarks section of an exercise in node.v1.
const Marker = "Role: exercise."

// Checks are the ways that a machine checks an answer (corpus K8.7).
var Checks = []string{"exact", "numeric", "choice", "cas", "lean", "none"}

// Groups are the groups of the MATH taxonomy: A is facts and routine procedures, B is transfer and application, and C is justification, conjecture and evaluation.
var Groups = []string{"A", "B", "C"}

// Exercise is one exercise, as its node has it.
type Exercise struct {
	ID         string
	About      []string // the nodes that the exercise practises
	Difficulty int      // 1 to 5
	Group      string   // one of Groups
	Check      string   // one of Checks
	External   []External
	Task       string // the Exercise section, or the Example section in node.v1
	Hint       string
	Answer     string
	Solution   string
	Old        bool // the node has the form of node.v1
}

// External is a link to an interactive version of the exercise in another system, for example a WeBWorK OPL path. It is a link only.
type External struct {
	System string
	ID     string
	URL    string
}

// Moves gives, for each body field of a node.v1 exercise, the fields of node.v2 that its text moves to. The line of the role in Remarks gives the fields about, difficulty, math_group and check. The migration copies the author of each one in the provenance log of the node to its new fields.
var Moves = map[string][]string{
	"body.example": {"body.exercise"},
	"body.remarks": {"about", "difficulty", "math_group", "check", "body.hint", "body.answer", "body.solution"},
}

var (
	itemRe    = regexp.MustCompile(`([A-Z][a-z]+): ([^.]*)\.`)
	headingRe = regexp.MustCompile(`(?m)^### (Hint|Answer|Solution)[ \t]*$`)
)

// Parse reads the exercise of the node. It returns nil and no error when the node is not an exercise: a node that is not of the kind exercise, and not an example whose Remarks section opens with [Marker]. It returns an error when a node.v1 exercise does not have the form of the line of the role or of the headings.
func Parse(n *node.Node) (*Exercise, error) {
	fields := prov.Fields(n)
	switch {
	case n.Kind() == Kind:
		e := &Exercise{ID: n.ID, About: n.List("about"), Group: n.Get("math_group"), Check: n.Get("check"), Task: fields["body.exercise"], Hint: fields["body.hint"], Answer: fields["body.answer"], Solution: fields["body.solution"]}
		if d, ok := n.Meta["difficulty"].(int); ok {
			e.Difficulty = d
		}
		ext, _ := n.Meta["external"].([]any)
		for _, x := range ext {
			if m, ok := x.(map[string]any); ok {
				s := func(k string) string { v, _ := m[k].(string); return v }
				e.External = append(e.External, External{System: s("system"), ID: s("id"), URL: s("url")})
			}
		}
		return e, nil
	case n.Kind() != "example" || !strings.HasPrefix(fields["body.remarks"], Marker):
		return nil, nil
	}
	remarks := fields["body.remarks"]
	e := &Exercise{ID: n.ID, Task: fields["body.example"], Old: true}
	first, rest, _ := strings.Cut(remarks, "\n")
	seen := map[string]bool{}
	for _, m := range itemRe.FindAllStringSubmatch(strings.TrimPrefix(first, Marker), -1) {
		key, v := m[1], strings.TrimSpace(m[2])
		if seen[key] {
			return nil, fmt.Errorf("the line of the role of %s has %s two times", n.ID, key)
		}
		seen[key] = true
		switch key {
		case "About":
			for a := range strings.SplitSeq(v, ",") {
				e.About = append(e.About, strings.TrimSpace(a))
			}
		case "Difficulty":
			d, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("the difficulty of %s is %q, and it must be a number from 1 to 5", n.ID, v)
			}
			e.Difficulty = d
		case "Group":
			e.Group = v
		case "Check":
			e.Check = v
		default:
			return nil, fmt.Errorf("the line of the role of %s has %s, and it has only About, Difficulty, Group and Check", n.ID, key)
		}
	}
	for _, key := range []string{"About", "Difficulty", "Group", "Check"} {
		if !seen[key] {
			return nil, fmt.Errorf("the line of the role of %s has no %s. Write it as %q", n.ID, key, Marker+" About: TAG. Difficulty: 2. Group: B. Check: exact.")
		}
	}
	at := headingRe.FindAllStringSubmatchIndex(rest, -1)
	head := rest
	if len(at) > 0 {
		head = rest[:at[0][0]]
	}
	if strings.TrimSpace(head) != "" {
		return nil, fmt.Errorf("the Remarks section of %s has text between the line of the role and the heading Hint, Answer or Solution. Put it under one of them", n.ID)
	}
	for i, m := range at {
		end := len(rest)
		if i+1 < len(at) {
			end = at[i+1][0]
		}
		name, text := rest[m[2]:m[3]], strings.TrimSpace(rest[m[1]:end])
		switch {
		case seen[name]:
			return nil, fmt.Errorf("the Remarks section of %s has the heading %s two times", n.ID, name)
		case name == "Hint":
			e.Hint = text
		case name == "Answer":
			e.Answer = text
		default:
			e.Solution = text
		}
		seen[name] = true
	}
	return e, nil
}

// Migrate returns the node.v2 form of a node.v1 exercise: the kind exercise, the fields about, difficulty, math_group and check from the line of the role, and the sections Exercise, Hint, Answer and Solution. The tag, the other fields and the path do not change. Migrate returns nil and no error for a node that is not a node.v1 exercise. It returns an error when the exercise does not parse, or when the node has a section other than Example and Remarks, because an exercise has no place for it.
func Migrate(n *node.Node) (*node.Node, error) {
	e, err := Parse(n)
	if err != nil || e == nil || !e.Old {
		return nil, err
	}
	for f := range prov.Fields(n) {
		if strings.HasPrefix(f, "body") && Moves[f] == nil {
			return nil, fmt.Errorf("%s has the body field %s, and an exercise has no place for it", n.ID, f)
		}
	}
	data, err := yaml.Marshal(n.Front)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	m := doc.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == "kind" {
			m.Content[i+1].Value = Kind
		}
	}
	about := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, a := range e.About {
		about.Content = append(about.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: a})
	}
	for _, kv := range []struct {
		key string
		val *yaml.Node
	}{
		{"about", about},
		{"difficulty", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(e.Difficulty)}},
		{"math_group", &yaml.Node{Kind: yaml.ScalarNode, Value: e.Group}},
		{"check", &yaml.Node{Kind: yaml.ScalarNode, Value: e.Check}},
	} {
		m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: kv.key}, kv.val)
	}
	front, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.Write(front)
	b.WriteString("---\n")
	for _, s := range [][2]string{{"Exercise", e.Task}, {"Hint", e.Hint}, {"Answer", e.Answer}, {"Solution", e.Solution}} {
		if s[1] != "" || s[0] == "Exercise" {
			fmt.Fprintf(&b, "\n## %s\n\n%s\n", s[0], s[1])
		}
	}
	return node.Parse(n.Path, []byte(b.String()))
}
