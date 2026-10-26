package cli

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"slices"
	"text/tabwriter"
	"time"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/review"
	"github.com/tamnd/math2-agent/status"
)

// runReviewSample draws the re-review sample of a month (spec 10.8).
func runReviewSample(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("m2 review sample", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree")
	out := flags.String("out", "", "the output directory of m2 build. Empty means build/ in the tree.")
	month := flags.String("month", time.Now().UTC().Format("2006-01"), "the month of the sample, YYYY-MM")
	seed := flags.String("seed", "", "the seed of the draw. Empty means a new random seed.")
	write := flags.Bool("write", false, "add the sample to registers/samples.reg")
	asJSON := flags.Bool("json", false, "write the sample as JSON")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+reviewUsage)
		return ExitUsage
	}
	if *out == "" {
		*out = filepath.Join(*root, "build")
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 review sample: %v\n", err)
		return ExitFailure
	}
	if *seed == "" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		*seed = hex.EncodeToString(b)
	}
	in, err := reviewInput(*root, *out)
	if err != nil {
		return fail(err)
	}
	reviewers, err := status.LoadReviewers(*root)
	if err != nil {
		return fail(err)
	}
	old, err := review.LoadSamples(*root)
	if err != nil {
		return fail(err)
	}
	if *write && slices.ContainsFunc(old, func(s review.Sample) bool { return s.Month == *month }) {
		return fail(fmt.Errorf("%s has the sample of %s. A month has one sample", review.SamplesFile, *month))
	}
	samples, err := review.Draw(in, reviewers, *month, *seed)
	if err != nil {
		return fail(err)
	}
	if *write && len(samples) > 0 {
		if err := review.AppendSamples(*root, samples); err != nil {
			return fail(err)
		}
	}
	if *asJSON {
		return writeJSON(stdout, stderr, "m2 review sample", samples)
	}
	if len(samples) == 0 {
		fmt.Fprintf(stdout, "No binding is at F2 or F3, thus the sample of %s is empty.\n", *month)
		return ExitOK
	}
	fmt.Fprintf(stdout, "The sample of %s with the seed %s has %s.\n\n", *month, *seed, count(len(samples), "binding", "bindings"))
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TAG\tLEVEL\tDECLARATION\tREVIEWER")
	for _, s := range samples {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.Tag, s.Level, s.Decl, or(s.Reviewer, "none can do it"))
	}
	_ = tw.Flush()
	if *write {
		fmt.Fprintf(stdout, "\nThe sample is in %s. Commit the register, and give each reviewer the tag. The reviewer runs m2 review -blind TAG.\n", review.SamplesFile)
	} else {
		fmt.Fprintf(stdout, "\nGive -write to add the sample to %s. The same seed gives the same sample.\n", review.SamplesFile)
	}
	return ExitOK
}

// runReviewMeasure shows the re-reviews of the samples and the disagreement of each level.
func runReviewMeasure(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("m2 review measure", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree")
	asJSON := flags.Bool("json", false, "write the results and the rates as JSON")
	if err := parseInterleaved(flags, args); err != nil {
		return ExitUsage
	}
	if flags.NArg() > 1 {
		fmt.Fprintln(stderr, "m2 review measure: give one month, or none for each month")
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 review measure: %v\n", err)
		return ExitFailure
	}
	samples, err := review.LoadSamples(*root)
	if err != nil {
		return fail(err)
	}
	if m := flags.Arg(0); m != "" {
		samples = slices.DeleteFunc(samples, func(s review.Sample) bool { return s.Month != m })
	}
	reviews, err := status.LoadReviews(*root)
	if err != nil {
		return fail(err)
	}
	nodes, err := node.Load(*root)
	if err != nil {
		return fail(err)
	}
	results, rates := review.Measure(samples, reviews, nodes)
	if *asJSON {
		return writeJSON(stdout, stderr, "m2 review measure", struct {
			Results []review.Result `json:"results"`
			Rates   []review.Rate   `json:"rates"`
		}{results, rates})
	}
	if len(results) == 0 {
		if m := flags.Arg(0); m != "" {
			fmt.Fprintf(stdout, "%s has no sample of %s.\n", review.SamplesFile, m)
		} else {
			fmt.Fprintf(stdout, "%s has no sample.\n", review.SamplesFile)
		}
		return ExitOK
	}
	done, disagree := 0, 0
	for _, r := range results {
		switch r.State {
		case review.Agrees:
			done++
		case review.Disagrees:
			done++
			disagree++
		}
	}
	verb := "disagree"
	if disagree == 1 {
		verb = "disagrees"
	}
	fmt.Fprintf(stdout, "%s, %s done, %d %s.\n\n", count(len(results), "sample", "samples"), count(done, "re-review", "re-reviews"), disagree, verb)
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "LEVEL\tAREA\tDRAWN\tDONE\tDISAGREE\tRATE")
	for _, r := range rates {
		share := "-"
		if s := r.Share(); !math.IsNaN(s) {
			share = fmt.Sprintf("%.1f%%", 100*s)
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%s\n", r.Level, r.Area, r.Drawn, r.Done, r.Disagrees, share)
	}
	_ = tw.Flush()
	for _, sec := range []struct{ state, head string }{{review.Disagrees, "Disagree"}, {review.Open, "Open"}, {review.Unassigned, "No reviewer"}} {
		first := true
		for _, r := range results {
			if r.State != sec.state {
				continue
			}
			if first {
				fmt.Fprintf(stdout, "\n%s\n", sec.head)
				first = false
			}
			switch sec.state {
			case review.Disagrees:
				fmt.Fprintf(stdout, "  %s %s %s: %s by %s, %s\n", r.Month, r.Tag, r.Decl, r.Review, r.Reviewer, or(r.Class, "no class"))
			case review.Open:
				fmt.Fprintf(stdout, "  %s %s %s: %s\n", r.Month, r.Tag, r.Decl, r.Reviewer)
			default:
				fmt.Fprintf(stdout, "  %s %s %s\n", r.Month, r.Tag, r.Decl)
			}
		}
	}
	return ExitOK
}

// count returns n with the singular or the plural noun.
func count(n int, one, more string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, more)
}
