// Package propose opens the pull requests of the write methods of the API (spec 11.1 and 11.5).
//
// Every write to math2 is a pull request against tamnd/math2. The API has no other write path, thus the audit is the only gate and git is the only history. A [Change] holds the files that one write adds or replaces. [NewNode], [Edit], [Bind] and [Align] make the change of each write method from the tree that the server reads.
//
// [Proposer.Open] adds a provenance line for each field that the change touches, with the person who asked for it. Then it writes the change to a git worktree of the tree and runs the audit of a pull request on it. It refuses the change when a hard rule fails. When the audit passes, it opens the pull request under the identity of the person, through a [Host] such as [GitHub]. The rules that need a build do not run there. CI runs them on the pull request.
//
// The review throughput limits the writes (spec 11.5). Each write method has a cap on its pull requests in seven days. The cap is a multiple of the weekly number of pull requests of that method that were closed or merged in the last four weeks, and it is never less than a floor. Thus the writes cannot go faster than the people who review them.
package propose

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tamnd/math2-agent/audit"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prov"
)

// The write methods of spec 11.2.
const (
	MethodNode      = "propose_node"
	MethodEdit      = "propose_edit"
	MethodBinding   = "propose_binding"
	MethodAlignment = "propose_alignment"
)

// Methods returns the write methods, in the order of spec 11.2.
func Methods() []string {
	return []string{MethodNode, MethodEdit, MethodBinding, MethodAlignment}
}

// Change is the content of one write.
type Change struct {
	Method string            // the write method
	Tag    string            // the tag or the provisional id of the node
	Title  string            // the title of the pull request
	Files  map[string][]byte // the path of each file from the tree root, with its new content
}

// Branch is the prefix of the head branch of each pull request that the API opens. The branch of a pull request is the prefix, the method, a slash and a random name.
const Branch = "m2-api/"

// Method returns the write method of a head branch, or "" when the branch is not one that the API made.
func Method(branch string) string {
	rest, ok := strings.CutPrefix(branch, Branch)
	if !ok {
		return ""
	}
	m, _, _ := strings.Cut(rest, "/")
	if !slices.Contains(Methods(), m) {
		return ""
	}
	return m
}

// Limit is the cap on the pull requests of each write method (spec 11.5).
type Limit struct {
	Multiple float64 // the pull requests that a week can have for each one that was closed in a week. Zero means 2.
	Floor    int     // the smallest cap, so that a method with few reviews can start. Zero means 10.
}

// Cap returns the number of pull requests of a method that seven days can have, when closed pull requests of the method were closed or merged in the last four weeks.
func (l Limit) Cap(closed int) int {
	m := cmp.Or(l.Multiple, 2)
	return max(cmp.Or(l.Floor, 10), int(math.Ceil(m*float64(closed)/4)))
}

// The windows of the cap.
const (
	week   = 7 * 24 * time.Hour
	window = 4 * week
)

// Proposer opens the pull requests of the write methods. Its methods are safe for concurrent use. It opens one pull request at a time.
type Proposer struct {
	Root  string // a git clone of tamnd/math2. Its HEAD is the commit that each pull request starts from.
	Base  string // the branch that the pull requests go to. Empty means main.
	Host  Host   // where the pull requests go
	Limit Limit

	// Now returns the time of a write, for the provenance lines and the cap. Nil means time.Now.
	Now func() time.Time

	mu sync.Mutex
}

// Result is the pull request of a write.
type Result struct {
	Pull
	User    string        `json:"user"`    // the person, as github:LOGIN
	Files   []string      `json:"files"`   // the files of the change, from the tree root
	Opened  int           `json:"opened"`  // the pull requests of the method in the last seven days, with this one
	Cap     int           `json:"cap"`     // the cap of the method
	Report  *check.Report `json:"audit"`   // the audit of the change
	Message string        `json:"message"` // what happens next
}

