// Package cluster groups the source records of the catalogues into resources (RS10). The passes C1 to C5 of RS10.2 join records, the most certain first, and no pass splits what an earlier pass joined. A pair in the middle band of C4 goes to the queue res-cluster for the judge and a person (C6 and C7).
package cluster

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

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

// ReadRecords reads each file *.jsonl of the directory of each source under RecordsDir in the tree at root. A tree with no directory has no records.
func ReadRecords(root string) ([]Record, error) {
	files, err := filepath.Glob(filepath.Join(root, RecordsDir, "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
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
	f, err := os.Open(path) // #nosec G304 -- the caller owns the tree.
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 64<<20)
	for n := 1; sc.Scan(); n++ {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var r Record
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, n, err)
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

// WriteRecords writes the records of one source to RecordsDir/<source>/<name>.jsonl in the tree at root, one record on each line in the order of the native id.
func WriteRecords(root, source, name string, rs []Record) error {
	if strings.ContainsAny(source+name, "/\\") || source == "" || name == "" {
		return fmt.Errorf("cluster: the source %q or the name %q is not a file name", source, name)
	}
	slices.SortFunc(rs, func(a, b Record) int { return strings.Compare(a.NativeID, b.NativeID) })
	var buf bytes.Buffer
	for i := range rs {
		data, err := json.Marshal(&rs[i])
		if err != nil {
			return err
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	dir := filepath.Join(root, RecordsDir, source)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	path := filepath.Join(dir, name+".jsonl")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// CrosswalksDir holds the links of the crosswalks of the snapshots, one file *.jsonl for each crosswalk, one Link on each line.
const CrosswalksDir = "build/resources/crosswalks"

// ReadCrosswalks reads each file *.jsonl of CrosswalksDir in the tree at root. A tree with no directory has no links.
func ReadCrosswalks(root string) ([]Link, error) {
	files, err := filepath.Glob(filepath.Join(root, CrosswalksDir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
	var out []Link
	for _, f := range files {
		data, err := os.ReadFile(f) // #nosec G304 -- the caller owns the tree.
		if err != nil {
			return nil, err
		}
		for n, line := range bytes.Split(data, []byte("\n")) {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var l Link
			if err := json.Unmarshal(line, &l); err != nil || l.A == "" || l.B == "" {
				return nil, fmt.Errorf("%s:%d: want a link with the fields a and b", f, n+1)
			}
			out = append(out, l)
		}
	}
	return out, nil
}
