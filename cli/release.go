package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/audit"
	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/release"
)

const releaseUsage = `m2 release cut [-root DIR] [-date YYYY-MM-DD] [-no-build] [-cache DIR] [-no-sandbox] [-no-commit] [NAME]
m2 release notes [-root DIR] [-o FILE] [FROM..]TO
m2 release verify [-root DIR] [-oleans DIR] [-against DIR] [-record RUNNER] NAME`

func runRelease(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "cut":
			return runReleaseCut(args[1:], stdout, stderr)
		case "notes":
			return runReleaseNotes(args[1:], stdout, stderr)
		case "verify":
			return runReleaseVerify(args[1:], stdout, stderr)
		}
	}
	fmt.Fprintln(stderr, "usage: "+releaseUsage)
	return ExitUsage
}

// runReleaseCut cuts a release in two runs (spec 06.2). The first run writes the manifest, the levels and the notes of a new release and commits them. The second run, after a person wrote the summary of the notes and a second machine recorded the rebuild check, runs the full audit and makes the tag.
func runReleaseCut(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("m2 release cut", flag.ContinueOnError)
	flags.SetOutput(stderr)
	opt := build.Options{Version: version, Log: stdout}
	flags.StringVar(&opt.Root, "root", ".", "the math2 working tree")
	flags.StringVar(&opt.Cache, "cache", "", "the directory where M2Extract is built. Empty means m2 in the user cache directory.")
	flags.BoolVar(&opt.NoSandbox, "no-sandbox", false, "run the checkers of the replay step without the bwrap sandbox, for example on macOS")
	date := flags.String("date", "", "the date of the cut, which gives the name of a new release. Empty means today in UTC.")
	noBuild := flags.Bool("no-build", false, "use the last build in build/, and do not build again")
	noCommit := flags.Bool("no-commit", false, "write the files of a new release, and make no commit")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() > 1 {
		fmt.Fprintln(stderr, "usage: "+releaseUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 release cut: %v\n", err)
		return ExitFailure
	}
	now := time.Now().UTC()
	if *date != "" {
		t, err := time.Parse(time.DateOnly, *date)
		if err != nil {
			fmt.Fprintf(stderr, "m2 release cut: the date %q is not YYYY-MM-DD\n", *date)
			return ExitUsage
		}
		now = t
	}
	names, err := release.List(opt.Root)
	if err != nil {
		return fail(err)
	}
	name := flags.Arg(0)
	switch {
	case name != "" && !release.IsName(name):
		fmt.Fprintf(stderr, "m2 release cut: %q is not a release. A release has the form YYYY.MM or YYYY.MM.N\n", name)
		return ExitUsage
	case name == "" && len(names) > 0 && !tagged(opt.Root, names[len(names)-1]):
		name = names[len(names)-1]
	case name == "":
		name = release.NextName(names, now)
	}
	if out, err := git(opt.Root, "status", "--porcelain"); err != nil || out != "" {
		return fail(fmt.Errorf("the working tree must be a git repository with no changes. git status gives:\n%s%v", out, orNil(err)))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if slices.Contains(names, name) {
		return tagRelease(opt.Root, name, stdout, stderr)
	}

	out := filepath.Join(opt.Root, "build")
	opt.Out = out
	if !*noBuild {
		if _, err := build.Run(ctx, opt); err != nil {
			return fail(err)
		}
	}
	m, err := release.Make(opt.Root, out, name)
	if err != nil {
		return fail(err)
	}
	if err := m.CheckPins(); err != nil {
		return fail(fmt.Errorf("%w. A release pins one stable Lean toolchain and one Mathlib commit (rule R05)", err))
	}
	st, err := build.ReadStatus(out)
	if err != nil {
		return fail(err)
	}
	data, err := m.Marshal()
	if err != nil {
		return fail(err)
	}
	files := map[string][]byte{release.ManifestFile(name): data, release.LevelsFile(name): release.LevelsOf(st).Marshal()}
	if err := writeFiles(opt.Root, files); err != nil {
		return fail(err)
	}
	prev := ""
	if i := len(names); i > 0 && release.CompareNames(names[i-1], name) < 0 {
		prev = names[i-1]
	}
	if err := writeNotes(ctx, opt.Root, prev, name, release.NotesFile(name)); err != nil {
		return fail(err)
	}
	if !*noCommit {
		if _, err := git(opt.Root, "add", "--", release.ManifestFile(name), release.LevelsFile(name), release.NotesFile(name)); err != nil {
			return fail(err)
		}
		if _, err := git(opt.Root, "commit", "-q", "-m", "Cut the release "+name); err != nil {
			return fail(err)
		}
	}
	fmt.Fprintf(stdout, "m2 release cut wrote the release %s of the commit %s: %d live nodes.\n", name, m.Commit, m.Counts.NodesLive)
	fmt.Fprintf(stdout, "Next:\n1. Write the summary in %s.\n2. On a second machine, run m2 release verify -oleans DIR -record RUNNER %s.\n3. Run m2 release cut %s again. It runs the full audit and makes the tag.\n", release.NotesFile(name), name, name)
	return ExitOK
}

// tagRelease is the second run of m2 release cut. It makes the tag when only the files of the release changed after the commit of the manifest and the full audit passes.
func tagRelease(root, name string, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 release cut: %v\n", err)
		return ExitFailure
	}
	if tagged(root, name) {
		return fail(fmt.Errorf("the tag %s is there already", name))
	}
	m, err := release.Read(filepath.Join(root, filepath.FromSlash(release.ManifestFile(name))))
	if err != nil {
		return fail(err)
	}
	if strings.HasPrefix(m.Commit, "-") {
		return fail(fmt.Errorf("%q is not a commit", m.Commit))
	}
	diff, err := git(root, "diff", "--name-only", m.Commit, "HEAD")
	if err != nil {
		return fail(err)
	}
	for f := range strings.SplitSeq(strings.TrimSpace(diff), "\n") {
		if f != "" && !strings.HasPrefix(f, release.Dir+"/") && !strings.HasPrefix(f, "reports/") {
			return fail(fmt.Errorf("%s changed after the commit %s of the manifest. Cut a new release", f, m.Commit))
		}
	}
	rep, err := audit.Run(root, audit.Options{})
	if err != nil {
		return fail(err)
	}
	if rep.Failed() {
		var failed []string
		for _, r := range rep.Results {
			if r.State == check.Fail {
				failed = append(failed, r.Rule)
			}
		}
		return fail(fmt.Errorf("the audit fails on the rules %s. Run m2 audit -release %s for the findings", strings.Join(failed, ", "), name))
	}
	if _, err := git(root, "tag", "-a", name, "-m", "math2 "+name); err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "m2 release cut made the tag %s. Push it with git push origin %s.\n", name, name)
	return ExitOK
}

