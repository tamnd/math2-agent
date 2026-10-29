// Package arxiv is the importer of the items of arXiv papers (corpus arxiv AX05 and AX12.4). It reads the analysis plane of a checkout of tamnd/arxiv and gives one mention for each item of each paper.
//
// The importer reads these files of the checkout:
//
//	analysis/objects/<YYMM>/<id>.md      the object records of each version (AX03)
//	analysis/edges/<YYMM>/<id>.md        the in-paper edges (AX06)
//	analysis/census/<YYMM>/<id>v<n>.json the census of each version (AX04)
//	analysis/bib/<YYMM>/<id>.md          the bibliography entries of each version, for the cited results (AX5.7)
//	metadata/<YYMM>.md                   the metadata of the papers, for the licence and the MSC codes
//
// A file of records is Markdown with the records in a YAML header (package mdrec). A checkout that has not moved to Markdown has a .jsonl file of the same name, one record on each line, and the importer reads it.
//
// The id of a mention is ax://paper/<id>#<tag>, where the tag is the tag of the object in the analysis plane. The id has no version, because the tag stays on its object from version to version. The mention describes the latest version that has the object. A mention whose object is not in the latest version of the paper is retired.
//
// The importer does not copy the text of an item. The field text_ref names the object record, and the field hash is its content hash. A version of the class record gives no name and no text_ref, so its mentions hold only ids, labels, numbers, hashes, codes and values from closed lists (rule D06).
//
// Which objects become mentions is the table of AX5.1. A proof is attached to what it proves, and an item inside a proof is attached to the proof (AX5.2). A restated item is one mention, from its first print, and the other prints add their locators to the field also (AX5.5).
package arxiv

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/mdrec"
)

// Source is the source of the mentions.
const Source = "arxiv"

// Version is the version of the importer.
const Version = "importer/arxiv/1"

// Object is the part of an object record of tamnd/arxiv that the importer reads.
type Object struct {
	Tag         string   `json:"tag"`
	Kind        string   `json:"kind"`
	Subkind     string   `json:"subkind"`
	Paper       string   `json:"paper"`
	Version     string   `json:"version"`
	Local       string   `json:"local"`
	Label       string   `json:"label"`
	Number      string   `json:"number"`
	Section     string   `json:"section"`
	Order       int      `json:"order"`
	Title       string   `json:"title"`
	BodyMD      string   `json:"body_md"`
	Path        string   `json:"path"`
	Env         string   `json:"env"`
	Item        string   `json:"item"`
	Head        string   `json:"head"`
	Name        string   `json:"name"`
	Parent      string   `json:"parent"`
	Scope       string   `json:"scope"`
	ProofStatus string   `json:"proof_status"`
	Proves      string   `json:"proves"`
	Origin      string   `json:"origin"`
	Restates    string   `json:"restates"`
	Flags       []string `json:"flags"`
	Hash        string   `json:"hash"`
	Extractor   string   `json:"extractor"`
}

// Edge is the part of an edge record of tamnd/arxiv that the importer reads.
type Edge struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Predicate  string `json:"predicate"`
	Part       string `json:"part"`
	Confidence string `json:"confidence"`
	Evidence   struct {
		Version int `json:"version"`
	} `json:"evidence"`
}

// Paper is the part of a metadata record of tamnd/arxiv that the importer reads.
type Paper struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Categories []string `json:"categories"`
	MSCClass   string   `json:"msc_class"`
	Versions   []struct {
		Version int    `json:"version"`
		Licence string `json:"licence"`
	} `json:"versions"`
}

// Licence returns the licence of the version, or of the latest earlier version that states one.
func (p *Paper) Licence(version int) string {
	l := ""
	for _, v := range p.Versions {
		if v.Version <= version && v.Licence != "" {
			l = v.Licence
		}
	}
	return l
}

// ClassOf returns the class of spec 9.3 of a licence of the corpus (AX2.3). CC0, CC BY and CC BY-SA are open. CC BY-NC-SA is share-alike-nc. CC BY-NC-ND is verbatim. The arXiv licence, no licence and an unknown licence are record.
func ClassOf(licence string) string {
	switch licence {
	case "cc0", "cc-by", "cc-by-sa":
		return "open"
	case "cc-by-nc-sa":
		return "share-alike-nc"
	case "cc-by-nc-nd":
		return "verbatim"
	}
	return "record"
}

