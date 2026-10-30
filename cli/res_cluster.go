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
)

const resClusterUsage = "m2 res cluster [-root DIR] [-day YYYY-MM-DD] [-commit C] [-no-mint]"

// ResClusterQueue is the file of the queue res-cluster in the tree.
const ResClusterQueue = "build/resources/queues/res-cluster.jsonl"

func runResCluster(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res cluster", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	day := fs.String("day", time.Now().UTC().Format(time.DateOnly), "the date of the run, for the history")
	commit := fs.String("commit", "", "the commit that goes into registers/resources/. Empty means the HEAD commit of the working tree.")
	noMint := fs.Bool("no-mint", false, "keep the provisional ids of the new resources")
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
	recs, err := rescluster.ReadRecords(*root)
	if err != nil {
		return fail(err)
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
	res, err := rescluster.Run(rescluster.Input{Records: recs, Crosswalks: links, Existing: existing, Order: order, By: "m2 " + version + " " + rescluster.Version, Day: *day})
	if err != nil {
		return fail(err)
	}
	if err := resource.WriteResources(*root, res.Resources); err != nil {
		return fail(err)
	}
	var buf bytes.Buffer
	for _, p := range res.Pairs {
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
	live := 0
	for i := range res.Resources {
		if res.Resources[i].Live() {
			live++
		}
	}
	fmt.Fprintf(stdout, "%d source records, %d resources, %d minted, %d pairs in the queue, %d blocks too large\n", len(recs), live, minted, len(res.Pairs), res.Skipped)
	for _, p := range []string{rescluster.C1, rescluster.C2, rescluster.C3, rescluster.C4, rescluster.C5} {
		fmt.Fprintf(stdout, "%s: %d joins\n", p, res.Joins[p])
	}
	return ExitOK
}
