package wpcorpus

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// TagsDir holds the tag register of each article (WP5.4).
const TagsDir = "tags"

// TagsPath gives the path of the tags file of a page. The file is Markdown
// with a YAML header (WP12.1). The header holds the tag register and the
// runs of the matcher, and the body is a table for the reader.
func TagsPath(page int) string {
	return filepath.Join(TagsDir, Shard(page), strconv.Itoa(page)+".md")
}

// tagAlphabet is the alphabet of the Stacks Project: the digits and the
// capital letters. A tag has 4 characters (2166-03).
const (
	tagAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	tagLen      = 4
)

// IsWPTag tells if s has the form of a tag of an article.
func IsWPTag(s string) bool {
	if len(s) != tagLen {
		return false
	}
	for i := range len(s) {
		if !strings.ContainsRune(tagAlphabet, rune(s[i])) {
			return false
		}
	}
	return true
}

// The passes of the matcher of WP5.4, and the outcomes of a tag that no
// pass matched.
const (
	PassAnchor = "anchor"
	PassName   = "name"
	PassHash   = "hash"
	PassNear   = "near"
	// PassRef matches by the ref, only when the objects file of the
	// revision is lost and the last run was for the same revision. The
	// extractor gives the same refs for the same revision.
	PassRef  = "ref"
	PassNew  = "new"
	PassGone = "gone"
)

// nearMin is the least similarity of the texts for the pass near.
const nearMin = 0.8

// TagLine is one entry of a tag register: the tag and the ref of its
// object in the revision of record. Gone is the revision in which the object
// went, as r<revision>, or "" while the object is there. A tag is never given
// to another object.
type TagLine struct {
	Tag  string `yaml:"tag"`
	Ref  string `yaml:"ref"`
	Gone string `yaml:"gone,omitempty"`
}

func (l TagLine) String() string {
	if l.Gone != "" {
		return l.Tag + "," + l.Ref + ",gone:" + l.Gone
	}
	return l.Tag + "," + l.Ref
}

// tagsFile is the YAML header of a tags file.
type tagsFile struct {
	Page  int       `yaml:"page"`
	Title string    `yaml:"title,omitempty"`
	Tags  []TagLine `yaml:"tags"`
	Runs  []runLine `yaml:"runs,omitempty"`
}

// runLine is one run of the matcher: the old revision, or "" for the first
// run, the new revision, and the pass of each tag that the run changed.
type runLine struct {
	From   string    `yaml:"from,omitempty"`
	To     string    `yaml:"to"`
	Passes []tagPass `yaml:"passes"`
}

type tagPass struct {
	Tag  string `yaml:"tag"`
	Pass string `yaml:"pass"`
}

// readTagsFile reads the tags file of a page. A page with no file has no
// tags and no runs.
func readTagsFile(root string, page int) (*tagsFile, error) {
	data, err := os.ReadFile(filepath.Join(root, TagsPath(page))) // #nosec G304 -- the path is in the corpus that the user names
	if errors.Is(err, fs.ErrNotExist) {
		return &tagsFile{Page: page}, nil
	}
	if err != nil {
		return nil, err
	}
	head, _, err := splitFrontMatter(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", TagsPath(page), err)
	}
	var f tagsFile
	if err := yaml.Unmarshal(head, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", TagsPath(page), err)
	}
	seen := map[string]bool{}
	for i, l := range f.Tags {
		switch {
		case !IsWPTag(l.Tag) || l.Ref == "":
			return nil, fmt.Errorf("%s: tag %d: an entry has a tag of 4 characters and a ref", TagsPath(page), i+1)
		case l.Gone != "" && !strings.HasPrefix(l.Gone, "r"):
			return nil, fmt.Errorf("%s: tag %s: gone is not r<revision>", TagsPath(page), l.Tag)
		case seen[l.Tag]:
			return nil, fmt.Errorf("%s: the tag %s is in the register twice", TagsPath(page), l.Tag)
		}
		seen[l.Tag] = true
	}
	return &f, nil
}

