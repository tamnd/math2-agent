// Package audit runs the audit rules of spec 15 on a math2 working tree.
//
// The audit is the contract of the content. Each rule has an id, a plane and a hardness. A hard rule that finds a problem fails, and a failed rule blocks a merge or a release. A soft rule that finds a problem reports it and does not block. Each rule in the report has one of the five states of spec 15.2. A rule that had nothing to look at has the state n/a, and never the state pass.
//
// The catalogue has the 65 rules of spec 15.3. A rule that is not written yet has the state skip with the reason "not written yet". The letter of a rule gives its plane:
//
//	T, S, B  nodes
//	G        graph
//	F, V     formal
//	C        compute
//	A        align
//	L        licence
//	P        prov
//	R        release
//	X        agents
//
// A full audit reads the whole working tree. An audit with a base revision is the audit of a pull request. It reads only the node shards and the other content files that changed since the merge base, and it lets the pull request use the provisional ids of its own new nodes (rule T03). The rules on the registers and on the git history always read the whole tree.
package audit

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/schema"
)

// The planes of the audit, in report order.
const (
	Nodes   = "nodes"
	Graph   = "graph"
	Formal  = "formal"
	Compute = "compute"
	Align   = "align"
	Licence = "licence"
	Prov    = "prov"
	Release = "release"
	Agents  = "agents"
)

// Planes returns the planes in report order.
func Planes() []string {
	return []string{Nodes, Graph, Formal, Compute, Align, Licence, Prov, Release, Agents}
}

// Rule is one rule of the catalogue.
type Rule struct {
	ID    string
	Plane string
	Hard  bool // a hard rule blocks a merge or a release, and a soft rule only reports
	Text  string
}

// Rules returns the catalogue in report order.
func Rules() []Rule {
	out := make([]Rule, len(catalogue))
	for i, e := range catalogue {
		out[i] = e.Rule
	}
	return out
}

// Options selects the rules and the scope of a run.
type Options struct {
	Plane string   // run only the rules of this plane. Empty means all the planes.
	Rules []string // run only these rules. Empty means all the rules of the plane.

	// Base is the git revision that a pull request starts from, for example origin/main. When Base is not empty, the audit reads only the changes since the merge base of Base and HEAD, the changes in the working tree and the untracked files.
	Base string
}

// Run audits the math2 working tree at root. The report has one result for each selected rule, in catalogue order. An error means that the audit could not run, for example because a register does not exist or Base is not a revision. A problem in the content is a finding and not an error.
func Run(root string, opt Options) (*check.Report, error) {
	selected, err := selectRules(opt)
	if err != nil {
		return nil, err
	}
	a, err := load(root, opt)
	if err != nil {
		return nil, err
	}
	rep := &check.Report{Files: len(a.scope)}
	for _, e := range selected {
		res := check.Result{Rule: e.ID}
		var o outcome
		if e.run == nil {
			o.reason = "not written yet"
		} else {
			o = e.run(a)
		}
		res.Checked, res.Findings = o.checked, len(o.findings)
		switch {
		case o.reason != "":
			res.State, res.Reason, res.Checked = check.Skip, o.reason, 0
		case res.Findings > 0 && e.Hard:
			res.State = check.Fail
		case res.Findings > 0:
			res.State = check.Found
		case res.Checked == 0:
			res.State = check.NA
		default:
			res.State = check.Pass
		}
		for _, f := range o.findings {
			f.Rule = e.ID
			rep.Findings = append(rep.Findings, f)
		}
		rep.Results = append(rep.Results, res)
	}
	slices.SortFunc(rep.Findings, func(a, b check.Finding) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Col, b.Col), strings.Compare(a.Rule, b.Rule))
	})
	return rep, nil
}

