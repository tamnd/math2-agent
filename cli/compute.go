package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/tamnd/math2-agent/compute"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/sandbox"
)

const computeUsage = `Usage:
  m2 compute run [-root DIR] [-out DIR] [-runtime R] [-json] <tag>...|-all
  m2 compute verify [-root DIR] [-out DIR] [-runtime R] [-json] <tag>...|-all

run runs each computation in the sandbox and writes build/compute/TAG/ARCH/run.json.
verify also checks the output hash and the certificate against the record.
-all runs each computation of the tree.`

// newSandbox makes the sandbox of m2 compute. A test can change it.
var newSandbox = func(ctx context.Context, runtime string) (sandbox.Sandbox, error) {
	d := &sandbox.Docker{}
	if err := d.Detect(ctx); err != nil {
		return nil, fmt.Errorf("%w. Install Docker, or start its daemon", err)
	}
	if runtime != "" {
		d.Runtime = runtime
	}
	return d, nil
}

func runCompute(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "run" && args[0] != "verify") {
		fmt.Fprintln(stderr, computeUsage)
		return ExitUsage
	}
	verify := args[0] == "verify"
	flags := flag.NewFlagSet("m2 compute "+args[0], flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree")
	out := flags.String("out", "", "the output directory. Empty means build/compute in the tree.")
	runtime := flags.String("runtime", "", "the OCI runtime, for example runsc. Empty means runsc when the daemon has it.")
	all := flags.Bool("all", false, "run each computation of the tree")
	asJSON := flags.Bool("json", false, "write the runs as JSON")
	if err := flags.Parse(args[1:]); err != nil {
		return ExitUsage
	}
	if (flags.NArg() == 0) == !*all {
		fmt.Fprintln(stderr, "m2 compute: give one or more tags, or -all")
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 compute: %v\n", err)
		return ExitFailure
	}
	recs, err := computations(*root, flags.Args(), *all)
	if err != nil {
		return fail(err)
	}
	images, err := compute.ReadImages(*root)
	if errors.Is(err, fs.ErrNotExist) {
		return fail(fmt.Errorf("%s does not exist in %s", compute.ImagesFile, *root))
	}
	if err != nil {
		return fail(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	sb, err := newSandbox(ctx, *runtime)
	if err != nil {
		return fail(err)
	}
	r := &compute.Runner{Root: *root, Out: *out, Images: images, Sandbox: sb}
	code := ExitOK
	var runs []*compute.Run
	for _, rec := range recs {
		var run *compute.Run
		if verify {
			run, err = r.Verify(ctx, rec)
		} else {
			run, err = r.Run(ctx, rec)
		}
		if err != nil {
			fmt.Fprintf(stderr, "m2 compute: %v\n", err)
			code = ExitFailure
			continue
		}
		runs = append(runs, run)
		ok := run.ExitCode == 0 && !run.TimedOut
		if verify {
			ok = run.Verified
		}
		if !ok {
			code = ExitFailure
		}
		if !*asJSON {
			printRun(stdout, run, verify, filepath.Join(r.Dir(run.Tag), compute.RunFile))
		}
	}
	if *asJSON {
		data, err := json.MarshalIndent(runs, "", "  ")
		if err != nil {
			return fail(err)
		}
		_, _ = fmt.Fprintf(stdout, "%s\n", data)
	}
	return code
}

func printRun(w io.Writer, run *compute.Run, verify bool, file string) {
	state := fmt.Sprintf("exit code %d", run.ExitCode)
	switch {
	case run.TimedOut:
		state = "out of time"
	case verify && run.Verified:
		state = "verified"
	case verify:
		state = "not verified: " + run.Problem
	}
	fmt.Fprintf(w, "%s on %s with the runtime %s and no network: %s\n", run.Tag, run.Arch, run.Runtime, state)
	fmt.Fprintf(w, "  output %s in %.1fs\n", run.Output, run.Seconds)
	if run.Certificate != "" {
		fmt.Fprintf(w, "  certificate %s\n", run.Certificate)
	}
	if run.Check != nil {
		fmt.Fprintf(w, "  checker %s %s: exit code %d\n", run.Check.Image, run.Check.Entry, run.Check.ExitCode)
	}
	fmt.Fprintf(w, "  %s\n", file)
}

// computations returns the records of the computations with the tags, or of each computation of the tree.
func computations(root string, tags []string, all bool) ([]*compute.Record, error) {
	nodes, err := node.Load(root)
	if err != nil {
		return nil, err
	}
	byID := map[string]*node.Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	var out []*compute.Record
	if all {
		for _, n := range nodes {
			if n.Kind() == "computation" {
				tags = append(tags, n.ID)
			}
		}
	}
	for _, t := range tags {
		n := byID[t]
		if n == nil {
			return nil, fmt.Errorf("no node has the tag %s", t)
		}
		rec, err := compute.FromNode(n)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}
