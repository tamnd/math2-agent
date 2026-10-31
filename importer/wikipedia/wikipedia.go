// Package wikipedia is the importer of the articles of Wikipedia (corpus wikipedia WP05 and WP12.4). It reads a checkout of the corpus repository tamnd/wikipedia-math and gives the mentions of WP5.1 and the edges of WP6.4. It replaces the package ingest/wikipedia. The ingest adapter wikipedia/1 of that package is now in dump.go.
//
// The importer reads these files of the checkout:
//
//	manifests/wikipedia/scope.tsv           the scope list (WP1.2)
//	metadata/<shard>/<pageid>.md            the metadata of each article
//	analysis/objects/<shard>/<pageid>-r<revision>.md the objects, with their tags (WP03, WP5.4)
//	analysis/edges/<shard>/<pageid>.md      the edges of each article (WP06)
//	tags/<shard>/<pageid>.md                the tag registers, for the tags that are gone (WP5.4)
//
// The id of an article mention is wikipedia://en/<pageid>, and the id of an object mention is wikipedia://en/<pageid>#<tag> (WP5.2). The text of the articles is CC BY-SA, and the record plane is CC0. Thus a mention holds no text. The field text_ref names the span of the item in the content plane, and the field hash is its content hash (rule Y05).
package wikipedia

import (
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/wpcorpus"
)

// Source is the source of the mentions.
const Source = "wikipedia"

// Version is the version of the importer.
const Version = "importer/wikipedia/1"

// Licence is the licence of the text of each article.
const Licence = "CC-BY-SA-4.0"

// Result is what one import gives.
type Result struct {
	Mentions []inventory.Mention
	Edges    []inventory.Edge
	Articles int
	// Objects is the objects of an item kind, which rule Y04 counts.
	Objects int
	// Untagged is the objects of an item kind that have no tag, as <pageid>#<ref>, so they get no mention. Run m2 wp extract on the checkout first.
	Untagged []string
	// NotExtracted is the articles in scope that have no metadata or no objects file for the revision of record.
	NotExtracted []int
	// StaleEdges is the articles whose edge file is of another revision, so their edges are not imported. Run m2 wp edges first.
	StaleEdges []int
	Attached   int
	Members    int
	// Retired is the mentions of an earlier import whose tag is gone (WP5.4).
	Retired int
	// Stopped is the mentions of an earlier import whose article is no longer in the scope or is not extracted.
	Stopped int
	// RedLinks is the edges to a page that does not exist. They are not imported.
	RedLinks int
	// other is the articles in scope that the filter did not select.
	other map[int]bool
}

// KeepEdges adds the edges of an earlier import whose source article is in scope and not selected by the filter, so that an import of a part of the scope keeps the edges of the other articles.
func (res *Result) KeepEdges(old []inventory.Edge) {
	for _, e := range old {
		if res.other[edgePage(e.From)] {
			res.Edges = append(res.Edges, e)
		}
	}
}

// edgePage gives the page id of a mention id of this source, or 0.
func edgePage(id string) int {
	s, ok := strings.CutPrefix(id, "wikipedia://en/")
	if !ok {
		return 0
	}
	s, _, _ = strings.Cut(s, "#")
	p, _ := strconv.Atoi(s)
	return p
}

// Import reads the checkout of the corpus repository at root. The mentions of an earlier import, in old, keep their ids: a mention whose tag is gone is retired, and a mention whose article is not in this import gets the flag stopped.
func Import(root string, old []inventory.Mention) (*Result, error) {
	return ImportPart(root, old, Filter{})
}

// Filter selects a part of the scope for an import (WP12.6). An empty field selects each article.
type Filter struct {
	Type  string // the article type of WP1.5, such as statement
	MSC   string // the start of an MSC code of the article, such as 05 or 05C
	Since string // a date as 2026-10-01. The article is selected when its revision of record is of this day or later.
}

func (f Filter) takes(md *wpcorpus.Metadata) bool {
	if f.Type != "" && articleType(md) != f.Type {
		return false
	}
	if f.MSC != "" && !slices.ContainsFunc(md.MSC, func(c string) bool { return strings.HasPrefix(c, f.MSC) }) {
		return false
	}
	return f.Since == "" || md.Timestamp >= f.Since
}

