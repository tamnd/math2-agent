package wikipedia

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/wpcorpus"
	"github.com/tamnd/math2-agent/wpsource"
)

// AdoptPath is the report of m2 corpus adopt -source wikipedia, relative to the root of the tree.
const AdoptPath = "reports/adopt/wikipedia.md"

// AdoptBy is the name of the run in the outcomes and the history of the rows.
const AdoptBy = "m2 corpus adopt"

// The states of an item in the adopt report.
const (
	Adopted = "adopted" // the run wrote a node with the text of the item
	Drafted = "drafted" // a node with a provisional id has the text, and waits for m2 mint
	Linked  = "linked"  // the run set the outcome of the row to the minted node
	Waiting = "waiting" // the item goes to the write role or to a person, with the reason
)

// AdoptNode is what the adopt job reads from a node of the tree.
type AdoptNode struct {
	ID        string
	Name      string
	Aliases   []string
	MSC       []string
	Inventory []string
	Proves    string
	Mentions  []string // the mention of each wikipedia source of the node
	Retired   bool
}

// AdoptOptions are the inputs of Adopt that are not the inventory and the nodes.
type AdoptOptions struct {
	Corpus string                 // the checkout of tamnd/wikipedia-math
	Pages  map[int]bool           // the pages to adopt from. Nil means each page.
	Hold   map[string]string      // the mentions that a person holds, with the reason
	Draw   func() (string, error) // gives a new provisional id
	Day    string                 // the date of the run, YYYY-MM-DD
	Until  string                 // when not empty, the date until which a row whose item waits is deferred
}

// AdoptItem is one line of the adopt report.
type AdoptItem struct {
	Mention string
	Row     string
	Page    int
	Title   string
	Name    string
	Kind    string
	Cue     string
	State   string
	Reason  string
	Node    string
}

// AdoptDraft is a node file that the run writes.
type AdoptDraft struct {
	ID   string
	Path string // nodes/~/<id>.md
	Data []byte
}

// AdoptResult is the result of Adopt.
type AdoptResult struct {
	Drafts   []AdoptDraft
	Rows     []inventory.Row // each row, with the outcomes that the run set
	Linked   int             // the rows that the run set to the outcome node
	Deferred int             // the rows that the run deferred
	Items    []AdoptItem
}

// adoptFlags are the flags of WP9.2 that stop the adoption of an item.
var adoptFlags = []string{"citation-needed", "dubious", "disputed", "informal"}

// genericName matches a name that is a label and not the name of the item, as Theorem (2), Theorem A (groups) or Examples.
var genericName = regexp.MustCompile(`(?i)^(theorem|lemma|corollary|proposition|claim|fact|definition|definitions|example|examples|remark|proof|statement|statements|properties|facts|notation|applications?|generali[sz]ations?|extensions?|construction|history|overview|motivation|algorithms?|representations?)( \(\d+\))?$|^(theorem|lemma|corollary|proposition) [A-Z]( \([^)]*\))?$`)

// adoptCues are the cues that give the item a name of its own (WP9.1 reason 2). A plain heading names a section, not an item.
var adoptCues = []string{"lead", "math-theorem", "named-heading", "term", "inline-head", "list-entry"}

// cardSection gives the kind of the node and the section of its card for the kind of a mention (K8.4).
func cardSection(kind string) (string, string) {
	switch kind {
	case "definition":
		return "concept", "Summary"
	case "statement":
		return "statement", "Statement"
	case "conjecture":
		return "conjecture", "Statement"
	case "problem":
		return "problem", "Problem"
	case "example":
		return "example", "Example"
	case "counterexample":
		return "counterexample", "Counterexample"
	case "proof":
		return "proof", "Proof"
	}
	return "", ""
}

