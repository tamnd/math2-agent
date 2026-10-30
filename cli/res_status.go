package cli

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/tamnd/math2-agent/resource/status"
)

const resStatusUsage = "m2 res status [-root DIR] [-day YYYY-MM-DD]"

func runResStatus(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	day := fs.String("day", time.Now().UTC().Format(time.DateOnly), "the date of the report")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+resStatusUsage)
		return ExitUsage
	}
	if _, err := time.Parse(time.DateOnly, *day); err != nil {
		fmt.Fprintf(stderr, "m2 res status: the day %q is not YYYY-MM-DD\n", *day)
		return ExitUsage
	}
	rep, err := status.Build(*root, *day)
	if err == nil {
		err = rep.Write(*root)
	}
	if err != nil {
		fmt.Fprintf(stderr, "m2 res status: %v\n", err)
		return ExitFailure
	}
	for _, c := range rep.Coverage {
		fmt.Fprintf(stdout, "%s: %d read, %d rejected, %d mapped, coverage %.2f%%\n", c.Source, c.Read, c.Rejected, c.Mapped, 100*c.Coverage)
	}
	fmt.Fprintf(stdout, "%d live resources, %d open decisions, %d of %d curriculum texts with no resource\n", rep.Resources, rep.Open.Total, len(rep.Closure.Missing), rep.Closure.Texts)
	fmt.Fprintf(stdout, "wrote %s and %s\n", status.ReportFile, status.JSONFile)
	return ExitOK
}
