package ocw

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
)

const slug = "18-06-linear-algebra-spring-2010"

// info is the course information block of a page of OCW, in the markup of the download page of 18.06.
func info(number, term, dept, deptName, topic string) string {
	return `<html><head><link rel="canonical" href="https://ocw.mit.edu/courses/` + slug + `/download/"></head><body>
<div id="course-banner"><h1><a href="/courses/` + slug + `/">Linear Algebra</a></h1></div>
<span class="course-number-term-detail">` + number + ` | ` + term + ` | Undergraduate</span>
<div class="search-variant" data-search-variant="learn">
<a class="course-info-instructor" href="/search/?q=Strang">Prof. Gilbert Strang</a>
<a class="course-info-department" href="/search/?department=` + dept + `">` + deptName + `</a>
<div class="course-info-topics"><ul><li><a class="course-info-topic" href="#">` + topic + `</a>
<ul><li><a class="course-info-topic" href="#">Linear Algebra</a></li></ul></li></ul></div>
</div></body></html>`
}

const download = `<html><body>
<h4>Lecture Videos</h4>
<div class="resource-item"><a class="resource-thumbnail" href="../resources/lecture-1-the-geometry-of-linear-equations/"><div class="resource-type-thumbnail video"></div></a>
<a class="resource-list-title" href="/courses/` + slug + `/resources/lecture-1-the-geometry-of-linear-equations/">Lecture 1: The geometry of linear equations</a></div>
<h4>Problem Sets</h4>
<div class="resource-item"><a class="resource-thumbnail" href="../resources/pset1/pset1.pdf"><div class="resource-type-thumbnail pdf"></div></a>
<a class="resource-list-title" href="/courses/` + slug + `/resources/pset1/">Problem set 1</a></div>
<h4>Readings</h4>
<div class="resource-item"><a class="resource-list-title" href="/courses/` + slug + `/resources/reading-list/">Reading list</a></div>
<h4>Exams</h4>
<div class="resource-item"><a class="resource-list-title" href="/courses/` + slug + `/resources/exam1/">Exam 1</a></div>
</body></html>`

const calendar = `<html><body><table>
<thead><tr><th>SES #</th><th>DATE</th><th>TOPICS</th><th>READINGS</th></tr></thead>
<tbody>
<tr><td>1</td><td>Wed Feb 3</td><td>The geometry of linear equations</td><td>1.1-1.3</td></tr>
<tr><td>2</td><td>Feb 5</td><td><ul><li>Elimination with matrices</li><li>Back substitution</li></ul></td><td>2.2 and Chapter 3</td></tr>
<tr><td>R1</td><td>2/8</td><td>Recitation</td><td></td></tr>
</tbody></table></body></html>`

const assignments = `<html><body><table>
<tr><th>SES #</th><th>ASSIGNMENTS</th></tr>
<tr><td>2</td><td><a href="../../resources/pset1/">Problem set 1</a></td></tr>
<tr><td>9</td><td><a href="../../resources/exam1/">Exam 1</a></td></tr>
</table></body></html>`

const syllabus = `<html><body><p>Textbook: Strang, Gilbert. Introduction to Linear Algebra. 4th ed. Wellesley-Cambridge Press, 2009. ISBN: 9780980232714.</p></body></html>`

