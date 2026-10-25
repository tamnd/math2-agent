package ingest

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"go.yaml.in/yaml/v3"
)

// The files of an import, relative to the root of the math2 working tree.
const (
	CatalogueFile = "manifests/sources.yaml"
	ManifestDir   = "manifests/ingest"
	RecordsDir    = "build/records"
)

// Source is one entry of the catalogue.
type Source struct {
	ID          string     `yaml:"id"`                    // the name of the source in sources: and same_as, for example oeis
	Name        string     `yaml:"name"`                  // the full name, for example The On-Line Encyclopedia of Integer Sequences
	Home        string     `yaml:"home,omitempty"`        // the home page of the source
	Licence     string     `yaml:"licence"`               // the SPDX identifier of the licence of what math2 takes
	Class       Class      `yaml:"class"`                 // the access class. A maintainer approves it.
	Attribution string     `yaml:"attribution,omitempty"` // the attribution line that a share-alike licence asks for
	Snapshots   []Snapshot `yaml:"snapshots,omitempty"`   // the snapshots that math2 used, oldest first
}

// Snapshot is one pinned download of a source.
type Snapshot struct {
	URL    string `yaml:"url"`
	Hash   string `yaml:"hash"`             // blake3: and 64 lowercase hex characters, of the downloaded bytes
	Date   string `yaml:"date"`             // the date of the download, YYYY-MM-DD
	Commit string `yaml:"commit,omitempty"` // the commit, when the source is a git repository
}

// Catalogue is the file manifests/sources.yaml. It records each source and each snapshot that math2 used.
type Catalogue struct {
	Sources []Source `yaml:"sources"`
}

var (
	hashPattern = regexp.MustCompile(`^blake3:[0-9a-f]{64}$`)
	datePattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	idPattern   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// ReadCatalogue reads manifests/sources.yaml in the tree at root. A tree without the file has an empty catalogue.
func ReadCatalogue(root string) (*Catalogue, error) {
	data, err := os.ReadFile(filepath.Join(root, CatalogueFile)) // #nosec G304 -- the caller owns the tree.
	if errors.Is(err, fs.ErrNotExist) {
		return &Catalogue{}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Catalogue
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", CatalogueFile, err)
	}
	return &c, nil
}

// Source returns the entry of the source with the id, or nil.
func (c *Catalogue) Source(id string) *Source {
	for i := range c.Sources {
		if c.Sources[i].ID == id {
			return &c.Sources[i]
		}
	}
	return nil
}

// Snapshot returns the snapshot with the hash, or nil.
func (s *Source) Snapshot(hash string) *Snapshot {
	for i := range s.Snapshots {
		if s.Snapshots[i].Hash == hash {
			return &s.Snapshots[i]
		}
	}
	return nil
}

// Problems returns the problems of the entry, one sentence each. An entry without problems names its licence and a class that agrees with it, and each snapshot has a URL, a hash and a date.
func (s *Source) Problems() []string {
	var out []string
	if !idPattern.MatchString(s.ID) {
		out = append(out, fmt.Sprintf("the id %q is not lowercase letters, digits and _", s.ID))
	}
	if s.Name == "" {
		out = append(out, "the source has no name")
	}
	switch {
	case s.Licence == "":
		out = append(out, "the source names no licence")
	case !s.Class.Valid():
		out = append(out, fmt.Sprintf("the class %q is not an access class", s.Class))
	case s.Class != ClassOf(s.Licence) && s.Class != RecordOnly:
		out = append(out, fmt.Sprintf("the licence %s gives the class %s, and the source has the class %s", s.Licence, ClassOf(s.Licence), s.Class))
	}
	seen := map[string]bool{}
	for i, sn := range s.Snapshots {
		switch {
		case sn.URL == "":
			out = append(out, fmt.Sprintf("snapshot %d has no URL", i+1))
		case !hashPattern.MatchString(sn.Hash):
			out = append(out, fmt.Sprintf("snapshot %d has the hash %q, which is not blake3: and 64 hex characters", i+1, sn.Hash))
		case !datePattern.MatchString(sn.Date):
			out = append(out, fmt.Sprintf("snapshot %d has the date %q, which is not YYYY-MM-DD", i+1, sn.Date))
		case seen[sn.Hash]:
			out = append(out, fmt.Sprintf("snapshot %d has the same hash as an earlier snapshot", i+1))
		}
		seen[sn.Hash] = true
	}
	return out
}

// Write writes the catalogue to manifests/sources.yaml in the tree at root. The sources are in the order of their ids, and the snapshots in the order of their dates. Thus the file is the same for the same catalogue.
func (c *Catalogue) Write(root string) error {
	slices.SortFunc(c.Sources, func(a, b Source) int { return cmp.Compare(a.ID, b.ID) })
	for i := range c.Sources {
		slices.SortStableFunc(c.Sources[i].Snapshots, func(a, b Snapshot) int { return cmp.Compare(a.Date, b.Date) })
	}
	var buf bytes.Buffer
	buf.WriteString("# The external sources of math2 and each snapshot that math2 used (spec 09.2). m2 ingest writes this file. A maintainer approves the class of each new source.\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return err
	}
	return writeFile(filepath.Join(root, CatalogueFile), buf.Bytes())
}

// writeFile writes data to path. It makes the directory first, and it replaces the file in one step.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil { // #nosec G302 -- the file is content of the working tree.
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
