// Package hathitrust is the adapter of the Hathifiles of HathiTrust (spec corpus resources RS4.1 and RS8.3, phase R2). It reads the volumes of HathiTrust that are copies of the books of the corpus, and it gives an ingest record and a source record for each catalogue record.
//
// A Hathifile has one line for each digitized volume, with its rights, and no subjects. Thus the adapter cannot test the class of a volume. Fetch reads the newest full Hathifile and keeps the volumes whose OCLC number, LCCN or ISBN is an id of a resource in the tree. It writes one JSON object for each catalogue record and each line, compressed with gzip. The records of HathiTrust are under CC0, and the records of the partner libraries give facts only (RQ04). The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/hathitrust"
package hathitrust

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// FileList is the URL of the list of the Hathifiles.
const FileList = "https://www.hathitrust.org/files/hathifiles/hathi_file_list.json"

// Fields are the columns of a Hathifile, in order (hathi_field_list.txt).
var Fields = []string{"htid", "access", "rights", "ht_bib_key", "description", "source", "source_bib_num", "oclc_num", "isbn", "issn", "lccn", "title", "imprint", "rights_reason_code",
	"rights_timestamp", "us_gov_doc_flag", "rights_date_used", "pub_place", "lang", "bib_fmt", "collection_code", "content_provider_code", "responsible_entity_code",
	"digitization_agent_code", "access_profile_code", "author"}

// Keys are the id systems that join a volume to a resource.
var Keys = []string{"oclc", "lccn", "isbn"}

// Adapter is the adapter of the Hathifiles.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of HathiTrust.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "hathitrust",
		Name:        "HathiTrust, the Hathifiles",
		Home:        "https://www.hathitrust.org/hathifiles",
		Licence:     "CC0-1.0",
		Class:       ingest.RecordOnly,
		Attribution: "HathiTrust Digital Library, https://www.hathitrust.org",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "hathitrust/1" }

// Dump returns the URL of the list of the Hathifiles.
func (Adapter) Dump() string { return FileList }

// Volume is one digitized volume.
type Volume struct {
	HTID        string `json:"htid"`
	Access      string `json:"access"`
	Rights      string `json:"rights"`
	Description string `json:"description,omitempty"`
}

// Item is one catalogue record of HathiTrust with its volumes, as one line of the snapshot.
type Item struct {
	Bib     string   `json:"bib"`
	Title   string   `json:"title"`
	Imprint string   `json:"imprint,omitempty"`
	Author  string   `json:"author,omitempty"`
	Lang    string   `json:"lang,omitempty"`
	Format  string   `json:"format,omitempty"` // BK, SE and the other formats of the record
	Date    string   `json:"date,omitempty"`
	OCLC    []string `json:"oclc,omitempty"`
	ISBN    []string `json:"isbn,omitempty"`
	ISSN    []string `json:"issn,omitempty"`
	LCCN    []string `json:"lccn,omitempty"`
	Volumes []Volume `json:"volumes"`
}

type file struct {
	Name string `json:"filename"`
	Full bool   `json:"full"`
	URL  string `json:"url"`
}

// Wanted gives the ids of the live resources in the tree at root, in the systems of Keys.
func Wanted(root string) (map[string]bool, error) {
	rs, err := resource.ReadResources(root)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	add := func(m resource.IDs) {
		for _, k := range Keys {
			for _, v := range m[k] {
				want[k+":"+v] = true
			}
		}
	}
	for i := range rs {
		if !rs[i].Live() {
			continue
		}
		add(rs[i].IDs)
		for _, e := range rs[i].Expressions {
			add(e.IDs)
			for _, m := range e.Manifestations {
				add(m.IDs)
			}
		}
	}
	return want, nil
}

// Fetch reads the list of the Hathifiles at req.URL, reads the newest full Hathifile, and writes the catalogue records whose volumes have an id of a resource in the tree.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	want, err := Wanted(req.Root)
	if err != nil {
		return err
	}
	if len(want) == 0 {
		return errors.New("hathitrust: the tree has no resource with an OCLC number, an LCCN or an ISBN")
	}
	body, err := req.Get(ctx, req.URL)
	if err != nil {
		return err
	}
	var files []file
	err = json.NewDecoder(body).Decode(&files)
	_ = body.Close()
	if err != nil {
		return fmt.Errorf("hathitrust: the list of the files: %w", err)
	}
	var full file
	for _, f := range files {
		if f.Full && f.Name > full.Name {
			full = f
		}
	}
	if full.URL == "" {
		return errors.New("hathitrust: the list has no full file")
	}
	body, err = req.Get(ctx, full.URL)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	items, err := Join(body, want)
	if err != nil {
		return fmt.Errorf("hathitrust: %s: %w", full.Name, err)
	}
	zw := gzip.NewWriter(w)
	enc := json.NewEncoder(zw)
	enc.SetEscapeHTML(false)
	for _, it := range items {
		if err := enc.Encode(it); err != nil {
			return err
		}
	}
	return zw.Close()
}

