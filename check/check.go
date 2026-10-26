// Package check runs the fast rules of m2 check on node files (spec 15.3).
//
// The rules are the schema rules S01 to S04, the body rules B01 to B03 and the graph rule G01. They need no Lean and no network. A Checker checks a set of files. The links in the files resolve against the files and against an Index of the other nodes in the tree. The Report gives each finding with its file, line and column, and one of five states for each rule (spec 15.2).
//
// Some rules report only the first problem, because one problem often causes others. B01 reports the first problem in each math span, and B03 reports the first problem in each body. B01 knows a static list of the commands that both KaTeX and MathJax render, and the macros of macros/m2.katex.json. B02 does not allow URLs in the sections Statement, Definition and Problem. G01 checks each value in an edge field that is a tag or a provisional id. The schema rules report the other values.
package check

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/schema"
	"github.com/tamnd/math2-agent/tag"
)

// MaxSize is the largest size of a node file in bytes (rule S04).
const MaxSize = 64 << 10

// State is the state of a rule after a run (spec 15.2).
type State string

// The five states.
const (
	Pass  State = "pass"  // the rule checked something and found nothing
	Fail  State = "fail"  // a hard rule found something
	Found State = "found" // a soft rule found something
	NA    State = "n/a"   // the rule had nothing to check
	Skip  State = "skip"  // the rule did not run
)

// Rule is one rule of m2 check.
type Rule struct {
	ID   string
	Hard bool // a hard rule blocks a merge, and a soft rule only reports
	Text string
}

var rules = []Rule{
	{"S01", true, "every node file validates against its schema version"},
	{"S02", true, "the fields that the kind needs are present"},
	{"S03", true, "no field states a status"},
	{"S04", true, "the node file is at most 64 KB"},
	{"B01", true, "the math renders in KaTeX and MathJax and uses only declared macros"},
	{"B02", true, "each link targets a live tag or a retired tag with a successor, and the statement sections have no URLs"},
	{"B03", true, "the body has only the sections of its kind, in order"},
	{"G01", true, "each edge target resolves"},
}

// Rules returns the rules that a Checker runs, in report order.
func Rules() []Rule { return slices.Clone(rules) }

// Finding is one problem in a node file.
type Finding struct {
	Rule string `json:"rule"`
	Path string `json:"path"`
	Line int    `json:"line"`
	Col  int    `json:"col"`
	Msg  string `json:"msg"`
}

// String returns the finding in the form path:line:col: RULE: message.
func (f Finding) String() string {
	return fmt.Sprintf("%s:%d:%d: %s: %s", f.Path, f.Line, f.Col, f.Rule, f.Msg)
}

// Result is the outcome of one rule.
type Result struct {
	Rule     string `json:"rule"`
	State    State  `json:"state"`
	Checked  int    `json:"checked"` // the number of things that the rule looked at
	Findings int    `json:"findings"`
	Reason   string `json:"reason,omitempty"` // why the rule has the state skip
}

// Report is the outcome of a run.
type Report struct {
	Files    int       `json:"files"`
	Results  []Result  `json:"results"`  // one for each rule, in the order of Rules
	Findings []Finding `json:"findings"` // in path, line and column order
}

// Failed reports whether a hard rule failed.
func (r *Report) Failed() bool {
	return slices.ContainsFunc(r.Results, func(res Result) bool { return res.State == Fail })
}

// WriteSummary writes the table of rule states, as in spec 15.2.
func (r *Report) WriteSummary(w io.Writer) error {
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "rule\tstate\tchecked\tfindings")
	failed := 0
	for _, res := range r.Results {
		n := ""
		switch {
		case res.State == Skip:
			n = res.Reason
		case res.Findings > 0:
			n = fmt.Sprint(res.Findings)
		}
		if res.State == Fail {
			failed++
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", res.Rule, res.State, res.Checked, n)
	}
	_ = tw.Flush()
	end := "nothing failed"
	if failed > 0 {
		end = count(failed, "rule") + " failed"
	}
	fmt.Fprintf(&b, "%s, %s, %s\n", count(r.Files, "file"), count(len(r.Findings), "finding"), end)
	lines := strings.SplitAfter(b.String(), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \n") + strings.Repeat("\n", strings.Count(l, "\n"))
	}
	_, err := io.WriteString(w, strings.Join(lines, ""))
	return err
}

