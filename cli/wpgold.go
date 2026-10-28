package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tamnd/math2-agent/wpcorpus"
)

const wpGoldUsage = "usage: m2 wp gold draw [-corpus DIR] [-seed N]\n       m2 wp gold measure [-corpus DIR]"

// runWPGold draws gold set S from the candidates, or measures the scope list
// on the marks of gold set S (WP11.2).
func runWPGold(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "draw" && args[0] != "measure") {
		fmt.Fprintln(stderr, wpGoldUsage)
		return ExitUsage
	}
	sub := args[0]
	fs := flag.NewFlagSet("m2 wp gold "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	corpus := fs.String("corpus", ".", "the Wikipedia corpus (a clone of tamnd/wikipedia-math)")
	seed := fs.Uint64("seed", 1, "the seed of the sample (draw)")
	if err := fs.Parse(args[1:]); err != nil {
		return ExitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(stderr, wpGoldUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 wp gold %s: %v\n", sub, err)
		return ExitFailure
	}
	dir := filepath.Join(*corpus, wpcorpus.ManifestDir)
	cs, err := readCandidates(filepath.Join(dir, "candidates.tsv"))
	if err != nil {
		return fail(err)
	}
	goldPath := filepath.Join(dir, "gold", "scope.tsv")

	if sub == "draw" {
		if _, err := os.Stat(goldPath); err == nil {
			return fail(fmt.Errorf("%s exists. A new draw removes the marks, so remove the file first", goldPath))
		}
		rows := wpcorpus.DrawGold(cs, wpcorpus.GoldSizes, *seed)
		if err := os.MkdirAll(filepath.Dir(goldPath), 0o750); err != nil {
			return fail(err)
		}
		if err := writeAtomic(goldPath, func(w io.Writer) error { return wpcorpus.WriteGold(w, rows) }); err != nil {
			return fail(err)
		}
		fmt.Fprintf(stdout, "%d rows in %s\n", len(rows), goldPath)
		return ExitOK
	}

	f, err := os.Open(goldPath) // #nosec G304 -- the path is in the corpus
	if err != nil {
		return fail(err)
	}
	rows, err := wpcorpus.ReadGold(f)
	_ = f.Close()
	if err != nil {
		return fail(fmt.Errorf("%s: %w", goldPath, err))
	}
	m := wpcorpus.Measure(rows, cs)
	if err := os.MkdirAll(filepath.Join(*corpus, "reports"), 0o750); err != nil {
		return fail(err)
	}
	if err := writeAtomic(filepath.Join(*corpus, "reports", "gold-s.md"), func(w io.Writer) error { return wpcorpus.GoldReport(w, m) }); err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "%d marked, precision %.3f, recall %.3f\n", m.Marked, m.Precision, m.Recall)
	return ExitOK
}

func readCandidates(path string) ([]*wpcorpus.Candidate, error) {
	f, err := os.Open(path) // #nosec G304 -- the path is in the corpus
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	cs, err := wpcorpus.ReadCandidates(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cs, nil
}
