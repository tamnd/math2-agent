package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/audit"
	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/bump"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
)

const bumpUsage = `m2 bump [-root DIR] [-to TAG | -master] [-n] [-date YYYY-MM-DD] [-cache DIR] [-no-sandbox] [-no-commit]`

// runBump moves the tree to a newer Mathlib release on a new branch (spec 07.9). The exit code is 0 when the build and the audit pass with the new pins, and 1 when they do not. The branch and the report are there in both cases, thus the breaks can go into a pull request.
func runBump(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("m2 bump", flag.ContinueOnError)
	flags.SetOutput(stderr)
	opt := build.Options{Version: version, Log: stdout}
	flags.StringVar(&opt.Root, "root", ".", "the math2 working tree")
	flags.StringVar(&opt.Cache, "cache", "", "the directory where M2Extract is built. Empty means m2 in the user cache directory.")
	flags.BoolVar(&opt.NoSandbox, "no-sandbox", false, "run the checkers of the replay step without the bwrap sandbox, for example on macOS")
	to := flags.String("to", "", "the Mathlib tag. Empty means the newest release tag.")
	master := flags.Bool("master", false, "build Mathlib master as an early warning. Do not merge such a branch.")
	dry := flags.Bool("n", false, "show the tag of the bump and stop")
	date := flags.String("date", "", "the date of the report. Empty means today in UTC.")
	noCommit := flags.Bool("no-commit", false, "change the working tree, and make no branch and no commit")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() > 0 || *master && *to != "" {
		fmt.Fprintln(stderr, "usage: "+bumpUsage)
		return ExitUsage
	}
	if *date == "" {
		*date = time.Now().UTC().Format(time.DateOnly)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 bump: %v\n", err)
		return ExitFailure
	}

	dir := filepath.Join(opt.Root, "lean")
	from, err := bump.ReadPins(dir)
	if err != nil {
		return fail(err)
	}
	target := *to
	switch {
	case *master:
		target = "master"
	case target == "":
		tags, err := bump.Tags(ctx, from.Git)
		if err != nil {
			return fail(err)
		}
		var ok bool
		if target, ok = bump.Latest(tags); !ok {
			return fail(fmt.Errorf("%s has no release tag", from.Git))
		}
	default:
		if _, ok := bump.Latest([]string{target}); !ok {
			return fail(fmt.Errorf("%s is not a release tag of Mathlib. A release never pins a release candidate. Use -master for an early-warning build", target))
		}
	}
	if target == from.Tag && !*master {
		fmt.Fprintf(stdout, "math2 pins %s already, the newest release of Mathlib. Nothing to bump.\n", target)
		return ExitOK
	}
	if *dry {
		fmt.Fprintf(stdout, "m2 bump moves Mathlib from %s to %s\n", from.Tag, target)
		return ExitOK
	}
	if out, err := git(opt.Root, "status", "--porcelain"); err != nil || out != "" {
		return fail(fmt.Errorf("the working tree must be a git repository with no changes. git status gives:\n%s%v", out, orNil(err)))
	}

	// The build with the old pins gives the type hashes of the bindings before the bump.
	out := filepath.Join(opt.Root, "build")
	if m, err := build.ReadManifest(out); err != nil || from.Commit == "" || m.Mathlib != from.Commit {
		fmt.Fprintf(stdout, "the build with the old pins\n")
		if _, err := build.Run(ctx, opt); err != nil {
			return fail(fmt.Errorf("the build with the old pins: %w", err))
		}
	}
	oldX, err := build.ReadExtraction(out)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fail(err)
	}
	oldS, err := build.ReadStatus(out)
	if err != nil {
		return fail(err)
	}

	if !*noCommit {
		if _, err := git(opt.Root, "switch", "-c", "bump/"+target); err != nil {
			return fail(err)
		}
	}
	start := time.Now()
	tc, err := bump.ToolchainOf(ctx, http.DefaultClient, from.Git, target, "m2/"+version+" (+https://github.com/tamnd/math2-agent)")
	if err != nil {
		return fail(err)
	}
	if err := bump.SetPins(dir, target, tc); err != nil {
		return fail(err)
	}
	if err := bump.Update(ctx, dir); err != nil {
		return fail(err)
	}
	cur, err := bump.ReadPins(dir)
	if err != nil {
		return fail(err)
	}
	r := &bump.Report{Date: *date, From: from, To: cur, Early: *master}
	fmt.Fprintf(stdout, "the build with %s\n", target)
	if _, err := build.Run(ctx, opt); err != nil {
		if r.Breaks = bump.Failures(err.Error()); len(r.Breaks) == 0 {
			return fail(err)
		}
	} else if err := classify(ctx, opt, oldX, oldS, r); err != nil {
		return fail(err)
	}
	r.Took = time.Since(start)

	path := filepath.Join(opt.Root, filepath.FromSlash(r.Path()))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(path, r.Markdown(), 0o644); err != nil { // #nosec G306 -- the report is part of the working tree.
		return fail(err)
	}
	if !*noCommit {
		msg := fmt.Sprintf("Bump Mathlib to %s\n\nThe report is in %s.", target, r.Path())
		if *master {
			msg += " This is an early-warning build of Mathlib master. Do not merge it."
		}
		if _, err := git(opt.Root, "add", "-A"); err != nil {
			return fail(err)
		}
		if _, err := git(opt.Root, "commit", "-q", "-m", msg); err != nil {
			return fail(err)
		}
	}
	for _, k := range bump.Kinds() {
		fmt.Fprintf(stdout, "%s %d\n", k, r.Count(k))
	}
	fmt.Fprintf(stdout, "stale %d, upstream %d\nreport %s\n", len(r.Stale), len(r.Upstream), r.Path())
	if !r.Green {
		fmt.Fprintln(stderr, "m2 bump: the build or the audit fails with the new pins. Look at the report.")
		return ExitFailure
	}
	return ExitOK
}