// tagged reports whether the repository has the tag.
func tagged(root, name string) bool {
	_, err := git(root, "rev-parse", "-q", "--verify", "refs/tags/"+name)
	return err == nil
}

func runReleaseNotes(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("m2 release notes", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree")
	out := flags.String("o", "", "the output file. Empty means the notes file of the release, and - means the standard output.")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: "+releaseUsage)
		return ExitUsage
	}
	from, to, explicit := strings.Cut(flags.Arg(0), "..")
	if !explicit {
		from, to = "", from
	}
	if !release.IsName(to) || explicit && !release.IsName(from) {
		fmt.Fprintf(stderr, "m2 release notes: %q is not a release or a range FROM..TO of releases\n", flags.Arg(0))
		return ExitUsage
	}
	if !explicit {
		names, err := release.List(*root)
		if err != nil {
			fmt.Fprintf(stderr, "m2 release notes: %v\n", err)
			return ExitFailure
		}
		for _, n := range names {
			if release.CompareNames(n, to) < 0 {
				from = n
			}
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	path := *out
	switch path {
	case "":
		path = release.NotesFile(to)
	case "-":
		n, err := notes(ctx, *root, from, to, "")
		if err != nil {
			fmt.Fprintf(stderr, "m2 release notes: %v\n", err)
			return ExitFailure
		}
		_, _ = stdout.Write(n.Markdown())
		return ExitOK
	default:
		abs, err := filepath.Abs(path)
		if err != nil {
			fmt.Fprintf(stderr, "m2 release notes: %v\n", err)
			return ExitFailure
		}
		path = abs
	}
	if err := writeNotes(ctx, *root, from, to, path); err != nil {
		fmt.Fprintf(stderr, "m2 release notes: %v\n", err)
		return ExitFailure
	}
	fmt.Fprintf(stdout, "m2 release notes wrote %s\n", path)
	return ExitOK
}

// notes makes the notes of the release to from the release from, which is empty for the first release. It keeps the summary of the notes in the file keep, when the file is there.
func notes(ctx context.Context, root, from, to, keep string) (*release.Notes, error) {
	var a *release.Snapshot
	if from != "" {
		var err error
		if a, err = release.Load(ctx, root, from); err != nil {
			return nil, err
		}
	}
	b, err := release.Load(ctx, root, to)
	if err != nil {
		return nil, err
	}
	n := release.Diff(a, b)
	if keep != "" {
		if data, err := os.ReadFile(keep); err == nil { // #nosec G304 -- the file is the notes file of the release or the file that the user names.
			old, err := release.ParseNotes(data)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", keep, err)
			}
			n.Summary = old.Summary
		}
	}
	return n, nil
}

// writeNotes writes the notes of the release to to the path, which is relative to the tree root or absolute.
func writeNotes(ctx context.Context, root, from, to, path string) error {
	full := path
	if !filepath.IsAbs(path) {
		full = filepath.Join(root, filepath.FromSlash(path))
	}
	n, err := notes(ctx, root, from, to, full)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return err
	}
	return os.WriteFile(full, n.Markdown(), 0o644) // #nosec G306 G703 -- the notes are part of the working tree.
}

