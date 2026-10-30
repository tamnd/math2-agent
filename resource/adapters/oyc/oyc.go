// Package oyc is the adapter of Open Yale Courses (spec corpus resources RS5.2 and RS5.5, phase R2). It reads the public course list of Open Yale Courses and gives a source record for each course with substantial mathematics.
//
// The snapshot is the course list page, https://oyc.yale.edu/courses. It is one HTML table with the department, the course number, the title, the professor and the term of each course, and the link to the page of the course. robots.txt of the site allows /courses and the course pages. The terms of use do not forbid automated reading. The adapter needs no key and no login.
//
// Open Yale Courses has no department of mathematics. Its 40 courses are courses of Yale College in the humanities, the social sciences and the sciences. The adapter keeps a course of a department of mathematics, if the site adds one, and the courses in Keep: game theory (MSC 91A) and financial theory (MSC 91G) of the Department of Economics. It drops the other courses, because their subject is not in the MSC. The courses of physics use calculus, but their subject is physics.
//
// One row of the list is one offering of a course, as ECON 159 in Fall 2007. The record of the row is the site of the offering, at the level manifestation. It has the course id <ror>:<code>, as 03v76x132:ECON159, the offering id <course>:<term>, as 03v76x132:ECON159:2007-fall, and the oyc id, the path of the page of the offering, as economics/econ-159.
//
// The lectures and the materials are under CC BY-NC-SA 3.0. The adapter keeps the facts of the list only: the code, the title, the professor and the term. It reads no course page and keeps no text of a lecture. The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/oyc"
package oyc

import (
	"errors"
	"fmt"
	"html"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// ROR is the ROR id of Yale University, the institution of each course of Open Yale Courses.
const ROR = "03v76x132"

// Base is the home of Open Yale Courses.
const Base = "https://oyc.yale.edu"

// MaxPage is the largest course list that the adapter reads. The list of 2026 has 68 kB.
const MaxPage = 8 << 20

// Keep gives the codes of the courses whose subject is in the MSC and that are not in a department of mathematics.
var Keep = []string{
	"ECON159", // Game Theory
	"ECON251", // Financial Theory
}

// Adapter is the adapter of the course list of Open Yale Courses.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of Open Yale Courses.
func (Adapter) Source() ingest.Source {
	return ingest.Source{ID: "oyc", Name: "Open Yale Courses", Kind: "course-platform", Home: Base, Licence: "CC-BY-NC-SA-3.0", Class: ingest.RecordOnly, Channel: "page",
		Terms:       &ingest.Terms{Robots: "allows /courses and the course pages"},
		Attribution: "Yale University: Open Yale Courses, " + Base + ", CC BY-NC-SA 3.0"}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "oyc/1" }

// Dump returns the URL of the course list.
func (Adapter) Dump() string { return Base + "/courses" }

// Course is one row of the course list: one offering of a course.
type Course struct {
	Department string // as Economics
	Number     string // as ECON 159
	Title      string // as Game Theory
	Professor  string // as Ben Polak
	Semester   string // as Fall 2007
	Path       string // the path of the page of the offering, as economics/econ-159
}

// The headers attribute of each cell of the table names its column.
const (
	colDepartment = "view-title-table-column"
	colNumber     = "view-field-course-number-table-column"
	colTitle      = "view-title-1-table-column"
	colProfessor  = "view-field-course-professor-last-name-table-column"
	colSemester   = "view-field-semester-table-column"
)

var (
	rowRE   = regexp.MustCompile(`(?s)<tr[^>]*>(.*?)</tr>`)
	cellRE  = regexp.MustCompile(`(?s)<td[^>]*\bheaders="([^"]+)"[^>]*>(.*?)</td>`)
	hrefRE  = regexp.MustCompile(`href="/([^"?#]+)"`)
	tagRE   = regexp.MustCompile(`<[^>]*>`)
	termRE  = regexp.MustCompile(`(?i)^(spring|summer|fall|autumn|winter)\s+((?:19|20)[0-9]{2})$`)
	yearRE  = regexp.MustCompile(`\s*\((?:19|20)[0-9]{2}\)$`)
	splitRE = regexp.MustCompile(`\s*(?:,|;|&|\band\b)\s*`)
)

func text(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tagRE.ReplaceAllString(s, " "))), " ")
}