// ReadTags reads the tag register of a page. A page with no tags file has
// no tags.
func ReadTags(root string, page int) ([]TagLine, error) {
	f, err := readTagsFile(root, page)
	if err != nil {
		return nil, err
	}
	return f.Tags, nil
}

// marshalTags gives the tags file of a page.
func marshalTags(f *tagsFile) ([]byte, error) {
	var b bytes.Buffer
	if err := writeFrontMatter(&b, f); err != nil {
		return nil, err
	}
	name := strconv.Itoa(f.Page)
	if f.Title != "" {
		name = f.Title
	}
	fmt.Fprintf(&b, "\n# Tags of %s\n\n", name)
	b.WriteString("Each object of the page has a tag of WP5.4. A tag is never given to another object. A tag whose object went has the revision in which it went.\n")
	if len(f.Tags) > 0 {
		b.WriteString("\n| Tag | Ref | Gone |\n|---|---|---|\n")
		for _, l := range f.Tags {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", l.Tag, l.Ref, l.Gone)
		}
	}
	return b.Bytes(), nil
}

// TagRun is the result of AssignTags for one page.
type TagRun struct {
	Page, From, To int
	// Pass gives the pass of each tag of the run: a pass of the matcher,
	// new or gone.
	Pass map[string]string
	// Order is the tags in the order of the objects, then the tags that
	// went.
	Order []string
}

// Count gives the number of tags of a pass.
func (r *TagRun) Count(pass string) int {
	n := 0
	for _, p := range r.Pass {
		if p == pass {
			n++
		}
	}
	return n
}

// MatchTags maps the objects of a new revision to the tagged objects of the
// old revision (WP5.4). It gives the tag and the pass of each new object, or
// "" when no pass matched it. Each pass matches only the objects that the
// earlier passes left, and only a match that is the one choice on both
// sides.
func MatchTags(old, cur []Object) (tags, passes []string) {
	tags = make([]string, len(cur))
	passes = make([]string, len(cur))
	used := map[int]bool{}
	var tagged []int
	for i := range old {
		if old[i].Tag != "" {
			tagged = append(tagged, i)
		}
	}
	unique := func(pass string, key func(*Object) string) {
		olds, curs := map[string][]int{}, map[string][]int{}
		for _, i := range tagged {
			if k := key(&old[i]); k != "" && !used[i] {
				olds[k] = append(olds[k], i)
			}
		}
		for j := range cur {
			if k := key(&cur[j]); k != "" && tags[j] == "" {
				curs[k] = append(curs[k], j)
			}
		}
		for k, js := range curs {
			if is := olds[k]; len(is) == 1 && len(js) == 1 {
				used[is[0]] = true
				tags[js[0]], passes[js[0]] = old[is[0]].Tag, pass
			}
		}
	}
	unique(PassAnchor, func(o *Object) string { return o.Anchor })
	unique(PassName, func(o *Object) string {
		if o.Name == "" {
			return ""
		}
		return kindFamily(o.Kind) + "\x00" + o.Cue + "\x00" + normAnchor(strings.ToLower(o.Name)) + "\x00" + parentPath(o.Section)
	})
	unique(PassHash, func(o *Object) string {
		if o.Hash == "" {
			return ""
		}
		return kindFamily(o.Kind) + "\x00" + o.Hash
	})

	// The pass near: the same kind, a similar section, and texts with a
	// similarity of nearMin or more. The two objects must be the best choice
	// of each other.
	sh := map[*Object]map[string]bool{}
	shingle := func(o *Object) map[string]bool {
		if s, ok := sh[o]; ok {
			return s
		}
		s := shingles(o.Text + " " + o.Name)
		sh[o] = s
		return s
	}
	// best gives the most similar object of the pool, or nil when two
	// objects of the pool have the same best similarity.
	best := func(o *Object, pool []*Object) (*Object, float64) {
		var b *Object
		var bs float64
		tie := false
		for _, p := range pool {
			if kindFamily(p.Kind) != kindFamily(o.Kind) || !nearSection(p.Section, o.Section) {
				continue
			}
			s := jaccard(shingle(o), shingle(p))
			switch {
			case s > bs:
				b, bs, tie = p, s, false
			case s == bs && s > 0:
				tie = true
			}
		}
		if tie {
			return nil, 0
		}
		return b, bs
	}
	var olds, curs []*Object
	oldIndex := map[*Object]int{}
	curIndex := map[*Object]int{}
	for _, i := range tagged {
		if !used[i] {
			olds = append(olds, &old[i])
			oldIndex[&old[i]] = i
		}
	}
	for j := range cur {
		if tags[j] == "" {
			curs = append(curs, &cur[j])
			curIndex[&cur[j]] = j
		}
	}
	for _, c := range curs {
		o, s := best(c, olds)
		if o == nil || s < nearMin || used[oldIndex[o]] {
			continue
		}
		if back, _ := best(o, curs); back != c {
			continue
		}
		used[oldIndex[o]] = true
		j := curIndex[c]
		tags[j], passes[j] = o.Tag, PassNear
	}
	return tags, passes
}

