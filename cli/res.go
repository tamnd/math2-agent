package cli

import (
	"flag"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/tamnd/math2-agent/resource"
)

const resUsage = "m2 res mint [-root DIR] [-commit C]\n       " + resCensusUsage + "\n       " + resSampleUsage + "\n       " + resLookupUsage + "\n       " + resClusterUsage + "\n       " + resResolveUsage + "\n       " + resScopeUsage + "\n       " + resImportUsage + "\n       " + resShowUsage + "\n       " + resQueueUsage + "\n       " + resStatusUsage + "\n       " + resGoldUsage + "\n       " + resRefsUsage + "\n       " + resRecordsUsage + "\n       " + resCompetitionsUsage + "\n       " + resNotesUsage + "\n       " + resInstitutionsUsage + "\n       m2 res gaps"

// resLater holds the res subcommands that are not written yet, with the milestone of the resources roadmap (RS15) that delivers each of them.
var resLater = map[string]string{
	"gaps": "RS-M4, languages, history and recall",
}

func runRes(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: "+resUsage)
		return ExitUsage
	}
	switch sub := args[0]; {
	case sub == "mint":
		return runResMint(args[1:], stdout, stderr)
	case sub == "census":
		return runResCensus(args[1:], stdout, stderr)
	case sub == "sample":
		return runResSample(args[1:], stdout, stderr)
	case sub == "lookup":
		return runResLookup(args[1:], stdout, stderr)
	case sub == "scope":
		return runResScope(args[1:], stdout, stderr)
	case sub == "resolve":
		return runResResolve(args[1:], stdout, stderr)
	case sub == "import":
		return runResImport(args[1:], stdout, stderr)
	case sub == "show":
		return runResShow(args[1:], stdout, stderr)
	case sub == "queue":
		return runResQueue(args[1:], stdout, stderr)
	case sub == "cluster":
		return runResCluster(args[1:], stdout, stderr)
	case sub == "refs":
		return runResRefs(args[1:], stdout, stderr)
	case sub == "status":
		return runResStatus(args[1:], stdout, stderr)
	case sub == "records":
		return runResRecords(args[1:], stdout, stderr)
	case sub == "competitions":
		return runResCompetitions(args[1:], stdout, stderr)
	case sub == "notes":
		return runResNotes(args[1:], stdout, stderr)
	case sub == "institutions":
		return runResInstitutions(args[1:], stdout, stderr)
	case sub == "gold":
		return runResGold(args[1:], stdout, stderr)
	case resLater[sub] != "":
		fmt.Fprintf(stderr, "m2 res %s: this command is not written yet. It arrives in %s.\n", sub, resLater[sub])
		return ExitNotWritten
	default:
		fmt.Fprintf(stderr, "m2 res: unknown subcommand %q\nusage: %s\n", sub, resUsage)
		return ExitUsage
	}
}

func runResMint(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res mint", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	commit := fs.String("commit", "", "the commit that goes into registers/resources/. Empty means the HEAD commit of the working tree.")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: m2 res mint [-root DIR] [-commit C]")
		return ExitUsage
	}
	if *commit == "" {
		out, err := exec.Command("git", "-C", *root, "rev-parse", "HEAD").Output() // #nosec G204 -- the user gives the root, and git gets it as one argument.
		if err != nil {
			fmt.Fprintln(stderr, "m2 res mint: the working tree has no HEAD commit. Give the commit with -commit.")
			return ExitFailure
		}
		*commit = strings.TrimSpace(string(out))
	}
	if strings.ContainsAny(*commit, " \t\n") {
		fmt.Fprintf(stderr, "m2 res mint: the commit %q has a space\n", *commit)
		return ExitUsage
	}
	minted, err := resource.Mint(*root, nil, *commit)
	if err != nil {
		fmt.Fprintf(stderr, "m2 res mint: %v\n", err)
		return ExitFailure
	}
	for _, m := range minted {
		fmt.Fprintf(stdout, "%s %s %s\n", m.Provisional, m.ID, m.Kind)
	}
	return ExitOK
}
