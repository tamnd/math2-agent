package ingest

import (
	"bufio"
	"bytes"
	"cmp"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/zeebo/blake3"
	"go.yaml.in/yaml/v3"
)

// Record is one item of a source: a paper, a sequence, a tag, a problem. An adapter gives the fields that it reads from the snapshot. The licence gate sets Class and Fingerprints, and it drops Text when the class does not let math2 store it.
type Record struct {
	ID           string            `json:"id"`                     // the id in the source, as in sources: and same_as, for example A000045
	Kind         string            `json:"kind,omitempty"`         // the kind of item in the source, for example sequence
	Title        string            `json:"title,omitempty"`        // the title or the name
	URL          string            `json:"url,omitempty"`          // the home of the item
	Locator      string            `json:"locator,omitempty"`      // where the item is in a larger work
	Licence      string            `json:"licence,omitempty"`      // the licence of the item, when it is not the licence of the source
	Class        Class             `json:"class"`                  // the access class of the item
	Text         string            `json:"text,omitempty"`         // the text, only when the class lets math2 store it
	Fingerprints []uint32          `json:"fingerprints,omitempty"` // the fingerprints of the text
	Meta         map[string]string `json:"meta,omitempty"`         // other fields of the item
}

// Gate applies the licence gate of spec 09.3 to r, an item of the source src. The class of r is the class of its own licence when it has one, and the class of src when it has none. Gate sets the fingerprints of the text, and it drops the text when the class does not let math2 store it.
func Gate(src *Source, r Record) Record {
	r.Class = src.Class
	if r.Licence != "" && r.Licence != src.Licence {
		r.Class = ClassOf(r.Licence)
	} else {
		r.Licence = ""
	}
	r.Fingerprints = Fingerprints(r.Text)
	if !r.Class.StoresText() {
		r.Text = ""
	}
	return r
}

// Import reads the snapshot with the adapter, applies the licence gate to each item and returns the records in the order of their ids. Each item must have an id, and no two items can have the same id. When the snapshot is compressed with gzip, the adapter reads the uncompressed bytes.
func Import(a Adapter, src *Source, snapshot io.Reader) ([]Record, error) {
	snapshot, err := gunzip(snapshot)
	if err != nil {
		return nil, fmt.Errorf("ingest %s: %w", src.ID, err)
	}
	var out []Record
	seen := map[string]bool{}
	err = a.Read(snapshot, func(r Record) error {
		if r.ID == "" {
			return fmt.Errorf("item %d has no id", len(out)+1)
		}
		if seen[r.ID] {
			return fmt.Errorf("two items have the id %s", r.ID)
		}
		seen[r.ID] = true
		out = append(out, Gate(src, r))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ingest %s: %w", src.ID, err)
	}
	slices.SortFunc(out, func(a, b Record) int { return cmp.Compare(a.ID, b.ID) })
	return out, nil
}

// gunzip returns a reader of the uncompressed bytes of r when r starts with the gzip magic number, and a reader of the bytes of r when it does not.
func gunzip(r io.Reader) (io.Reader, error) {
	br := bufio.NewReader(r)
	magic, err := br.Peek(2)
	if err != nil || magic[0] != 0x1f || magic[1] != 0x8b {
		return br, nil // a short snapshot is not compressed, and the adapter reports a read error
	}
	return gzip.NewReader(br)
}

// WriteRecords writes the records to w, one JSON object for each line, and returns the hash of the bytes.
func WriteRecords(w io.Writer, recs []Record) (string, error) {
	h := blake3.New()
	bw := bufio.NewWriter(io.MultiWriter(w, h))
	enc := json.NewEncoder(bw)
	enc.SetEscapeHTML(false)
	for _, r := range recs {
		if err := enc.Encode(r); err != nil {
			return "", err
		}
	}
	if err := bw.Flush(); err != nil {
		return "", err
	}
	return "blake3:" + hex.EncodeToString(h.Sum(nil)), nil
}

// ReadRecords reads the records of the source with the id from build/records/<id>.jsonl in the tree at root. It also returns the hash of the file.
func ReadRecords(root, id string) ([]Record, string, error) {
	data, err := os.ReadFile(filepath.Join(root, RecordsDir, id+".jsonl")) // #nosec G304 -- the caller owns the tree.
	if err != nil {
		return nil, "", err
	}
	sum := blake3.Sum256(data)
	var out []Record
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(nil, 64<<20)
	for line := 1; sc.Scan(); line++ {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, "", fmt.Errorf("%s/%s.jsonl:%d: %w", RecordsDir, id, line, err)
		}
		out = append(out, r)
	}
	return out, "blake3:" + hex.EncodeToString(sum[:]), sc.Err()
}

// Manifest is the import manifest of one source, manifests/ingest/<source>.yaml. It names the snapshot and the adapter of the last import, and the hash of its records. Thus anyone can make the same import again and compare the result.
type Manifest struct {
	Source   string        `yaml:"source"`
	Adapter  string        `yaml:"adapter"`  // the adapter and its version, for example oeis/1
	Snapshot string        `yaml:"snapshot"` // the hash of the snapshot in the catalogue
	Records  int           `yaml:"records"`  // the number of records
	Hash     string        `yaml:"hash"`     // the hash of build/records/<source>.jsonl
	Classes  map[Class]int `yaml:"classes"`  // the number of records of each class
}

// ManifestPath returns the path of the import manifest of the source with the id, relative to the root.
func ManifestPath(id string) string {
	return ManifestDir + "/" + id + ".yaml"
}

// ReadManifest reads the import manifest at path.
func ReadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the caller owns the tree.
	if err != nil {
		return nil, err
	}
	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("the file is empty")
		}
		return nil, err
	}
	return &m, nil
}

// Write writes the manifest to manifests/ingest/<source>.yaml in the tree at root.
func (m *Manifest) Write(root string) error {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# The import manifest of %s. m2 ingest writes this file. Run m2 ingest %s -snapshot %s to make the same records again.\n", m.Source, m.Source, m.Snapshot)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return err
	}
	return writeFile(filepath.Join(root, ManifestPath(m.Source)), buf.Bytes())
}
