package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/tamnd/math2-agent/wpcorpus"
)

// runWPCensus runs m2 wp census report, which writes the completeness report
// of WP4.9, and m2 wp census diff, which prints the readings of one article
// side by side (WP4.3).
func runWPCensus(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "report" && args[0] != "diff") {
		fmt.Fprintln(stderr, "usage: m2 wp census report [-corpus DIR]\n       m2 wp census diff [-corpus DIR] PAGE")
		return ExitUsage
	}
	sub := args[0]
	fs := flag.NewFlagSet("m2 wp census "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	corpus := fs.String("corpus", ".", "the Wikipedia corpus (a clone of tamnd/wikipedia-math)")
	if err := fs.Parse(args[1:]); err != nil {
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 wp census %s: %v\n", sub, err)
		return ExitFailure
	}
	if sub == "diff" {
		if fs.NArg() != 1 {
			fmt.Fprintln(stderr, "m2 wp census diff: give one page id")
			return ExitUsage
		}
		id, err := strconv.Atoi(fs.Arg(0))
		if err != nil {
			fmt.Fprintf(stderr, "m2 wp census diff: the page id %q is not a number\n", fs.Arg(0))
			return ExitUsage
		}
		p, err := wpcorpus.ReadContent(*corpus, id)
		if err != nil {
			return fail(err)
		}
		c, err := wpcorpus.ReadCensus(*corpus, p.ID, p.Revision)
		if err != nil {
			return fail(err)
		}
		if err := wpcorpus.WriteReadings(stdout, c); err != nil {
			return fail(err)
		}
		return ExitOK
	}
	as, err := wpcorpus.ReadCensusArticles(*corpus)
	if err != nil {
		return fail(err)
	}
	path := filepath.Join(*corpus, wpcorpus.CensusReportPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- the corpus is a public repository
		return fail(err)
	}
	if err := writeAtomic(path, func(w io.Writer) error { return wpcorpus.CensusReport(w, as) }); err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "%d articles in scope, report %s\n", len(as), wpcorpus.CensusReportPath)
	return ExitOK
}
