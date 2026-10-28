package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tamnd/math2-agent/wpcorpus"
)

const wpGoldUsage = "usage: m2 wp gold draw [s|y|m] [-corpus DIR] [-seed N]\n       m2 wp gold measure [s|y|m] [-corpus DIR]"

// runWPGold draws a gold set, or measures the corpus on the marks of a gold
// set (WP11.2). Set S is the default.
func runWPGold(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "draw" && args[0] != "measure") {
		fmt.Fprintln(stderr, wpGoldUsage)
		return ExitUsage
	}
	sub := args[0]
	set := "s"
	if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
		set, args = args[1], args[1:]
	}
	if set != "s" && set != wpcorpus.GoldSetY && set != wpcorpus.GoldSetM {
		fmt.Fprintln(stderr, wpGoldUsage)
		return ExitUsage
	}
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
	if set != "s" {
		if err := runWPGoldPages(sub, set, *corpus, *seed, stdout); err != nil {
			return fail(err)
		}
		return ExitOK
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

// runWPGoldPages draws or measures gold set Y or M. Each article of the set
// has a file under gold/<set>/v<n>/.
func runWPGoldPages(sub, set, corpus string, seed uint64, stdout io.Writer) error {
	dir := filepath.Join(corpus, wpcorpus.GoldDir(set, wpcorpus.GoldVersion))
	if sub == "draw" {
		if _, err := os.Stat(dir); err == nil {
			return fmt.Errorf("%s exists. A new draw removes the marks, so remove the directory first", dir)
		}
		as, err := wpcorpus.ReadCensusArticles(corpus)
		if err != nil {
			return err
		}
		var gs []*wpcorpus.GoldPage
		if set == wpcorpus.GoldSetY {
			gs = wpcorpus.DrawGoldY(as, seed)
		} else {
			gs = wpcorpus.DrawGoldM(as, seed)
		}
		n := 0
		for _, g := range gs {
			if err := wpcorpus.WriteGoldPage(corpus, set, g); err != nil {
				return err
			}
			n += len(g.Items) + len(g.Formulas)
		}
		what := "draft items"
		if set == wpcorpus.GoldSetM {
			what = "formulas"
		}
		fmt.Fprintf(stdout, "%d articles and %d %s in %s\n", len(gs), n, what, dir)
		return nil
	}
	gs, err := wpcorpus.ReadGoldPages(corpus, set)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(corpus, "reports"), 0o750); err != nil {
		return err
	}
	report := filepath.Join(corpus, "reports", "gold-"+set+".md")
	if set == wpcorpus.GoldSetY {
		m, err := wpcorpus.MeasureGoldY(corpus, gs)
		if err != nil {
			return err
		}
		if err := writeAtomic(report, func(w io.Writer) error { return wpcorpus.GoldYReport(w, m) }); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%d marked, cue recall %.3f, cue precision %.3f, prose recall %.3f\n", m.Marked, m.CueRecall.Value(), m.CuePrecision.Value(), m.ProseRecall.Value())
		return nil
	}
	m, err := wpcorpus.MeasureGoldM(corpus, gs)
	if err != nil {
		return err
	}
	if err := writeAtomic(report, func(w io.Writer) error { return wpcorpus.GoldMReport(w, m) }); err != nil {
		return err
	}
	var parts []string
	for _, f := range wpcorpus.GoldMForms {
		if r := m.ByForm[f]; r != nil {
			parts = append(parts, fmt.Sprintf("%s %.3f", f, r.Value()))
		}
	}
	fmt.Fprintf(stdout, "%d marked, %s\n", m.Marked, strings.Join(parts, ", "))
	return nil
}
