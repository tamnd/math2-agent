package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	axImport "github.com/tamnd/math2-agent/importer/arxiv"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/queue"
	"github.com/tamnd/math2-agent/tag"
)

const corpusUsage = "m2 corpus import [-root DIR] [-commit C] [source...]\n       m2 corpus import -source arxiv -corpus DIR [-root DIR] [-commit C]\n       m2 corpus import -plan FILE [-root DIR] [-commit C] [-cache DIR] [-summary FILE]\n       m2 corpus cluster [-root DIR] [-commit C] [-source arxiv [-msc CODE]]\n       m2 corpus gold [-root DIR] [-json] [-write]\n       m2 corpus recall [-root DIR] [-json] [-write]\n       m2 corpus index [-root DIR] [-json] [-write] [-missing]\n       m2 corpus status [-root DIR] [-json]\n       m2 corpus queue [-root DIR] [-n N] [-json] NAME\n       m2 corpus plan [-root DIR] [-n N] [-json] NAME\n       m2 corpus plan understand -tier T1..T4 [-msc CODE] [-root DIR] [-json]\n       m2 corpus report [-root DIR] [-store DIR] [-from YYYY-MM-DD] [-to YYYY-MM-DD] [-json]"

func runCorpus(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: "+corpusUsage)
		return ExitUsage
	}
	switch args[0] {
	case "import":
		return runCorpusImport(args[1:], stdout, stderr)
	case "cluster":
		return runCorpusCluster(args[1:], stdout, stderr)
	case "gold":
		return runCorpusGold(args[1:], stdout, stderr)
	case "recall":
		return runCorpusRecall(args[1:], stdout, stderr)
	case "index":
		return runCorpusIndex(args[1:], stdout, stderr)
	case "status":
		return runCorpusStatus(args[1:], stdout, stderr)
	case "queue", "plan":
		if args[0] == "plan" && len(args) > 1 && args[1] == "understand" {
			return runCorpusPlanUnderstand(args[2:], stdout, stderr)
		}
		return runCorpusQueue(args[0], args[1:], stdout, stderr)
	case "report":
		return runCorpusReport(args[1:], stdout, stderr)
	}
	fmt.Fprintln(stderr, "usage: "+corpusUsage)
	return ExitUsage
}

// writeMentions changes the records of the last import of the source src to mentions, and writes them to records/mentions/<source>/. It returns the number of mentions.
func writeMentions(root string, src *ingest.Source) (int, error) {
	m, err := ingest.ReadManifest(filepath.Join(root, ingest.ManifestPath(src.ID)))
	if err != nil {
		return 0, fmt.Errorf("the import manifest of %s: %w", src.ID, err)
	}
	recs, _, err := ingest.ReadRecords(root, src.ID)
	if err != nil {
		return 0, err
	}
	ms := inventory.FromRecords(src, m.Snapshot, m.Adapter, recs)
	if err := inventory.WriteMentions(root, src.ID, ms); err != nil {
		return 0, err
	}
	return len(ms), nil
}

