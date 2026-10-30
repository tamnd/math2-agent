package ocw

import (
	"bytes"
	"encoding/json"
	"maps"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Course is one course package: the metadata of the course and of its offering, and the materials of the offering.
type Course struct {
	Slug        string   // the name of the site, as 18-06-linear-algebra-spring-2010
	Title       string   // as Linear Algebra
	Number      string   // the primary course number, as 18.06
	Extra       []string // the other course numbers of a course with two departments
	Departments []string // the numbers or the names of the departments, as 18 or Mathematics
	Term        string   // as 2010-spring
	Label       string   // as Spring 2010
	Year        string
	Level       string // as Undergraduate
	Instructors []string
	Topics      []string // the topic paths, as Mathematics > Linear Algebra
	Textbook    string   // the ISBN of the one textbook that the course names
	Materials   []Material

	files map[string][]byte
}

// Material is one item of the offering with its own page under resources/.
type Material struct {
	Slug  string // the name of the page, as lecture-1-the-geometry-of-linear-equations
	Title string
	Type  string // the learning resource type of OCW, as Lecture Notes
}

// Types are the learning resource types of OCW whose items become resources of the kind course-material (RS5.1). A reading list and the insights of an instructor are not materials.
var Types = map[string]bool{
	"lecture notes": true, "lecture videos": true, "lecture audio": true, "lecture slides": true,
	"recitation notes": true, "recitation videos": true,
	"problem sets": true, "problem set solutions": true, "problem sets with solutions": true,
	"exams": true, "exam solutions": true, "exams with solutions": true, "practice exams": true,
	"written assignments": true, "online textbook": true,
	"problem-solving notes": true, "problem-solving videos": true, "worked examples": true,
}

// Own reports whether the material becomes a resource.
func (m Material) Own() bool { return Types[strings.ToLower(m.Type)] }

// ID gives the ocw id of the material.
func (c *Course) materialID(m Material) string { return c.Slug + "/resources/" + m.Slug }

// Code gives the code of the course in its id: the primary number in upper case with no space, as 18.06 or 18.06SC.
func (c *Course) Code() string {
	return strings.ToUpper(strings.Join(strings.Fields(c.Number), ""))
}

// CourseID gives the id of the course, as 042nb2s44:18.06, or an empty string for a course with no number.
func (c *Course) CourseID() string {
	if c.Code() == "" {
		return ""
	}
	return ROR + ":" + c.Code()
}

// OfferingID gives the id of the offering, as 042nb2s44:18.06:2010-spring, or an empty string for a course with no number or no term.
func (c *Course) OfferingID() string {
	if c.Code() == "" || c.Term == "" {
		return ""
	}
	return c.CourseID() + ":" + c.Term
}

// URL gives the home of the site of the offering.
func (c *Course) URL() string { return Base + "/courses/" + c.Slug + "/" }

// Math reports whether a course is a course of mathematics: a course of the department 18, a course with a topic of mathematics, or a course with a number of the department 18.
func Math(c *Course) bool {
	for _, d := range c.Departments {
		if d == "18" || strings.EqualFold(d, "Mathematics") {
			return true
		}
	}
	for _, t := range c.Topics {
		for _, p := range strings.Split(t, " > ") {
			if strings.EqualFold(p, "Mathematics") {
				return true
			}
		}
	}
	for _, n := range append([]string{c.Number}, c.Extra...) {
		if strings.HasPrefix(n, "18.") {
			return true
		}
	}
	return false
}

// SourceRecords gives the source records of the course: one record for the site of the offering, and one record for each material with its own page. The record of the site has the course id, the offering id and the ocw id, so the cluster puts each at its level (RS5.2).
func (c *Course) SourceRecords() []cluster.Record {
	put := func(rec *cluster.Record, key string, vs ...string) {
		for _, v := range vs {
			if n, err := ids.Normalize(key, v); err == nil && !slices.Contains(rec.IDs[key], n) {
				rec.IDs[key] = append(rec.IDs[key], n)
			}
		}
	}
	site := cluster.Record{NativeID: c.Slug, Level: "manifestation", Kind: "course", Title: c.Title, Date: c.Year, Language: "en", Edition: c.Label, Publisher: "MIT OpenCourseWare", IDs: resource.IDs{}, Links: []string{c.URL()}}
	if id := c.CourseID(); id != "" {
		put(&site, "course", id)
		for _, n := range c.Extra {
			if code := strings.ToUpper(strings.Join(strings.Fields(n), "")); code != "" {
				put(&site, "course", ROR+":"+code)
			}
		}
	}
	put(&site, "offering", c.OfferingID())
	put(&site, "ocw", c.Slug)
	for _, n := range c.Instructors {
		site.Creators = append(site.Creators, resource.Creator{Name: n, Role: "instructor"})
	}
	out := []cluster.Record{site}
	for _, m := range c.Materials {
		if !m.Own() {
			continue
		}
		rec := cluster.Record{NativeID: c.materialID(m), Level: "manifestation", Kind: "course-material", Title: m.Title, Date: c.Year, Language: "en", Publisher: "MIT OpenCourseWare", IDs: resource.IDs{},
			Links: []string{c.URL() + "resources/" + m.Slug + "/"}}
		put(&rec, "ocw", c.materialID(m))
		if o := c.OfferingID(); o != "" {
			rec.Relations = []resource.Relation{{Kind: "part_of", To: "offering:" + o, Source: "ocw"}}
		}
		out = append(out, rec)
	}
	return out
}

var (
	slugRE    = regexp.MustCompile(`/courses/([A-Za-z0-9][A-Za-z0-9._-]*)/`)
	resRE     = regexp.MustCompile(`(?:^|/)resources/([A-Za-z0-9][A-Za-z0-9._-]*)(?:/|$)`)
	termRE    = regexp.MustCompile(`(?i)\b(spring|summer|fall|autumn|winter|january iap|iap)\s+((?:19|20)[0-9]{2})\b`)
	numberRE  = regexp.MustCompile(`^[0-9A-Z]{1,4}\.[0-9A-Z.]+$`)
	deptRE    = regexp.MustCompile(`department=([0-9A-Za-z]+)`)
	isbnRE    = regexp.MustCompile(`ISBN(?:-1[03])?:?\s*([0-9][0-9 -]{8,15}[0-9Xx])`)
	nonSlugRE = regexp.MustCompile(`[^a-z0-9]+`)
)

// Parse reads the files of one package. It reads the data file data.json of the site when the package has it, and the course information on the pages: the number, the term, the title, the instructors, the departments and the topics.
func Parse(files map[string][]byte) *Course {
	c := &Course{files: files}
	if data, ok := files["data.json"]; ok {
		c.readData(data)
	}
	for _, name := range []string{"index.html", "download/index.html", "pages/syllabus/index.html"} {
		if data, ok := files[name]; ok {
			c.readInfo(data)
		}
	}
	if c.Label == "" && c.Term != "" {
		c.Label = label(c.Term)
	}
	// A package with no link to its own site gets the slug in the form of OCW, as 18-06-linear-algebra-spring-2010.
	if c.Slug == "" && c.Number != "" && c.Title != "" && c.Term != "" {
		year, season, _ := strings.Cut(c.Term, "-")
		c.Slug = strings.Trim(nonSlugRE.ReplaceAllString(strings.ToLower(c.Number+" "+c.Title+" "+season+" "+year), "-"), "-")
	}
	c.Materials = c.materials()
	c.Textbook = c.textbook()
	return c
}

// term gives the term of a label, as 2010-spring for Spring 2010.
func term(s string) (string, string) {
	m := termRE.FindStringSubmatch(s)
	if m == nil {
		return "", ""
	}
	season := strings.ToLower(m[1])
	switch season {
	case "autumn":
		season = "fall"
	case "january iap":
		season = "iap"
	}
	return m[2] + "-" + season, m[2]
}

// label gives the label of a term, as Spring 2010 for 2010-spring.
func label(t string) string {
	year, season, _ := strings.Cut(t, "-")
	if season == "iap" {
		return "January IAP " + year
	}
	return strings.ToUpper(season[:1]) + season[1:] + " " + year
}

func (c *Course) addTerm(s string) {
	if c.Term != "" {
		return
	}
	if t, y := term(s); t != "" {
		c.Term, c.Year = t, y
		c.Label = label(t)
	}
}

func (c *Course) addInstructor(n string) {
	n = strings.Join(strings.Fields(n), " ")
	for _, p := range []string{"Prof. ", "Professor ", "Dr. ", "Mr. ", "Ms. ", "Mrs. "} {
		n = strings.TrimPrefix(n, p)
	}
	if n != "" && !slices.Contains(c.Instructors, n) {
		c.Instructors = append(c.Instructors, n)
	}
}

func addOnce(list *[]string, v string) {
	if v = strings.TrimSpace(v); v != "" && !slices.Contains(*list, v) {
		*list = append(*list, v)
	}
}

// readData reads the data file of a site of ocw-studio. The fields that the file does not have stay empty.
func (c *Course) readData(data []byte) {
	var d map[string]any
	if json.Unmarshal(data, &d) != nil {
		return
	}
	str := func(k string) string {
		switch v := d[k].(type) {
		case string:
			return strings.TrimSpace(v)
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		}
		return ""
	}
	strs := func(v any) []string {
		var out []string
		switch v := v.(type) {
		case string:
			for _, p := range strings.Split(v, ",") {
				addOnce(&out, p)
			}
		case []any:
			for _, x := range v {
				if s, ok := x.(string); ok {
					addOnce(&out, s)
				}
			}
		}
		return out
	}
	c.Title = str("course_title")
	c.Number = str("primary_course_number")
	c.Extra = strs(d["extra_course_numbers"])
	c.Departments = strs(d["department_numbers"])
	if t := str("term"); t != "" {
		c.addTerm(t + " " + str("year"))
	}
	if ls := strs(d["level"]); len(ls) > 0 {
		c.Level = strings.Join(ls, ", ")
	}
	if p := strings.Trim(str("site_url_path"), "/"); p != "" {
		c.Slug = path.Base(p)
	}
	if is, ok := d["instructors"].([]any); ok {
		for _, x := range is {
			m, ok := x.(map[string]any)
			if !ok {
				continue
			}
			name := func(k string) string { s, _ := m[k].(string); return s }
			if t := name("title"); t != "" {
				c.addInstructor(t)
			} else {
				c.addInstructor(name("first_name") + " " + name("middle_initial") + " " + name("last_name"))
			}
		}
	}
	if ts, ok := d["topics"].([]any); ok {
		for _, t := range ts {
			addOnce(&c.Topics, strings.Join(strs(t), " > "))
		}
	}
}

// readInfo reads the course information on one page. A field that is already known stays.
func (c *Course) readInfo(data []byte) {
	doc := parse(data)
	if doc == nil {
		return
	}
	walk(doc, func(n *html.Node) bool {
		switch {
		case n.Type != html.ElementNode:
		case n.Data == "link" && attr(n, "rel") == "canonical", n.Data == "meta" && attr(n, "property") == "og:url":
			u := attr(n, "href")
			if n.Data == "meta" {
				u = attr(n, "content")
			}
			if m := slugRE.FindStringSubmatch(u); m != nil && c.Slug == "" {
				c.Slug = m[1]
			}
		case hasClass(n, "course-number-term-detail"):
			parts := strings.Split(text(n), "|")
			if c.Number == "" && len(parts) > 0 {
				nums := strings.FieldsFunc(parts[0], func(r rune) bool { return r == '/' || r == ',' || r == ' ' })
				for i, x := range nums {
					if !numberRE.MatchString(strings.ToUpper(x)) {
						continue
					}
					if c.Number == "" && i == 0 {
						c.Number = x
					} else {
						addOnce(&c.Extra, x)
					}
				}
			}
			if len(parts) > 1 {
				c.addTerm(parts[1])
			}
			if len(parts) > 2 && c.Level == "" {
				c.Level = strings.TrimSpace(parts[2])
			}
		case n.Data == "a" && hasClass(n, "course-info-instructor"):
			c.addInstructor(text(n))
		case n.Data == "a" && hasClass(n, "course-info-department"):
			if m := deptRE.FindStringSubmatch(attr(n, "href")); m != nil {
				addOnce(&c.Departments, m[1])
			}
			addOnce(&c.Departments, text(n))
		case n.Data == "a" && hasClass(n, "course-info-topic"):
			addOnce(&c.Topics, topicPath(n))
		case n.Data == "h1" && ancestor(n, func(p *html.Node) bool { return attr(p, "id") == "course-banner" }):
			walk(n, func(a *html.Node) bool {
				if a.Type == html.ElementNode && a.Data == "a" {
					if c.Title == "" {
						c.Title = text(a)
					}
					if m := slugRE.FindStringSubmatch(attr(a, "href")); m != nil && c.Slug == "" {
						c.Slug = m[1]
					}
					return false
				}
				return true
			})
			return false
		}
		return true
	})
	if c.Term == "" {
		c.addTerm(text(doc))
	}
}

// topicPath gives the path of a topic link from the nested lists, as Mathematics > Linear Algebra.
func topicPath(a *html.Node) string {
	var names []string
	for p := a.Parent; p != nil && !hasClass(p, "course-info-topics"); p = p.Parent {
		if p.Type != html.ElementNode || p.Data != "li" {
			continue
		}
		var first *html.Node
		walk(p, func(n *html.Node) bool {
			if first != nil || (n != p && n.Type == html.ElementNode && (n.Data == "ul" || n.Data == "ol")) {
				return false
			}
			if n.Type == html.ElementNode && n.Data == "a" && hasClass(n, "course-info-topic") {
				first = n
				return false
			}
			return true
		})
		if first != nil {
			names = append(names, text(first))
		}
	}
	if len(names) == 0 {
		names = []string{text(a)}
	}
	slices.Reverse(names)
	return strings.Join(names, " > ")
}

// materials finds the materials of the offering. It reads the groups of the download page, the data file of each resource, and the links of the pages, in this order. The first source that gives a title or a type wins.
func (c *Course) materials() []Material {
	var out []Material
	at := map[string]int{}
	add := func(slug, title, typ string) {
		if slug == "" {
			return
		}
		title = strings.TrimSpace(title)
		if i, ok := at[slug]; ok {
			if out[i].Title == "" {
				out[i].Title = title
			}
			if out[i].Type == "" {
				out[i].Type = typ
			}
			return
		}
		at[slug] = len(out)
		out = append(out, Material{Slug: slug, Title: title, Type: typ})
	}
	if doc := parse(c.files["download/index.html"]); doc != nil {
		group := ""
		walk(doc, func(n *html.Node) bool {
			if n.Type != html.ElementNode {
				return true
			}
			if n.Data == "h4" || n.Data == "h3" {
				group = text(n)
				return false
			}
			if n.Data == "a" && hasClass(n, "resource-list-title") {
				if m := resRE.FindStringSubmatch(attr(n, "href")); m != nil {
					add(m[1], text(n), group)
				}
				return false
			}
			return true
		})
	}
	for _, name := range slices.Sorted(maps.Keys(c.files)) {
		parts := strings.Split(name, "/")
		if len(parts) != 3 || parts[0] != "resources" || parts[2] != "data.json" {
			continue
		}
		var d struct {
			Title string   `json:"title"`
			Types []string `json:"learning_resource_types"`
		}
		if json.Unmarshal(c.files[name], &d) != nil {
			continue
		}
		typ := ""
		for _, t := range d.Types {
			if Types[strings.ToLower(t)] {
				typ = t
				break
			}
		}
		if typ == "" && len(d.Types) > 0 {
			typ = d.Types[0]
		}
		add(parts[1], d.Title, typ)
	}
	for _, name := range slices.Sorted(maps.Keys(c.files)) {
		if !strings.HasPrefix(name, "pages/") && !strings.HasPrefix(name, "video_galleries/") {
			continue
		}
		typ := pageType(name)
		doc := parse(c.files[name])
		if doc == nil {
			continue
		}
		walk(doc, func(n *html.Node) bool {
			if n.Type == html.ElementNode && n.Data == "a" {
				if m := resRE.FindStringSubmatch(attr(n, "href")); m != nil {
					add(m[1], text(n), typ)
				}
				return false
			}
			return true
		})
	}
	// A link from a page can have a short text, as PDF. Such a title is not the title of the material.
	for i := range out {
		if t := strings.ToLower(out[i].Title); t == "" || t == "pdf" || t == "mp4" || strings.HasPrefix(t, "(pdf") {
			out[i].Title = ""
		}
	}
	return slices.DeleteFunc(out, func(m Material) bool { return m.Title == "" })
}

// pageType gives the learning resource type of the materials that a page links.
func pageType(name string) string {
	switch {
	case strings.HasPrefix(name, "video_galleries/"), strings.Contains(name, "video-lectures"), strings.Contains(name, "lecture-videos"):
		return "Lecture Videos"
	case strings.Contains(name, "lecture-notes"), strings.Contains(name, "lecture-summaries"):
		return "Lecture Notes"
	case strings.Contains(name, "recitation"):
		return "Recitation Notes"
	case strings.Contains(name, "assignments"), strings.Contains(name, "problem-sets"):
		return "Problem Sets"
	case strings.Contains(name, "exams"), strings.Contains(name, "quizzes"):
		return "Exams"
	}
	return ""
}

// textbook gives the ISBN of the textbook when the syllabus and the readings name exactly one ISBN.
func (c *Course) textbook() string {
	var found []string
	for _, name := range slices.Sorted(maps.Keys(c.files)) {
		if !strings.HasPrefix(name, "pages/") || (!strings.Contains(name, "syllabus") && !strings.Contains(name, "readings")) {
			continue
		}
		doc := parse(c.files[name])
		if doc == nil {
			continue
		}
		for _, m := range isbnRE.FindAllStringSubmatch(text(doc), -1) {
			if n, err := ids.Normalize("isbn", m[1]); err == nil {
				addOnce(&found, n)
			}
		}
	}
	if len(found) != 1 {
		return ""
	}
	return found[0]
}

// zipLink finds the link to the package on a download page.
func zipLink(data []byte) string {
	doc := parse(data)
	href := ""
	walk(doc, func(n *html.Node) bool {
		if href == "" && n.Type == html.ElementNode && n.Data == "a" && strings.HasSuffix(strings.ToLower(attr(n, "href")), ".zip") {
			href = attr(n, "href")
		}
		return href == ""
	})
	return href
}

func parse(data []byte) *html.Node {
	if len(data) == 0 {
		return nil
	}
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return doc
}

// walk calls fn with n and each node under n, in document order. When fn gives false, walk skips the nodes under that node.
func walk(n *html.Node, fn func(*html.Node) bool) {
	if n == nil || !fn(n) {
		return
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		walk(ch, fn)
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	return n.Type == html.ElementNode && slices.Contains(strings.Fields(attr(n, "class")), class)
}

func ancestor(n *html.Node, fn func(*html.Node) bool) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && fn(p) {
			return true
		}
	}
	return false
}

// text gives the text under n with the white space folded. A br and the end of a block give one space.
func text(n *html.Node) string {
	var b strings.Builder
	walk(n, func(x *html.Node) bool {
		switch {
		case x.Type == html.TextNode:
			b.WriteString(x.Data)
		case x.Type == html.ElementNode && (x.Data == "script" || x.Data == "style"):
			return false
		case x.Type == html.ElementNode && (x.Data == "br" || x.Data == "p" || x.Data == "li" || x.Data == "div"):
			b.WriteByte(' ')
		}
		return true
	})
	return strings.Join(strings.Fields(b.String()), " ")
}
