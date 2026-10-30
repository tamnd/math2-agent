package ocw

import (
	"cmp"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"

	"github.com/tamnd/math2-agent/resource"
)

// MaxTitle is the longest title of a node, in characters (audit H09).
const MaxTitle = 500

var (
	labelRE   = regexp.MustCompile(`^(L|LEC|LECTURE|R|REC|RECITATION)?0*([0-9]+)$`)
	lectureRE = regexp.MustCompile(`(?i)^(lecture|lec\.?|recitation|rec\.?)\s*0*([0-9]+)\b\s*[:.,–-]?\s*(.*)$`)
	monthRE   = regexp.MustCompile(`(?i)\b(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\.?\s+([0-9]{1,2})\b`)
	slashRE   = regexp.MustCompile(`\b([0-9]{1,2})/([0-9]{1,2})\b`)
	rangeRE   = regexp.MustCompile(`\b([0-9]{1,2})\.([0-9]{1,2})\s*(?:-|–|to)\s*([0-9]{1,2})\.([0-9]{1,2})\b`)
	sectionRE = regexp.MustCompile(`\b([0-9]{1,2})\.([0-9]{1,2})\b`)
	chapterRE = regexp.MustCompile(`(?i)\b(?:chapters?|chap\.|ch\.)\s*([0-9]{1,2})\b`)
)

// The headers of the columns of a table of sessions, in upper case.
var (
	sessionCols = []string{"SES #", "SES#", "SES", "SESSION", "SESSION #", "LEC #", "LEC#", "LEC", "LECTURE", "LECTURE #", "#"}
	topicCols   = []string{"TOPICS", "TOPIC", "TITLE", "LECTURE TOPICS", "SUBJECT", "SUBJECTS"}
	dateCols    = []string{"DATE", "DATES"}
	readingCols = []string{"READINGS", "READING", "READING ASSIGNMENTS", "ASSIGNED READINGS"}
)

// pageRank gives the order in which Sessions reads the pages, and whether a page can add a session. The calendar, the readings and the lecture notes list the sessions. The assignments and the exams only add materials to the sessions.
func pageRank(name string) (int, bool, bool) {
	switch {
	case strings.Contains(name, "calendar"), strings.Contains(name, "syllabus-calendar"):
		return 0, true, true
	case strings.Contains(name, "lecture-notes"), strings.Contains(name, "lecture-summaries"), strings.Contains(name, "video-lectures"):
		return 1, true, true
	case strings.Contains(name, "readings"):
		return 2, true, true
	case strings.Contains(name, "assignments"), strings.Contains(name, "problem-sets"), strings.Contains(name, "exams"), strings.Contains(name, "recitations"):
		return 3, false, true
	}
	return 0, false, false
}

type builder struct {
	c     *Course
	nodes []resource.Node
	at    map[string]int
	ids   map[string]string // the material id of each slug of a material with its own page
}

