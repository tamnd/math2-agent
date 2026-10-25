package cli

import (
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/bump"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/tag"
)

const provUsage = `m2 prov show [-root DIR] [-runs DIR] [-field F] [-json] <tag>
m2 prov record [-root DIR] [-base REV] [-commit C] [-at TIME] [-source S] [-n] -by PERSON | -agent ROLE@VERSION -run ID [-model M] [-prompt P]
m2 prov credit [-root DIR] [-runs DIR] [-from YYYY-MM-DD] [-to YYYY-MM-DD] [-json] <person>
m2 prov quarantine [-root DIR] [-runs DIR] -model M [-prompt P] [-by PERSON] [-json]
m2 prov confirm [-root DIR] -by PERSON <tag> <field>...
m2 prov disclose [-root DIR] [-runs DIR] [-out DIR] [-repo URL] [-license SPDX] [-yaml] <tag>`

func runProv(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "show":
			return runProvShow(args[1:], stdout, stderr)
		case "record":
			return runProvRecord(args[1:], stdout, stderr)
		case "credit":
			return runProvCredit(args[1:], stdout, stderr)
		case "quarantine":
			return runProvQuarantine(args[1:], stdout, stderr)
		case "confirm":
			return runProvConfirm(args[1:], stdout, stderr)
		case "disclose":
			return runProvDisclose(args[1:], stdout, stderr)
		}
	}
	fmt.Fprintln(stderr, "usage: "+provUsage)
	return ExitUsage
}

// provFlags makes the flag set of a prov command with the flag -root.
func provFlags(name string, stderr io.Writer) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet("m2 prov "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs, fs.String("root", ".", "the math2 working tree")
}

