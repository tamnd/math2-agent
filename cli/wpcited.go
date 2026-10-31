package cli

import (
	"flag"
	"fmt"
	"io"

	wpImport "github.com/tamnd/math2-agent/importer/wikipedia"
)

// runWPCited writes the Wikipedia titles that a math2 tree cites, one on each line. The output is the -cited file of m2 wp scope (signal S4).
func runWPCited(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 wp cited", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 tree")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(stderr, "usage: m2 wp cited [-root DIR]")
		return ExitUsage
	}
	titles, bad, err := wpImport.Cited(*root)
	if err != nil {
		fmt.Fprintf(stderr, "m2 wp cited: %v\n", err)
		return ExitFailure
	}
	for _, t := range titles {
		fmt.Fprintln(stdout, t)
	}
	fmt.Fprintf(stderr, "%d titles, %d files did not parse\n", len(titles), bad)
	return ExitOK
}
