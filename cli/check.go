package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/schema"
)

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	only := fs.String("rule", "", "the rules to run, separated by commas. Empty means all the rules.")
	asJSON := fs.Bool("json", false, "write the report as JSON")
	quiet := fs.Bool("q", false, "do not write the table of rule states")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	var c check.Checker
	if *only != "" {
		for r := range strings.SplitSeq(*only, ",") {
			r = strings.ToUpper(strings.TrimSpace(r))
			if !slices.ContainsFunc(check.Rules(), func(x check.Rule) bool { return x.ID == r }) {
				fmt.Fprintf(stderr, "m2 check: %q is not a rule of m2 check\n", r)
				return ExitUsage
			}
			c.Only = append(c.Only, r)
		}
	}
	var err error
	if c.Schema, err = schema.Load(*root); err != nil {
		fmt.Fprintf(stderr, "m2 check: %v\n", err)
		return ExitFailure
	}
	if c.Macros, err = check.LoadMacros(*root); err != nil {
		fmt.Fprintf(stderr, "m2 check: %v\n", err)
		return ExitFailure
	}
	var files []check.File
	var ix check.Index
	if fs.NArg() == 0 {
		files, err = check.ReadTree(*root)
	} else {
		files, err = readPaths(*root, fs.Args())
		// The other nodes of the tree resolve the links. A node that does not parse is not in the index, and its own check reports it.
		nodes, _ := node.Load(*root)
		ix = check.Index{}
		for _, n := range nodes {
			ix.Add(n)
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "m2 check: %v\n", err)
		return ExitFailure
	}
	rep := c.Check(files, ix)
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintf(stderr, "m2 check: %v\n", err)
			return ExitFailure
		}
	} else {
		for _, f := range rep.Findings {
			fmt.Fprintln(stdout, f)
		}
		if !*quiet {
			_ = rep.WriteSummary(stderr)
		}
	}
	if rep.Failed() {
		return ExitFailure
	}
	return ExitOK
}

// readPaths reads the node files at paths. A directory gives each .md file below it. The path of each file is relative to root when the file is in root.
func readPaths(root string, paths []string) ([]check.File, error) {
	var files []check.File
	add := func(path string) error {
		data, err := os.ReadFile(path) // #nosec G304 G122 -- the user names the path on the command line.
		if err != nil {
			return err
		}
		name := path
		if abs, err := filepath.Abs(path); err == nil {
			if absRoot, err := filepath.Abs(root); err == nil {
				if rel, err := filepath.Rel(absRoot, abs); err == nil && !strings.HasPrefix(rel, "..") {
					name = rel
				}
			}
		}
		files = append(files, check.File{Path: filepath.ToSlash(name), Data: data})
		return nil
	}
	for _, p := range paths {
		err := filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			switch {
			case err != nil:
				return err
			case d.IsDir():
				return nil
			case path != p && filepath.Ext(path) != ".md":
				return nil
			}
			return add(path)
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}
