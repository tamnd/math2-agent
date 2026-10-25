// Package palomar is the ingest adapter of Palomar, the public registry of Lean 4 formalizations whose proofs were checked at one pinned commit (spec 09.2). It gives one record for each registered result, at the newest version of its entry.
//
// The bulk channel is the listing of the registry API, with the filters in the query, for example
//
//	https://data.palomar-registry.org/api/v1/results?order=registered&trust=all
//
// The adapter follows the next cursor of the listing, one page after the other, with a pause between the pages. When the registry changes during the fetch, the API answers 409 Conflict, and the adapter starts again from the first page. The listing can abbreviate the large fields of an entry. For such an entry, the adapter gets the complete record at its versioned URL.
//
// The id of a record is the id of the entry, for example PALOMAR-2026-10-23-000018, and the kind is result. The title is the title of the entry and the text is its abstract. The URL is the permalink of the entry at its version. The meta of a record has these keys:
//
//	version       the version of the entry
//	authors       the authors, with "|" between them
//	arxiv         the arXiv categories, with a space between them
//	msc           the MSC 2020 classes, with a space between them
//	theorems      the Lean names of the theorems, with a space between them
//	repository    the GitHub repository of the source, for example Arthur742Ramos/Math-Formalization
//	commit        the commit of the source that the registry checked
//	project_path  the directory of the Lean project in the repository
//	trust         the trust level of the registry, for example high
//	status        the status of the entry, for example registered
//	published_at  the time of the registration of this version
//
// A key is not in the meta when the entry has no value for it. The metadata of the registry is under CC0 1.0.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/palomar"
package palomar

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/ingest"
)

// Home is the home of the registry.
const Home = "https://palomar-registry.org"

// Data is the origin of the data of the registry. The complete record of an entry is at Data/entries/<id>-v<version>.json.
const Data = "https://data.palomar-registry.org"

// Default is the listing that m2 ingest palomar gets when no URL is given: all the results, in the order of registration.
const Default = Data + "/api/v1/results?order=registered&trust=all"

// pause is the time between two requests to the registry.
var pause = time.Second

// restarts is the number of times that Fetch starts again when the registry changes.
const restarts = 3

// errChanged is the error of list when the revision of the registry changes between two pages.
var errChanged = errors.New("the registry changed")

// Adapter is the adapter of Palomar.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of Palomar.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "palomar",
		Name:        "Palomar",
		Home:        Home,
		Licence:     "CC0-1.0",
		Class:       ingest.Open,
		Attribution: "The Palomar registry of Lean 4 formalizations, " + Home + ", metadata CC0 1.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "palomar/1" }

// Dump returns the listing of all the results.
func (Adapter) Dump() string { return Default }

// header is the first line of a snapshot.
type header struct {
	Query    string `json:"query"`    // the filters of the listing URL
	Revision int    `json:"revision"` // the revision of the registry
	Release  string `json:"release"`  // the hash of the release of the registry
	Results  int    `json:"results"`  // the number of results in the whole registry
}

// page is one page of the listing.
type page struct {
	SchemaVersion int    `json:"schema_version"`
	Revision      int    `json:"revision"`
	Release       string `json:"release"`
	Totals        struct {
		Results int `json:"results"`
	} `json:"totals"`
	Entries []json.RawMessage `json:"entries"`
	Next    *string           `json:"next"`
}

// Fetch gets the entries of the listing at req.URL and writes a snapshot, compressed with gzip: a header line with the filters and the revision of the registry, and then one line for each entry, in the order of the listing. An abbreviated entry is replaced by its complete record.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	u, err := url.Parse(req.URL)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(u.Path, "/api/v1/results") {
		return fmt.Errorf("%s is not the URL of the listing of the registry", req.URL)
	}
	q := u.Query()
	q.Del("cursor")
	u.RawQuery = q.Encode()
	data := u.Scheme + "://" + u.Host

	var (
		h       header
		entries []json.RawMessage
	)
	for try := 0; ; try++ {
		h, entries, err = list(ctx, req.Get, u)
		se, ok := errors.AsType[*ingest.StatusError](err)
		if (errors.Is(err, errChanged) || ok && se.Code == http.StatusConflict) && try < restarts {
			continue
		}
		if err != nil {
			return err
		}
		break
	}
	if err := complete(ctx, req.Get, data, entries); err != nil {
		return err
	}

	zw := gzip.NewWriter(w)
	bw := bufio.NewWriter(zw)
	enc := json.NewEncoder(bw)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(h); err != nil {
		return err
	}
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	return zw.Close()
}

// list gets all the pages of the listing at u. It fails when the revision of the registry changes between two pages.
func list(ctx context.Context, get ingest.Getter, u *url.URL) (header, []json.RawMessage, error) {
	h := header{Query: u.RawQuery}
	var entries []json.RawMessage
	first := *u
	next := first.String()
	for n := 1; ; n++ {
		p, err := getPage(ctx, get, next)
		if err != nil {
			return h, nil, fmt.Errorf("page %d: %w", n, err)
		}
		if p.SchemaVersion != 1 {
			return h, nil, fmt.Errorf("page %d has the schema version %d, and the adapter reads version 1", n, p.SchemaVersion)
		}
		if n == 1 {
			h.Revision, h.Release, h.Results = p.Revision, p.Release, p.Totals.Results
		} else if p.Revision != h.Revision {
			return h, nil, fmt.Errorf("page %d: %w from revision %d to %d", n, errChanged, h.Revision, p.Revision)
		}
		entries = append(entries, p.Entries...)
		if p.Next == nil || *p.Next == "" || len(p.Entries) == 0 {
			return h, entries, nil
		}
		q := first.Query()
		q.Set("cursor", *p.Next)
		c := first
		c.RawQuery = q.Encode()
		next = c.String()
		if err := wait(ctx); err != nil {
			return h, nil, err
		}
	}
}

