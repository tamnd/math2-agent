package resource

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func fixture(t *testing.T) Resource {
	t.Helper()
	data, err := os.ReadFile("testdata/rudin.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestValuesJSON(t *testing.T) {
	ids := IDs{"doi": {"10.1/x"}, "isbn": {"9780000000002", "9780000000019"}}
	data, err := json.Marshal(ids)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"doi":"10.1/x","isbn":["9780000000002","9780000000019"]}`; string(data) != want {
		t.Fatalf("got %s, want %s", data, want)
	}
	var back IDs
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, ids) || back.Get("doi") != "10.1/x" || back.Get("arxiv") != "" {
		t.Fatalf("got %v", back)
	}
	if err := json.Unmarshal([]byte(`{"doi":3}`), &back); err == nil {
		t.Fatal("a number is not an id")
	}
}

func TestParseRejectsUnknownField(t *testing.T) {
	if _, err := Parse([]byte(`{"id":"8F2K1QW7ZB","tier":"T1"}`)); err == nil {
		t.Fatal("the field tier is computed and is not in the record")
	}
}

func TestWriteRead(t *testing.T) {
	root := t.TempDir()
	a := fixture(t)
	b := fixture(t)
	b.ID = "8F2AAAAAAA"
	c := fixture(t)
	c.ID = "9XYAAAAAAA"
	if err := WriteResources(root, []Resource{c, a, b}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadResources(root)
	if err != nil {
		t.Fatal(err)
	}
	if ids := []string{got[0].ID, got[1].ID, got[2].ID}; !slices.Equal(ids, []string{"8F2AAAAAAA", "8F2K1QW7ZB", "9XYAAAAAAA"}) {
		t.Fatalf("got the order %v", ids)
	}
	if !reflect.DeepEqual(got[1], a) {
		t.Fatalf("the round trip changed the record:\n%+v\n%+v", got[1], a)
	}
	first, _ := os.ReadFile(filepath.Join(root, Dir, "8F2.jsonl"))
	if err := WriteResources(root, []Resource{a, b}); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(filepath.Join(root, Dir, "8F2.jsonl"))
	if string(first) != string(again) {
		t.Fatal("two writes of the same resources give different bytes")
	}
	if _, err := os.Stat(filepath.Join(root, Dir, "9XY.jsonl")); !os.IsNotExist(err) {
		t.Fatal("the shard with no resource is still there")
	}
	if err := WriteResources(root, []Resource{a, a}); err == nil {
		t.Fatal("two resources with one id must fail")
	}
}

func TestWalkGivesFileAndLine(t *testing.T) {
	root := t.TempDir()
	a, b := fixture(t), fixture(t)
	b.ID = "8F2ZZZZZZZ"
	if err := WriteResources(root, []Resource{a, b}); err != nil {
		t.Fatal(err)
	}
	var where []string
	err := WalkResources(root, func(file string, line int, r Resource) error {
		where = append(where, file+":"+r.ID+":"+string(rune('0'+line)))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"records/resources/8F2.jsonl:8F2K1QW7ZB:1", "records/resources/8F2.jsonl:8F2ZZZZZZZ:2"}; !slices.Equal(where, want) {
		t.Fatalf("got %v", where)
	}
}

func TestStructureAndCites(t *testing.T) {
	root := t.TempDir()
	s := &Structure{Resource: "8F2K1QW7ZB/e3", Source: "publisher-toc", Snapshot: "blake3:00", Nodes: []Node{
		{Key: "1", Kind: "chapter", Number: "1", Title: "The Real and Complex Number Systems", Children: []Node{{Key: "1.1", Kind: "section", Title: "Introduction"}}},
	}}
	if err := WriteStructure(root, s); err != nil {
		t.Fatal(err)
	}
	got, err := ReadStructure(root, "8F2K1QW7ZB", "e3")
	if err != nil {
		t.Fatal(err)
	}
	if got.Find("1.1") == nil || got.Find("2") != nil {
		t.Fatal("Find does not walk the tree")
	}
	if none, err := ReadStructure(root, "8F2K1QW7ZB", "e9"); none != nil || err != nil {
		t.Fatal("an expression with no file has no structure and no error")
	}
	if err := WriteCites(root, []Cite{{From: "8F2K1QW7ZB/e3", N: 2}, {From: "8F2K1QW7ZB/e3", N: 1, To: "9XYAAAAAAA"}}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, CitesDir, "8F2.jsonl"))
	if !strings.HasPrefix(string(data), `{"from":"8F2K1QW7ZB/e3","n":1,`) {
		t.Fatalf("the cites are not sorted: %s", data)
	}
	rs, err := ReadResources(root)
	if err != nil || len(rs) != 0 {
		t.Fatal("ReadResources reads the files of cites/ and structure/")
	}
}

func TestRegister(t *testing.T) {
	root := t.TempDir()
	if err := AppendRegister(root, []Entry{{"8F2K1QW7ZB", "abc123", "textbook"}, {"8F2AAAAAAA", "abc123", "article"}}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, RegisterDir, "8F2.reg"))
	if want := "# id commit kind\n8F2AAAAAAA abc123 article\n8F2K1QW7ZB abc123 textbook\n"; string(data) != want {
		t.Fatalf("got %q", data)
	}
	if err := AppendRegister(root, []Entry{{"8F2AAAAAAA", "def456", "article"}}); err == nil {
		t.Fatal("an id that the register has must fail")
	}
	if err := AppendRegister(root, []Entry{{"9XYAAAAAAA", "", "article"}}); err == nil {
		t.Fatal("an entry with no commit must fail")
	}
	ls, err := ReadRegister(root)
	if err != nil || len(ls) != 2 || ls[1].Line != 3 || ls[1].File != "registers/resources/8F2.reg" {
		t.Fatalf("got %+v, %v", ls, err)
	}
}

func TestCheck(t *testing.T) {
	r := fixture(t)
	if ps := Check(&r); len(ps) != 0 {
		t.Fatalf("the fixture has problems: %v", ps)
	}
	cases := []struct {
		field string
		edit  func(*Resource)
	}{
		{"kind", func(r *Resource) { r.Kind = "book" }},
		{"title", func(r *Resource) { r.Title = " " }},
		{"date", func(r *Resource) { r.Date = "1953?" }},
		{"level", func(r *Resource) { r.Level = "doctoral" }},
		{"genre[0]", func(r *Resource) { r.Genre = []string{"fiction"} }},
		{"creators[0].role", func(r *Resource) { r.Creators[0].Role = "writer" }},
		{"msc[0].code", func(r *Resource) { r.MSC[0].Code = "26" + "A" }},
		{"expressions[0].key", func(r *Resource) { r.Expressions[0].Key = "m1" }},
		{"expressions[0].manifestations[0].access.class", func(r *Resource) { r.Expressions[0].Manifestations[0].Access.Class = "free" }},
		{"expressions[0].manifestations[0].access", func(r *Resource) {
			m := &r.Expressions[0].Manifestations[0]
			m.Access, m.Links = nil, []string{"https://example.org/"}
		}},
		{"expressions[0].manifestations[0].access.licence_read", func(r *Resource) { r.Expressions[0].Manifestations[0].Access.Class = "open" }},
		{"relations[0].from", func(r *Resource) { r.Relations[0].From = "e3/m9" }},
		{"scope.target", func(r *Resource) { r.Scope = &Scope{Value: Duplicate, By: "x"} }},
		{"scope.until", func(r *Resource) { r.Scope = &Scope{Value: Deferred, Reason: "no class", By: "x"} }},
		{"catalogue", func(r *Resource) { r.Catalogue = nil }},
		{"history", func(r *Resource) { r.History = nil }},
	}
	for _, c := range cases {
		r := fixture(t)
		c.edit(&r)
		ps := Check(&r)
		if !slices.ContainsFunc(ps, func(p Problem) bool { return p.Field == c.field }) {
			t.Errorf("%s: got %v", c.field, ps)
		}
	}
}

func TestSchemaAcceptsFixture(t *testing.T) {
	data, _ := os.ReadFile("testdata/rudin.json")
	if err := Validate(ResourceSchema, data); err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(string(data), `"kind":"textbook"`, `"kind":"book"`, 1)
	if err := Validate(ResourceSchema, []byte(bad)); err == nil {
		t.Fatal("the schema accepts the kind book")
	}
	if err := Validate(CiteSchema, []byte(`{"from":"8F2K1QW7ZB/e3","n":1,"to":"9XYAAAAAAA"}`)); err != nil {
		t.Fatal(err)
	}
	if err := Validate(StructureSchema, []byte(`{"resource":"8F2K1QW7ZB/e3","source":"x","snapshot":"blake3:00","nodes":[{"key":"1","kind":"chapter"}]}`)); err != nil {
		t.Fatal(err)
	}
}

// TestSchemaMatchesTypes checks that each object of the schemas has the properties of its Go type, and no other.
func TestSchemaMatchesTypes(t *testing.T) {
	type pair struct {
		schema, def string
		typ         any
	}
	pairs := []pair{
		{ResourceSchema, "", Resource{}}, {ResourceSchema, "creator", Creator{}}, {ResourceSchema, "code", Code{}},
		{ResourceSchema, "subject", Subject{}}, {ResourceSchema, "licence_read", LicenceRead{}}, {ResourceSchema, "public_domain", PublicDomain{}},
		{ResourceSchema, "access", Access{}}, {ResourceSchema, "manifestation", Manifestation{}}, {ResourceSchema, "expression", Expression{}},
		{ResourceSchema, "relation", Relation{}}, {ResourceSchema, "scope", Scope{}}, {ResourceSchema, "catalogue_record", CatalogueRecord{}},
		{ResourceSchema, "event", Event{}}, {CiteSchema, "", Cite{}}, {CiteSchema, "parsed", Parsed{}},
		{StructureSchema, "", Structure{}}, {StructureSchema, "node", Node{}}, {StructureSchema, "reading", Reading{}},
	}
	for _, p := range pairs {
		data, err := RawSchema(p.schema)
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Properties map[string]any `json:"properties"`
			Defs       map[string]struct {
				Properties map[string]any `json:"properties"`
			} `json:"$defs"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		props := doc.Properties
		if p.def != "" {
			props = doc.Defs[p.def].Properties
		}
		var inSchema, inType []string
		for k := range props {
			inSchema = append(inSchema, k)
		}
		rt := reflect.TypeOf(p.typ)
		for i := range rt.NumField() {
			name, _, _ := strings.Cut(rt.Field(i).Tag.Get("json"), ",")
			inType = append(inType, name)
		}
		slices.Sort(inSchema)
		slices.Sort(inType)
		if !slices.Equal(inSchema, inType) {
			t.Errorf("%s %s: the schema has %v, the type %s has %v", p.schema, p.def, inSchema, rt.Name(), inType)
		}
	}
}

func TestSchemaEnumsMatchLists(t *testing.T) {
	data, _ := RawSchema(ResourceSchema)
	var doc struct {
		Properties struct {
			Kind struct {
				Enum []string `json:"enum"`
			} `json:"kind"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(doc.Properties.Kind.Enum, Kinds) {
		t.Fatalf("the kinds of the schema and of Kinds differ")
	}
}
