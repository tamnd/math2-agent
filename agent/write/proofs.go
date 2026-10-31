package write

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/agent/understand"
	axImport "github.com/tamnd/math2-agent/importer/arxiv"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/workspace"
)

// The proofs from papers (corpus arxiv AX9.4). When the target is the node of a row of the inventory, the brief gives the proof mentions that the row has attached (AX5.2), in the order of the choice: the proof of the paper that is the main source of the node, a shorter proof, a proof that uses fewer items, and a proof of an open version. The brief gives the text of a proof only for the class open, with the attribution that rule L03 asks for. For any other class it gives the structure that the pass proof of understand gave (AX7.6): the steps as sentence ranges with their reasons as ids, and no text.

// LongProof is the number of lemmas of its own paper from which a proof is written as main steps, each lemma a node (AX9.4 item 4).
const LongProof = 10

// The limits of the proofs in the brief: their count, and the characters of the text of an open proof.
const (
	maxProofs     = 3
	maxProofChars = 8000
)

// PaperProof is one proof of the target in a paper of tamnd/arxiv.
type PaperProof struct {
	ID       string // the mention, ax://paper/ID#TAG
	Paper    string
	Version  int
	Locator  string
	Class    string
	Licence  string // the licence of the version in tamnd/arxiv, or "" when math2 has no metadata
	Main     bool   // the paper is a source of the node
	Length   int    // the number of sentences, from the pass proof, or 0 when it has not run
	Uses     []string
	Lemmas   []string // the statements of its own paper that the proof uses
	Cited    []inventory.CitedResult
	Steps    []understand.Step
	Methods  []string
	Text     string // the body, only for the class open
	Title    string
	Authors  string
	Recorded bool // the pass proof has a record of the proof
}

// arxivPaper matches the arXiv id of a paper in a source of a node.
var arxivPaper = regexp.MustCompile(`(?:arxiv:|ax://paper/|arxiv\.org/abs/)([0-9]{4}\.[0-9]{4,5}|[a-z-]+(?:\.[A-Z]{2})?/[0-9]{7})`)

// PaperProofs returns the proofs from papers of the node n, in the order of the choice. arxiv is the checkout of tamnd/arxiv, which gives the text of an open proof and the licence; with arxiv "" no proof has text. A node that no row of the inventory has as its node gives no proof.
func PaperProofs(ws *workspace.Workspace, n *node.Node, arxiv string) ([]PaperProof, error) {
	var row *inventory.Row
	err := inventory.WalkRows(ws.Dir, func(_ string, _ int, r inventory.Row) error {
		if row == nil && len(r.Proofs) > 0 && r.Outcome != nil && r.Outcome.Value == inventory.OutcomeNode && slices.Contains(r.Outcome.Target, n.ID) {
			row = &r
		}
		return nil
	})
	if err != nil || row == nil {
		return nil, err
	}
	ms := mentions{ws: ws, shards: map[string]map[string]*inventory.Mention{}}
	main := map[string]bool{}
	for _, s := range sourceStrings(n.Meta["sources"]) {
		for _, m := range arxivPaper.FindAllStringSubmatch(s, -1) {
			main[m[1]] = true
		}
	}
	papers := map[string]*paperData{}
	var out []PaperProof
	for _, id := range row.Proofs {
		m, err := ms.get(id)
		if err != nil {
			return nil, err
		}
		if m == nil || m.Source != axImport.Source {
			continue
		}
		pp := PaperProof{ID: id, Paper: axImport.PaperOf(id), Version: m.Version, Locator: m.Locator, Class: m.Class, Cited: m.CitesResult}
		pp.Main = main[pp.Paper]
		if m.Structure != nil {
			pp.Uses = m.Structure.Uses
		}
		rec, err := proofRecord(ws, id)
		if err != nil {
			return nil, err
		}
		if rec != nil && rec.Proof != nil {
			pp.Recorded = true
			pp.Steps, pp.Methods = rec.Proof.Steps, rec.Proof.Methods
			for _, s := range rec.Proof.Steps {
				pp.Length = max(pp.Length, s.To)
			}
		}
		if arxiv != "" {
			pd := papers[pp.Paper]
			if pd == nil {
				if pd, err = readPaper(arxiv, pp.Paper); err != nil {
					return nil, err
				}
				papers[pp.Paper] = pd
			}
			pp.Title, pp.Authors, pp.Licence = pd.title, pd.authors, pd.licence
			if pd.class != "" {
				pp.Class = pd.class
			}
			if pp.Class == "open" {
				pp.Text = pd.bodies[id]
			}
		}
		if pp.Lemmas, err = lemmas(&ms, pp); err != nil {
			return nil, err
		}
		out = append(out, pp)
	}
	slices.SortStableFunc(out, func(a, b PaperProof) int {
		return cmp.Or(
			-cmpBool(a.Main, b.Main),
			cmp.Compare(length(a), length(b)),
			cmp.Compare(len(a.Uses), len(b.Uses)),
			-cmpBool(a.Class == "open", b.Class == "open"),
			strings.Compare(a.ID, b.ID),
		)
	})
	return out, nil
}

