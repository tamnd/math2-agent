// Package understand has the goal, the brief and the tools of the understand role (corpus arxiv AX07).
//
// A job of the role is one pass on one arXiv paper, thus the paper is the cached part of the prompt and each item of the paper shares it. The target is arxiv:ID/PASS, for example arxiv:2401.00001/classify. The passes that are written: classify (AX7.2) and concepts (AX7.3) of tier T1, prose (AX7.4) of tier T2, unfold (AX7.5) of tier T3, and proof (AX7.6) and summary (AX7.7) of tier T4. Each pass has one file, with its tool, what it reads, its part of the brief and the check of each result. The prompt of a pass is passes/PASS.md of the role, and the schema of its tool is schema/understand/PASS.v1.json. The brief gives the items of the latest version that the pass reads, from the analysis plane of a checkout of tamnd/arxiv, in data blocks marked with the licence of the version. The model gives its result with the tool of the pass, and the tool writes one record for each item to records/understand/arxiv/PASS/SHARD.jsonl.
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
	uschema "github.com/tamnd/math2-agent/schema/understand"
	"github.com/tamnd/math2-agent/workspace"
)

// The passes of tier T1. The pass prose of tier T2 is in prose.go, the pass unfold of tier T3 in unfold.go, and the passes proof and summary of tier T4 in proof.go and summary.go.
const (
	Classify = "classify"
	Concepts = "concepts"
)

// Passes are the passes that are written, in the order of AX7.1.
var Passes = []string{Classify, Concepts, Prose, Unfold, Proof, Summary}

// Dir is the directory of the records of the passes, with one directory for each pass.
const Dir = agent.UnderstandDir

// Failed is the prefix of the mark of an item whose result is not valid twice (AX7.1). The mark of the item is Failed and the pass, as model-failed: classify.
const Failed = "model-failed: "

// pass is the code of one pass, in the file of the pass.
type pass struct {
	tool  string // the tool that gives the results of the pass
	about string // the description of the tool
	other string // the description of the tool in a run of another pass
	chars int    // the limit of the text that the model reads of the body of each item
	reads func(item string) bool
	brief func(b *strings.Builder, p *Paper)                                                  // the work of the pass, at the start of the brief
	check func(r *Record, p *Paper, it Item, raw json.RawMessage) (left []string, err string) // fills the record from one result, or gives why the result is not valid
	// after runs on each valid record before it is stored, or is nil. A reason sends the result back to the model once, as a result that is not valid; the second time the record is stored as it is.
	after func(ctx context.Context, r *Record, p *Paper) (why string, err error)
	load  func(root string, p *Paper, latest []axImport.Object) error // reads what else the pass needs from the checkout, after the items, or is nil
}

// passes has the code of each pass of Passes.
var passes = map[string]pass{Classify: classifyPass, Concepts: conceptsPass, Prose: prosePass, Unfold: unfoldPass, Proof: proofPass, Summary: summaryPass}

// schemaOf returns the schema of the input of the tool of the pass, from schema/understand.
func schemaOf(name string) json.RawMessage {
	s := uschema.Of(name)
	if s == nil {
		panic("understand: the pass " + name + " has no schema " + uschema.File(name))
	}
	return s
}

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
	// Sections is the running text of each section for the pass prose, by the id of its item, from SetText.
	Sections map[string]Section
	// Check is the check of the prose items by a model of another family, or nil when math2 has none.
	Check ProseCheck
	// Context is what the model reads with each item, by the id of the item, for the pass unfold.
	Context map[string][]Context
	// Tags are the item kinds of the items of the paper with a tag, by their id, for the passes unfold and proof.
	Tags map[string]string
	// Steps is the number of sentences of each proof, by its id, and Orphans are the proofs with no item that they prove, for the pass proof.
	Steps   map[string]int
	Orphans map[string]bool
	// UnfoldCheck is the check of the unfolded statements by a model of another family, or nil when math2 has none.
	UnfoldCheck UnfoldCheck
}