// runCorpusImport changes the records of the last import of each source to mentions, and groups the mentions into rows.
func runCorpusImport(args []string, stdout, stderr io.Writer) int {
	const name = "m2 corpus import"
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	commit := fs.String("commit", "", "the commit that goes into registers/inventory/. Empty means the HEAD commit of the working tree.")
	plan := fs.String("plan", "", "run the importers of this plan, for example "+ingest.PlanFile+", before the mentions")
	cache := fs.String("cache", "", "with -plan, the directory of the downloaded snapshots. Empty means m2 in the user cache directory.")
	summary := fs.String("summary", "", "with -plan, also write the summary in Markdown to this file, for the body of a pull request")
	source := fs.String("source", "", "a source with its own importer, which reads a checkout and not build/records/. The one such source is arxiv.")
	corpus := fs.String("corpus", "", "with -source, the checkout that the importer reads, for example a checkout of tamnd/arxiv")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	switch *source {
	case "":
	case axImport.Source:
		if *corpus == "" || *plan != "" || fs.NArg() != 0 {
			fmt.Fprintln(stderr, name+": -source arxiv needs -corpus, the checkout of tamnd/arxiv, and takes no -plan and no source names")
			return ExitUsage
		}
	default:
		fmt.Fprintf(stderr, "%s: %s has no importer of its own. Give it with no -source.\n", name, *source)
		return ExitUsage
	}
	if *plan == "" && (*cache != "" || *summary != "") || *plan != "" && fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+corpusUsage)
		return ExitUsage
	}
	if !headCommit(name, *root, commit, stderr) {
		return ExitFailure
	}
	if *plan != "" {
		if err := importPlan(*root, *commit, *plan, *cache, *summary, stdout); err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return ExitFailure
		}
		return ExitOK
	}
	if *source != "" {
		res, err := axImport.Import(*corpus)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return ExitFailure
		}
		if err := inventory.WriteMentions(*root, *source, res.Mentions); err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return ExitFailure
		}
		fmt.Fprintf(stdout, "%s: %d mentions from %d papers, %d attached, %d retired, %d restated prints, %d objects with no tag\n", *source, len(res.Mentions), res.Papers, res.Attached, res.Retired, res.Restated, len(res.Untagged))
		if len(res.Untagged) > 0 {
			fmt.Fprintf(stderr, "%s: %d objects of %s have no tag, so they have no mention. Run ax tags analysis -all on the checkout.\n", name, len(res.Untagged), *corpus)
		}
		if _, err := cluster(*root, *commit, stdout); err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return ExitFailure
		}
		return ExitOK
	}
	sources := fs.Args()
	if len(sources) == 0 {
		var err error
		if sources, err = importedSources(*root); err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return ExitFailure
		}
	}
	if len(sources) == 0 {
		fmt.Fprintf(stderr, "%s: %s has no records. Run m2 ingest first.\n", name, filepath.Join(*root, ingest.RecordsDir))
		return ExitFailure
	}
	cat, err := ingest.ReadCatalogue(*root)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	for _, id := range sources {
		src := cat.Source(id)
		if src == nil {
			fmt.Fprintf(stderr, "%s: manifests/sources.yaml has no source %s\n", name, id)
			return ExitFailure
		}
		n, err := writeMentions(*root, src)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return ExitFailure
		}
		fmt.Fprintf(stdout, "%s: %d mentions\n", id, n)
	}
	if _, err := cluster(*root, *commit, stdout); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	return ExitOK
}

// importPlan runs the importers of the plan at path one after the other, writes their mentions, groups the mentions into rows and prints a summary. Each adapter keeps the rate limit of its source, and each request has the User-Agent of the toolchain. When summary is not empty, the summary also goes to that file.
func importPlan(root, commit, path, cache, summary string, stdout io.Writer) error {
	p, err := ingest.ReadPlan(path)
	if err != nil {
		return err
	}
	adapters := make([]ingest.Adapter, len(p.Imports))
	for i, s := range p.Imports {
		a, ok := ingest.Lookup(s.Source)
		if !ok {
			return fmt.Errorf("%s: the source %s has no adapter. The sources with an adapter: %s", path, s.Source, sourceList())
		}
		adapters[i] = a
	}
	before, err := inventory.ReadMentions(root)
	if err != nil {
		return err
	}
	oldRows, err := inventory.ReadRows(root)
	if err != nil {
		return err
	}
	had := map[string]bool{}
	for _, m := range before {
		had[m.ID] = true
	}
	type line struct {
		source, state, date string
		records, mentions   int
	}
	var lines []line
	for i, s := range p.Imports {
		res, err := ingest.Pull(context.Background(), adapters[i], ingest.Options{
			Root:      root,
			Snapshot:  s.Snapshot,
			URL:       s.URL,
			Cache:     cache,
			UserAgent: userAgent(),
		})
		if err != nil {
			return err
		}
		cat, err := ingest.ReadCatalogue(root)
		if err != nil {
			return err
		}
		n, err := writeMentions(root, cat.Source(s.Source))
		if err != nil {
			return err
		}
		state := "pinned"
		if res.New {
			state = "new"
		}
		lines = append(lines, line{s.Source, state, res.Snapshot.Date, res.Manifest.Records, n})
		fmt.Fprintf(stdout, "%s: %d records from a %s snapshot of %s, %d mentions\n", s.Source, res.Manifest.Records, state, res.Snapshot.Date, n)
	}
	res, err := cluster(root, commit, stdout)
	if err != nil {
		return err
	}
	after, err := inventory.ReadMentions(root)
	if err != nil {
		return err
	}
	added := map[string]int{}
	for _, m := range after {
		if !had[m.ID] {
			added[m.Source]++
		}
	}
	now := time.Now()
	open, wasOpen := inventory.Summarize(res.Rows, after, now).Open, inventory.Summarize(oldRows, before, now).Open
	var b strings.Builder
	b.WriteString("| source | snapshot | records | mentions | new mentions |\n|---|---|---:|---:|---:|\n")
	total := 0
	for _, l := range lines {
		fmt.Fprintf(&b, "| %s | %s, %s | %d | %d | %d |\n", l.source, l.state, l.date, l.records, l.mentions, added[l.source])
		total += added[l.source]
	}
	fmt.Fprintf(&b, "\nThe run added %d mentions and %d rows. The inventory has %d rows, and the open count is %d (it was %d).\n", total, len(res.Added), len(res.Rows), open, wasOpen)
	if _, err := io.WriteString(stdout, "\n"+b.String()); err != nil {
		return err
	}
	if summary == "" {
		return nil
	}
	return os.WriteFile(summary, []byte(b.String()), 0o644) // #nosec G306 -- the summary is the public body of a pull request.
}

