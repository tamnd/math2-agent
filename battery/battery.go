// Package battery runs the F1 battery of spec 10.4 on the Lean bindings of the nodes.
//
// [Run] gives a [Report] for each binding. A report has one [Check] for each of the ten checks of the battery. Each check has a [Result], and a check that fails gives the class of the misformalization (spec 10.5). A binding is F1 when each of the checks 1, 3, 4, 6, 7, 8 and 10 passes, flags or has nothing to test, and the checks 2 and 9 ran and raise no concern of high severity. Check 5 never blocks F1.
//
// Most checks read only the extraction records, the replay results and the graph. The checks 7 and 8 compare two formal statements, and they need a [Prover], for example a [*lean.Session]. The checks 2 and 9 need a model, through a [Reader]. When a program is missing, its checks have the result [NotRun], and the binding is not F1.
package battery

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/replay"
)

// Result is the result of one check.
type Result string

// The results of a check.
const (
	Pass   Result = "pass"    // the check ran and found no problem
	Fail   Result = "fail"    // the check found a misformalization
	Flag   Result = "flag"    // the check found something that a reviewer must see. It does not block F1.
	Open   Result = "open"    // the check ran and could not decide
	Skip   Result = "skip"    // the check has nothing to test, for example a statement without examples
	NotRun Result = "not-run" // the check needs a program or a record that the battery does not have
)

// The classes of misformalization of spec 10.5.
const (
	HypMissing = "M-hyp-missing" // a hypothesis of the informal statement is absent
	HypExtra   = "M-hyp-extra"   // a hypothesis is added
	Quant      = "M-quant"       // the order or the scope of the quantifiers is wrong
	Vacuous    = "M-vacuous"     // the hypotheses cannot all be true
	Junk       = "M-junk"        // a junk value makes the statement trivially true or false
	Def        = "M-def"         // a constant means something different from the informal notion
	Type       = "M-type"        // the carrier is wrong
	Conv       = "M-conv"        // a convention does not match
	Weak       = "M-weak"        // the formal statement is a special case
	Strong     = "M-strong"      // the formal statement is stronger, and false
	Context    = "M-context"     // the statement depends on the local context of the proof file
	Tamper     = "M-tamper"      // the proof environment changes something in the statement
)

// Classes returns the classes of misformalization in the order of spec 10.5.
func Classes() []string {
	return []string{HypMissing, HypExtra, Quant, Vacuous, Junk, Def, Type, Conv, Weak, Strong, Context, Tamper}
}

// The names of the checks, in the order of spec 10.4. The number of a check is its position in Names, from 1.
var names = []string{
	"elaborates",
	"back-translation",
	"constant audit",
	"hypotheses satisfiable",
	"conclusion not trivial",
	"negation not provable",
	"examples and counterexamples",
	"neighbour consistency",
	"critic",
	"pinned-statement compile",
}

// Names returns the names of the ten checks, in the order of spec 10.4.
func Names() []string { return slices.Clone(names) }

// blocking are the numbers of the checks that must pass, flag or skip for F1.
var blocking = []int{1, 3, 4, 6, 7, 8, 10}

// Check is the result of one check on one binding.
type Check struct {
	Number int    `json:"number"` // 1 to 10, as in spec 10.4
	Name   string `json:"name"`
	Result Result `json:"result"`
	Class  string `json:"class,omitempty"`  // the class of the misformalization, for a check that fails or flags
	Detail string `json:"detail,omitempty"` // what the check found, in one or more sentences
}

// Report is the result of the battery on one binding.
type Report struct {
	Tag    string  `json:"tag"`
	Decl   string  `json:"decl"` // the declaration that the kernel checks: the adapter of the binding, or the declaration
	F1     bool    `json:"f1"`
	Checks []Check `json:"checks"` // the ten checks in order
}

// Check returns the check with the number n, from 1 to 10.
func (r Report) Check(n int) Check { return r.Checks[n-1] }

// The severities of a concern.
const (
	High   = "high"
	Medium = "medium"
	Low    = "low"
)

// Concern is one concern of a model about a binding.
type Concern struct {
	Severity string `json:"severity"`        // High, Medium or Low
	Class    string `json:"class,omitempty"` // the class of the misformalization that the concern suggests
	Text     string `json:"text"`
}

