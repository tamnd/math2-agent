// Package pack compiles a volume of math2 into its outputs, and checks whether the volume is ready (corpus K11).
//
// A volume is a file volumes/<id>.yaml in the math2 tree: a selection of nodes with an order, some editorial text and a set of choices. [Pack] reads the tree and makes a [Document], the typed document tree of corpus K11.4. The tree holds the nodes, the glue, the numbering and the references. Each back end writes the tree in one format, and no back end changes the content. The packer adds no mathematics: each definition, statement, proof and example is a node, and the volume shows the main section of the node as the node says it.
//
// Pack makes the sequence in the steps of corpus K11.2. It takes the closure of the targets over the uses edges, and stops at the nodes that the volume assumes. A concept goes in with the definition that the volume chooses, and the other definitions come after it as equivalent characterizations. When the volume has no outline, Pack orders the nodes so that each node comes after its prerequisites. Among the orders with no forward reference, it keeps the nodes of one section and of one chapter together. The chapters come from the topics of the nodes, or from their MSC areas when they have no topic. The sections come from the concepts. The ties go to the order of the curricula, then to the MSC code, then to the tag. Pack never moves a node of an outline: [Document.Check] reports each forward reference.
//
// [Document.Lock] gives the lock file, volumes/<id>.lock. [Document.HTML], [Document.LaTeX] and [Document.PreTeXt] give the outputs, and [Document.Output] gives the output of one format. [Document.Check] runs the pack checks PC1 to PC10 of corpus K11.6, with no model and no network.
package pack

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/card"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/msc"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/notation"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/tag"
)

// DefaultSite is the URL of the site of math2. A citation of a node outside the volume links to its page there.
const DefaultSite = "https://math2.tamnd.com"

// Draft is the release of a volume that pins no release.
const Draft = "draft"

// Options are the options of Pack.
type Options struct {
	// Build is a build output directory with status.json and writing.json. Empty means that Pack computes the levels from the tree. With no build, no node has a formal level, because the Lean records are in the build.
	Build string
	// Site is the URL of the site, for the citations. Empty means DefaultSite.
	Site string
}

// Document is the typed document tree of a volume (corpus K11.4).
type Document struct {
	Volume   *Volume
	Release  string // the release that the edition pins, or Draft
	Preface  []Glue
	Chapters []*Chapter

	Assumed     []Ref           // the nodes that the reader knows, in the order of the volume file
	Cited       []Ref           // the nodes outside the volume that a body links to, in tag order
	Sources     []Source        // the works that the items cite, in the order of the first citation
	Conventions []ConventionUse // the conventions of the nodes in the volume, in the order of the register
	Notation    []NotationUse   // the macros that the nodes use, in reading order (corpus K10.4)
	Attribution Attribution

	site     string
	tree     *tree
	homes    map[string]*Item // the item that shows each tag of the volume. A concept that a definition stands for maps to the definition
	targets  []string         // the targets as tags, in order
	missing  []missingRef     // the references that end at no node
	problems []problem        // what Pack found while it made the sequence, for the checks
}

// Chapter is a chapter of the document.
type Chapter struct {
	Number   int
	ID       string // the place of its glue
	Title    string
	Glue     []Glue
	Sections []*Section
}

// Section is a section of a chapter.
type Section struct {
	Number string // for example 3.2
	ID     string
	Title  string
	Glue   []Glue
	Items  []*Item
}

// Item is one node in the document, at its home.
type Item struct {
	Number string // for example 3.4. A proof has no number
	Tag    string
	Kind   string
	Role   string
	Name   string
	Path   string
	Label  string // Definition, Theorem, Example, and so on. A statement below E2 has the label Claim
	// Concept is the tag of the concept that a definition stands for in the volume, or "".
	Concept string
	// Characterizes is the tag of the definition that the volume chooses, for another definition of the same concept, or "".
	Characterizes string
	Writing       card.Level
	Evidence      status.Level
	// Claim is the evidence level in words for a statement that the volume cannot show as a theorem, because it is below E2, for example "claimed". It is "" for a statement at E2 or better.
	Claim   string
	Main    string // the Markdown of the main section
	Remarks string // the Markdown of the section Remarks, or ""
	Sources []Citation
	Proofs  []*Item
	Proof   bool // the item is a proof, in the Proofs of its statement

	position int      // the place of the item in reading order
	sections []string // the sections of the node that the item shows
}

// Ref is a node that the document names but does not show.
type Ref struct {
	Tag  string
	Name string
	URI  string // m2://TAG
	URL  string // the page of the node on the site
}

// ConventionUse is a convention of the register that a node in the volume depends on.
type ConventionUse struct {
	ID      string
	Option  string // the option of the volume
	Default bool   // the option is the default of math2
	Differ  []Difference
}

// Difference is a node that uses another option of a convention.
type Difference struct {
	Tag, Name, Option, Note string
}

// NotationUse is a macro of the notation register that a node in the volume uses.
type NotationUse struct {
	ID         string // the id of the register entry
	Macro      string
	Args       int
	LaTeX      string // the expansion, or the expansion of the variant of the volume
	Variant    string // the variant of the volume, or ""
	Reads      string
	Convention string
	Introduced string // the number of the item that introduces it, or ""
	position   int
}

// Attribution is the attribution page (corpus K11.7). The people come first.
type Attribution struct {
	People []Credit // by the number of contributions, then by id
	Agents []string // the agent runs, as AGENT run RUN, in order
	NoLog  []string // the tags of the nodes with no provenance log
}

