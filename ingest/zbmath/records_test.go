package zbmath

import (
	"os"
	"testing"

	"github.com/tamnd/math2-agent/resource/cluster"
)

func TestRecords(t *testing.T) {
	f, err := os.Open("testdata/page1.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var rs []cluster.Record
	if err := (Adapter{}).Records(f, func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rs) == 0 {
		t.Fatal("no records")
	}
	r := rs[0]
	if r.NativeID != "8270026" || r.Kind != "article" || r.Date != "2026" || r.Language != "en" || r.Title != "Genuinely non-traditional logics" {
		t.Errorf("record = %+v", r)
	}
	if got := r.IDs["zbl_id"]; len(got) != 1 || got[0] != "8270026" {
		t.Errorf("zbl_id = %v", got)
	}
	if got := r.IDs["doi"]; len(got) != 1 || got[0] != "10.1007/s11787-026-00426-5" {
		t.Errorf("doi = %v", got)
	}
	if _, ok := r.IDs["zbl"]; ok {
		t.Errorf("a Zbl number of None gives an id: %v", r.IDs)
	}
	if len(r.Creators) != 2 || r.Creators[1].Name != "Estrada-González, Luis" || r.Creators[1].IDs["zbmath_author"][0] != "estrada-gonzalez.luis" {
		t.Errorf("creators = %+v", r.Creators)
	}
	if len(r.MSC) != 4 || r.MSC[0] != "03A05" {
		t.Errorf("msc = %v", r.MSC)
	}
}
