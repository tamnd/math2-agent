package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/inventory"
)

const ingestUsage = "m2 ingest [-root DIR] [-snapshot HASH] [-url URL] [-date YYYY-MM-DD] [-cache DIR] [-mentions] <source>"

func runIngest(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 ingest", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var opt ingest.Options
	fs.StringVar(&opt.Root, "root", ".", "the math2 working tree")
	fs.StringVar(&opt.Snapshot, "snapshot", "", "use again the snapshot with this hash from manifests/sources.yaml. Empty means a new download.")
	fs.StringVar(&opt.URL, "url", "", "download the new snapshot from this URL, and not from the bulk channel of the adapter")
	fs.StringVar(&opt.Date, "date", "", "the date of a new snapshot. Empty means today in UTC.")
	fs.StringVar(&opt.Cache, "cache", "", "the directory of the downloaded snapshots. Empty means m2 in the user cache directory.")
	mentions := fs.Bool("mentions", false, "also write the mentions of the source to records/mentions/<source>/")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	// The source can come before the flags, as in m2 ingest oeis -snapshot S.
	var source string
	if fs.NArg() > 0 {
		source = fs.Arg(0)
		if err := fs.Parse(fs.Args()[1:]); err != nil {
			return ExitUsage
		}
	}
	if source == "" || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+ingestUsage)
		fmt.Fprintln(stderr, "The sources with an adapter: "+sourceList())
		return ExitUsage
	}
	a, ok := ingest.Lookup(source)
	if !ok {
		fmt.Fprintf(stderr, "m2 ingest: the source %q has no adapter. The sources with an adapter: %s\n", source, sourceList())
		return ExitFailure
	}
	opt.UserAgent = userAgent()
	res, err := ingest.Pull(context.Background(), a, opt)
	if err != nil {
		fmt.Fprintf(stderr, "m2 ingest: %v\n", err)
		return ExitFailure
	}
	m, sn := res.Manifest, res.Snapshot
	state := "a pinned snapshot"
	if res.New {
		state = "a new snapshot"
	}
	var classes []string
	for _, c := range slices.Sorted(maps.Keys(m.Classes)) {
		classes = append(classes, fmt.Sprintf("%s %d", c, m.Classes[c]))
	}
	fmt.Fprintf(stdout, "%s: %d records from %s of %s\n", m.Source, m.Records, state, sn.Date)
	fmt.Fprintf(stdout, "  snapshot %s\n  url      %s\n  records  %s\n  classes  %s\n", sn.Hash, sn.URL, m.Hash, strings.Join(classes, ", "))
	if !*mentions {
		return ExitOK
	}
	cat, err := ingest.ReadCatalogue(opt.Root)
	if err != nil {
		fmt.Fprintf(stderr, "m2 ingest: %v\n", err)
		return ExitFailure
	}
	n, err := writeMentions(opt.Root, cat.Source(m.Source))
	if err != nil {
		fmt.Fprintf(stderr, "m2 ingest: %v\n", err)
		return ExitFailure
	}
	fmt.Fprintf(stdout, "  mentions %d in %s/%s/\n", n, inventory.MentionsDir, m.Source)
	return ExitOK
}

// userAgent is the User-Agent header of each request of an importer.
func userAgent() string { return "m2/" + version + " (+https://github.com/tamnd/math2-agent)" }

// sourceList returns the sources that have an adapter, or "none".
func sourceList() string {
	if ids := ingest.Adapters(); len(ids) > 0 {
		return strings.Join(ids, ", ")
	}
	return "none"
}
