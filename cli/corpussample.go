package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"text/tabwriter"

	"github.com/tamnd/math2-agent/inventory"
)

// SamplesDir is the directory of the audit packets of the monthly samples, in the tree.
const SamplesDir = "reviews/samples"

var monthIn = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// runCorpusSample draws the monthly sample audit of a source and writes its audit packets (corpus arxiv AX11.5). It calls no model.
func runCorpusSample(args []string, stdout, stderr io.Writer) int {
	const name = "m2 corpus sample"
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree")
	month := flags.String("month", "", "the month of the sample, YYYY-MM")
	source := flags.String("source", "", "the source. Only arxiv has strata now.")
	seed := flags.String("seed", "", "the seed of the draw. Empty means the month.")
	write := flags.Bool("write", false, "write the audit packets to "+SamplesDir+"/YYYY-MM-SOURCE.jsonl")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 0 || !monthIn.MatchString(*month) || *source == "" {
		fmt.Fprintln(stderr, "usage: m2 corpus sample -month YYYY-MM -source arxiv [-seed S] [-root DIR] [-write] [-json]")
		return ExitUsage
	}
	if *source != "arxiv" {
		fmt.Fprintf(stderr, "%s: the source %s has no strata. Only arxiv has strata now.\n", name, *source)
		return ExitUsage
	}
	if *seed == "" {
		*seed = *month
	}
	ms, err := inventory.ReadMentions(*root)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	rows, err := inventory.ReadRows(*root)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	strata, err := inventory.DrawArxiv(rows, ms, *month, *seed)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitUsage
	}
	path := filepath.Join(*root, SamplesDir, *month+"-"+*source+".jsonl")
	if *write {
		if _, err := os.Stat(path); err == nil {
			fmt.Fprintf(stderr, "%s: %s exists. The sample of a month is drawn once.\n", name, path)
			return ExitFailure
		} else if !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return ExitFailure
		}
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		for _, s := range strata {
			for _, p := range s.Packets {
				if err := enc.Encode(p); err != nil {
					fmt.Fprintf(stderr, "%s: %v\n", name, err)
					return ExitFailure
				}
			}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return ExitFailure
		}
		if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return ExitFailure
		}
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(strata); err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return ExitFailure
		}
		return ExitOK
	}
	fmt.Fprintf(stdout, "The sample of %s for %s with the seed %s.\n\n", *month, *source, *seed)
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "STRATUM\tUNIT\tPOPULATION\tDRAWN\tNOTE")
	total := 0
	for _, s := range strata {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\n", s.Name, s.Unit, s.Population, len(s.Packets), s.Note)
		total += len(s.Packets)
	}
	if err := tw.Flush(); err != nil {
		return ExitFailure
	}
	if *write {
		fmt.Fprintf(stdout, "\nWrote %d audit packets to %s.\n", total, path)
	} else {
		fmt.Fprintf(stdout, "\nGive -write to write the %d audit packets to %s.\n", total, path)
	}
	return ExitOK
}