// classify fills the report after a build with the new pins passes. It finds the breaks of the bindings, applies the renames, builds again when a register or a node changed, and runs the audit.
func classify(ctx context.Context, opt build.Options, oldX *build.Extraction, oldS []status.Status, r *bump.Report) error {
	out := filepath.Join(opt.Root, "build")
	x, err := build.ReadExtraction(out)
	if err != nil {
		return err
	}
	nodes, err := node.Load(opt.Root)
	if err != nil {
		return err
	}
	var byHash map[string][]string
	if needHashes(x) {
		fmt.Fprintf(opt.Log, "the type hashes of Mathlib\n")
		if byHash, err = hashes(ctx, filepath.Join(opt.Root, "lean"), opt.Cache); err != nil {
			return err
		}
	}
	r.Breaks = bump.Classify(nodes, oldX, x, byHash)
	r.Upstream = bump.Upstream(x, byHash)
	if r.Changed, err = bump.Rebind(opt.Root, r.Breaks, r.To.Commit); err != nil {
		return err
	}
	if r.Count(bump.Deprecated)+r.Count(bump.Renamed)+r.Count(bump.Refactored) > 0 {
		fmt.Fprintf(opt.Log, "the build after the renames\n")
		if _, err := build.Run(ctx, opt); err != nil {
			return err
		}
	}
	cur, err := build.ReadStatus(out)
	if err != nil {
		return err
	}
	r.Stale = bump.Stale(oldS, cur)
	rep, err := audit.Run(opt.Root, audit.Options{})
	if err != nil {
		return err
	}
	r.Audit = summary(rep)
	r.Green = !rep.Failed()
	return nil
}

// needHashes tells if the bump must read the type hashes of Mathlib: a bound declaration is gone, or a binding is in Math2 and can have a copy in Mathlib now.
func needHashes(x *build.Extraction) bool {
	for _, r := range x.Records {
		if r.Error != "" {
			return true
		}
	}
	for _, b := range x.Bindings {
		if strings.HasPrefix(b.Decl, "Math2.") {
			return true
		}
	}
	return false
}

// hashes returns the declarations of Mathlib by type hash, from M2Extract in the Lake project.
func hashes(ctx context.Context, project, cache string) (_ map[string][]string, err error) {
	exe, err := lean.Install(ctx, project, cache)
	if err != nil {
		return nil, err
	}
	s, err := lean.Start(ctx, exe, project)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := s.Close(); err == nil {
			err = cerr
		}
	}()
	if err := s.Import("Mathlib"); err != nil {
		return nil, err
	}
	ds, err := bump.Library(s, "Mathlib")
	if err != nil {
		return nil, err
	}
	return bump.ByHash(ds), nil
}

// summary returns the number of rules in each state of the audit, on one line.
func summary(rep *check.Report) string {
	n := map[check.State]int{}
	for _, r := range rep.Results {
		n[r.State]++
	}
	var parts []string
	for _, s := range []check.State{check.Pass, check.Fail, check.Found, check.NA, check.Skip} {
		parts = append(parts, fmt.Sprintf("%d %s", n[s], s))
	}
	return strings.Join(parts, ", ")
}

// git runs git in the tree and returns its output.
func git(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...) // #nosec G204 -- the arguments are fixed in this file, and the root is one argument.
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// orNil returns the error as text, or empty.
func orNil(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
