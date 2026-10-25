package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/search"
)

const searchUsage = "m2 search [-root DIR] [-out DIR] [-k N] [-channel C,...] [-kind K,...] [-open] [-premises] [-tag T] [-json] <query>"

func runSearch(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("m2 search", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree")
	out := flags.String("out", "", "the output directory of m2 build. Empty means build/ in the tree.")
	k := flags.Int("k", 10, "the number of results")
	channels := flags.String("channel", "", "the channels to run, separated by commas: "+strings.Join(search.Channels(), ", ")+". Empty means all of them.")
	kinds := flags.String("kind", "", "only nodes of these kinds, separated by commas")
	open := flags.Bool("open", false, "rank the open problems and the weak evidence first")
	premises := flags.Bool("premises", false, "the query is a Lean goal. Rank the declarations that can help to prove it.")
	tag := flags.String("tag", "", "with -premises, the node of the goal. The declarations of the nodes that it uses rank higher.")
	asJSON := flags.Bool("json", false, "write the results as JSON")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	text := strings.Join(flags.Args(), " ")
	if strings.TrimSpace(text) == "" && (!*premises || *tag == "") {
		fmt.Fprintln(stderr, "m2 search: give a query")
		return ExitUsage
	}
	if *out == "" {
		*out = filepath.Join(*root, "build")
	}
	path := filepath.Join(*out, build.SearchFile)
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(stderr, "m2 search: %s does not exist. Run m2 build first.\n", path)
		return ExitFailure
	}
	ix, err := search.Open(path, nil)
	if err != nil {
		fmt.Fprintf(stderr, "m2 search: %v\n", err)
		return ExitFailure
	}
	defer func() { _ = ix.Close() }()

	var v any
	if *premises {
		ps, err := ix.Premises(search.Goal{Goal: text, Tag: *tag, K: *k})
		if err != nil {
			fmt.Fprintf(stderr, "m2 search: %v\n", err)
			return ExitFailure
		}
		v = ps
		if !*asJSON {
			tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
			for i, p := range ps {
				fmt.Fprintf(tw, "%d\t%s\t%s\t%.4f\t%s\n", i+1, p.Decl, strings.Join(p.Tags, " "), p.Score, hits(p.Channels))
			}
			_ = tw.Flush()
			return ExitOK
		}
	} else {
		rs, err := ix.Search(search.Query{Text: text, K: *k, Channels: split(*channels), Kinds: split(*kinds), Open: *open})
		if err != nil {
			fmt.Fprintf(stderr, "m2 search: %v\n", err)
			return ExitUsage
		}
		v = rs
		if !*asJSON {
			if len(rs) == 0 {
				fmt.Fprintln(stderr, "m2 search: no results")
				return ExitOK
			}
			tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
			for i, r := range rs {
				key, kind := r.Tag, r.Kind
				if key == "" {
					key, kind = "lean:"+r.Decl, "no node yet"
				}
				name := r.Name
				if len(r.Folded) > 0 {
					name += " (folds " + strings.Join(r.Folded, " ") + ")"
				}
				fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%.4f\t%s\n", i+1, key, kind, name, r.Score, hits(r.Channels))
			}
			_ = tw.Flush()
			return ExitOK
		}
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "m2 search: %v\n", err)
		return ExitFailure
	}
	_, _ = fmt.Fprintf(stdout, "%s\n", data)
	return ExitOK
}

// hits writes the channels that found a result, with the rank in each one, as in "lexical#1 formula#3".
func hits(hs []search.Hit) string {
	parts := make([]string, len(hs))
	for i, h := range hs {
		parts[i] = fmt.Sprintf("%s#%d", h.Channel, h.Rank)
	}
	return strings.Join(parts, " ")
}

// split splits a list separated by commas, and leaves out the empty items.
func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
