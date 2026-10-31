package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/arxivtext"
	rescluster "github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/crlang"
	"github.com/tamnd/math2-agent/resource/refs"
	"github.com/tamnd/math2-agent/resource/s2"
	"github.com/tamnd/math2-agent/resource/sourceworks"
	"github.com/tamnd/math2-agent/resource/swh"
	"github.com/tamnd/math2-agent/resource/zbsearch"
)

const resImportUsage = "m2 res import [-root DIR] [-snapshot HASH] [-url URL] [-cache DIR] [-structure] [-held] <source> | m2 res import [-root DIR] [-lookup] [-gap D] refs | m2 res import [-root DIR] [-gap D] s2 | m2 res import [-root DIR] [-gap D] zbmath_search | m2 res import [-root DIR] [-gap D] crossref_language | m2 res import [-root DIR] -url CHECKOUT arxiv_text | m2 res import [-root DIR] [-gap D] [-n N] swh | m2 res import [-root DIR] source_works"

// structurer is an adapter that gives the table of contents of the work of its snapshot (RS4.5), as the adapter of an open textbook does. It also gives the commit of the snapshot.
type structurer interface {
	Structure(r io.Reader) ([]resource.Node, string, error)
}

// structures is an adapter that gives the structures of many expressions of its snapshot, as a course platform gives the sessions of each offering (RS5.4). The id names the expression by one of its ids, as offering:<offering id>. A material of a node and the book of a reading can name a resource by an id too, as ocw:<slug> or isbn:<isbn>.
type structures interface {
	Structures(r io.Reader, emit func(id string, nodes []resource.Node) error) error
}

