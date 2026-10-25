package check

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// ReadTree reads each node file below root/nodes, in path order. The path of each File is relative to root. A tree with no nodes directory gives no files and no error.
func ReadTree(root string) ([]File, error) {
	dir := filepath.Join(root, "nodes")
	var files []File
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && path == dir {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G304 G122 -- the path comes from a walk of the working tree, which the caller owns.
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, File{Path: filepath.ToSlash(rel), Data: data})
		return nil
	})
	return files, err
}