// Credit is the contributions of one person to the volume.
type Credit struct {
	Person  string
	Edited  bool // an editor of the volume
	Nodes   int  // the nodes that the person changed
	Reviews int  // the reviews of formal bindings
	Glue    int  // the parts of the glue that the person wrote or confirmed
}

// String returns the credit in words, for example github:ada, editor, 3 nodes.
func (c Credit) String() string {
	parts := []string{c.Person}
	if c.Edited {
		parts = append(parts, "editor")
	}
	for _, n := range []struct {
		n         int
		one, many string
	}{
		{c.Nodes, "node", "nodes"},
		{c.Reviews, "review", "reviews"},
		{c.Glue, "part of the editorial text", "parts of the editorial text"},
	} {
		switch {
		case n.n == 1:
			parts = append(parts, "1 "+n.one)
		case n.n > 1:
			parts = append(parts, fmt.Sprintf("%d %s", n.n, n.many))
		}
	}
	return strings.Join(parts, ", ")
}

// licence is the first sentence of the attribution page (corpus K11.7).
const licence = "The prose of this volume is under the licence CC BY-SA 4.0, as the prose of math2 is."

// problem is a finding of Pack for a check.
type problem struct {
	check string
	tag   string // the node, or ""
	key   string // the key of the volume file, when there is no node
	msg   string
}

// missingRef is a reference to a tag that is not a node.
type missingRef struct {
	from, to, how string
}

// tree is what Pack reads from the working tree.
type tree struct {
	root    string
	nodes   map[string]*node.Node
	graph   *graph.Graph
	writing map[string]card.Result
	status  map[string]status.Status
	reg     *notation.Register
	regErr  error
	conv    []*notation.Convention
	convErr error
	topics  *msc.Map
	courses map[string][]string // the node tags of each curriculum course, as ID/COURSE, in teaching order
	curPos  map[string]int      // the first place of each tag in a curriculum
	logs    map[string][]prov.Line
	macros  map[string]string   // the KaTeX macros of macros/m2.katex.json
	sty     []byte              // macros/m2.sty, or nil
	reviews map[string][]string // the reviewers of the bindings of each node
}

// Pack reads the working tree at root and makes the document of the volume.
func Pack(root string, v *Volume, opt Options) (*Document, error) {
	t, err := readTree(root, opt)
	if err != nil {
		return nil, err
	}
	return t.pack(v, opt)
}

