package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	"github.com/tamnd/math2-agent/resource/competitions"
)

const resCompetitionsUsage = "m2 res competitions [-root DIR] [-wikidata URL|none] [-year YYYY]"

func runResCompetitions(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res competitions", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	wd := fs.String("wikidata", competitions.Endpoint, "the SPARQL service of Wikidata, or none to read only the register")
	year := fs.Int("year", time.Now().UTC().Year(), "the current year. The gaps end the year before it")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+resCompetitionsUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 res competitions: %v\n", err)
		return ExitFailure
	}
	var found []competitions.Competition
	if *wd != "none" {
		var err error
		if found, err = competitions.FromWikidata(context.Background(), fetch.NewClient(time.Second).Get, *wd); err != nil {
			return fail(err)
		}
	}
	prev, err := competitions.Read(*root)
	if err != nil {
		return fail(err)
	}
	rs, err := resource.ReadResources(*root)
	if err != nil {
		return fail(err)
	}
	cs, c := competitions.Build(prev, found, rs)
	if err := competitions.Write(*root, cs); err != nil {
		return fail(err)
	}
	gaps := competitions.Gaps(cs, rs, *year)
	years := 0
	for _, g := range gaps {
		years += len(g.Years)
	}
	fmt.Fprintf(stdout, "%d competitions in the register, %d from Wikidata; added %d, withdrew %d, %d with a resource\n", len(cs), c.Wikidata, c.Added, c.Withdrawn, c.Linked)
	fmt.Fprintf(stdout, "%d competitions have a gap, with %d missing years in all\n", len(gaps), years)
	for _, g := range gaps[:min(len(gaps), 20)] {
		if g.Reason != "" {
			fmt.Fprintf(stdout, "  %s: %s\n", g.Name, g.Reason)
		} else {
			fmt.Fprintf(stdout, "  %s: %s\n", g.Name, strings.Trim(fmt.Sprint(g.Years), "[]"))
		}
	}
	fmt.Fprintf(stdout, "wrote %s\n", competitions.File)
	return ExitOK
}
