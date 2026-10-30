package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/queue"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	rescluster "github.com/tamnd/math2-agent/resource/cluster"
	resresolve "github.com/tamnd/math2-agent/resource/resolve"
)

const resResolveUsage = "m2 res resolve [-root DIR] [-live] [-gap D] [-index] [-cite TEXT | -in FILE | ID ...]"

// ResResolveQueue is the file of the queue res-resolve in the tree.
const ResResolveQueue = "build/resources/queues/res-resolve.md"

func runResResolve(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res resolve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	live := fs.Bool("live", false, "ask the live APIs for the ids that the catalogue and the crosswalks do not have")
	gap := fs.Duration("gap", time.Second, "the shortest time between two requests to a live API")
	index := fs.Bool("index", false, "write the index of all ids to "+resresolve.IndexFile)
	cite := fs.String("cite", "", "one reference as text")
	in := fs.String("in", "", "a file with one reference on each line, as text or as a JSON citation. The candidates go to the queue res-resolve.")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	inputs := 0
	for _, set := range []bool{*cite != "", *in != "", fs.NArg() > 0} {
		if set {
			inputs++
		}
	}
	if inputs > 1 || (inputs == 0 && !*index) {
		fmt.Fprintln(stderr, "usage: "+resResolveUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 res resolve: %v\n", err)
		return ExitFailure
	}
	rs, err := resource.ReadResources(*root)
	if err != nil {
		return fail(err)
	}
	if *index {
		n, err := resresolve.WriteIndex(*root, rs)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(stderr, "wrote %d ids to %s\n", n, resresolve.IndexFile)
		if inputs == 0 {
			return ExitOK
		}
	}
	links, err := rescluster.ReadCrosswalks(*root)
	if err != nil {
		return fail(err)
	}
	r := &resresolve.Resolver{Index: resresolve.Build(rs), Crosswalks: resresolve.Links(links)}
	cachePath := filepath.Join(*root, resresolve.CacheFile)
	if *live {
		r.Cache, err = resresolve.ReadCache(cachePath, time.Now().UTC().Format(time.DateOnly))
		if err != nil {
			return fail(err)
		}
		r.Fetchers = []resresolve.Fetcher{resresolve.Crossref{Base: resresolve.CrossrefBase, Client: fetch.NewClient(*gap)}}
	}
	ctx := context.Background()
	enc := json.NewEncoder(stdout)
	var items bytes.Buffer
	counts := map[string]int{}
	emit := func(res resresolve.Result) error {
		counts[res.Status]++
		if res.Status == resresolve.Candidates && *in != "" {
			line, err := json.Marshal(queue.Item{Target: res.Query, Kind: res.Pass, Goal: "the reference has one resource or is not found", Priority: candidateScore(res)})
			if err != nil {
				return err
			}
			items.Write(append(line, '\n'))
		}
		return enc.Encode(res) // #nosec G117 -- the field pass names a pass of the resolver, not a secret.
	}
	switch {
	case *cite != "":
		res, err := r.Cite(ctx, resresolve.Parse(*cite))
		if err != nil {
			return fail(err)
		}
		if err := emit(res); err != nil {
			return fail(err)
		}
	case *in != "":
		f, err := os.Open(*in) // #nosec G304 -- the user names the file.
		if err != nil {
			return fail(err)
		}
		defer func() { _ = f.Close() }()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for n := 1; sc.Scan(); n++ {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			c := resresolve.Parse(line)
			if strings.HasPrefix(line, "{") {
				c = resresolve.Citation{}
				if err := json.Unmarshal([]byte(line), &c); err != nil {
					return fail(fmt.Errorf("%s:%d: %w", *in, n, err))
				}
			}
			res, err := r.Cite(ctx, c)
			if err != nil {
				return fail(err)
			}
			if err := emit(res); err != nil {
				return fail(err)
			}
		}
		if err := sc.Err(); err != nil {
			return fail(err)
		}
		if err := writeOut(filepath.Join(*root, ResResolveQueue), items.Bytes()); err != nil {
			return fail(err)
		}
	default:
		for _, id := range fs.Args() {
			res, err := r.ID(ctx, id)
			if err != nil {
				return fail(err)
			}
			if err := emit(res); err != nil {
				return fail(err)
			}
		}
	}
	if err := r.Cache.Write(cachePath); err != nil {
		return fail(err)
	}
	fmt.Fprintf(stderr, "%d found, %d with candidates, %d not found\n", counts[resresolve.Found], counts[resresolve.Candidates], counts[resresolve.NotFound])
	return ExitOK
}

func candidateScore(res resresolve.Result) float64 {
	if len(res.Candidates) == 0 {
		return 0
	}
	return res.Candidates[0].Score
}