// The errors of Open.
var (
	ErrUnauthorized = errors.New("propose: the write needs a GitHub token of a person")
	ErrLimit        = errors.New("propose: the method has the most pull requests that the reviews permit")
)

// AuditError is the error of a change that fails a hard rule of the audit.
type AuditError struct {
	Report *check.Report
}

// Error returns the findings.
func (e *AuditError) Error() string {
	var b strings.Builder
	b.WriteString("propose: the change fails the audit. Correct these problems and try again:")
	for _, f := range e.Report.Findings {
		b.WriteString("\n  " + f.String())
	}
	return b.String()
}

// Open opens a pull request with the change, under the identity of the person who has the token. The note of the person goes in the body of the pull request. Open returns an error that wraps ErrUnauthorized when the host does not accept the token, an error that wraps ErrLimit when the method is at its cap, and an *AuditError when the change fails the audit.
func (p *Proposer) Open(ctx context.Context, token string, c Change, note string) (*Result, error) {
	if token == "" {
		return nil, ErrUnauthorized
	}
	if !slices.Contains(Methods(), c.Method) || len(c.Files) == 0 {
		return nil, fmt.Errorf("propose: the change has no files of a write method")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	at := now().UTC()

	login, err := p.Host.User(ctx, token)
	if err != nil {
		return nil, err
	}
	pulls, err := p.Host.Pulls(ctx, token, at.Add(-window))
	if err != nil {
		return nil, err
	}
	opened, closed := 0, 0
	for _, pr := range pulls {
		if Method(pr.Branch) != c.Method {
			continue
		}
		if pr.Opened.After(at.Add(-week)) {
			opened++
		}
		if !pr.Closed.IsZero() && pr.Closed.After(at.Add(-window)) {
			closed++
		}
	}
	limit := p.Limit.Cap(closed)
	if opened >= limit {
		return nil, fmt.Errorf("%w: %s has %d pull requests in the last seven days, and %d were closed in the last four weeks, so the cap is %d. Try again when the reviews catch up", ErrLimit, c.Method, opened, closed, limit)
	}

	who := "github:" + login
	files, err := withProvenance(p.Root, c.Files, prov.Line{Human: who, At: at.Format(time.RFC3339)})
	if err != nil {
		return nil, err
	}
	commit, rep, err := p.check(files)
	if err != nil {
		return nil, err
	}

	paths := slices.Sorted(maps.Keys(files))
	branch, err := branchName(c)
	if err != nil {
		return nil, err
	}
	pr, err := p.Host.Open(ctx, token, PullRequest{
		User:    login,
		Base:    cmp.Or(p.Base, "main"),
		Commit:  commit,
		Branch:  branch,
		Title:   c.Title,
		Body:    body(c, who, note, rep, paths),
		Message: c.Title + "\n\n" + c.Method + " of the math2 API, for " + who + ".",
		Files:   files,
	})
	if err != nil {
		return nil, err
	}
	return &Result{Pull: pr, User: who, Files: paths, Opened: opened + 1, Cap: limit, Report: rep,
		Message: "The pull request is open. CI runs the full audit on it, and a person reviews it."}, nil
}

// withProvenance returns the files of the change with a provenance line for each field that changed in each node file. A node file is new when the tree has no file at its path.
func withProvenance(root string, files map[string][]byte, who prov.Line) (map[string][]byte, error) {
	out := map[string][]byte{}
	for path, data := range files {
		out[path] = data
	}
	for _, path := range slices.Sorted(maps.Keys(files)) {
		if !strings.HasPrefix(path, "nodes/") || !strings.HasSuffix(path, ".md") {
			continue
		}
		after, err := node.Parse(path, files[path])
		if err != nil {
			return nil, fmt.Errorf("propose: %w", err)
		}
		var before *node.Node
		if old, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path))); err == nil { // #nosec G304 -- the path is a node file in the tree that the server reads.
			if before, err = node.Parse(path, old); err != nil {
				return nil, fmt.Errorf("propose: the tree has a node file that does not parse: %w", err)
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		lines := prov.Changes(before, after, who)
		if len(lines) == 0 {
			return nil, fmt.Errorf("propose: the change does not change %s", path)
		}
		logPath := prov.Path(after.ID)
		log, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(logPath))) // #nosec G304 -- the path is a provenance log in the tree that the server reads.
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if len(log) > 0 && !bytes.HasSuffix(log, []byte("\n")) {
			log = append(log, '\n')
		}
		for _, l := range lines {
			if err := l.Validate(); err != nil {
				return nil, fmt.Errorf("propose: %s %s: %w", after.ID, l.Field, err)
			}
			b, err := json.Marshal(l)
			if err != nil {
				return nil, err
			}
			log = append(append(log, b...), '\n')
		}
		out[logPath] = log
	}
	return out, nil
}

