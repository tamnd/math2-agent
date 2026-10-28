package wpcorpus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestRecordRevision(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	day := func(n int) string { return now.AddDate(0, 0, -n).Format(time.RFC3339) }
	for _, c := range []struct {
		name     string
		hist     []Revision
		rev      int
		unstable bool
	}{
		{"settled", []Revision{{ID: 3, Timestamp: day(2), SHA1: "c"}, {ID: 2, Timestamp: day(10), SHA1: "b"}, {ID: 1, Timestamp: day(20), SHA1: "a"}}, 2, false},
		{"tagged", []Revision{{ID: 3, Timestamp: day(8), SHA1: "c", Tags: []string{"mw-reverted"}}, {ID: 2, Timestamp: day(9), SHA1: "b"}, {ID: 1, Timestamp: day(20), SHA1: "a"}}, 2, false},
		// Revision 2 is undone by revision 3, which has the sha1 of the parent of 2.
		{"undone", []Revision{{ID: 3, Timestamp: day(1), SHA1: "a"}, {ID: 2, Timestamp: day(10), SHA1: "b"}, {ID: 1, Timestamp: day(20), SHA1: "a"}}, 1, false},
		{"quiet", []Revision{{ID: 1, Timestamp: day(400), SHA1: "a"}}, 1, false},
		{"young", []Revision{{ID: 2, Timestamp: day(1), SHA1: "b"}}, 0, true},
		{"unstable", []Revision{{ID: 3, Timestamp: day(10), SHA1: "a", Tags: []string{"mw-reverted"}}, {ID: 2, Timestamp: day(100), SHA1: "b"}}, 0, true},
	} {
		rev, unstable := RecordRevision(c.hist, now)
		if rev != c.rev || unstable != c.unstable {
			t.Errorf("%s: got %d %v, want %d %v", c.name, rev, unstable, c.rev, c.unstable)
		}
	}
}

func TestHistory(t *testing.T) {
	tries := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("pageids") != "7" || q.Get("rvprop") != "ids|timestamp|sha1|tags" || q.Get("maxlag") == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		tries++
		if tries == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"query": map[string]any{"pages": []any{map[string]any{"pageid": 7, "revisions": []any{
			map[string]any{"revid": 11, "parentid": 10, "timestamp": "2026-09-01T00:00:00Z", "sha1": "x", "tags": []string{"mw-reverted"}},
		}}}}})
	}))
	defer srv.Close()
	f := &Fetcher{API: srv.URL, UserAgent: "m2/test"}
	hist, err := f.History(context.Background(), 7, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 1 || hist[0].ID != 11 || hist[0].Parent != 10 || hist[0].SHA1 != "x" || !slices.Equal(hist[0].Tags, []string{"mw-reverted"}) || tries != 2 {
		t.Errorf("history = %+v after %d tries", hist, tries)
	}
}

func TestMetadata(t *testing.T) {
	m := &Metadata{Page: 12345, Title: "Widget", Revision: 678, Timestamp: "2026-01-02T03:04:05Z", Length: 99, Type: "article", Wikidata: "Q1", Short: "A set with a map", Categories: []string{"Widgets"}, Redirects: []string{"Widgets"}, Unstable: true}
	data, err := MarshalMetadata(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseMetadata(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Page != 12345 || got.Short != m.Short || !slices.Equal(got.Categories, m.Categories) || !got.Unstable {
		t.Errorf("metadata = %+v", got)
	}
	if MetadataPath(12345) != filepath.Join("metadata", "0001", "12345.md") {
		t.Errorf("path = %s", MetadataPath(12345))
	}
}

func TestPageFacts(t *testing.T) {
	cats, short := PageFacts("{{Short description|A set with a map}}\nText.\n[[Category:Widgets|W]]\n[[Category:Set_theory]]\n[[Category:Widgets]]\n")
	if short != "A set with a map" || !slices.Equal(cats, []string{"Widgets", "Set theory"}) {
		t.Errorf("facts = %q %q", cats, short)
	}
}

func TestFetchOrder(t *testing.T) {
	cs := []*Candidate{
		{PageID: 1, Title: "Low", Importance: "Low"},
		{PageID: 2, Title: "Unknown"},
		{PageID: 3, Title: "Pilot_one", Importance: "Low"},
		{PageID: 4, Title: "Cited", Signals: S4},
		{PageID: 5, Title: "Top", Importance: "Top"},
	}
	FetchOrder(cs, map[string]bool{"Pilot_one": true})
	var ids []int64
	for _, c := range cs {
		ids = append(ids, c.PageID)
	}
	if !slices.Equal(ids, []int64{4, 3, 5, 1, 2}) {
		t.Errorf("order = %v", ids)
	}
}

func TestLinkResolver(t *testing.T) {
	cs := []*Candidate{{PageID: 9, Title: "Root_of_unity", InScope: true}, {PageID: 8, Title: "Out", InScope: false}}
	r := LinkResolver(cs, map[string]string{"Roots_of_unity": "Root_of_unity"})
	if r("root of unity") != 9 || r("Roots of unity") != 9 || r("Out") != 0 || r("None") != 0 {
		t.Error("the resolver does not follow the redirects")
	}
}

func TestWriteStops(t *testing.T) {
	root := t.TempDir()
	in := []Stop{{PageID: 9, Reason: "merged", Into: 3, Note: "the page redirects to X"}, {PageID: 2, Reason: "deleted"}}
	if err := WriteStops(root, in); err != nil {
		t.Fatal(err)
	}
	got, err := ReadStops(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].PageID != 2 || got[1].Into != 3 || got[1].Note != in[0].Note {
		t.Errorf("stops = %+v", got)
	}
	if err := WriteStops(root, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := ReadStops(root); len(got) != 0 {
		t.Errorf("stops after an empty write = %+v", got)
	}
}