// ImportPart is Import for the articles that the filter selects. A mention of an earlier import whose article is in scope and not selected stays as it is.
func ImportPart(root string, old []inventory.Mention, f Filter) (*Result, error) {
	snapshot, err := commit(root)
	if err != nil {
		return nil, err
	}
	pages, meta, err := scopePages(root)
	if err != nil {
		return nil, err
	}
	// kinds is the mention kind of each article, for the member mentions.
	kinds := map[int]string{}
	for _, p := range pages {
		if md := meta[p]; md != nil {
			kinds[p], _ = ArticleKind(articleType(md), md.Title, md.Categories)
		}
	}
	res := &Result{other: map[int]bool{}}
	done := map[int]bool{}
	gone := map[string]int{}
	for _, p := range pages {
		md := meta[p]
		if md == nil {
			res.NotExtracted = append(res.NotExtracted, p)
			continue
		}
		if !f.takes(md) {
			res.other[p] = true
			continue
		}
		objs, err := wpcorpus.ReadObjects(root, p, md.Revision)
		if errors.Is(err, fs.ErrNotExist) {
			res.NotExtracted = append(res.NotExtracted, p)
			continue
		}
		if err != nil {
			return nil, err
		}
		var edges []wpcorpus.Edge
		f, err := wpcorpus.ReadEdges(root, p)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return nil, err
		case f.Revision != md.Revision:
			res.StaleEdges = append(res.StaleEdges, p)
		default:
			edges = f.Edges
		}
		tags, err := wpcorpus.ReadTags(root, p)
		if err != nil {
			return nil, err
		}
		for _, l := range tags {
			if r, err := strconv.Atoi(strings.TrimPrefix(l.Gone, "r")); err == nil && l.Gone != "" {
				gone[wpcorpus.MentionID(p, l.Tag)] = r
			}
		}
		a := &article{md: md, snapshot: snapshot, kinds: kinds, objs: objs, edges: edges}
		a.add(res)
		done[p] = true
		res.Articles++
	}
	res.keep(old, done, gone)
	slices.SortFunc(res.Mentions, func(a, b inventory.Mention) int { return strings.Compare(a.ID, b.ID) })
	return res, nil
}

// scopePages returns the page ids of the articles in scope, in order, and the metadata of each article that has metadata or a content file.
func scopePages(root string) ([]int, map[int]*wpcorpus.Metadata, error) {
	all, err := wpcorpus.ReadScopeFile(root, wpcorpus.ScopePath)
	if err != nil {
		return nil, nil, err
	}
	metas, err := wpcorpus.ReadMetadataFiles(root)
	if err != nil {
		return nil, nil, err
	}
	meta := map[int]*wpcorpus.Metadata{}
	for _, md := range metas {
		meta[md.Page] = md
	}
	var pages []int
	for _, c := range all {
		if !c.InScope {
			continue
		}
		p := int(c.PageID)
		pages = append(pages, p)
		// A corpus with no metadata plane has the facts of the content
		// header and of the scope list.
		if meta[p] == nil {
			cp, err := wpcorpus.ReadContent(root, p)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, nil, err
			}
			meta[p] = &wpcorpus.Metadata{Page: p, Title: cp.Title, Revision: cp.Revision, Timestamp: cp.Timestamp, Class: c.Class, Importance: c.Importance}
		}
	}
	slices.Sort(pages)
	return pages, meta, nil
}

// Want is one mention that an import of the checkout must give (rule Y04).
type Want struct {
	File    string // the objects file, relative to the checkout
	Ref     string // the ref of the object, or empty for the article
	Mention string // the mention id, or empty when the object has no tag
}

// Expect returns the mentions that an import of the checkout at root must give: one for each article in scope that has an objects file for its revision of record, and one for each object of an item kind of that file. An article with no objects file gives no want, because rules Y01 and Y02 report it.
func Expect(root string) ([]Want, error) {
	pages, meta, err := scopePages(root)
	if err != nil {
		return nil, err
	}
	var out []Want
	for _, p := range pages {
		md := meta[p]
		if md == nil {
			continue
		}
		objs, err := wpcorpus.ReadObjects(root, p, md.Revision)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		file := filepath.ToSlash(wpcorpus.ObjectsPath(p, md.Revision))
		out = append(out, Want{File: file, Mention: wpcorpus.MentionID(p, "")})
		for _, o := range objs {
			if !Wanted(o.Kind) {
				continue
			}
			w := Want{File: file, Ref: o.Ref}
			if o.Tag != "" {
				w.Mention = wpcorpus.MentionID(p, o.Tag)
			}
			out = append(out, w)
		}
	}
	return out, nil
}

