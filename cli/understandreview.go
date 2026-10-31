package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/tamnd/math2-agent/agent/understand"
)

// runCorpusUnderstand runs m2 corpus understand review, which prints the review queues of the passes of understand, and m2 corpus understand gold, which measures the passes unfold and classify on the gold sets U and C (corpus arxiv AX11.2 and AX11.3).
func runCorpusUnderstand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "review" && args[0] != "gold" {
		fmt.Fprintln(stderr, "usage: "+corpusUsage)
		return ExitUsage
	}
	name := "m2 corpus understand " + args[0]
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	asJSON := fs.Bool("json", false, "print JSON")
	items := fs.Bool("items", false, "with review, print the id of each record in a queue")
	if err := fs.Parse(args[1:]); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+corpusUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	var v any
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	if args[0] == "review" {
		qs, err := understand.Reviews(*root)
		if err != nil {
			return fail(err)
		}
		v = qs
		fmt.Fprintln(tw, "pass\tqueue\trecords")
		for _, q := range qs {
			fmt.Fprintf(tw, "%s\t%s\t%d\n", q.Pass, q.Name, len(q.Items))
			if *items {
				for _, id := range q.Items {
					fmt.Fprintf(tw, "\t\t%s\n", id)
				}
			}
		}
		if len(qs) == 0 {
			fmt.Fprintln(tw, "-\t-\t0")
		}
	} else {
		gu, err := understand.ReadGoldUnfold(*root)
		if err != nil {
			return fail(err)
		}
		gc, err := understand.ReadGoldClassify(*root)
		if err != nil {
			return fail(err)
		}
		if len(gu) == 0 && len(gc) == 0 {
			return fail(fmt.Errorf("the tree has no gold set in %s or %s", understand.GoldUnfoldFile, understand.GoldClassifyFile))
		}
		ru, err := understand.ReadAll(*root, understand.Unfold)
		if err != nil {
			return fail(err)
		}
		rc, err := understand.ReadAll(*root, understand.Classify)
		if err != nil {
			return fail(err)
		}
		scores := append(understand.MeasureUnfold(ru, gu), understand.MeasureClassify(rc, gc))
		v = scores
		fmt.Fprintln(tw, "set\tmeasure\tmarks\twith a record\tcounted\trate\ttarget\t")
		for _, s := range scores {
			r, met := "-", ""
			if s.Rate != nil {
				r = fmt.Sprintf("%.1f%%", 100**s.Rate)
				met = "miss"
				if s.Met() {
					met = "met"
				}
			}
			fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%s\t%.0f%%\t%s\n", s.Set, s.Measure, s.Marks, s.Matched, s.N, r, 100*s.Target, met)
		}
	}
	if *asJSON {
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fail(err)
		}
		if _, err := stdout.Write(append(data, '\n')); err != nil {
			return ExitFailure
		}
		return ExitOK
	}
	if err := tw.Flush(); err != nil {
		return ExitFailure
	}
	return ExitOK
}
