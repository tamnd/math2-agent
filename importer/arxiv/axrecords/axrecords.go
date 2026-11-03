// Package axrecords moves the arXiv records of the record plane to the repository tamnd/math2-records (corpus arxiv AQ11). The records are the mentions of records/mentions/arxiv/, the records of the passes of understand in records/understand/arxiv/, and the rows of the research layer in records/inventory/. Export writes them to the records tree, compressed with zstd, with an index of the shards. Keep removes them from the math2 tree. WritePin writes the pin of the commit of math2-records to the math2 tree. Unpack puts them back in a math2 tree, so that an import, a clustering or a build can read them. The id registers, the gold sets and the decisions of the review queues stay in the math2 tree.
package axrecords

import (
	"bytes"
	"cmp"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/zeebo/blake3"
	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/mdrec"
)

// The files of the records tree and of the math2 tree.
const (
	Repo      = "tamnd/math2-records"
	Source    = "arxiv"
	Ext       = ".zst"
	Dir       = "records/arxiv"
	IndexFile = Dir + "/index.md"
	RowsDir   = Dir + "/rows"
)

// PinFile is the pin of the records tree in the math2 tree.
var PinFile = inventory.PackedPins[Source]

// The directories of the math2 tree whose files move whole: the mentions of arXiv and the records of understand.
var (
	mentionsDir   = inventory.MentionsDir + "/" + Source
	understandDir = agent.UnderstandDir
)

// Shard is one compressed shard of the records tree.
type Shard struct {
	File  string `json:"file"`  // the path in the records tree, with forward slashes
	Hash  string `json:"hash"`  // the blake3 hash of the shard before compression
	Bytes int    `json:"bytes"` // the size of the shard before compression
}

// Pin is the pin of the records tree in the math2 tree (PinFile).
type Pin struct {
	Repo   string `yaml:"repo"`
	Commit string `yaml:"commit"`
	Index  string `yaml:"index"`  // the blake3 hash of IndexFile at the commit
	Shards int    `yaml:"shards"` // the count of shards of IndexFile
}

// files gives the files of the math2 tree at root that move whole, by their path in the records tree, with the data of each.
func files(root string) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, d := range []struct{ from, to string }{{mentionsDir, Dir + "/mentions"}, {understandDir, Dir + "/understand"}} {
		base := filepath.Join(root, filepath.FromSlash(d.from))
		err := filepath.WalkDir(base, func(p string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if e.IsDir() || filepath.Ext(p) != ".jsonl" {
				return nil
			}
			rel, err := filepath.Rel(base, p)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(p) // #nosec G304 G122 -- the path comes from a walk of a tree that the caller owns.
			if err != nil {
				return err
			}
			out[d.to+"/"+filepath.ToSlash(rel)] = data
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return out, nil
}

// research splits the rows of the math2 tree at root into the rows of the research layer and the others.
func research(root string) (res, other []inventory.Row, err error) {
	rows, err := inventory.ReadRows(root)
	if err != nil {
		return nil, nil, err
	}
	for _, r := range rows {
		if r.Layer == inventory.Research {
			res = append(res, r)
		} else {
			other = append(other, r)
		}
	}
	return res, other, nil
}

// Export writes the arXiv records of the math2 tree at root to the records tree at out, compressed with zstd, with IndexFile. It replaces the shards of out under Dir that the math2 tree no longer has. It refuses a math2 tree whose arXiv records are packed, because it would empty the records tree. It gives the shards in the order of their files.
func Export(root, out string) ([]Shard, error) {
	if slices.Contains(inventory.Packed(root), Source) {
		return nil, fmt.Errorf("axrecords: the tree has no arXiv records, only %s. Run m2 corpus records unpack first", PinFile)
	}
	all, err := files(root)
	if err != nil {
		return nil, err
	}
	res, _, err := research(root)
	if err != nil {
		return nil, err
	}
	by := map[string][]inventory.Row{}
	for _, r := range res {
		k := inventory.RowShard(r.ID)
		by[k] = append(by[k], r)
	}
	for k, rows := range by {
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		for _, r := range rows {
			if err := enc.Encode(r); err != nil {
				return nil, err
			}
		}
		all[RowsDir+"/"+k+".jsonl"] = b.Bytes()
	}
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBetterCompression))
	if err != nil {
		return nil, err
	}
	defer func() { _ = enc.Close() }()
	var shards []Shard
	keep := map[string]bool{}
	for rel, data := range all {
		file := rel + Ext
		p := filepath.Join(out, filepath.FromSlash(file))
		z := enc.EncodeAll(data, nil)
		if cur, err := os.ReadFile(p); err != nil || !bytes.Equal(cur, z) { // #nosec G304 -- the caller owns the tree.
			if err := put(p, z); err != nil {
				return nil, err
			}
		}
		keep[file] = true
		shards = append(shards, Shard{File: file, Hash: hash(data), Bytes: len(data)})
	}
	if err := prune(out, keep); err != nil {
		return nil, err
	}
	slices.SortFunc(shards, func(a, b Shard) int { return cmp.Compare(a.File, b.File) })
	recs, err := mdrec.Marshal(shards)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(out, filepath.FromSlash(Dir)), 0o750); err != nil {
		return nil, err
	}
	return shards, mdrec.Write(filepath.Join(out, filepath.FromSlash(IndexFile)), recs)
}

