package cli

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/tamnd/math2-agent/wpcorpus"
)

// errNextLoad tells that the store needs one more table. The message gives
// the command that loads it.
type errNextLoad struct{ table, cmd string }

func (e *errNextLoad) Error() string {
	return fmt.Sprintf("the store has no table %s. Load it with:\n  %s\nThen run m2 wp scope again.", e.table, e.cmd)
}

// scopeRun holds the inputs of one run of m2 wp scope.
type scopeRun struct {
	store    *wpcorpus.Store
	storeArg string
	dump     string
	work     string
	stdout   io.Writer
}

func (r *scopeRun) need(table, load string) error {
	ok, err := r.store.Has(table)
	if err != nil || ok {
		return err
	}
	return &errNextLoad{table, fmt.Sprintf("wiki dump load --db %s --date %s %s", r.storeArg, r.dump, load)}
}

func runWPScope(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 wp scope", flag.ContinueOnError)
	fs.SetOutput(stderr)
	corpus := fs.String("corpus", ".", "the Wikipedia corpus (a clone of tamnd/wikipedia-math)")
	storePath := fs.String("store", "", "the SQLite store of the dump, made with wiki dump load (required)")
	dump := fs.String("dump", "", "the date of the dump, YYYYMMDD (required)")
	assess := fs.String("assess", "", "the output of wiki assess -o tsv for the projects of S1 to S3 (required)")
	cited := fs.String("cited", "", "a file with the titles that math2 cites, one on each line (signal S4)")
	wikidata := fs.String("wikidata", "", "a file with the titles that have a Wikidata class or identifier of mathematics, one on each line (signal S5)")
	work := fs.String("work", "work/scope", "the directory for the id files of the next loads")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if *storePath == "" || *dump == "" || *assess == "" || fs.NArg() > 0 {
		fmt.Fprintln(stderr, "usage: m2 wp scope -store DB -dump DATE -assess FILE [-cited FILE] [-wikidata FILE] [-corpus DIR] [-work DIR]")
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 wp scope: %v\n", err)
		return ExitFailure
	}
	tree, err := wpcorpus.LoadCategoryTree(*corpus)
	if err != nil {
		return fail(err)
	}
	st, err := wpcorpus.OpenStore(*storePath)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = st.Close() }()
	if err := os.MkdirAll(*work, 0o750); err != nil {
		return fail(err)
	}
	r := &scopeRun{store: st, storeArg: *storePath, dump: *dump, work: *work, stdout: stdout}
	cs, pending, err := r.build(tree, *assess, *cited, *wikidata)
	if err != nil {
		return fail(err)
	}

	dir := filepath.Join(*corpus, wpcorpus.ManifestDir)
	for _, f := range []struct {
		path    string
		inScope bool
	}{{filepath.Join(dir, "candidates.tsv"), false}, {filepath.Join(dir, "scope.tsv"), true}} {
		if err := writeAtomic(f.path, func(w io.Writer) error { return wpcorpus.WriteCandidates(w, cs, *dump, f.inScope) }); err != nil {
			return fail(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(*corpus, "reports"), 0o750); err != nil {
		return fail(err)
	}
	if err := writeAtomic(filepath.Join(*corpus, "reports", "scope.md"), func(w io.Writer) error {
		return wpcorpus.ScopeReport(w, cs, *dump, pending)
	}); err != nil {
		return fail(err)
	}
	in := 0
	for _, c := range cs {
		if c.InScope {
			in++
		}
	}
	fmt.Fprintf(stdout, "%d candidates, %d in scope\n", len(cs), in)
	return ExitOK
}

// build computes the signals and the verdict of each candidate. It gives the
// names of the signals that it could not compute.
func (r *scopeRun) build(tree *wpcorpus.CategoryTree, assess, cited, wikidata string) ([]*wpcorpus.Candidate, []string, error) {
	for _, t := range []struct{ table, load string }{
		{wpcorpus.TablePage, "page --columns page_id,page_namespace,page_title,page_is_redirect,page_len,page_latest --where page_namespace=0,14 --index page_id --index page_title"},
		{wpcorpus.TableRedirect, "redirect --columns rd_from,rd_namespace,rd_title,rd_fragment --where rd_namespace=0 --index rd_from"},
		{wpcorpus.TableProps, "page_props --as props --where pp_propname=wikibase_item,disambiguation --index pp_page"},
		{wpcorpus.TableCatTargets, "linktarget --as cat_targets --where lt_namespace=14 --index lt_title --index lt_id"},
		{wpcorpus.TableSubcats, "categorylinks --as subcats --columns cl_from,cl_target_id --where cl_type=subcat --index cl_target_id"},
	} {
		if err := r.need(t.table, t.load); err != nil {
			return nil, nil, err
		}
	}

	cands := map[int64]*wpcorpus.Candidate{}
	get := func(id int64) *wpcorpus.Candidate {
		c := cands[id]
		if c == nil {
			c = &wpcorpus.Candidate{PageID: id, CatDepth: -1}
			cands[id] = c
		}
		return c
	}

	// S1 to S3: the project tags. The API gives the page id of the page
	// that has the tag. A page that is now a redirect is resolved by title.
	rows, err := readAssessFile(assess)
	if err != nil {
		return nil, nil, err
	}
	var titles []string
	for _, a := range rows {
		titles = append(titles, a.Title)
	}
	ids, err := r.store.Resolve(titles)
	if err != nil {
		return nil, nil, err
	}
	var s1 []int64
	for _, a := range rows {
		sig := wpcorpus.ProjectSignal(a.Project)
		id, ok := ids[a.Title]
		if sig == 0 || !ok {
			continue
		}
		c := get(id)
		c.Signals |= sig
		if !slices.Contains(c.Projects, a.Project) {
			c.Projects = append(c.Projects, a.Project)
		}
		// A tag on a redirect gives the signal to the target, but not its
		// class, which is the class Redirect.
		if id == a.PageID && (a.Project == "Mathematics" || c.Class == "") {
			c.Class, c.Importance = a.Class, a.Importance
		}
		if sig == wpcorpus.S1 {
			s1 = append(s1, id)
		}
	}
	slices.Sort(s1)
	s1 = slices.Compact(s1)

	// S4 and S5: title lists.
	for _, f := range []struct {
		path string
		sig  wpcorpus.Signals
	}{{cited, wpcorpus.S4}, {wikidata, wpcorpus.S5}} {
		if f.path == "" {
			continue
		}
		ts, err := readTitles(f.path)
		if err != nil {
			return nil, nil, err
		}
		ids, err := r.store.Resolve(ts)
		if err != nil {
			return nil, nil, err
		}
		for _, id := range ids {
			get(id).Signals |= f.sig
		}
	}

	// S6: the category tree.
	cats, err := r.store.Tree(tree)
	if err != nil {
		return nil, nil, err
	}
	treeIDs := filepath.Join(r.work, "tree-ids.txt")
	if err := writeIDs(treeIDs, cats, func(c wpcorpus.TreeCategory) int64 { return c.ID }); err != nil {
		return nil, nil, err
	}
	fmt.Fprintf(r.stdout, "the category tree has %d categories\n", len(cats))
	if err := r.need(wpcorpus.TableCatPages, "categorylinks --as cat_pages --columns cl_from,cl_target_id --where cl_type=page --in cl_target_id="+treeIDs); err != nil {
		return nil, nil, err
	}
	members, err := r.store.CategoryMembers(cats)
	if err != nil {
		return nil, nil, err
	}
	for id, d := range members {
		c := get(id)
		c.Signals |= wpcorpus.S6
		c.CatDepth = d
	}

	// S8 and S9: the links from the articles of S1.
	s1IDs := filepath.Join(r.work, "s1-ids.txt")
	if err := writeIDs(s1IDs, s1, func(id int64) int64 { return id }); err != nil {
		return nil, nil, err
	}
	if err := r.need(wpcorpus.TableS1Links, "pagelinks --as s1_links --columns pl_from,pl_target_id --where pl_from_namespace=0 --in pl_from="+s1IDs+" --index pl_target_id"); err != nil {
		return nil, nil, err
	}
	if ok, err := r.store.Has(wpcorpus.TableLinkTarget); err != nil {
		return nil, nil, err
	} else if !ok {
		targets, err := r.store.LinkTargetIDs()
		if err != nil {
			return nil, nil, err
		}
		p := filepath.Join(r.work, "s1-target-ids.txt")
		if err := writeIDs(p, targets, func(id int64) int64 { return id }); err != nil {
			return nil, nil, err
		}
		if err := r.need(wpcorpus.TableLinkTarget, "linktarget --as link_targets --where lt_namespace=0 --in lt_id="+p+" --index lt_id"); err != nil {
			return nil, nil, err
		}
	}
	s1Titles, err := r.store.Articles(s1)
	if err != nil {
		return nil, nil, err
	}
	lists := map[int64]bool{}
	for id, t := range s1Titles {
		if wpcorpus.ListArticle(t) {
			lists[id] = true
		}
	}
	sources, fromLists, err := r.store.LinkCounts(lists)
	if err != nil {
		return nil, nil, err
	}
	var linked []string
	for t, n := range sources {
		if n >= 3 || fromLists[t] > 0 {
			linked = append(linked, t)
		}
	}
	linkIDs, err := r.store.Resolve(linked)
	if err != nil {
		return nil, nil, err
	}
	var s9 []int64
	links := map[int64]int{}
	for t, id := range linkIDs {
		links[id] = max(links[id], sources[t])
		if fromLists[t] > 0 {
			get(id).Signals |= wpcorpus.S8
		}
		if sources[t] >= 3 {
			s9 = append(s9, id)
		}
	}
	slices.Sort(s9)
	s9 = slices.Compact(s9)
	s9IDs := filepath.Join(r.work, "s9-ids.txt")
	if err := writeIDs(s9IDs, s9, func(id int64) int64 { return id }); err != nil {
		return nil, nil, err
	}
	if err := r.need(wpcorpus.TablePageCats, "categorylinks --as page_cats --columns cl_from,cl_target_id --where cl_type=page --in cl_from="+s9IDs+" --index cl_from"); err != nil {
		return nil, nil, err
	}
	pageCats, err := r.store.PageCategories()
	if err != nil {
		return nil, nil, err
	}
	for _, id := range s9 {
		if slices.ContainsFunc(pageCats[id], tree.MathKeyword) {
			get(id).Signals |= wpcorpus.S9
		}
	}
	for id, n := range links {
		if c := cands[id]; c != nil {
			c.Links = n
		}
	}

	// Keep the articles, and classify them.
	all := make([]int64, 0, len(cands))
	for id := range cands {
		all = append(all, id)
	}
	articles, err := r.store.Articles(all)
	if err != nil {
		return nil, nil, err
	}
	out := make([]*wpcorpus.Candidate, 0, len(articles))
	for id, t := range articles {
		c := cands[id]
		if c.Signals == 0 {
			continue
		}
		c.Title = t
		wpcorpus.Classify(c)
		out = append(out, c)
	}
	var pending []string
	if cited == "" {
		pending = append(pending, "S4")
	}
	if wikidata == "" {
		pending = append(pending, "S5")
	}
	pending = append(pending, "S7 (it needs the wikitext of WP-M2)")
	return out, pending, nil
}

func readAssessFile(path string) ([]wpcorpus.AssessRow, error) {
	f, err := os.Open(path) // #nosec G304 -- the user names the file
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	rows, err := wpcorpus.ReadAssess(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return rows, nil
}

func writeIDs[T any](path string, v []T, id func(T) int64) error {
	return writeAtomic(path, func(w io.Writer) error {
		bw := bufio.NewWriter(w)
		for _, x := range v {
			fmt.Fprintln(bw, strconv.FormatInt(id(x), 10))
		}
		return bw.Flush()
	})
}

// writeAtomic writes a file through a temporary file in the same directory,
// so that a failed run does not leave half a file.
func writeAtomic(path string, fn func(io.Writer) error) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp) // #nosec G304 -- the path is in the corpus or the work directory
	if err != nil {
		return err
	}
	if err := fn(f); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
