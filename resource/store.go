package resource

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// WriteResources writes the resources to records/resources/ in the tree at root. It replaces each shard file that has a resource, and it removes the shard files that have no resource now. Two resources with the same id are an error.
func WriteResources(root string, rs []Resource) error {
	shards := map[string][]Resource{}
	seen := map[string]bool{}
	for _, r := range rs {
		if len(r.ID) < 3 {
			return fmt.Errorf("resource: the id %q is too short", r.ID)
		}
		if seen[r.ID] {
			return fmt.Errorf("resource: two resources have the id %s", r.ID)
		}
		seen[r.ID] = true
		k := Shard(r.ID)
		shards[k] = append(shards[k], r)
	}
	files := map[string][]byte{}
	for k, part := range shards {
		slices.SortFunc(part, func(a, b Resource) int { return cmp.Compare(a.ID, b.ID) })
		data, err := lines(part)
		if err != nil {
			return err
		}
		files[k+".jsonl"] = data
	}
	return replaceShards(filepath.Join(root, Dir), files)
}

// ReadResources reads the resources of records/resources/ in the tree at root, in the order of their ids. A tree with no catalogue gives no resource and no error.
func ReadResources(root string) ([]Resource, error) {
	var out []Resource
	err := WalkResources(root, func(_ string, _ int, r Resource) error {
		out = append(out, r)
		return nil
	})
	slices.SortFunc(out, func(a, b Resource) int { return cmp.Compare(a.ID, b.ID) })
	return out, err
}

// WalkResources calls fn for each resource of records/resources/ in the tree at root, in the order of the files and their lines. It gives fn the file, relative to root and with forward slashes, and the line of the resource. It reads only the shard files, not the files of cites/ and structure/. A field that Resource does not have is an error.
func WalkResources(root string, fn func(file string, line int, r Resource) error) error {
	dir := filepath.Join(root, Dir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !IsShard(e.Name()) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		rel := filepath.ToSlash(filepath.Join(Dir, e.Name()))
		data, err := os.ReadFile(path) // #nosec G304 -- the caller owns the tree.
		if err != nil {
			return err
		}
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(nil, 64<<20)
		for n := 1; sc.Scan(); n++ {
			if len(bytes.TrimSpace(sc.Bytes())) == 0 {
				continue
			}
			r, err := Parse(sc.Bytes())
			if err != nil {
				return fmt.Errorf("%s:%d: %w", rel, n, err)
			}
			if err := fn(rel, n, r); err != nil {
				return err
			}
		}
		if err := sc.Err(); err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
	}
	return nil
}

// Parse reads one resource from one JSON line. A field that Resource does not have is an error.
func Parse(line []byte) (Resource, error) {
	var r Resource
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return Resource{}, err
	}
	return r, nil
}

// WriteCites writes the cites edges to records/resources/cites/ in the tree at root, in shards by the citing resource. It replaces each shard file of the directory.
func WriteCites(root string, cs []Cite) error {
	shards := map[string][]Cite{}
	for _, c := range cs {
		id, _, _ := strings.Cut(c.From, "/")
		if len(id) < 3 {
			return fmt.Errorf("resource: the cite from %q has no resource id", c.From)
		}
		shards[Shard(id)] = append(shards[Shard(id)], c)
	}
	files := map[string][]byte{}
	for k, part := range shards {
		slices.SortFunc(part, func(a, b Cite) int { return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.N, b.N)) })
		data, err := lines(part)
		if err != nil {
			return err
		}
		files[k+".jsonl"] = data
	}
	return replaceShards(filepath.Join(root, CitesDir), files)
}

// ReadStructure reads the structure of the expression <id>/<key> from the tree at root. It returns nil and no error when the expression has no structure file.
func ReadStructure(root, id, key string) (*Structure, error) {
	data, err := os.ReadFile(filepath.Join(root, StructureDir, id, key+".json")) // #nosec G304 -- the caller owns the tree.
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s Structure
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("%s/%s/%s.json: %w", StructureDir, id, key, err)
	}
	return &s, nil
}

// WriteStructure writes the structure to records/resources/structure/<id>/<key>.json in the tree at root.
func WriteStructure(root string, s *Structure) error {
	id, key, ok := strings.Cut(s.Resource, "/")
	if !ok || len(id) < 3 || key == "" {
		return fmt.Errorf("resource: the structure names %q, not <id>/<expression>", s.Resource)
	}
	data, err := encode(s)
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(root, StructureDir, id, key+".json"), append(data, '\n'))
}

// lines writes each value as one JSON line.
func lines[T any](vs []T) ([]byte, error) {
	var buf bytes.Buffer
	for _, v := range vs {
		data, err := encode(v)
		if err != nil {
			return nil, err
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// replaceShards writes the files to dir and removes the other .jsonl files of dir. It does not touch the subdirectories.
func replaceShards(dir string, files map[string][]byte) error {
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && IsShard(e.Name()) && files[e.Name()] == nil {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	for name, data := range files {
		if err := writeFile(filepath.Join(dir, name), data); err != nil {
			return err
		}
	}
	return nil
}

// IsShard reports whether a file of records/resources/ is a shard of resources: three characters of an id, or four for a split shard (RS12.1), and .jsonl. The registers beside the shards, as decisions.jsonl and venues.jsonl, have longer names.
func IsShard(name string) bool {
	base, ok := strings.CutSuffix(name, ".jsonl")
	return ok && (len(base) == 3 || len(base) == 4)
}

// writeFile writes the file through a temporary file in the same directory, so a reader never sees half a file.
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
	if err := os.Chmod(tmp.Name(), 0o644); err != nil { // #nosec G302 -- records are public text in a git tree.
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
