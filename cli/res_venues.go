package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/adapters/series"
	resscope "github.com/tamnd/math2-agent/resource/scope"
	"github.com/tamnd/math2-agent/resource/venues"
)

const resVenuesUsage = "m2 res venues [-root DIR]"

func runResVenues(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res venues", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+resVenuesUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 res venues: %v\n", err)
		return ExitFailure
	}
	prev, err := venues.Read(*root)
	if err != nil {
		return fail(err)
	}
	rs, err := resource.ReadResources(*root)
	if err != nil {
		return fail(err)
	}
	vs, c := venues.Build(prev, rs, knownVenues())
	if err := venues.Write(*root, vs); err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "%d venues in the register; added %d, withdrew %d, %d with a scope\n", c.Venues, c.Added, c.Withdrawn, c.Scoped)
	fmt.Fprintf(stdout, "wrote %s\n", venues.File)
	return ExitOK
}

// knownVenues gives the scope core to each ISSN of the lists of mathematics series of the series adapters.
func knownVenues() map[string]venues.Known {
	out := map[string]venues.Known{}
	for _, a := range []series.Adapter{series.Springer, series.AMS} {
		for _, s := range a.Series {
			for _, n := range s.ISSN {
				out[n] = venues.Known{Scope: resscope.VenueCore, From: a.ID}
			}
		}
	}
	return out
}