// runCorpusCluster groups the mentions of the tree into rows again, with the passes 1 and 2 of corpus K4.3. It reads no records. With -source arxiv, it also runs the name pass of corpus arxiv AX8.2, writes its candidates for the dedup judge to reports/inventory/arxiv-names.jsonl, and writes the report of AX8.7 to reports/inventory-arxiv.md.
func runCorpusCluster(args []string, stdout, stderr io.Writer) int {
	const name = "m2 corpus cluster"
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	commit := fs.String("commit", "", "the commit that goes into registers/inventory/. Empty means the HEAD commit of the working tree.")
	source := fs.String("source", "", "arxiv runs the name pass and writes the report of the arXiv layer")
	msc := fs.String("msc", "", "with -source arxiv, the name pass reads only the block of this MSC code at two digits")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 || *source != "" && *source != axImport.Source || *msc != "" && *source == "" {
		fmt.Fprintln(stderr, "usage: "+corpusUsage)
		return ExitUsage
	}
	if !headCommit(name, *root, commit, stderr) {
		return ExitFailure
	}
	res, err := cluster(*root, *commit, stdout)
	if err == nil && *source == axImport.Source {
		err = arxivReport(*root, res, *msc, stdout)
	}
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	return ExitOK
}

// The files that m2 corpus cluster -source arxiv writes, relative to the root of the tree.
const (
	arxivNames     = "reports/inventory/arxiv-names.jsonl"
	arxivInventory = "reports/inventory-arxiv.md"
)

// arxivReport runs the name pass on the rows of the cluster and writes its candidates and the report of the arXiv layer.
func arxivReport(root string, res *inventory.Result, msc string, stdout io.Writer) error {
	ms, err := inventory.ReadMentions(root)
	if err != nil {
		return err
	}
	cands := axImport.Candidates(res.Rows, ms, msc)
	data, err := axImport.FormatCandidates(cands)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "reports", "inventory"), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(arxivNames)), data, 0o644); err != nil { // #nosec G306 -- the report is public.
		return err
	}
	report := axImport.Report(res.Rows, ms, cands, time.Now())
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(arxivInventory)), []byte(report), 0o644); err != nil { // #nosec G306 -- the report is public.
		return err
	}
	fmt.Fprintf(stdout, "arxiv: joined by hash %d, name candidates %d, wrote %s and %s\n", res.Hashed, len(cands), arxivNames, arxivInventory)
	return nil
}

// headCommit sets an empty commit to the HEAD commit of the working tree at root. It reports whether it has a commit.
func headCommit(name, root string, commit *string, stderr io.Writer) bool {
	if *commit != "" {
		return true
	}
	out, err := git(root, "rev-parse", "HEAD")
	if err != nil {
		fmt.Fprintf(stderr, "%s: the working tree has no HEAD commit. Give the commit with -commit.\n", name)
		return false
	}
	*commit = strings.TrimSpace(out)
	return true
}