// Result is what one import gives.
type Result struct {
	Mentions []inventory.Mention
	Papers   int
	// Objects is the objects of an item kind in the latest version of each paper, which rule D05 counts.
	Objects int
	// Untagged is the objects of an item kind that have no tag, so they get no mention. Run ax tags analysis on the checkout first.
	Untagged []string
	Attached int
	Retired  int
	Restated int
	// CitedMentions is the cited results that go to a mention of a paper of the import, and CitedWorks is the cited results that go to the record id of a work, as arxiv:<id> or doi:<doi> (AX5.7). The other cited results keep the key bib:<key>, because the bibliography does not resolve it.
	CitedMentions int
	CitedWorks    int
}

// Import reads the checkout of tamnd/arxiv at root.
func Import(root string) (*Result, error) { return ImportNames(root, nil) }

// ImportNames reads the checkout of tamnd/arxiv at root, and gives each mention the rows of the named results that its text names (AX6.2, the name method). With no names, it is Import.
func ImportNames(root string, names *Names) (*Result, error) {
	snapshot, err := commit(root)
	if err != nil {
		return nil, err
	}
	files, err := mdrec.Glob(filepath.Join(root, "analysis", "objects", "*", "*"+mdrec.Ext))
	if err != nil {
		return nil, err
	}
	months := map[string]bool{}
	for _, f := range files {
		months[filepath.Base(filepath.Dir(f))] = true
	}
	meta, err := readMetadata(root, months)
	if err != nil {
		return nil, err
	}
	res := &Result{}
	for _, f := range files {
		objs, err := readObjects(f)
		if err != nil {
			return nil, err
		}
		if len(objs) == 0 {
			continue
		}
		month := filepath.Base(filepath.Dir(f))
		id := objs[0].Paper
		es, err := readLines[Edge](filepath.Join(root, "analysis", "edges", month, filepath.Base(f)))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		bs, err := readLines[Bib](filepath.Join(root, "analysis", "bib", month, filepath.Base(f)))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		p := paperIn{root: root, month: month, id: id, file: filepath.ToSlash(filepath.Join("analysis", "objects", month, filepath.Base(f))), snapshot: snapshot, meta: meta[id], names: names, bib: bibOf(bs)}
		p.add(res, objs, es)
		res.Papers++
	}
	res.CitedMentions, res.CitedWorks = resolveCites(res.Mentions)
	slices.SortFunc(res.Mentions, func(a, b inventory.Mention) int { return strings.Compare(a.ID, b.ID) })
	return res, nil
}

// Wanted reports whether an object of the item kind gets a mention (AX5.1).
func Wanted(item string) bool {
	k, _, _ := strings.Cut(item, "/")
	switch k {
	case "statement", "definition", "example", "counterexample", "conjecture", "problem", "exercise", "proof", "unknown":
		return true
	}
	return false
}

// KindOf returns the mention kind and the role of an object of the item kind (AX5.1).
func KindOf(item string) (kind, role string) {
	k, sub, _ := strings.Cut(item, "/")
	switch k {
	case "statement":
		switch sub {
		case "theorem", "lemma", "proposition", "corollary", "fact", "axiom":
			return "statement", sub
		case "claim":
			return "statement", "lemma"
		}
		return "statement", ""
	case "example":
		return "example", ""
	}
	return k, ""
}

// MentionID returns the id of the mention of the object with the tag in the paper.
func MentionID(paper, tag string) string { return "ax://paper/" + paper + "#" + tag }

type paperIn struct {
	root, month, id, file, snapshot string
	meta                            *Paper
	names                           *Names
	bib                             map[int]map[string]Bib
}

