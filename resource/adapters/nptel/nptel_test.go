package nptel

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource/cluster"
)

func records(t *testing.T) []cluster.Record {
	t.Helper()
	f, err := os.Open("testdata/courses.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var rs []cluster.Record
	if err := (Adapter{}).Records(f, func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestRecords(t *testing.T) {
	rs := records(t)
	var got []string
	for _, r := range rs {
		got = append(got, r.NativeID)
		if why := cluster.Check(&r); why != "" {
			t.Errorf("%s: %s", r.NativeID, why)
		}
	}
	// 101101079 is in Aerospace Engineering and 111104120 is in Statistics, so the discipline decides and not the prefix of the id.
	if !slices.Equal(got, []string{"111101100", "111106134", "111106137", "111107112"}) {
		t.Fatalf("records %v", got)
	}
	r := rs[0]
	if r.Level != "manifestation" || r.Kind != "course" || r.Title != "Measure theory" || r.Language != "en" || r.Publisher != "IIT Bombay" || r.Links[0] != "https://nptel.ac.in/courses/111101100" {
		t.Errorf("record %+v", r)
	}
	if r.IDs["nptel"][0] != "111101100" || r.IDs["course"][0] != "02qyf5152:111101100" {
		t.Errorf("ids %+v", r.IDs)
	}
	if len(r.Creators) != 1 || r.Creators[0].Name != "Inder Kumar Rana" || r.Creators[0].Role != "instructor" {
		t.Errorf("creators %+v", r.Creators)
	}
	if r := rs[1]; r.Publisher != "IISER Pune" || r.IDs["course"] != nil {
		t.Errorf("a course of an institute with no ROR id has a course id: %+v", r)
	}
	if r := rs[3]; len(r.Creators) != 2 || r.Creators[0].Name != "Sanjeev Kumar" || r.Creators[1].Name != "S. K. Gupta" {
		t.Errorf("creators %+v", r.Creators)
	}
}

func TestRead(t *testing.T) {
	f, err := os.Open("testdata/courses.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	src := Adapter{}.Source()
	recs, err := ingest.Import(Adapter{}, &src, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 4 || recs[0].URL != "https://nptel.ac.in/courses/111101100" || recs[0].Text != "" {
		t.Errorf("records %+v", recs)
	}
}

func TestErrors(t *testing.T) {
	for _, in := range []string{
		`{"type":"data","nodes":[null]}`,
		`{"type":"data","nodes":[null,{"type":"data","data":[{"courses":1,"disciplines":2},[],[]]}]}`,
		`{"type":"data","nodes":[null,{"type":"data","data":[{"courses":9}]}]}`,
		`not json`,
	} {
		if _, err := Courses(strings.NewReader(in)); err == nil {
			t.Errorf("%s gives no error", in)
		}
	}
}

func TestInstructors(t *testing.T) {
	got := Instructors("Prof. M.D.Srinivas , Prof.K.Ramasubramanian, Prof M.S. Sriram and Dr. A. B")
	if !slices.Equal(got, []string{"M.D.Srinivas", "K.Ramasubramanian", "M.S. Sriram", "A. B"}) {
		t.Errorf("instructors %q", got)
	}
}

func TestLanguage(t *testing.T) {
	for title, want := range map[string]string{
		"Introduction to Probability & Statistics (in Hindi)":                 "hi",
		"Introduction to Probability Theory and Stochastic Processes (Tamil)": "ta",
		"Multivariate Procedures with R (in Hindi Language)":                  "hi",
		"Advanced Partial Differential Equations (Part I: Distributions)":     "en",
		"Mathematics in India - From Vedic Period to Modern Times":            "en",
	} {
		if got := Language(title); got != want {
			t.Errorf("Language(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestCourseID(t *testing.T) {
	if got := CourseID("02qyf5152", "ma 106"); got != "02qyf5152:MA106" {
		t.Errorf("CourseID = %q", got)
	}
	if got := CourseID("not-a-ror", "MA106"); got != "" {
		t.Errorf("a bad ROR id gives the course id %q", got)
	}
	for name, ror := range ROR {
		if CourseID(ror, "1") == "" {
			t.Errorf("the ROR id %s of %s is not valid", ror, name)
		}
	}
}

func TestSource(t *testing.T) {
	s := Adapter{}.Source()
	if s.ID != "nptel" || s.Class != ingest.RecordOnly || (Adapter{}).Dump() != "https://nptel.ac.in/courses/__data.json" {
		t.Errorf("source %+v", s)
	}
	if _, ok := ingest.Lookup("nptel"); !ok {
		t.Error("the adapter is not registered")
	}
}
