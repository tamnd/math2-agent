// Package cli holds the m2 command table and runs one command.
//
// Each command that is not written yet names the milestone that delivers it. Such a command exits with ExitNotWritten. It does not exit zero, because a command that does nothing and exits zero looks like a command that worked.
package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Exit codes.
const (
	ExitOK         = 0
	ExitFailure    = 1
	ExitUsage      = 2
	ExitNotWritten = 3
)

// milestone is one milestone of the project plan.
type milestone struct {
	ID     string
	Title  string
	Number int    // the GitHub milestone number in tamnd/math2-agent
	Due    string // the month the milestone is due
}

// plan is the project plan, in order.
var plan = []milestone{
	{"M0", "the skeleton", 1, "December 2026"},
	{"M1", "the seed and the trust core", 2, "March 2027"},
	{"M2", "the graph meets the world", 3, "June 2027"},
	{"M3", "open to readers", 4, "September 2027"},
	{"M4", "fidelity at scale", 5, "December 2027"},
	{"M5", "the research engine", 6, "June 2028"},
}

type runFunc func(args []string, stdout, stderr io.Writer) int

type command struct {
	name      string
	usage     string
	summary   string
	milestone string  // the milestone that delivers the command, or "" when the command is written
	run       runFunc // nil when the command is not written yet
}

// commands is the command table. Keep it in the same order as the help output.
var commands = []command{
	{name: "new", usage: "m2 new [-root DIR] <kind>", summary: "Make a node file with a provisional id.", run: runNew},
	{name: "fmt", usage: "m2 fmt [-root DIR] [-check] [path...]", summary: "Make front matter canonical and change aliases to tags.", run: runFmt},
	{name: "check", usage: "m2 check [-root DIR] [-rule R,...] [-json] [-q] [path...]", summary: "Check schema, body and links on the working tree. This does not use Lean.", run: runCheck},
	{name: "mint", usage: "m2 mint [-root DIR] [-commit C] [-seed]", summary: "Change the provisional ids to tags before merge, and add the tags to registers/tags.reg.", run: runMint},
	{name: "migrate", usage: migrateUsage, summary: "Move the files of the tree to a new layout. shard3 moves each file of nodes/, prov/ and reviews/ to the shard of the first three characters of its tag.", run: runMigrate},
	{name: "site", usage: "m2 site build|check|serve|archive [flags]", summary: "Build, check, preview and archive the wiki: a static site for people and agents.", run: runSite},
	{name: "build", usage: "m2 build [-root DIR] [-out DIR] [-cache DIR] [-only STEP] [-no-sandbox] [-json]", summary: "Build the derived data: Lean, extraction, replay, battery, status, graph, store and search.", run: runBuild},
	{name: "audit", usage: "m2 audit [-root DIR] [-plane P] [-rule R,...] [-changed] [-base REV] [-release R] [-json] [-q]", summary: "Run the audit rules and write the report to reports/.", run: runAudit},
	{name: "status", usage: "m2 status [-root DIR] [-out DIR] [-json] <tag|alias|uri>", summary: "Show the computed status of a node and the evidence for it.", run: runStatus},
	{name: "graph", usage: graphUsage, summary: "Run a graph query.", run: runGraph},
	{name: "ingest", usage: ingestUsage, summary: "Get a pinned snapshot of an external source and write its records.", run: runIngest},
	{name: "corpus", usage: corpusUsage, summary: "Change the records of the sources to inventory mentions and rows, show the counts of the inventory, and show the standing queues and their plans.", run: runCorpus},
	{name: "align", usage: alignUsage, summary: "Propose crosswalk edges to an external source, and decide on them.", run: runAlign},
	{name: "bump", usage: bumpUsage, summary: "Make a branch that moves Lean and Mathlib to the next release.", run: runBump},
	{name: "coverage", usage: coverageUsage, summary: "Write the coverage report of each MSC2020 code to reports/coverage/.", run: runCoverage},
	{name: "release", usage: releaseUsage, summary: "Cut, describe or verify a release.", run: runRelease},
	{name: "search", usage: searchUsage, summary: "Search the knowledge graph, or rank the premises of a Lean goal.", run: runSearch},
	{name: "serve", usage: serveUsage, summary: "Serve the read API over HTTP with JSON, and the MCP server over HTTP or stdio.", run: runServe},
	{name: "compute", usage: "m2 compute run|verify [-root DIR] [-out DIR] [-runtime R] [-json] <tag>...|-all", summary: "Run a computation in the sandbox, or verify its output and its certificate.", run: runCompute},
	{name: "prov", usage: provUsage, summary: "Show, record, credit, quarantine and confirm provenance.", run: runProv},
	{name: "review", usage: reviewUsage, summary: "Show the review packet of a node, and write a fidelity review.", run: runReview},
	{name: "autoform", usage: "m2 autoform <tag>", summary: "Make a formal statement for a node.", milestone: "M4"},
	{name: "agent", usage: agentUsage + "\n       " + agentReportUsage + "\n       " + agentDatasetUsage, summary: "Do one agent run in a new work tree, write the weekly agent report, or write the dataset of the runs of a quarter.", run: runAgent},
	{name: "orchestrate", usage: orchestrateUsage, summary: "Run the jobs of the agent roles from a queue, with budgets and the escalation ladder.", run: runOrchestrate},
}

