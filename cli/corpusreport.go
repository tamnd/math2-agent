package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/stage"
	"github.com/tamnd/math2-agent/status"
)

// runCorpusReport prints the cost, the acceptance rate and the review hours of each stage of the corpus (corpus K12.5 to K12.7). It reads the run records, the provenance logs, the reviews and the inventory, and it writes no file.
func runCorpusReport(args []string, stdout, stderr io.Writer) int {
	const name = "m2 corpus report"
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree")
	store := flags.String("store", "", "the directory of the run records. Empty means build/ in the tree.")
	from := flags.String("from", "", "the first day of the report, YYYY-MM-DD. Empty means no first day.")
	to := flags.String("to", "", "the last day of the report, YYYY-MM-DD. Empty means no last day.")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: m2 corpus report [-root DIR] [-store DIR] [-from YYYY-MM-DD] [-to YYYY-MM-DD] [-json]")
		return ExitUsage
	}
	in := stage.Input{Made: reportNow().UTC()}
	for _, d := range []struct {
		flag string
		t    *time.Time
		next bool
	}{{*from, &in.From, false}, {*to, &in.To, true}} {
		if d.flag == "" {
			continue
		}
		t, err := time.Parse(time.DateOnly, d.flag)
		if err != nil {
			fmt.Fprintf(stderr, "%s: the day %q is not YYYY-MM-DD\n", name, d.flag)
			return ExitUsage
		}
		if d.next {
			t = t.AddDate(0, 0, 1)
		}
		*d.t = t
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	if *store == "" {
		*store = filepath.Join(*root, "build")
	}
	var err error
	if in.Records, err = agent.ReadRecords(*store); err != nil {
		return fail(err)
	}
	if in.Logs, err = prov.ReadAll(*root); err != nil {
		return fail(err)
	}
	if in.Reviews, err = status.LoadReviews(*root); err != nil {
		return fail(err)
	}
	if in.Rows, err = inventory.ReadRows(*root); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fail(err)
	}
	rep := stage.Measure(in)
	if *asJSON {
		return writeJSON(stdout, stderr, name, rep)
	}
	if _, err := stdout.Write(rep.Markdown()); err != nil {
		return ExitFailure
	}
	return ExitOK
}
