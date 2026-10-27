// Package understand has the goal, the brief and the tools of the understand role (corpus arxiv AX07).
//
// A job of the role is one pass on one arXiv paper, thus the paper is the cached part of the prompt and each item of the paper shares it. The target is arxiv:ID/PASS, for example arxiv:2401.00001/classify. The passes of tier T1 are written: classify (AX7.2) and concepts (AX7.3). The brief gives the items of the latest version that the pass reads, from the analysis plane of a checkout of tamnd/arxiv, in data blocks marked with the licence of the version. The model gives its result with the tool of the pass, and the tool writes one record for each item to records/understand/arxiv/PASS/SHARD.jsonl.
//
// A record holds no text of the paper (AX5.4), other than the terms that a definition defines. It holds ids, codes, kinds, flags, a confidence and the names that the model gives for concepts. The defined terms of a definition are words of the paper, thus a record of a version of the class record has none.
//
// [Goal] holds when the changes are inside the field permission set of understand and each item of the pass has a record of the run.
package understand

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/tools"
	axImport "github.com/tamnd/math2-agent/importer/arxiv"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/workspace"
)

// The passes of tier T1.
const (
	Classify = "classify"
	Concepts = "concepts"
)

// Passes are the passes that are written, in the order of AX7.1.
var Passes = []string{Classify, Concepts}

// Dir is the directory of the records of the passes, with one directory for each pass.
const Dir = agent.UnderstandDir

// ReviewBelow is the confidence below which a classification goes to the queue classify-review (AX7.2).
const ReviewBelow = 0.7

// ClassifyReview is the queue of the classifications with a low confidence.
const ClassifyReview = "classify-review"

// The flags of classify.
const (
	ClaimInRemark       = "claim-in-remark"
	ConjectureCandidate = "conjecture-candidate"
)

// NewConcept is the flag of a concept that has no row and no Wikidata item. It is a candidate for the enumerate queue, and it is not a row (rule I03).
const NewConcept = "new-concept"

// Kinds are the item kinds of AX1.3 that classify can give.
var Kinds = []string{
	"statement/theorem", "statement/lemma", "statement/proposition", "statement/corollary", "statement/claim", "statement/fact", "statement/axiom", "statement/criterion",
	"definition", "definition/notation", "definition/terminology", "definition/convention", "definition/construction", "definition/algorithm",
	"example", "example/non-example", "counterexample", "conjecture", "problem", "exercise", "proof",
	"context/hypothesis", "remark", "other",
}

// Roles are the roles of a concept in an item (AX7.3).
var Roles = []string{"about", "uses", "defines"}

// The limits of the text that the model reads of each item: the start of the body for classify (AX7.2), and more of it for concepts.
const (
	classifyChars = 400
	conceptsChars = 2000
)

// Target is the paper and the pass of a job.
type Target struct {
	Paper string // the arXiv id, for example 2401.00001 or math/9802134
	Pass  string
}

var paperID = regexp.MustCompile(`^(?:[0-9]{4}\.[0-9]{4,5}|[a-z-]+(?:\.[A-Z]{2})?/[0-9]{7})$`)

// ParseTarget returns the target of a job, in the form arxiv:ID/PASS.
func ParseTarget(s string) (Target, error) {
	rest, ok := strings.CutPrefix(s, "arxiv:")
	i := strings.LastIndex(rest, "/")
	if !ok || i < 0 {
		return Target{}, fmt.Errorf("understand: the target %q is not in the form arxiv:ID/PASS, such as arxiv:2401.00001/classify", s)
	}
	t := Target{Paper: rest[:i], Pass: rest[i+1:]}
	if !paperID.MatchString(t.Paper) {
		return Target{}, fmt.Errorf("understand: %q in the target %q is not an arXiv id", t.Paper, s)
	}
	if !slices.Contains(Passes, t.Pass) {
		return Target{}, fmt.Errorf("understand: the pass %q is not one of %s", t.Pass, strings.Join(Passes, ", "))
	}
	return t, nil
}