// cluster groups the mentions of the tree at root into its rows, writes the rows, and adds the new row identifiers to the register with the commit.
func cluster(root, commit string, stdout io.Writer) (*inventory.Result, error) {
	ms, err := inventory.ReadMentions(root)
	if err != nil {
		return nil, err
	}
	rows, err := inventory.ReadRows(root)
	if err != nil {
		return nil, err
	}
	reg, err := inventory.ReadRegister(root)
	if err != nil {
		return nil, err
	}
	tags, err := tag.ReadUsed(filepath.Join(root, "registers", "tags.reg"), filepath.Join(root, "registers", "retired.reg"))
	if err != nil {
		return nil, err
	}
	res, err := inventory.Cluster(rows, ms, inventory.Options{
		Day:  time.Now(),
		By:   "m2/" + version,
		Used: func(id string) bool { return reg[id] || tags[id] },
	})
	if err != nil {
		return nil, err
	}
	if err := inventory.WriteRows(root, res.Rows); err != nil {
		return nil, err
	}
	if err := inventory.AppendRegister(root, res.Added, commit); err != nil {
		return nil, err
	}
	fmt.Fprintf(stdout, "rows: %d, new %d, with new mentions %d, merged %d, waiting for a source %d, joined by equal hashes %d, joined by equal formal statements %d, attached with no row %d\n", len(res.Rows), len(res.Added), res.Touched, res.Merged, res.Waiting, res.Hashed, res.Equivalent, res.Orphans)
	return res, nil
}

// goldReport is the file that m2 corpus gold -write writes, relative to the root of the tree.
const goldReport = "reports/inventory/clustering.json"

// runCorpusGold prints the precision and the recall of the passes of the clustering on the gold set of the tree.
func runCorpusGold(args []string, stdout, stderr io.Writer) int {
	const name = "m2 corpus gold"
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	asJSON := fs.Bool("json", false, "print JSON")
	write := fs.Bool("write", false, "also write the scores to "+goldReport)
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+corpusUsage)
		return ExitUsage
	}
	gold, err := inventory.ReadGold(*root)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	if len(gold) == 0 {
		fmt.Fprintf(stderr, "%s: the tree has no gold set in %s\n", name, inventory.GoldFile)
		return ExitFailure
	}
	ms, err := inventory.ReadMentions(*root)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	scores := inventory.Measure(ms, gold)
	data, err := json.MarshalIndent(struct {
		Gold   int               `json:"gold"`
		Scores []inventory.Score `json:"scores"`
	}{len(gold), scores}, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	data = append(data, '\n')
	if *write {
		if err := writeReport(filepath.Join(*root, filepath.FromSlash(goldReport)), data); err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return ExitFailure
		}
	}
	if *asJSON {
		if _, err := stdout.Write(data); err != nil {
			return ExitFailure
		}
		return ExitOK
	}
	pct := func(v *float64) string {
		if v == nil {
			return "-"
		}
		return fmt.Sprintf("%.1f%%", 100**v)
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "pass\tlayer\tpairs\tprecision\trecall\tfalse merges\tfalse splits\tmissing")
	for _, s := range scores {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%d\t%d\t%d\n", s.Pass, s.Layer, s.Pairs, pct(s.Precision), pct(s.Recall), s.FalsePos, s.FalseNeg, s.Missing)
	}
	if err := tw.Flush(); err != nil {
		return ExitFailure
	}
	return ExitOK
}

// recallReport is the file that m2 corpus recall -write writes, relative to the root of the tree.
const recallReport = "reports/inventory/recall.json"