func readTree(root string, opt Options) (*tree, error) {
	nodes, err := node.Load(root)
	if err != nil {
		return nil, err
	}
	t := &tree{root: root, nodes: map[string]*node.Node{}, graph: graph.Build(nodes), writing: map[string]card.Result{}, status: map[string]status.Status{}, curPos: map[string]int{}, courses: map[string][]string{}, reviews: map[string][]string{}}
	for _, n := range nodes {
		t.nodes[n.ID] = n
	}
	if err := t.readLevels(nodes, opt.Build); err != nil {
		return nil, err
	}
	t.reg, t.regErr = notation.Load(root)
	t.conv, t.convErr = notation.LoadConventions(root)
	if t.topics, err = msc.Read(root); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err := t.readCurricula(); err != nil {
		return nil, err
	}
	if t.logs, err = prov.ReadAll(root); err != nil {
		return nil, err
	}
	switch data, err := os.ReadFile(filepath.Join(root, "macros", "m2.sty")); { // #nosec G304 -- the file is in the working tree.
	case err == nil:
		t.sty = data
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	switch data, err := os.ReadFile(filepath.Join(root, "macros", "m2.katex.json")); { // #nosec G304 -- the file is in the working tree.
	case err == nil:
		if err := json.Unmarshal(data, &t.macros); err != nil {
			return nil, fmt.Errorf("pack: macros/m2.katex.json: %w", err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	return t, nil
}

// readLevels reads the writing level and the status of each node from the build, or computes them from the tree.
func (t *tree) readLevels(nodes []*node.Node, out string) error {
	var w *build.Levels
	var st []status.Status
	if out != "" {
		data, err := os.ReadFile(filepath.Join(out, build.WritingFile)) // #nosec G304 -- the caller names the build.
		switch {
		case err == nil:
			w = &build.Levels{}
			if err := json.Unmarshal(data, w); err != nil {
				return fmt.Errorf("pack: %s: %w", build.WritingFile, err)
			}
		case !errors.Is(err, fs.ErrNotExist):
			return err
		}
		if st, err = build.ReadStatus(out); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if w == nil {
		cards, err := card.Load(t.root)
		if err != nil {
			return err
		}
		if w, err = build.WritingLevels(t.root, cards, nodes); err != nil {
			return err
		}
	}
	if st == nil {
		in := status.Input{Nodes: nodes}
		var err error
		if in.Reviews, err = status.LoadReviews(t.root); err != nil {
			return err
		}
		if in.Reviewers, err = status.LoadReviewers(t.root); err != nil {
			return err
		}
		if in.Authors, err = status.LoadAuthors(t.root); err != nil {
			return err
		}
		st = status.Compute(in)
	}
	for _, r := range w.Nodes {
		t.writing[r.Tag] = r
	}
	for _, s := range st {
		t.status[s.Tag] = s
		for _, b := range s.Bindings {
			for _, r := range b.Reviews {
				if r.State == status.Counts && r.Reviewer != "" {
					t.reviews[s.Tag] = append(t.reviews[s.Tag], r.Reviewer)
				}
			}
		}
	}
	return nil
}

// curriculaDir holds the curricula (corpus K10.3).
const curriculaDir = "topics/curricula"

// readCurricula reads the node of each item of each curriculum course.
func (t *tree) readCurricula() error {
	files, err := filepath.Glob(filepath.Join(t.root, filepath.FromSlash(curriculaDir), "*.yaml"))
	if err != nil {
		return err
	}
	slices.Sort(files)
	pos := 0
	for _, f := range files {
		data, err := os.ReadFile(f) // #nosec G304 -- the file is in the working tree.
		if err != nil {
			return err
		}
		var c struct {
			Curriculum string `yaml:"curriculum"`
			Courses    []struct {
				Course string `yaml:"course"`
				Items  []struct {
					Node string `yaml:"node"`
				} `yaml:"items"`
			} `yaml:"courses"`
		}
		if err := yaml.Unmarshal(data, &c); err != nil {
			return fmt.Errorf("pack: %s: %w", filepath.Base(f), err)
		}
		id := c.Curriculum
		if id == "" {
			id = strings.TrimSuffix(filepath.Base(f), ".yaml")
		}
		for _, course := range c.Courses {
			key := id + "/" + course.Course
			for _, it := range course.Items {
				if it.Node == "" {
					continue
				}
				t.courses[key] = append(t.courses[key], it.Node)
				if _, ok := t.curPos[it.Node]; !ok {
					t.curPos[it.Node] = pos
				}
				pos++
			}
		}
	}
	return nil
}

// packer makes one document.
type packer struct {
	t       *tree
	v       *Volume
	d       *Document
	assumed map[string]bool
	in      map[string]bool     // the tags in the volume: the items, the proofs and the concepts that a definition stands for
	seq     []string            // the tags of the items in the order that the closure found them
	proofs  map[string][]string // the proofs that the volume shows for each statement
	chosen  map[string]string   // the definition that the volume uses, by concept
	alt     map[string]string   // the chosen definition, by each other definition of its concept
	stands  map[string]string   // the concept that a definition stands for, by definition
	proofOf map[string]string   // the item that shows each proof
}

func (t *tree) pack(v *Volume, opt Options) (*Document, error) {
	site := strings.TrimSuffix(cmp.Or(opt.Site, DefaultSite), "/")
	d := &Document{Volume: v, Release: cmp.Or(v.Release, Draft), site: site, tree: t, homes: map[string]*Item{}}
	p := &packer{t: t, v: v, d: d, assumed: map[string]bool{}, in: map[string]bool{}, proofs: map[string][]string{}, chosen: map[string]string{}, alt: map[string]string{}, stands: map[string]string{}, proofOf: map[string]string{}}
	for _, a := range v.Assumes {
		p.assumed[a] = true
		if n := t.nodes[a]; n != nil {
			d.Assumed = append(d.Assumed, p.ref(a))
		} else {
			p.problem("PC6", "", "assumes", "the volume assumes %s, and it is not a node", a)
		}
	}
	d.targets = p.resolveTargets()
	if len(v.Outline) > 0 {
		p.fromOutline()
	} else {
		p.collect(d.targets)
		p.order()
	}
	p.number()
	p.glue()
	p.references()
	p.sources()
	p.notation()
	p.conventions()
	p.attribution()
	return d, nil
}

func (p *packer) problem(check, tag, key, format string, args ...any) {
	p.d.problems = append(p.d.problems, problem{check: check, tag: tag, key: key, msg: fmt.Sprintf(format, args...)})
}

// ref returns the reference to a node by its name, its URI and its page on the site.
func (p *packer) ref(t string) Ref { return p.d.ref(t) }

// resolveTargets returns the tags of the targets. A topic stands for its nodes, a curriculum and a course for the nodes of their items, and msc:CODE for the nodes with a code under CODE.
func (p *packer) resolveTargets() []string {
	var out []string
	for _, x := range p.v.Targets {
		switch {
		case strings.HasPrefix(x, "curriculum:"):
			id := strings.TrimPrefix(x, "curriculum:")
			var keys []string
			for k := range p.t.courses {
				if k == id || strings.HasPrefix(k, id+"/") {
					keys = append(keys, k)
				}
			}
			if len(keys) == 0 {
				p.problem("PC6", "", "targets", "the target %s names no curriculum or course with nodes", x)
			}
			slices.Sort(keys)
			for _, k := range keys {
				out = append(out, p.t.courses[k]...)
			}
		case strings.HasPrefix(x, "msc:"):
			code := strings.TrimPrefix(x, "msc:")
			if !msc.Valid(code) {
				p.problem("PC6", "", "targets", "the target %s has no MSC2020 code", x)
				continue
			}
			var tags []string
			for _, gn := range p.t.graph.Nodes {
				if gn.Retired {
					continue
				}
				if slices.ContainsFunc(p.t.nodes[gn.Tag].List("msc"), func(c string) bool { return under(c, code) }) {
					tags = append(tags, gn.Tag)
				}
			}
			if len(tags) == 0 {
				p.problem("PC6", "", "targets", "no node has an MSC code under %s", code)
			}
			out = append(out, tags...)
		case p.t.nodes[x] == nil:
			p.problem("PC6", "", "targets", "the target %s is not a node", x)
		default:
			out = append(out, x)
		}
	}
	return out
}

// under reports whether the MSC code c is the code a or a code under it.
func under(c, a string) bool {
	for ; c != ""; c = msc.Parent(c) {
		if c == a {
			return true
		}
	}
	return false
}

// members returns the nodes of a topic: the nodes with a topics edge to it.
func (p *packer) members(topic string) []string {
	var out []string
	for _, e := range p.t.graph.In(graph.PartOf, topic) {
		out = append(out, e.From)
	}
	return out
}

// definitions returns the definitions of a concept that are not retired, in tag order.
func (p *packer) definitions(concept string) []string {
	var out []string
	for _, e := range p.t.graph.In(graph.Defines, concept) {
		if gn, ok := p.t.graph.Node(e.From); ok && !gn.Retired {
			out = append(out, e.From)
		}
	}
	return out
}

// choose returns the definition that the volume uses for the concept, or "" when it has none. It reports a concept with two or more definitions and no choice. With no choice, it takes prefer when prefer is a definition of the concept, else the first definition in tag order.
func (p *packer) choose(concept, prefer string) string {
	if d, ok := p.chosen[concept]; ok {
		return d
	}
	defs := p.definitions(concept)
	d := ""
	switch c, ok := p.v.Choose[concept]; {
	case ok && slices.Contains(defs, c):
		d = c
	case ok:
		p.problem("PC3", concept, "", "the volume chooses %s for the concept %s, and it is not a definition of the concept. Its definitions are %s", c, concept, strings.Join(defs, ", "))
		if len(defs) > 0 {
			d = defs[0]
		}
	case len(defs) == 1:
		d = defs[0]
	case len(defs) > 1:
		p.problem("PC3", concept, "", "the concept %s has %d definitions %s, and the volume chooses none. Add it to choose", concept, len(defs), strings.Join(defs, ", "))
		d = defs[0]
		if slices.Contains(defs, prefer) {
			d = prefer
		}
	}
	p.chosen[concept] = d
	if d != "" {
		p.stands[d] = concept
		for _, o := range defs {
			if o != d {
				p.alt[o] = d
			}
		}
	}
	return d
}

// statementProofs returns the proofs that the volume shows for a statement.
func (p *packer) statementProofs(s string) []string {
	var all []string
	for _, e := range p.t.graph.In(graph.Proves, s) {
		if gn, ok := p.t.graph.Node(e.From); ok && !gn.Retired && gn.Kind == "proof" {
			all = append(all, e.From)
		}
	}
	switch p.v.proofs() {
	case NoProofs:
		return nil
	case MainProofs:
		alt, ok := p.t.graph.Closure(s, graph.MinDepth)
		if !ok {
			return nil
		}
		var out []string
		for _, pr := range all {
			if _, found := slices.BinarySearch(alt.Proofs, pr); found {
				out = append(out, pr)
			}
		}
		return out
	}
	return all
}

// setProofs sets the proofs that the item x shows.
func (p *packer) setProofs(x string, proofs []string) {
	p.proofs[x] = proofs
	for _, pr := range proofs {
		if p.proofOf[pr] == "" {
			p.proofOf[pr] = x
		}
	}
}

// equivalenceProofs returns the proofs that a characterization is equivalent to the definition that the volume chooses: the via of each equivalent edge between them. A volume with no proofs shows none.
func (p *packer) equivalenceProofs(x string) []string {
	d := p.alt[x]
	if d == "" || p.v.proofs() == NoProofs {
		return nil
	}
	var out []string
	for _, e := range append(p.t.graph.Out(graph.Equivalent, x), p.t.graph.In(graph.Equivalent, x)...) {
		if (e.To == d || e.From == d) && e.Via != "" && p.t.nodes[e.Via] != nil && !slices.Contains(out, e.Via) {
			out = append(out, e.Via)
		}
	}
	return out
}

// collect takes the closure of the targets (corpus K11.2, step 1).
func (p *packer) collect(targets []string) {
	queue := slices.Clone(targets)
	from := map[string]string{}
	push := func(src string, tags ...string) {
		for _, x := range tags {
			if _, ok := from[x]; !ok {
				from[x] = src
			}
			queue = append(queue, x)
		}
	}
	for len(queue) > 0 {
		x := queue[0]
		queue = queue[1:]
		if p.assumed[x] || p.in[x] {
			continue
		}
		n := p.t.nodes[x]
		if n == nil {
			p.d.missing = append(p.d.missing, missingRef{from: from[x], to: x, how: "uses"})
			continue
		}
		if gn, _ := p.t.graph.Node(x); gn.Retired {
			p.problem("PC3", from[x], "", "%s uses %s, and %s is retired. Use its successor", from[x], x, x)
			continue
		}
		if n.Kind() == "topic" {
			p.in[x] = true
			push(x, p.members(x)...)
			continue
		}
		p.in[x] = true
		p.seq = append(p.seq, x)
		push(x, n.List("uses")...)
		switch n.Kind() {
		case "concept":
			if d := p.choose(x, ""); d != "" {
				push(x, d)
				for _, o := range p.definitions(x) {
					if o != d {
						push(x, o)
					}
				}
			}
		case "definition":
			if c := n.Get("defines"); c != "" && p.t.nodes[c] != nil {
				// A characterization needs the definition that it is equivalent to.
				if d := p.choose(c, x); d != x && d != "" {
					push(x, d)
				}
				if pr := p.equivalenceProofs(x); len(pr) > 0 {
					p.setProofs(x, pr)
					push(x, pr...)
				}
			}
		case "statement", "conjecture", "problem":
			pr := p.statementProofs(x)
			p.setProofs(x, pr)
			push(x, pr...)
		}
	}
	if p.v.Exercises.PerSection > 0 {
		p.exercises()
	}
}

// exercises adds the exercises about the concepts of the volume that use only nodes of the volume and of assumes. No node uses an exercise, so the closure does not find them.
func (p *packer) exercises() {
	for _, gn := range p.t.graph.Nodes {
		x := gn.Tag
		if gn.Kind != "exercise" || gn.Retired || p.in[x] {
			continue
		}
		about := p.sectionOf(x, 0)
		if about == x || !p.in[about] && !p.covered(about) {
			continue
		}
		if slices.ContainsFunc(p.t.nodes[x].List("uses"), func(u string) bool { return !p.in[u] && !p.assumed[u] }) {
			continue
		}
		p.in[x] = true
		p.seq = append(p.seq, x)
	}
}

// fromOutline takes the items of the outline of the volume, in order.
func (p *packer) fromOutline() {
	seen := map[string]bool{}
	for ci, oc := range p.v.Outline {
		c := &Chapter{Number: ci + 1, ID: oc.place(), Title: oc.Title}
		for _, os := range oc.Sections {
			s := &Section{ID: os.place(), Title: os.Title}
			for _, x := range os.Nodes {
				n := p.t.nodes[x]
				switch {
				case n == nil:
					p.d.missing = append(p.d.missing, missingRef{to: x, how: "outline"})
					continue
				case seen[x]:
					p.problem("PC2", x, "outline", "%s is in the outline two times. A node has one home in a volume", x)
					continue
				}
				seen[x] = true
				p.in[x] = true
				switch n.Kind() {
				case "definition":
					if cpt := n.Get("defines"); cpt != "" && p.t.nodes[cpt] != nil {
						p.choose(cpt, x)
						p.setProofs(x, p.equivalenceProofs(x))
						for _, pr := range p.proofs[x] {
							p.in[pr] = true
						}
					}
				case "statement", "conjecture", "problem":
					p.setProofs(x, p.statementProofs(x))
					for _, pr := range p.proofs[x] {
						p.in[pr] = true
					}
				}
				s.Items = append(s.Items, p.item(x))
			}
			c.Sections = append(c.Sections, s)
		}
		p.d.Chapters = append(p.d.Chapters, c)
	}
	for c, d := range p.chosen {
		if d != "" && p.in[d] && seen[c] {
			p.problem("PC2", c, "outline", "the concept %s and its definition %s are both in the outline. The definition stands for the concept", c, d)
		}
	}
	for _, x := range p.d.targets {
		if !p.in[x] && !p.covered(x) {
			p.problem("PC3", x, "outline", "the target %s is not in the outline", x)
		}
	}
}

// covered reports whether a concept is in the volume through its definition.
func (p *packer) covered(x string) bool {
	d := p.chosen[x]
	if d == "" {
		if n := p.t.nodes[x]; n != nil && n.Kind() == "concept" {
			d = p.choose(x, "")
		}
	}
	return d != "" && p.in[d]
}

// itemKeys are the keys that order the items when the volume has no outline.
type itemKeys struct {
	chapter, chapterTitle, chapterCode string
	section, sectionTitle              string
	curriculum                         int
	code                               string
}

// order makes the outline (corpus K11.2, step 2). Each item comes after its prerequisites. Among the items that are ready, the order takes the item of the current section, then of the current chapter, then the first by the keys.
func (p *packer) order() {
	var items []string
	for _, x := range p.seq {
		if p.isItem(x) {
			items = append(items, x)
		}
	}
	keys := map[string]itemKeys{}
	for _, x := range items {
		keys[x] = p.keys(x)
	}
	deps := map[string][]string{}
	waiting := map[string]int{}
	users := map[string][]string{}
	for _, x := range items {
		for _, dep := range p.prerequisites(x) {
			if dep == x || slices.Contains(deps[x], dep) {
				continue
			}
			deps[x] = append(deps[x], dep)
			users[dep] = append(users[dep], x)
			waiting[x]++
		}
	}
	// blocked is the number of items of each chapter that wait for a prerequisite. A new chapter is the one with the fewest, so that the order can finish it before it leaves it.
	blocked := map[string]int{}
	less := func(a, b string, cur itemKeys) int {
		ka, kb := keys[a], keys[b]
		return cmp.Or(
			-cmp.Compare(boolInt(ka.section == cur.section && ka.chapter == cur.chapter), boolInt(kb.section == cur.section && kb.chapter == cur.chapter)),
			-cmp.Compare(boolInt(ka.chapter == cur.chapter), boolInt(kb.chapter == cur.chapter)),
			cmp.Compare(blocked[ka.chapter], blocked[kb.chapter]),
			msc.Compare(ka.chapterCode, kb.chapterCode),
			strings.Compare(ka.chapter, kb.chapter),
			cmp.Compare(ka.curriculum, kb.curriculum),
			strings.Compare(ka.section, kb.section),
			msc.Compare(ka.code, kb.code),
			strings.Compare(a, b),
		)
	}
	var ready, out []string
	for _, x := range items {
		if waiting[x] == 0 {
			ready = append(ready, x)
		} else {
			blocked[keys[x].chapter]++
		}
	}
	done := map[string]bool{}
	closed := map[string]bool{}
	cur := itemKeys{chapter: "\x00", section: "\x00"}
	for len(out) < len(items) {
		if len(ready) == 0 {
			// The rest is in a cycle. Take the first by the keys, and the checks report its forward references.
			for _, x := range items {
				if !done[x] {
					ready = append(ready, x)
				}
			}
		}
		best := 0
		for i := range ready {
			if less(ready[i], ready[best], cur) < 0 {
				best = i
			}
		}
		x := ready[best]
		ready = slices.Delete(ready, best, best+1)
		if done[x] {
			continue
		}
		done[x] = true
		out = append(out, x)
		if k := keys[x]; k.chapter != cur.chapter && cur.chapter != "\x00" {
			if closed[k.chapter] {
				// A chapter that the order left does not start again. The item joins the current chapter.
				k.chapter, k.chapterTitle, k.chapterCode = cur.chapter, cur.chapterTitle, cur.chapterCode
				keys[x] = k
			} else {
				closed[cur.chapter] = true
			}
		}
		cur = keys[x]
		for _, u := range users[x] {
			waiting[u]--
			if waiting[u] == 0 && !done[u] {
				ready = append(ready, u)
				blocked[keys[u].chapter]--
			}
		}
	}
	chapters := map[string]int{}
	sections := map[string]int{}
	titles := map[string]bool{}
	var c *Chapter
	var s *Section
	last := itemKeys{chapter: "\x00"}
	for _, x := range out {
		k := keys[x]
		if c == nil || k.chapter != last.chapter {
			c = &Chapter{Number: len(p.d.Chapters) + 1, ID: unique(chapters, slug(k.chapterTitle)), Title: k.chapterTitle}
			p.d.Chapters = append(p.d.Chapters, c)
			s = nil
		}
		// A node that is about no concept stays in the section before it, so that a result comes after the definitions that it is about.
		kind := p.t.nodes[x].Kind()
		loose := k.section == x && kind != "concept" && kind != "definition"
		switch {
		case s != nil && loose:
			k.section = last.section
		case s == nil || k.section != last.section:
			title := k.sectionTitle
			if titles[c.ID+"\x00"+title] {
				title = "More on " + lowerFirst(title)
			}
			titles[c.ID+"\x00"+k.sectionTitle] = true
			s = &Section{ID: unique(sections, c.ID+"-"+slug(title)), Title: title}
			c.Sections = append(c.Sections, s)
		}
		s.Items = append(s.Items, p.item(x))
		last = k
	}
}

// lowerFirst changes the first letter of a title to lower case, unless the first word is a name or an abbreviation, with a capital letter after the first.
func lowerFirst(s string) string {
	word, _, _ := strings.Cut(s, " ")
	if len(word) < 2 || strings.ToLower(word[1:]) != word[1:] || strings.ContainsAny(word, "'’") {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// unique returns id, or id with a number when the id is taken.
func unique(taken map[string]int, id string) string {
	if id == "" {
		id = "part"
	}
	taken[id]++
	if n := taken[id]; n > 1 {
		return fmt.Sprintf("%s-%d", id, n)
	}
	return id
}

// isItem reports whether a tag in the volume has its own item: it is not a proof, and it is not a concept that a definition in the volume stands for.
func (p *packer) isItem(x string) bool {
	n := p.t.nodes[x]
	if n == nil || n.Kind() == "proof" || n.Kind() == "topic" {
		return false
	}
	if n.Kind() == "concept" {
		if d := p.chosen[x]; d != "" && p.in[d] {
			return false
		}
	}
	return true
}

// home returns the item tag where a tag of the volume is shown, or "" when it is not in the volume.
func (p *packer) home(x string) string {
	if !p.in[x] {
		return ""
	}
	if n := p.t.nodes[x]; n != nil {
		switch n.Kind() {
		case "concept":
			if d := p.chosen[x]; d != "" && p.in[d] {
				return d
			}
		case "proof":
			if s := p.proofOf[x]; s != "" {
				return s
			}
		}
	}
	return x
}

// prerequisites returns the items that must come before the item x: the homes of what it uses and of what its proofs use, and the chosen definition of an equivalent characterization.
func (p *packer) prerequisites(x string) []string {
	var out []string
	add := func(t string) {
		if h := p.home(t); h != "" && h != x {
			out = append(out, h)
		}
	}
	for _, u := range p.t.nodes[x].List("uses") {
		add(u)
	}
	for _, pr := range p.proofs[x] {
		for _, u := range p.t.nodes[pr].List("uses") {
			add(u)
		}
	}
	if d := p.alt[x]; d != "" {
		add(d)
	}
	return out
}

// keys returns the order keys of an item.
func (p *packer) keys(x string) itemKeys {
	n := p.t.nodes[x]
	k := itemKeys{curriculum: int(^uint(0) >> 1)}
	if pos, ok := p.t.curPos[x]; ok {
		k.curriculum = pos
	}
	codes := n.List("msc")
	if len(codes) > 0 {
		k.code = codes[0]
	}
	if topics := n.List("topics"); len(topics) > 0 && p.t.nodes[topics[0]] != nil {
		tn := p.t.nodes[topics[0]]
		k.chapter, k.chapterTitle = topics[0], tn.Name()
		if c := tn.List("msc"); len(c) > 0 {
			k.chapterCode = c[0]
		}
	} else {
		area := ""
		if k.code != "" {
			area = k.code
			if msc.Level(area) == 3 {
				area = msc.Parent(area)
			}
		}
		k.chapter, k.chapterCode, k.chapterTitle = "msc:"+area, area, "Other results"
		if area != "" {
			k.chapterTitle = area
			if p.t.topics != nil {
				if e, ok := p.t.topics.Lookup(area); ok && e.Label != "" {
					k.chapterTitle = capitalize(e.Label)
				}
			}
		}
	}
	k.section = p.sectionOf(x, 0)
	k.sectionTitle = n.Name()
	if sn := p.t.nodes[k.section]; sn != nil {
		k.sectionTitle = sn.Name()
	}
	return k
}

// sectionOf returns the concept that a node is about: the concept of a definition, the concept that an example is an instance of, the concept of the first about edge of a statement, or the node itself.
func (p *packer) sectionOf(x string, depth int) string {
	n := p.t.nodes[x]
	if n == nil || depth > 2 {
		return x
	}
	var next string
	switch n.Kind() {
	case "definition":
		next = n.Get("defines")
	case "example", "counterexample":
		next = first(n.List("instance_of"), n.List("illustrates"))
		if next == "" {
			for _, e := range p.t.graph.Out(graph.NotInstance, x) {
				next = e.To
				break
			}
		}
	case "statement", "conjecture", "problem", "exercise":
		next = first(n.List("about"))
	}
	if next == "" || p.t.nodes[next] == nil {
		return x
	}
	if p.t.nodes[next].Kind() == "concept" {
		return next
	}
	return p.sectionOf(next, depth+1)
}

func first(lists ...[]string) string {
	for _, l := range lists {
		if len(l) > 0 {
			return l[0]
		}
	}
	return ""
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// labels are the labels of the kinds and of the roles of statements.
var labels = map[string]string{
	"concept": "Concept", "definition": "Definition", "statement": "Proposition", "conjecture": "Conjecture", "problem": "Problem",
	"example": "Example", "counterexample": "Counterexample", "object": "Object", "proof": "Proof", "computation": "Computation", "exercise": "Exercise",
	"theorem": "Theorem", "proposition": "Proposition", "lemma": "Lemma", "corollary": "Corollary", "axiom": "Axiom", "fact": "Fact",
}

// mainSections are the sections that state the node, by kind. The first that the node has is its main section.
var mainSections = []string{"Statement", "Definition", "Problem", "Summary", "Description", "Example", "Counterexample", "Proof", "Exercise"}

// item makes the item of a tag, with its proofs.
func (p *packer) item(x string) *Item {
	n := p.t.nodes[x]
	it := &Item{Tag: x, Kind: n.Kind(), Role: n.Get("role"), Name: n.Name(), Path: n.Path}
	it.Label = labels[it.Kind]
	if l := labels[it.Role]; it.Kind == "statement" && l != "" {
		it.Label = l
	}
	if c := p.stands[x]; c != "" {
		it.Concept = c
	}
	if d := p.alt[x]; d != "" {
		it.Characterizes = d
		it.Label = "Characterization"
	}
	it.Writing = p.t.writing[x].Level
	it.Evidence = p.t.status[x].Evidence
	if it.Kind == "statement" && it.Role != "axiom" && it.Evidence < status.Standard {
		it.Claim = cmp.Or(it.Evidence.Word(), "unchecked")
		it.Label = "Claim"
	}
	for _, s := range check.Sections(n) {
		switch {
		case it.Main == "" && slices.Contains(mainSections, s.Name):
			it.Main = strings.TrimSpace(s.Text)
			it.sections = append(it.sections, s.Name)
		case s.Name == "Remarks" && p.v.Kind != "reference" && p.v.Kind != "survey":
			it.Remarks = strings.TrimSpace(s.Text)
			it.sections = append(it.sections, s.Name)
		}
	}
	for _, pr := range p.proofs[x] {
		pi := p.item(pr)
		pi.Proof = true
		it.Proofs = append(it.Proofs, pi)
	}
	return it
}

// number gives each chapter, section and item its number, and fills homes (corpus K11.2, step 4). The items of a chapter share one counter.
func (p *packer) number() {
	pos := 0
	for _, c := range p.d.Chapters {
		k := 0
		for si, s := range c.Sections {
			s.Number = fmt.Sprintf("%d.%d", c.Number, si+1)
			for _, it := range s.Items {
				k++
				it.Number = fmt.Sprintf("%d.%d", c.Number, k)
				it.position = pos
				pos++
				p.d.homes[it.Tag] = it
				if it.Concept != "" {
					p.d.homes[it.Concept] = it
				}
				for _, pr := range it.Proofs {
					pr.position = it.position
					p.d.homes[pr.Tag] = pr
				}
			}
		}
	}
	// A concept whose chosen definition is in the volume is shown there, also when the concept was not in the closure.
	for c, d := range p.chosen {
		if it := p.d.homes[d]; it != nil && p.d.homes[c] == nil {
			p.d.homes[c] = it
			if it.Concept == "" {
				it.Concept = c
			}
		}
	}
}

// glue puts each part of the glue at its place.
func (p *packer) glue() {
	places := map[string]*[]Glue{Preface: &p.d.Preface}
	for _, c := range p.d.Chapters {
		places[c.ID] = &c.Glue
		for _, s := range c.Sections {
			places[s.ID] = &s.Glue
		}
	}
	for _, g := range p.v.Glue {
		if l := places[g.At]; l != nil {
			*l = append(*l, g)
		} else {
			p.problem("PC7", "", "glue", "the glue at %s has no place in the volume. Use preface or the id of a chapter or a section", g.At)
		}
	}
}

// references finds the links of the bodies to nodes outside the volume, and the links to no node.
func (p *packer) references() {
	cited := map[string]bool{}
	p.d.walk(func(it *Item) {
		n := p.t.nodes[it.Tag]
		for _, l := range check.Links(n) {
			switch {
			case l.Target == "" || !slices.Contains(it.sections, l.Section):
			case p.t.nodes[l.Target] == nil:
				p.d.missing = append(p.d.missing, missingRef{from: it.Tag, to: l.Target, how: "links"})
			case p.d.homes[l.Target] == nil && !cited[l.Target]:
				cited[l.Target] = true
				p.d.Cited = append(p.d.Cited, p.ref(l.Target))
			}
		}
	})
	slices.SortFunc(p.d.Cited, func(a, b Ref) int { return strings.Compare(a.Tag, b.Tag) })
}

// walk calls f for each item and each proof, in reading order.
func (d *Document) walk(f func(*Item)) {
	for _, c := range d.Chapters {
		for _, s := range c.Sections {
			for _, it := range s.Items {
				f(it)
				for _, pr := range it.Proofs {
					f(pr)
				}
			}
		}
	}
}

// Items returns the items and the proofs of the document, in reading order.
func (d *Document) Items() []*Item {
	var out []*Item
	d.walk(func(it *Item) { out = append(out, it) })
	return out
}

// Home returns the item that shows the node with the tag, or nil when the node is not in the volume. A concept that a definition stands for has the item of the definition.
func (d *Document) Home(t string) *Item { return d.homes[t] }

// notation lists the macros that the items use, in reading order: by the item that introduces each one, else by its first use (corpus K10.4).
func (p *packer) notation() {
	if p.t.reg == nil {
		return
	}
	uses := map[string]*NotationUse{}
	p.d.walk(func(it *Item) {
		for _, c := range check.Commands(p.t.nodes[it.Tag]) {
			for _, e := range p.t.reg.Lookup(`\` + c.Name) {
				if uses[e.ID()] != nil {
					continue
				}
				u := &NotationUse{ID: e.ID(), Macro: e.Macro, Args: e.Args, LaTeX: e.LaTeX, Reads: e.Reads, Convention: e.Convention, position: it.position}
				if intro := p.d.homes[e.IntroducedBy]; intro != nil {
					u.Introduced, u.position = intro.Number, intro.position
				}
				if name, ok := p.v.Notation[e.ID()]; ok {
					u.Variant = name
					for _, va := range e.Variants {
						if va.Name == name {
							u.LaTeX = va.LaTeX
						}
					}
				}
				uses[e.ID()] = u
			}
		}
	})
	for _, u := range uses {
		p.d.Notation = append(p.d.Notation, *u)
	}
	slices.SortFunc(p.d.Notation, func(a, b NotationUse) int {
		return cmp.Or(cmp.Compare(a.position, b.position), strings.Compare(a.ID, b.ID))
	})
}

// conventions lists the conventions that the items depend on: each convention that a node names in its field conventions, or whose concept the volume shows.
func (p *packer) conventions() {
	byID := map[string]Convention{}
	for _, c := range p.v.Conventions {
		byID[c.Convention] = c
	}
	for _, c := range p.t.conv {
		use := ConventionUse{ID: c.ID, Option: c.Default}
		if vc, ok := byID[c.ID]; ok {
			use.Option = vc.Option
		}
		use.Default = use.Option == c.Default
		depends := c.Concept != "" && p.d.homes[c.Concept] != nil
		p.d.walk(func(it *Item) {
			opt, named := "", false
			for _, m := range mappings(p.t.nodes[it.Tag].Meta["conventions"]) {
				if s, _ := m["convention"].(string); s == c.ID {
					opt, _ = m["option"].(string)
					named = true
				}
			}
			if !named {
				return
			}
			depends = true
			if opt != use.Option {
				use.Differ = append(use.Differ, Difference{Tag: it.Tag, Name: it.Name, Option: opt, Note: byID[c.ID].Differ[it.Tag]})
			}
		})
		if depends {
			p.d.Conventions = append(p.d.Conventions, use)
		}
	}
}

func mappings(v any) []map[string]any {
	var out []map[string]any
	list, _ := v.([]any)
	for _, x := range list {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// attribution makes the attribution page from the provenance of the items and of the glue (corpus K11.7).
func (p *packer) attribution() {
	credits := map[string]*Credit{}
	get := func(id string) *Credit {
		if credits[id] == nil {
			credits[id] = &Credit{Person: id}
		}
		return credits[id]
	}
	for _, e := range p.v.Editors {
		get(e).Edited = true
	}
	agents := map[string]bool{}
	p.d.walk(func(it *Item) {
		lines, ok := p.t.logs[it.Tag]
		if !ok || len(lines) == 0 {
			if tag.IsTag(it.Tag) {
				p.d.Attribution.NoLog = append(p.d.Attribution.NoLog, it.Tag)
			}
		}
		people := map[string]bool{}
		for _, l := range lines {
			if l.Human != "" {
				people[l.Human] = true
			}
			if l.Agent != "" {
				agents[l.Agent+" run "+cmp.Or(l.Run, "unknown")] = true
			}
		}
		for h := range people {
			get(h).Nodes++
		}
		for _, r := range p.t.reviews[it.Tag] {
			get(r).Reviews++
		}
	})
	for _, g := range p.v.Glue {
		if prov.IsPerson(g.Writer) {
			get(g.Writer).Glue++
		}
		if g.Agent() {
			agents[g.Writer+" run "+cmp.Or(g.Run, "unknown")] = true
		}
		if g.Confirmed != "" && g.Confirmed != g.Writer {
			get(g.Confirmed).Glue++
		}
	}
	for _, c := range credits {
		p.d.Attribution.People = append(p.d.Attribution.People, *c)
	}
	total := func(c Credit) int { return c.Nodes + c.Reviews + c.Glue }
	slices.SortFunc(p.d.Attribution.People, func(a, b Credit) int {
		return cmp.Or(-cmp.Compare(total(a), total(b)), -cmp.Compare(boolInt(a.Edited), boolInt(b.Edited)), strings.Compare(a.Person, b.Person))
	})
	p.d.Attribution.Agents = slices.Sorted(func(yield func(string) bool) {
		for a := range agents {
			if !yield(a) {
				return
			}
		}
	})
	slices.Sort(p.d.Attribution.NoLog)
}
