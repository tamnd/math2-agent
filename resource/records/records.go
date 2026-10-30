// Package records moves the full catalogue of resources to the repository tamnd/math2-records (RS12.3). Export writes each shard of records/resources/, of its cites/ and of its structure/ to a second tree, compressed with zstd, with an index of the shards and a Parquet file of the resources. Keep removes from the math2 tree each resource that is not in the tiers T1, T2 and T5, and the cites of those resources. WritePin writes the pin of the commit of math2-records to the math2 tree. The id registers stay in the math2 tree.
package records

import (
	"bytes"
	"cmp"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/parquet-go/parquet-go"
	"github.com/zeebo/blake3"
	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/depth"
)

// The files of the records tree and of the math2 tree.
const (
	Repo        = "tamnd/math2-records"
	Ext         = ".zst"
	IndexFile   = "records/resources/index.md"
	ParquetFile = "records/resources/resources.parquet"
	PinFile     = "manifests/records.yaml"
	Version     = "records/1"
)

// Kept are the tiers whose records stay in the math2 tree (RS12.3).
var Kept = []string{depth.T1, depth.T2, depth.T5}

// Shard is one compressed shard of the records tree.
type Shard struct {
	File  string `json:"file"`  // the path in the records tree, with forward slashes
	Hash  string `json:"hash"`  // the blake3 hash of the shard before compression
	Bytes int    `json:"bytes"` // the size of the shard before compression
}

// Row is one row of ParquetFile.
type Row struct {
	ID     string `parquet:"id"`
	Kind   string `parquet:"kind"`
	Title  string `parquet:"title"`
	Date   string `parquet:"date,optional"`
	Scope  string `parquet:"scope,optional"`
	Tier   string `parquet:"tier,optional"`
	MSC    string `parquet:"msc,optional"` // the first class
	DOI    string `parquet:"doi,optional"`
	ISBN   string `parquet:"isbn,optional"`
	ArXiv  string `parquet:"arxiv,optional"`
	Zbl    string `parquet:"zbl,optional"`
	Groups int    `parquet:"catalogue"` // the count of the catalogue records
}

// Dirs are the directories of the math2 tree that Export reads.
var Dirs = []string{resource.Dir, resource.CitesDir, resource.StructureDir}

// Export writes the shards of the math2 tree at root to the records tree at out, compressed with zstd, with IndexFile and ParquetFile. It replaces the shards of out that the math2 tree no longer has. It gives the shards in the order of their files.
func Export(root, out string) ([]Shard, error) {
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBetterCompression))
	if err != nil {
		return nil, err
	}
	defer func() { _ = enc.Close() }()
	var shards []Shard
	for _, dir := range Dirs {
		files, err := mdrec.Glob(filepath.Join(root, dir, "*"+mdrec.Ext))
		if err != nil {
			return nil, err
		}
		keep := map[string]bool{}
		for _, path := range files {
			name := filepath.Base(path)
			if dir != resource.StructureDir && !resource.IsShard(name) {
				continue
			}
			data, err := os.ReadFile(path) // #nosec G304 -- the caller owns the tree.
			if err != nil {
				return nil, err
			}
			rel := filepath.ToSlash(filepath.Join(dir, name)) + Ext
			if err := put(filepath.Join(out, filepath.FromSlash(rel)), enc.EncodeAll(data, nil)); err != nil {
				return nil, err
			}
			keep[filepath.Base(rel)] = true
			shards = append(shards, Shard{File: rel, Hash: hash(data), Bytes: len(data)})
		}
		if err := prune(filepath.Join(out, dir), keep); err != nil {
			return nil, err
		}
	}
	slices.SortFunc(shards, func(a, b Shard) int { return cmp.Compare(a.File, b.File) })
	recs, err := mdrec.Marshal(shards)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(out, IndexFile)), 0o750); err != nil {
		return nil, err
	}
	if err := mdrec.Write(filepath.Join(out, IndexFile), recs); err != nil {
		return nil, err
	}
	rows, err := Rows(root)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := parquet.Write(&b, rows, parquet.CreatedBy("m2", Version, "")); err != nil {
		return nil, fmt.Errorf("records: write parquet: %w", err)
	}
	return shards, put(filepath.Join(out, ParquetFile), b.Bytes())
}

// Rows gives a row of ParquetFile for each resource of the math2 tree at root, with the tier of build/resources/depth.md.
func Rows(root string) ([]Row, error) {
	tiers, err := depth.Read(root)
	if err != nil {
		return nil, err
	}
	var rows []Row
	err = resource.WalkResources(root, func(_ string, _ int, r resource.Resource) error {
		row := Row{ID: r.ID, Kind: r.Kind, Title: r.Title, Date: r.Date, Tier: tiers[r.ID].Tier, Groups: len(r.Catalogue)}
		if r.Scope != nil {
			row.Scope = r.Scope.Value
		}
		if len(r.MSC) > 0 {
			row.MSC = r.MSC[0].Code
		}
		row.DOI, row.ISBN, row.ArXiv, row.Zbl = one(r.IDs["doi"]), one(r.IDs["isbn"]), one(r.IDs["arxiv"]), one(r.IDs["zbl"])
		rows = append(rows, row)
		return nil
	})
	slices.SortFunc(rows, func(a, b Row) int { return cmp.Compare(a.ID, b.ID) })
	return rows, err
}

