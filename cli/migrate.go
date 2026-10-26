package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/tamnd/math2-agent/tag"
)

const migrateUsage = "m2 migrate shard3 [-root DIR] [-q]"

func runMigrate(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "shard3" {
		fmt.Fprintln(stderr, "usage: "+migrateUsage)
		return ExitUsage
	}
	fs := flag.NewFlagSet("m2 migrate shard3", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	quiet := fs.Bool("q", false, "print only the count of the moves")
	if err := fs.Parse(args[1:]); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+migrateUsage)
		return ExitUsage
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
	fmt.Fprintf(stdout, "%d paths moved\n", len(moves))
	return ExitOK
}
