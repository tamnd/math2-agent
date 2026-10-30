package cli

import (
	"flag"
	"fmt"
	"io"

	resrecords "github.com/tamnd/math2-agent/resource/records"
)

const resRecordsUsage = "m2 res records export [-root DIR] -out DIR | m2 res records keep [-root DIR] | m2 res records pin [-root DIR] -out DIR -commit C"

func runResRecords(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: "+resRecordsUsage)
		return ExitUsage
	}
	fs := flag.NewFlagSet("m2 res records "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	out := fs.String("out", "", "the working tree of "+resrecords.Repo)
	commit := fs.String("commit", "", "the commit of "+resrecords.Repo+" that the pin names")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+resRecordsUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 res records %s: %v\n", args[0], err)
		return ExitFailure
	}
	switch args[0] {
	case "export":
		if *out == "" {
			fmt.Fprintln(stderr, "usage: "+resRecordsUsage)
			return ExitUsage
		}
		shards, err := resrecords.Export(*root, *out)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(stdout, "wrote %d shards, %s and %s to %s\n", len(shards), resrecords.IndexFile, resrecords.ParquetFile, *out)
	case "keep":
		kept, moved, err := resrecords.Keep(*root)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(stdout, "kept %d resources of the tiers T1, T2 and T5, and removed %d resources from the tree\n", kept, moved)
	case "pin":
		if *out == "" || *commit == "" {
			fmt.Fprintln(stderr, "usage: "+resRecordsUsage)
			return ExitUsage
		}
		p, err := resrecords.WritePin(*root, *out, *commit)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(stdout, "wrote %s: %s at %s, %d shards\n", resrecords.PinFile, p.Repo, p.Commit, p.Shards)
	default:
		fmt.Fprintln(stderr, "usage: "+resRecordsUsage)
		return ExitUsage
	}
	return ExitOK
}