// Reads reports whether the pass reads an object of the item kind.
func Reads(name, item string) bool {
	ps, ok := passes[name]
	return ok && ps.reads(item)
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
	limit := passes[t.Pass].chars
	for _, o := range latest {
		if o.Tag == "" || !Reads(t.Pass, o.Item) {
			continue
		}
		p.Items = append(p.Items, Item{
			ID: axImport.MentionID(t.Paper, o.Tag), Ref: fmt.Sprintf("%sv%d#%s", t.Paper, v, o.Local),
			Item: o.Item, Env: o.Env, Title: o.Title, Number: o.Number, Head: o.Head, Body: cut(o.BodyMD, limit),
		})
	}
	if load := passes[t.Pass].load; load != nil {
		if err := load(root, p, latest); err != nil {
			return nil, err
		}
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

// Brief returns the first message of a run of the pass on the paper. The prompt is the prompt of the pass, passes/PASS.md of the role, which comes after the line of the job.
func Brief(job *agent.Job, p *Paper, prompt string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Job %s: the pass %s on the arXiv paper %sv%d. The goal of the job: %s.\n\n", job.ID, p.Pass, p.ID, p.Version, strings.TrimSuffix(job.Goal, "."))
	if prompt = strings.TrimSpace(prompt); prompt != "" {
		fmt.Fprintf(&b, "%s\n\n", prompt)
	}
	passes[p.Pass].brief(&b, p)
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
		if it.Item == "section" {
			fmt.Fprintf(&head, "the running text of %s, one numbered sentence on each line\n\n%s", it.Title, it.Body)
		} else {
			fmt.Fprintf(&head, "kind now: %s", it.Item)
			for _, f := range [][2]string{{"environment", it.Env}, {"title", it.Title}, {"number", it.Number}, {"head", it.Head}} {
				if f[1] != "" {
					fmt.Fprintf(&head, "\n%s: %s", f[0], f[1])
				}
			}
			if it.Body != "" {
				fmt.Fprintf(&head, "\n\n%s", it.Body)
			}
		}
		fmt.Fprintf(&b, "Item %s\n%s\n\n", it.ID, tools.Data("arxiv:"+it.Ref, licenceLabel(p), head.String()))
		for _, c := range p.Context[it.ID] {
			fmt.Fprintf(&b, "Context of %s: %s, %s\n%s\n\n", it.ID, c.ID, c.Why, tools.Data("arxiv:"+c.Ref, licenceLabel(p), c.String()))
		}
	}
	return b.String()
}

func licenceLabel(p *Paper) string { return cmp.Or(p.Licence, "no licence: the arXiv licence") }

// Record is the stored result of a pass on one item.
type Record struct {
	ID         string       `json:"id"`  // the item, ax://paper/ID#TAG
	Ref        string       `json:"ref"` // the object in its version, IDvN#LOCAL
	Pass       string       `json:"pass"`
	By         string       `json:"by"`    // the role at its version, as agent:m2-agent/understand@0.1.0
	Model      string       `json:"model"` // the model of the rung
	Class      string       `json:"class"` // the licence class of the version
	Was        string       `json:"was,omitempty"`
	Kind       string       `json:"kind,omitempty"`
	Confidence float64      `json:"confidence,omitempty"`
	Flags      []string     `json:"flags,omitempty"`
	Queue      string       `json:"queue,omitempty"`
	Failed     string       `json:"failed,omitempty"` // Failed and the pass, when the result of the model was not valid twice
	MSC        []string     `json:"msc,omitempty"`
	Concepts   []Concept    `json:"concepts,omitempty"`
	Terms      []Term       `json:"terms,omitempty"`
	Prose      []ProseItem  `json:"prose,omitempty"`
	Unfold     *Unfolding   `json:"unfold,omitempty"`
	Proof      *ProofResult `json:"proof,omitempty"`
	Summary    string       `json:"summary,omitempty"`
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
		data, err := json.Marshal(r) // #nosec G117 -- Pass is the name of a pass of the run, not a password
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

// Tools returns the tools of the role for the paper: the tool of each pass. The tool of another pass gives an error that names the tool of the pass.
func Tools(ws *workspace.Workspace, p *Paper, model, by string) []agent.Tool {
	t := Tool(ws, p, model, by)
	out := []agent.Tool{t}
	for _, name := range Passes {
		ps := passes[name]
		if name == p.Pass {
			continue
		}
		out = append(out, agent.Func{Def: llm.Tool{Name: ps.tool, Description: ps.other, Schema: schemaOf(name)}, Fn: func(context.Context, *agent.Job, json.RawMessage) (agent.Result, error) {
			return agent.Result{}, fmt.Errorf("%s is not the tool of the pass %s. Use %s", ps.tool, p.Pass, t.Spec().Name)
		}})
	}
	return out
}

// Tool returns the tool of the pass of the paper, with the schema of the pass in schema/understand. The tool checks each result, writes the records to the work tree, and says which results it left out and why. A result of an item that is not valid for the second time in the run gives the item a record with the mark model-failed: PASS, and the job goes on (AX7.1). The records are by the agent id, for example agent:m2-agent/understand@0.1.0, with the model of the rung.
func Tool(ws *workspace.Workspace, p *Paper, model, by string) agent.Tool {
	write := tools.Write(ws)
	ps := passes[p.Pass]
	items := map[string]Item{}
	for _, it := range p.Items {
		items[it.ID] = it
	}
	bad := map[string]int{}     // the results of each item that were not valid, in the run
	stored := map[string]bool{} // the items with a valid result of the run
	return agent.Func{
		Def: llm.Tool{Name: ps.tool, Description: ps.about, Schema: schemaOf(p.Pass)},
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
				if r.Failed != "" {
					if bad[r.ID]++; bad[r.ID] < 2 || stored[r.ID] {
						notes = append(notes, fmt.Sprintf("item %d: %s", i+1, why))
						continue
					}
					notes = append(notes, fmt.Sprintf("item %d: %s, and it is the second result of the item that is not valid, thus the item is stored as %s", i+1, why, r.Failed))
				} else if why != "" {
					notes = append(notes, fmt.Sprintf("item %d: %s", i+1, why))
				}
				if ps.after != nil && r.Failed == "" {
					again, err := ps.after(ctx, r, p)
					if err != nil {
						return agent.Result{}, err
					}
					if again != "" {
						if bad[r.ID]++; bad[r.ID] < 2 && !stored[r.ID] {
							notes = append(notes, fmt.Sprintf("item %d: %s", i+1, again))
							continue
						}
						notes = append(notes, fmt.Sprintf("item %d: %s, and it is the second time, thus the item is stored in the queue %s", i+1, again, r.Queue))
					}
				}
				done[r.ID] = true
				stored[r.ID] = stored[r.ID] || r.Failed == ""
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

// record returns the record of one result of the model, or nil and the reason when the result names no item of the brief. When the result of an item is not valid, the record has only the mark Failed, with the reason. A valid record with a reason left out a part of the result.
func record(p *Paper, items map[string]Item, raw json.RawMessage, model, by string) (*Record, string) {
	var in struct{ ID string }
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err.Error()
	}
	it, ok := items[in.ID]
	if !ok {
		return nil, fmt.Sprintf("%q is not an item of the brief", in.ID)
	}
	r := &Record{ID: it.ID, Ref: it.Ref, Pass: p.Pass, By: by, Model: model, Class: p.Class}
	left, why := passes[p.Pass].check(r, p, it, raw)
	if why != "" {
		return &Record{ID: it.ID, Ref: it.Ref, Pass: p.Pass, By: by, Model: model, Class: p.Class, Failed: Failed + p.Pass}, why
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
