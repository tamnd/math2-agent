package battery

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
)

// structural are the constants of logic and of notation. The constant audit does not need a node for them. The meaning of a notation comes from its instance, and the junk patterns check the instances that are traps.
var structural = map[string]bool{}

func init() {
	for _, c := range strings.Fields(`
		Eq Ne Not And Or Iff True False Exists ExistsUnique HEq ite dite Decidable Decidable.decide Nonempty Subtype Subtype.val
		Bool Bool.true Bool.false Prod Prod.mk Prod.fst Prod.snd Function.comp id setOf
		OfNat.ofNat OfScientific.ofScientific Zero.zero One.one
		HAdd.hAdd HSub.hSub HMul.hMul HDiv.hDiv HMod.hMod HPow.hPow HSMul.hSMul HVAdd.hVAdd Neg.neg Inv.inv
		LE.le LT.lt GE.ge GT.gt Max.max Min.min Dvd.dvd
		Membership.mem HasSubset.Subset HasSSubset.SSubset Union.union Inter.inter SDiff.sdiff HasCompl.compl
		Singleton.singleton Insert.insert EmptyCollection.emptyCollection`) {
		structural[c] = true
	}
}

// instanceLike reports whether the constant c is an instance or another constant that the elaborator adds: the last component of the name starts with "inst", or the name has an auxiliary component.
func instanceLike(c string) bool {
	last := c[strings.LastIndex(c, ".")+1:]
	return strings.HasPrefix(last, "inst") || strings.Contains(c, "._")
}

// junk are the patterns of the pp.all statement that use a junk value of Mathlib (spec 10.5, M-junk). Each pattern allows the universe levels after the name.
var junk = []struct {
	re   *regexp.Regexp
	text string
}{
	{pattern(`@HSub\.hSub%s\s+Nat\b`), "natural subtraction, where m - n is 0 when n > m"},
	{pattern(`@HDiv\.hDiv%s\s+(Nat|Int)\b`), "integer division, which rounds and gives x / 0 = 0"},
	{pattern(`@HDiv\.hDiv%s\s+(Real|Rat|Complex|NNReal|ENNReal)\b`), "division in a field, where x / 0 = 0"},
	{pattern(`@Inv\.inv%s\s+(Real|Rat|Complex|NNReal)\b`), "the inverse in a field, where 0⁻¹ = 0"},
	{pattern(`\bReal\.sqrt\b`), "Real.sqrt, which gives 0 for a negative number"},
	{pattern(`\bReal\.log\b`), "Real.log, where log 0 = 0 and log x = log |x|"},
	{pattern(`@(SupSet\.sSup|InfSet\.sInf)%s\s+Real\b`), "sSup or sInf on the reals, which gives 0 for a set that is empty or not bounded"},
	{pattern(`\bNat\.pred\b`), "Nat.pred, where pred 0 = 0"},
	{pattern(`\bInt\.toNat\b`), "Int.toNat, which gives 0 for a negative integer"},
}

func pattern(p string) *regexp.Regexp {
	return regexp.MustCompile(strings.ReplaceAll(p, "%s", `(\.\{[^}]*\})?`))
}

// closure returns the nodes that the statement of the node with the tag can use: the node, and each node that a chain of uses, about and specializes edges reaches from it, with the definitions of each concept on the way.
func (r *runner) closure(tag string) map[string]bool {
	seen := map[string]bool{tag: true}
	queue := []string{tag}
	visit := func(t string) {
		if !seen[t] {
			seen[t] = true
			queue = append(queue, t)
		}
	}
	for len(queue) > 0 {
		t := queue[0]
		queue = queue[1:]
		for _, typ := range []string{graph.Uses, graph.About, graph.Specializes} {
			for _, e := range r.g.Out(typ, t) {
				visit(e.To)
			}
		}
		for _, e := range r.g.In(graph.Defines, t) {
			visit(e.From)
		}
	}
	return seen
}

// constants is check 3, the constant audit.
func (r *runner) constants(n *node.Node, rec lean.Record) (Result, string, string) {
	closure := r.closure(n.ID)
	var outside, unbound []string
	for _, c := range rec.UsesConstantsInStatement {
		if structural[c] || instanceLike(c) {
			continue
		}
		tags := r.bound[c]
		switch {
		case len(tags) == 0:
			unbound = append(unbound, c)
		case !slices.ContainsFunc(tags, func(t string) bool { return closure[t] }):
			outside = append(outside, fmt.Sprintf("%s (bound to %s)", c, strings.Join(tags, ", ")))
		}
	}
	var found []string
	for _, j := range junk {
		if j.re.MatchString(rec.StatementPPAll) {
			found = append(found, j.text)
		}
	}
	var res Result
	var class string
	var lines []string
	switch {
	case len(outside) > 0:
		res, class = Fail, Def
		lines = append(lines, "These constants are bound to nodes outside the uses closure of the node: "+strings.Join(outside, ", ")+".")
	case len(unbound) > 0:
		res = Open
	case len(found) > 0:
		res = Flag
	default:
		res = Pass
	}
	if len(unbound) > 0 {
		lines = append(lines, "No node binds these constants: "+strings.Join(unbound, ", ")+".")
	}
	if len(found) > 0 {
		if class == "" {
			class = Junk
		}
		lines = append(lines, "The statement uses "+strings.Join(found, "; ")+".")
	}
	return res, class, strings.Join(lines, "\n")
}

