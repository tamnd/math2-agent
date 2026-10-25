// Package release makes and checks the release manifests of tamnd/math2 (spec 06.2).
//
// A release is a git tag on tamnd/math2 and a manifest in manifests/releases/. The manifest pins the toolchain and gives the hash of each artefact of the release. [Make] makes a manifest from a working tree and its last m2 build. [RebuildGraph] rebuilds the derived graph from a commit, so that a second machine can compare the hash with the manifest (rule R01). [RebuildOleans] and [Compare] rebuild the .olean files and list each file that is different (rule R02).
package release

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zeebo/blake3"
	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/node"
)

// Dir is the directory of the release manifests in the tree.
const Dir = "manifests/releases"

// The names of the artefacts that this package makes. The search index, the site and the certificates come in later milestones.
const (
	Graph  = "graph"
	Oleans = "oleans"
)

// OleanDir is the directory of the .olean files of the math2 library in the tree.
const OleanDir = "lean/.lake/build/lib/lean"

// Manifest is a release manifest, manifests/releases/RELEASE.yaml.
type Manifest struct {
	Release      string              `yaml:"release"` // the calendar version, for example 2027.03
	Commit       string              `yaml:"commit"`  // the commit of tamnd/math2
	Toolchain    Toolchain           `yaml:"toolchain"`
	Artefacts    map[string]Artefact `yaml:"artefacts"`
	Counts       Counts              `yaml:"counts"`
	Archived     *Archived           `yaml:"archived,omitempty"`
	RebuildCheck *RebuildCheck       `yaml:"rebuild_check,omitempty"`
}

// Toolchain is the set of tools that a release pins.
type Toolchain struct {
	M2        string            `yaml:"m2"`      // the tag of tamnd/math2-agent
	Lean      string            `yaml:"lean"`    // the toolchain in lean/lean-toolchain
	Mathlib   string            `yaml:"mathlib"` // the Mathlib commit in lean/lake-manifest.json
	OtherLibs map[string]string `yaml:"other_libs,omitempty"`
	Images    map[string]string `yaml:"images,omitempty"` // the container of each computational engine, by digest
}

// Artefact is one artefact of a release.
type Artefact struct {
	Hash string `yaml:"hash"` // the hash of the files, as [build.HashTree] gives it
	Size int64  `yaml:"size"` // the size of the files in bytes
}

// Counts are the counts of a release. The README table comes from them.
type Counts struct {
	NodesLive  int            `yaml:"nodes_live"`
	ByKind     map[string]int `yaml:"by_kind"`
	ByEvidence map[string]int `yaml:"by_evidence"` // by evidence level, for example E5. A node without a level is not in the map.
}

// Archived gives the copies of a release outside the project.
type Archived struct {
	SWH       string `yaml:"swh,omitempty"`        // the Software Heritage revision id
	ZenodoDOI string `yaml:"zenodo_doi,omitempty"` // the DOI of the Zenodo record
}

// RebuildCheck is the result of the rebuild of a release on a second machine.
type RebuildCheck struct {
	Runner               string        `yaml:"runner"` // for example ubuntu-24.04/x86_64/4c/16G
	GraphHashReproduced  bool          `yaml:"graph_hash_reproduced"`
	OleansHashReproduced bool          `yaml:"oleans_hash_reproduced"`
	OleanDifferences     []Explanation `yaml:"olean_differences,omitempty"` // each file that is different, when OleansHashReproduced is false
}

// Explanation gives the reason why one file of a rebuild is different.
type Explanation struct {
	Path   string `yaml:"path"`
	Reason string `yaml:"reason"`
}

// Make makes the manifest of the release name from the working tree root and its last m2 build in out. The tree must be a git work tree without changes, because the manifest names the HEAD commit. Make does not set Archived and RebuildCheck.
func Make(root, out, name string) (*Manifest, error) {
	head, err := git(root, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	dirty, err := git(root, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return nil, err
	}
	if dirty != "" {
		return nil, errors.New("release: the working tree has changes. Commit them first")
	}
	bm, err := build.ReadManifest(out)
	if err != nil {
		return nil, fmt.Errorf("release: %w. Run m2 build first", err)
	}
	m := &Manifest{
		Release:   name,
		Commit:    head,
		Toolchain: Toolchain{M2: bm.M2, Lean: bm.Lean, Mathlib: bm.Mathlib},
		Artefacts: map[string]Artefact{},
	}
	if m.Artefacts[Graph], err = Hash(filepath.Join(out, build.GraphDir), all); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(root, OleanDir)); err == nil {
		if m.Artefacts[Oleans], err = Hash(filepath.Join(root, OleanDir), IsOlean); err != nil {
			return nil, err
		}
	}
	st, err := build.ReadStatus(out)
	if err != nil {
		return nil, fmt.Errorf("release: %w. Run m2 build first", err)
	}
	m.Counts = Counts{ByKind: map[string]int{}, ByEvidence: map[string]int{}}
	for _, s := range st {
		if s.Retired {
			continue
		}
		m.Counts.NodesLive++
		m.Counts.ByKind[s.Kind]++
		if l := s.Evidence.String(); l != "" {
			m.Counts.ByEvidence[l]++
		}
	}
	return m, nil
}

func all(string) bool { return true }