// version is the m2 version that Run got.
var version = "dev"

// current is the milestone that the project works on now.
const current = "M3"

// Run runs the command in args and returns the exit code.
func Run(args []string, v string, stdout, stderr io.Writer) int {
	version = v
	if len(args) == 0 {
		writeHelp(stderr)
		return ExitUsage
	}
	switch name := args[0]; name {
	case "help", "-h", "-help", "--help":
		if len(args) > 1 {
			return helpFor(args[1], stdout, stderr)
		}
		writeHelp(stdout)
		return ExitOK
	case "version", "-version", "--version":
		fmt.Fprintf(stdout, "m2 %s\n", version)
		return ExitOK
	default:
		c, ok := lookup(name)
		if !ok {
			fmt.Fprintf(stderr, "m2: unknown command %q. Run \"m2 help\" to see the commands.\n", name)
			return ExitUsage
		}
		if c.run == nil {
			m := goal(c.milestone)
			fmt.Fprintf(stderr, "m2 %s: this command is not written yet. It arrives in %s, %s.\n", c.name, m.ID, m.Title)
			return ExitNotWritten
		}
		return c.run(args[1:], stdout, stderr)
	}
}

func lookup(name string) (command, bool) {
	for _, c := range commands {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}

func goal(id string) milestone {
	for _, m := range plan {
		if m.ID == id {
			return m
		}
	}
	return milestone{ID: id, Title: "an unknown milestone"}
}

func helpFor(name string, stdout, stderr io.Writer) int {
	c, ok := lookup(name)
	if !ok {
		fmt.Fprintf(stderr, "m2: unknown command %q.\n", name)
		return ExitUsage
	}
	fmt.Fprintf(stdout, "Usage: %s\n\n%s\n", c.usage, c.summary)
	if c.run == nil {
		m := goal(c.milestone)
		fmt.Fprintf(stdout, "\nThis command is not written yet. It arrives in %s, %s.\n", m.ID, m.Title)
	}
	return ExitOK
}

func writeHelp(w io.Writer) {
	var b strings.Builder
	b.WriteString("m2 is the Math 2.0 toolchain.\n\nUsage: m2 <command> [arguments]\n\nCommands:\n")
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	for _, c := range commands {
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", c.name, c.summary, c.milestone)
	}
	fmt.Fprintf(tw, "  help\tShow this help, or the help for one command.\n")
	fmt.Fprintf(tw, "  version\tShow the m2 version.\n")
	_ = tw.Flush()
	b.WriteString("\nA command with a milestone is not written yet. It exits with code 3 and names the milestone that delivers it.\n")
	_, _ = io.WriteString(w, b.String())
}