// runCorpusRecall prints the estimated recall of the inventory for each MSC code at two digits (corpus K4.7).
func runCorpusRecall(args []string, stdout, stderr io.Writer) int {
	const name = "m2 corpus recall"
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	asJSON := fs.Bool("json", false, "print JSON")
	write := fs.Bool("write", false, "also write the estimates to "+recallReport)
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+corpusUsage)
		return ExitUsage
	}
	ms, err := inventory.ReadMentions(*root)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	rows, err := inventory.ReadRows(*root)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	areas := inventory.Estimate(rows, ms)
	data, err := json.MarshalIndent(struct {
		Floor float64          `json:"floor"`
		Areas []inventory.Area `json:"areas"`
	}{inventory.RecallFloor, areas}, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	data = append(data, '\n')
	if *write {
		if err := writeReport(filepath.Join(*root, filepath.FromSlash(recallReport)), data); err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return ExitFailure
		}
	}
	if *asJSON {
		if _, err := stdout.Write(data); err != nil {
			return ExitFailure
		}
		return ExitOK
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "msc\trows\tsources\trecall\tinterval\tpair\tin both\testimate")
	for _, a := range areas {
		if a.Recall == nil {
			fmt.Fprintf(tw, "%s\t%d\t%d\t-\t-\t-\t-\t-\n", a.MSC, a.Rows, a.Sources)
			continue
		}
		c := a.Captures[slices.IndexFunc(a.Captures, func(c inventory.Capture) bool { return c.Default })]
		fmt.Fprintf(tw, "%s\t%d\t%d\t%.0f%%\t%.0f%% to %.0f%%\t%s, %s\t%d\t%.0f\n", a.MSC, a.Rows, a.Sources, 100**a.Recall, 100**a.Low, 100**a.High, c.A, c.B, c.InBoth, c.Total)
	}
	if err := tw.Flush(); err != nil {
		return ExitFailure
	}
	return ExitOK
}

// indexReport is the file that m2 corpus index -write writes, relative to the root of the tree.
const indexReport = "reports/inventory/index.json"

// runCorpusIndex prints the match rate of the index terms of each book of records/index/ with the rows (corpus K4.7).
func runCorpusIndex(args []string, stdout, stderr io.Writer) int {
	const name = "m2 corpus index"
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	asJSON := fs.Bool("json", false, "print JSON")
	write := fs.Bool("write", false, "also write the match rates to "+indexReport)
	missing := fs.Bool("missing", false, "also print the terms with no row, one on each line after its book")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+corpusUsage)
		return ExitUsage
	}
	idx, err := inventory.ReadIndexes(*root)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	if len(idx) == 0 {
		fmt.Fprintf(stderr, "%s: the tree has no index terms. Add a file %s/<book>.jsonl for each book.\n", name, inventory.IndexDir)
		return ExitFailure
	}
	ms, err := inventory.ReadMentions(*root)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	rows, err := inventory.ReadRows(*root)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	books := inventory.MatchIndexes(rows, ms, idx)
	data, err := json.MarshalIndent(struct {
		Books []inventory.IndexMatch `json:"books"`
	}{books}, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	data = append(data, '\n')
	if *write {
		if err := writeReport(filepath.Join(*root, filepath.FromSlash(indexReport)), data); err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return ExitFailure
		}
	}
	if *asJSON {
		if _, err := stdout.Write(data); err != nil {
			return ExitFailure
		}
		return ExitOK
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "book\tterms\tmatched\trate")
	for _, b := range books {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%.0f%%\n", b.Book, b.Terms, b.Matched, 100*b.Rate)
	}
	if err := tw.Flush(); err != nil {
		return ExitFailure
	}
	if *missing {
		for _, b := range books {
			for _, t := range b.Missing {
				fmt.Fprintf(stdout, "%s\t%s\n", b.Book, t)
			}
		}
	}
	return ExitOK
}

// importedSources returns the sources that have records in build/records/ and an import manifest.
func importedSources(root string) ([]string, error) {
	files, err := os.ReadDir(filepath.Join(root, ingest.RecordsDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range files {
		id, ok := strings.CutSuffix(f.Name(), ".jsonl")
		if !ok {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, ingest.ManifestPath(id))); err == nil {
			out = append(out, id)
		}
	}
	return out, nil
}

// runCorpusStatus prints the counts of the inventory.
func runCorpusStatus(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 corpus status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+corpusUsage)
		return ExitUsage
	}
	ms, err := inventory.ReadMentions(*root)
	if err != nil {
		fmt.Fprintf(stderr, "m2 corpus status: %v\n", err)
		return ExitFailure
	}
	rows, err := inventory.ReadRows(*root)
	if err != nil {
		fmt.Fprintf(stderr, "m2 corpus status: %v\n", err)
		return ExitFailure
	}
	s := inventory.Summarize(rows, ms, time.Now())
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(s); err != nil {
			fmt.Fprintf(stderr, "m2 corpus status: %v\n", err)
			return ExitFailure
		}
		return ExitOK
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "rows\t%d\n", s.Rows)
	fmt.Fprintf(tw, "open\t%d\n", s.Open)
	fmt.Fprintf(tw, "mentions\t%d\n", s.Mentions)
	for _, part := range []struct {
		name string
		m    map[string]int
	}{{"layer", s.Layers}, {"kind", s.Kinds}, {"outcome", s.Outcomes}, {"source", s.Sources}} {
		for _, k := range slices.Sorted(maps.Keys(part.m)) {
			fmt.Fprintf(tw, "%s %s\t%d\n", part.name, k, part.m[k])
		}
	}
	if err := tw.Flush(); err != nil {
		return ExitFailure
	}
	return ExitOK
}

