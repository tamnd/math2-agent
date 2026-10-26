package cli

import (
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/card"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/project"
	"github.com/tamnd/math2-agent/schema"
	"github.com/tamnd/math2-agent/style"
)

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	only := fs.String("rule", "", "the rules to run, separated by commas. Empty means all the rules.")
	asJSON := fs.Bool("json", false, "write the report as JSON")
	quiet := fs.Bool("q", false, "do not write the table of rule states")
	withStyle := fs.Bool("style", false, "also run the rules Y01 to Y04 of the house style (corpus K8.2)")
	levels := fs.String("levels", "", "the writing.json of a build, for -style. A style finding in a node at W3 is an error. Empty means build/writing.json in the tree, when it exists.")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	known := check.Rules()
	if *withStyle {
		known = append(known, style.Rules()...)
	}
	var c check.Checker
	if *only != "" {
		for r := range strings.SplitSeq(*only, ",") {
			r = strings.ToUpper(strings.TrimSpace(r))
			if !slices.ContainsFunc(known, func(x check.Rule) bool { return x.ID == r }) {
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
	if *withStyle {
		if err := addStyle(rep, files, *root, *levels, c.Only); err != nil {
			fmt.Fprintf(stderr, "m2 check: %v\n", err)
			return ExitFailure
		}
	}
	if fs.NArg() == 0 && *only == "" {
		if err := checkProjects(*root, files); err != nil {
			fmt.Fprintf(stderr, "m2 check: %v\n", err)
			return ExitFailure
		}
	}
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

// addStyle runs the rules of the house style on the files that parse, and adds the results and the findings to rep. A node is strict when the levels file gives it W3.
func addStyle(rep *check.Report, files []check.File, root, levels string, only []string) error {
	path := levels
	if path == "" {
		path = filepath.Join(root, "build", build.WritingFile)
	}
	w3 := map[string]bool{}
	data, err := os.ReadFile(path) // #nosec G304 -- the path is a flag or in the tree.
	switch {
	case err == nil:
		var l build.Levels
		if err := json.Unmarshal(data, &l); err != nil {
			return fmt.Errorf("%s: %v", path, err)
		}
		for _, r := range l.Nodes {
			if r.Level == card.W3 {
				w3[r.Tag] = true
			}
		}
	case levels != "" || !errors.Is(err, fs.ErrNotExist):
		return err
	}
	var nodes []*node.Node
	for _, f := range files {
		if n, err := node.Parse(f.Path, f.Data); err == nil {
			nodes = append(nodes, n)
		}
	}
	results, findings := style.Lint(nodes, func(tag string) bool { return w3[tag] })
	for _, res := range results {
		if len(only) > 0 && !slices.Contains(only, res.Rule) {
			res = check.Result{Rule: res.Rule, State: check.Skip, Reason: "not selected"}
		} else {
			rep.Findings = append(rep.Findings, slices.DeleteFunc(slices.Clone(findings), func(f check.Finding) bool { return f.Rule != res.Rule })...)
		}
		rep.Results = append(rep.Results, res)
	}
	slices.SortStableFunc(rep.Findings, func(a, b check.Finding) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Col, b.Col), strings.Compare(a.Rule, b.Rule))
	})
	return nil
}

// checkProjects reads the projects of the tree, and reports the first project that is not valid or whose topic is not a topic node of the files.
func checkProjects(root string, files []check.File) error {
	ps, err := project.Read(root)
	if err != nil || len(ps) == 0 {
		return err
	}
	topics := map[string]bool{}
	for _, f := range files {
		if n, err := node.Parse(f.Path, f.Data); err == nil && n.Kind() == "topic" {
			topics[n.ID] = true
		}
	}
	for _, p := range ps {
		if !topics[p.Topic] {
			return fmt.Errorf("project %s: the topic %s is not a topic node of the tree", p.ID, p.Topic)
		}
	}
	return nil
}

// readPaths reads the node files at paths. A directory gives each .md file below it. The path of each file is relative to root when the file is in root.
func readPaths(root string, paths []string) ([]check.File, error) {
	disk, err := nodeFiles(paths)
	if err != nil {
		return nil, err
	}
	files := make([]check.File, 0, len(disk))
	for _, path := range disk {
		data, err := os.ReadFile(path) // #nosec G304 -- the user names the path on the command line.
		if err != nil {
			return nil, err
		}
		files = append(files, check.File{Path: treePath(root, path), Data: data})
	}
	return files, nil
}

// nodeFiles returns the files at paths. A directory gives each .md file below it, in path order. A file gives itself, also when it is not a .md file.
func nodeFiles(paths []string) ([]string, error) {
	var out []string
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
			out = append(out, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// treePath returns the path of a file relative to root, with forward slashes, when the file is in root. Otherwise it returns the path with forward slashes.
func treePath(root, path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		if absRoot, err := filepath.Abs(root); err == nil {
			if rel, err := filepath.Rel(absRoot, abs); err == nil && !strings.HasPrefix(rel, "..") {
				path = rel
			}
		}
	}
	return filepath.ToSlash(path)
}
