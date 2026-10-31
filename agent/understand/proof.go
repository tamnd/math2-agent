package understand

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	axImport "github.com/tamnd/math2-agent/importer/arxiv"
)

// The pass proof (AX7.6) gives the structure of each proof: its steps, each as a range of the numbered sentences of the proof with its reason, the results that a step uses and that no reference names, the methods of the proof, and for an orphan proof the item that it most likely proves. The model reads the proof with the items that it can use: the item that it proves, the items that its edges name, and the items before it in the paper.
//
// A record holds the spans and the reasons for each class. The one-line text of a step is words that the model writes about the paper, thus only a record of the class open holds it. An inferred use becomes an edge with the method model and the confidence low downstream (AX6.1 source 9). The target of an orphan proof goes to the queue proof-target, where a person confirms it. The pass does not judge whether the proof is correct.
//
// The status of a statement with proof_status none (the last part of AX7.6) is not in this pass, because its item is a statement and not a proof.

// Proof is the pass of the structure of the proofs.
const Proof = "proof"

// ProofTarget is the queue of the targets that the pass gives to an orphan proof.
const ProofTarget = "proof-target"

// Reasons are the kinds of the reason of a step. The enum of schema/understand/proof.v1.json has the same kinds.
var Reasons = []string{"item", "cited", "named", "definition", "algebra", "hypothesis", "unknown"}

// Methods are the proof methods that the pass can give, until manifests/methods.yaml has the list. The enum of schema/understand/proof.v1.json has the same methods.
var Methods = []string{
	"induction", "strong-induction", "structural-induction", "transfinite-induction", "contradiction", "contrapositive", "case-analysis", "construction",
	"compactness", "diagonal-argument", "probabilistic-method", "pigeonhole", "double-counting", "extremal-principle", "averaging", "bijection",
	"generating-function", "fixed-point", "zorn-lemma", "density-argument", "approximation", "localization", "duality", "exact-sequence", "spectral-sequence",
	"change-of-variables", "integration-by-parts", "estimate", "variational", "potential-function", "symmetry", "reduction", "other",
}

// The limits of the pass: the body of the proof, the body of each item that it can use, and their count.
const (
	proofChars   = 12000
	usableChars  = 400
	provedChars  = 2000
	maxUsable    = 40
	maxStepText  = 200
	maxProofStep = 200
)

// ProofResult is the result of the pass proof on one proof.
type ProofResult struct {
	Steps    []Step     `json:"steps"`
	Inferred []Inferred `json:"inferred,omitempty"`
	Methods  []string   `json:"methods,omitempty"`
	Target   *Proved    `json:"target,omitempty"`
}

// Step is one step of a proof: the range of its sentences, its reason, and its one-line text in a record of the class open.
type Step struct {
	From   int    `json:"from"`
	To     int    `json:"to"`
	Reason string `json:"reason"`        // one of Reasons
	Ref    string `json:"ref,omitempty"` // the item, ax://paper/ID#TAG, for item and definition; the key bib:KEY for cited; the name for named; the label for hypothesis
	Text   string `json:"text,omitempty"`
}

// Inferred is a result that a step uses and that no reference names, as the Cauchy-Schwarz inequality.
type Inferred struct {
	Name string `json:"name"`
	Step int    `json:"step"` // the number of the step, from 1
}

// Proved is the item that an orphan proof most likely proves, with a confidence.
type Proved struct {
	Item       string  `json:"item"`
	Confidence float64 `json:"confidence"`
}

var proofPass = pass{
	tool:  "give_proof",
	about: "Give the structure of each proof of the paper: its steps with their reasons, the results that it uses and that no reference names, its methods, and the target of an orphan proof. Give all the proofs in one call.",
	other: "Give the structure of each proof. Only for the pass proof.",
	chars: proofChars,
	reads: func(item string) bool { return item == "proof" },
	brief: func(b *strings.Builder, p *Paper) {
		fmt.Fprintf(b, "Give the structure of each of the %d proofs below with one call of give_proof. Each proof is in numbered sentences, and after it come the items that it can use. Split the proof into steps, each a range of sentence numbers, with its reason: %s. Give the ref of a reason: the item id for item and definition, the key as bib:KEY for cited, the name of the result for named, the label for hypothesis. Give in inferred each result that a step uses and that no reference names, with the number of its step, in methods the methods of the proof, and for a proof with no item that it proves, in target the item that it most likely proves. Do not judge whether the proof is correct.", len(p.Items), strings.Join(Reasons, ", "))
		if p.Class == "open" {
			b.WriteString(" Give each step a text of one line.")
		} else {
			b.WriteString(" Give no text of a step: the licence of the paper does not permit it.")
		}
		b.WriteString("\n\n")
	},
	check: checkProof,
	load:  loadProof,
}

