// Package nptel is the adapter of NPTEL, the National Programme on Technology Enhanced Learning of India (spec corpus resources RS5.5, phase R2). It reads the public course list of NPTEL and gives a source record for each course of the discipline Mathematics.
//
// The snapshot is the data file of the course list page, https://nptel.ac.in/courses/__data.json. The site builds this file for its own page, and it needs no key and no login. robots.txt of NPTEL allows all paths. The file has the id, the title, the institute and the instructors of each course, and no dates and no language. So a record has no offering, and its language comes from the title: Hindi or another language of India when the title names it, else English. The adapter takes no text of the course pages. The licence of the materials is to read (RQ10), so the records are facts only.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/nptel"
package nptel

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Home is the home page of NPTEL.
const Home = "https://nptel.ac.in"

// Discipline is the name of the discipline that the adapter keeps.
const Discipline = "Mathematics"

// Adapter is the adapter of the course list of NPTEL.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of NPTEL.
func (Adapter) Source() ingest.Source {
	return ingest.Source{ID: "nptel", Name: "NPTEL, the National Programme on Technology Enhanced Learning", Kind: "course-platform", Home: Home, Licence: "LicenseRef-facts", Class: ingest.RecordOnly,
		Channel: "page", Terms: &ingest.Terms{Robots: "allows all paths"}, Attribution: "NPTEL, " + Home}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "nptel/1" }

// Dump returns the URL of the data file of the course list.
func (Adapter) Dump() string { return Home + "/courses/__data.json" }

// Course is one course of the course list.
type Course struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Institute   string `json:"instituteName"`
	Professor   string `json:"professor"`
	Discipline  int    `json:"disciplineId"`
	ContentType string `json:"contentType"`
}

