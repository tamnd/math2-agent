// Package carmin is the adapter of Carmin.tv, the library of videos of mathematics of the Labex CARMIN (spec corpus resources RS7.4 and RS8.3, phase R4). CIRM and the IHES put their lectures on Carmin.tv, so one package serves both. It gives a source record of kind course-material for each lecture, and a source record of kind video-series for each collection that a lecture is part of.
//
// Fetch reads the OAI-PMH endpoint of Carmin.tv, which its help page documents. It reads the list of the sets once for the names of the collections, then it harvests the set of the institution in the format oai_dc. It writes one JSON object for each collection and each lecture, compressed with gzip. The endpoint names its lists ListsRecords and ListsSets, not ListRecords and ListSets, so Fetch gives the oai package each page with the names of the protocol.
//
// The catalogue takes the title, the speakers, the date, the language, the collections and the keywords. It never takes the video, the abstract or the description of the collection. The format oai_dc gives no duration and no MSC code. The legal notice of Carmin.tv states no licence for the records and reserves the rights in the content, so the adapter keeps the class record only. The IHES works in theoretical physics too, and oai_dc gives no class, so the scope decision of RS09 keeps the mathematics. The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/carmin"
package carmin

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/adapters/oai"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Endpoint is the OAI-PMH endpoint of Carmin.tv.
const Endpoint = "https://www.carmin.tv/fr/oai"

// Prefix is the metadata format that Fetch asks for.
const Prefix = "oai_dc"

// Home is the home page of Carmin.tv.
const Home = "https://www.carmin.tv"

// Adapter is the adapter of the videos of one institution of Carmin.tv.
type Adapter struct {
	ID   string // the source id
	Name string
	Set  string        // the OAI-PMH set of the institution
	Gap  time.Duration // the shortest time between two requests
}

// CIRM is the adapter of the library of videos of CIRM, the Centre International de Rencontres Mathématiques.
var CIRM = Adapter{ID: "cirm", Name: "The library of videos of CIRM, on Carmin.tv", Set: "videos:institution:cirm", Gap: time.Second}

// IHES is the adapter of the videos of the IHES, the Institut des Hautes Études Scientifiques.
var IHES = Adapter{ID: "ihes", Name: "The videos of the IHES, on Carmin.tv", Set: "videos:institution:ihes", Gap: time.Second}

func init() {
	ingest.Register(CIRM)
	ingest.Register(IHES)
}

// Source returns the catalogue entry of the videos of the institution.
func (a Adapter) Source() ingest.Source {
	return ingest.Source{ID: a.ID, Name: a.Name, Home: Home, Licence: "LicenseRef-facts", Class: ingest.RecordOnly, Attribution: "Carmin.tv, Labex CARMIN, " + Home}
}

// Version returns the version of the adapter.
func (a Adapter) Version() string { return a.ID + "/1" }

// Dump returns the OAI-PMH endpoint.
func (a Adapter) Dump() string { return Endpoint }

// Collection is one collection of Carmin.tv: a workshop, a school, a seminar or a course of lectures.
type Collection struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

// Lecture is one video of Carmin.tv.
type Lecture struct {
	Slug        string   `json:"slug"`
	URL         string   `json:"url,omitempty"`
	Title       string   `json:"title"`
	Speakers    []string `json:"speakers,omitempty"`
	Date        string   `json:"date,omitempty"`
	Language    string   `json:"language,omitempty"`
	Publisher   string   `json:"publisher,omitempty"`
	Topics      []string `json:"topics,omitempty"`      // the keywords as Carmin.tv states them
	Collections []string `json:"collections,omitempty"` // the slugs of the collections
}

// Line is one line of the snapshot: a collection or a lecture.
type Line struct {
	Collection *Collection `json:"collection,omitempty"`
	Lecture    *Lecture    `json:"lecture,omitempty"`
}

// The prefixes of the sets and of the names of the sets of the collections.
const (
	collectionSet  = "videos:collection:"
	collectionName = "Vidéos : collection : "
)

// Audiences are the values of dc:subject that name an audience and not a topic.
var Audiences = []string{"researchers", "graduate students"}

