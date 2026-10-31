package arxiv

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/mdrec"
)

// Reference is one reference of the copy check against the arXiv corpus (AX9.5): the text of a paper of the content plane, or the MinHash of an item of the latest version of a paper. The class is the licence class of the version (AX2.3).
type Reference struct {
	URI     string // arxiv:ID for a text, ax://paper/ID#TAG for an item
	Licence string
	Class   string
	Text    string
	MinHash string
}

// References reads the references of the copy check from the checkout at root: the text of each paper of the content plane, content/LANG/YYMM/ID/*.md, and the MinHash of each item with a tag of the latest version of each paper. The MinHash is the only trace of the text of a closed version, and it does not give the text back.
func References(root string) ([]Reference, error) {
	files, err := mdrec.Glob(filepath.Join(root, "analysis", "objects", "*", "*"+mdrec.Ext))
	if err != nil {
		return nil, err
	}
	months := map[string]bool{}
	for _, f := range files {
		months[filepath.Base(filepath.Dir(f))] = true
	}
	dirs, err := filepath.Glob(filepath.Join(root, "content", "*", "*", "*"))
	if err != nil {
		return nil, err
	}
	for _, d := range dirs {
		months[filepath.Base(filepath.Dir(d))] = true
	}
	meta, err := readMetadata(root, months)
	if err != nil {
		return nil, err
	}
	licence := func(id string, v int) string {
		if m := meta[id]; m != nil {
			if v == 0 {
				for _, x := range m.Versions {
					v = max(v, x.Version)
				}
			}
			return m.Licence(v)
		}
		return ""
	}
	var out []Reference
	for _, d := range dirs {
		id := filepath.Base(d)
		if dash := strings.LastIndex(id, "-"); dash > 0 && !strings.Contains(id, ".") {
			id = id[:dash] + "/" + id[dash+1:] // the path form of an old id, as math-9802134
		}
		parts, err := filepath.Glob(filepath.Join(d, "*.md"))
		if err != nil {
			return nil, err
		}
		slices.Sort(parts)
		var b strings.Builder
		for _, p := range parts {
			data, err := os.ReadFile(p) // #nosec G304 -- the file is in the checkout that the caller names
			if err != nil {
				return nil, err
			}
			b.Write(data)
			b.WriteString("\n\n")
		}
		if b.Len() > 0 {
			l := licence(id, 0)
			out = append(out, Reference{URI: "arxiv:" + id, Licence: l, Class: ClassOf(l), Text: b.String()})
		}
	}
	for _, f := range files {
		objs, err := readObjects(f, nil)
		if err != nil {
			return nil, err
		}
		latest, v := Latest(objs)
		for _, o := range latest {
			if o.Tag != "" && o.MinHash != "" {
				l := licence(o.Paper, v)
				out = append(out, Reference{URI: MentionID(o.Paper, o.Tag), Licence: l, Class: ClassOf(l), MinHash: o.MinHash})
			}
		}
	}
	slices.SortStableFunc(out, func(a, b Reference) int { return cmp.Compare(a.URI, b.URI) })
	return out, nil
}
