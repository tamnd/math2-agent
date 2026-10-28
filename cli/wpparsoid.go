package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/tamnd/math2-agent/wpcorpus"
	"github.com/tamnd/wikipedia-cli/wiki/parsoid"
)

// parsoidBase is the REST API that m2 wp parsoid uses. The tests change it.
var parsoidBase = parsoid.DefaultBase

func runWPParsoid(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 wp parsoid", flag.ContinueOnError)
	fs.SetOutput(stderr)
	corpus := fs.String("corpus", ".", "the Wikipedia corpus (a clone of tamnd/wikipedia-math)")
	page := fs.Int("page", 0, "fetch only the article with this page id")
	limit := fs.Int("limit", 0, "fetch only the first N articles in the order of WP2.9")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 wp parsoid: %v\n", err)
		return ExitFailure
	}
	started := time.Now().UTC()
	ms, err := wpcorpus.ReadMetadataFiles(*corpus)
	if err != nil {
		return fail(err)
	}
	if len(ms) == 0 {
		return fail(errors.New("the corpus has no metadata files. Run m2 wp fetch first"))
	}
	// The order of WP2.9 comes from the scope list.
	byID := map[int64]*wpcorpus.Metadata{}
	for _, m := range ms {
		byID[int64(m.Page)] = m
	}
	var cs []*wpcorpus.Candidate
	if scope, err := wpcorpus.ReadScopeFile(*corpus, wpcorpus.ScopePath); err == nil {
		for _, c := range scope {
			if byID[c.PageID] != nil {
				cs = append(cs, c)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fail(err)
	}
	if len(cs) < len(ms) {
		seen := map[int64]bool{}
		for _, c := range cs {
			seen[c.PageID] = true
		}
		for _, m := range ms {
			if !seen[int64(m.Page)] {
				cs = append(cs, &wpcorpus.Candidate{PageID: int64(m.Page), Title: m.Title, Importance: m.Importance})
			}
		}
	}
	pilot, err := readPilot(*corpus)
	if err != nil {
		return fail(err)
	}
	wpcorpus.FetchOrder(cs, pilot)

	rec := &wpcorpus.RunRecord{Command: "m2 wp parsoid", Args: args, M2: version, Wiki: wpcorpus.WikiVersion(), Started: started.Format(time.RFC3339)}
	f := &parsoid.Fetcher{Base: parsoidBase, UserAgent: userAgent()}
	ctx := context.Background()
	for _, c := range cs {
		m := byID[c.PageID]
		if *page != 0 && m.Page != *page {
			continue
		}
		path := filepath.Join(*corpus, wpcorpus.ParsoidPath(m.Page, m.Revision))
		// A revision does not change, so its HTML is fetched one time.
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if *limit > 0 && rec.Articles >= *limit {
			break
		}
		rec.Articles++
		body, err := f.Fetch(ctx, m.Title, m.Revision)
		if err == nil {
			err = writeWork(path, body)
		}
		if err != nil {
			fmt.Fprintf(stderr, "m2 wp parsoid: %d %s: %v\n", m.Page, m.Title, err)
			rec.Failed++
			rec.Failures = append(rec.Failures, fmt.Sprintf("%d r%d: %v", m.Page, m.Revision, err))
			continue
		}
		rec.Pages++
		fmt.Fprintf(stdout, "%d r%d %s: %d bytes\n", m.Page, m.Revision, m.Title, len(body))
	}
	rec.Requests, rec.PerMinute = f.Rate()
	rec.Finished = time.Now().UTC().Format(time.RFC3339)
	rel, err := wpcorpus.WriteRunRecord(*corpus, rec)
	if err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "%d articles, %d fetched, %d failed, %d requests at %.1f a minute, run record %s\n", rec.Articles, rec.Pages, rec.Failed, rec.Requests, rec.PerMinute, rel)
	if rec.Failed > 0 {
		return ExitFailure
	}
	return ExitOK
}

func writeWork(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
