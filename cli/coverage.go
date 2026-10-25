package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/coverage"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/zbmath"
	"github.com/tamnd/math2-agent/msc"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/release"
)

const coverageUsage = "m2 coverage [-root DIR] [-out DIR] [-name NAME] [-count] [-delay D]"

// countURL gives the URL of the count of one class. The tests change it.
var countURL = zbmath.CountURL

func runCoverage(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("m2 coverage", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree")
	out := flags.String("out", "", "the output directory of m2 build. Empty means build/ in the tree.")
	name := flags.String("name", "next", "the name of the report: a release, or next")
	count := flags.Bool("count", false, "ask zbMATH Open for the number of documents in each class, and write "+coverage.ExpectedFile)
	delay := flags.Duration("delay", 2*time.Second, "the time between two requests of -count")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 0 || (*name != "next" && !release.IsName(*name)) || strings.ContainsAny(*name, `/\`) {
		fmt.Fprintln(stderr, "usage: "+coverageUsage)
		return ExitUsage
	}
	if *out == "" {
		*out = filepath.Join(*root, "build")
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 coverage: %v\n", err)
		return ExitFailure
	}
	m, err := msc.Read(*root)
	if err != nil {
		return fail(err)
	}
	if *count {
		e, err := countClasses(context.Background(), m, *delay)
		if err != nil {
			return fail(err)
		}
		if err := e.Write(*root); err != nil {
			return fail(err)
		}
		fmt.Fprintf(stdout, "m2 coverage wrote the count of %d classes to %s\n", len(e.Counts), coverage.ExpectedFile)
	}
	in := coverage.Input{Map: m, Records: map[string][]ingest.Record{}}
	if in.Expected, err = coverage.ReadExpected(*root); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fail(err)
	}
	if in.Nodes, err = node.Load(*root); err != nil {
		return fail(err)
	}
	if in.Status, err = build.ReadStatus(*out); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fail(fmt.Errorf("%s does not exist. Run m2 build first", filepath.Join(*out, build.StatusFile)))
		}
		return fail(err)
	}
	files, err := filepath.Glob(filepath.Join(*root, "build", "records", "*.jsonl"))
	if err != nil {
		return fail(err)
	}
	for _, f := range files {
		id := strings.TrimSuffix(filepath.Base(f), ".jsonl")
		if in.Records[id], _, err = ingest.ReadRecords(*root, id); err != nil {
			return fail(err)
		}
	}
	r := &coverage.Report{Name: *name, Rows: coverage.Compute(in), Expected: in.Expected}
	paths, err := r.Write(*root)
	if err != nil {
		return fail(err)
	}
	var recorded, expected int64
	for _, row := range r.Rows {
		if row.Level == 1 {
			recorded += min(row.Recorded, row.Expected)
			expected += row.Expected
		}
	}
	summary := "no count of the documents"
	if expected > 0 {
		summary = fmt.Sprintf("C0 %.1f%% of %d documents", float64(recorded)/float64(expected)*100, expected)
	}
	fmt.Fprintf(stdout, "m2 coverage wrote the report %s: %d codes, %s, %d nodes, %s\n", *name, len(r.Rows), records(in.Records), len(in.Nodes), summary)
	for _, p := range paths[:2] {
		fmt.Fprintln(stdout, "  "+p)
	}
	fmt.Fprintf(stdout, "  %s/%s/ (%d files)\n", coverage.Dir, *name, len(paths)-2)
	return ExitOK
}

// records describes the records of the sources, for example "2 records from arxiv, zbmath".
func records(by map[string][]ingest.Record) string {
	if len(by) == 0 {
		return "no records"
	}
	n := 0
	for _, recs := range by {
		n += len(recs)
	}
	return fmt.Sprintf("%d records from %s", n, strings.Join(slices.Sorted(maps.Keys(by)), ", "))
}

// countClasses asks the REST interface of zbMATH Open for the number of documents of each top-level code of the map, one request at a time, with the delay between two requests.
func countClasses(ctx context.Context, m *msc.Map, delay time.Duration) (*coverage.Expected, error) {
	e := &coverage.Expected{Source: "zbmath", URL: countURL("11"), Date: time.Now().UTC().Format(time.DateOnly), Counts: map[string]int{}}
	client := &http.Client{Timeout: time.Minute}
	for i, c := range m.Children("") {
		if i > 0 {
			time.Sleep(delay)
		}
		n, err := countOne(ctx, client, countURL(c[:2]))
		if err != nil {
			return nil, fmt.Errorf("the count of %s: %w", c, err)
		}
		e.Counts[c] = n
	}
	return e, nil
}

func countOne(ctx context.Context, client *http.Client, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "m2/"+version+" (+https://github.com/tamnd/math2-agent)")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req) // #nosec G107 G704 -- the URL is the REST interface of zbMATH Open.
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("%s answers %s", url, resp.Status)
	}
	return zbmath.Count(resp.Body)
}
