package build_test

import (
	"context"
	"fmt"
	"os"

	"github.com/tamnd/math2-agent/build"
)

// A tree without a lean/ directory has no lean, extract and replay steps.
func ExampleRun() {
	out, err := os.MkdirTemp("", "build")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(out) }()
	m, err := build.Run(context.Background(), build.Options{Root: "../testdata/nodes/valid", Out: out})
	if err != nil {
		panic(err)
	}
	for _, s := range m.Steps {
		fmt.Println(s.Name, s.Cached)
	}
	// Output:
	// status false
	// graph false
	// store false
	// search false
	// writing false
	// formal false
}
