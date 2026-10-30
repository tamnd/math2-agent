// Package cluster groups the source records of the catalogues into resources (RS10). The passes C1 to C5 of RS10.2 join records, the most certain first, and no pass splits what an earlier pass joined. A pair in the middle band of C4 goes to the queue res-cluster for the judge and a person (C6 and C7).
package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource"
)

// RecordsDir is the directory of the source records in the tree, one directory for each source. It is rebuilt from the snapshots and is not in git (RS10.1).
const RecordsDir = "build/resources/source-records"

// Record is one source record: one record of one catalogue, mapped to the fields of RS2.4.
type Record struct {
	Source    string              `json:"source"`              // the source, from manifests/sources.yaml
	Snapshot  string              `json:"snapshot"`            // the hash of the snapshot
	NativeID  string              `json:"native_id"`           // the id of the record in the source
	Level     string              `json:"level"`               // work, expression or manifestation
	Kind      string              `json:"kind"`                // one of resource.Kinds
	Thesis    string              `json:"thesis,omitempty"`    // of a thesis: the level, one of resource.ThesisLevels
	Title     string              `json:"title"`               // the title in the original language
	TitleEn   string              `json:"title_en,omitempty"`  // an English title that the catalogue gives
	Creators  []resource.Creator  `json:"creators,omitempty"`  // the creators
	Date      string              `json:"date,omitempty"`      // ISO 8601
	Language  string              `json:"language,omitempty"`  // BCP 47
	Edition   string              `json:"edition,omitempty"`   // the edition statement
	MSC       []string            `json:"msc,omitempty"`       // MSC2020 codes
	Subjects  []resource.Subject  `json:"subjects,omitempty"`  // other classifications
	IDs       resource.IDs        `json:"ids,omitempty"`       // the external ids, in the normal form of RS3.3
	Venue     string              `json:"venue,omitempty"`     // the ISSN or the resource id of the journal or series
	Volume    string              `json:"volume,omitempty"`    // the volume in the venue
	Page      string              `json:"page,omitempty"`      // the first page
	Publisher string              `json:"publisher,omitempty"` // of a manifestation
	Links     []string            `json:"links,omitempty"`     // the home pages of the manifestation
	Relations []resource.Relation `json:"relations,omitempty"` // edges that the catalogue gives, with an external id as the target, as doi:10.1/x
}

// Key gives the source and the native id, which name the record.
func (r *Record) Key() string { return r.Source + ":" + r.NativeID }

// Year gives the year of the date, or 0.
func (r *Record) Year() int {
	if len(r.Date) < 4 {
		return 0
	}
	y := 0
	for _, c := range r.Date[:4] {
		if c < '0' || c > '9' {
			return 0
		}
		y = y*10 + int(c-'0')
	}
	return y
}

// ReadRecords reads each file of records (mdrec.Ext) of the directory of each source under RecordsDir in the tree at root. A tree with no directory has no records. A file of JSON Lines that has no Markdown file is read too.
func ReadRecords(root string) ([]Record, error) {
	files, err := mdrec.Glob(filepath.Join(root, RecordsDir, "*", "*"+mdrec.Ext))
	if err != nil {
		return nil, err
	}
	var out []Record
	for _, f := range files {
		rs, err := readFile(f)
		if err != nil {
			return nil, err
		}
		out = append(out, rs...)
	}
	return out, nil
}

func readFile(path string) ([]Record, error) {
	recs, err := mdrec.Read(path)
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(recs))
	for n, rec := range recs {
		var r Record
		dec := json.NewDecoder(bytes.NewReader(rec))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("%s: record %d: %w", path, n+1, err)
		}
		out = append(out, r)
	}
	return out, nil
}

// WriteRecords writes the records of one source to RecordsDir/<source>/<name>.md in the tree at root, in the order of the native id.
func WriteRecords(root, source, name string, rs []Record) error {
	if strings.ContainsAny(source+name, "/\\") || source == "" || name == "" {
		return fmt.Errorf("cluster: the source %q or the name %q is not a file name", source, name)
	}
	slices.SortFunc(rs, func(a, b Record) int { return strings.Compare(a.NativeID, b.NativeID) })
	recs, err := mdrec.Marshal(rs)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, RecordsDir, source)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	return mdrec.Write(filepath.Join(dir, name+mdrec.Ext), recs)
}

// CrosswalksDir holds the links of the crosswalks of the snapshots, one file of records for each crosswalk, one Link in each record.
const CrosswalksDir = "build/resources/crosswalks"

// ReadCrosswalks reads each file of records of CrosswalksDir in the tree at root. A tree with no directory has no links.
func ReadCrosswalks(root string) ([]Link, error) {
	files, err := mdrec.Glob(filepath.Join(root, CrosswalksDir, "*"+mdrec.Ext))
	if err != nil {
		return nil, err
	}
	var out []Link
	for _, f := range files {
		recs, err := mdrec.Read(f)
		if err != nil {
			return nil, err
		}
		for n, rec := range recs {
			var l Link
			if err := json.Unmarshal(rec, &l); err != nil || l.A == "" || l.B == "" {
				return nil, fmt.Errorf("%s: record %d: want a link with the fields a and b", f, n+1)
			}
			out = append(out, l)
		}
	}
	return out, nil
}
