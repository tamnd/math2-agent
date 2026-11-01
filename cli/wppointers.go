package cli

import (
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

// runWPPointers writes the incoming pointers of each article in scope, the
// witness W-in (WP4.2). The redirects with a fragment come from the store.
// The links with an anchor come from the wikitext of each article of the
// dump, when -dump is given.
func runWPPointers(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 wp pointers", flag.ContinueOnError)
	fs.SetOutput(stderr)
	corpus := fs.String("corpus", ".", "the Wikipedia corpus (a clone of tamnd/wikipedia-math)")
	storePath := fs.String("store", "", "the SQLite store of the scope pass, with the tables page and redirect")
	dumpPath := fs.String("dump", "", "the multistream XML dump. Without it, the command reads only the redirects.")
	index := fs.String("index", "", "the index of the dump. The default is the dump path with -index.txt.bz2.")
	workers := fs.Int("workers", 4, "the number of workers that read the dump")
	cache := fs.String("cache", "", "the anchor cache of the dump. When the file exists, the command reads the anchor links from it and not from the dump. When it does not exist, the command writes it.")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	started := time.Now().UTC()
	if *storePath == "" {
		fmt.Fprintln(stderr, "m2 wp pointers: give -store")
		return ExitUsage
	}
	if *index == "" {
		*index = strings.TrimSuffix(*dumpPath, ".xml.bz2") + "-index.txt.bz2"
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 wp pointers: %v\n", err)
		return ExitFailure
	}

	all, err := wpcorpus.ReadScopeFile(*corpus, wpcorpus.ScopePath)
	if err != nil {
		return fail(err)
	}
	var scope []*wpcorpus.Candidate
	var ids []int64
	titles := map[string]bool{}
	for _, c := range all {
		if c.InScope {
			scope = append(scope, c)
			ids = append(ids, c.PageID)
			titles[strings.ReplaceAll(c.Title, " ", "_")] = true
		}
	}
	store, err := wpcorpus.OpenStore(*storePath)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = store.Close() }()
	in, err := store.RedirectPointers(titles)
	if err != nil {
		return fail(err)
	}
	redirects := 0
	for _, ps := range in {
		redirects += len(ps)
	}

	links, pages, date := 0, 0, ""
	if *dumpPath != "" {
		date = dumpDate(*dumpPath)
		facts, err := store.Facts(ids, titles)
		if err != nil {
			return fail(err)
		}
		target := wpcorpus.ScopeTarget(scope, facts.Target)
		add := func(from string, ls []wpcorpus.AnchorLink) {
			for to, ps := range wpcorpus.AnchorPointers(from, ls, target) {
				in[to] = append(in[to], ps...)
				links += len(ps)
			}
		}
		if _, err := os.Stat(*cache); *cache != "" && err == nil {
			name, n, err := wpcorpus.ReadAnchorCache(*cache, func(from string, ls []wpcorpus.AnchorLink) error {
				add(from, ls)
				return nil
			})
			if err != nil {
				return fail(err)
			}
			if want := filepath.Base(*dumpPath); name != want {
				return fail(fmt.Errorf("the anchor cache %s is of the dump %s, not of %s", *cache, name, want))
			}
			pages = n
		} else {
			var ac *wpcorpus.AnchorCache
			if *cache != "" {
				if ac, err = wpcorpus.CreateAnchorCache(*cache, filepath.Base(*dumpPath)); err != nil {
					return fail(err)
				}
			}
			streams, _, err := dump.StreamsOf(*index, nil)
			if err != nil {
				return fail(err)
			}
			err = dump.ReadPages(*dumpPath, streams, nil, *workers, func(p dump.Page) error {
				if p.NS != 0 || p.Redirect != "" {
					return nil
				}
				pages++
				ls := wpcorpus.AnchorLinks(p.Text)
				add(p.Title, ls)
				if ac != nil {
					return ac.Add(p.Title, ls)
				}
				return nil
			})
			if err != nil {
				if ac != nil {
					ac.Abort()
				}
				return fail(err)
			}
			if ac != nil {
				if err := ac.Close(); err != nil {
					return fail(err)
				}
			}
		}
	}

	for _, c := range scope {
		f := &wpcorpus.PointerFile{Page: int(c.PageID), Title: c.Title, Pointers: in[strings.ReplaceAll(c.Title, " ", "_")]}
		wpcorpus.SortPointers(f.Pointers)
		if err := wpcorpus.WritePointers(*corpus, f); err != nil {
			return fail(err)
		}
	}
	rec := &wpcorpus.RunRecord{Command: "m2 wp pointers", Args: args, M2: version, Wiki: wpcorpus.WikiVersion(), Dump: date,
		Started: started.Format(time.RFC3339), Finished: time.Now().UTC().Format(time.RFC3339), Articles: len(scope), Pages: pages}
	rel, err := wpcorpus.WriteRunRecord(*corpus, rec)
	if err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "%d articles, %d redirects and %d links with an anchor, %d pages of the dump read, run record %s\n", len(scope), redirects, links, pages, rel)
	return ExitOK
}
