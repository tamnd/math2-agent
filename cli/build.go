package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/build"
)

func runBuild(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	opt := build.Options{Version: version}
	fs.StringVar(&opt.Root, "root", ".", "the math2 working tree")
	fs.StringVar(&opt.Out, "out", "", "the output directory. Empty means build/ in the tree.")
	fs.StringVar(&opt.Cache, "cache", "", "the directory where M2Extract is built. Empty means m2 in the user cache directory.")
	fs.StringVar(&opt.Only, "only", "", "run only this step: "+strings.Join(build.Steps(), ", "))
	fs.BoolVar(&opt.NoSandbox, "no-sandbox", false, "run the checkers of the replay step without the bwrap sandbox, for example on macOS. Such a replay does not give E5.")
	asJSON := fs.Bool("json", false, "write the manifest as JSON")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "m2 build: the command has no arguments, and it got %q\n", fs.Args())
		return ExitUsage
	}
	if opt.Only != "" && !slices.Contains(build.Steps(), opt.Only) {
		fmt.Fprintf(stderr, "m2 build: %q is not a step. The steps are %s\n", opt.Only, strings.Join(build.Steps(), ", "))
		return ExitUsage
	}
	if !*asJSON {
		opt.Log = stdout
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	m, err := build.Run(ctx, opt)
	if err != nil {
		fmt.Fprintf(stderr, "m2 build: %v\n", err)
		return ExitFailure
	}
	if *asJSON {
		data, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "m2 build: %v\n", err)
			return ExitFailure
		}
		_, _ = fmt.Fprintf(stdout, "%s\n", data)
		return ExitOK
	}
	fmt.Fprintf(stdout, "%d nodes, %d edges, %d records, %d bindings\n", m.Counts.Nodes, m.Counts.Edges, m.Counts.Records, m.Counts.Bindings)
	if m.Counts.Rows > 0 {
		fmt.Fprintf(stdout, "%d inventory rows, %d open\n", m.Counts.Rows, m.Counts.Open)
	}
	fmt.Fprintf(stdout, "output %s\n", m.Output)
	return ExitOK
}
