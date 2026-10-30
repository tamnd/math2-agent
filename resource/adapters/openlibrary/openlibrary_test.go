package openlibrary

import (
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource/cluster"
)

const dump = "/type/work\t/works/OL1W\t3\t2020-01-01\t" + `{"key":"/works/OL1W","title":"Calculus","subjects":["Calculus -- Textbooks"],"first_publish_date":"March 1967"}` + "\n" +
	"/type/work\t/works/OL2W\t1\t2020-01-01\t" + `{"key":"/works/OL2W","title":"Cooking","subjects":["Cookery"]}` + "\n" +
	"/type/work\t/works/OL3W\t1\t2020-01-01\t" + `{"key":"/works/OL3W","title":"Programming","lc_classifications":["QA76.73.P98"]}` + "\n" +
	"/type/author\t/authors/OL1A\t1\t2020-01-01\t" + `{"name":"M. Spivak"}` + "\n" +
	"/type/edition\t/books/OL1M\t2\t2020-01-01\t" + `{"key":"/books/OL1M","title":"Calculus","works":[{"key":"/works/OL1W"}],"isbn_10":["0805390219"],"publishers":["Benjamin"],"publish_date":"1967","languages":[{"key":"/languages/eng"}]}` + "\n" +
	"/type/edition\t/books/OL2M\t1\t2020-01-01\t" + `{"key":"/books/OL2M","title":"Cooking","works":[{"key":"/works/OL2W"}]}` + "\n" +
	"/type/edition\t/books/OL3M\t1\t2020-01-01\t" + `{"key":"/books/OL3M","title":"Algebra","lc_classifications":["QA-0154.00000000.L3"],"dewey_decimal_class":["512"]}` + "\n"

func TestRecords(t *testing.T) {
	var rs []cluster.Record
	if err := (Adapter{}).Records(strings.NewReader(dump), func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, r := range rs {
		keys = append(keys, r.NativeID+"/"+r.Level)
	}
	if strings.Join(keys, " ") != "OL1W/work OL1M/manifestation OL3M/manifestation" {
		t.Fatalf("records %v", keys)
	}
	if w := rs[0]; w.Date != "1967" || w.IDs["openlibrary_work"][0] != "OL1W" {
		t.Errorf("work %+v", w)
	}
	if e := rs[1]; e.IDs["isbn"][0] != "9780805390216" || e.IDs["openlibrary_work"][0] != "OL1W" || e.Language != "en" || e.Publisher != "Benjamin" {
		t.Errorf("edition %+v", e)
	}
	if e := rs[2]; e.Subjects[0].Code != "QA154.L3" || e.Subjects[1].Code != "512" {
		t.Errorf("edition %+v", e)
	}
	var recs []ingest.Record
	if err := (Adapter{}).Read(strings.NewReader(dump), func(r ingest.Record) error { recs = append(recs, r); return nil }); err != nil || len(recs) != 3 {
		t.Errorf("read %+v, %v", recs, err)
	}
	if err := (Adapter{}).Records(strings.NewReader("/type/work\tx\n"), func(cluster.Record) error { return nil }); err == nil {
		t.Error("a short line gives no error")
	}
}

func TestMath(t *testing.T) {
	for _, c := range []struct {
		d    Doc
		want bool
	}{
		{Doc{LCC: []string{"QA43"}}, true},
		{Doc{LCC: []string{"QA76.9.A43"}}, false},
		{Doc{LCC: []string{"QA75.4"}}, true},
		{Doc{LCC: []string{"QB43"}}, false},
		{Doc{DDC: []string{"515.35"}}, true},
		{Doc{DDC: []string{"500"}}, false},
		{Doc{Subjects: []string{"Number theory."}}, true},
		{Doc{Subjects: []string{"History"}}, false},
	} {
		if got := Math(&c.d); got != c.want {
			t.Errorf("Math(%+v) = %v", c.d, got)
		}
	}
}
