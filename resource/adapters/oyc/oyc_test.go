package oyc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource/cluster"
)

func records(t *testing.T) []cluster.Record {
	t.Helper()
	f, err := os.Open("testdata/courses.html")
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

func TestCourses(t *testing.T) {
	f, err := os.Open("testdata/courses.html")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	cs, err := Courses(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 8 {
		t.Fatalf("%d courses, want 8", len(cs))
	}
	want := Course{Department: "African American Studies", Number: "AFAM 162", Title: "African American History: From Emancipation to the Present (2010)", Professor: "Jonathan Holloway", Semester: "Spring 2010", Path: "african-american-studies/afam-162"}
	if cs[0] != want {
		t.Errorf("course %+v", cs[0])
	}
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
	// AFAM 162, CHEM 125a, PHYS 200, PHYS 201 and the two offerings of ECON 252 have no subject in the MSC, so the adapter drops them.
	if !slices.Equal(got, []string{"economics/econ-251", "economics/econ-159"}) {
		t.Fatalf("records %v", got)
	}
	r := rs[1]
	if r.Level != "manifestation" || r.Kind != "course" || r.Title != "Game Theory" || r.Language != "en" || r.Date != "2007" || r.Edition != "Fall 2007" || r.Publisher != "Open Yale Courses" || r.Links[0] != "https://oyc.yale.edu/economics/econ-159" {
		t.Errorf("record %+v", r)
	}
	if r.IDs["course"][0] != "03v76x132:ECON159" || r.IDs["offering"][0] != "03v76x132:ECON159:2007-fall" || r.IDs["oyc"][0] != "economics/econ-159" {
		t.Errorf("ids %+v", r.IDs)
	}
	if len(r.Creators) != 1 || r.Creators[0].Name != "Ben Polak" || r.Creators[0].Role != "instructor" {
		t.Errorf("creators %+v", r.Creators)
	}
}

func TestTitle(t *testing.T) {
	c := Course{Number: "ECON 252", Title: "Financial Markets (2008)", Semester: "Spring 2008", Path: "economics/econ-252-08"}
	r := SourceRecord(c)
	if r.Title != "Financial Markets" || r.IDs["offering"][0] != "03v76x132:ECON252:2008-spring" || r.IDs["oyc"][0] != "economics/econ-252-08" {
		t.Errorf("record %+v", r)
	}
}

func TestMath(t *testing.T) {
	for _, c := range []struct {
		c    Course
		want bool
	}{
		{Course{Department: "Mathematics", Number: "MATH 120"}, true},
		{Course{Department: "Statistics", Number: "MATH 241"}, true},
		{Course{Department: "Physics", Number: "PHYS 201"}, false},
		{Course{Department: "Economics", Number: "ECON 252"}, false},
		{Course{Department: "Chemistry", Number: "CHEM 125a"}, false},
	} {
		if got := Math(c.c); got != c.want {
			t.Errorf("Math(%s) = %v", c.c.Number, got)
		}
	}
}

func TestTerm(t *testing.T) {
	for in, want := range map[string]string{"Fall 2007": "2007-fall", "Autumn 2010": "2010-fall", "spring 2011": "2011-spring", "2010": "", "": ""} {
		if got := (Course{Semester: in}).Term(); got != want {
			t.Errorf("Term(%q) = %q, want %q", in, got, want)
		}
	}
	if got := (Course{Number: "CHEM 125a", Semester: "Fall 2008"}).OfferingID(); got != "03v76x132:CHEM125A:2008-fall" {
		t.Errorf("offering %q", got)
	}
	if got := (Course{Number: "CHEM 125a"}).OfferingID(); got != "" {
		t.Errorf("an offering with no term has the id %q", got)
	}
}

func TestInstructors(t *testing.T) {
	got := Course{Professor: "Jane Roe and John Doe, A. B. Smith"}.Instructors()
	if !slices.Equal(got, []string{"Jane Roe", "John Doe", "A. B. Smith"}) {
		t.Errorf("instructors %q", got)
	}
}

func TestErrors(t *testing.T) {
	for _, in := range []string{"", "<html><body><p>No table</p></body></html>", "<table><tr><th>Department</th></tr></table>"} {
		if _, err := Courses(strings.NewReader(in)); err == nil {
			t.Errorf("%q gives no error", in)
		}
	}
}

// TestPull reads the course list from a test server, as m2 res import does, and checks that the records have no text.
func TestPull(t *testing.T) {
	page, err := os.ReadFile("testdata/courses.html")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/courses" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	}))
	defer srv.Close()
	res, err := ingest.Pull(context.Background(), Adapter{}, ingest.Options{Root: t.TempDir(), Cache: t.TempDir(), URL: srv.URL + "/courses", Date: "2026-10-31", Client: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Manifest.Records != 2 || res.Manifest.Classes[ingest.RecordOnly] != 2 {
		t.Errorf("manifest %+v", res.Manifest)
	}
	src := Adapter{}.Source()
	f, err := os.Open("testdata/courses.html")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	recs, err := ingest.Import(Adapter{}, &src, f)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if r.Text != "" {
			t.Errorf("%s has text", r.ID)
		}
	}
	i := slices.IndexFunc(recs, func(r ingest.Record) bool { return r.ID == "economics/econ-251" })
	if i < 0 || recs[i].Meta["offering"] != "03v76x132:ECON251:2009-fall" || recs[i].Meta["department"] != "Economics" || recs[i].URL != "https://oyc.yale.edu/economics/econ-251" {
		t.Errorf("records %+v", recs)
	}
}

func TestSource(t *testing.T) {
	s := Adapter{}.Source()
	if s.ID != "oyc" || s.Class != ingest.RecordOnly || len(s.Problems()) > 0 || (Adapter{}).Dump() != "https://oyc.yale.edu/courses" {
		t.Errorf("source %+v %v", s, s.Problems())
	}
	if _, ok := ingest.Lookup("oyc"); !ok {
		t.Error("the adapter is not registered")
	}
}
