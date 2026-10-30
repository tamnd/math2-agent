package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/mdrec"
	rescluster "github.com/tamnd/math2-agent/resource/cluster"
)

const resImportUsage = "m2 res import [-root DIR] [-snapshot HASH] [-cache DIR] <source>"

// sourcers gives the ids of the sources whose adapter gives source records, in the order of Adapters.
func sourcers() []string {
	var out []string
	for _, id := range ingest.Adapters() {
		a, _ := ingest.Lookup(id)
		if _, ok := a.(rescluster.Sourcer); ok {
			out = append(out, id)
		}
	}
	return out
}

func runResImport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var opt ingest.Options
	fs.StringVar(&opt.Root, "root", ".", "the math2 working tree")
	fs.StringVar(&opt.Snapshot, "snapshot", "", "use again the snapshot with this hash from manifests/sources.yaml. Empty means a new fetch.")
	fs.StringVar(&opt.Cache, "cache", "", "the directory of the downloaded snapshots. Empty means m2 in the user cache directory.")
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
		fmt.Fprintln(stderr, "usage: "+resImportUsage)
		fmt.Fprintf(stderr, "The sources with source records: %v\n", sourcers())
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 res import: %v\n", err)
		return ExitFailure
	}
	a, ok := ingest.Lookup(source)
	if !ok {
		return fail(fmt.Errorf("the source %q has no adapter", source))
	}
	sr, ok := a.(rescluster.Sourcer)
	if !ok {
		return fail(fmt.Errorf("the adapter of %q gives no source records. The sources with source records: %v", source, sourcers()))
	}
	opt.UserAgent = userAgent()
	res, err := ingest.Pull(context.Background(), a, opt)
	if err != nil {
		return fail(err)
	}
	path, err := ingest.SnapshotFile(opt.Cache, res.Snapshot.Hash)
	if err != nil {
		return fail(err)
	}
	f, err := os.Open(path) // #nosec G304 -- the path is in the snapshot cache.
	if err != nil {
		return fail(err)
	}
	defer func() { _ = f.Close() }()
	snapshot, err := ingest.Uncompressed(f)
	if err != nil {
		return fail(err)
	}
	var rs []rescluster.Record
	var rejects []rescluster.Reject
	err = sr.Records(snapshot, func(r rescluster.Record) error {
		r.Source, r.Snapshot = source, res.Snapshot.Hash
		if why := rescluster.Check(&r); why != "" {
			rejects = append(rejects, rescluster.Reject{NativeID: r.NativeID, Snapshot: r.Snapshot, Reason: why})
			return nil
		}
		rs = append(rs, r)
		return nil
	})
	if err != nil {
		return fail(fmt.Errorf("%s: %w", source, err))
	}
	if err := rescluster.WriteRecords(opt.Root, source, rescluster.SourceFile, rs); err != nil {
		return fail(err)
	}
	if err := rescluster.WriteRejects(opt.Root, source, rejects); err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "%s: %d source records from the snapshot %s in %s, %d rejected\n", source, len(rs), res.Snapshot.Hash, filepath.Join(rescluster.RecordsDir, source, rescluster.SourceFile+mdrec.Ext), len(rejects))
	return ExitOK
}
