package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource"
	rescluster "github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/refs"
)

const resImportUsage = "m2 res import [-root DIR] [-snapshot HASH] [-cache DIR] [-structure] <source> | m2 res import [-root DIR] [-lookup] [-gap D] refs"

// structurer is an adapter that gives the table of contents of the work of its snapshot (RS4.5), as the adapter of an open textbook does. It also gives the commit of the snapshot.
type structurer interface {
	Structure(r io.Reader) ([]resource.Node, string, error)
}

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
	structure := fs.Bool("structure", false, "also write the table of contents of the snapshot to the first expression of the resource that groups the source records")
	lookup := fs.Bool("lookup", false, "for the source refs: confirm the ids of the references with a live lookup")
	gap := fs.Duration("gap", time.Second, "for the source refs: the shortest time between two lookups")
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
	if source == refs.Source {
		return runResImportRefs(opt.Root, *lookup, *gap, stdout, stderr)
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
	st, ok := a.(structurer)
	if *structure && !ok {
		return fail(fmt.Errorf("the adapter of %q gives no table of contents", source))
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
	if !*structure {
		return ExitOK
	}
	file, nodes, err := writeStructure(opt.Root, source, path, res.Snapshot.Hash, st, rs)
	if err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "%s: %d top nodes of the table of contents in %s\n", source, nodes, filepath.Join(resource.StructureDir, file))
	return ExitOK
}

// writeStructure reads the table of contents of the snapshot at path and writes it to the first expression of the resource that groups the source records rs. The resource gets an expression e1 when it has no expression. It gives the path of the structure file, relative to records/resources/structure/, and the number of top nodes. It fails when no resource, or more than one resource, groups the records, because then m2 res cluster must run first.
func writeStructure(root, source, path, hash string, st structurer, rs []rescluster.Record) (string, int, error) {
	f, err := os.Open(path) // #nosec G304 -- the path is in the snapshot cache.
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = f.Close() }()
	snapshot, err := ingest.Uncompressed(f)
	if err != nil {
		return "", 0, err
	}
	nodes, _, err := st.Structure(snapshot)
	if err != nil {
		return "", 0, fmt.Errorf("%s: %w", source, err)
	}
	native := map[string]bool{}
	for _, r := range rs {
		native[r.NativeID] = true
	}
	all, err := resource.ReadResources(root)
	if err != nil {
		return "", 0, err
	}
	var found []int
	for i := range all {
		if all[i].Live() && slices.ContainsFunc(all[i].Catalogue, func(c resource.CatalogueRecord) bool { return c.Source == source && native[c.NativeID] }) {
			found = append(found, i)
		}
	}
	if len(found) != 1 {
		return "", 0, fmt.Errorf("%d live resources group the source records of %s, and the table of contents needs one. Run m2 res cluster first", len(found), source)
	}
	r := &all[found[0]]
	if len(r.Expressions) == 0 {
		r.Expressions = []resource.Expression{{Key: "e1", Kind: "edition"}}
	}
	e := &r.Expressions[0]
	s := &resource.Structure{Resource: r.ID + "/" + e.Key, Source: source, Snapshot: hash, Nodes: nodes}
	if err := resource.WriteStructure(root, s); err != nil {
		return "", 0, err
	}
	e.Structure = r.ID + "/" + e.Key + ".json"
	if err := resource.WriteResources(root, all); err != nil {
		return "", 0, err
	}
	return e.Structure, len(nodes), nil
}
