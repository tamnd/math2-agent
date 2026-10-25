package search

import (
	"hash/fnv"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A tree is a node of the operator tree of a formula (spec 14.2). A leaf has no kids. The op of a variable is the name of the variable, and anonymous names it.
type tree struct {
	op   string
	vari bool // the leaf is a variable
	kids []*tree
}

// The operators whose operands can change places. The canonical form sorts their operands.
var commutative = map[string]bool{"+": true, "*": true, "=": true, `\ne`: true, `\cup`: true, `\cap`: true, `\oplus`: true, `\otimes`: false, `\equiv`: true, `\cong`: true, `\sim`: true, `\approx`: true, `\iff`: true, `\land`: true, `\lor`: true, "gcd": true, "lcm": true}

// The relations, from the weakest to the strongest binding. All of them bind less than + and -.
var relations = map[string]string{
	"=": "=", "<": "<", ">": ">", `\lt`: "<", `\gt`: ">", `\le`: `\le`, `\leq`: `\le`, `\leqslant`: `\le`, `\ge`: `\ge`, `\geq`: `\ge`, `\geqslant`: `\ge`,
	`\ne`: `\ne`, `\neq`: `\ne`, `\mid`: `\mid`, `\nmid`: `\nmid`, `\in`: `\in`, `\notin`: `\notin`, `\ni`: `\ni`,
	`\subset`: `\subset`, `\subseteq`: `\subseteq`, `\supset`: `\supset`, `\supseteq`: `\supseteq`, `\subsetneq`: `\subsetneq`,
	`\equiv`: `\equiv`, `\cong`: `\cong`, `\simeq`: `\simeq`, `\sim`: `\sim`, `\approx`: `\approx`, `\propto`: `\propto`, `\perp`: `\perp`, `\parallel`: `\parallel`,
	`\to`: `\to`, `\rightarrow`: `\to`, `\mapsto`: `\mapsto`, `\Rightarrow`: `\implies`, `\implies`: `\implies`, `\Leftrightarrow`: `\iff`, `\iff`: `\iff`, `\leftrightarrow`: `\iff`,
	`\models`: `\models`, `\vdash`: `\vdash`, `\triangleleft`: `\triangleleft`, `\trianglelefteq`: `\trianglelefteq`, `\land`: `\land`, `\wedge`: `\land`, `\lor`: `\lor`, `\vee`: `\lor`,
}

// The operators of sums, from the weakest to the strongest binding. Juxtaposition is a product.
var (
	sums     = map[string]string{"+": "+", "-": "-", `\pm`: `\pm`, `\mp`: `\mp`, `\cup`: `\cup`, `\cap`: `\cap`, `\oplus`: `\oplus`, `\setminus`: `\setminus`, `\vee`: `\lor`, `\wedge`: `\land`}
	products = map[string]string{"*": "*", `\cdot`: "*", `\times`: `\times`, "/": "/", `\div`: "/", `\circ`: `\circ`, `\otimes`: `\otimes`, `\ast`: "*", `\star`: `\star`, `\bmod`: "mod", `\mod`: "mod"}
)

// Big operators take a subscript, a superscript and the operand after them.
var bigOps = map[string]bool{`\sum`: true, `\prod`: true, `\int`: true, `\iint`: true, `\oint`: true, `\bigcup`: true, `\bigcap`: true, `\bigoplus`: true, `\bigotimes`: true, `\coprod`: true, `\lim`: true, `\limsup`: true, `\liminf`: true, `\sup`: true, `\inf`: true, `\max`: true, `\min`: true, `\bigvee`: true, `\bigwedge`: true}

// Commands that only change the look, and the parser skips.
var skip = map[string]bool{`\left`: true, `\right`: true, `\big`: true, `\Big`: true, `\bigg`: true, `\Bigg`: true, `\bigl`: true, `\bigr`: true, `\Bigl`: true, `\Bigr`: true, `\displaystyle`: true, `\textstyle`: true, `\,`: true, `\;`: true, `\:`: true, `\!`: true, `\ `: true, `\quad`: true, `\qquad`: true, `\limits`: true, `\nolimits`: true, `\middle`: true, "~": true}

// Commands with one argument whose argument names a constant, such as \mathbb{R} and \operatorname{rank}.
var named = map[string]bool{`\mathbb`: true, `\mathrm`: true, `\operatorname`: true, `\mathcal`: true, `\mathfrak`: true, `\mathsf`: true, `\mathscr`: true, `\text`: true, `\textrm`: true, `\mathit`: true, `\mathbf`: true, `\boldsymbol`: true, `\bm`: true}

// The Greek letters that are usually variables. The other Greek letters, such as \pi, \varphi and \zeta, usually name constants or functions.
var greekVars = map[string]bool{`\alpha`: true, `\beta`: true, `\gamma`: true, `\delta`: true, `\epsilon`: true, `\varepsilon`: true, `\eta`: true, `\theta`: true, `\vartheta`: true, `\kappa`: true, `\lambda`: true, `\nu`: true, `\xi`: true, `\rho`: true, `\tau`: true, `\omega`: true}

type token struct {
	s   string
	num bool
}

func lex(src string) []token {
	var out []token
	for i := 0; i < len(src); {
		r, w := utf8.DecodeRuneInString(src[i:])
		switch {
		case unicode.IsSpace(r):
			i += w
		case r == '\\':
			j := i + 1
			for j < len(src) && isLetter(src[j]) {
				j++
			}
			if j == i+1 && j < len(src) {
				_, w2 := utf8.DecodeRuneInString(src[j:])
				j += w2
			}
			out = append(out, token{s: src[i:j]})
			i = j
		case r >= '0' && r <= '9':
			j := i
			for j < len(src) && (src[j] >= '0' && src[j] <= '9' || src[j] == '.' && j+1 < len(src) && src[j+1] >= '0' && src[j+1] <= '9') {
				j++
			}
			out = append(out, token{s: src[i:j], num: true})
			i = j
		default:
			out = append(out, token{s: src[i : i+w]})
			i += w
		}
	}
	return out
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// parser is a precedence parser of LaTeX math. It accepts any input: a token that it does not know is a constant.
type parser struct {
	toks []token
	pos  int
	bars int // the number of open | groups. A | closes a group only when one is open.
}

// parseFormula returns the operator tree of the LaTeX formula, or nil when the formula is empty.
func parseFormula(src string) *tree {
	p := &parser{toks: lex(src)}
	var parts []*tree
	for p.pos < len(p.toks) {
		if t := p.list(); t != nil {
			parts = append(parts, t)
		}
		if p.pos < len(p.toks) {
			p.pos++ // a closing bracket with no opening bracket
		}
	}
	switch len(parts) {
	case 0:
		return nil
	case 1:
		return parts[0]
	}
	return &tree{op: ",", kids: parts}
}

func (p *parser) peek() string {
	for p.pos < len(p.toks) && skip[p.toks[p.pos].s] {
		p.pos++
	}
	if p.pos < len(p.toks) {
		return p.toks[p.pos].s
	}
	return ""
}

func (p *parser) next() token {
	p.peek()
	t := p.toks[p.pos]
	p.pos++
	return t
}

func (p *parser) closing(s string) bool {
	return s == ")" || s == "]" || s == "}" || s == `\}` || s == `\rangle` || s == `\rvert` || s == `\rVert` || (s == "|" || s == `\|`) && p.bars > 0
}

// ends reports whether the token s ends an operand of a product.
func (p *parser) ends(s string) bool {
	switch s {
	case "", ",", "&", `\\`, ";", "^", "_", ":", `\colon`:
		return true
	}
	_, rel := relations[s]
	_, sum := sums[s]
	_, prod := products[s]
	return rel || sum || prod || p.closing(s)
}

// list parses relations separated by commas, & and \\.
func (p *parser) list() *tree {
	var parts []*tree
	for {
		if t := p.colon(); t != nil {
			parts = append(parts, t)
		}
		s := p.peek()
		if s != "," && s != "&" && s != `\\` && s != ";" {
			break
		}
		p.next()
	}
	switch len(parts) {
	case 0:
		return nil
	case 1:
		return parts[0]
	}
	return &tree{op: ",", kids: parts}
}

// colon parses relations separated by a colon, as in a set {x : x > 0} or a map f : A \to B. The colon binds less than the relations.
func (p *parser) colon() *tree {
	left := p.relation()
	for s := p.peek(); s == ":" || s == `\colon`; s = p.peek() {
		p.next()
		left = mk(":", left, p.relation())
	}
	return left
}

func (p *parser) relation() *tree {
	left := p.sum()
	for {
		op, ok := relations[p.peek()]
		if !ok {
			return left
		}
		p.next()
		right := p.sum()
		left = mk(op, left, right)
	}
}

func (p *parser) sum() *tree {
	var left *tree
	if s := p.peek(); s == "-" || s == "+" || s == `\pm` || s == `\neg` || s == `\lnot` {
		p.next()
		op := "neg"
		switch s {
		case "+":
			op = "pos"
		case `\pm`:
			op = `\pm`
		case `\neg`, `\lnot`:
			op = `\neg`
		}
		left = mk(op, p.product())
	} else {
		left = p.product()
	}
	for {
		op, ok := sums[p.peek()]
		if !ok {
			return left
		}
		p.next()
		left = mk(op, left, p.product())
	}
}

func (p *parser) product() *tree {
	left := p.applied()
	for {
		s := p.peek()
		if op, ok := products[s]; ok {
			p.next()
			left = mk(op, left, p.applied())
			continue
		}
		if p.ends(s) {
			return left
		}
		right := p.applied()
		if right == nil {
			return left
		}
		if left != nil && left.vari || left != nil && len(left.kids) == 0 && isFunction(left.op) {
			if right.op == "()" {
				right = &tree{op: "apply", kids: append([]*tree{left}, right.kids...)}
				left = right
				continue
			}
		}
		left = mk("*", left, right)
	}
}

// isFunction reports whether a constant names a function, so that f(x) is an application and not a product.
func isFunction(op string) bool {
	return strings.HasPrefix(op, `\`) && !strings.HasPrefix(op, `\mathbb`) || len(op) > 1
}

// The functions that take their argument without brackets, as in \sin x and \dim V.
var prefixFuncs = map[string]bool{`\sin`: true, `\cos`: true, `\tan`: true, `\cot`: true, `\sec`: true, `\csc`: true, `\sinh`: true, `\cosh`: true, `\tanh`: true, `\arcsin`: true, `\arccos`: true, `\arctan`: true, `\log`: true, `\ln`: true, `\lg`: true, `\exp`: true, `\det`: true, `\dim`: true, `\ker`: true, `\deg`: true, `\arg`: true, `\hom`: true, `\Pr`: true, `\Re`: true, `\Im`: true}

// applied parses an operand of a product. A function that takes its argument without brackets applies to the operand after it, thus \sin x is an application and not a product. A name from \operatorname is such a function.
func (p *parser) applied() *tree {
	start := p.peek()
	t := p.postfix()
	if t == nil || len(t.kids) != 0 || t.vari || !prefixFuncs[t.op] && start != `\operatorname` {
		return t
	}
	if s := p.peek(); s == "(" || p.ends(s) {
		return t
	}
	arg := p.applied()
	if arg == nil {
		return t
	}
	return &tree{op: "apply", kids: []*tree{t, arg}}
}

func (p *parser) postfix() *tree {
	t := p.atom()
	for t != nil {
		switch p.peek() {
		case "^":
			p.next()
			t = mk("^", t, p.argument())
		case "_":
			p.next()
			t = mk("_", t, p.argument())
		case "!":
			p.next()
			t = mk("!", t)
		case "'":
			p.next()
			t = mk("'", t)
		default:
			return t
		}
	}
	return t
}

// argument parses the argument of ^, _ or a command: a group in braces, or one token.
func (p *parser) argument() *tree {
	if p.peek() == "{" {
		p.next()
		t := p.list()
		if p.peek() == "}" {
			p.next()
		}
		return t
	}
	if p.peek() == "" {
		return nil
	}
	tok := p.next()
	return p.leaf(tok)
}

// group parses a group in braces, which a command such as \frac needs.
func (p *parser) group() *tree { return p.argument() }

func (p *parser) leaf(tok token) *tree {
	switch {
	case tok.num:
		return &tree{op: tok.s}
	case len(tok.s) == 1 && isLetter(tok.s[0]) || greekVars[tok.s]:
		return &tree{op: tok.s, vari: true}
	}
	return &tree{op: tok.s}
}

func (p *parser) atom() *tree {
	s := p.peek()
	if s == "" || p.closing(s) {
		return nil
	}
	switch s {
	case ",", "&", `\\`, ";", "^", "_", ":", `\colon`:
		return nil
	}
	if _, ok := relations[s]; ok {
		return nil
	}
	tok := p.next()
	switch s {
	case "(", "[", "{", `\{`, `\langle`, "|", `\|`, `\lvert`, `\lVert`, `\lfloor`, `\lceil`:
		if s == "|" || s == `\|` {
			p.bars++
		}
		inner := p.list()
		if s == "|" || s == `\|` {
			p.bars--
		}
		end := p.peek()
		bar := s == "|" || s == `\|`
		if bar && (end == "|" || end == `\|`) || !bar && (p.closing(end) || end == `\rfloor` || end == `\rceil`) {
			p.next()
		}
		switch s {
		case "(":
			if inner != nil && inner.op == "," {
				return &tree{op: "()", kids: inner.kids}
			}
			if inner == nil {
				return &tree{op: "()"}
			}
			return &tree{op: "()", kids: []*tree{inner}}
		case "{":
			return inner
		case `\{`:
			return mk("set", inner)
		case "|", `\lvert`:
			return mk("abs", inner)
		case `\|`, `\lVert`:
			return mk("norm", inner)
		case `\lfloor`:
			return mk("floor", inner)
		case `\lceil`:
			return mk("ceil", inner)
		case `\langle`:
			return mk("<>", inner)
		}
		return mk("[]", inner)
	case `\frac`, `\dfrac`, `\tfrac`, `\binom`, `\dbinom`, `\tbinom`:
		a := p.group()
		b := p.group()
		op := "/"
		if strings.HasSuffix(s, "binom") {
			op = "binom"
		}
		return mk(op, a, b)
	case `\sqrt`:
		if p.peek() == "[" {
			p.next()
			n := p.list()
			if p.peek() == "]" {
				p.next()
			}
			return mk("root", p.group(), n)
		}
		return mk("root", p.group())
	case `\overline`, `\bar`, `\hat`, `\widehat`, `\tilde`, `\widetilde`, `\vec`, `\dot`, `\ddot`, `\underline`:
		return mk(s, p.group())
	}
	if named[s] {
		var b strings.Builder
		if p.peek() == "{" {
			p.next()
			for p.pos < len(p.toks) && p.toks[p.pos].s != "}" {
				b.WriteString(p.toks[p.pos].s)
				p.pos++
			}
			if p.pos < len(p.toks) {
				p.pos++
			}
		} else if p.peek() != "" {
			b.WriteString(p.next().s)
		}
		if s == `\operatorname` || s == `\mathrm` || s == `\text` || s == `\textrm` {
			return &tree{op: b.String()}
		}
		return &tree{op: s + "{" + b.String() + "}"}
	}
	if bigOps[s] {
		t := &tree{op: s}
		var sub, sup *tree
		for {
			switch p.peek() {
			case "_":
				p.next()
				sub = p.argument()
				continue
			case "^":
				p.next()
				sup = p.argument()
				continue
			}
			break
		}
		t.kids = []*tree{orEmpty(sub), orEmpty(sup), orEmpty(p.product())}
		return t
	}
	return p.leaf(tok)
}

func orEmpty(t *tree) *tree {
	if t == nil {
		return &tree{op: "∅"}
	}
	return t
}

// mk makes a tree with the operator and the kids. It leaves out the nil kids, and it joins the operands of an associative operator.
func mk(op string, kids ...*tree) *tree {
	t := &tree{op: op}
	for _, k := range kids {
		if k == nil {
			continue
		}
		if (op == "+" || op == "*") && k.op == op && !k.vari {
			t.kids = append(t.kids, k.kids...)
			continue
		}
		t.kids = append(t.kids, k)
	}
	if len(t.kids) == 1 && (op == "+" || op == "*") {
		return t.kids[0]
	}
	return t
}

// size returns the number of nodes of t.
func (t *tree) size() int {
	n := 1
	for _, k := range t.kids {
		n += k.size()
	}
	return n
}

// shape returns the form of t in which each variable is V. Two formulas that differ only in the names of their variables have the same shape.
func (t *tree) shape() string {
	if t.vari {
		return "V"
	}
	if len(t.kids) == 0 {
		return t.op
	}
	parts := make([]string, len(t.kids))
	for i, k := range t.kids {
		parts[i] = k.shape()
	}
	if commutative[t.op] {
		slices.Sort(parts)
	}
	return t.op + "(" + strings.Join(parts, ",") + ")"
}

// canonical sorts the operands of each commutative operator by their shape, so that a+b and b+a have one form.
func (t *tree) canonical() {
	for _, k := range t.kids {
		k.canonical()
	}
	if commutative[t.op] {
		slices.SortStableFunc(t.kids, func(a, b *tree) int { return strings.Compare(a.shape(), b.shape()) })
	}
}

// key returns the form of t in which the variables are numbered in the order that they first occur. Two subtrees with the same key are equal after a consistent change of the names of the variables.
func (t *tree) key(names map[string]int, b *strings.Builder) {
	if t.vari {
		n, ok := names[t.op]
		if !ok {
			n = len(names)
			names[t.op] = n
		}
		b.WriteString("v" + strconv.Itoa(n))
		return
	}
	b.WriteString(t.op)
	if len(t.kids) == 0 {
		return
	}
	b.WriteByte('(')
	for i, k := range t.kids {
		if i > 0 {
			b.WriteByte(',')
		}
		k.key(names, b)
	}
	b.WriteByte(')')
}

// paths returns the tokens of the leaf-to-root paths of t, with the variables anonymous (spec 14.2). Each leaf gives its symbol with the operators above it, up to three of them. Each operator gives itself with the shapes of the operators below it.
func (t *tree) paths() []string {
	var out []string
	var walk func(t *tree, up []string)
	walk = func(t *tree, up []string) {
		if len(t.kids) == 0 {
			sym := t.op
			if t.vari {
				sym = "V"
			}
			out = append(out, "l:"+sym)
			for d := 1; d <= 3 && d <= len(up); d++ {
				out = append(out, "p:"+sym+"/"+strings.Join(up[len(up)-d:], "/"))
			}
			return
		}
		local := make([]string, len(t.kids))
		for i, k := range t.kids {
			switch {
			case k.vari:
				local[i] = "V"
			case len(k.kids) == 0:
				local[i] = k.op
			default:
				local[i] = k.op + "()"
			}
		}
		if commutative[t.op] {
			slices.Sort(local)
		}
		out = append(out, "o:"+t.op, "o:"+t.op+"("+strings.Join(local, ",")+")")
		for i, k := range t.kids {
			step := t.op
			if !commutative[t.op] && len(t.kids) > 1 {
				step += "." + strconv.Itoa(i)
			}
			walk(k, append(slices.Clip(up), step))
		}
	}
	walk(t, nil)
	return out
}

// subtrees returns the keys of the subtrees of t with two or more nodes, with the size of each one.
func (t *tree) subtrees() map[string]int {
	out := map[string]int{}
	var walk func(t *tree) int
	walk = func(t *tree) int {
		n := 1
		for _, k := range t.kids {
			n += walk(k)
		}
		if n >= 2 {
			var b strings.Builder
			t.key(map[string]int{}, &b)
			out[hashWord(b.String())] = n
		}
		return n
	}
	walk(t)
	return out
}

// hashWord returns a word of letters and digits for a token, so that the FTS5 tokenizer keeps the token as one word.
func hashWord(s string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return "f" + strconv.FormatUint(h.Sum64(), 36)
}

// formula is a parsed formula, as the formula channel indexes it.
type formula struct {
	words    string         // the hashed path tokens, separated by spaces
	subtrees map[string]int // the hashed keys of the subtrees and their sizes
	size     int
}

func indexFormula(src string) (formula, bool) {
	t := parseFormula(src)
	if t == nil || t.size() < 2 {
		return formula{}, false
	}
	t.canonical()
	var words []string
	for _, p := range t.paths() {
		words = append(words, hashWord(p))
	}
	return formula{words: strings.Join(words, " "), subtrees: t.subtrees(), size: t.size()}, true
}

// structural returns the size of the largest subtree of the query that the candidate has, as a fraction of the size of the query. The variables of the subtree unify consistently, because the keys number them.
func structural(q, c formula) float64 {
	best := 0
	for k, n := range q.subtrees {
		if _, ok := c.subtrees[k]; ok && n > best {
			best = n
		}
	}
	if q.size == 0 {
		return 0
	}
	return float64(best) / float64(q.size)
}

// mathSpans returns the formulas in Markdown text: the text between $ and $, $$ and $$, \( and \), or \[ and \].
func mathSpans(text string) []string {
	var out []string
	for i := 0; i < len(text); i++ {
		var open, end string
		switch {
		case strings.HasPrefix(text[i:], "$$"):
			open, end = "$$", "$$"
		case text[i] == '$' && (i == 0 || text[i-1] != '\\'):
			open, end = "$", "$"
		case strings.HasPrefix(text[i:], `\(`):
			open, end = `\(`, `\)`
		case strings.HasPrefix(text[i:], `\[`):
			open, end = `\[`, `\]`
		default:
			continue
		}
		rest := text[i+len(open):]
		j := strings.Index(rest, end)
		if j < 0 {
			break
		}
		if s := strings.TrimSpace(rest[:j]); s != "" {
			out = append(out, s)
		}
		i += len(open) + j + len(end) - 1
	}
	return out
}
