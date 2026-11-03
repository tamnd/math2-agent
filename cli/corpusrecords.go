package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/tamnd/math2-agent/importer/arxiv/axrecords"
)

const corpusRecordsUsage = "m2 corpus records export|unpack [-root DIR] -out DIR | m2 corpus records pin [-root DIR] -out DIR -commit C | m2 corpus records keep [-root DIR]"

// runCorpusRecords moves the arXiv records of the record plane between the math2 tree and tamnd/math2-records (corpus arxiv AQ11).
func runCorpusRecords(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: "+corpusRecordsUsage)
		return ExitUsage
	}
	fs := flag.NewFlagSet("m2 corpus records "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	out := fs.String("out", "", "the working tree of "+axrecords.Repo)
	commit := fs.String("commit", "", "the commit of "+axrecords.Repo+" that the pin names")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+corpusRecordsUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 corpus records %s: %v\n", args[0], err)
		return ExitFailure
	}
	needOut := args[0] == "export" || args[0] == "unpack" || args[0] == "pin"
	if needOut && *out == "" || args[0] == "pin" && *commit == "" {
		fmt.Fprintln(stderr, "usage: "+corpusRecordsUsage)
		return ExitUsage
	}
	switch args[0] {
	case "export":
		shards, err := axrecords.Export(*root, *out)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(stdout, "wrote %d shards and %s to %s\n", len(shards), axrecords.IndexFile, *out)
	case "keep":
		files, rows, err := axrecords.Keep(*root)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(stdout, "removed %d files of arXiv mentions and understand records, and %d research rows, from the tree; they are in %s\n", files, rows, axrecords.Repo)
	case "pin":
		p, err := axrecords.WritePin(*root, *out, *commit)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(stdout, "wrote %s: %s at %s, %d shards\n", axrecords.PinFile, p.Repo, p.Commit, p.Shards)
	case "unpack":
		files, rows, err := axrecords.Unpack(*root, *out)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(stdout, "added %d files and %d research rows from %s\n", files, rows, *out)
	default:
		fmt.Fprintln(stderr, "usage: "+corpusRecordsUsage)
		return ExitUsage
	}
	return ExitOK
}
