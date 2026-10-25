package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/format"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/tag"
	"github.com/tamnd/math2-agent/uri"
)

func runStatus(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("m2 status", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree")
	out := flags.String("out", "", "the output directory of m2 build. Empty means build/ in the tree.")
	asJSON := flags.Bool("json", false, "write the status as JSON")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "m2 status: give one tag, alias or m2:// URI")
		return ExitUsage
	}
	if *out == "" {
		*out = filepath.Join(*root, "build")
	}
	all, err := build.ReadStatus(*out)
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(stderr, "m2 status: %s does not exist. Run m2 build first.\n", filepath.Join(*out, build.StatusFile))
		return ExitFailure
	}
	if err != nil {
		fmt.Fprintf(stderr, "m2 status: %v\n", err)
		return ExitFailure
	}
	found, err := resolve(*root, all, flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "m2 status: %v\n", err)
		return ExitFailure
	}
	if *asJSON {
		var v any = found
		if len(found) == 1 {
			v = found[0]
		}
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "m2 status: %v\n", err)
			return ExitFailure
		}
		_, _ = fmt.Fprintf(stdout, "%s\n", data)
		return ExitOK
	}
	m, _ := build.ReadManifest(*out)
	for i, s := range found {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		printStatus(stdout, s, m)
	}
	return ExitOK
}

// resolve finds the statuses that ref names. ref is a tag, a provisional id, an alias or an m2:// URI. Only the alias m2://lean/DECL can name more than one node.
func resolve(root string, all []status.Status, ref string) ([]status.Status, error) {
	byTag := func(t string) ([]status.Status, error) {
		i := slices.IndexFunc(all, func(s status.Status) bool { return s.Tag == t })
		if i < 0 {
			return nil, fmt.Errorf("no node has the tag %s in the last build. Check the tag, or run m2 build again", t)
		}
		return all[i : i+1], nil
	}
	byAlias := func(name string) ([]status.Status, error) {
		nodes, err := node.Load(root)
		if err != nil {
			return nil, err
		}
		aliases, err := format.ReadAliases(root, nodes)
		if err != nil {
			return nil, err
		}
		t, ok := aliases[name]
		if !ok {
			return nil, fmt.Errorf("no node has the alias %s", name)
		}
		return byTag(t)
	}
	switch {
	case strings.HasPrefix(ref, uri.Scheme):
		u, err := uri.Parse(ref)
		if err != nil {
			return nil, err
		}
		if u.Release != "" || u.Commit != "" {
			return nil, fmt.Errorf("%s names a release or a commit. m2 status shows only the last build of the working tree", ref)
		}
		switch {
		case !u.IsAlias():
			return byTag(u.Tag)
		case u.Namespace == "lean":
			var out []status.Status
			for _, s := range all {
				if slices.ContainsFunc(s.Bindings, func(b status.Binding) bool { return b.Decl == u.Name || b.Adapter == u.Name }) {
					out = append(out, s)
				}
			}
			if len(out) == 0 {
				return nil, fmt.Errorf("no node binds the Lean declaration %s", u.Name)
			}
			return out, nil
		case slices.Contains(uri.Kinds, u.Namespace):
			var found []status.Status
			var err error
			if tag.IsTag(u.Name) {
				found, err = byTag(u.Name)
			} else {
				found, err = byAlias(u.Name)
			}
			if err != nil {
				return nil, err
			}
			if found[0].Kind != u.Namespace {
				return nil, fmt.Errorf("%s names %s, and %s is a %s, not a %s", ref, found[0].Tag, found[0].Tag, found[0].Kind, u.Namespace)
			}
			return found, nil
		}
		return nil, fmt.Errorf("m2 status cannot resolve an alias in the namespace %s. Give a tag, an alias or a URI in the namespace lean or of a kind", u.Namespace)
	case tag.IsTag(ref) || tag.IsProvisional(ref):
		return byTag(ref)
	case tag.IsAlias(ref):
		return byAlias(ref)
	}
	return nil, fmt.Errorf("%q is not a tag, an alias or an m2:// URI", ref)
}

func printStatus(w io.Writer, s status.Status, m *build.Manifest) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	defer func() { _ = tw.Flush() }()
	fmt.Fprintf(tw, "%s\t%s: %s\n", s.Tag, s.Kind, s.Name)
	if s.Retired {
		fmt.Fprintf(tw, "retired\tyes\n")
	}
	level := "none"
	if s.Evidence != status.None {
		level = s.Evidence.String() + " (" + s.Evidence.Word() + ")"
	}
	fmt.Fprintf(tw, "evidence\t%s\n", level)
	fidelity := "unbound"
	if s.Fidelity != status.Unbound {
		fidelity = s.Fidelity.String()
	}
	fmt.Fprintf(tw, "fidelity\t%s\n", fidelity)
	fmt.Fprintf(tw, "flags\t%s\n", list(s.Flags))
	if s.StatementHash != "" {
		fmt.Fprintf(tw, "statement hash\t%s\n", s.StatementHash)
	}
	for _, it := range s.Items {
		detail := it.Detail
		if it.Kind == "resolution" || it.Kind == "refutation" {
			detail = strings.TrimSpace(detail + " at " + it.Level.String())
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", it.Kind, it.Ref, detail)
	}
	for _, b := range s.Bindings {
		head := b.Decl + ", match " + b.Match
		if b.Adapter != "" {
			head += ", through the adapter " + b.Adapter
		}
		fmt.Fprintf(tw, "binding\t%s\n", head)
		fmt.Fprintf(tw, "  type hash\t%s\n", or(b.TypeHash, "no record"))
		fmt.Fprintf(tw, "  axioms\t%s\n", list(b.Axioms))
		fmt.Fprintf(tw, "  fidelity\t%s\n", b.Fidelity)
		for _, r := range b.Reviews {
			fmt.Fprintf(tw, "  review\t%s %s %s\n", r.ID, or(r.Reviewer, "-"), r.State)
		}
		switch {
		case b.Replay == nil:
			fmt.Fprintf(tw, "  replay\tnot run\n")
		case b.Replay.Passed:
			fmt.Fprintf(tw, "  replay\tpassed\n")
		case b.Replay.Reason != "":
			fmt.Fprintf(tw, "  replay\tfailed: %s\n", b.Replay.Reason)
		default:
			fmt.Fprintf(tw, "  replay\tfailed\n")
		}
		if b.Level == status.Kernel || b.Level == status.Certified {
			checkers := map[string]string{}
			if b.Replay != nil && len(b.Replay.Checkers) > 0 {
				checkers = b.Replay.Checkers
			} else if m != nil {
				checkers = m.Checkers
			}
			for _, c := range slices.Sorted(maps.Keys(checkers)) {
				fmt.Fprintf(tw, "  checker\t%s %s\n", c, checkers[c])
			}
		}
	}
	for i, r := range s.Reasons {
		label := ""
		if i == 0 {
			label = "why"
		}
		fmt.Fprintf(tw, "%s\t%s\n", label, r)
	}
}

func list(s []string) string {
	if len(s) == 0 {
		return "none"
	}
	return strings.Join(s, ", ")
}

func or(s, alt string) string {
	if s == "" {
		return alt
	}
	return s
}
