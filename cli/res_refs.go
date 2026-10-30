package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/resource"
)

const resRefsUsage = "m2 res refs [-root DIR] [-json] <resource id or id/e<n>>"

// refsOut is the output of m2 res refs with -json.
type refsOut struct {
	Refs    []resource.Cite `json:"refs"`
	CitedBy []resource.Cite `json:"cited_by"`
}

// runResRefs prints the references of a resource with their resolution, and the resources that cite it (RS13.3). An id with an expression key limits both lists to that expression.
func runResRefs(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res refs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	asJSON := fs.Bool("json", false, "print the edges as JSON")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: "+resRefsUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 res refs: %v\n", err)
		return ExitFailure
	}
	q := fs.Arg(0)
	id, _, _ := strings.Cut(q, "/")
	rs, err := resource.ReadResources(*root)
	if err != nil {
		return fail(err)
	}
	titles := make(map[string]string, len(rs))
	for i := range rs {
		titles[rs[i].ID] = rs[i].Title
	}
	if _, ok := titles[id]; !ok {
		fmt.Fprintf(stderr, "m2 res refs: no resource has the id %s\n", id)
		return ExitFailure
	}
	cs, err := resource.ReadCites(*root)
	if err != nil {
		return fail(err)
	}
	out := refsOut{Refs: []resource.Cite{}, CitedBy: []resource.Cite{}}
	for _, c := range cs {
		if refMatch(c.From, q) {
			out.Refs = append(out.Refs, c)
		}
		if c.To != "" && refMatch(c.To, q) {
			out.CitedBy = append(out.CitedBy, c)
		}
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			return fail(err)
		}
		return ExitOK
	}
	resolved := 0
	for _, c := range out.Refs {
		if c.To != "" {
			resolved++
		}
	}
	fmt.Fprintf(stdout, "%s %s\n", id, titles[id])
	fmt.Fprintf(stdout, "references: %d, resolved: %d\n", len(out.Refs), resolved)
	for _, c := range out.Refs {
		if c.To == "" {
			fmt.Fprintf(stdout, "  %s #%d not resolved: %s\n", c.From, c.N, refText(c))
			continue
		}
		to, _, _ := strings.Cut(c.To, "/")
		fmt.Fprintf(stdout, "  %s #%d -> %s %s (score %.3f, %s)\n", c.From, c.N, c.To, titles[to], c.Score, c.Matcher)
	}
	fmt.Fprintf(stdout, "cited by: %d\n", len(out.CitedBy))
	for _, c := range out.CitedBy {
		from, _, _ := strings.Cut(c.From, "/")
		fmt.Fprintf(stdout, "  %s #%d %s\n", c.From, c.N, titles[from])
	}
	return ExitOK
}

// refMatch reports whether the edge end e is the resource or the expression q.
func refMatch(e, q string) bool {
	if strings.Contains(q, "/") {
		return e == q
	}
	return e == q || strings.HasPrefix(e, q+"/")
}

// refText gives the text of an unresolved reference: the text when the edge keeps it, else the parsed fields, else the ids.
func refText(c resource.Cite) string {
	if c.Text != "" {
		return c.Text
	}
	var parts []string
	if p := c.Parsed; p != nil {
		if len(p.Authors) > 0 {
			parts = append(parts, strings.Join(p.Authors, ", "))
		}
		for _, s := range []string{p.Title, p.Venue, p.Volume, p.Pages, p.Year} {
			if s != "" {
				parts = append(parts, s)
			}
		}
		for _, k := range slices.Sorted(maps.Keys(p.IDs)) {
			for _, v := range p.IDs[k] {
				parts = append(parts, k+":"+v)
			}
		}
	}
	if len(parts) == 0 {
		return "no text and no fields"
	}
	return strings.Join(parts, ", ")
}
