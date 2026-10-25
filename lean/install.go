package lean

import (
	"context"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zeebo/blake3"
)

// source is the Lake package of M2Extract, without the test fixtures.
//
//go:embed lakefile.toml Main.lean M2Extract/*.lean
var source embed.FS

// Toolchain returns the Lean toolchain of the Lake project in dir, as its lean-toolchain file gives it. An example is "leanprover/lean4:v4.35.0".
func Toolchain(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "lean-toolchain")) // #nosec G304 -- the caller names the project.
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// Install builds M2Extract with the toolchain of the Lake project in project, and returns the path of the program. It builds in a new directory in cache, with the source of M2Extract and the lean-toolchain file of the project. When that directory has a program, Install does not build again. Thus a change to the source or to the toolchain gives a new build. Install needs lake on the PATH.
func Install(ctx context.Context, project, cache string) (string, error) {
	toolchain, err := Toolchain(project)
	if err != nil {
		return "", err
	}
	h := blake3.New()
	_, _ = h.WriteString(toolchain + "\n")
	err = fs.WalkDir(source, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := source.ReadFile(path)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(h, "%s %d\n", path, len(b))
		_, _ = h.Write(b)
		return nil
	})
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "m2extract-"+hex.EncodeToString(h.Sum(nil))[:16])
	exe := filepath.Join(dir, ".lake", "build", "bin", "m2extract")
	if _, err := os.Stat(exe); err == nil {
		return exe, nil
	}
	if err := os.MkdirAll(cache, 0o750); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(cache, "build-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := os.CopyFS(tmp, source); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(tmp, "lean-toolchain"), []byte(toolchain+"\n"), 0o600); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "lake", "build")
	cmd.Dir = tmp
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build m2extract with %s: %w\n%s", toolchain, err, out)
	}
	if err := os.Rename(tmp, dir); err != nil && !os.IsExist(err) {
		if _, statErr := os.Stat(exe); statErr != nil {
			return "", err
		}
	}
	return exe, nil
}
