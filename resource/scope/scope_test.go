package scope

import (
	"testing"

	"github.com/tamnd/math2-agent/resource"
)

var env = Env{
	Venues:         map[string]string{"0001-8708": VenueCore, "0028-0836": VenueMixed, "1234-5679": VenueBorder},
	MathItems:      map[string]bool{"Q395": true},
	TopicThreshold: 0.5,
}

func TestTests(t *testing.T) {
	cases := []struct {
		name string
		test string
		rec  Record
		want bool
	}{
		{"msc analysis", "S1", Record{MSC: []string{"26A42"}}, true},
		{"msc 00", "S1", Record{MSC: []string{"00A05"}}, true},
		{"msc 58", "S1", Record{MSC: []string{"58J20"}}, true},
		{"msc probability", "S1", Record{MSC: []string{"60G05"}}, true},
		{"msc numerical", "S1", Record{MSC: []string{"65F10"}}, true},
		{"msc education", "S1", Record{MSC: []string{"97D40"}}, true},
		{"msc game theory", "S1", Record{MSC: []string{"91A10"}}, true},
		{"msc economics", "S1", Record{MSC: []string{"91B10"}}, false},
		{"msc statistics", "S1", Record{MSC: []string{"62F10"}}, false},
		{"msc mechanics", "S1", Record{MSC: []string{"70H05"}}, false},
		{"msc 68Q with a math code", "S1", Record{MSC: []string{"68Q25", "05C85"}}, true},
		{"msc 68Q alone", "S1", Record{MSC: []string{"68Q25"}}, false},
		{"msc 68N with a math code", "S1", Record{MSC: []string{"68N15", "03B70"}}, false},
		{"msc secondary only", "S1", Record{MSC: []string{"81T13", "53C07"}}, false},
		{"msc lower case", "S1", Record{MSC: []string{"11a41"}}, true},
		{"zbmath record", "S1", Record{ZBMath: true}, true},
		{"no msc", "S1", Record{}, false},

		{"arxiv math", "S2", Record{Arxiv: []string{"math.AG"}}, true},
		{"arxiv math-ph", "S2", Record{Arxiv: []string{"math-ph"}}, true},
		{"arxiv cs.LO with math", "S2", Record{Arxiv: []string{"cs.LO", "math.LO"}}, true},
		{"arxiv stat.TH with math", "S2", Record{Arxiv: []string{"stat.TH", "math.ST"}}, true},
		{"arxiv cs.LO alone", "S2", Record{Arxiv: []string{"cs.LO"}}, false},
		{"arxiv cs.LG with math", "S2", Record{Arxiv: []string{"cs.LG", "math.OC"}}, false},
		{"arxiv hep-th with math", "S2", Record{Arxiv: []string{"hep-th", "math.QA"}}, false},

		{"lcc QA300", "S3", Record{LCC: []string{"QA300"}}, true},
		{"lcc QA1", "S3", Record{LCC: []string{"QA1"}}, true},
		{"lcc QA939", "S3", Record{LCC: []string{"QA939 .B6"}}, true},
		{"lcc with space", "S3", Record{LCC: []string{"QA 9.58"}}, true},
		{"lcc QA75.4", "S3", Record{LCC: []string{"QA75.4"}}, true},
		{"lcc QA75.5", "S3", Record{LCC: []string{"QA75.5"}}, false},
		{"lcc QA76.73", "S3", Record{LCC: []string{"QA76.73.P98"}}, false},
		{"lcc QA76.95", "S3", Record{LCC: []string{"QA76.95"}}, false},
		{"lcc QA76.96", "S3", Record{LCC: []string{"QA76.96"}}, true},
		{"lcc QB", "S3", Record{LCC: []string{"QB500"}}, false},
		{"lcc Q", "S3", Record{LCC: []string{"Q300"}}, false},
		{"ddc 515.7", "S3", Record{DDC: []string{"515.7"}}, true},
		{"ddc 519", "S3", Record{DDC: []string{"519.2"}}, true},
		{"ddc 520", "S3", Record{DDC: []string{"520"}}, false},
		{"ddc 51", "S3", Record{DDC: []string{"51"}}, false},
		{"udc 517.9", "S3", Record{UDC: []string{"517.9"}}, true},
		{"udc 51-7", "S3", Record{UDC: []string{"51-7"}}, true},
		{"udc 52", "S3", Record{UDC: []string{"52"}}, false},
		{"clc O174", "S3", Record{CLC: []string{"O174"}}, true},
		{"clc O4", "S3", Record{CLC: []string{"O4"}}, false},
		{"bk 31.44", "S3", Record{BK: []string{"31.44"}}, true},
		{"bk 33.10", "S3", Record{BK: []string{"33.10"}}, false},
		{"bk 310", "S3", Record{BK: []string{"310"}}, false},

		{"venue core", "S4", Record{Venues: []string{"0001-8708"}}, true},
		{"venue mixed", "S4", Record{Venues: []string{"0028-0836"}}, false},
		{"venue not in the register", "S4", Record{Venues: []string{"9999-9999"}}, false},

		{"curriculum", "S5", Record{Curriculum: true}, true},
		{"no curriculum", "S5", Record{}, false},
		{"math unit", "S6", Record{MathUnit: true}, true},
		{"no math unit", "S6", Record{}, false},

		{"main subject math", "S7", Record{MainSubjects: []string{"Q11", "Q395"}}, true},
		{"main subject other", "S7", Record{MainSubjects: []string{"Q11"}}, false},
		{"topic above", "S7", Record{Topics: []Topic{{MathField, 0.7}}}, true},
		{"topic at", "S7", Record{Topics: []Topic{{MathField, 0.5}}}, true},
		{"topic below", "S7", Record{Topics: []Topic{{MathField, 0.4}}}, false},
		{"topic other field", "S7", Record{Topics: []Topic{{"Physics and Astronomy", 0.9}}}, false},
	}
	run := map[string]func(Record, Env) bool{}
	for _, tt := range Tests {
		run[tt.Name] = tt.Run
	}
	for _, c := range cases {
		t.Run(c.test+"/"+c.name, func(t *testing.T) {
			if got := run[c.test](c.rec, env); got != c.want {
				t.Errorf("%s(%+v) = %v, want %v", c.test, c.rec, got, c.want)
			}
		})
	}
}

