package tag

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Minted is one provisional id and the tag that replaces it.
type Minted struct {
	Provisional string
	Tag         string
	Kind        string
}

// textExts are the file types that can refer to a node.
var textExts = map[string]bool{".md": true, ".yaml": true, ".yml": true, ".lean": true}

// ContentDirs returns the directories of the working tree that can refer to a node (spec 20). Text in other files, for example a provisional id in README.md, is not a reference.
func ContentDirs() []string { return []string{"nodes", "topics", "reviews", "compute", "lean"} }

// skipDirs are the directories in a content directory that Rewrite does not read.
var skipDirs = map[string]bool{".git": true, ".lake": true, "node_modules": true}

// Rewrite changes each provisional id in the math2 working tree at root to a new tag. It mints one tag for each file in nodes/~/. It changes the id in the file name, in the front matter, in each [[text|~id]] link and in each other text file in nodes/, topics/, reviews/, compute/ and lean/ that refers to it. It does not change other files, for example the README. It moves the provenance log of each node from prov/~/ to the shard of its tag. Then it adds the new tags to registers/tags.reg with the given commit.
//
// Rewrite changes nothing if a file refers to a provisional id that has no file in nodes/~/ (rule T03).
func Rewrite(root string, r io.Reader, commit string) ([]Minted, error) {
	return rewrite(root, r, commit, Mint)
}

// RewriteSeed is Rewrite with tags that start with 0, as MintSeed draws them. Use it only for the nodes of the M1 seed.
func RewriteSeed(root string, r io.Reader, commit string) ([]Minted, error) {
	return rewrite(root, r, commit, MintSeed)
}

func rewrite(root string, r io.Reader, commit string, mint func(io.Reader, func(string) bool) (string, error)) ([]Minted, error) {
	regs := filepath.Join(root, "registers")
	used, err := ReadUsed(filepath.Join(regs, "tags.reg"), filepath.Join(regs, "retired.reg"))
	if err != nil {
		return nil, err
	}
	if err := addTreeTags(root, used); err != nil {
		return nil, err
	}

	pending, err := filepath.Glob(filepath.Join(root, "nodes", "~", "~*.md"))
	if err != nil {
		return nil, err
	}
	slices.Sort(pending)
	byID := map[string]Minted{}
	var minted []Minted
	for _, path := range pending {
		id := strings.TrimSuffix(filepath.Base(path), ".md")
		if !IsProvisional(id) {
			return nil, fmt.Errorf("%s: %q is not a provisional id", path, id)
		}
		kind, err := readKind(path)
		if err != nil {
			return nil, err
		}
		t, err := mint(r, func(s string) bool { return used[s] })
		if err != nil {
			return nil, err
		}
		used[t] = true
		m := Minted{Provisional: id, Tag: t, Kind: kind}
		byID[id] = m
		minted = append(minted, m)
	}

	// Read every file that can refer to a node before writing any file.
	changed := map[string][]byte{}
	var unknown []string
	walk := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !textExts[filepath.Ext(path)] {
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G304 G122 -- the path comes from a walk of the working tree, which the caller owns.
		if err != nil {
			return err
		}
		out := ProvisionalInText.ReplaceAllFunc(data, func(id []byte) []byte {
			m, ok := byID[string(id)]
			if !ok {
				rel, _ := filepath.Rel(root, path)
				unknown = append(unknown, rel+": "+string(id))
				return id
			}
			return []byte(m.Tag)
		})
		if string(out) != string(data) {
			changed[path] = out
		}
		return nil
	}
	for _, dir := range ContentDirs() {
		err := filepath.WalkDir(filepath.Join(root, dir), walk)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("these files refer to a provisional id that has no node file in nodes/~/ (rule T03):\n  %s", strings.Join(unknown, "\n  "))
	}

	for path, data := range changed {
		if err := os.WriteFile(path, data, 0o644); err != nil { // #nosec G306 -- content files are public text in a git tree.
			return nil, err
		}
	}
	var entries []Entry
	for _, m := range minted {
		from := filepath.Join(root, "nodes", "~", m.Provisional+".md")
		dir := filepath.Join(root, "nodes", Shard(m.Tag))
		to := filepath.Join(dir, m.Tag+".md")
		if _, err := os.Stat(to); err == nil {
			return nil, fmt.Errorf("%s exists already", to)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- content directories are public.
			return nil, err
		}
		if err := os.Rename(from, to); err != nil {
			return nil, err
		}
		if err := moveLog(root, m); err != nil {
			return nil, err
		}
		entries = append(entries, Entry{Tag: m.Tag, Commit: commit, Kind: m.Kind})
	}
	if len(entries) > 0 {
		if err := Append(filepath.Join(regs, "tags.reg"), entries); err != nil {
			return nil, err
		}
	}
	return minted, nil
}

// moveLog moves the provenance log of the provisional id, prov/~/ID.jsonl, to the log of the tag (spec 04.8). A node with no log has nothing to move.
func moveLog(root string, m Minted) error {
	from := filepath.Join(root, "prov", "~", m.Provisional+".jsonl")
	if _, err := os.Stat(from); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	dir := filepath.Join(root, "prov", Shard(m.Tag))
	to := filepath.Join(dir, m.Tag+".jsonl")
	if _, err := os.Stat(to); err == nil {
		return fmt.Errorf("%s exists already", to)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- content directories are public.
		return err
	}
	return os.Rename(from, to)
}

// addTreeTags adds the tags of the node files in the tree to used. A node file can exist before its tag is in the register, for example in a test tree.
func addTreeTags(root string, used map[string]bool) error {
	files, err := filepath.Glob(filepath.Join(root, "nodes", "*", "*.md"))
	if err != nil {
		return err
	}
	for _, f := range files {
		if id := strings.TrimSuffix(filepath.Base(f), ".md"); IsTag(id) {
			used[id] = true
		}
	}
	return nil
}

func readKind(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the path comes from a glob of nodes/~/.
	if err != nil {
		return "", err
	}
	text := string(data)
	if !strings.HasPrefix(text, "---\n") {
		return "", fmt.Errorf("%s: the file has no front matter", path)
	}
	head, _, ok := strings.Cut(text[4:], "\n---\n")
	if !ok {
		return "", fmt.Errorf("%s: the front matter has no end line", path)
	}
	var meta struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal([]byte(head), &meta); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	if meta.Kind == "" {
		return "", fmt.Errorf("%s: the front matter has no kind", path)
	}
	return meta.Kind, nil
}
