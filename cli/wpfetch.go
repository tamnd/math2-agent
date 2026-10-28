package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/wpcorpus"
	"github.com/tamnd/wikipedia-cli/wiki/dump"
)

// stickyStops are the stop reasons that a person or the scope pass gives.
// The fetch keeps them and does not read the article again.
var stickyStops = map[string]bool{"not-math": true, "protected-empty": true}

// historyLimit is the number of revisions that the check of WP2.6 reads.
const historyLimit = 50

// fetchRun holds the state of one run of m2 wp fetch.
type fetchRun struct {
	corpus  string
	m       *wpcorpus.Manifests
	resolve wpcorpus.Resolver
	facts   *wpcorpus.StoreFacts
	byID    map[int64]*wpcorpus.Candidate
	dump    string
	stdout  io.Writer
	stderr  io.Writer

	pages, objects, failed int
	failures               []string
}

func runWPFetch(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 wp fetch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	corpus := fs.String("corpus", ".", "the Wikipedia corpus (a clone of tamnd/wikipedia-math)")
	dumpPath := fs.String("dump", "", "the multistream XML dump, enwiki-DATE-pages-articles-multistream.xml.bz2")
	index := fs.String("index", "", "the index of the dump. The default is the dump path with -index.txt.bz2.")
	storePath := fs.String("store", "", "the SQLite store of the scope pass, with the tables page, props and redirect")
	date := fs.String("date", "", "the date of the dump, YYYYMMDD. The default is the date in the dump name.")
	workers := fs.Int("workers", 4, "the number of workers that read the dump")
	check := fs.Bool("check", true, "do the check of WP2.6 for a dump revision that is less than 7 days older than the dump")
	api := fs.String("api", wpcorpus.DefaultAPI, "the Action API URL")
	page := fs.Int64("page", 0, "fetch only the article with this page id")
	limit := fs.Int("limit", 0, "fetch only the first N articles in the order of WP2.9")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	started := time.Now().UTC()
	if *dumpPath == "" || *storePath == "" {
		fmt.Fprintln(stderr, "m2 wp fetch: give -dump and -store")
		return ExitUsage
	}
	if *index == "" {
		*index = strings.TrimSuffix(*dumpPath, ".xml.bz2") + "-index.txt.bz2"
	}
	if *date == "" {
		*date = dumpDate(*dumpPath)
	}
	at, err := time.Parse("20060102", *date)
	if err != nil {
		fmt.Fprintf(stderr, "m2 wp fetch: the dump date %q is not YYYYMMDD\n", *date)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 wp fetch: %v\n", err)
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
		if !sticky[c.PageID] && (*page == 0 || c.PageID == *page) {
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

	store, err := wpcorpus.OpenStore(*storePath)
	if err != nil {
		return fail(err)
	}
	ids := make([]int64, len(scope))
	titles := make(map[string]bool, len(scope))
	for i, c := range scope {
		ids[i] = c.PageID
		titles[strings.ReplaceAll(c.Title, " ", "_")] = true
	}
	defer func() { _ = store.Close() }()
	facts, err := store.Facts(ids, titles)
	if err != nil {
		return fail(err)
	}

	r := &fetchRun{corpus: *corpus, m: m, resolve: wpcorpus.LinkResolver(scope, facts.Target), facts: facts, byID: map[int64]*wpcorpus.Candidate{}, dump: *date, stdout: stdout, stderr: stderr}
	want := make(map[int64]bool, len(run))
	for _, c := range run {
		r.byID[c.PageID] = c
		want[c.PageID] = true
	}
	ids2 := map[string]int64{}
	for _, c := range scope {
		ids2[strings.ReplaceAll(c.Title, " ", "_")] = c.PageID
	}

	// The stops of the articles of this run are made again. The other stops
	// stay.
	var out []wpcorpus.Stop
	for _, s := range stops {
		if !want[s.PageID] {
			out = append(out, s)
		}
	}
	stopped := 0
	stop := func(id int64, reason string, into int64, note string) {
		stopped++
		out = append(out, wpcorpus.Stop{PageID: id, Reason: reason, Into: into, Note: note})
		fmt.Fprintf(stdout, "%d stopped: %s %s\n", id, reason, note)
	}

	streams, missing, err := dump.StreamsOf(*index, want)
	if err != nil {
		return fail(err)
	}
	for _, id := range missing {
		stop(id, "deleted", 0, "the page is not in the dump of "+*date)
	}
	var recent []dump.Page
	var merged []dump.Page
	err = dump.ReadPages(*dumpPath, streams, want, *workers, func(p dump.Page) error {
		switch {
		case p.Redirect != "":
			merged = append(merged, p)
		case strings.TrimSpace(p.Text) == "":
			stop(p.ID, "failed", 0, "the dump revision has no wikitext")
		case *check && recentRevision(p.Timestamp, at):
			recent = append(recent, p)
		default:
			r.write(dumpPage(p), p.SHA1, false, nil)
		}
		return nil
	})
	if err != nil {
		return fail(err)
	}

	// A merged article names the page that it goes into. The target of the
	// redirect can be out of scope, so the store gives its page id.
	var targets []string
	for _, p := range merged {
		targets = append(targets, p.Redirect)
	}
	into, err := store.Resolve(targets)
	if err != nil {
		return fail(err)
	}
	for _, p := range merged {
		to := strings.ReplaceAll(p.Redirect, " ", "_")
		id := ids2[to]
		if id == 0 {
			id = into[to]
		}
		if id == 0 {
			stop(p.ID, "failed", 0, "the page redirects to "+p.Redirect+", which the store does not have")
			continue
		}
		stop(p.ID, "merged", id, "the page redirects to "+p.Redirect)
	}

	// The check of WP2.6 uses the Action API, one request at a time.
	f := &wpcorpus.Fetcher{API: *api, UserAgent: userAgent()}
	ctx := context.Background()
	requests := 0
	for i, p := range recent {
		if i > 0 {
			time.Sleep(wpDelay)
		}
		requests++
		hist, err := f.History(ctx, int(p.ID), historyLimit)
		if err != nil {
			stop(p.ID, "failed", 0, err.Error())
			continue
		}
		rev, unstable := wpcorpus.RecordRevision(hist, at)
		var settled *wpcorpus.Revision
		if s, ok := wpcorpus.NewestSettled(hist, at); ok {
			settled = &s
		}
		if unstable || rev == int(p.RevID) {
			r.write(dumpPage(p), p.SHA1, unstable, settled)
			continue
		}
		time.Sleep(wpDelay)
		requests++
		got, err := f.FetchRevision(ctx, rev)
		if err != nil {
			stop(p.ID, "failed", 0, err.Error())
			continue
		}
		sha := ""
		for _, h := range hist {
			if h.ID == rev {
				sha = h.SHA1
			}
		}
		r.write(got, sha, false, settled)
	}

	if err := wpcorpus.WriteStops(*corpus, out); err != nil {
		return fail(err)
	}
	rec := &wpcorpus.RunRecord{Command: "m2 wp fetch", Args: args, M2: version, Wiki: wpcorpus.WikiVersion(), Dump: *date,
		Started: started.Format(time.RFC3339), Finished: time.Now().UTC().Format(time.RFC3339),
		Articles: len(run), Pages: r.pages, Stopped: stopped, Failed: r.failed, Requests: requests, Failures: r.failures}
	rel, err := wpcorpus.WriteRunRecord(*corpus, rec)
	if err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "%d articles, %d pages, %d objects, %d checked with the API, %d stopped, %d failed, run record %s\n",
		len(run), r.pages, r.objects, len(recent), len(out), r.failed, rel)
	if r.failed > 0 {
		return ExitFailure
	}
	return ExitOK
}

// write writes the content, analysis, Markdown and metadata files of one
// article. settled is the newest settled revision of the history, or nil
// when the command did not read the history.
func (r *fetchRun) write(p *wpcorpus.Page, sha string, unstable bool, settled *wpcorpus.Revision) {
	c := r.byID[int64(p.ID)]
	err := wpcorpus.WriteContent(r.corpus, p)
	var res *wpcorpus.Result
	if err == nil {
		res, err = wpcorpus.Extract(p, r.m, "m2/"+version)
	}
	if err == nil {
		err = wpcorpus.WriteResult(r.corpus, res)
	}
	if err == nil {
		err = wpcorpus.RemoveStale(r.corpus, p.ID, p.Revision)
	}
	if err == nil {
		var md []byte
		if md, err = wpcorpus.Render(p, r.m, res, r.resolve, "m2/"+version); err == nil {
			err = wpcorpus.WriteMarkdown(r.corpus, p.ID, md)
		}
	}
	if err == nil {
		cats, short := wpcorpus.PageFacts(p.Wikitext)
		if s := r.facts.Short[int64(p.ID)]; s != "" {
			short = s
		}
		md := &wpcorpus.Metadata{
			Page: p.ID, Title: p.Title, Revision: p.Revision, Timestamp: p.Timestamp, Length: len(p.Wikitext), SHA1: sha, Dump: r.dump,
			Type: wpcorpus.ArticleType(p.Title), Short: short, Categories: cats, Wikidata: r.facts.Wikidata[int64(p.ID)],
			Redirects: r.facts.Redirects[strings.ReplaceAll(p.Title, " ", "_")], Unstable: unstable,
		}
		if settled != nil && settled.ID != p.Revision {
			md.Settled, md.SettledTimestamp = settled.ID, settled.Timestamp
		}
		if c != nil {
			md.Signals, md.Rule, md.Projects, md.Class, md.Importance = c.Signals.String(), c.Rule, c.Projects, c.Class, c.Importance
		}
		err = wpcorpus.WriteMetadata(r.corpus, md)
	}
	if err != nil {
		fmt.Fprintf(r.stderr, "m2 wp fetch: %d %s: %v\n", p.ID, p.Title, err)
		r.failed++
		r.failures = append(r.failures, fmt.Sprintf("%d r%d: %v", p.ID, p.Revision, err))
		return
	}
	r.pages++
	r.objects += len(res.Objects)
	fmt.Fprintf(r.stdout, "%d r%d %s: %d objects, %d cues, %d gaps\n", p.ID, p.Revision, p.Title, len(res.Objects), len(res.Census.Cues), len(res.Census.Gaps()))
}

func dumpPage(p dump.Page) *wpcorpus.Page {
	return &wpcorpus.Page{ID: int(p.ID), Title: p.Title, Revision: int(p.RevID), Timestamp: p.Timestamp, Wikitext: p.Text}
}

// recentRevision tells if a revision is less than 7 days older than the
// dump, so that the check of WP2.6 must look at it.
func recentRevision(ts string, dumpAt time.Time) bool {
	t, err := time.Parse(time.RFC3339, ts)
	return err != nil || dumpAt.Sub(t) < 7*24*time.Hour
}

// dumpDate gives the date in the name of a dump file, such as 20261001 in
// enwiki-20261001-pages-articles-multistream.xml.bz2.
func dumpDate(path string) string {
	for _, f := range strings.Split(filepath.Base(path), "-") {
		if len(f) == 8 && strings.Trim(f, "0123456789") == "" {
			return f
		}
	}
	return ""
}

// readPilot reads the titles of the pilot area, manifests/wikipedia/pilot.txt.
// A corpus with no such file has no pilot.
func readPilot(corpus string) (map[string]bool, error) {
	ts, err := readTitles(filepath.Join(corpus, wpcorpus.ManifestDir, "pilot.txt"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(ts))
	for _, t := range ts {
		out[wpcorpus.NormTitle(t)] = true
	}
	return out, nil
}