// parentPath gives the section path without its last heading.
func parentPath(section string) string {
	if i := strings.LastIndex(section, " > "); i >= 0 {
		return section[:i]
	}
	return ""
}

// nearSection tells if two section paths are the same section or sections
// with a similar last heading. The last headings are similar when half of
// the words of the shorter heading or more are in the other heading.
func nearSection(a, b string) bool {
	if a == b {
		return true
	}
	last := func(s string) string {
		if i := strings.LastIndex(s, " > "); i >= 0 {
			s = s[i+3:]
		}
		return s
	}
	wa, wb := words(last(a)), words(last(b))
	if len(wa) == 0 || len(wb) == 0 {
		return false
	}
	n := 0
	for w := range wa {
		if wb[w] {
			n++
		}
	}
	return float64(n) >= 0.5*float64(min(len(wa), len(wb)))
}

func words(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(s)) {
		out[w] = true
	}
	return out
}

// shingles gives the sets of 3 words that follow each other in a text. A
// text of fewer than 3 words gives its words.
func shingles(s string) map[string]bool {
	w := strings.Fields(strings.ToLower(s))
	out := map[string]bool{}
	if len(w) < 3 {
		for _, x := range w {
			out[x] = true
		}
		return out
	}
	for i := 0; i+3 <= len(w); i++ {
		out[w[i]+" "+w[i+1]+" "+w[i+2]] = true
	}
	return out
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	n := 0
	for k := range a {
		if b[k] {
			n++
		}
	}
	return float64(n) / float64(len(a)+len(b)-n)
}

