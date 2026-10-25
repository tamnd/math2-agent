package cli

import (
	"flag"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/tamnd/math2-agent/tag"
)

func runMint(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 mint", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	commit := fs.String("commit", "", "the commit that goes into registers/tags.reg. Empty means the HEAD commit of the working tree.")
	seed := fs.Bool("seed", false, "mint tags that start with 0, for the nodes of the M1 seed")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: m2 mint [-root DIR] [-commit C] [-seed]")
		return ExitUsage
	}
	if *commit == "" {
		out, err := exec.Command("git", "-C", *root, "rev-parse", "HEAD").Output() // #nosec G204 -- the user gives the root, and git gets it as one argument.
		if err != nil {
			fmt.Fprintln(stderr, "m2 mint: the working tree has no HEAD commit. Give the commit with -commit.")
			return ExitFailure
		}
		*commit = strings.TrimSpace(string(out))
	}
	if strings.ContainsAny(*commit, " \t\n") {
		fmt.Fprintf(stderr, "m2 mint: the commit %q has a space\n", *commit)
		return ExitUsage
	}
	rewrite := tag.Rewrite
	if *seed {
		rewrite = tag.RewriteSeed
	}
	minted, err := rewrite(*root, nil, *commit)
	if err != nil {
		fmt.Fprintf(stderr, "m2 mint: %v\n", err)
		return ExitFailure
	}
	for _, m := range minted {
		fmt.Fprintf(stdout, "%s %s %s\n", m.Provisional, m.Tag, m.Kind)
	}
	return ExitOK
}
