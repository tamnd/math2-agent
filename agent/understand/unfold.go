package understand

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/tamnd/math2-agent/agent/tools"
	axImport "github.com/tamnd/math2-agent/importer/arxiv"
	"github.com/tamnd/math2-agent/llm"
)

// The pass unfold (AX7.5) writes a statement, a definition, a conjecture or a problem as one self-contained text, so that a reader of that one item, with standard background, can read it. The model reads the item with its context: the context items before it whose scope holds it (the paper, or its section for the scope local), and the items of the paper that it uses or names by reference, from the edges plane. It gives the unfolded form in TeX, what it inlined from the context, the terms it left as standard background, and the places where it had to choose.
//
// Two checks run (rule D11). The mechanical check is in the tool: each item that the result says it inlined is an item of the paper, the unfolded form names no item by its number, and each of its macros is standard TeX, not a macro of the paper. A result that fails it is not valid. Then a model of another family reads the original, the context and the unfolded form, and gives a verdict. Only equivalent passes. Another verdict sends the result back to the model once, with the verdict; a second one stores the record in the queue unfold-review. While math2 has no checker of another family, each record goes to unfold-review.
//
// The unfolded form is derived data under the licence of the version (AX5.4). A record of the class open or share-alike-nc holds it. A record of the class verbatim holds only its hash, the ids of what it inlined and the verdict, with no reason, because an unfolding is an adaptation and ND forbids it. A paper of the class record has no unfolding: its text goes to no model of a provider (AQ08).
//
// The pass runs on each item of the paper that the target names. AX7.8 picks the papers and the items that need it, before a job runs.

// Unfold is the pass of the self-contained statements.
const Unfold = "unfold"

// UnfoldReview is the queue of the unfoldings that the check did not accept.
const UnfoldReview = "unfold-review"

// Verdicts are the verdicts of the check of an unfolding. Only Equivalent passes.
var Verdicts = []string{Equivalent, "stronger", "weaker", "different", "unsure"}

// Equivalent is the verdict of an unfolding that says what the original says.
const Equivalent = "equivalent"

// The limits of the pass: the body of the item, the body of each context item, and the unfolded form.
const (
	unfoldChars   = 6000
	contextChars  = 1500
	maxUnfolded   = 8000
	maxContext    = 12
	maxUnfoldNote = 300
)

// Context is one item that the model reads with an item of the pass unfold.
type Context struct {
	ID     string // the id of the item, ax://paper/ID#TAG
	Ref    string // the object in its version, IDvN#LOCAL
	Item   string
	Number string
	Why    string // scope, when the scope of the context item holds the item, or the predicate of the edge: uses or refers
	Body   string
}

// String gives the context item as the model reads it.
func (c Context) String() string {
	head := "kind: " + c.Item
	if c.Number != "" {
		head += "\nnumber: " + c.Number
	}
	return head + "\n\n" + c.Body
}

// Unfolding is the result of the pass unfold on one item. Unfolded, the parts of Inlined, Assumed and Notes are empty in a record of the class verbatim.
type Unfolding struct {
	Unfolded string    `json:"unfolded,omitempty"`
	Hash     string    `json:"hash"` // the sha256 of the unfolded form, as sha256:HEX
	Inlined  []Inlined `json:"inlined"`
	Assumed  []string  `json:"assumed,omitempty"`
	Notes    []string  `json:"notes,omitempty"`
	Verdict  *Verdict  `json:"verdict,omitempty"`
}

// Inlined is one part of the context that the unfolding took: the item, or "" for a part with no item, and which part of it.
type Inlined struct {
	Item string `json:"item,omitempty"`
	Part string `json:"part,omitempty"`
}

// Verdict is the answer of the check of an unfolding by a model of another family.
type Verdict struct {
	Model   string `json:"model"`
	Verdict string `json:"verdict"`          // one of Verdicts
	Reason  string `json:"reason,omitempty"` // in a record of the class open or share-alike-nc
}

// UnfoldCheck reads an item, its context and its unfolded form, and gives a verdict. The model of the check is of a family other than the family of the model of the pass (rule X09).
type UnfoldCheck func(ctx context.Context, p *Paper, it Item, cs []Context, unfolded string) (*Verdict, error)