// length is the length of a proof for the choice: its sentences, or a large number when the pass proof has not run.
func length(p PaperProof) int {
	if p.Length == 0 {
		return 1 << 30
	}
	return p.Length
}

func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// lemmas returns the statements of the paper of the proof that the proof uses, by its edges and by the reasons of its steps.
func lemmas(ms *mentions, p PaperProof) ([]string, error) {
	ids := slices.Clone(p.Uses)
	for _, s := range p.Steps {
		if s.Reason == "item" {
			ids = append(ids, s.Ref)
		}
	}
	slices.Sort(ids)
	var out []string
	for _, id := range slices.Compact(ids) {
		if axImport.PaperOf(id) != p.Paper {
			continue
		}
		m, err := ms.get(id)
		if err != nil {
			return nil, err
		}
		if m != nil && m.Kind == "statement" {
			out = append(out, id)
		}
	}
	return out, nil
}

// mentions reads the arXiv mentions of the tree one shard at a time.
type mentions struct {
	ws     *workspace.Workspace
	shards map[string]map[string]*inventory.Mention
}

func (s *mentions) get(id string) (*inventory.Mention, error) {
	k := inventory.MentionShard(id)
	shard, ok := s.shards[k]
	if !ok {
		shard = map[string]*inventory.Mention{}
		data, err := s.ws.Read(inventory.MentionsDir + "/" + axImport.Source + "/" + k + ".jsonl")
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if err == nil {
			ms, err := inventory.ParseMentions(data)
			if err != nil {
				return nil, err
			}
			for i := range ms {
				shard[ms[i].ID] = &ms[i]
			}
		}
		s.shards[k] = shard
	}
	return shard[id], nil
}