// IsOlean reports whether the file rel of the library directory is a file that Lean writes: an .olean file and its parts, an .ilean file or an .ir file. The records of Lake, for example the .hash and .trace files, are not.
func IsOlean(rel string) bool {
	base := filepath.Base(rel)
	return !strings.HasSuffix(base, ".hash") && !strings.HasSuffix(base, ".trace") && !strings.HasSuffix(base, ".log")
}

// Hash returns the artefact of the files below dir for which keep is true.
func Hash(dir string, keep func(rel string) bool) (Artefact, error) {
	h, err := build.HashTree(dir, keep)
	if err != nil {
		return Artefact{}, err
	}
	a := Artefact{Hash: h}
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if !keep(filepath.ToSlash(rel)) {
			return nil
		}
		info, err := d.Info()
		if err == nil {
			a.Size += info.Size()
		}
		return err
	})
	return a, err
}

// Read reads a manifest from a file.
func Read(path string) (*Manifest, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the caller names the file.
	if err != nil {
		return nil, err
	}
	m := &Manifest{}
	if err := yaml.Unmarshal(data, m); err != nil {
		return nil, fmt.Errorf("release: %s: %w", path, err)
	}
	return m, nil
}

// Marshal returns the manifest as YAML.
func (m *Manifest) Marshal() ([]byte, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return b.Bytes(), enc.Close()
}

// RebuildGraph rebuilds the derived graph of commit from the git repository repo, in a new temporary directory, and returns its artefact. It reads only the node files of the commit, thus it needs no Lean toolchain.
func RebuildGraph(ctx context.Context, repo, commit string) (Artefact, error) {
	tmp, err := os.MkdirTemp("", "m2-release-")
	if err != nil {
		return Artefact{}, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := checkout(ctx, repo, commit, tmp, "nodes"); err != nil {
		return Artefact{}, err
	}
	nodes, err := node.Load(tmp)
	if err != nil {
		return Artefact{}, err
	}
	dir := filepath.Join(tmp, build.GraphDir)
	if _, err := graph.Build(nodes).WriteParquet(dir); err != nil {
		return Artefact{}, err
	}
	return Hash(dir, all)
}

// RebuildOleans writes the files of commit from the git repository repo to the empty directory work, gets the Mathlib cache and builds the Lean library. It returns the directory of the .olean files that the build made. The build takes as long as lake build.
func RebuildOleans(ctx context.Context, repo, commit, work string) (string, error) {
	if err := checkout(ctx, repo, commit, work); err != nil {
		return "", err
	}
	project := filepath.Join(work, "lean")
	for _, args := range [][]string{{"exe", "cache", "get"}, {"build"}} {
		cmd := exec.CommandContext(ctx, "lake", args...) // #nosec G204 -- the arguments are fixed in this package.
		cmd.Dir = project
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("release: lake %s: %w\n%s", strings.Join(args, " "), err, tail(out))
		}
	}
	return filepath.Join(work, OleanDir), nil
}

// Difference is one file that is different in two trees of .olean files.
type Difference struct {
	Path string // the path from the root of the trees
	Want string // the hash of the file in the first tree, or "" when only the second tree has it
	Got  string // the hash of the file in the second tree, or "" when only the first tree has it
}

// Compare compares the files of the trees want and got for which [IsOlean] is true, and returns each file that is different, in path order. An empty list means that the two trees have the same artefact hash.
func Compare(want, got string) ([]Difference, error) {
	w, err := hashes(want)
	if err != nil {
		return nil, err
	}
	g, err := hashes(got)
	if err != nil {
		return nil, err
	}
	var out []Difference
	for p, h := range w {
		if g[p] != h {
			out = append(out, Difference{Path: p, Want: h, Got: g[p]})
		}
	}
	for p, h := range g {
		if _, ok := w[p]; !ok {
			out = append(out, Difference{Path: p, Got: h})
		}
	}
	slices.SortFunc(out, func(a, b Difference) int { return strings.Compare(a.Path, b.Path) })
	return out, nil
}

// hashes returns the hash of each file of dir for which IsOlean is true.
func hashes(dir string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		if !IsOlean(rel) {
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G304 G122 -- the path comes from a walk of a directory that the caller names.
		if err != nil {
			return err
		}
		sum := blake3.Sum256(data)
		out[rel] = node.HashPrefix + hex.EncodeToString(sum[:])
		return nil
	})
	return out, err
}

// checkout writes the files of commit below the paths to dir, with git archive.
func checkout(ctx context.Context, repo, commit, dir string, paths ...string) error {
	if commit == "" || strings.HasPrefix(commit, "-") {
		return fmt.Errorf("release: %q is not a commit", commit)
	}
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo, "archive", "--format=tar", commit, "--"}, paths...)...) // #nosec G204 -- a commit that starts with - is refused above, and no shell reads the arguments.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	terr := untar(out, dir)
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("release: git archive %s: %w: %s", commit, err, strings.TrimSpace(stderr.String()))
	}
	return terr
}

// untar writes the regular files of a tar stream below dir.
func untar(r io.Reader, dir string) error {
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg || !filepath.IsLocal(h.Name) {
			continue
		}
		path := filepath.Join(dir, filepath.FromSlash(h.Name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) // #nosec G304 -- the name is local to dir.
		if err != nil {
			return err
		}
		_, err = io.Copy(f, tr) // #nosec G110 -- the archive comes from the repository of the caller.
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
}

func git(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...) // #nosec G204 -- the arguments are fixed in this package.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("release: git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

func tail(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 40 {
		lines = lines[len(lines)-40:]
	}
	return strings.Join(lines, "\n")
}
