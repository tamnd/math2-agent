package site

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The learning paths: the courses of the seed list (topics/curricula/seed.yaml) and the skeletons of the subjects (topics/skeletons/NN.yaml).
// Both lists are in teaching order. A comment line "# Heading" between the items of a list starts a section, for example "# Group actions and permutation groups".
// An item with the field node links to the page of its node. An item without a node is on the page too, so that a reader sees what is still to come.

// pathItem is one item of a course or a skeleton.
type pathItem struct {
	N          int       // the number of the item in its list, from 1
	Name, Kind string    // from the list
	Node       *nodeView // nil when the node does not exist yet
}

// pathSection is the items under one heading.
type pathSection struct {
	Title, ID string
	Items     []*pathItem
	Done      int // the items with a node
}

// learnPath is one course or one skeleton.
type learnPath struct {
	ID, Title, URL string
	Year           int
	Sections       []*pathSection
	Items          []*pathItem
	Done           int
}

// Percent returns the share of the items with a node, from 0 to 100.
func (p *learnPath) Percent() int {
	if len(p.Items) == 0 {
		return 0
	}
	return p.Done * 100 / len(p.Items)
}

// Start returns the first items with a node, for the band "Start here".
func (p *learnPath) Start(n int) []*nodeView {
	var out []*nodeView
	for _, it := range p.Items {
		if it.Node != nil && it.Node.Excerpt != "" && len(out) < n {
			out = append(out, it.Node)
		}
	}
	return out
}

// pathStep is the place of a node in a course, for the strip on the page of the node.
type pathStep struct {
	Path       *learnPath
	N, Of      int
	Prev, Next *nodeView // the nearest items before and after with a node, or nil
}

// areaRef is the section of the skeleton of its subject that has a node.
type areaRef struct{ Title, URL string }

var (
	pathItemLine = regexp.MustCompile(`^\s+- \{`)
	pathHeading  = regexp.MustCompile(`^\s+# (.+?)\s*$`)
	pathCourse   = regexp.MustCompile(`^\s+- course:`)
)

type rawItem struct {
	Name string `yaml:"name"`
	Kind string `yaml:"kind"`
	Node string `yaml:"node"`
}

// headings returns the heading of each item line of a list, in order, and the start of each course.
// A heading ends at the start of the next course.
func headings(data []byte) (heads []string) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	cur := ""
	for sc.Scan() {
		line := sc.Text()
		switch {
		case pathCourse.MatchString(line):
			cur = ""
		case pathItemLine.MatchString(line):
			heads = append(heads, cur)
		default:
			if m := pathHeading.FindStringSubmatch(line); m != nil {
				cur = m[1]
			}
		}
	}
	return heads
}

// newPath makes a path from the items of a list and the heading of each item.
func (b *builder) newPath(id, title, url string, items []rawItem, heads []string) *learnPath {
	p := &learnPath{ID: id, Title: title, URL: url}
	var sec *pathSection
	for i, r := range items {
		h := ""
		if i < len(heads) {
			h = heads[i]
		}
		if sec == nil || sec.Title != h {
			sec = &pathSection{Title: h, ID: "s-" + Slug(h)}
			if h == "" {
				sec.ID = fmt.Sprintf("s-%d", len(p.Sections)+1)
			}
			p.Sections = append(p.Sections, sec)
		}
		it := &pathItem{N: i + 1, Name: r.Name, Kind: r.Kind}
		if v := b.nodes[r.Node]; v != nil && v.Retired == nil && v.Slug != "" {
			it.Node = v
			sec.Done++
			p.Done++
		}
		sec.Items = append(sec.Items, it)
		p.Items = append(p.Items, it)
	}
	return p
}