type sets struct {
	Error struct {
		Code string `xml:"code,attr"`
		Text string `xml:",chardata"`
	} `xml:"error"`
	Sets []struct {
		Spec string `xml:"setSpec"`
		Name string `xml:"setName"`
	} `xml:"ListSets>set"`
}

type dc struct {
	Identifier  []string `xml:"identifier"`
	Title       string   `xml:"title"`
	Publisher   string   `xml:"publisher"`
	Date        string   `xml:"date"`
	Contributor []string `xml:"contributor"`
	Subject     []string `xml:"subject"`
	Language    string   `xml:"language"`
}

// protocol gives the names of the protocol to the lists that Carmin.tv names ListsRecords and ListsSets.
var protocol = strings.NewReplacer("<ListsRecords>", "<ListRecords>", "</ListsRecords>", "</ListRecords>", "<ListsSets>", "<ListSets>", "</ListsSets>", "</ListSets>")

// getter gives each body with the names of the protocol, and waits gap between two requests.
func getter(get ingest.Getter, gap time.Duration) ingest.Getter {
	var last time.Time
	return func(ctx context.Context, u string) (io.ReadCloser, error) {
		if wait := gap - time.Since(last); !last.IsZero() && wait > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
		last = time.Now()
		body, err := get(ctx, u)
		if err != nil {
			return nil, err
		}
		defer func() { _ = body.Close() }()
		data, err := io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", u, err)
		}
		return io.NopCloser(strings.NewReader(protocol.Replace(string(data)))), nil
	}
}

// Fetch reads the names of the collections, harvests the set of the institution from the endpoint at req.URL, and writes the collections that a lecture is part of, then the lectures, as JSON Lines compressed with gzip.
func (a Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	get := getter(req.Get, a.Gap)
	names, err := collections(ctx, get, req.URL)
	if err != nil {
		return err
	}
	var lectures []Lecture
	used := map[string]bool{}
	err = oai.Harvest(ctx, get, req.URL, Prefix, a.Set, func(r oai.Record) error {
		if r.Deleted {
			return nil
		}
		var d dc
		if err := oai.Unmarshal(r.Metadata, &d); err != nil {
			return fmt.Errorf("%s: %w", r.ID, err)
		}
		l := lecture(r, d)
		if l.Slug == "" || l.Title == "" {
			return nil
		}
		for _, c := range l.Collections {
			used[c] = true
		}
		lectures = append(lectures, l)
		return nil
	})
	if err != nil {
		return fmt.Errorf("%s: %w", a.ID, err)
	}
	zw := gzip.NewWriter(w)
	enc := json.NewEncoder(zw)
	enc.SetEscapeHTML(false)
	slugs := make([]string, 0, len(used))
	for s := range used {
		slugs = append(slugs, s)
	}
	slices.Sort(slugs)
	for _, s := range slugs {
		if names[s] == "" {
			continue
		}
		if err := enc.Encode(Line{Collection: &Collection{Slug: s, Title: names[s]}}); err != nil {
			return err
		}
	}
	for i := range lectures {
		if err := enc.Encode(Line{Lecture: &lectures[i]}); err != nil {
			return err
		}
	}
	return zw.Close()
}

// collections reads the list of the sets and gives the title of each collection by its slug. The list comes in one page.
func collections(ctx context.Context, get ingest.Getter, base string) (map[string]string, error) {
	u := base + "?verb=ListSets"
	body, err := get(ctx, u)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", u, err)
	}
	var s sets
	if err := oai.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", u, err)
	}
	if s.Error.Code != "" {
		return nil, fmt.Errorf("%s: %s: %s", u, s.Error.Code, strings.TrimSpace(s.Error.Text))
	}
	out := map[string]string{}
	for _, x := range s.Sets {
		slug, ok := strings.CutPrefix(strings.TrimSpace(x.Spec), collectionSet)
		if !ok {
			continue
		}
		if t := clean(strings.TrimPrefix(strings.TrimSpace(x.Name), collectionName)); t != "" {
			out[slug] = t
		}
	}
	return out, nil
}

