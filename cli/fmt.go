package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/tamnd/math2-agent/format"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/schema"
)

func runFmt(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 fmt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	checkOnly := fs.Bool("check", false, "change no file. Write the path of each file that is not canonical, and exit with code 1 if there is one.")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	s, err := schema.Load(*root)
	if err != nil {
		fmt.Fprintf(stderr, "m2 fmt: %v\n", err)
		return ExitFailure
	}
	// A node that does not parse gives no aliases. Its error comes when m2 fmt formats it.
	nodes, _ := node.Load(*root)
	aliases, err := format.ReadAliases(*root, nodes)
	if err != nil {
		fmt.Fprintf(stderr, "m2 fmt: %v\n", err)
		return ExitFailure
	}
	paths, err := fmtPaths(*root, fs.Args())
	if err != nil {
		fmt.Fprintf(stderr, "m2 fmt: %v\n", err)
		return ExitFailure
	}
	f := format.Formatter{Schema: s, Aliases: aliases}
	code := ExitOK
	for _, path := range paths {
		data, err := os.ReadFile(path) // #nosec G304 G122 -- the path is in the working tree or on the command line.
		if err != nil {
			fmt.Fprintf(stderr, "m2 fmt: %v\n", err)
			code = ExitFailure
			continue
		}
		out, err := f.Source(treePath(*root, path), data)
		switch {
		case err != nil:
			fmt.Fprintf(stderr, "m2 fmt: %v\n", err)
			code = ExitFailure
			continue
		case bytes.Equal(out, data):
			continue
		case *checkOnly:
			code = ExitFailure
		default:
			if err := os.WriteFile(path, out, 0o644); err != nil { // #nosec G306 G703 -- the user names the path, and content files are public text in a git tree.
				fmt.Fprintf(stderr, "m2 fmt: %v\n", err)
				code = ExitFailure
				continue
			}
		}
		fmt.Fprintln(stdout, treePath(*root, path))
	}
	return code
}

// fmtPaths returns the node files that m2 fmt formats. With no paths, it gives each node file of the tree. A tree with no nodes directory has no node files.
func fmtPaths(root string, paths []string) ([]string, error) {
	if len(paths) > 0 {
		return nodeFiles(paths)
	}
	dir := filepath.Join(root, "nodes")
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return nodeFiles([]string{dir})
}
