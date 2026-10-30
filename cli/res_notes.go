package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/ingest/arxiv"
	resnotes "github.com/tamnd/math2-agent/resource/notes"
)

const resNotesUsage = "m2 res notes [-root DIR] scan [-out FILE] [-gap D] [-pages N] | measure FILE"

// runResNotes finds lecture notes (RS7.1). scan runs the repository scan of GitHub and writes the candidates. measure gives the precision and the recall of the arXiv rule set on a labelled sample.
func runResNotes(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res notes", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "usage: "+resNotesUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 res notes: %v\n", err)
		return ExitFailure
	}
	sub, rest := fs.Arg(0), fs.Args()[1:]
	sf := flag.NewFlagSet("m2 res notes "+sub, flag.ContinueOnError)
	sf.SetOutput(stderr)
	switch sub {
	case "scan":
		out := sf.String("out", resnotes.File, "the file of the candidates, relative to the root")
		gap := sf.Duration("gap", resnotes.Gap, "the shortest time between two requests, at least 6s")
		pages := sf.Int("pages", resnotes.MaxPages, "the most pages of 100 repositories for one query, at most 10")
		if err := sf.Parse(rest); err != nil || sf.NArg() != 0 {
			fmt.Fprintln(stderr, "usage: "+resNotesUsage)
			return ExitUsage
		}
		if *gap < resnotes.MinGap {
			fmt.Fprintf(stderr, "m2 res notes scan: the gap %v is shorter than %v. The search API allows 10 requests a minute with no token.\n", *gap, resnotes.MinGap)
			return ExitUsage
		}
		path := filepath.Join(*root, *out)
		old, err := resnotes.Read(path)
		if err != nil {
			return fail(err)
		}
		s := &resnotes.Scanner{UserAgent: userAgent(), Gap: *gap, Pages: *pages, Log: stderr}
		found, counts, err := s.Scan(context.Background(), resnotes.Queries, time.Now().UTC().Format(time.DateOnly))
		if err != nil {
			return fail(err)
		}
		all, added := resnotes.Merge(old, found)
		if err := resnotes.Write(path, all); err != nil {
			return fail(err)
		}
		var dropped []string
		for why, n := range counts.Dropped {
			dropped = append(dropped, fmt.Sprintf("%d %s", n, why))
		}
		slices.Sort(dropped)
		fmt.Fprintf(stdout, "%s: %d requests, %d repositories seen, %d candidates, %d new, dropped: %s. %d candidates in %s\n",
			resnotes.Rules, counts.Requests, counts.Seen, counts.Kept, added, strings.Join(dropped, ", "), len(all), *out)
		return ExitOK
	case "measure":
		if err := sf.Parse(rest); err != nil || sf.NArg() != 1 {
			fmt.Fprintln(stderr, "usage: "+resNotesUsage)
			return ExitUsage
		}
		sample, err := arxiv.ReadLabelled(sf.Arg(0))
		if err != nil {
			return fail(err)
		}
		sc := arxiv.Measure(sample)
		fmt.Fprintf(stdout, "%s: %d papers, precision %.2f, recall %.2f (TP %d, FP %d, FN %d, TN %d)\n", sc.Rules, len(sample), sc.Precision, sc.Recall, sc.TP, sc.FP, sc.FN, sc.TN)
		if len(sc.Wrong) > 0 {
			fmt.Fprintf(stdout, "wrong: %s\n", strings.Join(sc.Wrong, " "))
		}
		return ExitOK
	default:
		fmt.Fprintf(stderr, "m2 res notes: unknown subcommand %q\nusage: %s\n", sub, resNotesUsage)
		return ExitUsage
	}
}
