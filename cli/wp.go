package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/wpcorpus"
)

const wpUsage = "m2 wp extract [-corpus DIR] [-list FILE] [-page ID] [-type T] [-api URL] [title...]\n       m2 wp scope -store DB -dump DATE -assess FILE [-cited FILE] [-wikidata FILE] [-corpus DIR] [-work DIR]\n       m2 wp gold draw|measure [s|y|m] [-corpus DIR] [-seed N]\n       m2 wp fetch -dump FILE -store DB [-index FILE] [-date DATE] [-corpus DIR] [-workers N] [-check=false] [-page ID] [-limit N]\n       m2 wp parsoid [-corpus DIR] [-page ID] [-limit N]\n       m2 wp pointers -store DB [-dump FILE] [-index FILE] [-workers N] [-corpus DIR]\n       m2 wp census report|diff [-corpus DIR] [PAGE]\n       m2 wp refresh"

// wpLater holds the wp subcommands that are not written yet, with the
// milestone of the Wikipedia roadmap (WP14) that delivers each of them.
var wpLater = map[string]string{
	"refresh": "WP-M7, all of the scope",
}

// wpDelay is the pause between two requests to the Action API.
var wpDelay = 200 * time.Millisecond

func runWP(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: "+wpUsage)
		return ExitUsage
	}
	switch sub := args[0]; {
	case sub == "extract":
		return runWPExtract(args[1:], stdout, stderr)
	case sub == "scope":
		return runWPScope(args[1:], stdout, stderr)
	case sub == "fetch":
		return runWPFetch(args[1:], stdout, stderr)
	case sub == "parsoid":
		return runWPParsoid(args[1:], stdout, stderr)
	case sub == "pointers":
		return runWPPointers(args[1:], stdout, stderr)
	case sub == "census":
		return runWPCensus(args[1:], stdout, stderr)
	case sub == "gold":
		return runWPGold(args[1:], stdout, stderr)
	case wpLater[sub] != "":
		fmt.Fprintf(stderr, "m2 wp %s: this command is not written yet. It arrives in %s.\n", sub, wpLater[sub])
		return ExitNotWritten
	default:
		fmt.Fprintf(stderr, "m2 wp: unknown subcommand %q\nusage: %s\n", sub, wpUsage)
		return ExitUsage
	}
}

func runWPExtract(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 wp extract", flag.ContinueOnError)
	fs.SetOutput(stderr)
	corpus := fs.String("corpus", ".", "the Wikipedia corpus (a clone of tamnd/wikipedia-math)")
	list := fs.String("list", "", "a file with one article title on each line. The articles are fetched like the title arguments.")
	page := fs.Int("page", 0, "extract only the page with this id from the content plane")
	typ := fs.String("type", "", "extract only the pages of this article type: article, statement, glossary, list, outline or timeline")
	api := fs.String("api", wpcorpus.DefaultAPI, "the Action API URL")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	titles := fs.Args()
	if *list != "" {
		more, err := readTitles(*list)
		if err != nil {
			fmt.Fprintf(stderr, "m2 wp extract: %v\n", err)
			return ExitFailure
		}
		titles = append(titles, more...)
	}
	if len(titles) > 0 && *page != 0 {
		fmt.Fprintln(stderr, "m2 wp extract: give titles or -page, not both")
		return ExitUsage
	}
	m, err := wpcorpus.Load(*corpus)
	if err != nil {
		fmt.Fprintf(stderr, "m2 wp extract: %v\n", err)
		return ExitFailure
	}

	var pages []*wpcorpus.Page
	failed := 0
	if len(titles) > 0 {
		f := &wpcorpus.Fetcher{API: *api, UserAgent: userAgent()}
		for i, t := range titles {
			if i > 0 {
				time.Sleep(wpDelay)
			}
			p, err := f.Fetch(context.Background(), t)
			if err == nil {
				err = wpcorpus.WriteContent(*corpus, p)
			}
			if err != nil {
				fmt.Fprintf(stderr, "m2 wp extract: %v\n", err)
				failed++
				continue
			}
			pages = append(pages, p)
		}
	} else {
		ids := []int{*page}
		if *page == 0 {
			if ids, err = wpcorpus.Pages(*corpus); err != nil {
				fmt.Fprintf(stderr, "m2 wp extract: %v\n", err)
				return ExitFailure
			}
		}
		for _, id := range ids {
			p, err := wpcorpus.ReadContent(*corpus, id)
			if err != nil {
				fmt.Fprintf(stderr, "m2 wp extract: %v\n", err)
				failed++
				continue
			}
			pages = append(pages, p)
		}
	}

	resolve, err := wpcorpus.ScopeResolver(*corpus)
	if err != nil {
		fmt.Fprintf(stderr, "m2 wp extract: %v\n", err)
		return ExitFailure
	}
	var objects, cues, gaps, done int
	for _, p := range pages {
		if *typ != "" && wpcorpus.ArticleType(p.Title) != *typ {
			continue
		}
		var r *wpcorpus.Result
		in, err := wpcorpus.ReadPointers(*corpus, p.ID)
		if err == nil {
			r, err = wpcorpus.ExtractPage(p, m, "m2/"+version, in)
		}
		if err == nil {
			err = wpcorpus.JoinWitness(*corpus, r, m)
		}
		if err == nil {
			err = wpcorpus.WriteResult(*corpus, r)
		}
		if err == nil {
			err = wpcorpus.RemoveStale(*corpus, p.ID, p.Revision)
		}
		if err == nil {
			var md []byte
			if md, err = wpcorpus.Render(p, m, r, resolve, "m2/"+version); err == nil {
				err = wpcorpus.WriteMarkdown(*corpus, p.ID, md)
			}
		}
		if err != nil {
			fmt.Fprintf(stderr, "m2 wp extract: %s: %v\n", p.Title, err)
			failed++
			continue
		}
		c := &r.Census
		g := len(c.Gaps())
		fmt.Fprintf(stdout, "%d r%d %s: %d objects, %d cues, %d gaps, %d unmapped templates, %d unmapped headings\n",
			p.ID, p.Revision, p.Title, len(r.Objects), len(c.Cues), g, len(c.Unmapped), len(c.UnmappedHeadings))
		objects += len(r.Objects)
		cues += len(c.Cues)
		gaps += g
		done++
	}
	fmt.Fprintf(stdout, "%d pages, %d objects, %d cues, %d gaps, %d failed\n", done, objects, cues, gaps, failed)
	if failed > 0 {
		return ExitFailure
	}
	return ExitOK
}

// readTitles reads a list file: one title on each line. Empty lines and
// lines that start with # are skipped.
func readTitles(path string) ([]string, error) {
	f, err := os.Open(path) // #nosec G304 -- the user names the list file
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New(path + ": the list has no titles")
	}
	return out, nil
}