// runCorpusQueue prints a standing queue in priority order, or the plan of its first jobs. It calls no model.
func runCorpusQueue(cmd string, args []string, stdout, stderr io.Writer) int {
	name := "m2 corpus " + cmd
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	n := fs.Int("n", 20, "the number of jobs. -1 means each job of the queue.")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(stderr, "usage: %s [-root DIR] [-n N] [-json] NAME\nThe queues are %s.\n", name, strings.Join(queue.Names(), ", "))
		return ExitUsage
	}
	ms, err := inventory.ReadMentions(*root)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	rows, err := inventory.ReadRows(*root)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	q, err := queue.Compute(fs.Arg(0), queue.Input{Rows: rows, Mentions: ms, Day: time.Now().UTC()})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitUsage
	}
	total := len(q.Items)
	if *n >= 0 && *n < total {
		q.Items = q.Items[:*n]
	}
	var out any = q
	if cmd == "plan" {
		out = queue.MakePlan(q, -1)
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return ExitFailure
		}
		return ExitOK
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	if cmd == "plan" {
		printPlan(tw, out.(queue.Plan), total)
	} else {
		fmt.Fprintf(tw, "%s: %d items, role %s, goal: %s\n", q.Name, total, q.Role, q.Goal)
		if q.Reason != "" {
			fmt.Fprintf(tw, "%s\n", q.Reason)
		}
		for _, it := range q.Items {
			fmt.Fprintf(tw, "%g\t%s\t%s\t%s\n", it.Priority, it.Target, it.Kind, it.Name)
		}
	}
	if err := tw.Flush(); err != nil {
		return ExitFailure
	}
	return ExitOK
}

// printPlan prints a plan of total jobs.
func printPlan(tw io.Writer, p queue.Plan, total int) {
	fmt.Fprintf(tw, "queue\t%s\n", p.Queue)
	fmt.Fprintf(tw, "jobs\t%d of %d\n", p.Jobs, total)
	fmt.Fprintf(tw, "tokens\t%d in, %d out\n", p.TokensIn, p.TokensOut)
	fmt.Fprintf(tw, "model cost\t$%.2f to $%.2f\n", p.CostLow, p.CostHigh)
	if p.Per.ReviewKnown {
		fmt.Fprintf(tw, "review\t%.1f to %.1f hours\n", p.ReviewLow, p.ReviewHigh)
	}
	for _, note := range p.Notes {
		fmt.Fprintf(tw, "note\t%s\n", note)
	}
	fmt.Fprintln(tw, "This is a plan. No model run starts without a budget that a person approved.")
}

// runCorpusPlanUnderstand prints the cost estimate of one tier of the passes on the arXiv mentions (AX7.8). It calls no model.
func runCorpusPlanUnderstand(args []string, stdout, stderr io.Writer) int {
	name := "m2 corpus plan understand"
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	tier := fs.String("tier", "", "the tier: "+strings.Join(axImport.TierNames(), ", "))
	msc := fs.String("msc", "", "count only the mentions of this MSC code at two digits")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 || *tier == "" {
		fmt.Fprintf(stderr, "usage: %s -tier T1..T4 [-msc CODE] [-root DIR] [-json]\n", name)
		return ExitUsage
	}
	ms, err := inventory.ReadMentions(*root)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	rows, err := inventory.ReadRows(*root)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	p, err := axImport.PlanUnderstand(*tier, rows, ms, *msc)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitUsage
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(p); err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return ExitFailure
		}
		return ExitOK
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	printPlan(tw, p, p.Jobs)
	if err := tw.Flush(); err != nil {
		return ExitFailure
	}
	return ExitOK
}
