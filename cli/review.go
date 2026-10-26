package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/review"
	"github.com/tamnd/math2-agent/status"
)

const reviewUsage = "m2 review [-root DIR] [-out DIR] [-decl D] [-json] [-reviewer ID -verdict V [-convention C]... [-class M-X] [-notes TEXT] [-date D] [-dry-run] [-commit]] <tag|alias|uri>"

func runReview(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("m2 review", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree")
	out := flags.String("out", "", "the output directory of m2 build. Empty means build/ in the tree.")
	decl := flags.String("decl", "", "the declaration or the adapter of the binding. Empty means the only binding.")
	asJSON := flags.Bool("json", false, "write the packet, or the review, as JSON")
	width := flags.Int("width", 0, "the width of the text in columns. Zero means $COLUMNS, or 100.")
	reviewer := flags.String("reviewer", os.Getenv("M2_REVIEWER"), "orcid: and your ORCID iD, or github: and your GitHub login. The default is $M2_REVIEWER.")
	verdict := flags.String("verdict", "", "faithful, faithful-with-conventions or not-faithful. Without a verdict, m2 review shows the packet.")
	var conventions []string
	flags.Func("convention", "a convention, for the verdict faithful-with-conventions. Give the flag once for each convention.", func(s string) error {
		conventions = append(conventions, s)
		return nil
	})
	class := flags.String("class", "", "the class of the fault, for the verdict not-faithful, for example M-junk")
	notes := flags.String("notes", "", "free text")
	date := flags.String("date", "", "the date of the review, YYYY-MM-DD. Empty means today in UTC.")
	dryRun := flags.Bool("dry-run", false, "check the review and show it, and change no file")
	commit := flags.Bool("commit", false, "commit the two files with git commit -S, which signs the commit with your key")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "m2 review: give one tag, alias or m2:// URI")
		return ExitUsage
	}
	if *verdict == "" && (*dryRun || *commit) {
		fmt.Fprintln(stderr, "m2 review: -dry-run and -commit need a verdict")
		return ExitUsage
	}
	if *out == "" {
		*out = filepath.Join(*root, "build")
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 review: %v\n", err)
		return ExitFailure
	}

	in, err := reviewInput(*root, *out)
	if err != nil {
		return fail(err)
	}
	found, err := resolve(*root, in.Status, flags.Arg(0))
	if err != nil {
		return fail(err)
	}
	if len(found) != 1 {
		return fail(fmt.Errorf("%s names %d nodes. Give a tag", flags.Arg(0), len(found)))
	}
	p, err := review.Make(in, found[0].Tag)
	if err != nil {
		return fail(err)
	}

	if *verdict == "" {
		if *asJSON {
			return writeJSON(stdout, stderr, "m2 review", p)
		}
		if *decl != "" {
			b, err := p.Binding(*decl)
			if err != nil {
				return fail(err)
			}
			p.Bindings = []review.Binding{b}
		}
		if *width == 0 {
			*width, _ = strconv.Atoi(os.Getenv("COLUMNS"))
		}
		if *width == 0 {
			*width = 100
		}
		if err := p.WriteText(stdout, *width); err != nil {
			return fail(err)
		}
		return ExitOK
	}

	b, err := p.Binding(*decl)
	if err != nil {
		return fail(err)
	}
	r := status.Review{Reviewer: *reviewer, Date: *date, Verdict: *verdict, Conventions: conventions, Class: *class, Notes: *notes}
	var paths []string
	if *dryRun {
		r, err = review.Fill(p, b, r)
	} else {
		r, paths, err = review.Write(*root, p, b, r)
	}
	if err != nil {
		return fail(err)
	}
	if *commit {
		if err := gitCommit(*root, "Review the binding of "+p.Tag, paths); err != nil {
			fmt.Fprintf(stderr, "m2 review: the review %s is in %s, and the commit failed: %v\n", r.ID, paths[0], err)
			return ExitFailure
		}
	}
	if *asJSON {
		return writeJSON(stdout, stderr, "m2 review", r)
	}
	switch {
	case *dryRun:
		fmt.Fprintf(stdout, "The review is valid. With no -dry-run, m2 review writes it to %s.\n", review.File(p.Tag))
	case *commit:
		fmt.Fprintf(stdout, "The review %s is in %s and %s, in a signed commit.\n", r.ID, paths[0], paths[1])
	default:
		fmt.Fprintf(stdout, "The review %s is in %s and %s. Commit the two files with git commit -S.\n", r.ID, paths[0], paths[1])
	}
	return ExitOK
}

// reviewInput reads the files of the build in out and the tree in root that a review packet uses.
func reviewInput(root, out string) (review.Input, error) {
	in := review.Input{Records: map[string]lean.Record{}}
	var err error
	if in.Status, err = build.ReadStatus(out); errors.Is(err, fs.ErrNotExist) {
		return in, fmt.Errorf("%s does not exist. Run m2 build first", filepath.Join(out, build.StatusFile))
	} else if err != nil {
		return in, err
	}
	if in.Nodes, err = node.Load(root); err != nil {
		return in, err
	}
	if in.Authors, err = status.LoadAuthors(root); err != nil {
		return in, err
	}
	x, err := build.ReadExtraction(out)
	switch {
	case err == nil:
		in.Bindings = x.Bindings
		for _, r := range x.Records {
			in.Records[r.Decl] = r
		}
	case !errors.Is(err, fs.ErrNotExist):
		return in, err
	}
	if in.Battery, err = build.ReadBattery(out); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return in, err
	}
	return in, nil
}

// gitCommit commits the paths in the tree in root with a signature. git uses the signing key of the user.
func gitCommit(root, msg string, paths []string) error {
	for _, args := range [][]string{append([]string{"add", "--"}, paths...), append([]string{"commit", "-S", "-q", "-m", msg, "--"}, paths...)} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...) // #nosec G204 -- the arguments are fixed in this file, and the root and the paths are single arguments.
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %w: %s", args[0], err, out)
		}
	}
	return nil
}
