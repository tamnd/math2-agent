package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/store"
)

const graphUsage = "m2 graph neighbourhood|closure|rdeps|path|frontier [flags] <tag>..."

func runGraph(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "Usage: %s\n", graphUsage)
		return ExitUsage
	}
	sub := args[0]
	name := "m2 graph " + sub
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree")
	out := flags.String("out", "", "the output directory of m2 build. Empty means build/ in the tree.")
	asJSON := flags.Bool("json", false, "write the result as JSON")
	var measure, msc, topic, evidence *string
	var open *bool
	var hops *int
	nargs := 1
	switch sub {
	case "neighbourhood":
		hops = flags.Int("hops", 2, "the number of edges from the node")
	case "closure":
		measure = flags.String("measure", "depth", "the measure of the best alternative: depth or size")
	case "rdeps":
	case "path":
		nargs = 2
	case "frontier":
		nargs = 0
		open = flags.Bool("open", false, "the open problems and their bounds, not the formal statements with no proof")
		msc = flags.String("msc", "", "only the nodes with an MSC code that starts with this")
		topic = flags.String("topic", "", "only the nodes that are part of this topic")
		evidence = flags.String("evidence", "", "only the nodes at this evidence level, for example E1")
	default:
		fmt.Fprintf(stderr, "m2 graph: unknown query %q. Usage: %s\n", sub, graphUsage)
		return ExitUsage
	}
	if err := flags.Parse(args[1:]); err != nil {
		return ExitUsage
	}
	if flags.NArg() != nargs {
		switch nargs {
		case 0:
			fmt.Fprintf(stderr, "%s: give no argument, only flags\n", name)
		case 1:
			fmt.Fprintf(stderr, "%s: give one tag, alias or m2:// URI\n", name)
		default:
			fmt.Fprintf(stderr, "%s: give two tags, aliases or m2:// URIs\n", name)
		}
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	if *out == "" {
		*out = filepath.Join(*root, "build")
	}
	path := filepath.Join(*out, build.StoreFile)
	s, err := store.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(stderr, "%s: %s does not exist. Run m2 build first.\n", name, path)
		return ExitFailure
	}
	if err != nil {
		return fail(err)
	}
	defer func() { _ = s.Close() }()
	var tags []string
	for _, ref := range flags.Args() {
		t, err := one(s, ref)
		if err != nil {
			return fail(err)
		}
		tags = append(tags, t)
	}

	var v any
	var text func(w io.Writer)
	switch sub {
	case "neighbourhood":
		if *hops < 0 {
			fmt.Fprintf(stderr, "%s: the number of hops %d is negative\n", name, *hops)
			return ExitUsage
		}
		nb, err := s.Neighbourhood(tags[0], *hops)
		if err != nil {
			return fail(err)
		}
		v, text = nb, func(w io.Writer) { printNeighbourhood(w, s, nb) }
	case "closure":
		m := graph.MinDepth
		switch *measure {
		case "depth":
		case "size":
			m = graph.MinSize
		default:
			fmt.Fprintf(stderr, "%s: the measure %q is not depth or size\n", name, *measure)
			return ExitUsage
		}
		c, err := s.Closure(tags[0], m)
		if err != nil {
			return fail(err)
		}
		v, text = c, func(w io.Writer) { printClosure(w, s, c) }
	case "rdeps":
		d, err := s.Dependents(tags[0])
		if err != nil {
			return fail(err)
		}
		v, text = d, func(w io.Writer) {
			fmt.Fprintf(w, "needs %s (%d)\n", d.Tag, len(d.Needs))
			printTags(w, s, d.Needs)
			fmt.Fprintf(w, "uses %s (%d)\n", d.Tag, len(d.Uses))
			printTags(w, s, d.Uses)
		}
	case "path":
		p, err := s.Path(tags[0], tags[1])
		if err != nil {
			return fail(err)
		}
		v, text = p, func(w io.Writer) { printTags(w, s, p) }
	case "frontier":
		f := store.Filter{Open: *open, MSC: *msc, Topic: *topic}
		if *topic != "" {
			if f.Topic, err = one(s, *topic); err != nil {
				return fail(err)
			}
		}
		if *evidence != "" {
			if err := f.Evidence.UnmarshalText([]byte(*evidence)); err != nil || f.Evidence == status.None {
				fmt.Fprintf(stderr, "%s: the evidence level %q is not one of E0 to E5\n", name, *evidence)
				return ExitUsage
			}
		}
		es, err := s.Frontier(f)
		if err != nil {
			return fail(err)
		}
		v, text = es, func(w io.Writer) { printFrontier(w, es) }
	}
	if *asJSON {
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fail(err)
		}
		_, _ = fmt.Fprintf(stdout, "%s\n", data)
		return ExitOK
	}
	text(stdout)
	return ExitOK
}