// previousObjects gives the objects of the revision of a page that the
// analysis plane holds, other than rev when it has another, and the
// revision. It gives no objects when the plane has no objects file.
func previousObjects(root string, page, rev int) ([]Object, int, error) {
	ents, err := os.ReadDir(filepath.Join(root, ObjectsDir, Shard(page)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	prefix := strconv.Itoa(page) + "-r"
	from := 0
	for _, e := range ents {
		r, ok := strings.CutPrefix(e.Name(), prefix)
		if !ok {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSuffix(r, ".md"))
		if err != nil {
			continue
		}
		// The newest other revision comes first. The same revision is used
		// only when there is no other.
		if from == 0 || from == rev || n != rev && n > from {
			from = n
		}
	}
	if from == 0 {
		return nil, 0, nil
	}
	objs, err := ReadObjects(root, page, from)
	return objs, from, err
}

// AssignTags gives a tag to each object of a result (WP5.4). It matches the
// objects to the tagged objects of the revision that the analysis plane
// holds, gives a new tag to each object with no match, and gives a
// tombstone to each tag whose object went. It writes the tags file, and adds
// the run to it when a tag is new or went. Call it before
// WriteResult and RemoveStale, while the old objects file is there.
func AssignTags(root string, r *Result) (*TagRun, error) {
	c := &r.Census
	tf, err := readTagsFile(root, c.Page)
	if err != nil {
		return nil, err
	}
	reg := tf.Tags
	old, from, err := previousObjects(root, c.Page, c.Revision)
	if err != nil {
		return nil, err
	}
	run := &TagRun{Page: c.Page, From: from, To: c.Revision, Pass: map[string]string{}}
	tags, passes := MatchTags(old, r.Objects)
	if old == nil {
		if lastRun(tf) == c.Revision {
			byRef := map[string]string{}
			for _, l := range reg {
				if l.Gone == "" {
					byRef[l.Ref] = l.Tag
				}
			}
			for i := range r.Objects {
				if t := byRef[r.Objects[i].Ref]; t != "" {
					tags[i], passes[i] = t, PassRef
				}
			}
		}
	}
	used := map[string]bool{}
	for _, l := range reg {
		used[l.Tag] = true
	}
	rng := rand.New(rand.NewPCG(uint64(c.Page), uint64(len(reg)))) // #nosec G404 G115 -- the tags are not a secret, and the page id and the count are not negative
	mint := func() (string, error) {
		for range 1000 {
			b := make([]byte, tagLen)
			for i := range b {
				b[i] = tagAlphabet[rng.IntN(len(tagAlphabet))]
			}
			if t := string(b); !used[t] {
				used[t] = true
				return t, nil
			}
		}
		return "", fmt.Errorf("page %d: no free tag after 1000 draws", c.Page)
	}
	ref := map[string]string{}
	for i := range r.Objects {
		o := &r.Objects[i]
		if tags[i] == "" {
			if tags[i], err = mint(); err != nil {
				return nil, err
			}
			passes[i] = PassNew
		}
		o.Tag = tags[i]
		ref[o.Tag] = o.Ref
		run.Pass[o.Tag] = passes[i]
		run.Order = append(run.Order, o.Tag)
	}
	gone := "r" + strconv.Itoa(c.Revision)
	changed := false
	for i := range reg {
		l := &reg[i]
		switch {
		case ref[l.Tag] != "":
			l.Ref, l.Gone = ref[l.Tag], ""
		case l.Gone == "":
			l.Gone = gone
			run.Pass[l.Tag] = PassGone
			run.Order = append(run.Order, l.Tag)
			changed = true
		}
	}
	for _, t := range run.Order {
		if run.Pass[t] == PassNew {
			reg = append(reg, TagLine{Tag: t, Ref: ref[t]})
			changed = true
		}
	}
	tf.Page, tf.Title, tf.Tags = c.Page, c.Title, reg
	if changed || from != c.Revision {
		tf.Runs = append(tf.Runs, runOf(run))
	}
	data, err := marshalTags(tf)
	if err != nil {
		return nil, err
	}
	if err := writeFile(filepath.Join(root, TagsPath(c.Page)), data); err != nil {
		return nil, err
	}
	return run, nil
}

// lastRun gives the new revision of the last run of a tags file, or 0.
func lastRun(f *tagsFile) int {
	if len(f.Runs) == 0 {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimPrefix(f.Runs[len(f.Runs)-1].To, "r"))
	return n
}

// runOf gives the run line of a run: the revisions, and the pass of each
// tag in the order of the run.
func runOf(run *TagRun) runLine {
	l := runLine{To: "r" + strconv.Itoa(run.To)}
	if run.From != 0 {
		l.From = "r" + strconv.Itoa(run.From)
	}
	for _, t := range run.Order {
		l.Passes = append(l.Passes, tagPass{Tag: t, Pass: run.Pass[t]})
	}
	return l
}
