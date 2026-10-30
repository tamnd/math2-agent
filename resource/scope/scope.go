// Package scope decides if a work is in the scope of the catalogue of resources (corpus resources RS9.2). It has the tests S1 to S7 of the scope predicate. The classifier of RS9.3 (test S8) is not in this package.
//
// The tests read a Record, not a resource.Resource, so the census can decide the scope of a record of a catalogue before the record becomes a resource. FromResource makes a Record from a resource.
package scope

import (
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/tamnd/math2-agent/resource"
)

// Record holds the signals of one work that the tests read. In each list, the first value is the primary value when the source gives one.
type Record struct {
	MSC          []string `json:"msc,omitempty"`           // the MSC2020 codes, the primary code first
	ZBMath       bool     `json:"zbmath,omitempty"`        // the work has a record in zbMATH Open
	Arxiv        []string `json:"arxiv,omitempty"`         // the arXiv categories, the primary category first
	LCC          []string `json:"lcc,omitempty"`           // the Library of Congress classes, for example QA300 or QA76.73.P98
	DDC          []string `json:"ddc,omitempty"`           // the Dewey classes, for example 515.7
	UDC          []string `json:"udc,omitempty"`           // the UDC classes, for example 517.9
	CLC          []string `json:"clc,omitempty"`           // the Chinese Library Classification, for example O174
	BK           []string `json:"bk,omitempty"`            // the Basisklassifikation, for example 31.44
	MathHeading  bool     `json:"math_heading,omitempty"`  // a subject heading of the work is a heading of mathematics
	Venues       []string `json:"venues,omitempty"`        // the keys of the venues of the work in the venue register: resource ids or ISSN-L
	Curriculum   bool     `json:"curriculum,omitempty"`    // a curriculum or a syllabus names the work (RS5.6)
	MathUnit     bool     `json:"math_unit,omitempty"`     // the work is a course of a mathematics unit of an institution (RS5.3)
	MainSubjects []string `json:"main_subjects,omitempty"` // the Wikidata items of the property P921 main subject
	Topics       []Topic  `json:"topics,omitempty"`        // the OpenAlex topics of the work
}

// Topic is one OpenAlex topic of a work with its score.
type Topic struct {
	Field string  `json:"field"` // the OpenAlex field of the topic, for example Mathematics
	Score float64 `json:"score"` // from 0 to 1
}

// Env holds the registers and the thresholds that the tests read.
type Env struct {
	Venues         map[string]string `json:"venues,omitempty"`          // the scope of each venue of the venue register (RS6.5): core, mixed or border
	MathItems      map[string]bool   `json:"math_items,omitempty"`      // the Wikidata items of mathematics
	TopicThreshold float64           `json:"topic_threshold,omitempty"` // the lowest score of an OpenAlex topic for test S7 (RS9.3)
}

// The scopes of a venue in the venue register (RS6.5).
const (
	VenueCore   = "core"
	VenueMixed  = "mixed"
	VenueBorder = "border"
)

// MathField is the name of the OpenAlex field of mathematics.
const MathField = "Mathematics"

// Decision is the outcome of the scope predicate and the test that gave it. Value is resource.In, resource.Border, resource.Out or resource.Deferred. Rule is S1 to S7 for an in, the name of the border rule for a border, and empty for out and deferred.
type Decision struct {
	Value string
	Rule  string
}

// A Test is one test of RS9.2 that can say in.
type Test struct {
	Name string
	Run  func(Record, Env) bool
}

// Tests are the tests S1 to S7 in the order of RS9.2.
var Tests = []Test{
	{"S1", func(r Record, _ Env) bool { return S1(r) }},
	{"S2", func(r Record, _ Env) bool { return S2(r) }},
	{"S3", func(r Record, _ Env) bool { return S3(r) }},
	{"S4", S4},
	{"S5", func(r Record, _ Env) bool { return S5(r) }},
	{"S6", func(r Record, _ Env) bool { return S6(r) }},
	{"S7", S7},
}

// Decide applies the tests S1 to S7 in order, then the border rules. A record that no rule decides is out when it has a class, a venue or a subject, because a test would have read it. It is deferred when it has none of them, because only the classifier of RS9.3 can decide it.
func Decide(r Record, env Env) Decision {
	for _, t := range Tests {
		if t.Run(r, env) {
			return Decision{resource.In, t.Name}
		}
	}
	if rule := BorderRule(r, env); rule != "" {
		return Decision{resource.Border, rule}
	}
	if hasSignal(r) {
		return Decision{resource.Out, ""}
	}
	return Decision{resource.Deferred, ""}
}

