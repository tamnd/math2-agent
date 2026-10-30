// Command m2 is the Math 2.0 toolchain. It builds, checks, searches and serves the math2 knowledge graph.
package main

import (
	"os"

	"github.com/tamnd/math2-agent/cli"

	// The ingest adapters register themselves with m2 ingest.
	_ "github.com/tamnd/math2-agent/ingest/afp"
	_ "github.com/tamnd/math2-agent/ingest/arxiv"
	_ "github.com/tamnd/math2-agent/ingest/arxivresults"
	_ "github.com/tamnd/math2-agent/ingest/curricula"
	_ "github.com/tamnd/math2-agent/ingest/erdosproblems"
	_ "github.com/tamnd/math2-agent/ingest/formalconjectures"
	_ "github.com/tamnd/math2-agent/ingest/lmfdb"
	_ "github.com/tamnd/math2-agent/ingest/mathcomp"
	_ "github.com/tamnd/math2-agent/ingest/mathlib"
	_ "github.com/tamnd/math2-agent/ingest/nlab"
	_ "github.com/tamnd/math2-agent/ingest/oeis"
	_ "github.com/tamnd/math2-agent/ingest/palomar"
	_ "github.com/tamnd/math2-agent/ingest/planetmath"
	_ "github.com/tamnd/math2-agent/ingest/proofwiki"
	_ "github.com/tamnd/math2-agent/ingest/setmm"
	_ "github.com/tamnd/math2-agent/ingest/stacks"
	_ "github.com/tamnd/math2-agent/ingest/textbook"
	_ "github.com/tamnd/math2-agent/ingest/wikidata"
	_ "github.com/tamnd/math2-agent/ingest/wikipedia"
	_ "github.com/tamnd/math2-agent/ingest/zbmath"

	// The adapters of the catalogues of resources register themselves with m2 ingest and m2 res import.
	_ "github.com/tamnd/math2-agent/resource/adapters/crossref"
	_ "github.com/tamnd/math2-agent/resource/adapters/datacite"
	_ "github.com/tamnd/math2-agent/resource/adapters/doaj"
	_ "github.com/tamnd/math2-agent/resource/adapters/gutenberg"
	_ "github.com/tamnd/math2-agent/resource/adapters/hal"
	_ "github.com/tamnd/math2-agent/resource/adapters/numdam"
	_ "github.com/tamnd/math2-agent/resource/adapters/openalex"
	_ "github.com/tamnd/math2-agent/resource/adapters/openlibrary"
	_ "github.com/tamnd/math2-agent/resource/adapters/wikidataworks"
)

// Version is set at build time with -ldflags "-X main.Version=v0.1.0".
var Version = "dev"

func main() {
	os.Exit(cli.Run(os.Args[1:], Version, os.Stdout, os.Stderr))
}
