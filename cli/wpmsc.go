package cli

import (
	"flag"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/tamnd/math2-agent/wpcorpus"
)

const wpMSCUsage = "m2 wp msc [-corpus DIR] [-page ID] [-dry-run]"

// runWPMSC gives each article an MSC code (WP7.3) from the categories, mapped by the msc rules of categories.yaml, the context phrase of the lead and, for an article with no code or a tie, the links of the lead. For an article that still has no code or a tie, the decision of the model pass in msc-model.yaml gives the code. It writes the codes in the msc field of the metadata files. An article with a tie and no decision keeps all the codes of the tie, and an article with no signal and no decision keeps no code.
func runWPMSC(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 wp msc", flag.ContinueOnError)
	fs.SetOutput(stderr)
	corpus := fs.String("corpus", ".", "the Wikipedia corpus (a clone of tamnd/wikipedia-math)")
	page := fs.Int("page", 0, "classify only the article with this page id")
	dry := fs.Bool("dry-run", false, "print the counts, and write nothing")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		fmt.Fprintln(stderr, "usage: "+wpMSCUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 wp msc: %v\n", err)
		return ExitFailure
	}
	started := time.Now().UTC()
	tree, err := wpcorpus.LoadCategoryTree(*corpus)
	if err != nil {
		return fail(err)
	}
	if len(tree.MSC) == 0 {
		return fail(fmt.Errorf("categories.yaml has no msc rules"))
	}
	model, err := wpcorpus.LoadMSCModel(*corpus)
	if err != nil {
		return fail(err)
	}
	mds, err := wpcorpus.ReadMetadataFiles(*corpus)
	if err != nil {
		return fail(err)
	}
	// The first pass reads the categories and the context phrase. The second
	// pass adds the links of the lead for the articles with no code or a tie,
	// with the codes of the first pass, so the order of the articles does not
	// change the result.
	type article struct {
		md    *wpcorpus.Metadata
		lead  string
		links []int
		res   wpcorpus.MSCResult
	}
	var as []*article
	first := map[int]string{}
	for _, md := range mds {
		lead, links, err := wpcorpus.ReadLead(*corpus, md.Page)
		if err != nil {
			return fail(err)
		}
		a := &article{md: md, lead: lead, links: links, res: wpcorpus.ClassifyMSC(tree, md, lead, nil)}
		if len(a.res.Codes) == 1 {
			first[md.Page] = a.res.Codes[0]
		}
		if *page == 0 || md.Page == *page {
			as = append(as, a)
		}
	}
	var articles, coded, both, catOnly, leadOnly, agree, fromLinks, fromModel, ties, none, written int
	classes := map[string]int{}
	for _, a := range as {
		md, res := a.md, a.res
		if len(res.Codes) != 1 {
			var linked []string
			for _, id := range a.links {
				if c := first[id]; c != "" && id != md.Page {
					linked = append(linked, c)
				}
			}
			res = wpcorpus.ClassifyMSC(tree, md, a.lead, linked)
		}
		d, decided := model[md.Page]
		decided = decided && len(res.Codes) != 1
		if decided {
			res.Codes = []string{d.Code}
		}
		articles++
		switch {
		case decided:
			fromModel++
		case len(res.Codes) == 0:
			none++
		case len(res.Links) > 0:
			fromLinks++
		case len(res.Categories) > 0 && res.Lead != "":
			both++
			if res.Categories[res.Lead] > 0 {
				agree++
			}
		case res.Lead != "":
			leadOnly++
		default:
			catOnly++
		}
		if len(res.Codes) > 0 {
			coded++
		}
		if len(res.Codes) > 1 {
			ties++
		}
		for _, c := range res.Codes {
			classes[c[:2]]++
		}
		if slices.Equal(res.Codes, md.MSC) || *dry {
			continue
		}
		md.MSC = res.Codes
		if err := wpcorpus.WriteMetadata(*corpus, md); err != nil {
			return fail(err)
		}
		written++
	}
	keys := make([]string, 0, len(classes))
	for k := range classes {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		fmt.Fprintf(stdout, "%s %d\n", k, classes[k])
	}
	fmt.Fprintf(stdout, "%d articles, %d with a code, %d from the categories, %d from the lead, %d from both (%d agree), %d with the links of the lead, %d from the model, %d with a tie, %d with no signal, %d written\n",
		articles, coded, catOnly, leadOnly, both, agree, fromLinks, fromModel, ties, none, written)
	if *dry {
		return ExitOK
	}
	rec := &wpcorpus.RunRecord{Command: "m2 wp msc", Args: args, M2: version, Started: started.Format(time.RFC3339),
		Finished: time.Now().UTC().Format(time.RFC3339), Articles: articles, Pages: written}
	if _, err := wpcorpus.WriteRunRecord(*corpus, rec); err != nil {
		return fail(err)
	}
	return ExitOK
}
