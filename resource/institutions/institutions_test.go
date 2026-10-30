package institutions

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"
)

const dump = `[
{"id":"https://ror.org/0aaaaaa01","status":"active","types":["education"],"names":[{"value":"Uni A","types":["label"]},{"value":"University A","types":["ror_display"]}],"locations":[{"geonames_details":{"country_code":"VN"}}],"external_ids":[{"type":"wikidata","all":["Q1"],"preferred":null}],"links":[{"type":"website","value":"https://a.example"}]},
{"id":"https://ror.org/0bbbbbb02","status":"active","types":["company"],"names":[{"value":"Firm B","types":["ror_display"]}]},
{"id":"https://ror.org/0cccccc03","status":"inactive","types":["education"],"names":[{"value":"Old C","types":["ror_display"]}]},
{"id":"https://ror.org/0dddddd04","status":"active","types":["education","funder"],"names":[{"value":"College D","types":["ror_display"]}],"locations":[{"geonames_details":{"country_code":"FR"}}]}
]`

func orgs(t *testing.T, data []byte) []Org {
	t.Helper()
	var out []Org
	if err := ReadROR(data, func(o Org) error { out = append(out, o); return nil }); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestReadROR(t *testing.T) {
	os := orgs(t, []byte(dump))
	if len(os) != 4 {
		t.Fatalf("got %d organizations, want 4", len(os))
	}
	a := os[0]
	if a.ID != "0aaaaaa01" || a.Name != "University A" || a.Country != "VN" || a.Wikidata != "Q1" || a.Website != "https://a.example" {
		t.Errorf("the first organization is %+v", a)
	}
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, name := range []string{"v1.json", "v1.0-ror-data_schema_v2.json"} {
		w, _ := zw.Create(name)
		body := dump
		if name == "v1.json" {
			body = "[]"
		}
		_, _ = w.Write([]byte(body))
	}
	_ = zw.Close()
	if got := orgs(t, b.Bytes()); len(got) != 4 {
		t.Errorf("the zip file gives %d organizations, want 4", len(got))
	}
	if err := ReadROR([]byte(`{}`), func(Org) error { return nil }); err == nil {
		t.Error("an object must be an error")
	}
}

func TestUnits(t *testing.T) {
	answer := `{"results":{"bindings":[
{"unit":{"value":"http://www.wikidata.org/entity/Q20"},"unitLabel":{"value":"Department of Mathematics"},"site":{"value":"https://math.a.example"},"ror":{"value":"0aaaaaa01"}},
{"unit":{"value":"http://www.wikidata.org/entity/Q10"},"unitLabel":{"value":"Institute of Mathematics"},"ror":{"value":"0aaaaaa01"}},
{"unit":{"value":"http://www.wikidata.org/entity/Q10"},"unitLabel":{"value":"Institute of Mathematics"},"ror":{"value":"0aaaaaa01"}}]}}`
	var asked string
	get := func(_ context.Context, u string) ([]byte, error) { asked = u; return []byte(answer), nil }
	us, err := Units(context.Background(), get, Endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(asked, Endpoint+"?") || !strings.Contains(asked, "format=json") || strings.Contains(asked, "mailto") {
		t.Errorf("the request was %s", asked)
	}
	if got := us["0aaaaaa01"]; len(got) != 2 || got[0].Wikidata != "Q10" || got[1].URL != "https://math.a.example" {
		t.Errorf("the units are %+v", got)
	}
}

func TestBuild(t *testing.T) {
	units := map[string][]Unit{"0aaaaaa01": {{Name: "Department of Mathematics", Wikidata: "Q20"}}}
	prev := []Institution{
		{ROR: "0dddddd04", Name: "D", Level: College, Tier: I2, Catalogue: &Catalogue{URL: "https://d.example/courses", Format: "json"}},
		{ROR: "0eeeeee05", Name: "Gone E", Level: University, Tier: I4},
	}
	got, c := Build(prev, orgs(t, []byte(dump)), units)
	if c.Orgs != 2 || c.Added != 1+len(Platforms) || c.Withdrawn != 1 || c.WithUnit != 1 {
		t.Errorf("the counts are %+v", c)
	}
	by := map[string]Institution{}
	for _, in := range got {
		by[in.Key()] = in
		if p := in.Problems(); len(p) > 0 {
			t.Errorf("%s has the problems %v", in.Key(), p)
		}
	}
	if a := by["0aaaaaa01"]; a.Name != "University A" || a.Tier != I4 || a.Level != University || a.MathUnit == nil || a.MathUnit.Wikidata != "Q20" {
		t.Errorf("A is %+v", a)
	}
	if d := by["0dddddd04"]; d.Name != "College D" || d.Tier != I2 || d.Level != College || d.Country != "FR" {
		t.Errorf("D lost the fields of a person: %+v", d)
	}
	if e := by["0eeeeee05"]; e.Status != "withdrawn" {
		t.Errorf("E is %+v", e)
	}
	if _, ok := by["0bbbbbb02"]; ok {
		t.Error("a company is in the register")
	}
	if ocw := by["name:MIT OpenCourseWare"]; ocw.Tier != I1 || ocw.Level != Platform {
		t.Errorf("OCW is %+v", ocw)
	}
}

func TestRegister(t *testing.T) {
	root := t.TempDir()
	if got, err := Read(root); err != nil || got != nil {
		t.Fatalf("an empty tree gives %v, %v", got, err)
	}
	ins, _ := Build(nil, orgs(t, []byte(dump)), nil)
	if err := Write(root, ins); err != nil {
		t.Fatal(err)
	}
	got, err := Read(root)
	if err != nil || len(got) != len(ins) {
		t.Fatalf("read %d records, want %d: %v", len(got), len(ins), err)
	}
	bad := Institution{Name: "X", Level: "school", Tier: I2}
	if p := bad.Problems(); len(p) != 3 {
		t.Errorf("the problems are %v", p)
	}
}