// loadPaths reads the courses and the skeletons of the content tree. A tree without them has no paths.
func (b *builder) loadPaths() error {
	b.steps = map[*nodeView][]pathStep{}
	b.areas = map[*nodeView]areaRef{}
	b.skeletons = map[string]*learnPath{}
	if data, err := os.ReadFile(filepath.Join(b.opt.Content, "topics", "curricula", "seed.yaml")); err == nil {
		var seed struct {
			Courses []struct {
				Course, Title string
				Year          int
				Items         []rawItem
			}
		}
		if err := yaml.Unmarshal(data, &seed); err != nil {
			return fmt.Errorf("topics/curricula/seed.yaml: %w", err)
		}
		heads := headings(data)
		n := 0
		for _, c := range seed.Courses {
			var h []string
			if n+len(c.Items) <= len(heads) {
				h = heads[n : n+len(c.Items)]
			}
			n += len(c.Items)
			p := b.newPath(c.Course, c.Title, b.url("learn/"+c.Course+"/"), c.Items, h)
			p.Year = c.Year
			b.courses = append(b.courses, p)
			b.addSteps(p)
		}
	}
	files, _ := filepath.Glob(filepath.Join(b.opt.Content, "topics", "skeletons", "*.yaml"))
	for _, f := range files {
		data, err := os.ReadFile(f) // #nosec G304 -- the path is a skeleton of the content tree.
		if err != nil {
			return err
		}
		var sk struct {
			Title string
			Items []rawItem
		}
		if err := yaml.Unmarshal(data, &sk); err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		code := strings.TrimSuffix(filepath.Base(f), ".yaml")
		heads := headings(data)
		if len(heads) != len(sk.Items) {
			heads = nil
		}
		url := b.url("msc/" + code + "/")
		p := b.newPath(code, mscTop[code], url, sk.Items, heads)
		b.skeletons[code] = p
		for _, s := range p.Sections {
			for _, it := range s.Items {
				if it.Node == nil || s.Title == "" {
					continue
				}
				if _, ok := b.areas[it.Node]; !ok && it.Node.SubjectCode() == code {
					b.areas[it.Node] = areaRef{Title: s.Title, URL: url + "#" + s.ID}
				}
			}
		}
	}
	return nil
}

// addSteps gives each node of a course its place in the course.
func (b *builder) addSteps(p *learnPath) {
	var written []*pathItem
	for _, it := range p.Items {
		if it.Node != nil {
			written = append(written, it)
		}
	}
	for i, it := range written {
		s := pathStep{Path: p, N: i + 1, Of: len(written)}
		if i > 0 {
			s.Prev = written[i-1].Node
		}
		if i+1 < len(written) {
			s.Next = written[i+1].Node
		}
		if !slices.ContainsFunc(b.steps[it.Node], func(x pathStep) bool { return x.Path == p }) {
			b.steps[it.Node] = append(b.steps[it.Node], s)
		}
	}
}

// learnData is the page learn/: the courses by year and the subjects.
type learnData struct {
	Years []learnYear
	Done  int
	Items int
}

type learnYear struct {
	Year    int
	Courses []*learnPath
}

func (b *builder) writeLearn() error {
	d := learnData{}
	courses := slices.Clone(b.courses)
	slices.SortStableFunc(courses, func(x, y *learnPath) int { return x.Year - y.Year })
	for _, c := range courses {
		if len(d.Years) == 0 || d.Years[len(d.Years)-1].Year != c.Year {
			d.Years = append(d.Years, learnYear{Year: c.Year})
		}
		y := &d.Years[len(d.Years)-1]
		y.Courses = append(y.Courses, c)
		d.Done += c.Done
		d.Items += len(c.Items)
	}
	if err := b.writePage("learn", "learn", d, page{Title: "Learn", Description: "Courses of Math 2.0: the ideas of each course in teaching order, from logic and sets to probability.", Section: "learn"}); err != nil {
		return err
	}
	for _, c := range b.courses {
		if err := b.writePage("learn/"+c.ID, "course", c, page{Title: c.Title, Description: "The course " + c.Title + " of Math 2.0: its ideas in teaching order.", Section: "learn", Math: true}); err != nil {
			return err
		}
	}
	return nil
}

// Steps returns the place of the node in each course that has it.
func (b *builder) Steps(v *nodeView) []pathStep { return b.steps[v] }

// Area returns the section of the skeleton of the subject of the node, or nil.
func (b *builder) Area(v *nodeView) *areaRef {
	if a, ok := b.areas[v]; ok {
		return &a
	}
	return nil
}