// Packet is what a [Reader] reads about one binding.
type Packet struct {
	Tag       string
	Name      string // the name of the node
	Statement string // the informal statement: the first section of the node that states the mathematics
	Decl      string
	Lean      string  // the statement as Lean prints it
	LeanAll   string  // the statement with pp.all
	Checks    []Check // the checks 1 to 8, for the critic. It is empty for the back-translation.
}

// A Reader is a model that reads a packet and lists its concerns. The back-translation (check 2) translates the Lean statement into English without the informal statement, and a judge compares the two. The critic (check 9) reads the packet with the results of the checks 1 to 8.
type Reader interface {
	Read(ctx context.Context, p Packet) ([]Concern, error)
}

// A Prover compares formal statements, as [lean.Session.Implies] does.
type Prover interface {
	Implies(heartbeats int, pairs ...[2]string) ([]lean.Implication, error)
}

// Input is the evidence that the battery reads.
type Input struct {
	Nodes    []*node.Node
	Records  map[string]lean.Record   // the extraction record of each declaration
	Bindings []lean.Binding           // the declarations with the attribute @[m2 "TAG"]
	Replays  map[string]replay.Result // the replay result of each declaration
}

// Options are the programs that some checks need. A nil field means that its checks do not run.
type Options struct {
	Prover          Prover // the checks 7 and 8
	Heartbeats      int    // the limit of each tactic of the Prover, in thousands. Zero means lean.CheckHeartbeats.
	BackTranslation Reader // check 2
	Critic          Reader // check 9
}

// Run runs the battery on each Lean binding of each node that is not retired. The reports are in the order of the tags, then of the declarations.
func Run(ctx context.Context, in Input, opt Options) ([]Report, error) {
	if opt.Heartbeats == 0 {
		opt.Heartbeats = lean.CheckHeartbeats
	}
	var live []*node.Node
	for _, n := range in.Nodes {
		if _, retired := n.Meta["retired"]; !retired {
			live = append(live, n)
		}
	}
	r := &runner{in: in, opt: opt, g: graph.Build(live), byTag: map[string]*node.Node{}, bound: map[string][]string{}}
	for _, n := range live {
		r.byTag[n.ID] = n
		for _, d := range decls(n) {
			r.bound[d.decl] = append(r.bound[d.decl], n.ID)
		}
	}
	for _, b := range in.Bindings {
		r.bound[b.Decl] = append(r.bound[b.Decl], b.Tag)
	}
	var out []Report
	for _, n := range live {
		for _, d := range decls(n) {
			rep, err := r.report(ctx, n, d.checked)
			if err != nil {
				return nil, fmt.Errorf("battery: %s %s: %w", n.ID, d.checked, err)
			}
			out = append(out, rep)
		}
	}
	slices.SortStableFunc(out, func(a, b Report) int { return cmp.Or(strings.Compare(a.Tag, b.Tag), strings.Compare(a.Decl, b.Decl)) })
	return slices.CompactFunc(out, func(a, b Report) bool { return a.Tag == b.Tag && a.Decl == b.Decl }), nil
}

// F1 reports whether the checks give F1.
func F1(checks []Check) bool {
	if len(checks) != len(names) {
		return false
	}
	for _, n := range blocking {
		switch checks[n-1].Result {
		case Pass, Flag, Skip:
		default:
			return false
		}
	}
	for _, n := range []int{2, 9} {
		switch checks[n-1].Result {
		case Pass, Flag:
		default:
			return false
		}
	}
	return true
}

type runner struct {
	in    Input
	opt   Options
	g     *graph.Graph
	byTag map[string]*node.Node
	bound map[string][]string // the tags of the nodes that bind each declaration
}

type binding struct{ decl, checked string }

// decls returns the Lean bindings of n.
func decls(n *node.Node) []binding {
	list, _ := n.Meta["formal"].([]any)
	var out []binding
	for _, item := range list {
		m, _ := item.(map[string]any)
		decl, _ := m["decl"].(string)
		adapter, _ := m["adapter"].(string)
		if m["system"] != "lean4" || decl == "" {
			continue
		}
		out = append(out, binding{decl, cmp.Or(adapter, decl)})
	}
	return out
}

// checked returns the first declaration that the kernel checks for the node with the tag, and whether the node has a binding with a record of a theorem.
func (r *runner) checked(tag string) (string, bool) {
	n, ok := r.byTag[tag]
	if !ok {
		return "", false
	}
	for _, d := range decls(n) {
		if rec, ok := r.in.Records[d.checked]; ok && rec.Error == "" && rec.Kind == "theorem" {
			return d.checked, true
		}
	}
	return "", false
}