func (p *paperIn) add(res *Result, objs []Object, es []Edge) {
	byVersion := map[int][]Object{}
	latest := 0
	for _, o := range objs {
		if o.Path != "source" {
			continue
		}
		v, _ := strconv.Atoi(o.Version)
		byVersion[v] = append(byVersion[v], o)
		latest = max(latest, v)
	}
	if latest == 0 {
		return
	}
	var versions []int
	for v := range byVersion {
		versions = append(versions, v)
	}
	slices.Sort(versions)
	// done is the tags that have a mention, from the latest version first.
	done := map[string]bool{}
	start := len(res.Mentions)
	for i := len(versions) - 1; i >= 0; i-- {
		v := versions[i]
		vp := p.version(v, byVersion[v], es)
		for _, o := range byVersion[v] {
			if !Wanted(o.Item) {
				continue
			}
			if v == latest {
				res.Objects++
				if o.Tag == "" {
					res.Untagged = append(res.Untagged, fmt.Sprintf("%sv%d#%s", p.id, v, o.Local))
					continue
				}
			}
			if o.Tag == "" || done[o.Tag] || o.Restates != "" {
				continue
			}
			done[o.Tag] = true
			m := vp.mention(o)
			for _, w := range versions {
				if w == v {
					continue
				}
				for _, x := range byVersion[w] {
					if x.Tag == o.Tag {
						m.Versions = append(m.Versions, inventory.VersionRef{Version: w, Number: x.Number})
					}
				}
			}
			if v != latest {
				m.Retired = latest
				res.Retired++
			}
			if m.Attached() {
				res.Attached++
			}
			res.Mentions = append(res.Mentions, m)
		}
		// The other prints of a restated item add their locators to the mention of the first print.
		for _, o := range byVersion[v] {
			if o.Restates == "" || !Wanted(o.Item) || o.Tag == "" {
				continue
			}
			t := vp.target(o.Restates)
			for j := start; j < len(res.Mentions); j++ {
				if t != "" && res.Mentions[j].ID == t && res.Mentions[j].Version == v && !slices.Contains(res.Mentions[j].Also, vp.locator(o)) {
					res.Mentions[j].Also = append(res.Mentions[j].Also, vp.locator(o))
					res.Restated++
				}
			}
		}
	}
}

// versionIn is one version of a paper, with what its mentions need.
type versionIn struct {
	*paperIn
	v       int
	class   string
	census  string
	msc     []string
	byLocal map[string]*Object
	edges   []Edge
}

func (p *paperIn) version(v int, objs []Object, es []Edge) *versionIn {
	vp := &versionIn{paperIn: p, v: v, byLocal: map[string]*Object{}}
	for i := range objs {
		vp.byLocal[objs[i].Local] = &objs[i]
	}
	vp.class = "record"
	if p.meta != nil {
		vp.class = ClassOf(p.meta.Licence(v))
		vp.msc = MSC(p.meta.MSCClass)
		if len(vp.msc) == 0 {
			vp.msc = CategoryMSC(p.meta.Categories)
		}
	}
	vp.census = readStrength(filepath.Join(p.root, "analysis", "census", p.month, fmt.Sprintf("%sv%d.json", p.id, v)))
	for _, e := range es {
		if e.Evidence.Version == v {
			vp.edges = append(vp.edges, e)
		}
	}
	return vp
}

// target returns the mention id of the object with the local name, through a restatement, or "" when it has no tag.
func (vp *versionIn) target(local string) string {
	o := vp.byLocal[local]
	if o != nil && o.Restates != "" && vp.byLocal[o.Restates] != nil {
		o = vp.byLocal[o.Restates]
	}
	if o == nil || o.Tag == "" {
		return ""
	}
	return MentionID(vp.id, o.Tag)
}

// attach returns the mention that an attached mention goes to: the mention of the object with the local name, or "unknown". An object that gets no mention, as a remark with a proof, gives "unknown" and the hint target-not-an-item.
func (vp *versionIn) attach(local string, m *inventory.Mention) string {
	if o := vp.byLocal[local]; o != nil && !Wanted(o.Item) {
		m.Hints = append(m.Hints, "target-not-an-item")
		return "unknown"
	}
	if t := vp.target(local); t != "" {
		return t
	}
	return "unknown"
}

// localOf returns the local name of an end of an edge, or the end itself when it is not in the paper, as bib:<key>.
func localOf(id string) string {
	if _, l, ok := strings.Cut(id, "#"); ok {
		return l
	}
	return id
}

func (vp *versionIn) locator(o Object) string {
	ref := fmt.Sprintf("%sv%d", vp.id, vp.v)
	if vp.class == "record" {
		return ref + "#" + o.Local
	}
	head := o.Title
	if head == "" {
		head = o.Item
	}
	s := ref + " " + strings.TrimSpace(head+" "+o.Number)
	if sec := vp.byLocal[o.Section]; sec != nil && sec.Number != "" {
		s += ", section " + sec.Number
	}
	return s
}

