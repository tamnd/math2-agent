// Package nptelvideo builds the source records of the dates of the courses of NPTEL and the outlines of their pages (spec corpus resources RS5.4 and RS5.5). The course list of NPTEL has no dates, so a course has no date and stays at depth D0.
//
// A run reads the data file of the page of each course of NPTEL in scope, https://nptel.ac.in/courses/<id>/__data.json. The file has the outline of the course: its units and the lectures of each unit, with the id of the video of each lecture on YouTube. The date of the course is the day that YouTube gives as the upload day of the video of the first lecture. The run asks for it with the fetcher ytb of tamnd/ytb-cli, which reads the public pages of YouTube and needs no key. Each record has the NPTEL id of the course, so the clustering puts it on the resource of the course. The outline is the structure of the expression of the course.
package nptelvideo

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strconv"

	"github.com/zeebo/blake3"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/adapters/nptel"
	"github.com/tamnd/math2-agent/resource/cluster"
)

// Source is the id of the source in manifests/sources.yaml.
const Source = "nptel_video"

// Version is the version of the adapter.
const Version = "nptel_video/1"

// Page gives the outline of the page of the course with the NPTEL id. A course with no page gives no unit and no error.
type Page func(ctx context.Context, id string) ([]nptel.Unit, error)

// Uploaded gives the upload day of the video on YouTube, as 2013-07-02, or "" when YouTube gives none.
type Uploaded func(ctx context.Context, video string) (string, error)

// Counts are the counts of one run.
type Counts struct {
	Asked int // the courses that the run asked for
	Found int // the courses with a date or an outline
}

// Outline is the structure of one course: the id of its expression, as nptel:<id>, and the nodes of the outline.
type Outline struct {
	ID    string
	Nodes []resource.Node
}

// needs reports whether the resource is live and in scope.
func needs(r *resource.Resource) bool {
	return r.Live() && r.Scope != nil && r.Scope.Value == resource.In
}

// courseID gives the first NPTEL id of a manifestation of the resource, or "".
func courseID(r *resource.Resource) string {
	for _, e := range r.Expressions {
		for _, m := range e.Manifestations {
			if v := m.IDs["nptel"]; len(v) > 0 {
				return v[0]
			}
		}
	}
	return ""
}

// Build reads the page of each course of NPTEL in scope and gives one record for each course with a date, and the outline of each course with one.
func Build(ctx context.Context, rs []resource.Resource, page Page, uploaded Uploaded) ([]cluster.Record, []Outline, Counts, error) {
	var c Counts
	var recs []cluster.Record
	var outs []Outline
	seen := map[string]bool{}
	for i := range rs {
		r := &rs[i]
		id := courseID(r)
		if id == "" || !needs(r) || seen[id] {
			continue
		}
		seen[id] = true
		c.Asked++
		units, err := page(ctx, id)
		if err != nil {
			return nil, nil, c, err
		}
		nodes := Nodes(units)
		day := ""
		if v := first(units); v != "" {
			if day, err = uploaded(ctx, v); err != nil {
				return nil, nil, c, err
			}
		}
		if len(nodes) == 0 && day == "" {
			continue
		}
		c.Found++
		if len(nodes) > 0 {
			outs = append(outs, Outline{ID: "nptel:" + id, Nodes: nodes})
		}
		if day != "" {
			recs = append(recs, cluster.Record{Source: Source, NativeID: id, Level: "manifestation", Kind: r.Kind, Title: r.Title, Date: day, IDs: resource.IDs{"nptel": {id}}, Links: []string{nptel.Home + "/courses/" + id}})
		}
	}
	return recs, outs, c, nil
}

// first gives the video of the first lecture of the outline that has one, or "".
func first(units []nptel.Unit) string {
	for _, u := range units {
		if i := slices.IndexFunc(u.Lessons, func(l nptel.Lesson) bool { return l.YouTube != "" }); i >= 0 {
			return u.Lessons[i].YouTube
		}
	}
	return ""
}

// Nodes gives the nodes of the outline: a part for each unit, and a session for each lecture of the unit. The sessions have one count through the whole course.
func Nodes(units []nptel.Unit) []resource.Node {
	var out []resource.Node
	n := 0
	for i, u := range units {
		part := resource.Node{Key: "u" + strconv.Itoa(i+1), Kind: "part", Number: strconv.Itoa(i + 1), Title: u.Title}
		for _, l := range u.Lessons {
			n++
			part.Children = append(part.Children, resource.Node{Key: "l" + strconv.Itoa(n), Kind: "session", Number: strconv.Itoa(n), Title: l.Title})
		}
		out = append(out, part)
	}
	return out
}

// Entry gives the entry of the source in manifests/sources.yaml.
func Entry() ingest.Source {
	return ingest.Source{
		ID:          Source,
		Name:        "NPTEL, the outlines of the course pages and the upload days of the videos on YouTube",
		Kind:        "course-platform",
		Home:        nptel.Home,
		Licence:     "LicenseRef-facts",
		Class:       ingest.RecordOnly,
		Channel:     "page",
		Terms:       &ingest.Terms{Robots: "allows all paths"},
		Adapter:     Version,
		Attribution: "NPTEL, https://nptel.ac.in; YouTube, https://www.youtube.com",
	}
}

// Snapshot gives the snapshot of the records of one run: the hash of the records and the day.
func Snapshot(recs []cluster.Record, day string) (ingest.Snapshot, error) {
	h := blake3.New()
	enc := json.NewEncoder(h)
	for _, r := range recs {
		if err := enc.Encode(r); err != nil {
			return ingest.Snapshot{}, err
		}
	}
	return ingest.Snapshot{URL: "m2 res import " + Source, Hash: "blake3:" + hex.EncodeToString(h.Sum(nil)), Date: day}, nil
}
