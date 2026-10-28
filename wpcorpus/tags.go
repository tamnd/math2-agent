package wpcorpus

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// TagsDir holds the tag register of each article (WP5.4).
const TagsDir = "tags"

// TagsPath gives the path of the tag register of a page.
func TagsPath(page int) string {
	return filepath.Join(TagsDir, Shard(page), strconv.Itoa(page)+".tags")
}

// RunsPath gives the path of the runs file of a page.
func RunsPath(page int) string {
	return filepath.Join(TagsDir, Shard(page), strconv.Itoa(page)+".runs")
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

// TagLine is one line of a tag register: the tag and the ref of its object
// in the revision of record. Gone is the revision in which the object went,
// as r<revision>, or "" while the object is there. A tag is never given to
// another object.
type TagLine struct {
	Tag, Ref, Gone string
}

func (l TagLine) String() string {
	if l.Gone != "" {
		return l.Tag + "," + l.Ref + ",gone:" + l.Gone
	}
	return l.Tag + "," + l.Ref
}

// ReadTags reads the tag register of a page. A page with no register has
// no tags.
func ReadTags(root string, page int) ([]TagLine, error) {
	data, err := os.ReadFile(filepath.Join(root, TagsPath(page))) // #nosec G304 -- the path is in the corpus that the user names
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []TagLine
	seen := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, ",")
		if len(f) < 2 || len(f) > 3 || !IsWPTag(f[0]) || f[1] == "" {
			return nil, fmt.Errorf("%s:%d: a line is a tag and a ref, and then gone:r<revision> for an object that went", TagsPath(page), n)
		}
		l := TagLine{Tag: f[0], Ref: f[1]}
		if len(f) == 3 {
			g, ok := strings.CutPrefix(f[2], "gone:")
			if !ok || g == "" {
				return nil, fmt.Errorf("%s:%d: the third field is not gone:r<revision>", TagsPath(page), n)
			}
			l.Gone = g
		}
		if seen[l.Tag] {
			return nil, fmt.Errorf("%s:%d: the tag %s is in the register twice", TagsPath(page), n, l.Tag)
		}
		seen[l.Tag] = true
		out = append(out, l)
	}
	return out, sc.Err()
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
// tombstone to each tag whose object went. It writes the register, and adds
// the run to the runs file when a tag is new or went. Call it before
// WriteResult and RemoveStale, while the old objects file is there.
func AssignTags(root string, r *Result) (*TagRun, error) {
	c := &r.Census
	reg, err := ReadTags(root, c.Page)
	if err != nil {
		return nil, err
	}
	old, from, err := previousObjects(root, c.Page, c.Revision)
	if err != nil {
		return nil, err
	}
	run := &TagRun{Page: c.Page, From: from, To: c.Revision, Pass: map[string]string{}}
	tags, passes := MatchTags(old, r.Objects)
	if old == nil {
		last, err := lastRun(root, c.Page)
		if err != nil {
			return nil, err
		}
		if last == c.Revision {
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
	var b bytes.Buffer
	fmt.Fprintf(&b, "# The tag register of the page %d (WP5.4). Each line is a tag and the ref of its object in the revision of record. A tag whose object went has gone:r<revision>. A tag is never given to another object.\n", c.Page)
	for _, l := range reg {
		b.WriteString(l.String() + "\n")
	}
	if err := writeFile(filepath.Join(root, TagsPath(c.Page)), b.Bytes()); err != nil {
		return nil, err
	}
	if changed || from != c.Revision {
		if err := appendRun(root, run); err != nil {
			return nil, err
		}
	}
	return run, nil
}

// lastRun gives the new revision of the last run of a page, or 0.
func lastRun(root string, page int) (int, error) {
	data, err := os.ReadFile(filepath.Join(root, RunsPath(page))) // #nosec G304 -- the path is in the corpus that the user names
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	f := strings.Split(lines[len(lines)-1], ",")
	if len(f) != 4 {
		return 0, nil
	}
	n, _ := strconv.Atoi(strings.TrimPrefix(f[1], "r"))
	return n, nil
}

// appendRun adds a run to the runs file: one line for each tag, with the
// revisions and the pass.
func appendRun(root string, run *TagRun) error {
	path := filepath.Join(root, RunsPath(run.Page))
	var b bytes.Buffer
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(&b, "# The runs of the matcher of the page %d (WP5.4). Each line is the old revision, the new revision, a tag and the pass that matched it, or new or gone.\n", run.Page)
	}
	from := "-"
	if run.From != 0 {
		from = "r" + strconv.Itoa(run.From)
	}
	for _, t := range run.Order {
		fmt.Fprintf(&b, "%s,r%d,%s,%s\n", from, run.To, t, run.Pass[t])
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- the corpus is a public repository
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) // #nosec G302 G304 -- the corpus is a public repository, and the path is in it
	if err != nil {
		return err
	}
	if _, err := f.Write(b.Bytes()); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
