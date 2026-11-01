package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tamnd/math2-agent/wpcorpus"
)

// runWPEdges writes the edge file of each article of the content plane and
// the edge report (WP06). It reads the objects files of m2 wp extract for
// the tags. With -store, it resolves the links out of the scope to page ids
// and finds the red links.
func runWPEdges(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 wp edges", flag.ContinueOnError)
	fs.SetOutput(stderr)
	corpus := fs.String("corpus", ".", "the Wikipedia corpus (a clone of tamnd/wikipedia-math)")
	storePath := fs.String("store", "", "the SQLite store of the scope pass, with the tables page and redirect")
	page := fs.Int("page", 0, "write only the edges of the page with this id")
	workers := fs.Int("workers", 4, "the number of pages to read and write at the same time")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if *workers < 1 {
		*workers = 1
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 wp edges: %v\n", err)
		return ExitFailure
	}
	m, err := wpcorpus.Load(*corpus)
	if err != nil {
		return fail(err)
	}

	all, err := wpcorpus.ReadScopeFile(*corpus, wpcorpus.ScopePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(err)
	}
	var scope []*wpcorpus.Candidate
	titleOf := map[int]string{}
	for _, c := range all {
		if c.InScope {
			scope = append(scope, c)
			titleOf[int(c.PageID)] = c.Title
		}
	}
	ctx := wpcorpus.EdgeContext{Scope: wpcorpus.LinkResolver(scope, nil)}
	var store *wpcorpus.Store
	short := map[int]string{}
	if *storePath != "" {
		if store, err = wpcorpus.OpenStore(*storePath); err != nil {
			return fail(err)
		}
		defer func() { _ = store.Close() }()
		ids := make([]int64, len(scope))
		titles := make(map[string]bool, len(scope))
		for i, c := range scope {
			ids[i] = c.PageID
			titles[strings.ReplaceAll(c.Title, " ", "_")] = true
		}
		facts, err := store.Facts(ids, titles)
		if err != nil {
			return fail(err)
		}
		ctx.Scope = wpcorpus.LinkResolver(scope, facts.Target)
		for id, s := range facts.Short {
			short[int(id)] = s
		}
	}
	metas, err := wpcorpus.ReadMetadataFiles(*corpus)
	if err != nil {
		return fail(err)
	}
	meta := map[int]*wpcorpus.Metadata{}
	for _, md := range metas {
		meta[md.Page] = md
	}
	ctx.Type = func(p int) string {
		if md := meta[p]; md != nil {
			if wpcorpus.IsType(md.Type) {
				return md.Type
			}
			return wpcorpus.TypeOf(md.Title, md.Short, md.Categories)
		}
		return wpcorpus.TypeOf(titleOf[p], short[p], nil)
	}

	ids, err := wpcorpus.Pages(*corpus)
	if err != nil {
		return fail(err)
	}
	// The first pass extracts each page again and joins the tags of the
	// objects files. The resolution needs the objects of each page, so it
	// waits for the second pass.
	results := map[int]*wpcorpus.Result{}
	var order []int
	failed := 0
	var mu sync.Mutex
	eachPage(ids, *workers, func(id int) {
		r, err := edgeResult(*corpus, id, m)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			fmt.Fprintf(stderr, "m2 wp edges: page %d: %v\n", id, err)
			failed++
			return
		}
		results[id] = r
	})
	for _, id := range ids {
		if results[id] != nil && (*page == 0 || id == *page) {
			order = append(order, id)
		}
	}
	ctx.Objects = func(p int) []wpcorpus.Object {
		if r := results[p]; r != nil {
			return r.Objects
		}
		return nil
	}

	outTitles := map[int]string{}
	if store != nil {
		want := map[string]bool{}
		for _, id := range order {
			for _, u := range results[id].Links {
				if u.Title != "" && !strings.Contains(u.Title, ":") && ctx.Scope(u.Title) == 0 {
					want[u.Title] = true
				}
			}
		}
		list := make([]string, 0, len(want))
		for t := range want {
			list = append(list, t)
		}
		found, err := store.Resolve(list)
		if err != nil {
			return fail(err)
		}
		ctx.Page = func(t string) (int, bool) {
			id, ok := found[strings.ReplaceAll(strings.TrimSpace(t), " ", "_")]
			return int(id), ok
		}
		for _, t := range list {
			if id, ok := ctx.Page(t); ok && outTitles[id] == "" {
				outTitles[id] = t
			}
		}
	}

	// The context is read only, so the workers build and write the edge
	// files at the same time. The summary adds the files in page order.
	files := make(map[int]*wpcorpus.EdgeFile, len(order))
	eachPage(order, *workers, func(id int) {
		f := wpcorpus.BuildEdges(results[id], ctx)
		err := wpcorpus.WriteEdges(*corpus, f)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			fmt.Fprintf(stderr, "m2 wp edges: %s: %v\n", f.Title, err)
			failed++
			return
		}
		files[id] = f
	})
	var sum wpcorpus.EdgeSummary
	for _, id := range order {
		if f := files[id]; f != nil {
			sum.Add(f)
		}
	}
	if *page == 0 {
		path := filepath.Join(*corpus, wpcorpus.EdgesReportPath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- the corpus is a public repository
			return fail(err)
		}
		if err := writeAtomic(path, func(w io.Writer) error { return wpcorpus.EdgeReport(w, sum, outTitles) }); err != nil {
			return fail(err)
		}
	}
	fmt.Fprintf(stdout, "%d pages, %d edges, %d broken anchors, %d red links, %d failed\n", sum.Pages, sum.Edges, sum.Broken, len(sum.RedLinks), failed)
	if failed > 0 {
		return ExitFailure
	}
	return ExitOK
}

// edgeResult extracts a page again and joins the tags of its objects file.
func edgeResult(corpus string, id int, m *wpcorpus.Manifests) (*wpcorpus.Result, error) {
	p, err := wpcorpus.ReadContent(corpus, id)
	if err != nil {
		return nil, err
	}
	in, err := wpcorpus.ReadPointers(corpus, id)
	if err != nil {
		return nil, err
	}
	r, err := wpcorpus.ExtractPage(p, m, "m2/"+version, in)
	if err != nil {
		return nil, err
	}
	objs, err := wpcorpus.ReadObjects(corpus, p.ID, p.Revision)
	if err != nil {
		return nil, err
	}
	if err := wpcorpus.JoinTags(r, objs); err != nil {
		return nil, err
	}
	r.Slim()
	return r, nil
}

// eachPage calls fn for each id, with n workers.
func eachPage(ids []int, n int, fn func(id int)) {
	ch := make(chan int)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range ch {
				fn(id)
			}
		}()
	}
	for _, id := range ids {
		ch <- id
	}
	close(ch)
	wg.Wait()
}