// pkg builds one course package in memory, with the files under a top directory, as a zip of OCW has them.
func pkg(t *testing.T, top string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range slices.Sorted(func(yield func(string) bool) {
		for k := range files {
			if !yield(k) {
				return
			}
		}
	}) {
		w, err := zw.Create(top + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(files[name])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func linear(t *testing.T) []byte {
	return pkg(t, "18.06-spring-2010", map[string]string{
		"index.html":                    info("18.06", "Spring 2010", "18", "Mathematics", "Mathematics"),
		"download/index.html":           download,
		"pages/calendar/index.html":     calendar,
		"pages/assignments/index.html":  assignments,
		"pages/syllabus/index.html":     syllabus,
		"resources/exam1/data.json":     `{"title": "Exam 1", "learning_resource_types": ["Exams"]}`,
		"static_resources/pset1.pdf":    "%PDF-1.4 the text of the problems",
		"resources/pset1/index.html":    "<html><body>Problem 1. Solve x + y = 2.</body></html>",
		"pages/syllabus/notes.txt":      "not a page",
		"video_galleries/index.html":    "<html></html>",
		"resources/pset1/pset1.pdf":     "%PDF-1.4",
		"resources/lecture-1/data.json": `{"title": "Lecture 1"}`,
	})
}

func physics(t *testing.T) []byte {
	return pkg(t, "8.01-fall-2016", map[string]string{
		"index.html": strings.ReplaceAll(info("8.01", "Fall 2016", "8", "Physics", "Science"), slug, "8-01-classical-mechanics-fall-2016"),
	})
}

func get(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return resp.Body, nil
}

// snapshot writes the packages to a directory and runs Fetch on it.
func snapshot(t *testing.T, zips map[string][]byte) []byte {
	t.Helper()
	dir := t.TempDir()
	for name, data := range zips {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: dir, Cache: t.TempDir(), Get: get}, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func members(t *testing.T, snap []byte) []string {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(snap))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	var out []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, h.Name)
	}
}

func uncompressed(t *testing.T, snap []byte) io.Reader {
	t.Helper()
	r, err := ingest.Uncompressed(bytes.NewReader(snap))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestFetchKeepsPagesOfMathCourses(t *testing.T) {
	zips := map[string][]byte{"18.06-spring-2010.zip": linear(t), "8.01-fall-2016.zip": physics(t)}
	snap := snapshot(t, zips)
	got := members(t, snap)
	want := []string{
		slug + "/download/index.html",
		slug + "/index.html",
		slug + "/pages/assignments/index.html",
		slug + "/pages/calendar/index.html",
		slug + "/pages/syllabus/index.html",
		slug + "/resources/exam1/data.json",
		slug + "/resources/lecture-1/data.json",
		slug + "/video_galleries/index.html",
	}
	if !slices.Equal(got, want) {
		t.Errorf("the members are\n%v\nwant\n%v", got, want)
	}
	if again := snapshot(t, zips); !bytes.Equal(snap, again) {
		t.Error("two snapshots of the same packages have different bytes")
	}
}

func TestFetchCoursePage(t *testing.T) {
	data := linear(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/courses/"+slug+"/download/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<html><body><a href="/courses/`+slug+`/18.06-spring-2010.zip">Download course</a></body></html>`)
	})
	mux.HandleFunc("/courses/"+slug+"/18.06-spring-2010.zip", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(data) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	list := filepath.Join(t.TempDir(), "courses.txt")
	if err := os.WriteFile(list, []byte("# one course\n"+srv.URL+"/courses/"+slug+"/18.06-spring-2010.zip\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{list, srv.URL + "/courses/" + slug + "/18.06-spring-2010.zip"} {
		var buf bytes.Buffer
		if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: u, Cache: t.TempDir(), Get: get}, &buf); err != nil {
			t.Fatalf("%s: %v", u, err)
		}
		if got := members(t, buf.Bytes()); len(got) != 8 {
			t.Errorf("%s: %d members, want 8", u, len(got))
		}
	}
	href, err := packageURL(context.Background(), &ingest.Request{Get: get}, srv.URL+"/courses/"+slug+"/")
	if err != nil {
		t.Fatal(err)
	}
	if href != Base+"/courses/"+slug+"/18.06-spring-2010.zip" {
		t.Errorf("the package of the page is %s", href)
	}
}

func records(t *testing.T) []cluster.Record {
	t.Helper()
	snap := snapshot(t, map[string][]byte{"a.zip": linear(t), "b.zip": physics(t)})
	var out []cluster.Record
	if err := (Adapter{}).Records(uncompressed(t, snap), func(r cluster.Record) error {
		if why := cluster.Check(&r); why != "" {
			t.Errorf("%s: %s", r.NativeID, why)
		}
		out = append(out, r)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRecordsMapThreeLevels(t *testing.T) {
	rs := records(t)
	if len(rs) != 4 {
		t.Fatalf("%d records, want the site and three materials: %+v", len(rs), rs)
	}
	site := rs[0]
	if site.NativeID != slug || site.Kind != "course" || site.Level != "manifestation" || site.Title != "Linear Algebra" || site.Edition != "Spring 2010" || site.Date != "2010" {
		t.Errorf("the site record is %+v", site)
	}
	if got := site.IDs.Get("course"); got != "042nb2s44:18.06" {
		t.Errorf("the course id is %q", got)
	}
	if got := site.IDs.Get("offering"); got != "042nb2s44:18.06:2010-spring" {
		t.Errorf("the offering id is %q", got)
	}
	if got := site.IDs.Get("ocw"); got != slug {
		t.Errorf("the ocw id is %q", got)
	}
	if len(site.Creators) != 1 || site.Creators[0].Name != "Gilbert Strang" || site.Creators[0].Role != "instructor" {
		t.Errorf("the instructors are %+v", site.Creators)
	}
	var titles []string
	for _, r := range rs[1:] {
		titles = append(titles, r.Title)
		if r.Kind != "course-material" || len(r.Relations) != 1 || r.Relations[0] != (resource.Relation{Kind: "part_of", To: "offering:042nb2s44:18.06:2010-spring", Source: "ocw"}) {
			t.Errorf("the material record is %+v", r)
		}
		if !strings.HasPrefix(r.IDs.Get("ocw"), slug+"/resources/") {
			t.Errorf("the material has the ocw id %q", r.IDs.Get("ocw"))
		}
	}
	want := []string{"Lecture 1: The geometry of linear equations", "Problem set 1", "Exam 1"}
	if !slices.Equal(titles, want) {
		t.Errorf("the materials are %q, want %q. A reading list is not a material", titles, want)
	}
}

func TestReadKeepsNoText(t *testing.T) {
	snap := snapshot(t, map[string][]byte{"a.zip": linear(t)})
	var n int
	err := (Adapter{}).Read(uncompressed(t, snap), func(r ingest.Record) error {
		n++
		if r.Text != "" {
			t.Errorf("%s has text", r.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("%d ingest records, want 4", n)
	}
}

func TestStructures(t *testing.T) {
	snap := snapshot(t, map[string][]byte{"a.zip": linear(t)})
	var ids []string
	var nodes []resource.Node
	err := (Adapter{}).Structures(uncompressed(t, snap), func(id string, ns []resource.Node) error {
		ids = append(ids, id)
		nodes = ns
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids, []string{"offering:042nb2s44:18.06:2010-spring"}) {
		t.Fatalf("the structures are of %v", ids)
	}
	var keys []string
	for _, n := range nodes {
		keys = append(keys, n.Key)
	}
	if !slices.Equal(keys, []string{"L01", "L02", "R01"}) {
		t.Fatalf("the sessions are %v", keys)
	}
	l1, l2, r1 := nodes[0], nodes[1], nodes[2]
	if l1.Kind != "session" || l1.Number != "1" || l1.Title != "The geometry of linear equations" || l1.Date != "2010-02-03" {
		t.Errorf("L01 is %+v", l1)
	}
	if !slices.Equal(l1.Materials, []string{"ocw:" + slug + "/resources/lecture-1-the-geometry-of-linear-equations"}) {
		t.Errorf("the materials of L01 are %v", l1.Materials)
	}
	var at []string
	for _, r := range l1.Readings {
		if r.Resource != "isbn:9780980232714" {
			t.Errorf("a reading of L01 names %q", r.Resource)
		}
		at = append(at, r.At["section"])
	}
	if !slices.Equal(at, []string{"1.1", "1.2", "1.3"}) {
		t.Errorf("the sections of L01 are %v", at)
	}
	if l2.Title != "Elimination with matrices; Back substitution" || len(l2.Children) != 2 || l2.Children[1].Kind != "topic" || l2.Children[1].Title != "Back substitution" {
		t.Errorf("L02 is %+v", l2)
	}
	if len(l2.Readings) != 2 || l2.Readings[0].At["section"] != "2.2" || l2.Readings[1].At["chapter"] != "3" {
		t.Errorf("the readings of L02 are %+v", l2.Readings)
	}
	if !slices.Equal(l2.Materials, []string{"ocw:" + slug + "/resources/pset1"}) {
		t.Errorf("the materials of L02 are %v. The assignments page adds them", l2.Materials)
	}
	if r1.Number != "1" || r1.Date != "2010-02-08" || len(r1.Readings) != 0 {
		t.Errorf("R01 is %+v", r1)
	}
}

func TestSessionsFromMaterials(t *testing.T) {
	c := Parse(map[string][]byte{"index.html": []byte(info("18.06", "Spring 2010", "18", "Mathematics", "Mathematics")), "download/index.html": []byte(download)})
	ns := c.Sessions()
	if len(ns) != 1 || ns[0].Key != "L01" || ns[0].Title != "The geometry of linear equations" || len(ns[0].Materials) != 1 {
		t.Errorf("the sessions are %+v", ns)
	}
}

func TestReadingsWithNoTextbook(t *testing.T) {
	c := &Course{}
	got := c.readings("Hoffman and Kunze, sections 1.2 to 1.4")
	if len(got) != 1 || got[0].Resource != "" || got[0].Text != "Hoffman and Kunze, sections 1.2 to 1.4" {
		t.Errorf("the readings are %+v", got)
	}
}

func TestKeyAndTerm(t *testing.T) {
	for label, want := range map[string]string{"1": "L01", "L12": "L12", "Lec 3": "L03", "R2": "R02", "Ex 1": "EX1"} {
		if got, _ := keyOf(label); got != want {
			t.Errorf("keyOf(%q) = %q, want %q", label, got, want)
		}
	}
	for s, want := range map[string]string{"Spring 2010": "2010-spring", "Fall 2011": "2011-fall", "January IAP 2006": "2006-iap"} {
		if got, _ := term(s); got != want {
			t.Errorf("term(%q) = %q, want %q", s, got, want)
		}
	}
}

func TestMath(t *testing.T) {
	for _, c := range []struct {
		c    Course
		want bool
	}{
		{Course{Departments: []string{"18"}}, true},
		{Course{Topics: []string{"Science > Physics", "Mathematics > Probability and Statistics"}}, true},
		{Course{Number: "18.S096"}, true},
		{Course{Number: "6.006", Departments: []string{"6"}, Topics: []string{"Engineering > Computer Science"}}, false},
	} {
		if got := Math(&c.c); got != c.want {
			t.Errorf("Math(%+v) = %v", c.c, got)
		}
	}
}
