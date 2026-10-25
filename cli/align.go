package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/align"
	"github.com/tamnd/math2-agent/ingest"
)

const alignUsage = `m2 align [-root DIR] [-threshold T] [-close C] [-margin M] [-k K] [-date YYYY-MM-DD] [-n] <source>
m2 align queue [-root DIR] [-system S] [-json]
m2 align accept -by PERSON [-root DIR] [-date YYYY-MM-DD] [-note TEXT] [-match M] <id>...
m2 align reject -by PERSON [-root DIR] [-date YYYY-MM-DD] <id>...
m2 align confirm -by PERSON [-root DIR] [-date YYYY-MM-DD] <tag> <system>`

func runAlign(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "queue":
			return runAlignQueue(args[1:], stdout, stderr)
		case "accept", "reject", "confirm":
			return runAlignDecide(args[0], args[1:], stdout, stderr)
		}
	}
	fs := flag.NewFlagSet("m2 align", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	var opt align.Options
	fs.Float64Var(&opt.Threshold, "threshold", 0.9, "the lowest score of a same_as or aligned proposal")
	fs.Float64Var(&opt.Close, "close", 0.75, "the lowest score of a close_to proposal")
	fs.Float64Var(&opt.Margin, "margin", 0.05, "how much lower the second candidate must be for a same_as proposal")
	fs.IntVar(&opt.K, "k", 5, "the number of candidates for each node")
	fs.StringVar(&opt.Date, "date", "", "the date of the run. Empty means today in UTC.")
	dry := fs.Bool("n", false, "show the proposals and do not write the queue")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	var source string
	if fs.NArg() > 0 {
		source = fs.Arg(0)
		if err := fs.Parse(fs.Args()[1:]); err != nil {
			return ExitUsage
		}
	}
	if source == "" || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+alignUsage)
		return ExitUsage
	}
	if opt.Date == "" {
		opt.Date = time.Now().UTC().Format(time.DateOnly)
	}
	opt.By = "m2/" + version + " align"
	res, err := align.Propose(*root, source, opt)
	if err != nil {
		fmt.Fprintf(stderr, "m2 align: %v\n", err)
		return ExitFailure
	}
	count := map[string]int{}
	for _, p := range res.Proposals {
		count[p.Edge]++
	}
	fmt.Fprintf(stdout, "%s: %d same_as, %d close_to, %d aligned and %d duplicate proposals for %d nodes\n",
		source, count[align.SameAs], count[align.CloseTo], count[align.Aligned], len(res.Duplicates), res.Nodes)
	titles := titlesOf(*root, source)
	for _, p := range append(res.Proposals, res.Duplicates...) {
		writeProposal(stdout, p, titles)
	}
	if *dry {
		return ExitOK
	}
	if err := res.Write(*root); err != nil {
		fmt.Fprintf(stderr, "m2 align: %v\n", err)
		return ExitFailure
	}
	return ExitOK
}

func runAlignQueue(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 align queue", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	system := fs.String("system", "", "show only the proposals of this system, or math2 for the duplicates")
	asJSON := fs.Bool("json", false, "write the proposals as JSON lines")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+alignUsage)
		return ExitUsage
	}
	ps, err := align.ReadQueue(*root)
	if err != nil {
		fmt.Fprintf(stderr, "m2 align queue: %v\n", err)
		return ExitFailure
	}
	titles := map[string]map[string]string{}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	for _, p := range ps {
		if *system != "" && p.System != *system {
			continue
		}
		if *asJSON {
			if err := enc.Encode(p); err != nil {
				fmt.Fprintf(stderr, "m2 align queue: %v\n", err)
				return ExitFailure
			}
			continue
		}
		if titles[p.Source] == nil {
			titles[p.Source] = titlesOf(*root, p.Source)
		}
		writeProposal(stdout, p, titles[p.Source])
	}
	return ExitOK
}

func runAlignDecide(verb string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 align "+verb, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	var r align.Review
	fs.StringVar(&r.Person, "by", "", "the person who decides: orcid: and an ORCID iD, or github: and a GitHub login")
	fs.StringVar(&r.Date, "date", "", "the date of the decision. Empty means today in UTC.")
	if verb == "accept" {
		fs.StringVar(&r.Note, "note", "", "how the node and the record differ, for a close_to edge")
		fs.StringVar(&r.Match, "match", "", "how the paper states the node, for an aligned edge: exact, stronger, weaker, variant or partial")
	}
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if r.Date == "" {
		r.Date = time.Now().UTC().Format(time.DateOnly)
	}
	if verb == "confirm" {
		if fs.NArg() != 2 {
			fmt.Fprintln(stderr, "usage: "+alignUsage)
			return ExitUsage
		}
		if err := align.Confirm(*root, fs.Arg(0), fs.Arg(1), r); err != nil {
			fmt.Fprintf(stderr, "m2 align confirm: %v\n", err)
			return ExitFailure
		}
		fmt.Fprintf(stdout, "confirmed the same_as value of %s in %s\n", fs.Arg(0), fs.Arg(1))
		return ExitOK
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "usage: "+alignUsage)
		return ExitUsage
	}
	decide := align.Accept
	if verb == "reject" {
		decide = align.Reject
	}
	for _, id := range fs.Args() {
		p, err := decide(*root, id, r)
		if err != nil {
			fmt.Fprintf(stderr, "m2 align %s: %v\n", verb, err)
			return ExitFailure
		}
		fmt.Fprintf(stdout, "%sed %s: %s %s %s:%s\n", strings.TrimSuffix(verb, "e"), p.ID, p.Node, p.Edge, p.System, p.Target)
	}
	return ExitOK
}

// writeProposal writes one proposal on one line, with the title of its record when the records have one.
func writeProposal(w io.Writer, p align.Proposal, titles map[string]string) {
	line := fmt.Sprintf("%s  %.3f  %s %s %s:%s", p.ID, p.Score, p.Node, p.Edge, p.System, p.Target)
	if t := titles[p.Target]; t != "" {
		line += "  " + t
	}
	fmt.Fprintln(w, line)
}

// titlesOf returns the title of each record of the source by its id, its locator and its URL. A tree with no records of the source has no titles.
func titlesOf(root, source string) map[string]string {
	recs, _, err := ingest.ReadRecords(root, source)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, r := range recs {
		for _, k := range []string{r.ID, r.Locator, r.URL} {
			if k != "" && r.Title != "" {
				out[k] = r.Title
			}
		}
	}
	return out
}
