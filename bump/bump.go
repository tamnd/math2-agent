// Package bump moves a math2 working tree to a newer release of Mathlib (spec 07.9).
//
// A bump has these parts. [ReadPins] gives the pins of the Lake project in lean/: the Lean toolchain, the Mathlib tag and the Mathlib commit. [Latest] picks the newest release tag from the tags of the Mathlib repository, which [Tags] lists. [SetPins] writes a new tag and its toolchain, which [ToolchainOf] gets, and [Update] runs lake update to write the commit of the tag in lake-manifest.json.
//
// After the build with the new pins, [Classify] compares the extraction of the old pins with the extraction of the new pins. It gives a [Break] for each binding that the bump changed: a rename with a deprecation alias, a rename without one, a refactor, or a declaration that is gone. [Failures] reads the errors of a failed lake build, which are breaks in the Math2 code. [Rebind] writes the renames into the node files and records each break of a binding in registers/bindings.reg. [Upstream] finds the Math2 declarations that the new Mathlib has with the same type hash. [Report] is the bump report of reports/bump/.
package bump

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Pins are the versions that the Lake project in lean/ pins (spec 07.1).
type Pins struct {
	Toolchain string // the content of lean-toolchain, for example leanprover/lean4:v4.35.0
	Git       string // the git URL of Mathlib in lakefile.toml
	Tag       string // the rev of Mathlib in lakefile.toml, for example v4.35.0
	Commit    string // the commit of Mathlib in lake-manifest.json, or empty when the file does not name one
}

// lakefile is the name of the Lake configuration that the package reads and writes.
const lakefile = "lakefile.toml"

// ReadPins reads the pins of the Lake project in dir.
func ReadPins(dir string) (Pins, error) {
	var p Pins
	data, err := os.ReadFile(filepath.Join(dir, "lean-toolchain")) // #nosec G304 -- the file is in the Lake project of the tree.
	if err != nil {
		return p, err
	}
	p.Toolchain = strings.TrimSpace(string(data))
	data, err = os.ReadFile(filepath.Join(dir, lakefile)) // #nosec G304 -- the file is in the Lake project of the tree.
	if err != nil {
		return p, err
	}
	block, _, ok := mathlibBlock(data)
	if !ok {
		return p, fmt.Errorf("bump: %s has no [[require]] of mathlib", lakefile)
	}
	p.Git, p.Tag = field(block, "git"), field(block, "rev")
	if p.Tag == "" {
		return p, fmt.Errorf("bump: the [[require]] of mathlib in %s has no rev", lakefile)
	}
	data, err = os.ReadFile(filepath.Join(dir, "lake-manifest.json")) // #nosec G304 -- the file is in the Lake project of the tree.
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	var m struct {
		Packages []struct{ Name, Rev string }
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return p, fmt.Errorf("bump: lake-manifest.json: %w", err)
	}
	for _, q := range m.Packages {
		if q.Name == "mathlib" {
			p.Commit = q.Rev
		}
	}
	return p, nil
}

// SetPins writes the Mathlib tag into lakefile.toml and the toolchain into lean-toolchain of the Lake project in dir. Run [Update] after it.
func SetPins(dir, tag, toolchain string) error {
	if tag == "" || strings.ContainsAny(tag, "\"\n") || toolchain == "" || strings.ContainsAny(toolchain, " \n") {
		return fmt.Errorf("bump: the tag %q or the toolchain %q is not valid", tag, toolchain)
	}
	path := filepath.Join(dir, lakefile)
	data, err := os.ReadFile(path) // #nosec G304 -- the file is in the Lake project of the tree.
	if err != nil {
		return err
	}
	block, start, ok := mathlibBlock(data)
	if !ok {
		return fmt.Errorf("bump: %s has no [[require]] of mathlib", lakefile)
	}
	loc := revLine.FindIndex(block)
	if loc == nil {
		return fmt.Errorf("bump: the [[require]] of mathlib in %s has no rev", lakefile)
	}
	out := slices.Concat(data[:start+loc[0]], []byte(`rev = "`+tag+`"`), data[start+loc[1]:])
	if err := os.WriteFile(path, out, 0o644); err != nil { // #nosec G306 G703 -- the file is part of the working tree.
		return err
	}
	return os.WriteFile(filepath.Join(dir, "lean-toolchain"), []byte(toolchain+"\n"), 0o644) // #nosec G306 -- the file is part of the working tree.
}

