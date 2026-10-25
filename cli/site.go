package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/tamnd/math2-agent/site"
)

func runSite(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "build" {
		fmt.Fprintln(stderr, "Usage: m2 site build [-content DIR] [-spec DIR] [-out DIR] [-url URL] [-commit C] [-date now]")
		return ExitUsage
	}
	fs := flag.NewFlagSet("m2 site build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	opt := site.Options{Version: version, Current: current}
	fs.StringVar(&opt.Content, "content", ".", "the math2 working tree")
	fs.StringVar(&opt.Spec, "spec", "", "the spec directory of math2-agent. Leave it empty for no spec pages.")
	fs.StringVar(&opt.Out, "out", "_site", "the output directory. The build removes it first.")
	fs.StringVar(&opt.URL, "url", "https://tamnd.github.io/math2", "the public URL of the site")
	fs.StringVar(&opt.Repo, "repo", "tamnd/math2", "the content repository on GitHub")
	fs.StringVar(&opt.AgentRepo, "agent-repo", "tamnd/math2-agent", "the toolchain repository on GitHub")
	fs.StringVar(&opt.Commit, "commit", "", "the content commit")
	date := fs.String("date", "", "the build date: now, or empty for no date. SOURCE_DATE_EPOCH also sets it.")
	if err := fs.Parse(args[1:]); err != nil {
		return ExitUsage
	}
	switch {
	case *date == "now":
		opt.Date = time.Now()
	case *date != "":
		fmt.Fprintf(stderr, "m2 site build: -date must be now or empty, not %q\n", *date)
		return ExitUsage
	case os.Getenv("SOURCE_DATE_EPOCH") != "":
		sec, err := strconv.ParseInt(os.Getenv("SOURCE_DATE_EPOCH"), 10, 64)
		if err != nil {
			fmt.Fprintf(stderr, "m2 site build: SOURCE_DATE_EPOCH: %v\n", err)
			return ExitUsage
		}
		opt.Date = time.Unix(sec, 0)
	}
	for _, m := range plan {
		opt.Milestones = append(opt.Milestones, site.Milestone{
			ID:    m.ID,
			Title: m.Title,
			Dates: "due " + m.Due,
			URL:   fmt.Sprintf("https://github.com/%s/milestone/%d", opt.AgentRepo, m.Number),
		})
	}
	if err := site.Build(opt); err != nil {
		fmt.Fprintf(stderr, "m2 site build: %v\n", err)
		return ExitFailure
	}
	fmt.Fprintf(stdout, "m2 site build: wrote %s\n", opt.Out)
	return ExitOK
}
