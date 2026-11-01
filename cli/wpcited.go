package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	wpImport "github.com/tamnd/math2-agent/importer/wikipedia"
	"github.com/tamnd/math2-agent/wpcorpus"
)

// runWPCited writes the Wikipedia titles that a math2 tree cites, one on each line. The output is the -cited file of m2 wp scope (signal S4). With -store, it resolves each title and writes reports/cited.md of the corpus, which gives the mention or the reason of each title (WP14.6).
func runWPCited(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 wp cited", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 tree")
	corpus := fs.String("corpus", ".", "the Wikipedia corpus, for -store")
	storePath := fs.String("store", "", "the SQLite store of the scope pass. With it, the command writes the report of the cited titles")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(stderr, "usage: m2 wp cited [-root DIR] [-store DB -corpus DIR]")
		return ExitUsage
	}
	titles, bad, err := wpImport.Cited(*root)
	if err != nil {
		fmt.Fprintf(stderr, "m2 wp cited: %v\n", err)
		return ExitFailure
	}
	fmt.Fprintf(stderr, "%d titles, %d files did not parse\n", len(titles), bad)
	if *storePath == "" {
		for _, t := range titles {
			fmt.Fprintln(stdout, t)
		}
		return ExitOK
	}
	if err := citedReport(*corpus, *storePath, titles, stdout); err != nil {
		fmt.Fprintf(stderr, "m2 wp cited: %v\n", err)
		return ExitFailure
	}
	return ExitOK
}

// citedReport resolves the titles in the store and writes reports/cited.md
// of the corpus.
func citedReport(corpus, storePath string, titles []string, stdout io.Writer) error {
	cands, err := wpcorpus.ReadScopeFile(corpus, wpcorpus.CandidatesPath)
	if err != nil {
		return err
	}
	store, err := wpcorpus.OpenStore(storePath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	page, err := store.Resolve(titles)
	if err != nil {
		return err
	}
	rs := wpcorpus.ResolveCited(titles, page, cands)
	path := filepath.Join(corpus, wpcorpus.CitedReportPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- the corpus is a public repository
		return err
	}
	if err := writeAtomic(path, func(w io.Writer) error { return wpcorpus.CitedReport(w, rs) }); err != nil {
		return err
	}
	count := map[string]int{}
	for _, r := range rs {
		if r.Mention != "" {
			count["mention"]++
		} else {
			count[r.Reason]++
		}
	}
	fmt.Fprintf(stdout, "%d titles, %d with a mention, %d %s, %d %s, %d %s, report %s\n", len(rs), count["mention"], count[wpcorpus.CitedOutOfScope], wpcorpus.CitedOutOfScope, count[wpcorpus.CitedNoPage], wpcorpus.CitedNoPage, count[wpcorpus.CitedNoSignal], wpcorpus.CitedNoSignal, wpcorpus.CitedReportPath)
	return nil
}