// selectRules returns the entries of the catalogue that opt selects.
func selectRules(opt Options) ([]entry, error) {
	if opt.Plane != "" && !slices.Contains(Planes(), opt.Plane) {
		return nil, fmt.Errorf("audit: %q is not a plane. The planes are %s", opt.Plane, strings.Join(Planes(), ", "))
	}
	for _, id := range opt.Rules {
		if !slices.ContainsFunc(catalogue, func(e entry) bool { return e.ID == id }) {
			return nil, fmt.Errorf("audit: %q is not a rule of the audit", id)
		}
	}
	var out []entry
	for _, e := range catalogue {
		if (opt.Plane == "" || e.Plane == opt.Plane) && (len(opt.Rules) == 0 || slices.Contains(opt.Rules, e.ID)) {
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("audit: no rule of the plane %s is in the list %s", opt.Plane, strings.Join(opt.Rules, ","))
	}
	return out, nil
}

// outcome is what one rule found.
type outcome struct {
	checked  int
	findings []check.Finding // the Rule field is set by Run
	reason   string          // when not empty, the rule did not run, and the reason says why
}

func (o *outcome) add(path string, line, col int, format string, args ...any) {
	o.findings = append(o.findings, check.Finding{Path: path, Line: line, Col: col, Msg: fmt.Sprintf(format, args...)})
}

// auditor holds the tree that a run reads.
type auditor struct {
	root     string
	pr       bool            // the audit is of a pull request
	touched  map[string]bool // the shards and the files that changed, when pr is true
	schema   *schema.Schema  // the schema of the tree
	macros   check.Macros    // the macros of the tree
	all      []check.File    // every node file in the tree
	scope    []check.File    // the node files in scope
	nodes    []*node.Node    // every node file that parses
	ix       check.Index     // every node file that parses
	checkRun *check.Report   // the run of m2 check on the scope, made when a rule needs it
	graph    *graph.Graph    // the graph of every node file that parses, made when a rule needs it
}

func load(root string, opt Options) (*auditor, error) {
	if _, err := os.Stat(root); err != nil {
		return nil, err
	}
	a := &auditor{root: root, ix: check.Index{}}
	var err error
	if a.schema, err = schema.Load(root); err != nil {
		return nil, err
	}
	if a.macros, err = check.LoadMacros(root); err != nil {
		return nil, err
	}
	if opt.Base != "" {
		a.pr = true
		if a.touched, err = changed(root, opt.Base); err != nil {
			return nil, err
		}
	}
	if a.all, err = check.ReadTree(root); err != nil {
		return nil, err
	}
	for _, f := range a.all {
		if a.inScope(f.Path) {
			a.scope = append(a.scope, f)
		}
		n, err := node.Parse(f.Path, f.Data)
		if err != nil {
			continue // S01 reports it
		}
		a.nodes = append(a.nodes, n)
		a.ix.Add(n)
	}
	return a, nil
}

// inScope reports whether the file at path, relative to the root, is in the scope of the run. In a pull request, a node file is in scope when a file in its shard changed, and another file is in scope when it changed.
func (a *auditor) inScope(path string) bool {
	return !a.pr || a.touched[shard(path)]
}

// shard returns the shard directory of a node file, for example nodes/G5. For another file it returns the path.
func shard(path string) string {
	if dir := filepath.ToSlash(filepath.Dir(path)); strings.HasPrefix(dir, "nodes/") && strings.Count(dir, "/") == 1 {
		return dir
	}
	return path
}

// scopeNodes returns the parsed node files in scope.
func (a *auditor) scopeNodes() []*node.Node {
	var out []*node.Node
	for _, n := range a.nodes {
		if a.inScope(n.Path) {
			out = append(out, n)
		}
	}
	return out
}

// readFile reads a file of the tree. A file that does not exist gives no data and no error.
func (a *auditor) readFile(rel string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(a.root, rel)) // #nosec G304 -- rel is a fixed path in the working tree, which the caller owns.
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

// lineCol returns the line and the column, in runes, of the byte at offset in data.
func lineCol(data []byte, offset int) (line, col int) {
	before := data[:offset]
	line = 1 + strings.Count(string(before), "\n")
	start := strings.LastIndexByte(string(before), '\n') + 1
	return line, len([]rune(string(before[start:]))) + 1
}

// retired reports whether n is retired.
func retired(n *node.Node) bool {
	_, ok := n.Meta["retired"]
	return ok
}

// str returns a string value, or "" for a value that is not a string.
func str(v any) string {
	s, _ := v.(string)
	return s
}