// Keep removes the arXiv records from the math2 tree at root: the mentions of arXiv, the records of understand and the rows of the research layer. It needs PinFile, so that the tree names where the records are. It gives the counts of the files and of the rows that it removes.
func Keep(root string) (moved, rows int, err error) {
	if _, ok, err := ReadPin(root); err != nil || !ok {
		if err == nil {
			err = fmt.Errorf("axrecords: the tree has no %s. Run m2 corpus records pin first", PinFile)
		}
		return 0, 0, err
	}
	all, err := files(root)
	if err != nil {
		return 0, 0, err
	}
	res, other, err := research(root)
	if err != nil {
		return 0, 0, err
	}
	if len(res) > 0 {
		if err := inventory.KeepRows(root, other); err != nil {
			return 0, 0, err
		}
	}
	for _, d := range []string{mentionsDir, understandDir} {
		// The paths are constants of the package below root.
		if err := os.RemoveAll(filepath.Join(root, filepath.FromSlash(d))); err != nil {
			return 0, 0, err
		}
	}
	return len(all), len(res), nil
}

// ReadIndex reads IndexFile of the records tree at out.
func ReadIndex(out string) ([]Shard, error) {
	recs, err := mdrec.Read(filepath.Join(out, filepath.FromSlash(IndexFile)))
	if err != nil {
		return nil, err
	}
	return mdrec.Unmarshal[Shard](recs)
}

// ReadShard reads one shard of the records tree at out and checks its hash against the shard of IndexFile.
func ReadShard(out string, s Shard) ([]byte, error) {
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
	return data, nil
}

// WritePin writes PinFile to the math2 tree at root, with the commit of the records tree at out.
func WritePin(root, out, commit string) (Pin, error) {
	if len(commit) < 7 || strings.Trim(commit, "0123456789abcdef") != "" {
		return Pin{}, fmt.Errorf("axrecords: the commit %q is not a hex commit id", commit)
	}
	data, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(IndexFile))) // #nosec G304 -- the caller owns the tree.
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
	return p, put(filepath.Join(root, filepath.FromSlash(PinFile)), b)
}

// ReadPin reads PinFile of the math2 tree at root. A tree with no pin gives false.
func ReadPin(root string) (Pin, bool, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(PinFile))) // #nosec G304 -- the caller owns the tree.
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

