package cli

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	wpImport "github.com/tamnd/math2-agent/importer/wikipedia"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	rescluster "github.com/tamnd/math2-agent/resource/cluster"
	resresolve "github.com/tamnd/math2-agent/resource/resolve"
	"github.com/tamnd/math2-agent/wpcorpus"
)

const wpCitesUsage = "m2 wp cites [-corpus DIR] [-root DIR] [-live] [-gap D] [-workers N] [-page ID]"

// wpObjFile is the newest objects file of one page.
type wpObjFile struct{ page, rev int }

// runWPCites resolves the citations of the corpus to the resources of the math2 tree (corpus wikipedia WP10.2). It writes the citation file of each page to analysis/citations/ and the report reports/citations.md of the corpus.
func runWPCites(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 wp cites", flag.ContinueOnError)
	fs.SetOutput(stderr)
	corpus := fs.String("corpus", ".", "the Wikipedia corpus (a clone of tamnd/wikipedia-math)")
	root := fs.String("root", "", "the math2 tree with the catalogue of resources in records/resources/")
	live := fs.Bool("live", false, "ask Crossref for the DOIs that the catalogue and the crosswalks do not have. The command then reads one page at a time")
	gap := fs.Duration("gap", time.Second, "the shortest time between two requests to a live API")
	workers := fs.Int("workers", 8, "the number of pages to resolve at the same time")
	page := fs.Int("page", 0, "resolve only the page with this id. The report is not written")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 || *root == "" {
		fmt.Fprintln(stderr, "usage: "+wpCitesUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 wp cites: %v\n", err)
		return ExitFailure
	}
	rs, err := resource.ReadResources(*root)
	if err != nil {
		return fail(err)
	}
	links, err := rescluster.ReadCrosswalks(*root)
	if err != nil {
		return fail(err)
	}
	r := &resresolve.Resolver{Index: resresolve.Build(rs), Crosswalks: resresolve.Links(links)}
	cachePath := filepath.Join(*root, resresolve.CacheFile)
	var dead func(string) bool
	if *live {
		if r.Cache, err = resresolve.ReadCache(cachePath, time.Now().UTC().Format(time.DateOnly)); err != nil {
			return fail(err)
		}
		cr := resresolve.Crossref{Base: resresolve.CrossrefBase, Client: fetch.NewClient(*gap)}
		r.Fetchers = []resresolve.Fetcher{cr}
		dead = func(id string) bool {
			others, ok := r.Cache.Get(cr.Name(), id)
			return ok && len(others) == 0
		}
		*workers = 1
	}
	files, err := wpObjFiles(*corpus)
	if err != nil {
		return fail(err)
	}
	if *page != 0 {
		files = slices.DeleteFunc(files, func(f wpObjFile) bool { return f.page != *page })
	}
	ctx := context.Background()
	out := make([]*wpImport.CiteFile, len(files))
	errs := make([]error, len(files))
	var wg sync.WaitGroup
	next := make(chan int)
	for range max(*workers, 1) {
		wg.Go(func() {
			for i := range next {
				out[i], errs[i] = wpCitesPage(ctx, r, *corpus, files[i], dead)
			}
		})
	}
	for i := range files {
		next <- i
	}
	close(next)
	wg.Wait()
	if err := r.Cache.Write(cachePath); err != nil {
		return fail(err)
	}
	for _, err := range errs {
		if err != nil {
			return fail(err)
		}
	}
	sum := citeSummary(out)
	fmt.Fprintf(stdout, "%d pages, %d citations, %d found, %d with candidates, %d not found, %d with no fields, %d with a bad id\n", len(out), sum.total, sum.status[resresolve.Found], sum.status[resresolve.Candidates], sum.status[resresolve.NotFound], sum.status[wpImport.CiteNoFields], sum.bad)
	if *page != 0 {
		return ExitOK
	}
	if err := writePlain(filepath.Join(*corpus, wpImport.CitationsReport), citeReport(sum, len(out), time.Now().UTC().Format(time.DateOnly))); err != nil {
		return fail(err)
	}
	return ExitOK
}

// wpObjFiles gives the newest objects file of each page of the corpus, in the order of the page ids.
func wpObjFiles(corpus string) ([]wpObjFile, error) {
	paths, err := filepath.Glob(filepath.Join(corpus, wpcorpus.ObjectsDir, "*", "*-r*.md"))
	if err != nil {
		return nil, err
	}
	newest := map[int]int{}
	for _, p := range paths {
		name := strings.TrimSuffix(filepath.Base(p), ".md")
		ps, rs, _ := strings.Cut(name, "-r")
		pg, err1 := strconv.Atoi(ps)
		rev, err2 := strconv.Atoi(rs)
		if err1 != nil || err2 != nil {
			continue
		}
		newest[pg] = max(newest[pg], rev)
	}
	var out []wpObjFile
	for _, pg := range slices.Sorted(maps.Keys(newest)) {
		out = append(out, wpObjFile{pg, newest[pg]})
	}
	return out, nil
}

// wpCitesPage resolves the citations of one page and writes its citation file.
func wpCitesPage(ctx context.Context, r *resresolve.Resolver, corpus string, f wpObjFile, dead func(string) bool) (*wpImport.CiteFile, error) {
	objs, err := wpcorpus.ReadObjects(corpus, f.page, f.rev)
	if err != nil {
		return nil, err
	}
	title, err := objectsTitle(filepath.Join(corpus, wpcorpus.ObjectsPath(f.page, f.rev)))
	if err != nil {
		return nil, err
	}
	cf, err := wpImport.ResolveCitations(ctx, r, f.page, f.rev, title, objs, dead)
	if err != nil {
		return nil, err
	}
	data, err := wpImport.MarshalCiteFile(cf)
	if err != nil {
		return nil, err
	}
	return cf, writePlain(filepath.Join(corpus, wpImport.CitationPath(f.page, f.rev)), data)
}

// objectsTitle reads the title from the YAML header of an objects file.
func objectsTitle(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- the path is in the corpus that the user names.
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for n := 0; sc.Scan() && n < 20; n++ {
		if v, ok := strings.CutPrefix(sc.Text(), "title: "); ok {
			return strings.Trim(v, `"'`), nil
		}
	}
	return "", sc.Err()
}

// citeSum is the summary of the citation files for the report.
type citeSum struct {
	total, bad, dead int
	status, pass     map[string]int
	systems          map[string]int // the citations with an id of each system
	cues             map[string]int // the citations with no fields, by cue
	resources        map[string]int // the citations of each resource
	badList          []string       // the bad and the dead ids, as title (ref): id
}

func citeSummary(files []*wpImport.CiteFile) citeSum {
	s := citeSum{status: map[string]int{}, pass: map[string]int{}, systems: map[string]int{}, cues: map[string]int{}, resources: map[string]int{}}
	for _, f := range files {
		for _, c := range f.Citations {
			s.total++
			s.status[c.Status]++
			if c.Pass != "" && c.Status == resresolve.Found {
				s.pass[c.Pass]++
			}
			if c.Status == wpImport.CiteNoFields {
				s.cues[c.Cue]++
			}
			seen := map[string]bool{}
			for _, id := range c.IDs {
				sys, _, _ := strings.Cut(id, ":")
				if !seen[sys] {
					seen[sys] = true
					s.systems[sys]++
				}
			}
			if c.Resource != "" {
				s.resources[c.Resource]++
			}
			if len(c.BadIDs) > 0 {
				s.bad++
			}
			if len(c.DeadIDs) > 0 {
				s.dead++
			}
			for _, id := range append(slices.Clone(c.BadIDs), c.DeadIDs...) {
				s.badList = append(s.badList, fmt.Sprintf("%s (%s): %s", f.Title, c.Ref, id))
			}
		}
	}
	return s
}

// citeReport gives reports/citations.md of the corpus.
func citeReport(s citeSum, pages int, day string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "---\nsource: wikipedia\ndate: %s\nresolver: %s\npages: %d\ncitations: %d\nfound: %d\ncandidates: %d\nnot_found: %d\nno_fields: %d\nbad_ids: %d\ndead_ids: %d\n---\n\n", day, resresolve.Version, pages, s.total, s.status[resresolve.Found], s.status[resresolve.Candidates], s.status[resresolve.NotFound], s.status[wpImport.CiteNoFields], s.bad, s.dead)
	b.WriteString("# Citations\n\n")
	fmt.Fprintf(&b, "This report gives the resolution of the citations of the corpus to the resources of math2 (corpus wikipedia WP10.2). m2 wp cites writes it, and the citation file of each page is in %s. A citation resolves by an id of its template first, and then by the matcher of the title, the authors and the year. A citation with no fields is a short citation or a <ref> with free text. Rule Y16 reports a citation with an id that does not have the syntax of its system, or that a live API does not know.\n\n", wpImport.CitationsDir)
	fmt.Fprintf(&b, "The corpus has %d pages and %d citations. %s of the citations resolve to a resource.\n\n", pages, s.total, percent(s.status[resresolve.Found], s.total))
	b.WriteString("| State | Citations |\n|---|---|\n")
	for _, k := range []string{resresolve.Found, resresolve.Candidates, resresolve.NotFound, wpImport.CiteNoFields} {
		fmt.Fprintf(&b, "| %s | %d |\n", k, s.status[k])
	}
	writeCounts(&b, "Pass of a found citation", s.pass, 0)
	writeCounts(&b, "System of an id", s.systems, 0)
	writeCounts(&b, "Cue of a citation with no fields", s.cues, 0)
	writeCounts(&b, "Resource", s.resources, 30)
	if len(s.badList) > 0 {
		fmt.Fprintf(&b, "\n## Bad and dead ids\n\n%d citations have an id that does not resolve (rule Y16). The first 100 are here.\n\n", s.bad+s.dead)
		for _, l := range s.badList[:min(100, len(s.badList))] {
			fmt.Fprintf(&b, "- %s\n", l)
		}
	}
	return b.Bytes()
}

// writeCounts writes a table of counts, the largest first, with at most n rows when n is above 0.
func writeCounts(b *bytes.Buffer, head string, m map[string]int, n int) {
	if len(m) == 0 {
		return
	}
	keys := slices.Sorted(maps.Keys(m))
	slices.SortStableFunc(keys, func(x, y string) int { return cmp.Compare(m[y], m[x]) })
	if n > 0 && len(keys) > n {
		keys = keys[:n]
	}
	fmt.Fprintf(b, "\n| %s | Citations |\n|---|---|\n", head)
	for _, k := range keys {
		fmt.Fprintf(b, "| %s | %d |\n", k, m[k])
	}
}

func percent(n, d int) string {
	if d == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(n)/float64(d))
}

// writePlain writes a Markdown file of the corpus, and makes its directory.
func writePlain(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- the corpus is public.
		return err
	}
	return os.WriteFile(path, data, 0o644) // #nosec G306 -- the corpus is public text in a git tree.
}