// ReadShard reads one shard of the records tree at out and checks its hash against the shard of IndexFile. It gives the records of the shard.
func ReadShard(out string, s Shard) ([][]byte, error) {
	f, err := os.Open(filepath.Join(out, filepath.FromSlash(s.File))) // #nosec G304 -- the caller owns the tree.
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	dec, err := zstd.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	data, err := io.ReadAll(dec)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.File, err)
	}
	if h := hash(data); h != s.Hash {
		return nil, fmt.Errorf("%s: the hash is %s, and the index gives %s", s.File, h, s.Hash)
	}
	return mdrec.Decode(data)
}

// ReadIndex reads IndexFile of the records tree at out.
func ReadIndex(out string) ([]Shard, error) {
	recs, err := mdrec.Read(filepath.Join(out, IndexFile))
	if err != nil {
		return nil, err
	}
	return mdrec.Unmarshal[Shard](recs)
}

// Keep removes from the math2 tree at root each resource whose tier is not in Kept, and the cites from those resources. It needs build/resources/depth.md, which m2 res status writes, because a resource with no row would go. It gives the counts of the resources that stay and that go.
func Keep(root string) (kept, moved int, err error) {
	tiers, err := depth.Read(root)
	if err != nil {
		return 0, 0, err
	}
	if len(tiers) == 0 {
		return 0, 0, fmt.Errorf("records: %s has no rows. Run m2 res status first", depth.File)
	}
	rs, err := resource.ReadResources(root)
	if err != nil {
		return 0, 0, err
	}
	stay := map[string]bool{}
	var out []resource.Resource
	for _, r := range rs {
		if slices.Contains(Kept, tiers[r.ID].Tier) {
			stay[r.ID] = true
			out = append(out, r)
		}
	}
	cs, err := resource.ReadCites(root)
	if err != nil {
		return 0, 0, err
	}
	cs = slices.DeleteFunc(cs, func(c resource.Cite) bool {
		id, _, _ := strings.Cut(c.From, "/")
		return !stay[id]
	})
	if err := resource.WriteResources(root, out); err != nil {
		return 0, 0, err
	}
	if err := resource.WriteCites(root, cs); err != nil {
		return 0, 0, err
	}
	return len(out), len(rs) - len(out), nil
}

// Pin is the pin of the records tree in the math2 tree (PinFile).
type Pin struct {
	Repo   string `yaml:"repo"`
	Commit string `yaml:"commit"`
	Index  string `yaml:"index"`  // the blake3 hash of IndexFile at the commit
	Shards int    `yaml:"shards"` // the count of shards of IndexFile
}

// WritePin writes PinFile to the math2 tree at root, with the commit of the records tree at out.
func WritePin(root, out, commit string) (Pin, error) {
	if len(commit) < 7 || strings.Trim(commit, "0123456789abcdef") != "" {
		return Pin{}, fmt.Errorf("records: the commit %q is not a hex commit id", commit)
	}
	data, err := os.ReadFile(filepath.Join(out, IndexFile)) // #nosec G304 -- the caller owns the tree.
	if err != nil {
		return Pin{}, err
	}
	shards, err := ReadIndex(out)
	if err != nil {
		return Pin{}, err
	}
	p := Pin{Repo: Repo, Commit: commit, Index: hash(data), Shards: len(shards)}
	b, err := yaml.Marshal(p)
	if err != nil {
		return Pin{}, err
	}
	return p, put(filepath.Join(root, PinFile), b)
}

// ReadPin reads PinFile of the math2 tree at root. A tree with no pin gives false.
func ReadPin(root string) (Pin, bool, error) {
	data, err := os.ReadFile(filepath.Join(root, PinFile)) // #nosec G304 -- the caller owns the tree.
	if errors.Is(err, fs.ErrNotExist) {
		return Pin{}, false, nil
	}
	if err != nil {
		return Pin{}, false, err
	}
	var p Pin
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return Pin{}, false, fmt.Errorf("%s: %w", PinFile, err)
	}
	return p, true, nil
}

// one gives the first value, or empty.
func one(v resource.Values) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

func hash(data []byte) string {
	h := blake3.Sum256(data)
	return "blake3:" + hex.EncodeToString(h[:])
}

func put(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644) // #nosec G306 G703 -- the records are public text in a git tree, and the caller owns the tree.
}

// prune removes the compressed shards of dir that are not in keep. It does not touch the subdirectories.
func prune(dir string, keep map[string]bool) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), Ext) && !keep[e.Name()] {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