// keep adds the mentions of an earlier import that this import does not give, so that their ids stay.
func (res *Result) keep(old []inventory.Mention, done map[int]bool, gone map[string]int) {
	have := map[string]bool{}
	for _, m := range res.Mentions {
		have[m.ID] = true
	}
	for _, m := range old {
		if m.Source != Source || !strings.HasPrefix(m.By, "importer/wikipedia") || have[m.ID] {
			continue
		}
		switch {
		case res.other[m.Page]:
		case !done[m.Page]:
			if !slices.Contains(m.Flags, "stopped") {
				m.Flags = append(m.Flags, "stopped")
			}
			res.Stopped++
		case gone[m.ID] != 0:
			m.Retired = gone[m.ID]
			res.Retired++
		default:
			continue
		}
		res.Mentions = append(res.Mentions, m)
	}
}

func articleType(md *wpcorpus.Metadata) string {
	if wpcorpus.IsType(md.Type) {
		return md.Type
	}
	return wpcorpus.TypeOf(md.Title, md.Short, md.Categories)
}

var conjectureWord = regexp.MustCompile(`(?i)\b(conjectures?|hypothesis|unsolved)\b`)

// ArticleKind returns the mention kind and the role of an article of the type (WP5.1). Spec 04.2 has no kind field and no kind article, so a field, a list, a biography and the other types give topic.
func ArticleKind(typ, title string, categories []string) (kind, role string) {
	switch typ {
	case wpcorpus.TypeConcept, wpcorpus.TypeMethod:
		return "definition", ""
	case wpcorpus.TypeStatement:
		return "statement", ""
	case wpcorpus.TypeObject:
		return "object", ""
	case wpcorpus.TypeProblem:
		if conjectureWord.MatchString(title) || slices.ContainsFunc(categories, conjectureWord.MatchString) {
			return "conjecture", ""
		}
		return "problem", ""
	}
	return "topic", ""
}

// Wanted reports whether an object of the kind gets a mention (WP5.1). The subject of the lead has no mention of its own, because the article mention is the subject.
func Wanted(kind string) bool {
	k, _, _ := strings.Cut(kind, "/")
	switch k {
	case "subject", "section", "equation", "citation", "infobox", "figure", "table", "table-row":
		return false
	}
	return true
}

// KindOf returns the mention kind and the role of an object of the kind (WP5.1).
func KindOf(kind string) (string, string) {
	k, sub, _ := strings.Cut(kind, "/")
	switch k {
	case "statement":
		switch sub {
		case "theorem", "lemma", "proposition", "corollary", "fact", "axiom":
			return "statement", sub
		}
		return "statement", ""
	case "example":
		if sub == "counterexample" {
			return "counterexample", ""
		}
		return "example", ""
	case "application":
		return "example", ""
	case "definition", "counterexample", "conjecture", "problem", "proof", "exercise":
		return k, ""
	case "entry":
		return "definition", ""
	}
	return inventory.Unknown, ""
}

// Name returns the name of an article: the title without the disambiguator, as Lagrange's theorem for Lagrange's theorem (group theory).
func Name(title string) string {
	if i := strings.LastIndex(title, " ("); i > 0 && strings.HasSuffix(title, ")") {
		return title[:i]
	}
	return title
}

// Permalink returns the permanent link of a revision, with the anchor.
func Permalink(rev int, anchor string) string {
	s := "https://en.wikipedia.org/w/index.php?oldid=" + strconv.Itoa(rev)
	if anchor != "" {
		s += "#" + strings.ReplaceAll(anchor, " ", "_")
	}
	return s
}

// article is one article of the import, with what its mentions need.
type article struct {
	md       *wpcorpus.Metadata
	snapshot string
	kinds    map[int]string
	objs     []wpcorpus.Object
	edges    []wpcorpus.Edge
}