func (r *runner) report(ctx context.Context, n *node.Node, decl string) (Report, error) {
	rep := Report{Tag: n.ID, Decl: decl, Checks: make([]Check, len(names))}
	for i := range rep.Checks {
		rep.Checks[i] = Check{Number: i + 1, Name: names[i], Result: NotRun}
	}
	set := func(num int, res Result, class, detail string) {
		c := &rep.Checks[num-1]
		c.Result, c.Class, c.Detail = res, class, detail
	}
	rec, ok := r.in.Records[decl]
	switch {
	case !ok:
		set(1, NotRun, "", "The build has no extraction record of the declaration.")
	case rec.Error != "":
		set(1, Fail, "", "Lean gives no record: "+rec.Error+".")
	default:
		set(1, Pass, "", "")
	}
	if rep.Check(1).Result == Pass {
		res, class, detail := r.constants(n, rec)
		set(3, res, class, detail)
		r.degenerate(rec, set)
		res, class, detail, err := r.examples(n, decl)
		if err != nil {
			return rep, err
		}
		set(7, res, class, detail)
		res, detail, err = r.neighbours(n, decl)
		if err != nil {
			return rep, err
		}
		class = ""
		if res == Fail {
			class = Strong
		}
		set(8, res, class, detail)
	}
	if res, ok := r.in.Replays[decl]; ok {
		if res.Passed {
			set(10, Pass, "", "")
		} else {
			set(10, Fail, Context, "The replay with the pinned statement fails: "+strings.TrimSuffix(res.Reason, ".")+".")
		}
	}
	if rep.Check(1).Result == Pass {
		p := Packet{Tag: n.ID, Name: n.Name(), Statement: statement(n), Decl: decl, Lean: rec.StatementPP, LeanAll: rec.StatementPPAll}
		if r.opt.BackTranslation != nil {
			cs, err := r.opt.BackTranslation.Read(ctx, p)
			if err != nil {
				return rep, err
			}
			res, class, detail := judge(cs)
			set(2, res, class, detail)
		}
		if r.opt.Critic != nil {
			p.Checks = slices.Clone(rep.Checks[:8])
			cs, err := r.opt.Critic.Read(ctx, p)
			if err != nil {
				return rep, err
			}
			res, class, detail := judge(cs)
			set(9, res, class, detail)
		}
	}
	rep.F1 = F1(rep.Checks)
	return rep, nil
}

// statement returns the text of the first section of n that states the mathematics.
func statement(n *node.Node) string {
	for _, sec := range check.Sections(n) {
		if slices.Contains(check.StatementSections(), sec.Name) {
			return strings.TrimSpace(sec.Text)
		}
	}
	return ""
}

// judge turns the concerns of a model into a result. A concern of high severity fails the check, with the class of the first such concern. Another concern flags the check.
func judge(cs []Concern) (Result, string, string) {
	if len(cs) == 0 {
		return Pass, "", ""
	}
	res, class := Flag, ""
	var lines []string
	for _, c := range cs {
		if c.Severity == High && res != Fail {
			res, class = Fail, c.Class
		}
		lines = append(lines, c.Severity+": "+c.Text)
	}
	return res, class, strings.Join(lines, "\n")
}

// degenerate sets the checks 4, 5 and 6 from the degenerate-statement checks of the record.
func (r *runner) degenerate(rec lean.Record, set func(int, Result, string, string)) {
	if rec.Kind != "theorem" {
		for _, num := range []int{4, 5, 6} {
			set(num, Skip, "", "The declaration is a "+rec.Kind+", not a theorem.")
		}
		return
	}
	c := rec.Checks
	if c == nil {
		return
	}
	if c.Vacuous != "" {
		set(4, Fail, Vacuous, fmt.Sprintf("The tactic %s proves False from the hypotheses.", c.Vacuous))
	} else {
		set(4, Pass, "", "No tactic proves False from the hypotheses.")
	}
	if c.Trivial != "" {
		set(5, Flag, Junk, fmt.Sprintf("The tactic %s proves the statement. A reviewer must check that the statement is not true only by a junk value.", c.Trivial))
	} else {
		set(5, Pass, "", "")
	}
	if c.Refuted != "" {
		set(6, Fail, Strong, fmt.Sprintf("The tactic %s proves the negation of the statement.", c.Refuted))
	} else {
		set(6, Pass, "", "No tactic proves the negation.")
	}
}
