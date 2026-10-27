package inventory_test

import (
	"slices"
	"testing"

	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/afp"
	"github.com/tamnd/math2-agent/ingest/arxiv"
	"github.com/tamnd/math2-agent/ingest/arxivresults"
	"github.com/tamnd/math2-agent/ingest/curricula"
	"github.com/tamnd/math2-agent/ingest/erdosproblems"
	"github.com/tamnd/math2-agent/ingest/formalconjectures"
	"github.com/tamnd/math2-agent/ingest/lmfdb"
	"github.com/tamnd/math2-agent/ingest/mathcomp"
	"github.com/tamnd/math2-agent/ingest/mathlib"
	"github.com/tamnd/math2-agent/ingest/oeis"
	"github.com/tamnd/math2-agent/ingest/palomar"
	"github.com/tamnd/math2-agent/ingest/setmm"
	"github.com/tamnd/math2-agent/ingest/zbmath"
	"github.com/tamnd/math2-agent/inventory"
)

// The inventory names sources by their ids. The ids must be the ids of the adapters, or the layer, the kind and the system of a mention are wrong.
func TestSourceIDs(t *testing.T) {
	for _, a := range []ingest.Adapter{mathlib.Adapter{}, formalconjectures.Adapter{}, afp.Adapter{}, setmm.Adapter{}, mathcomp.Adapter{}} {
		if id := a.Source().ID; !inventory.IsFormal(id) {
			t.Errorf("the source %s is not formal", id)
		}
	}
	for _, a := range []ingest.Adapter{arxiv.Adapter{}, arxivresults.Adapter{}, zbmath.Adapter{}, palomar.Adapter{}} {
		if id := a.Source().ID; !inventory.IsResearch(id) {
			t.Errorf("the source %s is not research", id)
		}
	}
	kinds := []struct {
		a    ingest.Adapter
		kind string
	}{
		{erdosproblems.Adapter{}, "problem"},
		{formalconjectures.Adapter{}, "conjecture"},
		{oeis.Adapter{}, "object"},
		{lmfdb.Adapter{}, "object"},
		{curricula.Thousand{}, "statement"},
	}
	for _, l := range curricula.Lists {
		want := inventory.Unknown
		if l.Form == curricula.Items {
			want = "statement"
		}
		kinds = append(kinds, struct {
			a    ingest.Adapter
			kind string
		}{l, want})
	}
	for _, c := range kinds {
		src := c.a.Source()
		m := inventory.FromRecords(&src, "blake3:00", "test", []ingest.Record{{ID: "1", Title: "x", Class: src.Class}})[0]
		if m.Kind != c.kind {
			t.Errorf("a mention of %s has the kind %s, want %s", src.ID, m.Kind, c.kind)
		}
	}
	for _, a := range []ingest.Adapter{erdosproblems.Adapter{}, formalconjectures.Adapter{}} {
		if sys := inventory.SystemOf(a.Source().ID); !crosswalk.Known(sys) {
			t.Errorf("the source %s has the system %s, which is not in the crosswalk", a.Source().ID, sys)
		}
	}
}

func TestFromRecordsIDs(t *testing.T) {
	src := ingest.Source{ID: "arxiv", Licence: "CC0-1.0", Class: ingest.Open}
	m := inventory.FromRecords(&src, "blake3:00", "test", []ingest.Record{{ID: "2401.00001", Class: ingest.Open, Meta: map[string]string{"msc": "11A41 11N05", "mathlib": "Nat.Prime|Nat.Coprime"}}})[0]
	if !slices.Equal(m.MSC, []string{"11A41", "11N05"}) {
		t.Errorf("the MSC codes are %q", m.MSC)
	}
	if !slices.Equal(m.Same["mathlib"], []string{"Nat.Prime", "Nat.Coprime"}) {
		t.Errorf("the ids are %v", m.Same)
	}
}