// examples is check 7. The statement must not imply the negation of an example of its concepts, and a counterexample of its concepts must not imply the negation of the statement.
func (r *runner) examples(n *node.Node, decl string) (Result, string, string, error) {
	var tags []string
	for _, about := range r.g.Out(graph.About, n.ID) {
		for _, typ := range []string{graph.InstanceOf, graph.About} {
			for _, e := range r.g.In(typ, about.To) {
				if k := r.kind(e.From); e.From != n.ID && (k == "example" || k == "counterexample") {
					tags = append(tags, e.From)
				}
			}
		}
	}
	slices.Sort(tags)
	tags = slices.Compact(tags)
	var pairs [][2]string
	var of []string
	for _, t := range tags {
		d, ok := r.checked(t)
		if !ok {
			continue
		}
		if r.kind(t) == "example" {
			pairs = append(pairs, [2]string{decl, d})
		} else {
			pairs = append(pairs, [2]string{d, decl})
		}
		of = append(of, t)
	}
	if len(pairs) == 0 {
		return Skip, "", "No example or counterexample of the concepts of the node has a Lean binding.", nil
	}
	if r.opt.Prover == nil {
		return NotRun, "", "The check needs a prover.", nil
	}
	got, err := r.opt.Prover.Implies(r.opt.Heartbeats, pairs...)
	if err != nil {
		return "", "", "", err
	}
	var fails, open, ok []string
	class := ""
	for i, imp := range got {
		if i >= len(of) || imp.Error != "" {
			continue
		}
		t := of[i]
		switch {
		case r.kind(t) == "example" && imp.Contradicts != "":
			fails = append(fails, fmt.Sprintf("the statement implies the negation of the example %s (%s, with %s)", t, imp.To, imp.Contradicts))
			class = cmp.Or(class, Strong)
		case r.kind(t) == "counterexample" && imp.Contradicts != "":
			fails = append(fails, fmt.Sprintf("the counterexample %s (%s) implies the negation of the statement, with %s", t, imp.From, imp.Contradicts))
			class = cmp.Or(class, HypMissing)
		case r.kind(t) == "example" && imp.Implies == "":
			open = append(open, t)
		default:
			ok = append(ok, t)
		}
	}
	switch {
	case len(fails) > 0:
		return Fail, class, sentence(strings.Join(fails, "; ")), nil
	case len(open) > 0:
		return Open, "", "The prover cannot derive the examples " + strings.Join(open, ", ") + " from the statement.", nil
	case len(ok) == 0:
		return Skip, "", "The prover cannot compare the statement with its examples and counterexamples.", nil
	}
	return Pass, "", "The statement agrees with " + strings.Join(ok, ", ") + ".", nil
}

// neighbours is check 8. For each generalizes, specializes, implies or equivalent edge between the node and another bound node, the prover must derive the conclusion from the premise.
func (r *runner) neighbours(n *node.Node, decl string) (Result, string, error) {
	type pair struct{ from, to, tag string }
	var ps []pair
	add := func(from, to, tag string) {
		if !slices.Contains(ps, pair{from, to, tag}) {
			ps = append(ps, pair{from, to, tag})
		}
	}
	for _, typ := range []string{graph.Generalizes, graph.Implies, graph.Equivalent} {
		for _, e := range r.g.Out(typ, n.ID) {
			if d, ok := r.checked(e.To); ok && d != decl {
				add(decl, d, e.To)
				if typ == graph.Equivalent {
					add(d, decl, e.To)
				}
			}
		}
		for _, e := range r.g.In(typ, n.ID) {
			if d, ok := r.checked(e.From); ok && d != decl {
				add(d, decl, e.From)
				if typ == graph.Equivalent {
					add(decl, d, e.From)
				}
			}
		}
	}
	// A specializes B means that A is a special case of B, thus B implies A.
	for _, e := range r.g.Out(graph.Specializes, n.ID) {
		if d, ok := r.checked(e.To); ok && d != decl {
			add(d, decl, e.To)
		}
	}
	for _, e := range r.g.In(graph.Specializes, n.ID) {
		if d, ok := r.checked(e.From); ok && d != decl {
			add(decl, d, e.From)
		}
	}
	if len(ps) == 0 {
		return Skip, "The node has no generalizes, specializes, implies or equivalent edge to another node with a Lean binding.", nil
	}
	if r.opt.Prover == nil {
		return NotRun, "The check needs a prover.", nil
	}
	pairs := make([][2]string, len(ps))
	for i, p := range ps {
		pairs[i] = [2]string{p.from, p.to}
	}
	got, err := r.opt.Prover.Implies(r.opt.Heartbeats, pairs...)
	if err != nil {
		return "", "", err
	}
	var fails, open []string
	for i, imp := range got {
		if i >= len(ps) {
			break
		}
		switch {
		case imp.Error != "":
			open = append(open, fmt.Sprintf("%s to %s (%s)", imp.From, imp.To, imp.Error))
		case imp.Contradicts != "":
			fails = append(fails, fmt.Sprintf("%s implies the negation of %s, with %s", imp.From, imp.To, imp.Contradicts))
		case imp.Implies == "":
			open = append(open, imp.From+" to "+imp.To)
		}
	}
	switch {
	case len(fails) > 0:
		return Fail, sentence(strings.Join(fails, "; ")), nil
	case len(open) > 0:
		return Open, "The prover cannot derive " + strings.Join(open, ", ") + ". When the node is the premise, the formal statement can be a special case (M-weak).", nil
	}
	return Pass, "", nil
}

func (r *runner) kind(tag string) string {
	if n, ok := r.byTag[tag]; ok {
		return n.Kind()
	}
	return ""
}

// sentence makes s a sentence: a capital letter at the start and a full stop at the end.
func sentence(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:] + "."
}
