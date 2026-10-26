package cli

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/dataset"
	"github.com/tamnd/math2-agent/prov"
)

const agentDatasetUsage = "m2 agent dataset [-root DIR] [-store DIR] [-quarter YYYY-Qn] [-o FILE]"

// runAgentDataset writes the dataset of the runs of a quarter as JSON Lines (spec 17.4).
func runAgentDataset(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("m2 agent dataset", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree")
	store := flags.String("store", "", "the directory of the run records. Empty means build/ in the tree.")
	quarter := flags.String("quarter", "", "the quarter of the dataset, for example 2026-Q4. Empty means the last full quarter.")
	out := flags.String("o", "", "write the dataset to this file, and not to the standard output")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "Usage: %s\n", agentDatasetUsage)
		return ExitUsage
	}
	q := dataset.QuarterOf(dataset.QuarterOf(reportNow()).Start().AddDate(0, 0, -1))
	if *quarter != "" {
		var err error
		if q, err = dataset.ParseQuarter(*quarter); err != nil {
			fmt.Fprintf(stderr, "m2 agent dataset: %v\n", err)
			return ExitUsage
		}
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 agent dataset: %v\n", err)
		return ExitFailure
	}
	if *store == "" {
		*store = filepath.Join(*root, "build")
	}
	recs, err := agent.ReadRecords(*store)
	if err != nil {
		return fail(err)
	}
	logs, err := prov.ReadAll(*root)
	if err != nil {
		return fail(err)
	}
	w := stdout
	if *out != "" {
		f, err := os.Create(*out) // #nosec G304 -- the file that the caller names.
		if err != nil {
			return fail(err)
		}
		defer func() { _ = f.Close() }()
		w = f
	}
	bw := bufio.NewWriter(w)
	enc := json.NewEncoder(bw)
	rows := dataset.Make(q, recs, logs)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			return fail(err)
		}
	}
	if err := bw.Flush(); err != nil {
		return fail(err)
	}
	if *out != "" {
		fmt.Fprintf(stdout, "wrote %d runs of %s to %s\n", len(rows), q, *out)
	}
	return ExitOK
}