func TestDecide(t *testing.T) {
	cases := []struct {
		name string
		rec  Record
		want Decision
	}{
		{"first test wins", Record{MSC: []string{"11A41"}, Arxiv: []string{"math.NT"}}, Decision{resource.In, "S1"}},
		{"arxiv after msc", Record{MSC: []string{"81T13"}, Arxiv: []string{"math.QA"}}, Decision{resource.In, "S2"}},
		{"subject", Record{Topics: []Topic{{MathField, 0.8}}}, Decision{resource.In, "S7"}},
		{"border msc", Record{MSC: []string{"62F10"}}, Decision{resource.Border, BorderMSC}},
		{"border msc 68", Record{MSC: []string{"68T05"}}, Decision{resource.Border, BorderMSC}},
		{"border msc 94", Record{MSC: []string{"94A17"}}, Decision{resource.Border, BorderMSC}},
		{"border stat", Record{Arxiv: []string{"stat.ME"}}, Decision{resource.Border, BorderArxiv}},
		{"border cs with math", Record{Arxiv: []string{"cs.LG", "math.OC"}}, Decision{resource.Border, BorderArxiv}},
		{"border physics with math-ph", Record{Arxiv: []string{"hep-th", "math-ph"}}, Decision{resource.Border, BorderArxiv}},
		{"border lcc computing", Record{LCC: []string{"QA76.9.A43"}}, Decision{resource.Border, BorderClass}},
		{"border lcc QC with heading", Record{LCC: []string{"QC174.12"}, MathHeading: true}, Decision{resource.Border, BorderClass}},
		{"lcc QC with no heading", Record{LCC: []string{"QC174.12"}}, Decision{resource.Out, ""}},
		{"border venue mixed", Record{Venues: []string{"0028-0836"}}, Decision{resource.Border, BorderVenue}},
		{"border venue border", Record{Venues: []string{"1234-5679"}}, Decision{resource.Border, BorderVenue}},
		{"out msc", Record{MSC: []string{"95A10"}}, Decision{resource.Out, ""}},
		{"out cs alone", Record{Arxiv: []string{"cs.CV"}}, Decision{resource.Out, ""}},
		{"out ddc", Record{DDC: []string{"530.1"}}, Decision{resource.Out, ""}},
		{"deferred", Record{}, Decision{resource.Deferred, ""}},
		{"deferred with a venue not in the register", Record{Venues: []string{"PCP2HG7X5G"}}, Decision{resource.Deferred, ""}},
		{"deferred with flags only", Record{MathHeading: true}, Decision{resource.Deferred, ""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Decide(c.rec, env); got != c.want {
				t.Errorf("Decide(%+v) = %+v, want %+v", c.rec, got, c.want)
			}
		})
	}
}

func TestFromResource(t *testing.T) {
	r := &resource.Resource{
		MSC:       []resource.Code{{Code: "26-01", From: "zbmath"}},
		Subjects:  []resource.Subject{{System: "lcc", Code: "QA300"}, {System: "arXiv", Code: "math.CA"}, {System: "ddc", Code: "515"}, {System: "wikidata", Code: "Q395"}},
		IDs:       resource.IDs{"zbl": {"0346.26002"}},
		Relations: []resource.Relation{{Kind: "part_of", To: "8F2K1QW7ZJ/e1"}, {Kind: "translation_of", To: "1A2B3C4D5E"}},
	}
	rec := FromResource(r)
	if len(rec.MSC) != 1 || rec.MSC[0] != "26-01" {
		t.Errorf("MSC = %v", rec.MSC)
	}
	if len(rec.LCC) != 1 || len(rec.Arxiv) != 1 || len(rec.DDC) != 1 || len(rec.MainSubjects) != 1 {
		t.Errorf("subjects = %+v", rec)
	}
	if !rec.ZBMath {
		t.Error("ZBMath = false, want true")
	}
	if len(rec.Venues) != 1 || rec.Venues[0] != "8F2K1QW7ZJ" {
		t.Errorf("Venues = %v", rec.Venues)
	}
	if d := Decide(rec, env); d != (Decision{resource.In, "S1"}) {
		t.Errorf("Decide = %+v", d)
	}
}
