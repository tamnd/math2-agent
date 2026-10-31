package ingest

import (
	"strings"
	"testing"
)

func TestCadence(t *testing.T) {
	s := Source{ID: "x", Name: "X", Licence: "CC0-1.0", Class: ClassOf("CC0-1.0"), Cadence: "monthly", Snapshots: []Snapshot{
		{URL: "u", Hash: "blake3:" + strings.Repeat("1", 64), Date: "2026-10-01"},
		{URL: "u", Hash: "blake3:" + strings.Repeat("0", 64), Date: "2026-01-01"},
	}}
	if got := s.CadenceDays(); got != 30 {
		t.Errorf("CadenceDays = %d, want 30", got)
	}
	if got := s.Latest().Date; got != "2026-10-01" {
		t.Errorf("Latest = %s, want 2026-10-01", got)
	}
	if p := s.Problems(); len(p) != 0 {
		t.Errorf("Problems = %v, want none", p)
	}
	s.Cadence = "fortnightly"
	if p := s.Problems(); len(p) != 1 || !strings.Contains(p[0], "cadence") {
		t.Errorf("Problems = %v, want one problem of the cadence", p)
	}
	s.Cadence, s.Snapshots = "none", nil
	if s.CadenceDays() != 0 || s.Latest() != nil {
		t.Error("the cadence none has days, or a source with no snapshot has a last snapshot")
	}
}

func TestLineage(t *testing.T) {
	s := Source{ID: "openalex", Name: "OpenAlex", Licence: "CC0-1.0", Class: ClassOf("CC0-1.0"), Lineage: []string{"crossref", "pubmed"}}
	if p := s.Problems(); len(p) != 0 {
		t.Errorf("Problems = %v, want none", p)
	}
	s.Lineage = []string{"crossref", "openalex", "crossref"}
	if p := s.Problems(); len(p) != 2 {
		t.Errorf("Problems = %v, want the source itself and the repeat", p)
	}
	c := Catalogue{Sources: []Source{s, {ID: "zbmath"}}}
	if l := c.Lineage(); len(l) != 1 || len(l["openalex"]) != 3 {
		t.Errorf("Lineage = %v", l)
	}
}

func TestSourceID(t *testing.T) {
	for id, ok := range map[string]bool{"springer-series": true, "loc-marc": true, "lebl_ra": true, "Springer": false, "-x": false, "x y": false} {
		s := Source{ID: id, Name: "X", Licence: "CC0-1.0", Class: ClassOf("CC0-1.0")}
		if got := len(s.Problems()) == 0; got != ok {
			t.Errorf("the id %q: no problems is %v, want %v (%v)", id, got, ok, s.Problems())
		}
	}
}