// count returns n and the word, with an s when n is not 1.
func count(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// File is one node file to check.
type File struct {
	Path string // the path from the tree root, for example nodes/4F2/4F2M9A.md
	Data []byte
}

// Entry is what a link needs to know about a node.
type Entry struct {
	Retired    bool
	Successors []string
}

// Index maps each tag and provisional id in a tree to its entry.
type Index map[string]Entry

// Add adds the node n to the index.
func (ix Index) Add(n *node.Node) {
	e := Entry{}
	if r, ok := n.Meta["retired"].(map[string]any); ok {
		e.Retired = true
		if list, ok := r["successor"].([]any); ok {
			for _, s := range list {
				if s, ok := s.(string); ok {
					e.Successors = append(e.Successors, s)
				}
			}
		}
	}
	ix[n.ID] = e
}

// Checker runs the rules on node files. The zero value uses the embedded schema and the embedded macros.
type Checker struct {
	Schema *schema.Schema
	Macros Macros
	Only   []string // the rules to run. Empty means all the rules.
}

// Check checks files. A link or an edge resolves when its target is one of files or is in ix. Check does not change ix.
func (c *Checker) Check(files []File, ix Index) *Report {
	r := &run{
		c:       c,
		s:       cmp.Or(c.Schema, schema.Embedded()),
		macros:  c.Macros,
		ix:      ix,
		local:   Index{},
		checked: map[string]int{},
	}
	if r.macros == nil {
		r.macros = EmbeddedMacros()
	}
	var nodes []*node.Node
	for _, f := range files {
		r.checked["S01"]++
		r.checked["S04"]++
		if len(f.Data) > MaxSize {
			r.add("S04", f.Path, 1, 1, fmt.Sprintf("the file has %d bytes, and the limit is %d", len(f.Data), MaxSize))
		}
		n, err := node.Parse(f.Path, f.Data)
		if err != nil {
			var pe *node.Error
			if errors.As(err, &pe) {
				r.add("S01", pe.Path, pe.Line, pe.Col, pe.Msg)
			} else {
				r.add("S01", f.Path, 0, 0, err.Error())
			}
			continue
		}
		nodes = append(nodes, n)
		r.local.Add(n)
	}
	for _, n := range nodes {
		r.node(n)
	}
	rep := &Report{Files: len(files), Findings: r.out}
	slices.SortFunc(rep.Findings, func(a, b Finding) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Col, b.Col), strings.Compare(a.Rule, b.Rule))
	})
	count := map[string]int{}
	for _, f := range rep.Findings {
		count[f.Rule]++
	}
	for _, rule := range rules {
		res := Result{Rule: rule.ID, Checked: r.checked[rule.ID], Findings: count[rule.ID]}
		switch {
		case len(c.Only) > 0 && !slices.Contains(c.Only, rule.ID):
			res.State, res.Reason, res.Checked = Skip, "not selected", 0
		case res.Findings > 0 && rule.Hard:
			res.State = Fail
		case res.Findings > 0:
			res.State = Found
		case res.Checked == 0:
			res.State = NA
		default:
			res.State = Pass
		}
		rep.Results = append(rep.Results, res)
	}
	return rep
}

// run holds the state of one call of Check.
type run struct {
	c       *Checker
	s       *schema.Schema
	macros  Macros
	ix      Index
	local   Index // the nodes in the files of this run
	checked map[string]int
	out     []Finding
}

func (r *run) on(rule string) bool {
	return len(r.c.Only) == 0 || slices.Contains(r.c.Only, rule)
}

func (r *run) add(rule, path string, line, col int, msg string) {
	if r.on(rule) {
		r.out = append(r.out, Finding{Rule: rule, Path: path, Line: line, Col: col, Msg: msg})
	}
}

// lookup returns the entry of the node id.
func (r *run) lookup(id string) (Entry, bool) {
	if e, ok := r.local[id]; ok {
		return e, true
	}
	e, ok := r.ix[id]
	return e, ok
}

// node runs every rule after the parse on n.
func (r *run) node(n *node.Node) {
	r.checked["S02"]++
	r.checked["S03"]++
	for _, f := range r.s.Validate(n) {
		r.add(f.Rule, f.Path, f.Line, f.Col, f.Msg)
	}
	if t := n.Get("tag"); t != n.ID && (tag.IsTag(t) || tag.IsProvisional(t)) {
		line, col := keyPlace(n.Front, "tag")
		r.add("S01", n.Path, line, col, fmt.Sprintf("the tag field is %s, but the file name gives %s", t, n.ID))
	}
	b := parseBody(n)
	if sec, ok := r.s.Sections(n.Kind()); ok {
		r.checked["B03"]++
		if f, bad := b.sections(n.Kind(), sec, r.s); bad {
			r.add("B03", n.Path, f.Line, f.Col, f.Msg)
		}
	}
	r.links(n, b)
	r.math(n, b)
	r.edges(n)
}

// keyPlace returns the line and the column of the top-level key in the front matter.
func keyPlace(front *yaml.Node, key string) (line, col int) {
	m := front
	if m.Kind == yaml.DocumentNode && len(m.Content) > 0 {
		m = m.Content[0]
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i].Line, m.Content[i].Column
		}
	}
	return m.Line, m.Column
}
