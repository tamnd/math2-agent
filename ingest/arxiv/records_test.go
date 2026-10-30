package arxiv

import (
	"os"
	"testing"

	"github.com/tamnd/math2-agent/resource/cluster"
)

func TestRecords(t *testing.T) {
	f, err := os.Open("testdata/0211.jsonl")
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
	if r.NativeID != "math/0211159" || r.Kind != "preprint" || r.Level != "work" || r.Date != "2002-11-11" {
		t.Errorf("record = %+v", r)
	}
	if got := r.IDs["arxiv"]; len(got) != 1 || got[0] != "math/0211159" {
		t.Errorf("arxiv id = %v", got)
	}
	if len(r.Creators) != 1 || r.Creators[0].Name != "Grisha Perelman" || r.Creators[0].Role != "author" {
		t.Errorf("creators = %v", r.Creators)
	}
	if len(r.Subjects) != 1 || r.Subjects[0].Code != "math.DG" {
		t.Errorf("subjects = %v", r.Subjects)
	}
}

func TestDate(t *testing.T) {
	for id, want := range map[string]string{"2312.01234": "2023-12", "0704.0001": "2007-04", "math/9901001": "1999-01", "math/0211159": "2002-11", "x": "", "2313.00001": ""} {
		if got := Date(id); got != want {
			t.Errorf("Date(%q) = %q, want %q", id, got, want)
		}
	}
}