// check writes the files to a new git worktree of the HEAD of the tree and runs the audit of a pull request on it. It returns the commit of HEAD and the report. A report with a failed rule gives an *AuditError.
func (p *Proposer) check(files map[string][]byte) (string, *check.Report, error) {
	commit, err := git(p.Root, "rev-parse", "HEAD")
	if err != nil {
		return "", nil, err
	}
	dir, err := os.MkdirTemp("", "m2-propose-")
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	wt := filepath.Join(dir, "tree")
	if _, err := git(p.Root, "worktree", "add", "-q", "--detach", wt, commit); err != nil {
		return "", nil, err
	}
	defer func() { _, _ = git(p.Root, "worktree", "remove", "--force", wt) }()
	for path, data := range files {
		if !filepath.IsLocal(filepath.FromSlash(path)) || strings.HasPrefix(path, ".") {
			return "", nil, fmt.Errorf("propose: %q is not a path in the tree", path)
		}
		full := filepath.Join(wt, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			return "", nil, err
		}
		if err := os.WriteFile(full, data, 0o600); err != nil {
			return "", nil, err
		}
	}
	rep, err := audit.Run(wt, audit.Options{Base: commit})
	if err != nil {
		return "", nil, fmt.Errorf("propose: the audit did not run: %w", err)
	}
	if rep.Failed() {
		return "", nil, &AuditError{Report: rep}
	}
	return commit, rep, nil
}

// git runs git in dir and returns its output with no space at the ends.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...) // #nosec G204 -- the arguments are fixed, or paths and revisions that the proposer made.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("propose: git %s: %v: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

// branchName returns a new head branch for the change. A git branch cannot have the ~ of a provisional id, thus the branch drops it.
func branchName(c Change) (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return Branch + c.Method + "/" + strings.TrimPrefix(c.Tag, "~") + "-" + hex.EncodeToString(b), nil
}

// maxNote is the longest note of a person, in bytes.
const maxNote = 4000

// body returns the body of the pull request.
func body(c Change, who, note string, rep *check.Report, paths []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The math2 API opened this pull request with the write method %s, for %s and with their token (spec 11.1).\n\n", c.Method, who)
	if note = strings.TrimSpace(note); note != "" {
		if len(note) > maxNote {
			note = strings.ToValidUTF8(note[:maxNote], "") + " ..."
		}
		b.WriteString("The note of the author:\n\n")
		for line := range strings.Lines(note) {
			b.WriteString("> " + line)
		}
		b.WriteString("\n\n")
	}
	pass := 0
	for _, r := range rep.Results {
		if r.State == check.Pass {
			pass++
		}
	}
	fmt.Fprintf(&b, "The audit of a pull request passed before the API opened it: %d rules passed, and no hard rule failed. The rules that need a build did not run there. CI runs them.\n\nThe files:\n\n", pass)
	for _, path := range paths {
		fmt.Fprintf(&b, "- `%s`\n", path)
	}
	return b.String()
}
