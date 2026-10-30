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
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/tag"
)

// The files of an import, relative to the root of the math2 working tree.
const (
	CatalogueFile = "manifests/sources.yaml"
	ManifestDir   = "manifests/ingest"
	RecordsDir    = "build/records"
)

// Source is one entry of the catalogue: the source document of corpus K5.2. A person approves each entry and each change to its licence fields. The fields name and home are the title and the url of K5.2.
type Source struct {
	ID             string        `yaml:"id"`                         // the name of the source in sources: and same_as, for example oeis
	Name           string        `yaml:"name"`                       // the full name, for example The On-Line Encyclopedia of Integer Sequences
	Authors        []string      `yaml:"authors,omitempty"`          // the authors of a book or a paper
	Kind           string        `yaml:"kind,omitempty"`             // one of Kinds
	Home           string        `yaml:"home,omitempty"`             // the home page of the source
	Licence        string        `yaml:"licence"`                    // the SPDX expression of the licence, as the source states it
	Election       string        `yaml:"election,omitempty"`         // for a choice of licences, the SPDX identifier of the option that math2 takes (K5.2.2)
	Class          Class         `yaml:"class"`                      // the access class. A maintainer approves it.
	LicenceRead    *LicenceRead  `yaml:"licence_read,omitempty"`     // where, when and by whom the licence was read (K5.2.1)
	PerItemLicence bool          `yaml:"per_item_licence,omitempty"` // each item has its own licence (K13.4)
	PublicDomain   *PublicDomain `yaml:"public_domain,omitempty"`    // the public-domain answers for a work in the public domain (K5.2.3)
	Authored       string        `yaml:"authored,omitempty"`         // one of Authorships
	Intake         string        `yaml:"intake,omitempty"`           // for a formal library, one of Intakes (corpus K6.2)
	IntakeReason   string        `yaml:"intake_reason,omitempty"`    // why the library has its intake class
	Modules        []string      `yaml:"modules,omitempty"`          // for a Lean library, the root modules, for example Mathlib. Rule F13 finds the library of a declaration by them.
	Edition        string        `yaml:"edition,omitempty"`          // the pinned edition, where editions differ in licence
	Channel        string        `yaml:"channel,omitempty"`          // one of Channels
	Terms          *Terms        `yaml:"terms,omitempty"`            // the terms of use of the source
	Attribution    string        `yaml:"attribution,omitempty"`      // the attribution line that the licence asks for
	Adapter        string        `yaml:"adapter,omitempty"`          // the importer that reads the source
	ApprovedBy     string        `yaml:"approved_by,omitempty"`      // the maintainer who approved the entry
	Snapshots      []Snapshot    `yaml:"snapshots,omitempty"`        // the snapshots that math2 used, oldest first
	Resource       string        `yaml:"resource,omitempty"`         // for a source that is one work, for example an open textbook, the resource id of the work (corpus resources RS8.1)
	RecordLicence  string        `yaml:"record_licence,omitempty"`   // the SPDX expression of the licence of the bibliographic records, when it is not the licence of the texts (RS8.1)
}

// LicenceRead records one reading of the licence at the source itself, never at an aggregator (K5.2.1).
type LicenceRead struct {
	At string `yaml:"at"` // the URL of the page or the file with the licence
	On string `yaml:"on"` // the date of the reading, YYYY-MM-DD
	By string `yaml:"by"` // the person who read it
}

// PublicDomain records why a work is in the public domain in the United States and in the EU (K5.2.3).
type PublicDomain struct {
	US    bool   `yaml:"us"`
	EU    bool   `yaml:"eu"`
	Basis string `yaml:"basis"` // for example the date of publication and the year of the death of the author
}

// Terms summarises the terms of use of a source.
type Terms struct {
	Robots string `yaml:"robots,omitempty"` // a summary of robots.txt, or n/a
	Rate   string `yaml:"rate,omitempty"`   // the limit that the importer keeps
	AI     string `yaml:"ai,omitempty"`     // the clause on AI or machine learning, or "none stated"
}

// The values of the fields kind, authored, channel and intake of a source document (corpus K5.2 and K6.2).
var (
	Kinds       = []string{"reference", "wiki", "textbook", "paper-index", "extraction", "curriculum", "exercises", "database", Library, "catalogue", "course-platform"}
	Authorships = []string{"human", "ai", "mixed"}
	Channels    = []string{"dump", "git", "api", "bulk", "page", "catalogue"}
	Intakes     = []string{"bind", "crosswalk", "link", "evaluate", "exclude"}
)

// Library is the kind of a formal library, for example Mathlib (corpus K6.3).
const Library = "library"

// NoText reports whether the corpus takes no text and no statements from a library with the intake class: link, evaluate or exclude (rule F11).
func NoText(intake string) bool {
	return intake == "link" || intake == "evaluate" || intake == "exclude"
}

// ByAI reports whether a model wrote all or part of a source whose field authored has the value. The value mixed has the rules of ai (corpus K6.5).
func ByAI(authored string) bool {
	return authored == "ai" || authored == "mixed"
}

// PublicDomainLicence is the licence of a work in the public domain. SPDX has no identifier for it.
const PublicDomainLicence = "LicenseRef-public-domain"

// Choices returns the options of a licence that offers a choice, as in "CC-BY-SA-4.0 OR CC-BY-NC-SA-4.0". It returns nil for a licence with no choice.
func Choices(licence string) []string {
	parts := strings.Split(licence, " OR ")
	if len(parts) < 2 {
		return nil
	}
	for i := range parts {
		parts[i] = strings.Trim(strings.TrimSpace(parts[i]), "()")
	}
	return parts
}

// Elected returns the licence that math2 takes from the source: the election for a choice of licences, else the licence.
func (s *Source) Elected() string {
	if s.Election != "" {
		return s.Election
	}
	return s.Licence
}

// Snapshot is one pinned download of a source.
type Snapshot struct {
	URL         string `yaml:"url"`
	Hash        string `yaml:"hash"`                   // blake3: and 64 lowercase hex characters, of the downloaded bytes
	Date        string `yaml:"date"`                   // the date of the download, YYYY-MM-DD
	Commit      string `yaml:"commit,omitempty"`       // the commit, when the source is a git repository
	Licence     string `yaml:"licence,omitempty"`      // the licence of the snapshot, when it is not the licence of the source (K5.2.1)
	LicenceRead string `yaml:"licence_read,omitempty"` // the date on which the licence of the snapshot was read, when it is not the date of licence_read of the source
}

// ReadOn returns the date on which the licence of the snapshot was read, or "" when the source records no reading.
func (s *Source) ReadOn(sn *Snapshot) string {
	if sn.LicenceRead != "" {
		return sn.LicenceRead
	}
	if s.LicenceRead != nil {
		return s.LicenceRead.On
	}
	return ""
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
	case Choices(s.Licence) != nil && s.Election == "":
		// L09 reports it.
	case s.Class != ClassOf(s.Elected()) && s.Class != RecordOnly:
		out = append(out, fmt.Sprintf("the licence %s gives the class %s, and the source has the class %s", s.Elected(), ClassOf(s.Elected()), s.Class))
	}
	if s.Resource != "" && !tag.IsTag(s.Resource) {
		out = append(out, fmt.Sprintf("the resource %q is not a resource id", s.Resource))
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