// Item is one item that a pass reads.
type Item struct {
	ID     string // the id of the item, ax://paper/ID#TAG, which is also the id of its mention when it has one
	Ref    string // the object in its version, ID vN#LOCAL
	Item   string // the item kind that the extractor gave
	Env    string
	Title  string
	Number string
	Head   string
	Body   string // the start of the body that the model reads, or "" when the object record has none
}

// Paper is the paper of a job, with the items of its pass.
type Paper struct {
	ID      string
	Title   string
	Version int
	Licence string // the licence of the version, or "" when the metadata has none
	Class   string // the licence class of the version (AX2.3)
	MSC     []string
	Pass    string
	Items   []Item
}

// Reads reports whether the pass reads an object of the item kind. Classify reads the objects that the extractor could not place, the remarks and the problems. Concepts reads each item that gets a mention, other than a proof and an object that the extractor could not place, which classify reads first.
func Reads(pass, item string) bool {
	k, _, _ := strings.Cut(item, "/")
	switch pass {
	case Classify:
		return k == "unknown" || k == "remark" || k == "problem"
	case Concepts:
		return axImport.Wanted(item) && k != "proof" && k != "unknown"
	}
	return false
}

// Load reads the paper of the target from the checkout of tamnd/arxiv at root. The items are the objects of the latest version that the pass reads and that have a tag.
func Load(root string, t Target) (*Paper, error) {
	meta, objs, err := axImport.ReadPaper(root, t.Paper)
	if err != nil {
		return nil, fmt.Errorf("understand: %w", err)
	}
	latest, v := axImport.Latest(objs)
	if v == 0 {
		return nil, fmt.Errorf("understand: %s has no object of the source path", axImport.ObjectsFile(t.Paper))
	}
	p := &Paper{ID: t.Paper, Version: v, Pass: t.Pass, Class: "record"}
	if meta != nil {
		p.Title = clean(meta.Title)
		p.Licence = meta.Licence(v)
		p.Class = axImport.ClassOf(p.Licence)
		p.MSC = axImport.MSC(meta.MSCClass)
	}
	limit := classifyChars
	if t.Pass == Concepts {
		limit = conceptsChars
	}
	for _, o := range latest {
		if o.Tag == "" || !Reads(t.Pass, o.Item) {
			continue
		}
		p.Items = append(p.Items, Item{
			ID: axImport.MentionID(t.Paper, o.Tag), Ref: fmt.Sprintf("%sv%d#%s", t.Paper, v, o.Local),
			Item: o.Item, Env: o.Env, Title: o.Title, Number: o.Number, Head: o.Head, Body: cut(o.BodyMD, limit),
		})
	}
	return p, nil
}

// cut returns the first n characters of s, at a word end when it can.
func cut(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)[:n]
	if i := strings.LastIndexFunc(string(r), unicode.IsSpace); i > n/2 {
		return string(r)[:i] + " ..."
	}
	return string(r) + " ..."
}

// Brief returns the first message of a run of the pass on the paper.
func Brief(job *agent.Job, p *Paper) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Job %s: the pass %s on the arXiv paper %sv%d. The goal of the job: %s.\n\n", job.ID, p.Pass, p.ID, p.Version, strings.TrimSuffix(job.Goal, "."))
	switch p.Pass {
	case Classify:
		fmt.Fprintf(&b, "Give the item kind of each of the %d items below with one call of classify_items. The kinds are: %s. Give a confidence from 0 to 1. Give the flag %s to a remark or an observation that states a claim, and the flag %s to a problem or a question with a yes or no answer.\n\n", len(p.Items), strings.Join(Kinds, ", "), ClaimInRemark, ConjectureCandidate)
	case Concepts:
		fmt.Fprintf(&b, "Give the MSC2020 codes and the concepts of each of the %d items below with one call of tag_concepts. Give at most three codes at five characters for each item, the first one first. For each concept that the item is about, uses or defines, give a Wikidata item when you know it exactly, or else the common name of the concept. For a definition, give the terms that it defines and the symbol that it introduces.\n\n", len(p.Items))
	}
	if p.Title != "" {
		fmt.Fprintf(&b, "%s\n\n", tools.Data("arxiv:"+p.ID+" title", licenceLabel(p), p.Title))
	}
	if len(p.MSC) > 0 {
		fmt.Fprintf(&b, "The paper lists the MSC2020 codes %s. Use them as the first guess, and give other codes where an item is about another area.\n\n", strings.Join(p.MSC, ", "))
	}
	if len(p.Items) == 0 {
		b.WriteString("The paper has no item for this pass.\n")
		return b.String()
	}
	b.WriteString("The items:\n\n")
	for _, it := range p.Items {
		var head strings.Builder
		fmt.Fprintf(&head, "kind now: %s", it.Item)
		for _, f := range [][2]string{{"environment", it.Env}, {"title", it.Title}, {"number", it.Number}, {"head", it.Head}} {
			if f[1] != "" {
				fmt.Fprintf(&head, "\n%s: %s", f[0], f[1])
			}
		}
		if it.Body != "" {
			fmt.Fprintf(&head, "\n\n%s", it.Body)
		}
		fmt.Fprintf(&b, "Item %s\n%s\n\n", it.ID, tools.Data("arxiv:"+it.Ref, licenceLabel(p), head.String()))
	}
	return b.String()
}

