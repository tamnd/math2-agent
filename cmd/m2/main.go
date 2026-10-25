// Command m2 is the Math 2.0 toolchain. It builds, checks, searches and serves the math2 knowledge graph.
package main

import (
	"os"

	"github.com/tamnd/math2-agent/cli"
)

// Version is set at build time with -ldflags "-X main.Version=v0.1.0".
var Version = "dev"

func main() {
	os.Exit(cli.Run(os.Args[1:], Version, os.Stdout, os.Stderr))
}