// Adopt makes the nodes of WP9.2 from the Wikipedia rows of the pages. First it sets the outcome node on each open row that a minted node names in its inventory field. Then it takes each open row with a mention of the pages, and it writes a node with the text of the item, or it reports why the item waits. A proof attached to a statement with a node, or to a statement that the run adopts, becomes a proof node.
func Adopt(rows []inventory.Row, ms []inventory.Mention, nodes []AdoptNode, opt AdoptOptions) (*AdoptResult, error) {
	res := &AdoptResult{Rows: rows}
	byRow := map[string]*inventory.Row{}
	rowOf := map[string]*inventory.Row{}
	for i := range res.Rows {
		r := &res.Rows[i]
		byRow[r.ID] = r
		for _, m := range r.Mentions {
			rowOf[m] = r
		}
	}

	// The nodes that the tree has.
	byName := map[string]string{}
	nameOf := map[string]string{}
	proved := map[string]bool{}
	done := map[string]string{}
	msc := map[string][]string{}
	for _, n := range nodes {
		if n.Retired {
			continue
		}
		for _, s := range append([]string{n.Name}, n.Aliases...) {
			if k := inventory.NameKey(s); k != "" {
				if _, ok := byName[k]; !ok {
					byName[k] = n.ID
				}
			}
		}
		if n.Proves != "" {
			proved[n.Proves] = true
		}
		for _, m := range n.Mentions {
			done[m] = n.ID
		}
		msc[n.ID] = n.MSC
		nameOf[n.ID] = n.Name
		provisional := strings.HasPrefix(n.ID, "~")
		for _, id := range n.Inventory {
			r := byRow[id]
			if r == nil || r.Outcome != nil || provisional || len(n.Mentions) == 0 {
				continue
			}
			r.Outcome = &inventory.Outcome{Value: inventory.OutcomeNode, Target: []string{n.ID}, Rule: "WP9.2", By: AdoptBy}
			r.History = append(r.History, inventory.Event{Op: "outcome", At: opt.Day, By: AdoptBy, Note: "node " + n.ID + " has the adopted text"})
			res.Linked++
		}
	}

	// The subjects of the articles.
	byMention := map[string]*inventory.Mention{}
	subjectOf := map[string]int{}
	title := map[int]string{}
	for i := range ms {
		m := &ms[i]
		if m.Source != Source {
			continue
		}
		byMention[m.ID] = m
		if strings.Contains(m.ID, "#") {
			continue
		}
		title[m.Page] = m.NativeID
		for _, s := range append([]string{m.Name, Name(m.NativeID)}, m.Aliases...) {
			if k := inventory.NameKey(s); k != "" {
				if _, ok := subjectOf[k]; !ok {
					subjectOf[k] = m.Page
				}
			}
		}
	}
	nodeOf := func(mention string) string {
		r := rowOf[mention]
		if r == nil || r.Outcome == nil || r.Outcome.Value != inventory.OutcomeNode || len(r.Outcome.Target) == 0 {
			return ""
		}
		return r.Outcome.Target[0]
	}
	pageNode := func(page int) string { return nodeOf(wpcorpus.MentionID(page, "")) }

	pages := &pageCache{corpus: opt.Corpus, files: map[int]*pageFile{}}
	adopted := map[string]string{}
	var proofs []*inventory.Mention
	for i := range ms {
		m := &ms[i]
		if m.Source != Source || opt.Pages != nil && !opt.Pages[m.Page] {
			continue
		}
		if m.Kind == "proof" {
			proofs = append(proofs, m)
			continue
		}
		r := rowOf[m.ID]
		if r == nil || !r.IsOpen(dayTime(opt.Day)) {
			continue
		}
		it := AdoptItem{Mention: m.ID, Row: r.ID, Page: m.Page, Title: title[m.Page], Name: m.Name, Kind: m.Kind, Cue: m.Cue}
		if id, ok := done[m.ID]; ok {
			it.State, it.Node = Drafted, id
			res.Items = append(res.Items, it)
			continue
		}
		it.State = Waiting
		it.Reason = whyWait(m, opt.Hold, subjectOf, byName)
		if it.Reason == "" {
			d, why, err := adoptNode(m, displayName(m), freeAliases(m, byName), pages, pageNode, opt, msc[pageNode(m.Page)], "", r.ID)
			if err != nil {
				return nil, err
			}
			if d == nil {
				it.Reason = why
			} else {
				res.Drafts = append(res.Drafts, *d)
				adopted[m.ID] = d.ID
				nameOf[d.ID] = m.Name
				it.State, it.Node = Adopted, d.ID
			}
		}
		if it.State == Waiting && opt.Until != "" {
			r.Outcome = &inventory.Outcome{Value: inventory.OutcomeDeferred, Reason: it.Reason, Rule: "WP9.2", Until: opt.Until, By: AdoptBy}
			r.History = append(r.History, inventory.Event{Op: "outcome", At: opt.Day, By: AdoptBy, Note: "deferred until " + opt.Until})
			res.Deferred++
		}
		res.Items = append(res.Items, it)
	}

	// The proofs of the statements with a node.
	for _, m := range proofs {
		it := AdoptItem{Mention: m.ID, Page: m.Page, Title: title[m.Page], Name: m.Name, Kind: m.Kind, Cue: m.Cue, State: Waiting}
		target := adopted[m.AttachedTo]
		if target == "" {
			target = nodeOf(m.AttachedTo)
		}
		t := byMention[m.AttachedTo]
		switch {
		case done[m.ID] != "":
			it.State, it.Node = Drafted, done[m.ID]
		case target == "":
			it.Reason = "the statement that the proof proves has no node"
		case t != nil && t.Kind != "statement":
			it.Reason = "the proof is attached to an item that is not a statement"
		case nameOf[target] == "":
			it.Reason = "node " + target + " has no name"
		case proved[target]:
			it.Reason = "node " + target + " has a proof node"
		default:
			it.Reason = whyFlag(m, opt.Hold)
		}
		if it.State == Waiting && it.Reason == "" {
			d, why, err := adoptOne(m, "Proof of "+nameOf[target], pages, pageNode, opt, msc[pageNode(m.Page)], target, "")
			if err != nil {
				return nil, err
			}
			if d == nil {
				it.Reason = why
			} else {
				res.Drafts = append(res.Drafts, *d)
				proved[target] = true
				it.State, it.Node = Adopted, d.ID
			}
		}
		res.Items = append(res.Items, it)
	}
	slices.SortStableFunc(res.Items, func(a, b AdoptItem) int {
		if a.Page != b.Page {
			return a.Page - b.Page
		}
		return strings.Compare(a.Mention, b.Mention)
	})
	return res, nil
}