func licenceLabel(p *Paper) string { return cmp.Or(p.Licence, "no licence: the arXiv licence") }

// Record is the stored result of a pass on one item.
type Record struct {
	ID         string    `json:"id"`  // the item, ax://paper/ID#TAG
	Ref        string    `json:"ref"` // the object in its version, IDvN#LOCAL
	Pass       string    `json:"pass"`
	By         string    `json:"by"`    // the role at its version, as agent:m2-agent/understand@0.1.0
	Model      string    `json:"model"` // the model of the rung
	Class      string    `json:"class"` // the licence class of the version
	Was        string    `json:"was,omitempty"`
	Kind       string    `json:"kind,omitempty"`
	Confidence float64   `json:"confidence,omitempty"`
	Flags      []string  `json:"flags,omitempty"`
	Queue      string    `json:"queue,omitempty"`
	MSC        []string  `json:"msc,omitempty"`
	Concepts   []Concept `json:"concepts,omitempty"`
	Terms      []Term    `json:"terms,omitempty"`
}

// Concept is one concept of an item (AX7.3): a row of the inventory, a Wikidata item, or a name with the flag new-concept.
type Concept struct {
	Row      string   `json:"row,omitempty"`
	Wikidata string   `json:"wikidata,omitempty"`
	Name     string   `json:"name,omitempty"`
	Role     string   `json:"role"`
	Flags    []string `json:"flags,omitempty"`
}

// Term is a term that a definition defines, with the symbol that it introduces.
type Term struct {
	Term   string `json:"term"`
	Symbol string `json:"symbol,omitempty"`
}

// Path returns the path of the shard file of the records of the pass for the item.
func Path(pass, id string) string {
	return Dir + "/" + pass + "/" + inventory.MentionShard(id) + ".jsonl"
}

// ParseRecords reads the records of one shard file, one JSON object on each line. A field that Record does not have is an error.
func ParseRecords(data []byte) ([]Record, error) {
	var out []Record
	n := 0
	for line := range strings.Lines(string(data)) {
		n++
		if strings.TrimSpace(line) == "" {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(line))
		dec.DisallowUnknownFields()
		var r Record
		if err := dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		out = append(out, r)
	}
	return out, nil
}

