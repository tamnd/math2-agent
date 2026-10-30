package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/resource/census/fetch"
	"github.com/tamnd/math2-agent/resource/institutions"
)

const resInstitutionsUsage = "m2 res institutions [-root DIR] -ror FILE|URL [-wikidata URL|none]"

func runResInstitutions(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res institutions", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	ror := fs.String("ror", "", "the ROR data dump in the schema v2, as the zip file of Zenodo or its JSON file, by path or URL")
	wd := fs.String("wikidata", institutions.Endpoint, "the SPARQL service of Wikidata, or none to add no units")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *ror == "" {
		fmt.Fprintln(stderr, "usage: "+resInstitutionsUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 res institutions: %v\n", err)
		return ExitFailure
	}
	ctx := context.Background()
	client := fetch.NewClient(time.Second)
	var data []byte
	var err error
	if strings.HasPrefix(*ror, "http://") || strings.HasPrefix(*ror, "https://") {
		data, err = client.Get(ctx, *ror)
	} else {
		data, err = os.ReadFile(*ror) // #nosec G304 -- the user names the dump.
	}
	if err != nil {
		return fail(err)
	}
	var orgs []institutions.Org
	if err := institutions.ReadROR(data, func(o institutions.Org) error { orgs = append(orgs, o); return nil }); err != nil {
		return fail(err)
	}
	units := map[string][]institutions.Unit{}
	if *wd != "none" {
		if units, err = institutions.Units(ctx, client.Get, *wd); err != nil {
			return fail(err)
		}
	}
	prev, err := institutions.Read(*root)
	if err != nil {
		return fail(err)
	}
	ins, c := institutions.Build(prev, orgs, units)
	if err := institutions.Write(*root, ins); err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "read %d organizations of ROR, %d of them active with the type education; added %d, withdrew %d, %d with a unit of mathematics\n", len(orgs), c.Orgs, c.Added, c.Withdrawn, c.WithUnit)
	for _, t := range institutions.Tiers {
		fmt.Fprintf(stdout, "%s: %d\n", t, c.ByTier[t])
	}
	fmt.Fprintf(stdout, "wrote %s\n", institutions.File)
	return ExitOK
}
