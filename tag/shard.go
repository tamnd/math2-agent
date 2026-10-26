package tag

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// ShardDirs are the directories of the tree that keep one file for each tag, in a shard directory below them.
var ShardDirs = []string{"nodes", "prov", "reviews"}

// CodeDir returns the directory of the code of the computation with the tag or the provisional id, compute/SHARD/ID, as a slash path (corpus spec K3.5). The shard is the shard of the node file.
func CodeDir(id string) string { return "compute/" + Shard(id) + "/" + id }

// CodeID returns the id of the computation that has the file at the slash path p in its code. It reports false when p is not in a directory CodeDir(ID) for a tag or a provisional id ID.
func CodeID(p string) (string, bool) {
	rest, ok := strings.CutPrefix(p, "compute/")
	if !ok {
		return "", false
	}
	shard, rest, _ := strings.Cut(rest, "/")
	id, _, ok := strings.Cut(rest, "/")
	if !ok || !IsTag(id) && !IsProvisional(id) || Shard(id) != shard {
		return "", false
	}
	return id, true
}

// Move is a file that MoveShards moved, with slash paths from the root of the tree.
type Move struct {
	From, To string
}

// MoveShards moves each file of the ShardDirs that is not in the shard of its tag to that shard, and removes the shard directories that are then empty. The tag of a file is its name without the extension. It leaves the provisional files in ~ and the files with a name that is not a tag. It also moves the code of each computation that is not in CodeDir of its id to that directory. A move of code is one move of the directory. It is safe to run again: a tree with no file in the wrong shard gives no moves. It stops at the first file that has a file at its new path, and the moves before that one stay done.
func MoveShards(root string) ([]Move, error) {
	var moves []Move
	for _, top := range ShardDirs {
		dirs, err := os.ReadDir(filepath.Join(root, top))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return moves, err
		}
		for _, d := range dirs {
			if !d.IsDir() || d.Name() == "~" {
				continue
			}
			dir := filepath.Join(root, top, d.Name())
			files, err := os.ReadDir(dir)
			if err != nil {
				return moves, err
			}
			for _, f := range files {
				id := strings.TrimSuffix(f.Name(), path.Ext(f.Name()))
				if f.IsDir() || !IsTag(id) || Shard(id) == d.Name() {
					continue
				}
				m := Move{From: path.Join(top, d.Name(), f.Name()), To: path.Join(top, Shard(id), f.Name())}
				if err := move(root, m); err != nil {
					return moves, err
				}
				moves = append(moves, m)
			}
			if left, err := os.ReadDir(dir); err == nil && len(left) == 0 {
				if err := os.Remove(dir); err != nil {
					return moves, err
				}
			}
		}
	}
	code, err := moveCode(root)
	moves = append(moves, code...)
	if err != nil {
		return moves, err
	}
	slices.SortFunc(moves, func(a, b Move) int { return strings.Compare(a.From, b.From) })
	return moves, nil
}

// moveCode moves the code of each computation in compute/ that is not in CodeDir of its id. The code is in compute/ID/ in a tree from before the shards, or in a shard directory that is not the shard of ID.
func moveCode(root string) ([]Move, error) {
	var moves []Move
	dirs, err := os.ReadDir(filepath.Join(root, "compute"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		if id := d.Name(); IsTag(id) || IsProvisional(id) {
			m := Move{From: "compute/" + id, To: CodeDir(id)}
			if err := move(root, m); err != nil {
				return moves, err
			}
			moves = append(moves, m)
			continue
		}
		dir := filepath.Join(root, "compute", d.Name())
		ids, err := os.ReadDir(dir)
		if err != nil {
			return moves, err
		}
		for _, c := range ids {
			id := c.Name()
			if !c.IsDir() || !IsTag(id) && !IsProvisional(id) || Shard(id) == d.Name() {
				continue
			}
			m := Move{From: path.Join("compute", d.Name(), id), To: CodeDir(id)}
			if err := move(root, m); err != nil {
				return moves, err
			}
			moves = append(moves, m)
		}
		if left, err := os.ReadDir(dir); err == nil && len(left) == 0 {
			if err := os.Remove(dir); err != nil {
				return moves, err
			}
		}
	}
	return moves, nil
}

// move renames one file or one directory, and makes its new shard directory when it is not there.
func move(root string, m Move) error {
	to := filepath.Join(root, filepath.FromSlash(m.To))
	if _, err := os.Lstat(to); err == nil {
		return fmt.Errorf("%s and %s have the same tag", m.From, m.To)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o750); err != nil {
		return err
	}
	return os.Rename(filepath.Join(root, filepath.FromSlash(m.From)), to)
}
