package site

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// loadHistory gives each node the dates of the first and the last commit that changed its file, from the git history of the content repository. The content directory must be the top of a git work tree with its full history: a shallow clone has one commit, so each node would have the date of that commit. With no history, the nodes have no dates and the site has no feed of updates.
func (b *builder) loadHistory() {
	dir, err := filepath.Abs(b.opt.Content)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	git := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) // #nosec G204 -- the arguments are fixed, and dir is the content directory of the build.
		var out bytes.Buffer
		cmd.Stdout = &out
		err := cmd.Run()
		return out.Bytes(), err
	}
	out, err := git("rev-parse", "--show-toplevel", "--is-shallow-repository")
	if err != nil {
		return
	}
	lines := strings.Fields(string(out))
	if len(lines) != 2 || lines[1] != "false" || !samePath(lines[0], dir) {
		return
	}
	out, err = git("log", "--no-renames", "--format=%x1e%cI", "--name-status", "--", "nodes")
	if err != nil {
		return
	}
	byPath := map[string]*nodeView{}
	for _, v := range b.order {
		byPath[filepath.ToSlash(v.Path)] = v
	}
	// The log has the newest commit first.
	for _, c := range strings.Split(string(out), "\x1e") {
		rows := strings.Split(strings.TrimSpace(c), "\n")
		at, err := time.Parse(time.RFC3339, strings.TrimSpace(rows[0]))
		if err != nil {
			continue
		}
		for _, row := range rows[1:] {
			op, file, ok := strings.Cut(row, "\t")
			if !ok || (op != "A" && op != "M") {
				continue
			}
			v := byPath[file]
			if v == nil {
				continue
			}
			if v.Updated.IsZero() {
				v.Updated = at
			}
			v.Added = at
		}
	}
	b.history = true
}

// samePath tells if two paths name the same directory, after symbolic links.
func samePath(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}