// loadProof numbers the sentences of each proof, and gives each proof the items that it can use.
func loadProof(root string, p *Paper, latest []axImport.Object) error {
	es, err := axImport.ReadEdges(root, p.ID)
	if err != nil {
		return fmt.Errorf("understand: %w", err)
	}
	p.Context, p.Tags, p.Steps, p.Orphans = map[string][]Context{}, map[string]string{}, map[string]int{}, map[string]bool{}
	byLocal := map[string]axImport.Object{}
	for _, o := range latest {
		if o.Tag != "" {
			byLocal[o.Local] = o
			p.Tags[axImport.MentionID(p.ID, o.Tag)] = o.Item
		}
	}
	ctxOf := func(o axImport.Object, why string, n int) Context {
		return Context{ID: axImport.MentionID(p.ID, o.Tag), Ref: fmt.Sprintf("%sv%d#%s", p.ID, p.Version, o.Local), Item: o.Item, Number: o.Number, Why: why, Body: cut(o.BodyMD, n)}
	}
	for i, it := range p.Items {
		_, local, _ := strings.Cut(it.Ref, "#")
		self := byLocal[local]
		ss := sentences(cut(self.BodyMD, proofChars))
		var b strings.Builder
		for n, s := range ss {
			fmt.Fprintf(&b, "[%d] %s\n", n+1, s)
		}
		p.Items[i].Item, p.Items[i].Body = "proof", strings.TrimSuffix(b.String(), "\n")
		p.Steps[it.ID] = len(ss)
		var cs []Context
		add := func(o axImport.Object, why string, n int) {
			c := ctxOf(o, why, n)
			if c.ID != it.ID && c.Body != "" && len(cs) < maxUsable && !slices.ContainsFunc(cs, func(x Context) bool { return x.ID == c.ID }) {
				cs = append(cs, c)
			}
		}
		if o, ok := byLocal[self.Proves]; ok && self.Proves != "" {
			add(o, "proves", provedChars)
		} else {
			p.Orphans[it.ID] = true
		}
		for _, e := range es {
			from, ok1 := strings.CutPrefix(e.From, p.ID+"#")
			to, ok2 := strings.CutPrefix(e.To, p.ID+"#")
			if o, ok3 := byLocal[to]; ok1 && ok2 && ok3 && from == local && e.Evidence.Version == p.Version && e.Predicate != "proves" {
				add(o, e.Predicate, usableChars)
			}
		}
		var before []axImport.Object
		for _, o := range latest {
			k, _, _ := strings.Cut(o.Item, "/")
			if o.Tag != "" && o.Order < self.Order && (k == "statement" || k == "definition" || k == "context") {
				before = append(before, o)
			}
		}
		for j := len(before) - 1; j >= 0; j-- { // the nearest items first
			add(before[j], "before", usableChars)
		}
		p.Context[it.ID] = cs
	}
	return nil
}

// sentences splits a text of TeX into its sentences: at a full stop, a question mark or an exclamation mark outside math that space and a capital letter, a dollar or a backslash follow, and at a blank line.
func sentences(s string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if t := clean(cur.String()); t != "" {
			out = append(out, t)
		}
		cur.Reset()
	}
	rs := []rune(s)
	math := false
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		cur.WriteRune(r)
		switch {
		case r == '\\' && i+1 < len(rs):
			i++
			cur.WriteRune(rs[i])
			switch rs[i] {
			case '[', '(':
				math = true
			case ']', ')':
				math = false
			}
		case r == '$':
			math = !math
		case r == '\n' && i+1 < len(rs) && rs[i+1] == '\n' && !math:
			flush()
		case (r == '.' || r == '?' || r == '!') && !math && i+2 < len(rs) && unicode.IsSpace(rs[i+1]):
			j := i + 1
			for j < len(rs) && unicode.IsSpace(rs[j]) {
				j++
			}
			if j < len(rs) && (unicode.IsUpper(rs[j]) || rs[j] == '$' || rs[j] == '\\') {
				flush()
			}
		}
	}
	flush()
	return out
}