// one resolves ref to the tag of one node.
func one(s *store.Store, ref string) (string, error) {
	tags, err := s.Resolve(ref)
	if err != nil {
		return "", err
	}
	if len(tags) > 1 {
		return "", fmt.Errorf("%s names %d nodes: %s. Give one tag", ref, len(tags), strings.Join(tags, ", "))
	}
	return tags[0], nil
}

func printNeighbourhood(w io.Writer, s *store.Store, nb store.Neighbourhood) {
	fmt.Fprintf(w, "nodes at most %d edges from %s (%d)\n", nb.Hops, nb.Tag, len(nb.Nodes))
	printTags(w, s, nb.Nodes)
	fmt.Fprintf(w, "edges (%d)\n", len(nb.Edges))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, e := range nb.Edges {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", e.From, e.Type, e.To, strings.TrimSpace(strings.Join([]string{e.System, e.Label, via(e.Via)}, " ")))
	}
	_ = tw.Flush()
}

func via(v string) string {
	if v == "" {
		return ""
	}
	return "via " + v
}

func printClosure(w io.Writer, s *store.Store, c store.Closure) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "closure\t%s\n", c.Tag)
	fmt.Fprintf(tw, "nodes\t%d\n", len(c.Nodes))
	fmt.Fprintf(tw, "depth\t%d\n", c.Depth)
	fmt.Fprintf(tw, "proofs\t%s\n", list(c.Proofs))
	fmt.Fprintf(tw, "evidence\t%s\n", level(c.Evidence))
	fmt.Fprintf(tw, "weakest\t%s\n", level(c.Weakest))
	fmt.Fprintf(tw, "below\t%s\n", list(c.Below))
	fmt.Fprintf(tw, "axioms\t%s\n", list(c.Axioms))
	fmt.Fprintf(tw, "hypotheses\t%s\n", list(c.Hypotheses))
	_ = tw.Flush()
	printTags(w, s, c.Nodes)
}

// printTags writes one line for each tag, with the kind, the evidence level and the name of the node.
func printTags(w io.Writer, s *store.Store, tags []string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, t := range tags {
		n, _ := s.Node(t)
		st, _ := s.Status(t)
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", t, n.Kind, level(st.Evidence), n.Name)
	}
	_ = tw.Flush()
}

func printFrontier(w io.Writer, es []store.Entry) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, e := range es {
		fidelity := "-"
		if e.Fidelity != status.Unbound {
			fidelity = e.Fidelity.String()
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", e.Tag, e.Kind, level(e.Evidence), fidelity, or(strings.Join(e.MSC, " "), "-"), e.Name)
		for _, b := range e.Bounds {
			via := ""
			if b.Via != "" {
				via = ", via " + b.Via
			}
			fmt.Fprintf(tw, "\t%s bound %s by %s at %s%s\n", b.Side, b.Value, b.Tag, level(b.Evidence), via)
		}
	}
	_ = tw.Flush()
	fmt.Fprintf(w, "%d nodes\n", len(es))
}

func level(l status.Level) string {
	if l == status.None {
		return "-"
	}
	return l.String()
}