// Join reads a full Hathifile, compressed with gzip, and gives the catalogue records with a volume whose id is in want, in the order of their keys.
func Join(r io.Reader, want map[string]bool) ([]Item, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	items := map[string]*Item{}
	sc := bufio.NewScanner(zr)
	sc.Buffer(nil, 4<<20)
	for line := 1; sc.Scan(); line++ {
		row := strings.Split(sc.Text(), "\t")
		if len(row) < len(Fields) {
			if strings.TrimSpace(sc.Text()) == "" {
				continue
			}
			return nil, fmt.Errorf("line %d has %d fields, not %d", line, len(row), len(Fields))
		}
		get := func(i int) string { return strings.TrimSpace(row[i]) }
		oclc, isbn, issn, lccn := list("oclc", get(7)), list("isbn", get(8)), list("issn", get(9)), list("lccn", get(10))
		if !match(want, "oclc", oclc) && !match(want, "isbn", isbn) && !match(want, "lccn", lccn) {
			continue
		}
		bib := get(3)
		it := items[bib]
		if it == nil {
			it = &Item{Bib: bib, Title: get(11), Imprint: get(12), Author: get(25), Lang: get(18), Format: get(19)}
			if d := get(16); d != "9999" && d != "0000" {
				it.Date = d
			}
			items[bib] = it
		}
		it.OCLC, it.ISBN, it.ISSN, it.LCCN = union(it.OCLC, oclc), union(it.ISBN, isbn), union(it.ISSN, issn), union(it.LCCN, lccn)
		it.Volumes = append(it.Volumes, Volume{HTID: get(0), Access: get(1), Rights: get(2), Description: get(4)})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(items))
	for _, k := range slices.Sorted(maps.Keys(items)) {
		out = append(out, *items[k])
	}
	return out, nil
}

// list splits a field of ids at the commas and gives the ids in the normal form of the system key.
func list(key, s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if n, err := ids.Normalize(key, strings.TrimSpace(v)); err == nil && !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

func match(want map[string]bool, key string, vs []string) bool {
	for _, v := range vs {
		if want[key+":"+v] {
			return true
		}
	}
	return false
}

func union(a, b []string) []string {
	for _, v := range b {
		if !slices.Contains(a, v) {
			a = append(a, v)
		}
	}
	return a
}

func each(r io.Reader, fn func(Item) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 64<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var it Item
		if err := json.Unmarshal(sc.Bytes(), &it); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if it.Bib == "" {
			return fmt.Errorf("line %d: the record has no key", line)
		}
		if err := fn(it); err != nil {
			return err
		}
	}
	return sc.Err()
}

func link(it Item) string { return "https://catalog.hathitrust.org/Record/" + it.Bib }

// Kind gives the kind of a catalogue record: a journal for a serial, else a monograph.
func Kind(it Item) string {
	if it.Format == "SE" {
		return "journal"
	}
	return "monograph"
}

// Read reads the catalogue records and emits one ingest record for each.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(it Item) error {
		return emit(ingest.Record{ID: it.Bib, Kind: Kind(it), Title: Title(it.Title), URL: link(it)})
	})
}

// Records reads the catalogue records and emits one source record for each (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(it Item) error { return emit(SourceRecord(it)) })
}

// Title gives the title without the statement of responsibility, as Calculus in Calculus / by G. H. Hardy.
func Title(s string) string {
	if i := strings.Index(s, " / "); i > 0 {
		s = s[:i]
	}
	return strings.TrimSpace(strings.TrimRight(strings.Join(strings.Fields(s), " "), " ./:;,"))
}

// year finds the year at the end of an imprint, as , 1969- in Matica Slovenská, 1969-.
var year = regexp.MustCompile(`[,\s]*\[?c?[0-9]{4}[^,]*$`)

// dates finds the dates at the end of a name, as , 1877-1947.
var dates = regexp.MustCompile(`,?\s*(?:b\.\s*|d\.\s*)?[0-9]{4}\??-?(?:[0-9]{4})?\??\.?$`)

// SourceRecord makes the source record of one catalogue record. The record describes one edition, so its level is manifestation.
func SourceRecord(it Item) cluster.Record {
	rec := cluster.Record{NativeID: it.Bib, Level: "manifestation", Kind: Kind(it), Title: Title(it.Title), Date: it.Date, Language: fetch.Lang(it.Lang),
		Publisher: strings.TrimSpace(year.ReplaceAllString(it.Imprint, "")), IDs: resource.IDs{}, Links: []string{link(it)}}
	put := func(key string, vs ...string) {
		for _, v := range vs {
			if n, err := ids.Normalize(key, v); err == nil && !slices.Contains(rec.IDs[key], n) {
				rec.IDs[key] = append(rec.IDs[key], n)
			}
		}
	}
	put("hathi_record", it.Bib)
	for _, v := range it.Volumes {
		put("hathi", v.HTID)
	}
	put("oclc", it.OCLC...)
	put("isbn", it.ISBN...)
	put("issn", it.ISSN...)
	put("lccn", it.LCCN...)
	if a := strings.TrimSpace(strings.TrimRight(dates.ReplaceAllString(strings.TrimSpace(it.Author), ""), " ,")); a != "" {
		rec.Creators = []resource.Creator{{Name: a, Role: "author"}}
	}
	return rec
}