// dayTime gives the time of a date YYYY-MM-DD, or the zero time.
func dayTime(day string) time.Time {
	t, _ := time.Parse(time.DateOnly, day)
	return t
}

func whyFlag(m *inventory.Mention, hold map[string]string) string {
	if why, ok := hold[m.ID]; ok {
		return "a person holds the item: " + why
	}
	for _, f := range m.Flags {
		if slices.Contains(adoptFlags, f) {
			return "the item has the flag " + f + " (WP9.2)"
		}
	}
	return ""
}

// whyWait gives the reason why the item of a mention is not adopted, or "".
func whyWait(m *inventory.Mention, hold map[string]string, subjectOf map[string]int, byName map[string]string) string {
	if why := whyFlag(m, hold); why != "" {
		return why
	}
	if m.Kind == "topic" {
		return "a list or a glossary is not a node (WP9.1)"
	}
	if _, sec := cardSection(m.Kind); sec == "" {
		return "the kind of the item is " + m.Kind
	}
	if !slices.Contains(adoptCues, m.Cue) {
		return "the cue " + m.Cue + " names a section and not one item"
	}
	if m.Name == "" || genericName.MatchString(m.Name) {
		return "the item has no name of its own, and the write role names it"
	}
	if m.ArticleType == "biography" || m.ArticleType == "field" {
		return "an article of type " + m.ArticleType + " gives a record and not a node (WP9.1)"
	}
	k := inventory.NameKey(m.Name)
	if p, ok := subjectOf[k]; ok && strings.Contains(m.ID, "#") {
		if p == m.Page {
			return "the item has the name of the subject of its article, and a person decides if they are one item"
		}
		return "the item has its own article, page " + strconv.Itoa(p)
	}
	if id, ok := byName[k]; ok {
		return "node " + id + " has the same name, and a person decides if they are one item"
	}
	return ""
}