var unfoldPass = pass{
	tool:  "unfold_items",
	about: "Give the self-contained form of each item of the paper, with what it inlined, what it assumed and the places where it had to choose. Give all the items in one call.",
	other: "Give the self-contained form of each item. Only for the pass unfold.",
	chars: unfoldChars,
	reads: func(item string) bool {
		k, _, _ := strings.Cut(item, "/")
		return k == "statement" || k == "definition" || k == "conjecture" || k == "problem"
	},
	brief: func(b *strings.Builder, p *Paper) {
		fmt.Fprintf(b, "Unfold each of the %d items below with one call of unfold_items. After each item come its context items: the standing hypotheses whose scope holds it, and the items that it uses or names by reference. Write the item as one self-contained text in TeX that a reader of that one item, with standard background, can read: inline the hypotheses, the definitions and the statements that it depends on, name no item by its number, and use standard notation in place of the macros and the local notation of the paper. Give in inlined each part that you took, with the id of its item, in assumed the terms that you left as standard background, and in notes each place where you had to choose.\n\n", len(p.Items))
	},
	check: checkUnfold,
	after: afterUnfold,
	load:  loadUnfold,
}

// loadUnfold gives each item its context, from the objects of the version and the edges plane.
func loadUnfold(root string, p *Paper, latest []axImport.Object) error {
	if p.Class == "record" {
		return fmt.Errorf("understand: %sv%d is of the class record, and its text goes to no model of a provider (AQ08), thus it has no unfolding", p.ID, p.Version)
	}
	es, err := axImport.ReadEdges(root, p.ID)
	if err != nil {
		return fmt.Errorf("understand: %w", err)
	}
	p.Context, p.Tags = map[string][]Context{}, map[string]string{}
	byLocal := map[string]axImport.Object{}
	for _, o := range latest {
		if o.Tag != "" {
			byLocal[o.Local] = o
			p.Tags[axImport.MentionID(p.ID, o.Tag)] = o.Item
		}
	}
	ctxOf := func(o axImport.Object, why string) Context {
		return Context{ID: axImport.MentionID(p.ID, o.Tag), Ref: fmt.Sprintf("%sv%d#%s", p.ID, p.Version, o.Local), Item: o.Item, Number: o.Number, Why: why, Body: cut(o.BodyMD, contextChars)}
	}
	for _, it := range p.Items {
		_, local, _ := strings.Cut(it.Ref, "#")
		self := byLocal[local]
		var cs []Context
		add := func(c Context) {
			if c.ID != it.ID && c.Body != "" && len(cs) < maxContext && !slices.ContainsFunc(cs, func(x Context) bool { return x.ID == c.ID }) {
				cs = append(cs, c)
			}
		}
		for _, o := range latest {
			if o.Tag != "" && strings.HasPrefix(o.Item, "context/") && (o.Scope == "paper" || o.Scope == "local" && o.Section == self.Section) && o.Order < self.Order {
				add(ctxOf(o, "scope"))
			}
		}
		for _, e := range es {
			from, ok1 := strings.CutPrefix(e.From, p.ID+"#")
			to, ok2 := strings.CutPrefix(e.To, p.ID+"#")
			o, ok3 := byLocal[to]
			if ok1 && ok2 && ok3 && from == local && e.Evidence.Version == p.Version && (e.Predicate == "uses" || e.Predicate == "refers") {
				add(ctxOf(o, e.Predicate))
			}
		}
		p.Context[it.ID] = cs
	}
	return nil
}

// byNumber finds a name of an item by its number, as Theorem 2.1, Lemma~3 or (2.4), and a reference macro of TeX.
var byNumber = regexp.MustCompile(`(?i)\b(?:theorem|thm|lemma|lem|proposition|prop|corollary|cor|definition|defn|def|remark|example|conjecture|problem|assumption|hypothesis|hypotheses|condition|claim|section|equation|eq|eqn)s?\.?(?:~|\s)*\(?[0-9]+(?:\.[0-9]+)*\)?|\\(?:ref|eqref|cref|Cref|autoref|pageref|cite)\b`)

// macro finds a control word of TeX.
var macro = regexp.MustCompile(`\\([A-Za-z]+)`)