// Sessions gives the sessions of the offering (RS5.4). It reads the tables of the calendar, the readings and the lecture notes, and the assignments and the exams add their materials. A course with no table gets its sessions from the materials whose titles start with Lecture n. A session has its key (L01 for lecture 1, R01 for recitation 1), its number, its topics as the title, its date, its materials and its readings.
func (c *Course) Sessions() []resource.Node {
	b := &builder{c: c, at: map[string]int{}, ids: map[string]string{}}
	for _, m := range c.Materials {
		if m.Own() {
			b.ids[m.Slug] = "ocw:" + c.materialID(m)
		}
	}
	type page struct {
		name   string
		rank   int
		create bool
	}
	var pages []page
	for _, name := range slices.Sorted(maps.Keys(c.files)) {
		if !strings.HasPrefix(name, "pages/") || !strings.HasSuffix(name, ".html") {
			continue
		}
		if r, create, ok := pageRank(name); ok {
			pages = append(pages, page{name, r, create})
		}
	}
	slices.SortStableFunc(pages, func(x, y page) int { return cmp.Compare(x.rank, y.rank) })
	for _, p := range pages {
		doc := parse(c.files[p.name])
		walk(doc, func(n *html.Node) bool {
			if n.Type == html.ElementNode && n.Data == "table" {
				b.table(n, p.create)
				return false
			}
			return true
		})
	}
	fromTables := len(b.nodes) > 0
	var made []resource.Node
	for _, m := range c.Materials {
		id, ok := b.ids[m.Slug]
		if !ok {
			continue
		}
		mm := lectureRE.FindStringSubmatch(m.Title)
		if mm == nil {
			continue
		}
		key, number := keyOf(mm[1] + mm[2])
		if i, ok := b.at[key]; ok {
			addOnce(&b.nodes[i].Materials, id)
			continue
		}
		if fromTables {
			continue
		}
		made = append(made, resource.Node{Key: key, Kind: "session", Number: number, Title: cut(mm[3]), Materials: []string{id}})
	}
	if !fromTables {
		// The materials of one session, as the notes and the video of a lecture, come once each.
		var merged []resource.Node
		idx := map[string]int{}
		for _, n := range made {
			if i, ok := idx[n.Key]; ok {
				for _, m := range n.Materials {
					addOnce(&merged[i].Materials, m)
				}
				if merged[i].Title == "" {
					merged[i].Title = n.Title
				}
				continue
			}
			idx[n.Key] = len(merged)
			merged = append(merged, n)
		}
		slices.SortFunc(merged, func(x, y resource.Node) int {
			nx, _ := strconv.Atoi(x.Number)
			ny, _ := strconv.Atoi(y.Number)
			return cmp.Or(cmp.Compare(x.Key[:1], y.Key[:1]), cmp.Compare(nx, ny))
		})
		return merged
	}
	return b.nodes
}

// keyOf gives the key and the number of a session from its label: L01 and 1 for 1, L1 or Lecture 1, R01 and 1 for R1. Another label, as Ex1, gives its letters and digits in upper case and no number.
func keyOf(label string) (string, string) {
	l := strings.ToUpper(strings.Join(strings.Fields(label), ""))
	l = strings.TrimSuffix(l, ".")
	if m := labelRE.FindStringSubmatch(l); m != nil {
		n, err := strconv.Atoi(m[2])
		if err == nil {
			p := "L"
			if strings.HasPrefix(m[1], "R") {
				p = "R"
			}
			return fmt.Sprintf("%s%02d", p, n), strconv.Itoa(n)
		}
	}
	var b strings.Builder
	for _, r := range l {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String(), ""
}

func header(s string) string {
	return strings.TrimSuffix(strings.ToUpper(strings.Join(strings.Fields(s), " ")), ".")
}

func cells(tr *html.Node) []*html.Node {
	var out []*html.Node
	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
			out = append(out, c)
		}
	}
	return out
}

// table reads one table of sessions. A table with no column of sessions is not a table of sessions.
func (b *builder) table(t *html.Node, create bool) {
	var rows [][]*html.Node
	walk(t, func(n *html.Node) bool {
		if n != t && n.Type == html.ElementNode && n.Data == "table" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "tr" {
			rows = append(rows, cells(n))
			return false
		}
		return true
	})
	if len(rows) < 2 {
		return
	}
	col := func(names []string) int {
		for i, c := range rows[0] {
			if slices.Contains(names, header(text(c))) {
				return i
			}
		}
		return -1
	}
	ses, top, date, read := col(sessionCols), col(topicCols), col(dateCols), col(readingCols)
	if ses < 0 {
		return
	}
	get := func(row []*html.Node, i int) *html.Node {
		if i < 0 || i >= len(row) {
			return nil
		}
		return row[i]
	}
	for _, row := range rows[1:] {
		cell := get(row, ses)
		if cell == nil {
			continue
		}
		key, number := keyOf(text(cell))
		if key == "" {
			continue
		}
		i, ok := b.at[key]
		if !ok {
			if !create {
				continue
			}
			i = len(b.nodes)
			b.at[key] = i
			b.nodes = append(b.nodes, resource.Node{Key: key, Kind: "session", Number: number})
		}
		n := &b.nodes[i]
		if tc := get(row, top); tc != nil && n.Title == "" {
			items := items(tc)
			n.Title = cut(strings.Join(items, "; "))
			if len(items) > 1 {
				for k, it := range items {
					n.Children = append(n.Children, resource.Node{Key: fmt.Sprintf("%s.T%d", key, k+1), Kind: "topic", Number: strconv.Itoa(k + 1), Title: cut(it)})
				}
			}
		}
		if dc := get(row, date); dc != nil && n.Date == "" {
			n.Date = b.date(text(dc))
		}
		if rc := get(row, read); rc != nil {
			for _, r := range b.c.readings(text(rc)) {
				if !slices.ContainsFunc(n.Readings, func(x resource.Reading) bool {
					return x.Resource == r.Resource && maps.Equal(x.At, r.At) && x.Text == r.Text
				}) {
					n.Readings = append(n.Readings, r)
				}
			}
		}
		for _, c := range row {
			walk(c, func(x *html.Node) bool {
				if x.Type == html.ElementNode && x.Data == "a" {
					if m := resRE.FindStringSubmatch(attr(x, "href")); m != nil {
						if id, ok := b.ids[m[1]]; ok {
							addOnce(&n.Materials, id)
						}
					}
					return false
				}
				return true
			})
		}
	}
}