// proofRecord returns the record of the pass proof of the proof, or nil.
func proofRecord(ws *workspace.Workspace, id string) (*understand.Record, error) {
	p := understand.Path(understand.Proof, id)
	data, err := ws.Read(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rs, err := understand.ParseRecords(data)
	if err != nil {
		return nil, fmt.Errorf("write: %s: %w", p, err)
	}
	for i := range rs {
		if rs[i].ID == id && rs[i].Failed == "" {
			return &rs[i], nil
		}
	}
	return nil, nil
}

type paperData struct {
	title, authors, licence, class string
	bodies                         map[string]string // the bodies of the proofs of the latest version, by their mention, for the class open
}

func readPaper(root, id string) (*paperData, error) {
	meta, objs, err := axImport.ReadPaper(root, id)
	if err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	latest, v := axImport.Latest(objs)
	pd := &paperData{class: "record", bodies: map[string]string{}}
	if meta != nil {
		pd.title = strings.Join(strings.Fields(meta.Title), " ")
		pd.authors = meta.AuthorNames()
		pd.licence = meta.Licence(v)
		pd.class = axImport.ClassOf(pd.licence)
	}
	if pd.class != "open" {
		return pd, nil
	}
	for _, o := range latest {
		if o.Tag != "" && o.Item == "proof" {
			pd.bodies[axImport.MentionID(id, o.Tag)] = strings.TrimSpace(o.BodyMD)
		}
	}
	return pd, nil
}

// sourceStrings returns the string values in the sources of a node.
func sourceStrings(v any) []string {
	switch v := v.(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, x := range v {
			out = append(out, sourceStrings(x)...)
		}
		return out
	case map[string]any:
		var out []string
		for _, x := range v {
			out = append(out, sourceStrings(x)...)
		}
		return out
	}
	return nil
}

// briefProofs writes the part of the brief on the proofs from papers.
func briefProofs(b *strings.Builder, ps []PaperProof) {
	if len(ps) == 0 {
		return
	}
	fmt.Fprintf(b, "The row of the target has %d proofs in arXiv papers. They are in the order in which to take them: the proof of the paper that is a source of the node, a shorter proof, a proof that uses fewer items, and a proof of an open version. Write the proof from the first one that you can use. Each step of the proof has its reason as a link to a node, or as a source with its locator when the result is not a node.\n\n", len(ps))
	for i, p := range ps[:min(len(ps), maxProofs)] {
		fmt.Fprintf(b, "Proof %d: %s, in %s (the class %s).\n", i+1, p.ID, p.Locator, p.Class)
		if len(p.Lemmas) >= LongProof {
			fmt.Fprintf(b, "The proof uses %d statements of its own paper. Write it as a proof of the main steps, make a new node for each of these statements, and give the proof node a uses edge to each of them: %s.\n", len(p.Lemmas), strings.Join(p.Lemmas, ", "))
		} else if len(p.Lemmas) > 0 {
			fmt.Fprintf(b, "It uses these statements of its paper: %s.\n", strings.Join(p.Lemmas, ", "))
		}
		if others := outside(p); len(others) > 0 {
			fmt.Fprintf(b, "It uses results of works that are not in the corpus. Give each as a source with its locator, not as a node: %s.\n", strings.Join(others, "; "))
		}
		if len(p.Methods) > 0 {
			fmt.Fprintf(b, "Its methods: %s.\n", strings.Join(p.Methods, ", "))
		}
		if p.Class == "open" && p.Text != "" {
			fmt.Fprintf(b, "The version is open. Adapt the proof into numbered steps in the house notation and in your own words, and add the attribution line: %s.\n%s\n\n", attribution(p), tools.Data("arxiv:"+p.ID, p.Licence, cut(p.Text, maxProofChars)))
			continue
		}
		if len(p.Steps) > 0 {
			b.WriteString("The version is not open, thus the brief has no text of it, only its steps as ranges of its sentences and their reasons:\n")
			for j, s := range p.Steps {
				fmt.Fprintf(b, "%d. sentences %d to %d: %s", j+1, s.From, s.To, s.Reason)
				if s.Ref != "" {
					fmt.Fprintf(b, " %s", s.Ref)
				}
				b.WriteString("\n")
			}
		} else {
			b.WriteString("The brief has neither its text nor its steps.\n")
		}
		fmt.Fprintf(b, "Write your own proof from these, from the nodes and from what you know. If you cannot, leave the proof out: the target stays at W1, and its sources say that the proof is in %s.\n\n", p.Locator)
	}
}

// outside returns the cited results of the proof that are not mentions of the corpus, with their locators.
func outside(p PaperProof) []string {
	var out []string
	for _, c := range p.Cited {
		if strings.HasPrefix(c.To, "ax://") {
			continue
		}
		out = append(out, strings.TrimSpace(c.To+" "+c.Locator))
	}
	return out
}

// attribution is the line of rule L03 for a section adapted from an open proof.
func attribution(p PaperProof) string {
	var parts []string
	for _, s := range []string{p.Authors, p.Title, fmt.Sprintf("arXiv:%sv%d", p.Paper, p.Version), p.Licence} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ") + ", adapted"
}

// cut returns the first n bytes of s, at a line end when it can and never inside a character.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	t := s[:n]
	if i := strings.LastIndexByte(t, '\n'); i > n/2 {
		t = t[:i]
	}
	return t + "\n..."
}
