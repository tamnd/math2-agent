package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/wpcorpus"
)

const wpRefreshUsage = "m2 wp refresh [-corpus DIR] [-api URL] [-page ID] [-limit N] [-now DATE] [-dry-run]"

// runWPRefresh is the weekly refresh of WP2.7. It asks the Action API for the newest revision of each article in scope that has a metadata file, 50 articles in a request. For each article with a new revision, it reads the history and applies the rule of WP2.6. When the revision of record changes, it fetches the new revision and writes the content, analysis, Markdown and metadata files again, as m2 wp fetch does. An article that is gone gets the stop reason deleted, and an article that is a redirect now gets the stop reason merged with the page id of its target. The command does not need the dump or the store: the Wikidata item, the short description and the redirects come from the metadata files.
func runWPRefresh(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 wp refresh", flag.ContinueOnError)
	fs.SetOutput(stderr)
	corpus := fs.String("corpus", ".", "the Wikipedia corpus (a clone of tamnd/wikipedia-math)")
	api := fs.String("api", wpcorpus.DefaultAPI, "the Action API URL")
	page := fs.Int64("page", 0, "refresh only the article with this page id")
	limit := fs.Int("limit", 0, "refresh only the first N articles in the order of WP2.9")
	nowFlag := fs.String("now", "", "the date of the rule of WP2.6, YYYY-MM-DD. The default is the time of the run.")
	dry := fs.Bool("dry-run", false, "list the articles with a new revision, and write nothing")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		fmt.Fprintln(stderr, "usage: "+wpRefreshUsage)
		return ExitUsage
	}
	started := time.Now().UTC()
	now := started
	if *nowFlag != "" {
		t, err := time.Parse(time.DateOnly, *nowFlag)
		if err != nil {
			fmt.Fprintf(stderr, "m2 wp refresh: the date %q is not YYYY-MM-DD\n", *nowFlag)
			return ExitUsage
		}
		now = t
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 wp refresh: %v\n", err)
		return ExitFailure
	}

	m, err := wpcorpus.Load(*corpus)
	if err != nil {
		return fail(err)
	}
	all, err := wpcorpus.ReadScopeFile(*corpus, wpcorpus.ScopePath)
	if err != nil {
		return fail(err)
	}
	stops, err := wpcorpus.ReadStops(*corpus)
	if err != nil {
		return fail(err)
	}
	mds, err := wpcorpus.ReadMetadataFiles(*corpus)
	if err != nil {
		return fail(err)
	}
	prior := make(map[int]*wpcorpus.Metadata, len(mds))
	facts := &wpcorpus.StoreFacts{Wikidata: map[int64]string{}, Short: map[int64]string{}, Redirects: map[string][]string{}, Target: map[string]string{}}
	for _, md := range mds {
		prior[md.Page] = md
		facts.Wikidata[int64(md.Page)] = md.Wikidata
		facts.Short[int64(md.Page)] = md.Short
		title := strings.ReplaceAll(md.Title, " ", "_")
		facts.Redirects[title] = md.Redirects
		for _, rd := range md.Redirects {
			facts.Target[strings.ReplaceAll(rd, " ", "_")] = title
		}
	}
	sticky := map[int64]bool{}
	for _, s := range stops {
		if stickyStops[s.Reason] {
			sticky[s.PageID] = true
		}
	}
	var scope, run []*wpcorpus.Candidate
	for _, c := range all {
		if !c.InScope {
			continue
		}
		scope = append(scope, c)
		if prior[int(c.PageID)] != nil && !sticky[c.PageID] && (*page == 0 || c.PageID == *page) {
			run = append(run, c)
		}
	}
	pilot, err := readPilot(*corpus)
	if err != nil {
		return fail(err)
	}
	wpcorpus.FetchOrder(run, pilot)
	if *limit > 0 && len(run) > *limit {
		run = run[:*limit]
	}

	r := &fetchRun{corpus: *corpus, m: m, resolve: wpcorpus.LinkResolver(scope, facts.Target), facts: facts, byID: map[int64]*wpcorpus.Candidate{}, prior: prior, stdout: stdout, stderr: stderr}
	for _, c := range run {
		r.byID[c.PageID] = c
	}
	f := &wpcorpus.Fetcher{API: *api, UserAgent: userAgent()}
	ctx := context.Background()
	requests := 0
	wait := func() {
		if requests > 0 {
			time.Sleep(wpDelay)
		}
		requests++
	}
	failed := func(id int, err error) {
		fmt.Fprintf(stderr, "m2 wp refresh: %d: %v\n", id, err)
		r.failed++
		r.failures = append(r.failures, fmt.Sprintf("%d: %v", id, err))
	}
	var newStops []wpcorpus.Stop
	stop := func(id int64, reason string, into int64, note string) {
		newStops = append(newStops, wpcorpus.Stop{PageID: id, Reason: reason, Into: into, Note: note})
		fmt.Fprintf(stdout, "%d stopped: %s %s\n", id, reason, note)
	}

	// The newest revision of each article, 50 articles in a request.
	var changed []int
	for i := 0; i < len(run); i += 50 {
		var ids []int
		for _, c := range run[i:min(i+50, len(run))] {
			ids = append(ids, int(c.PageID))
		}
		wait()
		got, err := f.LatestOf(ctx, ids)
		if err != nil {
			failed(ids[0], fmt.Errorf("the request for %d pages: %w", len(ids), err))
			continue
		}
		for _, id := range ids {
			l, ok := got[id]
			switch {
			case !ok:
				failed(id, fmt.Errorf("the API did not give the page"))
			case l.Missing:
				if !*dry {
					stop(int64(id), "deleted", 0, "the page does not exist on "+now.Format(time.DateOnly))
				}
			case l.Redirect:
				if *dry {
					continue
				}
				wait()
				into, title, err := f.RedirectTarget(ctx, id)
				if err != nil {
					stop(int64(id), "failed", 0, err.Error())
					continue
				}
				stop(int64(id), "merged", int64(into), "the page redirects to "+title)
			case l.ID != prior[id].Revision:
				changed = append(changed, id)
			}
		}
	}
	if *dry {
		for _, id := range changed {
			fmt.Fprintf(stdout, "%d r%d %s: a new revision\n", id, prior[id].Revision, prior[id].Title)
		}
		fmt.Fprintf(stdout, "%d articles, %d with a new revision, %d requests\n", len(run), len(changed), requests)
		return ExitOK
	}

	// The rule of WP2.6 for each article with a new revision.
	refreshed, kept := 0, 0
	for _, id := range changed {
		old := prior[id]
		wait()
		hist, err := f.History(ctx, id, historyLimit)
		if err != nil {
			failed(id, err)
			continue
		}
		rev, unstable := wpcorpus.RecordRevision(hist, now)
		var settled *wpcorpus.Revision
		if s, ok := wpcorpus.NewestSettled(hist, now); ok {
			settled = &s
		}
		if unstable || rev == old.Revision {
			// The revision of record stays. Only the fields of rule Y13 and the flag unstable can change.
			md := *old
			md.Settled, md.SettledTimestamp, md.Unstable = 0, "", unstable
			if settled != nil && settled.ID != old.Revision {
				md.Settled, md.SettledTimestamp = settled.ID, settled.Timestamp
			}
			if md.Settled != old.Settled || md.Unstable != old.Unstable {
				if err := wpcorpus.WriteMetadata(*corpus, &md); err != nil {
					failed(id, err)
					continue
				}
			}
			kept++
			fmt.Fprintf(stdout, "%d r%d %s: kept\n", id, old.Revision, old.Title)
			continue
		}
		wait()
		got, err := f.FetchRevision(ctx, rev)
		if err != nil {
			failed(id, err)
			continue
		}
		sha := ""
		for _, h := range hist {
			if h.ID == rev {
				sha = h.SHA1
			}
		}
		before := r.failed
		r.write(got, sha, false, settled)
		if r.failed == before {
			refreshed++
		}
	}

	if len(newStops) > 0 {
		again := map[int64]bool{}
		for _, s := range newStops {
			again[s.PageID] = true
		}
		out := slices.DeleteFunc(slices.Clone(stops), func(s wpcorpus.Stop) bool { return again[s.PageID] })
		if err := wpcorpus.WriteStops(*corpus, append(out, newStops...)); err != nil {
			return fail(err)
		}
	}
	rec := &wpcorpus.RunRecord{Command: "m2 wp refresh", Args: args, M2: version, Wiki: wpcorpus.WikiVersion(),
		Started: started.Format(time.RFC3339), Finished: time.Now().UTC().Format(time.RFC3339),
		Articles: len(run), Pages: r.pages, Stopped: len(newStops), Failed: r.failed, Requests: requests, Failures: r.failures}
	rel, err := wpcorpus.WriteRunRecord(*corpus, rec)
	if err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "%d articles, %d with a new revision, %d refreshed, %d kept, %d stopped, %d failed, %d requests, run record %s\n",
		len(run), len(changed), refreshed, kept, len(newStops), r.failed, requests, rel)
	if r.failed > 0 {
		return ExitFailure
	}
	return ExitOK
}
