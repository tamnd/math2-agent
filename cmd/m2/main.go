// Command m2 is the Math 2.0 toolchain. It builds, checks, searches and serves the math2 knowledge graph.
package main

import (
	"os"

	"github.com/tamnd/math2-agent/cli"

	// The ingest adapters register themselves with m2 ingest.
	_ "github.com/tamnd/math2-agent/ingest/arxiv"
	_ "github.com/tamnd/math2-agent/ingest/erdosproblems"
	_ "github.com/tamnd/math2-agent/ingest/formalconjectures"
	_ "github.com/tamnd/math2-agent/ingest/lmfdb"
	_ "github.com/tamnd/math2-agent/ingest/mathlib"
	_ "github.com/tamnd/math2-agent/ingest/oeis"
	_ "github.com/tamnd/math2-agent/ingest/wikidata"
	_ "github.com/tamnd/math2-agent/ingest/zbmath"
)

// Version is set at build time with -ldflags "-X main.Version=v0.1.0".
var Version = "dev"

func main() {
	os.Exit(cli.Run(os.Args[1:], Version, os.Stdout, os.Stderr))
}