func hasSignal(r Record) bool {
	return len(r.MSC)+len(r.Arxiv)+len(r.LCC)+len(r.DDC)+len(r.UDC)+len(r.CLC)+len(r.BK)+len(r.Venues)+len(r.MainSubjects)+len(r.Topics) > 0 || r.ZBMath
}

// S1 is the test MSC: a primary MSC2020 code in 00 to 58, 60, 65 or 97, or 91A, or 68Q to 68W with a code of mathematics among the other codes; or a record in zbMATH Open.
func S1(r Record) bool {
	if r.ZBMath {
		return true
	}
	if len(r.MSC) == 0 {
		return false
	}
	p := strings.ToUpper(r.MSC[0])
	if mathMSC(p) {
		return true
	}
	if theoryCS(p) {
		return slices.ContainsFunc(r.MSC[1:], func(c string) bool { return mathMSC(strings.ToUpper(c)) })
	}
	return false
}

// mathMSC reports whether the code is in 00 to 58, 60, 65, 97 or 91A.
func mathMSC(c string) bool {
	n, ok := mscTop(c)
	if !ok {
		return false
	}
	return n <= 58 || n == 60 || n == 65 || n == 97 || strings.HasPrefix(c, "91A")
}

// theoryCS reports whether the code is in 68Q to 68W.
func theoryCS(c string) bool {
	return len(c) >= 3 && c[:2] == "68" && c[2] >= 'Q' && c[2] <= 'W'
}

// mscTop gives the number of the top level of an MSC code.
func mscTop(c string) (int, bool) {
	if len(c) < 2 {
		return 0, false
	}
	n, err := strconv.Atoi(c[:2])
	return n, err == nil
}

// S2 is the test arXiv: a primary category math.* or math-ph; or a primary category cs.LO, cs.DM, cs.CC, cs.IT or stat.TH with a cross-list into math.*.
func S2(r Record) bool {
	if len(r.Arxiv) == 0 {
		return false
	}
	p := r.Arxiv[0]
	if strings.HasPrefix(p, "math.") || p == "math-ph" {
		return true
	}
	switch p {
	case "cs.LO", "cs.DM", "cs.CC", "cs.IT", "stat.TH":
		return mathCross(r.Arxiv[1:])
	}
	return false
}

func mathCross(cs []string) bool {
	return slices.ContainsFunc(cs, func(c string) bool { return strings.HasPrefix(c, "math.") })
}

// S3 is the test library class: LCC QA1 to QA939 except QA75.5 to QA76.95; DDC 510 to 519; UDC 51; CLC O1; BK 31.
func S3(r Record) bool {
	for _, c := range r.LCC {
		if n, ok := lcc(c, "QA"); ok && n >= 1 && n <= 939 && !lccComputing(n) {
			return true
		}
	}
	for _, c := range r.DDC {
		if n, err := strconv.Atoi(leadDigits(c)); err == nil && len(leadDigits(c)) == 3 && n >= 510 && n <= 519 {
			return true
		}
	}
	for _, c := range r.UDC {
		if strings.HasPrefix(leadDigits(c), "51") {
			return true
		}
	}
	for _, c := range r.CLC {
		if c = strings.ToUpper(strings.TrimSpace(c)); strings.HasPrefix(c, "O1") {
			return true
		}
	}
	for _, c := range r.BK {
		if d := leadDigits(c); d == "31" {
			return true
		}
	}
	return false
}

// lcc reads the number of an LCC class with the letters cls, for example 76.73 from QA76.73.P98. The second value is false when the class has other letters or no number.
func lcc(c, cls string) (float64, bool) {
	letters, num := splitLCC(c)
	if letters != cls {
		return 0, false
	}
	end, dot := 0, false
	for ; end < len(num); end++ {
		if num[end] == '.' && !dot && end+1 < len(num) && num[end+1] >= '0' && num[end+1] <= '9' {
			dot = true
			continue
		}
		if num[end] < '0' || num[end] > '9' {
			break
		}
	}
	n, err := strconv.ParseFloat(num[:end], 64)
	return n, err == nil
}

// splitLCC splits an LCC class into its letters and the rest, with no space at the start of the rest.
func splitLCC(c string) (string, string) {
	c = strings.ToUpper(strings.TrimSpace(c))
	i := strings.IndexFunc(c, func(r rune) bool { return !unicode.IsLetter(r) })
	if i < 0 {
		return c, ""
	}
	return c[:i], strings.TrimSpace(c[i:])
}