// lecture maps one record of the endpoint to a lecture.
func lecture(r oai.Record, d dc) Lecture {
	l := Lecture{Title: clean(d.Title), Publisher: clean(d.Publisher), Language: fetch.Lang(d.Language)}
	if s, err := ids.Normalize("carmin", r.ID); err == nil {
		l.Slug = s
	}
	for _, v := range d.Identifier {
		if v = strings.TrimSpace(v); strings.HasPrefix(v, Home+"/") {
			l.URL = v
			break
		}
	}
	if len(d.Date) >= 10 {
		l.Date = d.Date[:10]
	}
	for _, n := range d.Contributor {
		if n = clean(n); n != "" && !slices.Contains(l.Speakers, n) {
			l.Speakers = append(l.Speakers, n)
		}
	}
	for _, s := range d.Subject {
		for _, t := range strings.Split(s, ",") {
			if t = clean(t); t != "" && !slices.Contains(Audiences, strings.ToLower(t)) && !slices.Contains(l.Topics, t) {
				l.Topics = append(l.Topics, t)
			}
		}
	}
	for _, s := range r.Sets {
		if c, ok := strings.CutPrefix(strings.TrimSpace(s), collectionSet); ok && !slices.Contains(l.Collections, c) {
			l.Collections = append(l.Collections, c)
		}
	}
	return l
}

func clean(s string) string { return strings.Join(strings.Fields(s), " ") }

// each reads the snapshot and calls the function of each line. The collections come before the lectures.
func each(r io.Reader, collection func(Collection) error, lecture func(Lecture) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 16<<20)
	for line := 1; sc.Scan(); line++ {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var l Line
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		switch {
		case l.Collection != nil:
			if l.Collection.Slug == "" {
				return fmt.Errorf("line %d: the collection has no slug", line)
			}
			if err := collection(*l.Collection); err != nil {
				return err
			}
		case l.Lecture != nil:
			if l.Lecture.Slug == "" {
				return fmt.Errorf("line %d: the lecture has no slug", line)
			}
			if err := lecture(*l.Lecture); err != nil {
				return err
			}
		default:
			return fmt.Errorf("line %d: the line has no collection and no lecture", line)
		}
	}
	return sc.Err()
}

// Read reads the snapshot and emits one ingest record for each collection and each lecture.
func (a Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(c Collection) error {
		return emit(ingest.Record{ID: collectionSet + c.Slug, Kind: "video-series", Title: c.Title})
	}, func(l Lecture) error {
		return emit(ingest.Record{ID: l.Slug, Kind: "course-material", Title: l.Title, URL: l.URL})
	})
}

// Records reads the snapshot and emits one source record for each collection and each lecture (RS13.1). A lecture has a part_of edge to each collection of the snapshot that it is in.
func (a Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	known := map[string]bool{}
	return each(r, func(c Collection) error {
		known[c.Slug] = true
		return emit(SeriesRecord(c))
	}, func(l Lecture) error {
		rec := LectureRecord(l)
		for _, c := range l.Collections {
			if known[c] {
				rec.Relations = append(rec.Relations, resource.Relation{Kind: "part_of", To: "carmin_collection:" + c, Source: a.ID})
			}
		}
		return emit(rec)
	})
}

// SeriesRecord makes the source record of one collection. Its native id is the set of the collection, so that it never meets the slug of a lecture.
func SeriesRecord(c Collection) cluster.Record {
	return cluster.Record{NativeID: collectionSet + c.Slug, Level: "work", Kind: "video-series", Title: c.Title, IDs: resource.IDs{"carmin_collection": {c.Slug}}}
}

// LectureRecord makes the source record of one lecture, with no edge. A speaker is a creator with the role author. The keywords are subjects of the system keyword.
func LectureRecord(l Lecture) cluster.Record {
	rec := cluster.Record{NativeID: l.Slug, Level: "work", Kind: "course-material", Title: l.Title, Date: l.Date, Language: l.Language, Publisher: l.Publisher, IDs: resource.IDs{"carmin": {l.Slug}}}
	if l.URL != "" {
		rec.Links = []string{l.URL}
	}
	for _, n := range l.Speakers {
		rec.Creators = append(rec.Creators, resource.Creator{Name: n, Role: "author"})
	}
	for _, t := range l.Topics {
		rec.Subjects = append(rec.Subjects, resource.Subject{System: "keyword", Code: t})
	}
	return rec
}
