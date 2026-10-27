package cli

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/tamnd/math2-agent/migrate"
	"github.com/tamnd/math2-agent/schema"
	"github.com/tamnd/math2-agent/tag"
)

const migrateUsage = "m2 migrate shard3|" + schema.Version + " [-root DIR] [-q]"

func runMigrate(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "shard3" && args[0] != schema.Version {
		fmt.Fprintln(stderr, "usage: "+migrateUsage)
		return ExitUsage
	}
	what := args[0]
	fs := flag.NewFlagSet("m2 migrate "+what, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	quiet := fs.Bool("q", false, "print only the count of the changes")
	if err := fs.Parse(args[1:]); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+migrateUsage)
		return ExitUsage
	}
	if what == schema.Version {
		return migrateSchema(*root, *quiet, stdout, stderr)
	}
	moves, err := tag.MoveShards(*root)
	if !*quiet {
		for _, m := range moves {
			fmt.Fprintf(stdout, "%s %s\n", m.From, m.To)
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "m2 migrate shard3: %v\n", err)
		return ExitFailure
	}
	if len(moves) == 1 {
		fmt.Fprintln(stdout, "1 path moved")
	} else {
		fmt.Fprintf(stdout, "%d paths moved\n", len(moves))
	}
	return ExitOK
}

// migrateSchema moves each node of the tree at root to schema.Version, and prints each node that changed with the count of its new provenance lines.
func migrateSchema(root string, quiet bool, stdout, stderr io.Writer) int {
	changes, err := migrate.Tree(root, time.Now().UTC().Format(time.RFC3339))
	if !quiet {
		for _, c := range changes {
			fmt.Fprintf(stdout, "%s %d provenance lines\n", c.Path, c.Lines)
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "m2 migrate %s: %v\n", schema.Version, err)
		return ExitFailure
	}
	if len(changes) == 1 {
		fmt.Fprintf(stdout, "1 node moved to %s\n", schema.Version)
	} else {
		fmt.Fprintf(stdout, "%d nodes moved to %s\n", len(changes), schema.Version)
	}
	return ExitOK
}