func (vp *versionIn) mention(o Object) inventory.Mention {
	kind, role := KindOf(o.Item)
	m := inventory.Mention{
		ID:          MentionID(vp.id, o.Tag),
		Source:      Source,
		Snapshot:    vp.snapshot,
		Locator:     vp.locator(o),
		NativeID:    vp.id + "#" + o.Tag,
		Kind:        kind,
		Role:        role,
		Label:       o.Label,
		MSC:         vp.msc,
		Class:       vp.class,
		By:          Version,
		Hash:        hashOf(o),
		Version:     vp.v,
		Number:      o.Number,
		Env:         o.Env,
		Head:        o.Head,
		Origin:      o.Origin,
		ProofStatus: o.ProofStatus,
		Census:      vp.census,
		Flags:       o.Flags,
		Extractor:   o.Extractor,
	}
	if vp.class != "record" {
		m.Name = HeadName(o.Name)
		m.TextRef = vp.file + "#" + o.Local + "@v" + strconv.Itoa(vp.v)
	}
	if kind == "problem" && yesNo(o.BodyMD) {
		m.Hints = append(m.Hints, "conjecture-candidate")
	}
	switch {
	case kind == "proof":
		m.AttachedTo = vp.attach(o.Proves, &m)
	case o.Scope == "local":
		m.AttachedTo = vp.attach(o.Parent, &m)
	}
	s := &inventory.Structure{}
	for _, e := range vp.edges {
		from, to := localOf(e.From), localOf(e.To)
		switch {
		case from == o.Local && e.Predicate == "cites-result":
			if key, ok := strings.CutPrefix(to, "bib:"); ok {
				if w := vp.work(key); w != "" {
					to = w
				}
			}
			m.CitesResult = append(m.CitesResult, inventory.CitedResult{To: to, Locator: e.Part, Confidence: e.Confidence})
		case from == o.Local && e.Predicate == "uses" && !strings.HasPrefix(to, "bib:"):
			if t := vp.target(to); t != "" && !slices.Contains(s.Uses, t) {
				s.Uses = append(s.Uses, t)
			}
		case to == o.Local && e.Predicate == "proves":
			if t := vp.target(from); t != "" && !slices.Contains(s.ProvedBy, t) {
				s.ProvedBy = append(s.ProvedBy, t)
			}
		}
	}
	s.In = vp.target(o.Section)
	s.Names = vp.names.Find(o.BodyMD, vp.msc, o.Name)
	slices.Sort(s.Uses)
	slices.Sort(s.ProvedBy)
	if len(s.Uses) > 0 || len(s.ProvedBy) > 0 || s.In != "" || len(s.Names) > 0 {
		m.Structure = s
	}
	return m
}

// MinHashText is the shortest text of a body, with no labels and no space, that gives the mention its content hash. A short body, such as an empty one or one label, has the same hash in many papers, and the hash pass of AX8.2 must not join these items.
const MinHashText = 40

var labelCmd = regexp.MustCompile(`\\label\{[^}]*\}`)

// hashOf returns the content hash of the object, or "" when its body is too short to tell one item from another.
func hashOf(o Object) string {
	text := strings.Join(strings.Fields(labelCmd.ReplaceAllString(o.BodyMD, "")), "")
	if utf8.RuneCountInString(text) < MinHashText {
		return ""
	}
	return o.Hash
}

// yesNo reports whether the text of a problem is a yes or no question: it ends with a question mark and starts with a verb that asks one.
func yesNo(body string) bool {
	body = strings.TrimSpace(body)
	if !strings.HasSuffix(body, "?") {
		return false
	}
	first, _, _ := strings.Cut(body, " ")
	switch strings.ToLower(strings.Trim(first, "*_.:")) {
	case "is", "are", "does", "do", "can", "must", "should", "will", "has", "have":
		return true
	}
	return false
}

// readStrength reads the census strength of a version, or "" when the version has no census.
func readStrength(file string) string {
	b, err := os.ReadFile(file) // #nosec G304 -- the file is in the checkout that the user names
	if err != nil {
		return ""
	}
	var c struct {
		Strength string `json:"strength"`
	}
	if json.Unmarshal(b, &c) != nil {
		return ""
	}
	return c.Strength
}

// readMetadata reads the metadata of the papers of the months.
func readMetadata(root string, months map[string]bool) (map[string]*Paper, error) {
	out := map[string]*Paper{}
	for m := range months {
		ps, err := readLines[Paper](filepath.Join(root, "metadata", m+mdrec.Ext))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for i := range ps {
			out[ps[i].ID] = &ps[i]
		}
	}
	return out, nil
}