// Courses reads the course list page and gives each offering of the list, in the order of the page.
func Courses(r io.Reader) ([]Course, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxPage))
	if err != nil {
		return nil, fmt.Errorf("oyc: %w", err)
	}
	var out []Course
	table := false
	for _, row := range rowRE.FindAllStringSubmatch(string(data), -1) {
		var c Course
		for _, cell := range cellRE.FindAllStringSubmatch(row[1], -1) {
			v := text(cell[2])
			switch cell[1] {
			case colDepartment:
				c.Department = v
			case colNumber:
				c.Number = v
				if m := hrefRE.FindStringSubmatch(cell[2]); m != nil {
					c.Path = m[1]
				}
			case colTitle:
				c.Title = v
			case colProfessor:
				c.Professor = v
			case colSemester:
				c.Semester = v
			}
		}
		if c.Number == "" && c.Title == "" {
			continue
		}
		table = true
		if c.Code() != "" && c.Title != "" {
			out = append(out, c)
		}
	}
	if !table {
		return nil, errors.New("oyc: the page has no course list")
	}
	return out, nil
}

// Code gives the code of the course in upper case with no space, as ECON159.
func (c Course) Code() string { return strings.ToUpper(strings.Join(strings.Fields(c.Number), "")) }

// CourseID gives the course id of RS5.2, as 03v76x132:ECON159.
func (c Course) CourseID() string {
	if c.Code() == "" {
		return ""
	}
	return ROR + ":" + c.Code()
}

// Term gives the term of the offering, as 2007-fall, or an empty string when the semester is not a season and a year.
func (c Course) Term() string {
	m := termRE.FindStringSubmatch(strings.TrimSpace(c.Semester))
	if m == nil {
		return ""
	}
	season := strings.ToLower(m[1])
	if season == "autumn" {
		season = "fall"
	}
	return m[2] + "-" + season
}

// OfferingID gives the offering id, as 03v76x132:ECON159:2007-fall, or an empty string for an offering with no term.
func (c Course) OfferingID() string {
	if c.CourseID() == "" || c.Term() == "" {
		return ""
	}
	return c.CourseID() + ":" + c.Term()
}

// URL gives the page of the offering.
func (c Course) URL() string {
	if c.Path == "" {
		return Base + "/courses"
	}
	return Base + "/" + c.Path
}

// Instructors splits the professor of a course into names.
func (c Course) Instructors() []string {
	var out []string
	for _, n := range splitRE.Split(c.Professor, -1) {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// Math reports whether a course has substantial mathematics: a course of a department of mathematics, a course with the subject code MATH, or a course in Keep.
func Math(c Course) bool {
	return strings.Contains(strings.ToLower(c.Department), "mathematic") || strings.HasPrefix(c.Code(), "MATH") || slices.Contains(Keep, c.Code())
}

// SourceRecord makes the source record of one offering. The title has no year, because the term gives it. The level is manifestation, because the page is one site of the offering.
func SourceRecord(c Course) cluster.Record {
	rec := cluster.Record{NativeID: c.Path, Level: "manifestation", Kind: "course", Title: yearRE.ReplaceAllString(c.Title, ""), Language: "en", Edition: c.Semester, Publisher: "Open Yale Courses",
		IDs: resource.IDs{}, Links: []string{c.URL()}}
	if rec.NativeID == "" {
		rec.NativeID = strings.ToLower(c.Code())
	}
	if t := c.Term(); t != "" {
		rec.Date = t[:4]
	}
	put := func(key, v string) {
		if n, err := ids.Normalize(key, v); err == nil {
			rec.IDs[key] = []string{n}
		}
	}
	put("course", c.CourseID())
	put("offering", c.OfferingID())
	put("oyc", c.Path)
	for _, n := range c.Instructors() {
		rec.Creators = append(rec.Creators, resource.Creator{Name: n, Role: "instructor"})
	}
	return rec
}

func each(r io.Reader, fn func(Course) error) error {
	cs, err := Courses(r)
	if err != nil {
		return err
	}
	for _, c := range cs {
		if !Math(c) {
			continue
		}
		if err := fn(c); err != nil {
			return err
		}
	}
	return nil
}

// Read reads the snapshot and emits one ingest record for each offering with substantial mathematics. A record has no text.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(c Course) error {
		rec := SourceRecord(c)
		meta := map[string]string{"course": c.CourseID(), "department": c.Department}
		if o := c.OfferingID(); o != "" {
			meta["offering"] = o
			meta["term"] = c.Semester
		}
		if c.Professor != "" {
			meta["instructors"] = strings.Join(c.Instructors(), "; ")
		}
		return emit(ingest.Record{ID: rec.NativeID, Kind: rec.Kind, Title: rec.Title, URL: rec.Links[0], Meta: meta})
	})
}

// Records reads the snapshot and emits one source record for each offering with substantial mathematics (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(c Course) error { return emit(SourceRecord(c)) })
}
