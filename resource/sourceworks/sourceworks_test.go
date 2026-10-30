package sourceworks

import (
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource/cluster"
)

func TestSites(t *testing.T) {
	sites, err := Sites()
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	for _, s := range sites {
		kinds[s.ID] = s.Kind
		if s.Name == "" || s.Home == "" {
			t.Errorf("site %+v has no name or no home", s)
		}
	}
	for id, want := range map[string]string{"oeis": "dataset", "lmfdb": "dataset", "house_of_graphs": "dataset", "stacks": "website", "kerodon": "website", "mathlib": "software"} {
		if kinds[id] != want {
			t.Errorf("the kind of %s is %q, want %q", id, kinds[id], want)
		}
	}
}

func TestBuild(t *testing.T) {
	sites := []Site{
		{ID: "oeis", Kind: "dataset", QID: "Q728415", Name: "OEIS", Home: "https://oeis.org"},
		{ID: "stacks", Kind: "website", QID: "Q25099801", Name: "Stacks", Home: "https://stacks.example"},
		{ID: "kerodon", Kind: "website", Name: "Kerodon", Home: "https://kerodon.net"},
	}
	cat := &ingest.Catalogue{Sources: []ingest.Source{
		{ID: "oeis", Name: "The On-Line Encyclopedia of Integer Sequences", Kind: "database"},
		{ID: "unimath", Name: "UniMath", Kind: ingest.Library, Home: "https://github.com/UniMath/UniMath"},
		{ID: "arxiv", Name: "arXiv", Kind: "paper-index"},
		{ID: Source, Name: "this source", Kind: "catalogue"},
	}}
	lookup := func(id string) (ingest.Source, bool) {
		if id == "stacks" {
			return ingest.Source{ID: id, Name: "The Stacks Project", Home: "https://stacks.math.columbia.edu"}, true
		}
		return ingest.Source{}, false
	}
	recs := Build(sites, cat, lookup)
	if len(recs) != 4 {
		t.Fatalf("records %+v", recs)
	}
	r := recs[0]
	if r.Source != Source || r.NativeID != "oeis" || r.Level != "work" || r.Kind != "dataset" || r.Title != "The On-Line Encyclopedia of Integer Sequences" || r.IDs.Get("source") != "oeis" || r.IDs.Get("wikidata") != "Q728415" || r.Links[0] != "https://oeis.org" {
		t.Errorf("the catalogue must give the name: %+v", r)
	}
	if r := recs[1]; r.Title != "The Stacks Project" || r.Links[0] != "https://stacks.math.columbia.edu" {
		t.Errorf("the adapter must give the name and the home: %+v", r)
	}
	if r := recs[2]; r.Title != "Kerodon" || r.IDs.Get("wikidata") != "" {
		t.Errorf("sites.yaml must give the name: %+v", r)
	}
	if r := recs[3]; r.NativeID != "unimath" || r.Kind != "software" || r.IDs.Get("source") != "unimath" {
		t.Errorf("a library of the catalogue must be software: %+v", r)
	}
	for i := range recs {
		if why := cluster.Check(&recs[i]); why != "" {
			t.Errorf("%s: %s", recs[i].NativeID, why)
		}
	}
	if recs := Build(sites, nil, nil); len(recs) != 3 || recs[1].Title != "Stacks" {
		t.Errorf("with no catalogue and no adapters: %+v", recs)
	}
}