func getPage(ctx context.Context, get ingest.Getter, u string) (*page, error) {
	body, err := get(ctx, u)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()
	var p page
	if err := json.NewDecoder(body).Decode(&p); err != nil {
		return nil, fmt.Errorf("the answer is not JSON: %w", err)
	}
	return &p, nil
}

// complete replaces each abbreviated entry by its complete record.
func complete(ctx context.Context, get ingest.Getter, data string, entries []json.RawMessage) error {
	for i, raw := range entries {
		var e struct {
			ID          string `json:"id"`
			Version     int    `json:"version"`
			Abbreviated bool   `json:"abbreviated"`
		}
		if err := json.Unmarshal(raw, &e); err != nil {
			return fmt.Errorf("entry %d: %w", i+1, err)
		}
		if !e.Abbreviated {
			continue
		}
		if err := wait(ctx); err != nil {
			return err
		}
		u := data + "/entries/" + e.ID + "-v" + strconv.Itoa(e.Version) + ".json"
		body, err := get(ctx, u)
		if err != nil {
			return err
		}
		b, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil {
			return err
		}
		if !json.Valid(b) {
			return fmt.Errorf("the record of %s is not JSON", e.ID)
		}
		entries[i] = b
	}
	return nil
}

func wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(pause):
		return nil
	}
}

// entry is the part of a listing entry or a complete record that the adapter reads.
type entry struct {
	ID       string `json:"id"`
	Version  int    `json:"version"`
	Title    string `json:"title"`
	Abstract string `json:"abstract"`
	Authors  []struct {
		Name string `json:"name"`
	} `json:"authors"`
	Classification struct {
		Arxiv   []string `json:"arxiv"`
		MSC2020 []string `json:"msc2020"`
	} `json:"classification"`
	Formalization struct {
		TheoremNames []string `json:"theorem_names"`
	} `json:"formalization"`
	Source struct {
		Repository  string `json:"repository"`
		Commit      string `json:"commit"`
		ProjectPath string `json:"project_path"`
	} `json:"source"`
	Trust struct {
		Level string `json:"level"`
	} `json:"trust"`
	Status      string `json:"status"`
	PublishedAt string `json:"published_at"`
	// RegisteredAt is the time of the registration in a complete record, which has no published_at.
	RegisteredAt string `json:"registered_at"`
	Abbreviated  bool   `json:"abbreviated"`
}

// Read reads a snapshot and emits one record for each entry. Read fails on an id that does not pass crosswalk.Check, on two entries with the same id, on an entry with no title or no version, on an abbreviated entry and on a snapshot with no entries.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	dec := json.NewDecoder(r)
	var h header
	if err := dec.Decode(&h); err != nil {
		return fmt.Errorf("the header: %w", err)
	}
	seen := map[string]bool{}
	for n := 1; ; n++ {
		var e entry
		err := dec.Decode(&e)
		if errors.Is(err, io.EOF) {
			if n == 1 {
				return errors.New("the snapshot has no entries")
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("entry %d: %w", n, err)
		}
		if err := crosswalk.Check("palomar", e.ID); err != nil {
			return fmt.Errorf("entry %d: %w", n, err)
		}
		switch {
		case seen[e.ID]:
			return fmt.Errorf("the snapshot has %s two times", e.ID)
		case e.Title == "" || e.Version < 1:
			return fmt.Errorf("%s has no title or no version", e.ID)
		case e.Abbreviated:
			return fmt.Errorf("%s is abbreviated", e.ID)
		}
		seen[e.ID] = true
		if err := emit(record(e)); err != nil {
			return err
		}
	}
}

// record makes the record of one entry.
func record(e entry) ingest.Record {
	authors := make([]string, len(e.Authors))
	for i, a := range e.Authors {
		authors[i] = a.Name
	}
	published := e.PublishedAt
	if published == "" {
		published = e.RegisteredAt
	}
	meta := map[string]string{"version": strconv.Itoa(e.Version)}
	for k, v := range map[string]string{
		"authors":      strings.Join(authors, "|"),
		"arxiv":        strings.Join(e.Classification.Arxiv, " "),
		"msc":          strings.Join(e.Classification.MSC2020, " "),
		"theorems":     strings.Join(e.Formalization.TheoremNames, " "),
		"repository":   e.Source.Repository,
		"commit":       e.Source.Commit,
		"project_path": e.Source.ProjectPath,
		"trust":        e.Trust.Level,
		"status":       e.Status,
		"published_at": published,
	} {
		if v != "" {
			meta[k] = v
		}
	}
	return ingest.Record{
		ID:    e.ID,
		Kind:  "result",
		Title: e.Title,
		URL:   Home + "/entry?id=" + e.ID + "&version=" + strconv.Itoa(e.Version),
		Text:  e.Abstract,
		Meta:  meta,
	}
}