// items gives the topics of a cell: the items of its list, or its lines.
func items(cell *html.Node) []string {
	var lis []string
	walk(cell, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "li" {
			if t := text(n); t != "" {
				lis = append(lis, t)
			}
			return false
		}
		return true
	})
	if len(lis) > 0 {
		return lis
	}
	var b strings.Builder
	walk(cell, func(n *html.Node) bool {
		switch {
		case n.Type == html.TextNode:
			b.WriteString(n.Data)
		case n.Type == html.ElementNode && (n.Data == "br" || n.Data == "p" || n.Data == "div"):
			b.WriteByte('\n')
		}
		return true
	})
	var out []string
	for _, l := range strings.Split(b.String(), "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// date gives the date of a session as YYYY-MM-DD, with the year of the offering. A cell with no month and day gives an empty string.
func (b *builder) date(s string) string {
	year, err := strconv.Atoi(b.c.Year)
	if err != nil {
		return ""
	}
	month, day := 0, 0
	if m := monthRE.FindStringSubmatch(s); m != nil {
		month = slices.Index([]string{"jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"}, strings.ToLower(m[1])) + 1
		day, _ = strconv.Atoi(m[2])
	} else if m := slashRE.FindStringSubmatch(s); m != nil {
		month, _ = strconv.Atoi(m[1])
		day, _ = strconv.Atoi(m[2])
	}
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
}

// readings gives the readings of one cell. With the one textbook of the course, each section and each chapter of the cell is one reading of the book, as isbn:<isbn> at section 1.1. With no textbook, the cell is one reading with the citation as text, for the reference resolver.
func (c *Course) readings(s string) []resource.Reading {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "none") || s == "-" || s == "\u2014" {
		return nil
	}
	txt := cut(s)
	if c.Textbook == "" {
		return []resource.Reading{{Text: txt}}
	}
	book := "isbn:" + c.Textbook
	var out []resource.Reading
	seen := map[string]bool{}
	add := func(kind, v string) {
		if !seen[kind+v] {
			seen[kind+v] = true
			out = append(out, resource.Reading{Resource: book, At: map[string]string{kind: v}, Text: txt})
		}
	}
	rest := s
	for _, m := range rangeRE.FindAllStringSubmatch(s, -1) {
		a, _ := strconv.Atoi(m[2])
		z, _ := strconv.Atoi(m[4])
		if m[1] != m[3] || z < a || z-a > 20 {
			continue
		}
		for k := a; k <= z; k++ {
			add("section", m[1]+"."+strconv.Itoa(k))
		}
		rest = strings.Replace(rest, m[0], " ", 1)
	}
	for _, m := range sectionRE.FindAllStringSubmatch(rest, -1) {
		ch, _ := strconv.Atoi(m[1])
		sec, _ := strconv.Atoi(m[2])
		add("section", strconv.Itoa(ch)+"."+strconv.Itoa(sec))
	}
	for _, m := range chapterRE.FindAllStringSubmatch(s, -1) {
		ch, _ := strconv.Atoi(m[1])
		add("chapter", strconv.Itoa(ch))
	}
	if len(out) == 0 {
		return []resource.Reading{{Resource: book, Text: txt}}
	}
	return out
}

// cut gives s with at most MaxTitle characters.
func cut(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= MaxTitle {
		return s
	}
	return string([]rune(s)[:MaxTitle])
}
