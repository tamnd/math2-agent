package nptelvideo

import (
	"context"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/adapters/nptel"
)

func TestBuild(t *testing.T) {
	in := &resource.Scope{Value: resource.In}
	course := func(id, nptelID string, scope *resource.Scope) resource.Resource {
		return resource.Resource{ID: id, Kind: "course", Title: "Course " + id, Scope: scope, Expressions: []resource.Expression{{Key: "e1", Manifestations: []resource.Manifestation{{Key: "m1", IDs: resource.IDs{"nptel": {nptelID}}}}}}}
	}
	rs := []resource.Resource{
		course("8F2AAAAAA3", "111105069", in),
		course("8F2BBBBBB6", "111105070", in),
		course("8F2CCCCCC9", "111105071", &resource.Scope{Value: resource.Out}),
		course("8F2DDDDDD2", "111105069", in),
		{ID: "8F2EEEEEE5", Kind: "monograph", Title: "No course", Scope: in},
	}
	pages := map[string][]nptel.Unit{
		"111105069": {{Title: "Week 1", Lessons: []nptel.Lesson{{Title: "Intro"}, {Title: "Cuts", YouTube: "Bef8QjIjCy0"}}}, {Title: "Week 2", Lessons: []nptel.Lesson{{Title: "Sets", YouTube: "tsdoIka0mEg"}}}},
	}
	var asked []string
	page := func(_ context.Context, id string) ([]nptel.Unit, error) {
		asked = append(asked, id)
		return pages[id], nil
	}
	uploaded := func(_ context.Context, v string) (string, error) {
		if v != "Bef8QjIjCy0" {
			t.Errorf("asked for the video %s", v)
		}
		return "2013-07-02", nil
	}
	recs, outs, n, err := Build(context.Background(), rs, page, uploaded)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(asked, " ") != "111105069 111105070" || n.Asked != 2 || n.Found != 1 {
		t.Fatalf("asked %v, counts %+v", asked, n)
	}
	if len(recs) != 1 || recs[0].Date != "2013-07-02" || recs[0].IDs["nptel"][0] != "111105069" || recs[0].Kind != "course" {
		t.Fatalf("records %+v", recs)
	}
	if len(outs) != 1 || outs[0].ID != "nptel:111105069" || len(outs[0].Nodes) != 2 {
		t.Fatalf("outlines %+v", outs)
	}
	w2 := outs[0].Nodes[1]
	if w2.Kind != "part" || w2.Title != "Week 2" || len(w2.Children) != 1 || w2.Children[0].Number != "3" || w2.Children[0].Kind != "session" || w2.Children[0].Title != "Sets" {
		t.Errorf("week 2 %+v", w2)
	}
}
