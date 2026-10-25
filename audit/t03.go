package audit

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tamnd/math2-agent/tag"
)

// textExts are the file types that can refer to a node.
var textExts = map[string]bool{".md": true, ".yaml": true, ".yml": true, ".lean": true}

// skipDirs are the directories that the audit does not read.
var skipDirs = map[string]bool{".git": true, ".lake": true, "node_modules": true}

// t03 checks the provisional ids in the content directories (rule T03). In a full audit, each provisional id is a finding, because the mint workflow changes each one to a tag before merge. In the audit of a pull request, a provisional id is correct when its node file is in nodes/~/. Another provisional id is from another pull request.
func t03(a *auditor) outcome {
	var o outcome
	for _, dir := range tag.ContentDirs() {
		err := filepath.WalkDir(filepath.Join(a.root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(a.root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if !textExts[filepath.Ext(path)] || !a.inScope(rel) {
				return nil
			}
			data, err := os.ReadFile(path) // #nosec G304 G122 -- the path comes from a walk of the working tree, which the caller owns.
			if err != nil {
				return err
			}
			o.checked++
			for _, m := range tag.ProvisionalInText.FindAllIndex(data, -1) {
				id := string(data[m[0]:m[1]])
				line, col := lineCol(data, m[0])
				switch {
				case !a.pr:
					o.add(rel, line, col, "%s is a provisional id. The mint workflow changes it to a tag before merge", id)
				case !a.pending(id):
					o.add(rel, line, col, "%s is a provisional id with no node file in nodes/~/. Refer to a node of another pull request after it merges, by its tag", id)
				}
			}
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			o.reason = err.Error()
			return o
		}
	}
	return o
}

// pending reports whether the provisional id has a node file in nodes/~/.
func (a *auditor) pending(id string) bool {
	_, err := os.Stat(filepath.Join(a.root, "nodes", "~", id+".md")) // #nosec G703 -- id matches ~[0-9a-f]{8}, so the path stays in nodes/~/.
	return err == nil
}

// isRef reports whether s has the form of a node reference: a tag or a provisional id.
func isRef(s string) bool { return tag.IsTag(s) || tag.IsProvisional(s) }

// trimExt returns the base name of path without its extension.
func trimExt(path string) string {
	base := path[strings.LastIndexByte(path, '/')+1:]
	return strings.TrimSuffix(base, filepath.Ext(base))
}