// FormatRecords returns the records sorted by id, one JSON object on each line.
func FormatRecords(rs []Record) ([]byte, error) {
	rs = slices.Clone(rs)
	slices.SortFunc(rs, func(a, b Record) int { return strings.Compare(a.ID, b.ID) })
	var b strings.Builder
	for _, r := range rs {
		data, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return []byte(b.String()), nil
}

func read(ws *workspace.Workspace, p string) ([]Record, error) {
	data, err := ws.Read(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rs, err := ParseRecords(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return rs, nil
}

// classifySchema is the schema of the input of classify_items.
var classifySchema = json.RawMessage(`{"type":"object","properties":{"items":{"type":"array","items":{"type":"object","properties":{` +
	`"id":{"type":"string","description":"the id of the item, as ax://paper/2401.00001#K3F9"},` +
	`"kind":{"type":"string","enum":[` + quoted(Kinds) + `]},` +
	`"confidence":{"type":"number","minimum":0,"maximum":1},` +
	`"flags":{"type":"array","items":{"type":"string","enum":["` + ClaimInRemark + `","` + ConjectureCandidate + `"]}}},` +
	`"required":["id","kind","confidence"],"additionalProperties":false}}},"required":["items"],"additionalProperties":false}`)

// conceptsSchema is the schema of the input of tag_concepts.
var conceptsSchema = json.RawMessage(`{"type":"object","properties":{"items":{"type":"array","items":{"type":"object","properties":{` +
	`"id":{"type":"string","description":"the id of the item, as ax://paper/2401.00001#K3F9"},` +
	`"msc":{"type":"array","items":{"type":"string"},"maxItems":3,"description":"MSC2020 codes at five characters, such as 14J60, the first one first"},` +
	`"concepts":{"type":"array","items":{"type":"object","properties":{` +
	`"wikidata":{"type":"string","description":"the Wikidata item, such as Q193297, only when you know it exactly"},` +
	`"name":{"type":"string","description":"the common name of the concept"},` +
	`"role":{"type":"string","enum":["about","uses","defines"]}},"required":["name","role"],"additionalProperties":false}},` +
	`"terms":{"type":"array","items":{"type":"object","properties":{` +
	`"term":{"type":"string"},"symbol":{"type":"string","description":"the macro or the symbol, as \\mathcal{M}_g"}},"required":["term"],"additionalProperties":false},"description":"for a definition only: the terms that it defines"}},` +
	`"required":["id","msc","concepts"],"additionalProperties":false}}},"required":["items"],"additionalProperties":false}`)

func quoted(list []string) string { return `"` + strings.Join(list, `","`) + `"` }

var (
	mscCode  = regexp.MustCompile(`^[0-9]{2}[A-Z-][0-9xX]{2}$`)
	qid      = regexp.MustCompile(`^Q[1-9][0-9]*$`)
	maxName  = 80
	maxTerms = 8
)

// Tools returns the tools of the role for the paper: classify_items and tag_concepts. The tool of the other pass gives an error that names the tool of the pass.
func Tools(ws *workspace.Workspace, p *Paper, model, by string) []agent.Tool {
	t := Tool(ws, p, model, by)
	name, other := "tag_concepts", llm.Tool{Name: "tag_concepts", Description: "Give the MSC2020 codes and the concepts of each item. Only for the pass concepts.", Schema: conceptsSchema}
	if p.Pass == Concepts {
		name, other = "classify_items", llm.Tool{Name: "classify_items", Description: "Give the item kind of each item. Only for the pass classify.", Schema: classifySchema}
	}
	return []agent.Tool{t, agent.Func{Def: other, Fn: func(context.Context, *agent.Job, json.RawMessage) (agent.Result, error) {
		return agent.Result{}, fmt.Errorf("%s is not the tool of the pass %s. Use %s", name, p.Pass, t.Spec().Name)
	}}}
}

// Tool returns the tool of the pass of the paper: classify_items or tag_concepts. The tool checks each result, writes the records to the work tree, and says which results it left out and why. The records are by the agent id, for example agent:m2-agent/understand@0.1.0, with the model of the rung.
func Tool(ws *workspace.Workspace, p *Paper, model, by string) agent.Tool {
	write := tools.Write(ws)
	items := map[string]Item{}
	for _, it := range p.Items {
		items[it.ID] = it
	}
	def := llm.Tool{Name: "classify_items", Description: "Give the item kind of each item of the paper. Give all the items in one call.", Schema: classifySchema}
	if p.Pass == Concepts {
		def = llm.Tool{Name: "tag_concepts", Description: "Give the MSC2020 codes and the concepts of each item of the paper. Give all the items in one call.", Schema: conceptsSchema}
	}
	return agent.Func{
		Def: def,
		Fn: func(ctx context.Context, job *agent.Job, input json.RawMessage) (agent.Result, error) {
			var in struct{ Items []json.RawMessage }
			if err := json.Unmarshal(input, &in); err != nil {
				return agent.Result{}, err
			}
			var notes []string
			files := map[string][]Record{}
			done := map[string]bool{}
			for i, raw := range in.Items {
				r, why := record(p, items, raw, model, by)
				if r == nil {
					notes = append(notes, fmt.Sprintf("item %d: %s", i+1, why))
					continue
				}
				if done[r.ID] {
					notes = append(notes, fmt.Sprintf("item %d: %s is in the call twice, and the first one counts", i+1, r.ID))
					continue
				}
				if why != "" {
					notes = append(notes, fmt.Sprintf("item %d: %s", i+1, why))
				}
				done[r.ID] = true
				path := Path(p.Pass, r.ID)
				if _, ok := files[path]; !ok {
					have, err := read(ws, path)
					if err != nil {
						return agent.Result{}, err
					}
					files[path] = have
				}
				files[path] = slices.DeleteFunc(files[path], func(x Record) bool { return x.ID == r.ID })
				files[path] = append(files[path], *r)
			}
			if len(done) == 0 {
				return agent.Result{}, fmt.Errorf("stored no result of the %d. %s", len(in.Items), strings.Join(notes, ". "))
			}
			for _, path := range slices.Sorted(maps.Keys(files)) {
				text, err := FormatRecords(files[path])
				if err != nil {
					return agent.Result{}, err
				}
				args, err := json.Marshal(map[string]string{"path": path, "text": string(text)})
				if err != nil {
					return agent.Result{}, err
				}
				if _, err := write.Call(ctx, job, args); err != nil {
					return agent.Result{}, err
				}
			}
			var missing []string
			for _, it := range p.Items {
				if !done[it.ID] {
					missing = append(missing, it.ID)
				}
			}
			text := fmt.Sprintf("Stored the results of %d of %d items.", len(done), len(in.Items))
			if len(notes) > 0 {
				text += " " + strings.Join(notes, ". ") + "."
			}
			if len(missing) > 0 {
				text += " These items have no result yet: " + strings.Join(missing, ", ") + "."
			}
			return agent.Result{Text: text}, nil
		},
	}
}

// record returns the record of one result of the model, or nil and the reason when the result is not valid. A record with a reason left out a part of the result.
func record(p *Paper, items map[string]Item, raw json.RawMessage, model, by string) (*Record, string) {
	var in struct {
		ID         string
		Kind       string
		Confidence *float64
		Flags      []string
		MSC        []string
		Concepts   []struct{ Wikidata, Name, Role string }
		Terms      []Term
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err.Error()
	}
	it, ok := items[in.ID]
	if !ok {
		return nil, fmt.Sprintf("%q is not an item of the brief", in.ID)
	}
	r := &Record{ID: it.ID, Ref: it.Ref, Pass: p.Pass, By: by, Model: model, Class: p.Class}
	var left []string
	switch p.Pass {
	case Classify:
		switch {
		case !slices.Contains(Kinds, in.Kind):
			return nil, fmt.Sprintf("the kind %q of %s is not one of the kinds", in.Kind, it.ID)
		case in.Confidence == nil || *in.Confidence < 0 || *in.Confidence > 1:
			return nil, fmt.Sprintf("%s has no confidence from 0 to 1", it.ID)
		}
		r.Was, r.Kind, r.Confidence = it.Item, in.Kind, *in.Confidence
		for _, f := range in.Flags {
			k, _, _ := strings.Cut(in.Kind, "/")
			switch {
			case slices.Contains(r.Flags, f):
			case f == ClaimInRemark && (k == "remark" || strings.HasPrefix(it.Item, "remark")):
				r.Flags = append(r.Flags, f)
			case f == ConjectureCandidate && (k == "problem" || k == "conjecture"):
				r.Flags = append(r.Flags, f)
			default:
				left = append(left, fmt.Sprintf("the flag %q, which is not for the kind %s", f, in.Kind))
			}
		}
		if r.Confidence < ReviewBelow {
			r.Queue = ClassifyReview
		}
	case Concepts:
		for _, c := range in.MSC {
			c = strings.TrimSpace(c)
			switch {
			case !mscCode.MatchString(c):
				left = append(left, fmt.Sprintf("the code %q, which is not an MSC2020 code at five characters", c))
			case len(r.MSC) == 3:
				left = append(left, fmt.Sprintf("the code %s, because an item has at most three", c))
			case !slices.Contains(r.MSC, c):
				r.MSC = append(r.MSC, c)
			}
		}
		for _, c := range in.Concepts {
			name := clean(c.Name)
			switch {
			case !slices.Contains(Roles, c.Role):
				left = append(left, fmt.Sprintf("the concept %q, whose role %q is not about, uses or defines", name, c.Role))
				continue
			case name == "" || utf8.RuneCountInString(name) > maxName:
				left = append(left, fmt.Sprintf("a concept with no name or a name longer than %d characters", maxName))
				continue
			}
			x := Concept{Name: name, Role: c.Role}
			if w := strings.TrimSpace(c.Wikidata); w != "" {
				if qid.MatchString(w) {
					x.Wikidata = w
				} else {
					left = append(left, fmt.Sprintf("the Wikidata item %q of %q, which is not a QID", w, name))
				}
			}
			if x.Wikidata == "" {
				x.Flags = []string{NewConcept}
			}
			if !slices.ContainsFunc(r.Concepts, func(y Concept) bool { return y.Name == x.Name && y.Role == x.Role }) {
				r.Concepts = append(r.Concepts, x)
			}
		}
		k, _, _ := strings.Cut(it.Item, "/")
		for _, t := range in.Terms {
			t = Term{Term: clean(t.Term), Symbol: strings.TrimSpace(t.Symbol)}
			switch {
			case k != "definition":
				left = append(left, "the terms, because the item is not a definition")
			case p.Class == "record":
				left = append(left, "the terms, because math2 stores no words of a paper of the class record")
			case t.Term == "" || utf8.RuneCountInString(t.Term) > maxName || utf8.RuneCountInString(t.Symbol) > maxName:
				left = append(left, fmt.Sprintf("a term with no text or longer than %d characters", maxName))
			case len(r.Terms) == maxTerms:
				left = append(left, fmt.Sprintf("the terms after the first %d", maxTerms))
			default:
				r.Terms = append(r.Terms, t)
				continue
			}
			break
		}
		if len(r.MSC) == 0 && len(r.Concepts) == 0 {
			return nil, fmt.Sprintf("%s has no valid code and no valid concept", it.ID)
		}
	}
	if len(left) > 0 {
		return r, fmt.Sprintf("%s is stored, and the tool left out %s", it.ID, strings.Join(left, "; "))
	}
	return r, ""
}

// clean replaces each run of space with one space, and removes the space at the ends.
func clean(s string) string {
	return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
}

// Goal returns the goal of a run of the pass on the paper: the changes are inside the field permission set of understand, and each item of the pass has a record by the run.
func Goal(ws *workspace.Workspace, job *agent.Job, p *Paper, by string) agent.Goal {
	return agent.OnChanges(ws, func(_ context.Context, changes []workspace.Change) (bool, string, error) {
		if len(p.Items) == 0 {
			return true, "", nil
		}
		if len(changes) == 0 {
			return false, "the run has stored no result yet. Give the results with the tool of the pass", nil
		}
		if err := agent.Permit(job.Role, job.Target, changes); err != nil {
			return false, err.Error(), nil
		}
		files := map[string][]Record{}
		var missing []string
		for _, it := range p.Items {
			path := Path(p.Pass, it.ID)
			if _, ok := files[path]; !ok {
				rs, err := read(ws, path)
				if err != nil {
					return false, "", err
				}
				files[path] = rs
			}
			if !slices.ContainsFunc(files[path], func(r Record) bool { return r.ID == it.ID && r.By == by }) {
				missing = append(missing, it.ID)
			}
		}
		if len(missing) > 0 {
			return false, fmt.Sprintf("%d of the %d items have no result yet: %s", len(missing), len(p.Items), strings.Join(missing, ", ")), nil
		}
		return true, "", nil
	})
}