var (
	tableLine = regexp.MustCompile(`(?m)^\s*\[`)
	revLine   = regexp.MustCompile(`(?m)^[ \t]*rev[ \t]*=[ \t]*"[^"\n]*"`)
)

// mathlibBlock returns the [[require]] table of mathlib in a lakefile.toml and its offset.
func mathlibBlock(data []byte) ([]byte, int, bool) {
	locs := tableLine.FindAllIndex(data, -1)
	for i, loc := range locs {
		end := len(data)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		block := data[loc[0]:end]
		if bytes.HasPrefix(bytes.TrimSpace(block), []byte("[[require]]")) && field(block, "name") == "mathlib" {
			return block, loc[0], true
		}
	}
	return nil, 0, false
}

// field returns the string value of a key of a TOML table, or empty.
func field(block []byte, key string) string {
	re := regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(key) + `[ \t]*=[ \t]*"([^"\n]*)"`)
	if m := re.FindSubmatch(block); m != nil {
		return string(m[1])
	}
	return ""
}

// release matches a release tag of Mathlib. A release candidate such as v4.36.0-rc1 does not match.
var release = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)

// Latest returns the newest release tag in tags. A release never pins a release candidate or master (spec 07.1). It is false when tags has no release tag.
func Latest(tags []string) (string, bool) {
	best, bv := "", [3]int{}
	for _, t := range tags {
		m := release.FindStringSubmatch(t)
		if m == nil {
			continue
		}
		var v [3]int
		for i := range v {
			v[i], _ = strconv.Atoi(m[i+1])
		}
		if best == "" || slices.Compare(v[:], bv[:]) > 0 {
			best, bv = t, v
		}
	}
	return best, best != ""
}

// Tags returns the tags of the git repository at the URL, with git ls-remote. It needs git on the PATH.
func Tags(ctx context.Context, git string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--tags", "--refs", "--", git) // #nosec G204 -- the URL is an argument after --, and git does not run it.
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bump: git ls-remote %s: %w", git, err)
	}
	var tags []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if _, ref, ok := strings.Cut(sc.Text(), "\t"); ok {
			tags = append(tags, strings.TrimPrefix(ref, "refs/tags/"))
		}
	}
	return tags, sc.Err()
}

// ToolchainOf returns the Lean toolchain that the tag of Mathlib names: the content of its lean-toolchain file. It reads the file from raw.githubusercontent.com, thus git must be a GitHub URL. The request has the User-Agent agent.
func ToolchainOf(ctx context.Context, c *http.Client, git, tag, agent string) (string, error) {
	u, err := url.Parse(strings.TrimSuffix(git, ".git"))
	if err != nil || u.Host != "github.com" || strings.Count(strings.Trim(u.Path, "/"), "/") != 1 {
		return "", fmt.Errorf("bump: %s is not a GitHub repository, thus the toolchain of %s is not known", git, tag)
	}
	raw := "https://raw.githubusercontent.com" + u.Path + "/" + url.PathEscape(tag) + "/lean-toolchain"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return "", err
	}
	if agent != "" {
		req.Header.Set("User-Agent", agent)
	}
	if c == nil {
		c = http.DefaultClient
	}
	resp, err := c.Do(req) // #nosec G704 -- the host is raw.githubusercontent.com, and the path comes from the pinned repository.
	if err != nil {
		return "", fmt.Errorf("bump: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bump: GET %s: %s", raw, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
	if err != nil {
		return "", fmt.Errorf("bump: %w", err)
	}
	tc := strings.TrimSpace(string(data))
	if !strings.HasPrefix(tc, "leanprover/lean4:") || strings.ContainsAny(tc, " \n") {
		return "", fmt.Errorf("bump: the lean-toolchain of %s is %q, which is not a toolchain", tag, tc)
	}
	return tc, nil
}

// Update runs lake update mathlib in the Lake project in dir, thus lake-manifest.json names the commit of the new tag. It needs lake on the PATH.
func Update(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, "lake", "update", "mathlib")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("bump: lake update mathlib: %w\n%s", err, tail(string(out), 40))
	}
	return nil
}

// tail returns the last n lines of s.
func tail(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.Join(lines[max(0, len(lines)-n):], "\n")
}

// sortBreaks sorts breaks by kind, tag and declaration.
func sortBreaks(bs []Break) {
	slices.SortFunc(bs, func(a, b Break) int {
		return cmp.Or(cmp.Compare(slices.Index(kinds, a.Kind), slices.Index(kinds, b.Kind)), cmp.Compare(a.Tag, b.Tag), cmp.Compare(a.Old, b.Old), cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
	})
}
