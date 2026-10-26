package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/tamnd/math2-agent/agent"
	agentreport "github.com/tamnd/math2-agent/agent/report"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prov"
)

const agentReportUsage = "m2 agent report [-root DIR] [-store DIR] [-dir DIR] [-week YYYY-Www] [-json]"

// reportNow is the clock of the weekly report. The tests replace it.
var reportNow = time.Now

// runAgentReport writes the weekly agent report of a week (spec 12.9).
func runAgentReport(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("m2 agent report", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree")
	store := flags.String("store", "", "the directory of the run records. Empty means build/ in the tree.")
	dir := flags.String("dir", "", "the directory of the reports. Empty means reports/agents/ in the tree.")
	week := flags.String("week", "", "the ISO week of the report, for example 2026-W43. Empty means the last full week.")
	asJSON := flags.Bool("json", false, "write the report as JSON to the standard output, and not as Markdown")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "Usage: %s\n", agentReportUsage)
		return ExitUsage
	}
	now := reportNow().UTC()
	w := agentreport.WeekOf(now).Prev()
	if *week != "" {
		var err error
		if w, err = agentreport.ParseWeek(*week); err != nil {
			fmt.Fprintf(stderr, "m2 agent report: %v\n", err)
			return ExitUsage
		}
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 agent report: %v\n", err)
		return ExitFailure
	}
	if *store == "" {
		*store = filepath.Join(*root, "build")
	}
	if *dir == "" {
		*dir = filepath.Join(*root, "reports", "agents")
	}
	in := agentreport.Input{Areas: map[string]string{}, Made: now}
	var err error
	if in.Records, err = agent.ReadRecords(*store); err != nil {
		return fail(err)
	}
	if in.Logs, err = prov.ReadAll(*root); err != nil {
		return fail(err)
	}
	nodes, err := node.Load(*root)
	if err != nil {
		return fail(err)
	}
	for _, n := range nodes {
		if msc := n.List("msc"); len(msc) > 0 && len(msc[0]) >= 2 {
			in.Areas[n.ID] = msc[0][:2]
		}
	}
	prev, err := os.ReadFile(filepath.Join(*dir, w.Prev().String()+".json")) // #nosec G304 -- the directory of the reports, which the caller names.
	switch {
	case err == nil:
		in.Prev = &agentreport.Report{}
		if err := json.Unmarshal(prev, in.Prev); err != nil {
			return fail(fmt.Errorf("the report of %s: %w", w.Prev(), err))
		}
	case !errors.Is(err, fs.ErrNotExist):
		return fail(err)
	}

	rep := agentreport.Make(w, in)
	js, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return fail(err)
	}
	js = append(js, '\n')
	md := rep.Markdown()
	if err := os.MkdirAll(*dir, 0o755); err != nil { // #nosec G301 -- the reports are public.
		return fail(err)
	}
	base := filepath.Join(*dir, w.String())
	if err := os.WriteFile(base+".md", md, 0o644); err != nil { // #nosec G306 -- the reports are public text in a git tree.
		return fail(err)
	}
	if err := os.WriteFile(base+".json", js, 0o644); err != nil { // #nosec G306 -- the reports are public text in a git tree.
		return fail(err)
	}
	if *asJSON {
		_, _ = stdout.Write(js)
	} else {
		_, _ = stdout.Write(md)
	}
	return ExitOK
}