// adoptOne writes the node of one item. It gives a nil draft and the reason when the text cannot be adopted.
func adoptOne(m *inventory.Mention, name string, pages *pageCache, pageNode func(int) string, opt AdoptOptions, msc []string, proves, row string) (*AdoptDraft, string, error) {
	return adoptNode(m, name, nil, pages, pageNode, opt, msc, proves, row)
}

func adoptNode(m *inventory.Mention, name string, aliases []string, pages *pageCache, pageNode func(int) string, opt AdoptOptions, msc []string, proves, row string) (*AdoptDraft, string, error) {
	pf, err := pages.get(m.Page, m.Revision)
	if err != nil {
		return nil, "", err
	}
	if pf == nil {
		return nil, "the corpus has no content Markdown of the revision", nil
	}
	ref := pf.refs[tagOf(m.ID)]
	if !strings.Contains(m.ID, "#") {
		ref = pf.subject
	}
	span, ok := pf.span(ref)
	if !ok {
		return nil, "the content Markdown has no block for the item", nil
	}
	text := convert(span, pageNode)
	if text == "" {
		return nil, "the block of the item has no text", nil
	}
	if strings.HasPrefix(m.Assessment, "Stub") && sentences(text) < 2 {
		return nil, "the article is a stub with fewer than two sentences about the item (WP9.2)", nil
	}
	kind, section := cardSection(m.Kind)
	id, err := opt.Draw()
	if err != nil {
		return nil, "", err
	}
	src := wpsource.Ref{Page: int64(m.Page), Revision: int64(m.Revision)}
	locator := pf.title
	if m.Section != "" {
		locator += ", section " + m.Section
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	fmt.Fprintf(&b, "tag: %s\nkind: %s\n", id, kind)
	if kind == "statement" {
		role := m.Role
		if role == "" || role == "fact" || role == "axiom" {
			role = "proposition"
		}
		fmt.Fprintf(&b, "role: %s\n", role)
	}
	fmt.Fprintf(&b, "name: %s\n", scalar(name))
	if len(aliases) > 0 && kind != "proof" {
		fmt.Fprintf(&b, "aliases: %s\n", flow(aliases))
	}
	if len(msc) > 0 {
		fmt.Fprintf(&b, "msc: %s\n", flow(msc))
	}
	if proves != "" {
		fmt.Fprintf(&b, "proves: %s\n", proves)
	}
	b.WriteString("sources:\n")
	fmt.Fprintf(&b, "  - %s: %s\n    locator: %s\n    note: adopted\n    mention: %s\n    sections: [%s]\n", wpsource.Key, src, scalar(locator), scalar(m.ID), section)
	if q := m.Same["wikidata"]; len(q) > 0 && !strings.Contains(m.ID, "#") {
		fmt.Fprintf(&b, "same_as:\n  wikidata: %s\n  wikipedia: %s\n", q[0], scalar(pf.title))
	}
	if row != "" {
		fmt.Fprintf(&b, "inventory: [%s]\n", row)
	}
	fmt.Fprintf(&b, "---\n\n## %s\n\n%s\n", section, text)
	return &AdoptDraft{ID: id, Path: "nodes/~/" + id + ".md", Data: b.Bytes()}, "", nil
}

// displayName gives the name of the node: the name of the mention, with a capital first letter for a glossary term, as the other node names have. A name that starts with a letter symbol, as p-group, does not change.
func displayName(m *inventory.Mention) string {
	if m.Cue != "term" || m.Name == "" {
		return m.Name
	}
	r := []rune(m.Name)
	if !unicode.IsLower(r[0]) || len(r) > 1 && !unicode.IsLower(r[1]) {
		return m.Name
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// freeAliases gives the aliases of the mention that are not the name or an alias of a node, so that the new node does not take the name of another node.
func freeAliases(m *inventory.Mention, byName map[string]string) []string {
	var out []string
	for _, a := range m.Aliases {
		if _, ok := byName[inventory.NameKey(a)]; !ok {
			out = append(out, a)
		}
	}
	return out
}

func tagOf(id string) string {
	_, t, _ := strings.Cut(id, "#")
	return t
}

func scalar(s string) string {
	out, err := yaml.Marshal(s)
	if err != nil {
		return strconv.Quote(s)
	}
	return strings.TrimSuffix(string(out), "\n")
}

func flow(vs []string) string {
	q := make([]string, len(vs))
	for i, v := range vs {
		q[i] = scalar(v)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

var sentenceEnd = regexp.MustCompile(`[.!?](\s|$)`)

func sentences(text string) int { return len(sentenceEnd.FindAllStringIndex(text, -1)) }

// pageFile is the content Markdown of one revision, with the objects file that maps the tags to the refs of the blocks.
type pageFile struct {
	title   string
	lines   []string
	refs    map[string]string // tag to ref
	subject string            // the ref of the subject block
}

type pageCache struct {
	corpus string
	files  map[int]*pageFile
}

func (c *pageCache) get(page, rev int) (*pageFile, error) {
	if pf, ok := c.files[page]; ok {
		return pf, nil
	}
	c.files[page] = nil
	md, err := os.ReadFile(filepath.Join(c.corpus, wpcorpus.MarkdownPath(page))) // #nosec G304 -- the corpus is a checkout that the user gives.
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var head struct {
		Title    string `yaml:"title"`
		Revision int    `yaml:"revision"`
	}
	if h, _, err := splitHeader(md); err == nil {
		_ = yaml.Unmarshal(h, &head)
	}
	if head.Revision != rev {
		return nil, nil
	}
	od, err := os.ReadFile(filepath.Join(c.corpus, wpcorpus.ObjectsPath(page, rev))) // #nosec G304 -- the corpus is a checkout that the user gives.
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	objs, err := wpcorpus.ParseObjects(od)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", wpcorpus.ObjectsPath(page, rev), err)
	}
	pf := &pageFile{title: head.Title, lines: strings.Split(string(md), "\n"), refs: map[string]string{}}
	for _, o := range objs {
		if o.Tag != "" {
			pf.refs[o.Tag] = o.Ref
		}
		if o.Kind == "subject" && pf.subject == "" {
			pf.subject = o.Ref
		}
	}
	c.files[page] = pf
	return pf, nil
}

func splitHeader(data []byte) ([]byte, []byte, error) {
	const fm = "---\n"
	if !bytes.HasPrefix(data, []byte(fm)) {
		return nil, data, fmt.Errorf("no header")
	}
	rest := data[len(fm):]
	i := bytes.Index(rest, []byte("\n"+fm))
	if i < 0 {
		return nil, data, fmt.Errorf("the header does not close")
	}
	return rest[:i+1], rest[i+1+len(fm):], nil
}

// divOpen matches the first line of a fenced div of the content Markdown, as ::: {#o7 .statement ...}.
var divOpen = regexp.MustCompile(`^:{3,} *\{#([A-Za-z0-9_-]+)`)

func isDivClose(l string) bool { return len(l) >= 3 && strings.Trim(l, ":") == "" }

// span gives the lines inside the div of the ref. The divs inside it are other items, so they are not in the result.
func (pf *pageFile) span(ref string) ([]string, bool) {
	if ref == "" {
		return nil, false
	}
	for i, l := range pf.lines {
		g := divOpen.FindStringSubmatch(l)
		if g == nil || g[1] != ref {
			continue
		}
		var out []string
		depth := 0
		for _, l := range pf.lines[i+1:] {
			switch {
			case strings.HasPrefix(l, ":::") && strings.Contains(l, "{"):
				depth++
				continue
			case isDivClose(l):
				if depth == 0 {
					return out, true
				}
				depth--
				continue
			}
			if depth == 0 {
				out = append(out, l)
			}
		}
		return out, true
	}
	return nil, false
}

var (
	mdLink    = regexp.MustCompile(`\[((?:[^\[\]]|\\\[|\\\])*)\]\((wikipedia://en/([0-9]+)(?:#[^)]*)?|https?://[^)\s]+)\)`)
	footnote  = regexp.MustCompile(`\[\^[^\]]+\]`)
	heading   = regexp.MustCompile(`^#{1,6} `)
	boldLabel = regexp.MustCompile(`^\*\*[^*]+\*\*$`)
)

// convert gives the body text of the item from the lines of its block (WP9.2 step 3). It drops the headings, the images, the label of the item and the footnote marks, and it changes each link to a link to a node, or to its text when the target has no node. The words are not changed.
func convert(lines []string, pageNode func(int) string) string {
	var paras []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			paras = append(paras, strings.Join(cur, "\n"))
			cur = nil
		}
	}
	for _, l := range lines {
		l = strings.TrimRight(l, " \t")
		if l == "" {
			flush()
			continue
		}
		cur = append(cur, l)
	}
	flush()
	var out []string
	for i, p := range paras {
		switch {
		case heading.MatchString(p) && !strings.Contains(p, "\n"):
			continue
		case strings.HasPrefix(p, "*Image:"):
			continue
		case i <= 1 && boldLabel.MatchString(p) && len(out) == 0:
			continue
		}
		p = footnote.ReplaceAllString(p, "")
		p = mdLink.ReplaceAllStringFunc(p, func(s string) string {
			g := mdLink.FindStringSubmatch(s)
			text := g[1]
			if g[3] != "" {
				if page, err := strconv.Atoi(g[3]); err == nil {
					if t := pageNode(page); t != "" && !strings.ContainsAny(text, "|[]") {
						return "[[" + text + "|" + t + "]]"
					}
				}
			}
			return text
		})
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n\n")
}

// AdoptReport gives the report of the run: a YAML header with the counts, and a table of the items of each article.
func AdoptReport(res *AdoptResult, day string) []byte {
	count := map[string]int{}
	reasons := map[string]int{}
	for _, it := range res.Items {
		count[it.State]++
		if it.State == Waiting {
			r, _, _ := strings.Cut(it.Reason, ": ")
			r = pageNum.ReplaceAllString(r, "page P")
			r = nodeTag.ReplaceAllString(r, "node T")
			reasons[r]++
		}
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	fmt.Fprintf(&b, "source: wikipedia\ndate: %s\nitems: %d\nadopted: %d\ndrafted: %d\nlinked: %d\nwaiting: %d\ndeferred: %d\n", day, len(res.Items), count[Adopted], count[Drafted], res.Linked, count[Waiting], res.Deferred)
	b.WriteString("---\n\n# The adoption of Wikipedia text\n\n")
	fmt.Fprintf(&b, "This report gives the items of the Wikipedia articles that m2 corpus adopt read on %s (corpus wikipedia WP9.2). The run adopted %d items, %d items wait for m2 mint, and it set the outcome node on %d rows. %d items wait for the write role or for a person, and the run deferred %d of their rows.\n", day, count[Adopted], count[Drafted], res.Linked, count[Waiting], res.Deferred)
	if len(reasons) > 0 {
		b.WriteString("\n## Why items wait\n\n| Reason | Items |\n|---|---|\n")
		keys := make([]string, 0, len(reasons))
		for k := range reasons {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, func(a, b string) int {
			if reasons[a] != reasons[b] {
				return reasons[b] - reasons[a]
			}
			return strings.Compare(a, b)
		})
		for _, k := range keys {
			fmt.Fprintf(&b, "| %s | %d |\n", k, reasons[k])
		}
	}
	page := -1
	for _, it := range res.Items {
		if it.Page != page {
			page = it.Page
			fmt.Fprintf(&b, "\n## %s\n\nPage %d.\n\n| Mention | Kind | Cue | Name | State | Node or reason |\n|---|---|---|---|---|---|\n", cell(it.Title), it.Page)
		}
		what := it.Node
		if it.State == Waiting {
			what = it.Reason
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", cell(it.Mention), it.Kind, it.Cue, cell(it.Name), it.State, cell(what))
	}
	return b.Bytes()
}

var (
	pageNum = regexp.MustCompile(`page [0-9]+`)
	nodeTag = regexp.MustCompile(`node [~0-9A-Za-z]+`)
)

func cell(s string) string { return strings.ReplaceAll(s, "|", `\|`) }