// standard are the control words of TeX, LaTeX, amsmath and amssymb that an unfolded form may use, other than those that macro families of standardFamily give.
var standard = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`
alpha beta gamma delta epsilon varepsilon zeta eta theta vartheta iota kappa varkappa lambda mu nu xi pi varpi rho varrho sigma varsigma tau upsilon phi varphi chi psi omega
Gamma Delta Theta Lambda Xi Pi Sigma Upsilon Phi Psi Omega digamma aleph beth gimel hbar ell wp Re Im partial infty nabla emptyset varnothing
in notin ni subset subseteq subsetneq supset supseteq supsetneq sqsubset sqsubseteq cup cap bigcup bigcap setminus sqcup bigsqcup uplus biguplus
le leq ge geq neq ne equiv approx sim simeq cong propto prec preceq succ succeq ll gg mid nmid parallel perp models vdash dashv lhd rhd unlhd unrhd trianglelefteq trianglerighteq lesssim gtrsim asymp doteq
leqslant geqslant nleq ngeq nsubseteq ncong
to gets mapsto rightarrow leftarrow leftrightarrow Rightarrow Leftarrow Leftrightarrow longrightarrow longleftarrow longmapsto Longrightarrow Longleftrightarrow iff implies impliedby hookrightarrow hookleftarrow twoheadrightarrow rightharpoonup uparrow downarrow updownarrow nearrow searrow rightrightarrows leftrightarrows xrightarrow xleftarrow
forall exists nexists neg lnot land lor wedge vee bigwedge bigvee oplus bigoplus otimes bigotimes ominus odot bigodot circ bullet cdot cdots ldots dots dotsc dotsb vdots ddots times div pm mp ast star dagger ddagger amalg wr
sum prod coprod int iint iiint oint lim limsup liminf sup inf max min arg argmax argmin det dim ker deg gcd lcm hom Hom exp log ln lg sin cos tan cot sec csc arcsin arccos arctan sinh cosh tanh coth Pr
frac dfrac tfrac binom dbinom tbinom sqrt overline underline widehat widetilde hat tilde bar vec dot ddot check breve acute grave mathring overbrace underbrace overrightarrow overleftarrow stackrel overset underset substack
left right big Big bigg Bigg bigl bigr Bigl Bigr biggl biggr middle langle rangle lceil rceil lfloor rfloor lvert rvert lVert rVert vert Vert backslash
quad qquad enspace thinspace negthinspace hspace vspace hfill
text textbf textit textrm textsf texttt textup emph operatorname mod bmod pmod pod
mathbb mathcal mathfrak mathscr mathrm mathbf mathit mathsf mathtt mathbbm boldsymbol bm
begin end item label displaystyle textstyle scriptstyle limits nolimits not colon prime top bot angle triangle square Box Diamond blacksquare diamond flat sharp natural surd
cdotp ast setminus smallsetminus complement therefore because qed square checkmark
`) {
		standard[w] = true
	}
}

