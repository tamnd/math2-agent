// Package workspace is the working tree of one agent run (spec 12.2).
//
// A [Workspace] is a git work tree of math2 at a base commit. The tools of an agent read and write files in it through [Workspace.Read] and [Workspace.Write]. These methods take a path relative to the tree, and they cannot reach a file outside the tree or in the .git directory, even through a symbolic link. The agent gets no shell, thus these methods are its only way to change the tree.
//
// [Workspace.Changes] gives each file that the run added, changed or deleted, with its text at the base commit and its text now. The run checks the changes against the field permissions of its role before it opens a pull request.
package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// MaxFile is the largest file in bytes that Write writes.
const MaxFile = 1 << 20

// Workspace is a git work tree at a base commit.
type Workspace struct {
	Dir  string // the directory of the work tree
	Base string // the base commit
	root *os.Root
}

// Change is one file that a run added, changed or deleted.
type Change struct {
	Path string // the path in the tree, with forward slashes
	Old  []byte // the text at the base commit. It is nil when the run added the file.
	New  []byte // the text now. It is nil when the run deleted the file.
}

// Add makes a new work tree of the git repository repo in dir, at the commit, and opens it. The directory must not exist.
func Add(ctx context.Context, repo, dir, commit string) (*Workspace, error) {
	if _, err := git(ctx, repo, "worktree", "add", "--detach", "--quiet", dir, commit); err != nil {
		return nil, err
	}
	return Open(ctx, dir)
}

// Open opens the git work tree in dir. Its base commit is the commit of HEAD.
func Open(ctx context.Context, dir string) (*Workspace, error) {
	out, err := git(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}
	return &Workspace{Dir: dir, Base: strings.TrimSpace(string(out)), root: root}, nil
}

// Close closes the workspace. It does not remove the work tree.
func (w *Workspace) Close() error { return w.root.Close() }

// Remove closes the workspace and removes its work tree from the repository.
func (w *Workspace) Remove(ctx context.Context) error {
	_ = w.root.Close()
	_, err := git(ctx, w.Dir, "worktree", "remove", "--force", w.Dir)
	return err
}

// clean returns the path in the form of the os package, or an error for a path that is not local or that is in .git.
func clean(rel string) (string, error) {
	p := path.Clean(rel)
	if rel == "" || strings.Contains(rel, `\`) || !filepath.IsLocal(filepath.FromSlash(p)) || p == "." {
		return "", fmt.Errorf("workspace: %q is not a path in the tree", rel)
	}
	if first, _, _ := strings.Cut(p, "/"); first == ".git" {
		return "", fmt.Errorf("workspace: %q is in the .git directory", rel)
	}
	return filepath.FromSlash(p), nil
}

// Read returns the text of the file at the path in the tree.
func (w *Workspace) Read(rel string) ([]byte, error) {
	p, err := clean(rel)
	if err != nil {
		return nil, err
	}
	data, err := w.root.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}
	return data, nil
}

// Write writes the text to the file at the path in the tree, and makes its directories.
func (w *Workspace) Write(rel string, data []byte) error {
	p, err := clean(rel)
	if err != nil {
		return err
	}
	if len(data) > MaxFile {
		return fmt.Errorf("workspace: %s has %d bytes, and the limit is %d", rel, len(data), MaxFile)
	}
	if dir := filepath.Dir(p); dir != "." {
		if err := w.root.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("workspace: %w", err)
		}
	}
	if err := w.root.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("workspace: %w", err)
	}
	return nil
}

// Delete deletes the file at the path in the tree.
func (w *Workspace) Delete(rel string) error {
	p, err := clean(rel)
	if err != nil {
		return err
	}
	if err := w.root.Remove(p); err != nil {
		return fmt.Errorf("workspace: %w", err)
	}
	return nil
}

// Changes returns the files that differ from the base commit, in order of path. A file that git ignores is not a change.
func (w *Workspace) Changes(ctx context.Context) ([]Change, error) {
	diff, err := git(ctx, w.Dir, "diff", "--name-only", "--no-renames", "-z", w.Base, "--")
	if err != nil {
		return nil, err
	}
	other, err := git(ctx, w.Dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, b := range bytes.Split(append(diff, other...), []byte{0}) {
		if len(b) > 0 {
			paths = append(paths, string(b))
		}
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	out := make([]Change, 0, len(paths))
	for _, p := range paths {
		c := Change{Path: p}
		if old, err := git(ctx, w.Dir, "cat-file", "blob", w.Base+":"+p); err == nil {
			c.Old = append([]byte{}, old...)
		}
		data, err := w.root.ReadFile(filepath.FromSlash(p))
		switch {
		case err == nil:
			c.New = data
		case !errors.Is(err, fs.ErrNotExist):
			return nil, fmt.Errorf("workspace: %w", err)
		}
		if c.Old == nil && c.New == nil || c.Old != nil && c.New != nil && bytes.Equal(c.Old, c.New) {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) // #nosec G204 -- the arguments are fixed, paths and commits, and no shell reads them.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("workspace: git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