func (a *article) base() inventory.Mention {
	m := inventory.Mention{
		Source:      Source,
		Snapshot:    a.snapshot,
		Licence:     Licence,
		MSC:         a.md.MSC,
		Class:       "open",
		By:          Version,
		Page:        a.md.Page,
		Revision:    a.md.Revision,
		ArticleType: articleType(a.md),
	}
	if a.md.Wikidata != "" {
		m.Same = map[string][]string{"wikidata": {a.md.Wikidata}}
	}
	if a.md.Class != "" || a.md.Importance != "" {
		m.Assessment = a.md.Class + "/" + a.md.Importance
	}
	if a.md.Unstable {
		m.Flags = append(m.Flags, "unstable")
	}
	return m
}

func (a *article) add(res *Result) {
	p := a.md.Page
	art := wpcorpus.MentionID(p, "")
	byRef := map[string]*wpcorpus.Object{}
	for i := range a.objs {
		byRef[a.objs[i].Ref] = &a.objs[i]
	}
	ids := map[string]string{}
	for _, o := range a.objs {
		if Wanted(o.Kind) && o.Tag != "" {
			ids[o.Ref] = wpcorpus.MentionID(p, o.Tag)
		}
	}
	// proofs is the proofs of each mention.
	proofs := map[string][]string{}
	for _, o := range a.objs {
		if o.Kind == "proof" && ids[o.Ref] != "" {
			to := attach(o, byRef, ids, art)
			proofs[to] = append(proofs[to], ids[o.Ref])
		}
	}

	m := a.base()
	m.ID = art
	m.Kind, m.Role = ArticleKind(m.ArticleType, a.md.Title, a.md.Categories)
	m.Name = Name(a.md.Title)
	m.NativeID = a.md.Title
	m.Locator = a.md.Title + ", revision " + strconv.Itoa(a.md.Revision) + ", " + Permalink(a.md.Revision, "")
	m.TextRef = filepath.ToSlash(wpcorpus.WikitextPath(p)) + "@r" + strconv.Itoa(a.md.Revision)
	for _, r := range a.md.Redirects {
		if r != a.md.Title && !slices.Contains(m.Aliases, r) {
			m.Aliases = append(m.Aliases, r)
			m.AliasesFrom = append(m.AliasesFrom, "redirect")
		}
	}
	for _, o := range a.objs {
		if o.Kind == "subject" {
			m.Hash, m.Cue, m.Strength, m.Extractor = o.Hash, o.Cue, o.Strength, o.Extractor
			m.Flags = append(m.Flags, o.Flags...)
			break
		}
	}
	m.Structure = a.structure(art, "", proofs)
	res.Mentions = append(res.Mentions, m)

	for _, o := range a.objs {
		if !Wanted(o.Kind) {
			continue
		}
		res.Objects++
		if o.Tag == "" {
			res.Untagged = append(res.Untagged, fmt.Sprintf("%d#%s", p, o.Ref))
			continue
		}
		m := a.object(o)
		if o.Kind == "proof" {
			m.AttachedTo = attach(o, byRef, ids, art)
			res.Attached++
		}
		a.member(&m, o)
		if m.MemberOf != "" {
			res.Members++
		}
		m.Structure = a.structure(m.ID, art, proofs)
		res.Mentions = append(res.Mentions, m)
	}

	for _, e := range a.edges {
		r := inventory.Edge{From: e.From, Predicate: e.Predicate, Method: e.Method, Confidence: e.Confidence, Span: e.Span, Revision: e.Revision, Finding: e.Finding, By: Version}
		switch {
		case e.Finding == wpcorpus.FindingRedLink:
			res.RedLinks++
			continue
		case e.To != "":
			r.To = e.To
		case e.Page != 0:
			r.To = wpcorpus.MentionID(e.Page, "")
		case strings.HasPrefix(e.Target, "Category:") || strings.HasPrefix(e.Target, "Template:"):
			r.To = e.Target
		default:
			// A title that the edge pass did not resolve has no id yet.
			continue
		}
		res.Edges = append(res.Edges, r)
	}
}