// localMacros returns the control words of the unfolded form that are not standard, in their order.
func localMacros(s string) []string {
	var out []string
	for _, m := range macro.FindAllStringSubmatch(s, -1) {
		if !standard[m[1]] && !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}

func checkUnfold(r *Record, p *Paper, it Item, raw json.RawMessage) ([]string, string) {
	var in struct {
		Unfolded string
		Inlined  []Inlined
		Assumed  []string
		Notes    []string
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Sprintf("the result of %s: %v", it.ID, err)
	}
	u := strings.TrimSpace(in.Unfolded)
	switch {
	case u == "":
		return nil, fmt.Sprintf("%s has no unfolded form", it.ID)
	case utf8.RuneCountInString(u) > maxUnfolded:
		return nil, fmt.Sprintf("the unfolded form of %s has more than %d characters", it.ID, maxUnfolded)
	}
	if m := byNumber.FindString(u); m != "" {
		return nil, fmt.Sprintf("the unfolded form of %s names an item by its number or by a reference, %q. Write out what that item says", it.ID, m)
	}
	if ms := localMacros(u); len(ms) > 0 {
		return nil, fmt.Sprintf("the unfolded form of %s uses the macros \\%s, which are not standard TeX. Write them in standard notation", it.ID, strings.Join(ms, ", \\"))
	}
	un := &Unfolding{Inlined: []Inlined{}}
	for _, x := range in.Inlined {
		x.Item, x.Part = strings.TrimSpace(x.Item), cut(clean(x.Part), maxUnfoldNote)
		if x.Item != "" && p.Tags[x.Item] == "" {
			return nil, fmt.Sprintf("the item %q that %s inlined is not an item of the paper", x.Item, it.ID)
		}
		un.Inlined = append(un.Inlined, x)
	}
	for _, a := range in.Assumed {
		if a = cut(clean(a), maxName); a != "" {
			un.Assumed = append(un.Assumed, a)
		}
	}
	for _, n := range in.Notes {
		if n = cut(clean(n), maxUnfoldNote); n != "" {
			un.Notes = append(un.Notes, n)
		}
	}
	sum := sha256.Sum256([]byte(u))
	un.Unfolded, un.Hash = u, "sha256:"+hex.EncodeToString(sum[:])
	r.Unfold = un
	return nil, ""
}

// afterUnfold runs the check of the unfolding by a model of another family, and then keeps in the record only what the class of the version permits. With no check, the record goes to unfold-review, because no model of another family has read it.
func afterUnfold(ctx context.Context, r *Record, p *Paper) (string, error) {
	un := r.Unfold
	defer keep(r, p.Class)
	if p.UnfoldCheck == nil {
		r.Queue = UnfoldReview
		return "", nil
	}
	var it Item
	for _, x := range p.Items {
		if x.ID == r.ID {
			it = x
		}
	}
	v, err := p.UnfoldCheck(ctx, p, it, p.Context[r.ID], un.Unfolded)
	if err != nil {
		return "", err
	}
	un.Verdict = v
	if v != nil && v.Verdict == Equivalent {
		r.Queue = ""
		return "", nil
	}
	r.Queue = UnfoldReview
	why := "the check by a model of another family gives no verdict"
	if v != nil {
		why = fmt.Sprintf("the check by a model of another family gives the verdict %s", v.Verdict)
		if v.Reason != "" {
			why += ": " + v.Reason
		}
	}
	return fmt.Sprintf("%s for the unfolded form of %s. Unfold the item again", why, r.ID), nil
}

// keep removes from the unfolding what the class of the version does not permit a record to hold (AX5.4): for the class verbatim, all but the hash, the ids of the inlined items and the verdict.
func keep(r *Record, class string) {
	un := r.Unfold
	if un == nil || class == "open" || class == "share-alike-nc" {
		return
	}
	un.Unfolded, un.Assumed, un.Notes = "", nil, nil
	ins := []Inlined{}
	for _, x := range un.Inlined {
		if x.Item != "" && !slices.ContainsFunc(ins, func(y Inlined) bool { return y.Item == x.Item }) {
			ins = append(ins, Inlined{Item: x.Item})
		}
	}
	un.Inlined = ins
	if un.Verdict != nil {
		v := *un.Verdict
		v.Reason = ""
		un.Verdict = &v
	}
}

// unfoldTool is the tool of the check of an unfolding.
var unfoldTool = llm.Tool{Name: "check_unfold", Description: "Give the verdict on the unfolded form against the original, with the reason.", Schema: json.RawMessage(`{"type":"object","properties":{` +
	`"verdict":{"type":"string","enum":["equivalent","stronger","weaker","different","unsure"],"description":"equivalent when the unfolded form says what the original says in its context; stronger when it claims more; weaker when it claims less"},` +
	`"reason":{"type":"string","description":"why, in one or two sentences"}},"required":["verdict","reason"],"additionalProperties":false}`)}

// UnfoldCheckWith returns the check of the unfoldings with the model, which must be of another family than the model of the pass. The system prompt is checks/unfold.md of the role.
func UnfoldCheckWith(m llm.Model, model, system string) UnfoldCheck {
	return func(ctx context.Context, p *Paper, it Item, cs []Context, unfolded string) (*Verdict, error) {
		var b strings.Builder
		fmt.Fprintf(&b, "Read the original item with its context, and the unfolded form, and give your verdict with the tool %s.\n\n", unfoldTool.Name)
		fmt.Fprintf(&b, "The original, %s:\n%s\n\n", it.ID, tools.Data("arxiv:"+it.Ref, licenceLabel(p), "kind: "+it.Item+"\n\n"+it.Body))
		for _, c := range cs {
			fmt.Fprintf(&b, "Context, %s, %s:\n%s\n\n", c.ID, c.Why, tools.Data("arxiv:"+c.Ref, licenceLabel(p), c.String()))
		}
		fmt.Fprintf(&b, "The unfolded form:\n%s\n", tools.Data("unfolded:"+it.ID, licenceLabel(p), unfolded))
		resp, err := m.Complete(ctx, &llm.Request{
			Model:     model,
			System:    system,
			Messages:  []llm.Message{{Role: llm.User, Content: []llm.Block{llm.TextBlock(b.String())}}},
			Tools:     []llm.Tool{unfoldTool},
			MaxTokens: 800,
		})
		if err != nil {
			return nil, fmt.Errorf("understand: the check of the unfolding: %w", err)
		}
		for _, call := range resp.Calls() {
			if call.Name != unfoldTool.Name {
				continue
			}
			var v struct{ Verdict, Reason string }
			if json.Unmarshal(call.Input, &v) == nil && slices.Contains(Verdicts, v.Verdict) {
				return &Verdict{Model: model, Verdict: v.Verdict, Reason: cut(clean(v.Reason), maxUnfoldNote)}, nil
			}
		}
		return &Verdict{Model: model, Verdict: "unsure"}, nil
	}
}
