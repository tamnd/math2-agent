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

// Move is a file that MoveShards moved, with slash paths from the root of the tree.
type Move struct {
	From, To string
}

// MoveShards moves each file of the ShardDirs that is not in the shard of its tag to that shard, and removes the shard directories that are then empty. The tag of a file is its name without the extension. It leaves the provisional files in ~ and the files with a name that is not a tag. It is safe to run again: a tree with no file in the wrong shard gives no moves. It stops at the first file that has a file at its new path, and the moves before that one stay done.
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
	slices.SortFunc(moves, func(a, b Move) int { return strings.Compare(a.From, b.From) })
	return moves, nil
}

// move renames one file, and makes its new shard directory when it is not there.
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