func lccComputing(n float64) bool { return n >= 75.5 && n <= 76.95 }

// leadDigits gives the digits at the start of a class.
func leadDigits(c string) string {
	c = strings.TrimSpace(c)
	i := strings.IndexFunc(c, func(r rune) bool { return r < '0' || r > '9' })
	if i < 0 {
		return c
	}
	return c[:i]
}

// S4 is the test venue: a venue of the work has the scope core in the venue register.
func S4(r Record, env Env) bool {
	return slices.ContainsFunc(r.Venues, func(v string) bool { return env.Venues[v] == VenueCore })
}

// S5 is the test curriculum: a curriculum or a syllabus names the work.
func S5(r Record) bool { return r.Curriculum }

// S6 is the test course: the work is a course of a mathematics unit of an institution.
func S6(r Record) bool { return r.MathUnit }

// S7 is the test subject: the Wikidata main subject of the work is an item of mathematics, or an OpenAlex topic of the work is in the field of mathematics with a score at or above the threshold.
func S7(r Record, env Env) bool {
	if slices.ContainsFunc(r.MainSubjects, func(q string) bool { return env.MathItems[q] }) {
		return true
	}
	return slices.ContainsFunc(r.Topics, func(t Topic) bool { return t.Field == MathField && t.Score >= env.TopicThreshold })
}

// The names of the border rules of RS9.2.
const (
	BorderMSC   = "border-msc"
	BorderArxiv = "border-arxiv"
	BorderClass = "border-class"
	BorderVenue = "border-venue"
)

// BorderRule gives the name of the first border rule of RS9.2 that holds, or an empty string. It does not check the tests S1 to S7, so Decide calls it only when no test says in.
func BorderRule(r Record, env Env) string {
	if len(r.MSC) > 0 {
		p := strings.ToUpper(r.MSC[0])
		if n, ok := mscTop(p); ok && (n == 62 || n == 68 || (n >= 70 && n <= 94)) {
			return BorderMSC
		}
	}
	if len(r.Arxiv) > 0 {
		p, rest := r.Arxiv[0], r.Arxiv[1:]
		switch {
		case strings.HasPrefix(p, "stat."):
			return BorderArxiv
		case strings.HasPrefix(p, "cs.") && mathCross(rest):
			return BorderArxiv
		case !strings.HasPrefix(p, "cs.") && slices.Contains(rest, "math-ph"):
			return BorderArxiv
		}
	}
	for _, c := range r.LCC {
		if n, ok := lcc(c, "QA"); ok && lccComputing(n) {
			return BorderClass
		}
		if r.MathHeading && (lccClass(c, "QB") || lccClass(c, "QC")) {
			return BorderClass
		}
	}
	if slices.ContainsFunc(r.Venues, func(v string) bool {
		s := env.Venues[v]
		return s == VenueMixed || s == VenueBorder
	}) {
		return BorderVenue
	}
	return ""
}

// lccClass reports whether the LCC class has exactly the letters cls.
func lccClass(c, cls string) bool {
	letters, _ := splitLCC(c)
	return letters == cls
}

// FromResource makes a Record from the fields of a resource: the MSC codes, the subjects, the zbMATH ids and the venues of its part_of relations. The fields Curriculum, MathUnit and MathHeading come from other registers, so the caller sets them.
func FromResource(r *resource.Resource) Record {
	var rec Record
	for _, c := range r.MSC {
		rec.MSC = append(rec.MSC, c.Code)
	}
	for _, s := range r.Subjects {
		switch strings.ToLower(s.System) {
		case "arxiv":
			rec.Arxiv = append(rec.Arxiv, s.Code)
		case "lcc":
			rec.LCC = append(rec.LCC, s.Code)
		case "ddc":
			rec.DDC = append(rec.DDC, s.Code)
		case "udc":
			rec.UDC = append(rec.UDC, s.Code)
		case "clc":
			rec.CLC = append(rec.CLC, s.Code)
		case "bk":
			rec.BK = append(rec.BK, s.Code)
		case "wikidata":
			rec.MainSubjects = append(rec.MainSubjects, s.Code)
		}
	}
	rec.ZBMath = len(r.IDs["zbl"]) > 0 || len(r.IDs["zbl_id"]) > 0
	for _, rel := range r.Relations {
		if rel.Kind == "part_of" {
			id, _, _ := strings.Cut(rel.To, "/")
			rec.Venues = append(rec.Venues, id)
		}
	}
	return rec
}