// Unpack puts the arXiv records of the records tree at out back in the math2 tree at root: the files of the mentions and of understand, and the rows of the research layer that the tree does not have. When the tree has a pin, the index of out must have the hash of the pin. A tree that has its arXiv records already gets nothing, so Unpack can run twice. It gives the counts of the files and of the rows that it adds.
func Unpack(root, out string) (added, rows int, err error) {
	if !slices.Contains(inventory.Packed(root), Source) {
		return 0, 0, nil
	}
	data, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(IndexFile))) // #nosec G304 -- the caller owns the tree.
	if err != nil {
		return 0, 0, err
	}
	p, ok, err := ReadPin(root)
	if err != nil {
		return 0, 0, err
	}
	if ok && hash(data) != p.Index {
		return 0, 0, fmt.Errorf("axrecords: the index of %s has the hash %s, and %s gives %s. Check out the commit %s of %s", out, hash(data), PinFile, p.Index, p.Commit, p.Repo)
	}
	shards, err := ReadIndex(out)
	if err != nil {
		return 0, 0, err
	}
	have, err := inventory.ReadRows(root)
	if err != nil {
		return 0, 0, err
	}
	ids := map[string]bool{}
	for _, r := range have {
		ids[r.ID] = true
	}
	type move struct {
		path string
		data []byte
	}
	var moves []move
	for _, s := range shards {
		data, err := ReadShard(out, s)
		if err != nil {
			return 0, 0, err
		}
		rel := strings.TrimSuffix(s.File, Ext)
		switch {
		case path.Dir(rel) == RowsDir:
			for line := range bytes.Lines(data) {
				var r inventory.Row
				if err := json.Unmarshal(line, &r); err != nil {
					return 0, 0, fmt.Errorf("%s: %w", s.File, err)
				}
				if !ids[r.ID] {
					ids[r.ID] = true
					have = append(have, r)
					rows++
				}
			}
		case strings.HasPrefix(rel, Dir+"/mentions/"):
			moves = append(moves, move{path.Join(mentionsDir, strings.TrimPrefix(rel, Dir+"/mentions/")), data})
		case strings.HasPrefix(rel, Dir+"/understand/"):
			moves = append(moves, move{path.Join(understandDir, strings.TrimPrefix(rel, Dir+"/understand/")), data})
		default:
			return 0, 0, fmt.Errorf("axrecords: %s gives the shard %s, which is in no directory of the arXiv records", IndexFile, s.File)
		}
	}
	// The understand records go first, so that a failure leaves the tree packed: Packed looks at the mentions.
	slices.SortStableFunc(moves, func(a, b move) int {
		last := func(m move) int {
			if strings.HasPrefix(m.path, mentionsDir+"/") {
				return 1
			}
			return 0
		}
		return cmp.Compare(last(a), last(b))
	})
	if rows > 0 {
		if err := inventory.KeepRows(root, have); err != nil {
			return 0, 0, err
		}
	}
	for _, m := range moves {
		if err := put(filepath.Join(root, filepath.FromSlash(m.path)), m.data); err != nil {
			return 0, 0, err
		}
	}
	return len(moves), rows, nil
}

func hash(data []byte) string {
	h := blake3.Sum256(data)
	return "blake3:" + hex.EncodeToString(h[:])
}

func put(p string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644) // #nosec G306 G703 -- the records are public text in a git tree, and the caller owns the tree.
}

// prune removes the compressed shards below Dir of the records tree at out that are not in keep.
func prune(out string, keep map[string]bool) error {
	base := filepath.Join(out, filepath.FromSlash(Dir))
	err := filepath.WalkDir(base, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() || !strings.HasSuffix(p, Ext) {
			return nil
		}
		rel, err := filepath.Rel(out, p)
		if err != nil {
			return err
		}
		if !keep[filepath.ToSlash(rel)] {
			return os.Remove(p) // #nosec G122 -- the path comes from a walk of the output directory that the caller names.
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