// readLines reads the records of a file of records, or of the .jsonl file of the same name in a checkout that has not moved to Markdown.
func readLines[T any](file string) ([]T, error) {
	recs, err := mdrec.Read(file)
	if err != nil {
		return nil, err
	}
	out, err := mdrec.Unmarshal[T](recs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return out, nil
}

// readObjects reads an objects file of tamnd/arxiv, and gives each object of the kind unknown the kind of ItemOf.
func readObjects(file string) ([]Object, error) {
	objs, err := readLines[Object](file)
	for i := range objs {
		objs[i].Item = ItemOf(&objs[i])
	}
	return objs, err
}

// commit returns the HEAD commit of the checkout, which is the snapshot of the mentions.
func commit(root string) (string, error) {
	out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output() // #nosec G204 -- the command is git, and root is the checkout that the user names
	if err != nil {
		return "", fmt.Errorf("importer/arxiv: %s is not a git checkout of tamnd/arxiv: %w", root, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// mscCode matches an MSC code: 11N05, 11Nxx or 11-XX, and 11-02.
var mscCode = regexp.MustCompile(`\b\d\d(?:[A-Z]\d\d|[A-Z]xx|-XX|-\d\d)\b`)

// MSC returns the MSC codes in the free text of the msc-class field of arXiv, in the order of the text and without repeats. It is the rule of the ingest adapter arxiv. This package does not import that adapter, because the import registers the adapter.
func MSC(text string) []string {
	var out []string
	for _, c := range mscCode.FindAllString(text, -1) {
		if !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// Want is one object of the latest version of a paper that must have a mention (rule D05).
type Want struct {
	Ref     string // the object, as <id>v<n>#<local>
	File    string // the objects file, relative to the checkout
	Mention string // the mention that must hold it, or "" when the object has no tag
}

// Expect returns the objects of an item kind in the latest version of each paper of the checkout at root, with the mention that must hold each one. A restated print is held by the mention of its first print.
func Expect(root string) ([]Want, error) {
	files, err := mdrec.Glob(filepath.Join(root, "analysis", "objects", "*", "*"+mdrec.Ext))
	if err != nil {
		return nil, err
	}
	var out []Want
	for _, f := range files {
		objs, err := readObjects(f)
		if err != nil {
			return nil, err
		}
		latest := 0
		for _, o := range objs {
			if v, _ := strconv.Atoi(o.Version); o.Path == "source" && v > latest {
				latest = v
			}
		}
		if len(objs) == 0 {
			continue
		}
		vp := &versionIn{paperIn: &paperIn{id: objs[0].Paper}, byLocal: map[string]*Object{}}
		var in []Object
		for i := range objs {
			if v, _ := strconv.Atoi(objs[i].Version); objs[i].Path == "source" && v == latest {
				vp.byLocal[objs[i].Local] = &objs[i]
				in = append(in, objs[i])
			}
		}
		// A finding names the file that is there, .md or .jsonl.
		rel, _ := filepath.Rel(root, mdrec.Which(f))
		for _, o := range in {
			if !Wanted(o.Item) {
				continue
			}
			w := Want{Ref: fmt.Sprintf("%sv%d#%s", o.Paper, latest, o.Local), File: filepath.ToSlash(rel)}
			if o.Tag != "" {
				w.Mention = vp.target(o.Local)
			}
			out = append(out, w)
		}
	}
	return out, nil
}

// Month returns the month directory of a paper id: the first four digits of its number, as 2401 for 2401.00001 and 9802 for math/9802134.
func Month(id string) string {
	n := id
	if _, after, ok := strings.Cut(id, "/"); ok {
		n = after
	}
	if len(n) < 4 {
		return ""
	}
	return n[:4]
}

// PathForm returns the form of a paper id in a file name, as math-9802134 for math/9802134.
func PathForm(id string) string { return strings.ReplaceAll(id, "/", "-") }

// ObjectsFile returns the objects file of a paper, relative to the checkout.
func ObjectsFile(id string) string {
	return filepath.ToSlash(filepath.Join("analysis", "objects", Month(id), PathForm(id)+mdrec.Ext))
}

// ReadPaper reads the metadata record and the object records of one paper from the checkout at root. A paper with no metadata record gives a nil Paper.
func ReadPaper(root, id string) (*Paper, []Object, error) {
	objs, err := readObjects(filepath.Join(root, filepath.FromSlash(ObjectsFile(id))))
	if err != nil {
		return nil, nil, err
	}
	meta, err := readMetadata(root, map[string]bool{Month(id): true})
	if err != nil {
		return nil, nil, err
	}
	return meta[id], objs, nil
}

// Latest returns the objects of the source path in the latest version of the objects, and that version.
func Latest(objs []Object) ([]Object, int) {
	latest := 0
	for _, o := range objs {
		if v, _ := strconv.Atoi(o.Version); o.Path == "source" && v > latest {
			latest = v
		}
	}
	var out []Object
	for _, o := range objs {
		if v, _ := strconv.Atoi(o.Version); o.Path == "source" && v == latest {
			out = append(out, o)
		}
	}
	return out, latest
}
