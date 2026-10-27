package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/audit"
	"github.com/tamnd/math2-agent/release"
)

func runAudit(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 audit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	plane := fs.String("plane", "", "run only the rules of this plane: "+strings.Join(audit.Planes(), ", "))
	only := fs.String("rule", "", "the rules to run, separated by commas. Empty means all the rules.")
	changed := fs.Bool("changed", false, "audit only the shards and the files that changed since the merge base of -base and HEAD")
	base := fs.String("base", "origin/main", "the revision that the changes of -changed start from")
	rel := fs.String("release", "", "run the full audit before the release R, for example 2027.03, and write reports/releases/R.txt")
	asJSON := fs.Bool("json", false, "write the report as JSON")
	quiet := fs.Bool("q", false, "do not write the findings, only the table of rule states")
	arxiv := fs.String("arxiv", "", "a checkout of tamnd/arxiv, for rule D05. Empty means that rule D05 skips.")
	wikipedia := fs.String("wikipedia", "", "a checkout of tamnd/wikipedia-math, for the Y rules. Empty means that the Y rules skip.")
	group := fs.String("group", "", "run only the rules whose id starts with one of these letters, for example Y")
	hard := fs.Bool("hard", false, "run only the hard rules")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "m2 audit: the command has no arguments, and it got %q\n", fs.Args())
		return ExitUsage
	}
	opt := audit.Options{Plane: *plane, Group: strings.ToUpper(*group), Hard: *hard, Arxiv: *arxiv, Wikipedia: *wikipedia}
	for _, c := range opt.Group {
		if !slices.ContainsFunc(audit.Rules(), func(x audit.Rule) bool { return rune(x.ID[0]) == c }) {
			fmt.Fprintf(stderr, "m2 audit: no rule of the audit starts with the letter %q\n", c)
			return ExitUsage
		}
	}
	if *plane != "" && !slices.Contains(audit.Planes(), *plane) {
		fmt.Fprintf(stderr, "m2 audit: %q is not a plane. The planes are %s\n", *plane, strings.Join(audit.Planes(), ", "))
		return ExitUsage
	}
	if *only != "" {
		for r := range strings.SplitSeq(*only, ",") {
			r = strings.ToUpper(strings.TrimSpace(r))
			if !slices.ContainsFunc(audit.Rules(), func(x audit.Rule) bool { return x.ID == r }) {
				fmt.Fprintf(stderr, "m2 audit: %q is not a rule of the audit\n", r)
				return ExitUsage
			}
			opt.Rules = append(opt.Rules, r)
		}
	}
	if *changed {
		opt.Base = *base
	}
	if *rel != "" {
		if !release.IsName(*rel) {
			fmt.Fprintf(stderr, "m2 audit: %q is not a release. A release has the form YYYY.MM or YYYY.MM.N\n", *rel)
			return ExitUsage
		}
		if *changed || *plane != "" || *only != "" || *group != "" || *hard {
			fmt.Fprintln(stderr, "m2 audit: -release runs all the rules on the whole tree, and it cannot have -changed, -plane, -rule, -group or -hard")
			return ExitUsage
		}
	}
	rep, err := audit.Run(*root, opt)
	if err != nil {
		fmt.Fprintf(stderr, "m2 audit: %v\n", err)
		return ExitFailure
	}

	var text bytes.Buffer
	for _, f := range rep.Findings {
		fmt.Fprintln(&text, f)
	}
	_ = rep.WriteSummary(&text)
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "m2 audit: %v\n", err)
		return ExitFailure
	}
	data = append(data, '\n')
	reports := map[string][]byte{}
	if *rel != "" {
		reports[filepath.Join("reports", "releases", *rel+".txt")] = text.Bytes()
	} else {
		name := scope(opt)
		reports[filepath.Join("reports", "audit", name+".txt")] = text.Bytes()
		reports[filepath.Join("reports", "audit", name+".json")] = data
	}
	for rel, b := range reports {
		if err := writeReport(filepath.Join(*root, rel), b); err != nil {
			fmt.Fprintf(stderr, "m2 audit: %v\n", err)
			return ExitFailure
		}
	}

	switch {
	case *asJSON:
		_, _ = stdout.Write(data)
	case *quiet:
		_ = rep.WriteSummary(stdout)
	default:
		_, _ = stdout.Write(text.Bytes())
	}
	if rep.Failed() {
		return ExitFailure
	}
	return ExitOK
}

// scope returns the name of the report of a run: all for the full audit, or the plane, the rules and the word changed, with a hyphen between them.
func scope(opt audit.Options) string {
	var parts []string
	if opt.Plane != "" {
		parts = append(parts, opt.Plane)
	}
	if opt.Group != "" {
		parts = append(parts, opt.Group)
	}
	parts = append(parts, opt.Rules...)
	if opt.Hard {
		parts = append(parts, "hard")
	}
	if opt.Base != "" {
		parts = append(parts, "changed")
	}
	if len(parts) == 0 {
		return "all"
	}
	return strings.Join(parts, "-")
}

func writeReport(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- the reports are public.
		return err
	}
	return os.WriteFile(path, data, 0o644) // #nosec G306 -- the reports are public text in a git tree.
}