type list struct {
	Courses     []Course `json:"courses"`
	Disciplines []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"disciplines"`
}

// ROR gives the ROR id of the institutes of NPTEL that teach the most courses of mathematics. A course of another institute has no course id until the institution register names its institute.
var ROR = map[string]string{
	"IIT Bombay":     "02qyf5152",
	"IIT Delhi":      "049tgcd06",
	"IIT Guwahati":   "0022nd079",
	"IIT Kanpur":     "05pjsgx75",
	"IIT Kharagpur":  "03w5sq511",
	"IIT Madras":     "03v0r5n49",
	"IIT Roorkee":    "00582g326",
	"IISc Bangalore": "04dese585",
}

// Courses reads the data file of the course list and gives the courses of the discipline Mathematics. The file is in the form of the devalue library of SvelteKit: one flat array of values, where an object or an array holds the indices of its values.
func Courses(r io.Reader) ([]Course, error) {
	var page struct {
		Nodes []*struct {
			Type string            `json:"type"`
			Data []json.RawMessage `json:"data"`
		} `json:"nodes"`
	}
	if err := json.NewDecoder(r).Decode(&page); err != nil {
		return nil, fmt.Errorf("nptel: %w", err)
	}
	for _, n := range page.Nodes {
		if n == nil || n.Type != "data" || len(n.Data) == 0 {
			continue
		}
		v, err := hydrate(n.Data, 0, 0)
		if err != nil {
			return nil, err
		}
		root, ok := v.(map[string]any)
		if !ok || root["courses"] == nil {
			continue
		}
		b, err := json.Marshal(root)
		if err != nil {
			return nil, err
		}
		var l list
		if err := json.Unmarshal(b, &l); err != nil {
			return nil, fmt.Errorf("nptel: %w", err)
		}
		math := -1
		for _, d := range l.Disciplines {
			if strings.EqualFold(strings.TrimSpace(d.Name), Discipline) {
				if math, err = strconv.Atoi(d.ID); err != nil {
					return nil, fmt.Errorf("nptel: the discipline %s has the id %q", Discipline, d.ID)
				}
			}
		}
		if math < 0 {
			return nil, fmt.Errorf("nptel: the course list has no discipline %s", Discipline)
		}
		var out []Course
		for _, c := range l.Courses {
			if c.Discipline == math && c.ID > 0 {
				out = append(out, c)
			}
		}
		return out, nil
	}
	return nil, errors.New("nptel: the data file has no course list")
}

// hydrate gives the value at index i of the flat array of devalue. A negative index is a special value, as undefined or NaN, and gives nil. An array that starts with a string is a typed value, as a date, and gives its string.
func hydrate(data []json.RawMessage, i, depth int) (any, error) {
	if i < 0 {
		return nil, nil
	}
	if i >= len(data) || depth > 64 {
		return nil, fmt.Errorf("nptel: the index %d is not in the data", i)
	}
	raw := data[i]
	switch {
	case len(raw) > 0 && raw[0] == '{':
		var obj map[string]int
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, fmt.Errorf("nptel: value %d: %w", i, err)
		}
		out := make(map[string]any, len(obj))
		for k, j := range obj {
			v, err := hydrate(data, j, depth+1)
			if err != nil {
				return nil, err
			}
			out[k] = v
		}
		return out, nil
	case len(raw) > 0 && raw[0] == '[':
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, fmt.Errorf("nptel: value %d: %w", i, err)
		}
		if len(arr) > 0 && len(arr[0]) > 0 && arr[0][0] == '"' {
			var typed []any
			_ = json.Unmarshal(raw, &typed)
			if len(typed) > 1 {
				return typed[1], nil
			}
			return nil, nil
		}
		out := make([]any, 0, len(arr))
		for _, a := range arr {
			j, err := strconv.Atoi(string(a))
			if err != nil {
				return nil, fmt.Errorf("nptel: value %d: %w", i, err)
			}
			v, err := hydrate(data, j, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("nptel: value %d: %w", i, err)
	}
	return v, nil
}

// Read reads the snapshot and emits one ingest record for each course of mathematics.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	cs, err := Courses(r)
	if err != nil {
		return err
	}
	for _, c := range cs {
		rec := SourceRecord(c)
		if err := emit(ingest.Record{ID: rec.NativeID, Kind: rec.Kind, Title: rec.Title, URL: rec.Links[0]}); err != nil {
			return err
		}
	}
	return nil
}

// Records reads the snapshot and emits one source record for each course of mathematics (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	cs, err := Courses(r)
	if err != nil {
		return err
	}
	for _, c := range cs {
		if err := emit(SourceRecord(c)); err != nil {
			return err
		}
	}
	return nil
}

// honorific finds the titles before a name, as Prof. and Dr.
var honorific = regexp.MustCompile(`^(?:(?:Prof|Dr|Mr|Mrs|Ms)(?:\.\s*|\s+))+`)

// Instructors splits the field professor of a course into names, with no titles.
func Instructors(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '&' || r == ';' }) {
		for _, n := range strings.Split(p, " and ") {
			n = strings.Join(strings.Fields(honorific.ReplaceAllString(strings.TrimSpace(n), "")), " ")
			if n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

// languages gives the BCP 47 code of the languages of India that a title of NPTEL names, as (in Hindi).
var languages = map[string]string{"hindi": "hi", "tamil": "ta", "telugu": "te", "bengali": "bn", "bangla": "bn", "kannada": "kn", "malayalam": "ml", "marathi": "mr", "gujarati": "gu", "odia": "or"}

var inParens = regexp.MustCompile(`\(([^()]*)\)`)

// Language gives the language of a course from its title. NPTEL teaches in English, and a course in another language names it in its title, as (in Hindi) or (Tamil).
func Language(title string) string {
	for _, m := range inParens.FindAllStringSubmatch(title, -1) {
		for _, w := range strings.Fields(strings.ToLower(m[1])) {
			if l, ok := languages[w]; ok {
				return l
			}
		}
	}
	return "en"
}

// CourseID gives the course id of RS5.2, <ror>:<code>, with the code in upper case and with no space. It gives an empty string when the ROR id is not valid.
func CourseID(ror, code string) string {
	code = strings.ToUpper(strings.Join(strings.Fields(code), ""))
	v, err := ids.Normalize("course", ror+":"+code)
	if err != nil {
		return ""
	}
	return v
}

// SourceRecord makes the source record of one course. A course page of NPTEL is one site of the course, so the level is manifestation. The NPTEL id is the code of the course, and the institute is the institution of the course.
func SourceRecord(c Course) cluster.Record {
	id := strconv.FormatInt(c.ID, 10)
	title := strings.Join(strings.Fields(c.Title), " ")
	title = strings.TrimSpace(strings.TrimPrefix(title, "NOC:"))
	inst := strings.Join(strings.Fields(c.Institute), " ")
	rec := cluster.Record{NativeID: id, Level: "manifestation", Kind: "course", Title: title, Language: Language(title), Publisher: inst,
		IDs: resource.IDs{}, Links: []string{Home + "/courses/" + id}}
	if v, err := ids.Normalize("nptel", id); err == nil {
		rec.IDs["nptel"] = []string{v}
	}
	if ror, ok := ROR[inst]; ok {
		if v := CourseID(ror, id); v != "" {
			rec.IDs["course"] = []string{v}
		}
	}
	for _, n := range Instructors(c.Professor) {
		rec.Creators = append(rec.Creators, resource.Creator{Name: n, Role: "instructor"})
	}
	return rec
}
