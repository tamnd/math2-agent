package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/queue"
	"github.com/tamnd/math2-agent/resource"
	rescluster "github.com/tamnd/math2-agent/resource/cluster"
	resscope "github.com/tamnd/math2-agent/resource/scope"
)

const resClusterUsage = "m2 res cluster [-root DIR] [-day YYYY-MM-DD] [-commit C] [-no-mint] [-spill DIR] [-bucket N]"

// ResClusterQueue is the file of the queue res-cluster in the tree.
const ResClusterQueue = "build/resources/queues/res-cluster.md"

func runResCluster(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res cluster", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	day := fs.String("day", time.Now().UTC().Format(time.DateOnly), "the date of the run, for the history")
	commit := fs.String("commit", "", "the commit that goes into registers/resources/. Empty means the HEAD commit of the working tree.")
	noMint := fs.Bool("no-mint", false, "keep the provisional ids of the new resources")
	spill := fs.String("spill", "", "run the passes on one bucket of records at a time and keep the spill files in this directory (RS10.8). Empty means one run in memory.")
	bucket := fs.Int("bucket", rescluster.BucketSize, "the records of one bucket, with -spill")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+resClusterUsage)
		return ExitUsage
	}
	if _, err := time.Parse(time.DateOnly, *day); err != nil {
		fmt.Fprintf(stderr, "m2 res cluster: the day %q is not YYYY-MM-DD\n", *day)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 res cluster: %v\n", err)
		return ExitFailure
	}
	links, err := rescluster.ReadCrosswalks(*root)
	if err != nil {
		return fail(err)
	}
	existing, err := resource.ReadResources(*root)
	if err != nil {
		return fail(err)
	}
	reg, err := resource.ReadRegister(*root)
	if err != nil {
		return fail(err)
	}
	order := map[string]int{}
	for i, l := range reg {
		order[l.ID] = i
	}
	ds, err := resscope.ReadDecisions(*root)
	if err != nil {
		return fail(err)
	}
	var decisions []rescluster.Decision
	for _, d := range ds {
		if d.Kind == "merge" || d.Kind == "split" {
			decisions = append(decisions, rescluster.Decision{Kind: d.Kind, A: d.ID, B: d.Target})
		}
	}
	in := rescluster.Input{Crosswalks: links, Existing: existing, Order: order, Decisions: decisions, By: "m2 " + version + " " + rescluster.Version, Day: *day}
	var (
		records, live, skipped int
		pairs                  []rescluster.Pair
		joins                  map[string]int
	)
	if *spill != "" {
		files, err := rescluster.RecordFiles(*root)
		if err != nil {
			return fail(err)
		}
		st, err := rescluster.RunFiles(in, files, *spill, *bucket, *root)
		if err != nil {
			return fail(err)
		}
		records, live, skipped, pairs, joins = st.Records, st.Resources, st.Skipped, st.Pairs, st.Joins
		fmt.Fprintf(stdout, "%d buckets, %d components, the largest with %d records\n", st.Partition.Buckets, st.Partition.Components, st.Partition.Largest)
	} else {
		recs, err := rescluster.ReadRecords(*root)
		if err != nil {
			return fail(err)
		}
		in.Records = recs
		res, err := rescluster.Run(in)
		if err != nil {
			return fail(err)
		}
		if err := resource.WriteResources(*root, res.Resources); err != nil {
			return fail(err)
		}
		records, skipped, pairs, joins = len(recs), res.Skipped, res.Pairs, res.Joins
		for i := range res.Resources {
			if res.Resources[i].Live() {
				live++
			}
		}
	}
	var buf bytes.Buffer
	for _, p := range pairs {
		goal := "the pair " + p.A + " and " + p.B + " is one resource or two"
		if p.Pass != rescluster.C4 {
			goal = "a person confirms that " + p.A + " and " + p.B + " are two resources"
		}
		line, err := json.Marshal(queue.Item{Target: p.A + " " + p.B, Kind: p.Pass, Name: p.Reason, Goal: goal, Priority: p.Score})
		if err != nil {
			return fail(err)
		}
		buf.Write(append(line, '\n'))
	}
	if err := writeOut(filepath.Join(*root, ResClusterQueue), buf.Bytes()); err != nil {
		return fail(err)
	}
	minted := 0
	if !*noMint {
		if *commit == "" {
			out, err := exec.Command("git", "-C", *root, "rev-parse", "HEAD").Output() // #nosec G204 -- the user gives the root, and git gets it as one argument.
			if err != nil {
				fmt.Fprintln(stderr, "m2 res cluster: the working tree has no HEAD commit. Give the commit with -commit, or use -no-mint.")
				return ExitFailure
			}
			*commit = strings.TrimSpace(string(out))
		}
		ms, err := resource.Mint(*root, nil, *commit)
		if err != nil {
			return fail(err)
		}
		minted = len(ms)
	}
	fmt.Fprintf(stdout, "%d source records, %d resources, %d minted, %d pairs in the queue, %d blocks too large\n", records, live, minted, len(pairs), skipped)
	for _, p := range []string{rescluster.C1, rescluster.C2, rescluster.C3, rescluster.C4, rescluster.C5} {
		fmt.Fprintf(stdout, "%s: %d joins\n", p, joins[p])
	}
	return ExitOK
}