var bibKey = regexp.MustCompile(`^bib:[^\s,{}]+$`)

func checkProof(r *Record, p *Paper, it Item, raw json.RawMessage) ([]string, string) {
	var in struct {
		Steps []struct {
			From, To          int
			Reason, Ref, Text string
		}
		Inferred []Inferred
		Methods  []string
		Target   *Proved
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Sprintf("the result of %s: %v", it.ID, err)
	}
	n := p.Steps[it.ID]
	if len(in.Steps) == 0 && n > 0 {
		return nil, fmt.Sprintf("%s has no step", it.ID)
	}
	res := &ProofResult{Steps: []Step{}}
	var left []string
	last := 0
	for i, s := range in.Steps {
		name := fmt.Sprintf("step %d of %s", i+1, it.ID)
		switch {
		case s.From < 1 || s.To > n || s.To < s.From:
			return nil, fmt.Sprintf("%s spans the sentences %d to %d, and the proof has the sentences 1 to %d", name, s.From, s.To, n)
		case s.From <= last:
			return nil, fmt.Sprintf("%s starts at the sentence %d, before the end of the step before it. Give the steps in order, with no overlap", name, s.From)
		case !slices.Contains(Reasons, s.Reason):
			return nil, fmt.Sprintf("the reason %q of %s is not one of %s", s.Reason, name, strings.Join(Reasons, ", "))
		}
		last = s.To
		st := Step{From: s.From, To: s.To, Reason: s.Reason, Ref: strings.TrimSpace(s.Ref)}
		switch {
		case (s.Reason == "item" || s.Reason == "definition") && p.Tags[st.Ref] == "":
			return nil, fmt.Sprintf("the ref %q of %s is not an item of the paper", st.Ref, name)
		case s.Reason == "cited" && !bibKey.MatchString(st.Ref):
			return nil, fmt.Sprintf("the ref %q of %s is not a key as bib:KEY", st.Ref, name)
		case s.Reason == "named" || s.Reason == "hypothesis":
			if st.Ref = cut(clean(st.Ref), maxName); st.Ref == "" {
				return nil, fmt.Sprintf("%s has the reason %s and no ref", name, s.Reason)
			}
		case s.Reason == "algebra" || s.Reason == "unknown":
			st.Ref = ""
		}
		if t := clean(s.Text); t != "" {
			if p.Class == "open" {
				st.Text = cut(t, maxStepText)
			} else if !slices.Contains(left, "the text of each step") {
				left = append(left, "the text of each step, because math2 stores no text of a step of a paper of the class "+p.Class)
			}
		}
		res.Steps = append(res.Steps, st)
	}
	if len(res.Steps) > maxProofStep {
		return nil, fmt.Sprintf("%s has more than %d steps", it.ID, maxProofStep)
	}
	for _, x := range in.Inferred {
		x.Name = cut(clean(x.Name), maxName)
		if x.Name == "" || x.Step < 1 || x.Step > len(res.Steps) {
			left = append(left, fmt.Sprintf("the inferred use %q, whose step is not a step of the result", x.Name))
			continue
		}
		res.Inferred = append(res.Inferred, x)
	}
	for _, m := range in.Methods {
		if !slices.Contains(Methods, m) {
			left = append(left, fmt.Sprintf("the method %q, which is not one of the list", m))
		} else if !slices.Contains(res.Methods, m) {
			res.Methods = append(res.Methods, m)
		}
	}
	if t := in.Target; t != nil {
		switch k, _, _ := strings.Cut(p.Tags[t.Item], "/"); {
		case !p.Orphans[it.ID]:
			left = append(left, "the target, because the proof has an item that it proves")
		case k != "statement":
			left = append(left, fmt.Sprintf("the target %q, which is not a statement of the paper", t.Item))
		case t.Confidence < 0 || t.Confidence > 1:
			left = append(left, fmt.Sprintf("the target, whose confidence %v is not between 0 and 1", t.Confidence))
		default:
			res.Target = t
			r.Queue = ProofTarget
		}
	}
	r.Proof = res
	return left, ""
}