// writeFiles writes the files below the tree root.
func writeFiles(root string, files map[string][]byte) error {
	for p, data := range files {
		path := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil { // #nosec G306 G703 -- the files of a release are part of the working tree.
			return err
		}
	}
	return nil
}

// runReleaseVerify rebuilds the artefacts of a release from the commit of its manifest and compares the hashes (rules R01 and R02). With -record, it writes the result into the rebuild check of the manifest. The exit code is 0 when each rebuild gives the hash of the manifest.
func runReleaseVerify(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("m2 release verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree, a git repository that has the commit of the release")
	work := flags.String("oleans", "", "rebuild the .olean files in this empty directory. Empty means rebuild only the graph.")
	against := flags.String("against", "", "the .olean files of the release, to list each file that is different")
	record := flags.String("record", "", "write the result into the rebuild check of the manifest, with this name of the machine, for example ubuntu-24.04/x86_64/4c/16G")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 1 || !release.IsName(flags.Arg(0)) || *record != "" && *work == "" || *against != "" && *work == "" {
		fmt.Fprintln(stderr, "usage: "+releaseUsage)
		if *work == "" && (*record != "" || *against != "") {
			fmt.Fprintln(stderr, "m2 release verify: -record and -against need -oleans")
		}
		return ExitUsage
	}
	name := flags.Arg(0)
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 release verify: %v\n", err)
		return ExitFailure
	}
	path := filepath.Join(*root, filepath.FromSlash(release.ManifestFile(name)))
	m, err := release.Read(path)
	if err != nil {
		return fail(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	rc := &release.RebuildCheck{Runner: *record}
	g, err := release.RebuildGraph(ctx, *root, m.Commit)
	if err != nil {
		return fail(err)
	}
	want := m.Artefacts[release.Graph]
	rc.GraphHashReproduced = g.Hash == want.Hash
	report(stdout, "graph", g.Hash, want.Hash)
	ok := rc.GraphHashReproduced
	if *work != "" {
		dir, err := release.RebuildOleans(ctx, *root, m.Commit, *work)
		if err != nil {
			return fail(err)
		}
		o, err := release.Hash(dir, release.IsOlean)
		if err != nil {
			return fail(err)
		}
		want := m.Artefacts[release.Oleans]
		rc.OleansHashReproduced = o.Hash == want.Hash
		report(stdout, "oleans", o.Hash, want.Hash)
		ok = ok && rc.OleansHashReproduced
		if !rc.OleansHashReproduced && *against != "" {
			ds, err := release.Compare(*against, dir)
			if err != nil {
				return fail(err)
			}
			for _, d := range ds {
				fmt.Fprintf(stdout, "  %s\n", d.Path)
				rc.OleanDifferences = append(rc.OleanDifferences, release.Explanation{Path: d.Path})
			}
		}
	}
	if *record != "" {
		m.RebuildCheck = rc
		data, err := m.Marshal()
		if err != nil {
			return fail(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil { // #nosec G306 G703 -- the manifest is part of the working tree.
			return fail(err)
		}
		fmt.Fprintf(stdout, "m2 release verify wrote the rebuild check into %s.", release.ManifestFile(name))
		if len(rc.OleanDifferences) > 0 {
			fmt.Fprint(stdout, " Write the reason of each different file into olean_differences (rule R02).")
		}
		fmt.Fprintln(stdout)
	}
	if !ok {
		return ExitFailure
	}
	return ExitOK
}

// report writes the result of the rebuild of one artefact.
func report(w io.Writer, artefact, got, want string) {
	if got == want {
		fmt.Fprintf(w, "%s: the rebuild gives the hash of the manifest, %s\n", artefact, got)
		return
	}
	fmt.Fprintf(w, "%s: the rebuild gives %s, and the manifest has %q\n", artefact, got, want)
}