// setStructurer is an adapter whose snapshot holds many sets, as the years of a competition. It gives the structure of each set by the native id of the set, as the problems of a year by number (RS7.2).
type setStructurer interface {
	SetStructures(r io.Reader) (map[string][]resource.Node, error)
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
	fs.StringVar(&opt.URL, "url", "", "fetch the new snapshot from this URL or local path, and not from the bulk channel of the adapter")
	fs.StringVar(&opt.Cache, "cache", "", "the directory of the downloaded snapshots. Empty means m2 in the user cache directory.")
	structure := fs.Bool("structure", false, "also write the table of contents of the snapshot to the first expression of the resource that groups the source records, or the sessions of each offering of a course platform to its expression")
	held := fs.Bool("held", false, "keep only the source records of the papers that the corpus holds: the records whose native id is the paper of a mention in records/mentions/<source>/")
	lookup := fs.Bool("lookup", false, "for the source refs: confirm the ids of the references with a live lookup")
	gap := fs.Duration("gap", time.Second, "for the sources refs, s2, swh, zbmath_search and crossref_language: the shortest time between two lookups")
	limit := fs.Int("n", 0, "for the source swh: ask for at most this number of new origins. 0 means no limit.")
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
	if source == s2.Source {
		return runResImportS2(opt.Root, *gap, stdout, stderr)
	}
	if source == zbsearch.Source {
		return runResImportZbsearch(opt.Root, *gap, stdout, stderr)
	}
	if source == crlang.Source {
		return runResImportCrlang(opt.Root, *gap, stdout, stderr)
	}
	if source == arxivtext.Source {
		return runResImportArxivText(opt.Root, opt.URL, stdout, stderr)
	}
	if source == swh.Source {
		return runResImportSWH(opt.Root, *gap, *limit, stdout, stderr)
	}
	if source == sourceworks.Source {
		return runResImportSourceWorks(opt.Root, stdout, stderr)
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
	ss, many := a.(structures)
	sst, sets := a.(setStructurer)
	if *structure && !ok && !many && !sets {
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
	var keep map[string]bool
	if *held {
		if keep, err = heldPapers(opt.Root, source); err != nil {
			return fail(err)
		}
	}
	var rs []rescluster.Record
	var rejects []rescluster.Reject
	err = sr.Records(snapshot, func(r rescluster.Record) error {
		if keep != nil && !keep[r.NativeID] {
			return nil
		}
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
	if many {
		n, missing, err := writeStructures(opt.Root, source, path, res.Snapshot.Hash, ss)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(stdout, "%s: %d structures in %s, %d with no resource\n", source, n, resource.StructureDir, missing)
		return ExitOK
	}
	if sets {
		n, skipped, err := writeSetStructures(opt.Root, source, path, res.Snapshot.Hash, sst)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(stdout, "%s: the structures of %d sets in %s, %d sets skipped with no single live resource\n", source, n, resource.StructureDir, skipped)
		return ExitOK
	}
	file, nodes, err := writeStructure(opt.Root, source, path, res.Snapshot.Hash, st, rs)
	if err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "%s: %d top nodes of the table of contents in %s\n", source, nodes, filepath.Join(resource.StructureDir, file))
	return ExitOK
}

// heldPapers gives the papers of the mentions of the source in the tree at root: the native id of each mention before the #.
func heldPapers(root, source string) (map[string]bool, error) {
	ms, err := inventory.ReadMentions(root)
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	for _, m := range ms {
		if m.Source == source {
			paper, _, _ := strings.Cut(m.NativeID, "#")
			keep[paper] = true
		}
	}
	if len(keep) == 0 {
		return nil, fmt.Errorf("the tree has no mentions of %s, so -held keeps no records", source)
	}
	return keep, nil
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

// writeStructures reads the structures of the snapshot at path and writes each to the expression that has its id. A material of a node that names a resource by an id gets the resource id. A reading of a book that the tree has gets the expression of the book, and the expression of the course gets a uses_text edge to it. A reading of a book that the tree does not have keeps its text for the reference resolver. It gives the number of structures that it writes and the number with no expression. It fails when no structure has an expression, because then m2 res cluster must run first.
func writeStructures(root, source, path, hash string, ss structures) (int, int, error) {
	f, err := os.Open(path) // #nosec G304 -- the path is in the snapshot cache.
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = f.Close() }()
	snapshot, err := ingest.Uncompressed(f)
	if err != nil {
		return 0, 0, err
	}
	all, err := resource.ReadResources(root)
	if err != nil {
		return 0, 0, err
	}
	type place struct{ r, e int }
	exprs := map[string]place{}   // an id of an expression, or of one of its manifestations, as system:value
	owners := map[string]string{} // an id of any level, as system:value, to the resource id
	for i := range all {
		r := &all[i]
		if !r.Live() {
			continue
		}
		put := func(ids resource.IDs, e int) {
			for sys, vs := range ids {
				for _, v := range vs {
					k := sys + ":" + v
					if _, ok := owners[k]; !ok {
						owners[k] = r.ID
					}
					if _, ok := exprs[k]; !ok && e >= 0 {
						exprs[k] = place{i, e}
					}
				}
			}
		}
		put(r.IDs, -1)
		for j := range r.Expressions {
			put(r.Expressions[j].IDs, j)
			for _, m := range r.Expressions[j].Manifestations {
				put(m.IDs, j)
			}
		}
	}
	written, missing := 0, 0
	err = ss.Structures(snapshot, func(id string, nodes []resource.Node) error {
		p, ok := exprs[id]
		if !ok {
			missing++
			return nil
		}
		r := &all[p.r]
		e := &r.Expressions[p.e]
		var books []string
		var resolve func(ns []resource.Node)
		resolve = func(ns []resource.Node) {
			for i := range ns {
				n := &ns[i]
				for k, m := range n.Materials {
					if rid, ok := owners[m]; ok {
						n.Materials[k] = rid
					}
				}
				for k := range n.Readings {
					rd := &n.Readings[k]
					if rd.Resource == "" || !strings.Contains(rd.Resource, ":") {
						continue
					}
					if b, ok := exprs[rd.Resource]; ok {
						rd.Resource, rd.Text = all[b.r].ID+"/"+all[b.r].Expressions[b.e].Key, ""
						if !slices.Contains(books, rd.Resource) {
							books = append(books, rd.Resource)
						}
					} else {
						rd.Resource = ""
					}
				}
				resolve(n.Children)
			}
		}
		resolve(nodes)
		s := &resource.Structure{Resource: r.ID + "/" + e.Key, Source: source, Snapshot: hash, Nodes: nodes}
		if err := resource.WriteStructure(root, s); err != nil {
			return err
		}
		e.Structure = r.ID + "/" + e.Key + ".json"
		for _, b := range books {
			rel := resource.Relation{Kind: "uses_text", From: e.Key, To: b, Source: source}
			if !slices.ContainsFunc(r.Relations, func(x resource.Relation) bool { return x.Kind == rel.Kind && x.From == rel.From && x.To == rel.To }) {
				r.Relations = append(r.Relations, rel)
			}
		}
		written++
		return nil
	})
	if err != nil {
		return 0, 0, fmt.Errorf("%s: %w", source, err)
	}
	if written == 0 && missing > 0 {
		return 0, missing, fmt.Errorf("no live resource has the expressions of the %d structures of %s. Run m2 res cluster first", missing, source)
	}
	if err := resource.WriteResources(root, all); err != nil {
		return 0, 0, err
	}
	return written, missing, nil
}

// writeSetStructures reads the structure of each set of the snapshot at path. It writes each one to the first expression of the live resource that groups the source record of the set, and the resource gets an expression e1 when it has no expression. A set that no live resource groups, or that more than one live resource groups, is skipped. It gives the number of structures written and the number of sets skipped. It fails when it writes no structure, because then m2 res cluster must run first.
func writeSetStructures(root, source, path, hash string, st setStructurer) (int, int, error) {
	f, err := os.Open(path) // #nosec G304 -- the path is in the snapshot cache.
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = f.Close() }()
	snapshot, err := ingest.Uncompressed(f)
	if err != nil {
		return 0, 0, err
	}
	sets, err := st.SetStructures(snapshot)
	if err != nil {
		return 0, 0, fmt.Errorf("%s: %w", source, err)
	}
	all, err := resource.ReadResources(root)
	if err != nil {
		return 0, 0, err
	}
	by := map[string][]int{}
	for i := range all {
		if !all[i].Live() {
			continue
		}
		for _, c := range all[i].Catalogue {
			if _, ok := sets[c.NativeID]; ok && c.Source == source && !slices.Contains(by[c.NativeID], i) {
				by[c.NativeID] = append(by[c.NativeID], i)
			}
		}
	}
	ids := slices.Sorted(maps.Keys(sets))
	written, skipped := 0, 0
	for _, id := range ids {
		if len(by[id]) != 1 {
			skipped++
			continue
		}
		r := &all[by[id][0]]
		if len(r.Expressions) == 0 {
			r.Expressions = []resource.Expression{{Key: "e1", Kind: "edition"}}
		}
		e := &r.Expressions[0]
		if err := resource.WriteStructure(root, &resource.Structure{Resource: r.ID + "/" + e.Key, Source: source, Snapshot: hash, Nodes: sets[id]}); err != nil {
			return written, skipped, err
		}
		e.Structure = r.ID + "/" + e.Key + ".json"
		written++
	}
	if written == 0 {
		return 0, skipped, fmt.Errorf("no live resource groups a set of %s, so no structure is written. Run m2 res cluster first", source)
	}
	return written, skipped, resource.WriteResources(root, all)
}