func (a *article) object(o wpcorpus.Object) inventory.Mention {
	m := a.base()
	m.ID = wpcorpus.MentionID(a.md.Page, o.Tag)
	m.Kind, m.Role = KindOf(o.Kind)
	m.Name = o.Name
	for _, al := range o.Aliases {
		m.Aliases = append(m.Aliases, al)
		m.AliasesFrom = append(m.AliasesFrom, "anchor-redirect")
	}
	m.Section = o.Section
	m.Anchor = o.Anchor
	if m.Anchor == "" && (o.Cue == "heading" || o.Cue == "named-heading") {
		m.Anchor = o.SectionAnchor
	}
	m.Cue, m.Strength = o.Cue, o.Strength
	m.Locator = a.md.Title + ", revision " + strconv.Itoa(a.md.Revision)
	if o.Section != "" {
		m.Locator += ", section " + o.Section
	}
	m.Locator += ", " + Permalink(a.md.Revision, cmpOr(m.Anchor, o.SectionAnchor))
	// Pass 1 of WP8.2 joins two mentions with a key in common. The Wikidata id names the subject of the article, not each object in it, so an object mention does not have it. The native id of an object is the title and the tag, as for an object of arXiv, so that it is not the key of an object of another article.
	m.Same = nil
	m.NativeID = a.md.Title + "#" + o.Tag
	m.TextRef = fmt.Sprintf("%s@r%d#%d-%d", filepath.ToSlash(wpcorpus.WikitextPath(a.md.Page)), a.md.Revision, o.Span[0], o.Span[1])
	m.Hash = o.Hash
	m.Flags = append(m.Flags, o.Flags...)
	m.Extractor = o.Extractor
	return m
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// attach returns the mention that a proof proves: the mention of its target, or the article mention when the target is the subject of the lead or has no mention.
func attach(o wpcorpus.Object, byRef map[string]*wpcorpus.Object, ids map[string]string, art string) string {
	if t := byRef[o.AttachedTo]; t != nil && ids[t.Ref] != "" && t.Kind != "proof" {
		return ids[t.Ref]
	}
	return art
}

// member makes the mention a member of an article (WP5.1 and WP5.7). An entry of a glossary or a list is a member of the article in scope that its first link names. A section item under {{Main}} is a member of the main article when the kinds agree. A list entry with no article gets the kind unknown and the hint list-entry, for the prose pass.
func (a *article) member(m *inventory.Mention, o wpcorpus.Object) {
	own := wpcorpus.MentionID(a.md.Page, "") + "#"
	for _, e := range a.edges {
		if e.From != m.ID || e.To == "" || strings.HasPrefix(e.To, own) || e.To == strings.TrimSuffix(own, "#") {
			continue
		}
		to, _, _ := strings.Cut(e.To, "#")
		page, _ := strconv.Atoi(strings.TrimPrefix(to, "wikipedia://en/"))
		switch {
		case o.Kind == "entry" && (e.Method == wpcorpus.MethodGlossary || e.Method == wpcorpus.MethodList):
			m.MemberOf = to
			if o.Cue == "list-entry" {
				m.Kind = cmpOr(a.kinds[page], inventory.Unknown)
			}
			return
		case e.Predicate == wpcorpus.PredRestates && e.Method == wpcorpus.MethodHatnote && a.kinds[page] == m.Kind:
			m.MemberOf = to
			return
		}
	}
	if o.Kind == "entry" && o.Cue == "list-entry" {
		m.Kind = inventory.Unknown
		m.Hints = append(m.Hints, "list-entry")
	}
}

// structure returns the edges of a mention inside its article: what it uses, what proves it and what it is in.
func (a *article) structure(id, in string, proofs map[string][]string) *inventory.Structure {
	own := wpcorpus.MentionID(a.md.Page, "")
	s := &inventory.Structure{In: in, ProvedBy: proofs[id]}
	for _, e := range a.edges {
		if e.From == id && e.Predicate == wpcorpus.PredUses && e.To != id && (e.To == own || strings.HasPrefix(e.To, own+"#")) && !slices.Contains(s.Uses, e.To) {
			s.Uses = append(s.Uses, e.To)
		}
	}
	slices.Sort(s.Uses)
	slices.Sort(s.ProvedBy)
	if len(s.Uses) == 0 && len(s.ProvedBy) == 0 && s.In == "" {
		return nil
	}
	return s
}

// commit returns the HEAD commit of the checkout, which is the snapshot of the mentions.
func commit(root string) (string, error) {
	out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output() // #nosec G204 -- the command is git, and root is the checkout that the user names
	if err != nil {
		return "", fmt.Errorf("importer/wikipedia: %s is not a git checkout of the corpus repository: %w", root, err)
	}
	return strings.TrimSpace(string(out)), nil
}