// parseInterleaved parses the flags in args, also the flags that come after an argument.
func parseInterleaved(fs *flag.FlagSet, args []string) error {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			return fs.Parse(pos)
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// readRuns reads the run summaries in dir, or in build/runs of the tree when dir is empty. A directory that does not exist has no runs.
func readRuns(root, dir string) (map[string]*prov.Run, error) {
	if dir == "" {
		dir = filepath.Join(root, "build", prov.RunsDir)
	}
	runs, err := prov.ReadRuns(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return runs, err
}

func runProvShow(args []string, stdout, stderr io.Writer) int {
	fs, root := provFlags("show", stderr)
	runsDir := fs.String("runs", "", "the run summaries. Empty means build/runs in the tree.")
	field := fs.String("field", "", "show only the lines on this field, on a part of it or on the whole node")
	asJSON := fs.Bool("json", false, "write the lines as JSON")
	if err := parseInterleaved(fs, args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 || !tag.IsTag(fs.Arg(0)) && !tag.IsProvisional(fs.Arg(0)) {
		fmt.Fprintln(stderr, "usage: m2 prov show [-root DIR] [-runs DIR] [-field F] [-json] <tag>")
		return ExitUsage
	}
	t := fs.Arg(0)
	lines, err := prov.Read(*root, t)
	if err != nil {
		fmt.Fprintf(stderr, "m2 prov show: %v\n", err)
		return ExitFailure
	}
	if *field != "" {
		lines = slices.DeleteFunc(lines, func(l prov.Line) bool { return !prov.Covers(l.Field, *field) })
	}
	if *asJSON {
		return writeJSON(stdout, stderr, "m2 prov show", lines)
	}
	if len(lines) == 0 {
		fmt.Fprintf(stdout, "%s has no provenance lines in %s\n", t, prov.Path(t))
		return ExitOK
	}
	runs, err := readRuns(*root, *runsDir)
	if err != nil {
		fmt.Fprintf(stderr, "m2 prov show: %v\n", err)
		return ExitFailure
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	for _, l := range lines {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", l.At, l.Op, l.Field, who(l, runs[l.Run]))
	}
	_ = tw.Flush()
	q := prov.Quarantined(lines)
	for _, f := range slices.Sorted(maps.Keys(q)) {
		fmt.Fprintf(stdout, "%s is in quarantine since %s. A person must change it or confirm it.\n", f, q[f].At)
	}
	return ExitOK
}

// who says who made the change of the line.
func who(l prov.Line, r *prov.Run) string {
	var parts []string
	if l.Agent != "" {
		s := l.Agent
		var with []string
		if l.Model != "" {
			with = append(with, l.Model)
		}
		if l.Prompt != "" {
			with = append(with, "prompt "+l.Prompt)
		}
		if l.Run != "" {
			with = append(with, "run "+l.Run)
		}
		if r != nil && r.OnBehalfOf != "" {
			with = append(with, "for "+r.OnBehalfOf)
		}
		if len(with) > 0 {
			s += " (" + strings.Join(with, ", ") + ")"
		}
		parts = append(parts, s)
	}
	if l.Human != "" {
		parts = append(parts, l.Human)
	}
	s := strings.Join(parts, " and ")
	if l.Source != "" {
		s += " from " + l.Source
	}
	if l.Commit != "" {
		s += " in " + l.Commit
	}
	return s
}

func runProvRecord(args []string, stdout, stderr io.Writer) int {
	fs, root := provFlags("record", stderr)
	base := fs.String("base", "HEAD", "the revision that the change starts from")
	var who prov.Line
	fs.StringVar(&who.Human, "by", "", "the person who made the change: orcid: and an ORCID iD, or github: and a GitHub login")
	fs.StringVar(&who.Agent, "agent", "", "the agent role at its version that made the change, for example m2-agent/formalize@0.9.1")
	fs.StringVar(&who.Run, "run", "", "the id of the agent run")
	fs.StringVar(&who.Model, "model", "", "the model of the agent")
	fs.StringVar(&who.Prompt, "prompt", "", "the prompt version of the agent")
	fs.StringVar(&who.Source, "source", "", "the external source of the new values, for example arxiv:2401.01234")
	fs.StringVar(&who.Commit, "commit", "", "the commit of the change. Empty means that the line names no commit.")
	fs.StringVar(&who.At, "at", "", "the time of the change, RFC 3339. Empty means now.")
	dry := fs.Bool("n", false, "show the lines and do not write them")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 || who.Human == "" && who.Agent == "" || strings.HasPrefix(*base, "-") {
		fmt.Fprintln(stderr, "usage: m2 prov record [-root DIR] [-base REV] [-commit C] [-at TIME] [-source S] [-n] -by PERSON | -agent ROLE@VERSION -run ID [-model M] [-prompt P]")
		return ExitUsage
	}
	if who.At == "" {
		who.At = time.Now().UTC().Format(time.RFC3339)
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 prov record: %v\n", err)
		return ExitFailure
	}
	if err := (prov.Line{Field: prov.Whole, Op: prov.Add, Agent: who.Agent, Run: who.Run, Human: who.Human, At: who.At}).Validate(); err != nil {
		return fail(err)
	}
	changed, err := changedNodes(*root, *base)
	if err != nil {
		return fail(err)
	}
	total := 0
	for _, p := range changed {
		id := strings.TrimSuffix(filepath.Base(p), ".md")
		before := nodeAt(*root, *base, p)
		after := nodeAt(*root, "", p)
		lines := prov.Changes(before, after, who)
		// A field that has a new line since the base is recorded already.
		old, err := blobAt(*root, *base, prov.Path(id))
		if err != nil {
			return fail(err)
		}
		was, _ := prov.Parse(strings.NewReader(old))
		now, err := prov.Read(*root, id)
		if err != nil {
			return fail(err)
		}
		if len(now) >= len(was) {
			added := now[len(was):]
			lines = slices.DeleteFunc(lines, func(c prov.Line) bool {
				return slices.ContainsFunc(added, func(l prov.Line) bool { return prov.Covers(l.Field, c.Field) })
			})
		}
		for _, l := range lines {
			fmt.Fprintf(stdout, "%s %s %s\n", id, l.Op, l.Field)
		}
		total += len(lines)
		if !*dry {
			if err := prov.Append(*root, id, lines...); err != nil {
				return fail(err)
			}
		}
	}
	if total == 0 {
		fmt.Fprintf(stdout, "no field changed since %s has no line\n", *base)
	}
	return ExitOK
}

// changedNodes returns the node files that differ between the revision base and the working tree, with the untracked node files.
func changedNodes(root, base string) ([]string, error) {
	diff, err := git(root, "diff", "--name-only", "--no-renames", "-z", base, "--", "nodes")
	if err != nil {
		return nil, err
	}
	untracked, err := git(root, "ls-files", "--others", "--exclude-standard", "-z", "--", "nodes")
	if err != nil {
		return nil, err
	}
	var out []string
	for p := range strings.SplitSeq(diff+untracked, "\x00") {
		if strings.HasSuffix(p, ".md") && strings.Count(p, "/") == 2 {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// blobAt returns the file at the path in the revision rev, or "" when the revision does not have it.
func blobAt(root, rev, path string) (string, error) {
	if _, err := git(root, "cat-file", "-e", rev+":"+path); err != nil {
		return "", nil
	}
	return git(root, "show", rev+":"+path)
}

// nodeAt returns the node file at the path in the revision rev, or in the working tree when rev is empty. It returns nil when the file is not there or does not parse.
func nodeAt(root, rev, path string) *node.Node {
	var data []byte
	if rev == "" {
		data, _ = os.ReadFile(filepath.Join(root, filepath.FromSlash(path))) // #nosec G304 -- the path is a node file that git listed in the tree.
	} else if s, err := blobAt(root, rev, path); err == nil && s != "" {
		data = []byte(s)
	}
	if data == nil {
		return nil
	}
	n, err := node.Parse(path, data)
	if err != nil {
		return nil
	}
	return n
}

func runProvCredit(args []string, stdout, stderr io.Writer) int {
	fs, root := provFlags("credit", stderr)
	runsDir := fs.String("runs", "", "the run summaries. Empty means build/runs in the tree.")
	fromS := fs.String("from", "", "the first day, YYYY-MM-DD. Empty means no bound.")
	toS := fs.String("to", "", "the last day, YYYY-MM-DD. Empty means no bound.")
	asJSON := fs.Bool("json", false, "write the contributions as JSON")
	if err := parseInterleaved(fs, args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: m2 prov credit [-root DIR] [-runs DIR] [-from YYYY-MM-DD] [-to YYYY-MM-DD] [-json] <person>")
		return ExitUsage
	}
	person := fs.Arg(0)
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 prov credit: %v\n", err)
		return ExitFailure
	}
	if !prov.IsResponsible(person) {
		return fail(fmt.Errorf("%q is not a person, a project or a schedule", person))
	}
	var from, to time.Time
	var err error
	if *fromS != "" {
		if from, err = time.Parse(time.DateOnly, *fromS); err != nil {
			return fail(fmt.Errorf("the date %q is not YYYY-MM-DD", *fromS))
		}
	}
	if *toS != "" {
		if to, err = time.Parse(time.DateOnly, *toS); err != nil {
			return fail(fmt.Errorf("the date %q is not YYYY-MM-DD", *toS))
		}
		to = to.AddDate(0, 0, 1)
	}
	logs, err := prov.ReadAll(*root)
	if err != nil {
		return fail(err)
	}
	runs, err := readRuns(*root, *runsDir)
	if err != nil {
		return fail(err)
	}
	cs := prov.Credit(logs, runs, person, from, to)
	if *asJSON {
		return writeJSON(stdout, stderr, "m2 prov credit", cs)
	}
	period := "in all of the history of math2"
	switch {
	case *fromS != "" && *toS != "":
		period = "from " + *fromS + " to " + *toS
	case *fromS != "":
		period = "from " + *fromS
	case *toS != "":
		period = "up to " + *toS
	}
	fmt.Fprintf(stdout, "Contributions of %s to math2 %s, in the CRediT roles.\n", person, period)
	if len(cs) == 0 {
		fmt.Fprintln(stdout, "\nThe provenance has no contributions.")
		return ExitOK
	}
	for i := 0; i < len(cs); {
		j := i
		nodes := map[string]bool{}
		for j < len(cs) && cs[j].Role == cs[i].Role {
			nodes[cs[j].Tag] = true
			j++
		}
		fmt.Fprintf(stdout, "\n%s: %d changes to %d nodes\n", cs[i].Role, j-i, len(nodes))
		for _, c := range cs[i:j] {
			via := ""
			if c.Run != "" {
				via = ", by an agent in the run " + c.Run
			}
			fmt.Fprintf(stdout, "  m2://%s  %s %s on %s%s\n", c.Tag, c.Op, c.Field, day(c.At), via)
		}
		i = j
	}
	return ExitOK
}

// day returns the date of an RFC 3339 time.
func day(at string) string {
	d, _, _ := strings.Cut(at, "T")
	return d
}

func runProvQuarantine(args []string, stdout, stderr io.Writer) int {
	fs, root := provFlags("quarantine", stderr)
	runsDir := fs.String("runs", "", "the run summaries. Empty means build/runs in the tree.")
	model := fs.String("model", "", "the model id")
	prompt := fs.String("prompt", "", "the prompt version. Empty means each prompt version.")
	by := fs.String("by", "", "the person who quarantines the fields. Empty means that the command only lists them.")
	asJSON := fs.Bool("json", false, "write the fields as JSON")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 || *model == "" {
		fmt.Fprintln(stderr, "usage: m2 prov quarantine [-root DIR] [-runs DIR] -model M [-prompt P] [-by PERSON] [-json]")
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 prov quarantine: %v\n", err)
		return ExitFailure
	}
	logs, err := prov.ReadAll(*root)
	if err != nil {
		return fail(err)
	}
	runs, err := readRuns(*root, *runsDir)
	if err != nil {
		return fail(err)
	}
	hits := prov.Produced(logs, runs, *model, *prompt)
	if *asJSON {
		return writeJSON(stdout, stderr, "m2 prov quarantine", hits)
	}
	for _, h := range hits {
		fmt.Fprintf(stdout, "%s %s %s run %s %s\n", h.Tag, h.Line.Field, h.Line.Agent, h.Line.Run, h.Line.At)
	}
	what := *model
	if *prompt != "" {
		what += " with the prompt " + *prompt
	}
	fmt.Fprintf(stdout, "%d fields are from %s, and no person changed or confirmed them\n", len(hits), what)
	if *by == "" || len(hits) == 0 {
		return ExitOK
	}
	at := time.Now().UTC().Format(time.RFC3339)
	for _, h := range hits {
		l := prov.Line{Field: h.Line.Field, Op: prov.Quarantine, Run: h.Line.Run, Human: *by, At: at}
		if err := prov.Append(*root, h.Tag, l); err != nil {
			return fail(err)
		}
	}
	fmt.Fprintf(stdout, "The provenance logs put the %d fields in quarantine. Commit the logs and open a pull request. A person clears a field when they change it or run m2 prov confirm.\n", len(hits))
	return ExitOK
}

func runProvConfirm(args []string, stdout, stderr io.Writer) int {
	fs, root := provFlags("confirm", stderr)
	by := fs.String("by", "", "the person who read the fields and approves them")
	if err := parseInterleaved(fs, args); err != nil {
		return ExitUsage
	}
	if fs.NArg() < 2 || *by == "" {
		fmt.Fprintln(stderr, "usage: m2 prov confirm [-root DIR] -by PERSON <tag> <field>...")
		return ExitUsage
	}
	t, fields := fs.Arg(0), fs.Args()[1:]
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 prov confirm: %v\n", err)
		return ExitFailure
	}
	if !tag.IsTag(t) && !tag.IsProvisional(t) {
		return fail(fmt.Errorf("%q is not a tag", t))
	}
	log, err := prov.Read(*root, t)
	if err != nil {
		return fail(err)
	}
	at := time.Now().UTC().Format(time.RFC3339)
	var lines []prov.Line
	for _, f := range fields {
		l, ok := prov.Author(log, f)
		if !ok || l.Agent == "" || l.Human != "" {
			return fail(fmt.Errorf("the field %s of %s has no text of an agent that a person must confirm", f, t))
		}
		lines = append(lines, prov.Line{Field: f, Op: prov.Confirm, Run: l.Run, Human: *by, At: at})
	}
	if err := prov.Append(*root, t, lines...); err != nil {
		return fail(err)
	}
	for _, l := range lines {
		fmt.Fprintf(stdout, "%s confirm %s\n", t, l.Field)
	}
	return ExitOK
}

// writeJSON writes v as indented JSON.
func writeJSON(stdout, stderr io.Writer, cmd string, v any) int {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", cmd, err)
		return ExitFailure
	}
	return ExitOK
}

func runProvDisclose(args []string, stdout, stderr io.Writer) int {
	flags, root := provFlags("disclose", stderr)
	runsDir := flags.String("runs", "", "the run summaries. Empty means build/runs in the tree.")
	out := flags.String("out", "", "the output directory of m2 build. Empty means build/ in the tree.")
	repo := flags.String("repo", "https://github.com/tamnd/math2", "the URL of the content repository")
	license := flags.String("license", "Apache-2.0", "the SPDX licence of the formal code")
	asYAML := flags.Bool("yaml", false, "write the formalization.yaml of the node, and not the paragraph")
	if err := parseInterleaved(flags, args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 1 || !tag.IsTag(flags.Arg(0)) {
		fmt.Fprintln(stderr, "usage: m2 prov disclose [-root DIR] [-runs DIR] [-out DIR] [-repo URL] [-license SPDX] [-yaml] <tag>")
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 prov disclose: %v\n", err)
		return ExitFailure
	}
	t := flags.Arg(0)
	path := "nodes/" + tag.Shard(t) + "/" + t + ".md"
	data, err := os.ReadFile(filepath.Join(*root, filepath.FromSlash(path))) // #nosec G304 -- the tag is checked above, thus the path is in the tree.
	if err != nil {
		return fail(err)
	}
	n, err := node.Parse(path, data)
	if err != nil {
		return fail(err)
	}
	f := prov.Facts{Node: n, Repo: *repo, License: *license}
	if f.Log, err = prov.Read(*root, t); err != nil {
		return fail(err)
	}
	if f.Runs, err = readRuns(*root, *runsDir); err != nil {
		return fail(err)
	}
	if *out == "" {
		*out = filepath.Join(*root, "build")
	}
	all, err := build.ReadStatus(*out)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return fail(err)
	default:
		f.Build = map[string]prov.Binding{}
		if i := slices.IndexFunc(all, func(s status.Status) bool { return s.Tag == t }); i >= 0 {
			for _, b := range all[i].Bindings {
				pb := prov.Binding{Level: cmp.Or(b.Level.String(), "None"), Axioms: b.Axioms, Fidelity: b.Fidelity.String()}
				for _, r := range b.Reviews {
					if r.State == status.Counts && r.Reviewer != "" {
						pb.Reviewers = append(pb.Reviewers, r.Reviewer)
					}
				}
				f.Build[b.Decl] = pb
			}
		}
	}
	if pins, err := bump.ReadPins(filepath.Join(*root, "lean")); err == nil {
		f.Mathlib = pins.Commit
	}
	if c, err := git(*root, "rev-parse", "HEAD"); err == nil {
		f.Commit = strings.TrimSpace(c)
	}
	if !*asYAML {
		fmt.Fprint(stdout, prov.Disclose(f))
		return ExitOK
	}
	y, err := prov.Formalization(f)
	if err != nil {
		return fail(err)
	}
	_, _ = stdout.Write(y)
	return ExitOK
}
