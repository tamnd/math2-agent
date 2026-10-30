package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/resource"
	rescluster "github.com/tamnd/math2-agent/resource/cluster"
	resresolve "github.com/tamnd/math2-agent/resource/resolve"
	"github.com/tamnd/math2-agent/tag"
)

const resShowUsage = "m2 res show [-root DIR] [-json] <resource id or external id>"

func runResShow(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res show", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	asJSON := fs.Bool("json", false, "print the record as JSON")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: "+resShowUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 res show: %v\n", err)
		return ExitFailure
	}
	rs, err := resource.ReadResources(*root)
	if err != nil {
		return fail(err)
	}
	ix := resresolve.Build(rs)
	q := fs.Arg(0)
	var at *resresolve.Ref
	var r *resource.Resource
	for i := range rs {
		if rs[i].ID == q {
			r = &rs[i]
		}
	}
	if r == nil && !tag.IsTag(q) && !tag.IsProvisional(q) {
		links, err := rescluster.ReadCrosswalks(*root)
		if err != nil {
			return fail(err)
		}
		res, err := (&resresolve.Resolver{Index: ix, Crosswalks: resresolve.Links(links)}).ID(context.Background(), q)
		if err != nil {
			return fail(err)
		}
		switch res.Status {
		case resresolve.Found:
			r, at = ix.Resource(res.Ref.Resource), res.Ref
		case resresolve.Candidates:
			var ids []string
			for _, c := range res.Candidates {
				ids = append(ids, c.Resource)
			}
			fmt.Fprintf(stderr, "m2 res show: the id %s belongs to more than one resource: %s. Rule H04 reports it\n", res.Query, strings.Join(ids, ", "))
			return ExitFailure
		}
	}
	if r == nil {
		fmt.Fprintf(stderr, "m2 res show: no resource has the id %s\n", q)
		return ExitFailure
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			return fail(err)
		}
		return ExitOK
	}
	showResource(stdout, r, at)
	return ExitOK
}

// showResource prints a resource as text: the work, its expressions and manifestations, its relations, its catalogue records and its history. The place that the query named has a mark.
func showResource(w io.Writer, r *resource.Resource, at *resresolve.Ref) {
	p := func(format string, args ...any) { fmt.Fprintf(w, format+"\n", args...) }
	p("%s %s", r.ID, resource.URI(r.ID))
	p("kind: %s", r.Kind)
	if len(r.Genre) > 0 {
		p("genre: %s", strings.Join(r.Genre, ", "))
	}
	p("title: %s", r.Title)
	if r.TitleEn != "" {
		p("title in English: %s", r.TitleEn)
	}
	for _, c := range r.Creators {
		p("%s: %s%s", c.Role, c.Name, idList(c.IDs, " "))
	}
	if r.Date != "" {
		p("date: %s", r.Date)
	}
	for _, c := range r.MSC {
		p("msc: %s (from %s)", c.Code, c.From)
	}
	for _, s := range r.Subjects {
		p("subject: %s %s", s.System, s.Code)
	}
	if s := r.Scope; s != nil {
		line := "scope: " + s.Value
		switch {
		case s.Value == resource.Duplicate:
			line += " of " + s.Target
		case s.Value == resource.Deferred && s.Until != "":
			line += " until " + s.Until
		}
		if s.Rule != "" {
			line += ", rule " + s.Rule
		}
		if s.Reason != "" {
			line += ", " + s.Reason
		}
		p("%s, by %s", line, s.By)
	}
	mark := func(key string) string {
		if at != nil && at.At() == strings.TrimSuffix(r.ID+"/"+key, "/") {
			return " <- the id of the query"
		}
		return ""
	}
	if ids := idList(r.IDs, ""); ids != "" {
		p("ids:%s%s", ids, mark(""))
	}
	for _, e := range r.Expressions {
		line := e.Key + " " + e.Kind
		for _, v := range []string{e.Label, e.Language, e.Date} {
			if v != "" {
				line += ", " + v
			}
		}
		p("%s%s%s", line, idList(e.IDs, " "), mark(e.Key))
		for _, c := range e.Contributors {
			p("  %s: %s%s", c.Role, c.Name, idList(c.IDs, " "))
		}
		if e.Structure != "" {
			p("  structure: %s", e.Structure)
		}
		for _, m := range e.Manifestations {
			line := e.Key + "/" + m.Key + " " + m.Kind
			for _, v := range []string{m.Publisher, m.Place, m.Date} {
				if v != "" {
					line += ", " + v
				}
			}
			if a := m.Access; a != nil {
				line += ", access " + a.Class
				if a.Licence != "" {
					line += " " + a.Licence
				}
			}
			p("  %s%s%s", line, idList(m.IDs, " "), mark(e.Key+"/"+m.Key))
			for _, l := range m.Links {
				p("    %s", l)
			}
		}
	}
	for _, rel := range r.Relations {
		line := rel.Kind + " " + rel.To
		if rel.From != "" {
			line = rel.From + " " + line
		}
		if rel.Volume != "" {
			line += ", volume " + rel.Volume
		}
		if rel.Source != "" {
			line += ", from " + rel.Source
		}
		p("relation: %s", line)
	}
	for _, c := range r.Catalogue {
		p("catalogue: %s %s at the level %s, snapshot %s", c.Source, c.NativeID, c.Level, c.Snapshot)
	}
	for _, e := range r.History {
		line := e.Op + " " + e.At + " by " + e.By
		if e.Note != "" {
			line += ", " + e.Note
		}
		p("history: %s", line)
	}
}

// idList gives the ids as " key:value key:value", in the order of the keys, after the prefix. No id gives an empty string.
func idList(m resource.IDs, prefix string) string {
	if len(m) == 0 {
		return ""
	}
	var parts []string
	for _, k := range slices.Sorted(maps.Keys(m)) {
		for _, v := range m[k] {
			parts = append(parts, k+":"+v)
		}
	}
	if prefix == "" {
		return " " + strings.Join(parts, " ")
	}
	return prefix + "[" + strings.Join(parts, " ") + "]"
}
