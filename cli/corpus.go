package cli

import (
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

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/queue"
	"github.com/tamnd/math2-agent/tag"
)

const corpusUsage = "m2 corpus import [-root DIR] [-commit C] [source...]\n       m2 corpus status [-root DIR] [-json]\n       m2 corpus queue [-root DIR] [-n N] [-json] NAME\n       m2 corpus plan [-root DIR] [-n N] [-json] NAME"

func runCorpus(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: "+corpusUsage)
		return ExitUsage
	}
	switch args[0] {
	case "import":
		return runCorpusImport(args[1:], stdout, stderr)
	case "status":
		return runCorpusStatus(args[1:], stdout, stderr)
	case "queue", "plan":
		return runCorpusQueue(args[0], args[1:], stdout, stderr)
	}
	fmt.Fprintln(stderr, "usage: "+corpusUsage)
	return ExitUsage
}

// writeMentions changes the records of the last import of the source src to mentions, and writes them to records/mentions/<source>/. It returns the number of mentions.
func writeMentions(root string, src *ingest.Source) (int, error) {
	m, err := ingest.ReadManifest(filepath.Join(root, ingest.ManifestPath(src.ID)))
	if err != nil {
		return 0, fmt.Errorf("the import manifest of %s: %w", src.ID, err)
	}
	recs, _, err := ingest.ReadRecords(root, src.ID)
	if err != nil {
		return 0, err
	}
	ms := inventory.FromRecords(src, m.Snapshot, m.Adapter, recs)
	if err := inventory.WriteMentions(root, src.ID, ms); err != nil {
		return 0, err
	}
	return len(ms), nil
}

// runCorpusImport changes the records of the last import of each source to mentions, and groups the mentions into rows.
func runCorpusImport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 corpus import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	commit := fs.String("commit", "", "the commit that goes into registers/inventory/. Empty means the HEAD commit of the working tree.")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if *commit == "" {
		out, err := git(*root, "rev-parse", "HEAD")
		if err != nil {
			fmt.Fprintln(stderr, "m2 corpus import: the working tree has no HEAD commit. Give the commit with -commit.")
			return ExitFailure
		}
		*commit = strings.TrimSpace(out)
	}
	sources := fs.Args()
	if len(sources) == 0 {
		var err error
		if sources, err = importedSources(*root); err != nil {
			fmt.Fprintf(stderr, "m2 corpus import: %v\n", err)
			return ExitFailure
		}
	}
	if len(sources) == 0 {
		fmt.Fprintf(stderr, "m2 corpus import: %s has no records. Run m2 ingest first.\n", filepath.Join(*root, ingest.RecordsDir))
		return ExitFailure
	}
	cat, err := ingest.ReadCatalogue(*root)
	if err != nil {
		fmt.Fprintf(stderr, "m2 corpus import: %v\n", err)
		return ExitFailure
	}
	for _, id := range sources {
		src := cat.Source(id)
		if src == nil {
			fmt.Fprintf(stderr, "m2 corpus import: manifests/sources.yaml has no source %s\n", id)
			return ExitFailure
		}
		n, err := writeMentions(*root, src)
		if err != nil {
			fmt.Fprintf(stderr, "m2 corpus import: %v\n", err)
			return ExitFailure
		}
		fmt.Fprintf(stdout, "%s: %d mentions\n", id, n)
	}
	ms, err := inventory.ReadMentions(*root)
	if err != nil {
		fmt.Fprintf(stderr, "m2 corpus import: %v\n", err)
		return ExitFailure
	}
	rows, err := inventory.ReadRows(*root)
	if err != nil {
		fmt.Fprintf(stderr, "m2 corpus import: %v\n", err)
		return ExitFailure
	}
	reg, err := inventory.ReadRegister(*root)
	if err != nil {
		fmt.Fprintf(stderr, "m2 corpus import: %v\n", err)
		return ExitFailure
	}
	tags, err := tag.ReadUsed(filepath.Join(*root, "registers", "tags.reg"), filepath.Join(*root, "registers", "retired.reg"))
	if err != nil {
		fmt.Fprintf(stderr, "m2 corpus import: %v\n", err)
		return ExitFailure
	}
	res, err := inventory.Cluster(rows, ms, inventory.Options{
		Day:  time.Now(),
		By:   "m2/" + version,
		Used: func(id string) bool { return reg[id] || tags[id] },
	})
	if err != nil {
		fmt.Fprintf(stderr, "m2 corpus import: %v\n", err)
		return ExitFailure
	}
	if err := inventory.WriteRows(*root, res.Rows); err != nil {
		fmt.Fprintf(stderr, "m2 corpus import: %v\n", err)
		return ExitFailure
	}
	if err := inventory.AppendRegister(*root, res.Added, *commit); err != nil {
		fmt.Fprintf(stderr, "m2 corpus import: %v\n", err)
		return ExitFailure
	}
	fmt.Fprintf(stdout, "rows: %d, new %d, with new mentions %d, merged %d, waiting for a source %d\n", len(res.Rows), len(res.Added), res.Touched, res.Merged, res.Waiting)
	return ExitOK
}

// importedSources returns the sources that have records in build/records/ and an import manifest.
func importedSources(root string) ([]string, error) {
	files, err := os.ReadDir(filepath.Join(root, ingest.RecordsDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range files {
		id, ok := strings.CutSuffix(f.Name(), ".jsonl")
		if !ok {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, ingest.ManifestPath(id))); err == nil {
			out = append(out, id)
		}
	}
	return out, nil
}

// runCorpusStatus prints the counts of the inventory.
func runCorpusStatus(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 corpus status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+corpusUsage)
		return ExitUsage
	}
	ms, err := inventory.ReadMentions(*root)
	if err != nil {
		fmt.Fprintf(stderr, "m2 corpus status: %v\n", err)
		return ExitFailure
	}
	rows, err := inventory.ReadRows(*root)
	if err != nil {
		fmt.Fprintf(stderr, "m2 corpus status: %v\n", err)
		return ExitFailure
	}
	s := inventory.Summarize(rows, ms, time.Now())
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(s); err != nil {
			fmt.Fprintf(stderr, "m2 corpus status: %v\n", err)
			return ExitFailure
		}
		return ExitOK
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "rows\t%d\n", s.Rows)
	fmt.Fprintf(tw, "open\t%d\n", s.Open)
	fmt.Fprintf(tw, "mentions\t%d\n", s.Mentions)
	for _, part := range []struct {
		name string
		m    map[string]int
	}{{"layer", s.Layers}, {"kind", s.Kinds}, {"outcome", s.Outcomes}, {"source", s.Sources}} {
		for _, k := range slices.Sorted(maps.Keys(part.m)) {
			fmt.Fprintf(tw, "%s %s\t%d\n", part.name, k, part.m[k])
		}
	}
	if err := tw.Flush(); err != nil {
		return ExitFailure
	}
	return ExitOK
}

// runCorpusQueue prints a standing queue in priority order, or the plan of its first jobs. It calls no model.
func runCorpusQueue(cmd string, args []string, stdout, stderr io.Writer) int {
	name := "m2 corpus " + cmd
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	n := fs.Int("n", 20, "the number of jobs. -1 means each job of the queue.")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(stderr, "usage: %s [-root DIR] [-n N] [-json] NAME\nThe queues are %s.\n", name, strings.Join(queue.Names(), ", "))
		return ExitUsage
	}
	ms, err := inventory.ReadMentions(*root)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	rows, err := inventory.ReadRows(*root)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	q, err := queue.Compute(fs.Arg(0), queue.Input{Rows: rows, Mentions: ms, Day: time.Now().UTC()})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitUsage
	}
	total := len(q.Items)
	if *n >= 0 && *n < total {
		q.Items = q.Items[:*n]
	}
	var out any = q
	if cmd == "plan" {
		out = queue.MakePlan(q, -1)
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return ExitFailure
		}
		return ExitOK
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	if cmd == "plan" {
		p := out.(queue.Plan)
		fmt.Fprintf(tw, "queue\t%s\n", p.Queue)
		fmt.Fprintf(tw, "jobs\t%d of %d\n", p.Jobs, total)
		fmt.Fprintf(tw, "tokens\t%d in, %d out\n", p.TokensIn, p.TokensOut)
		fmt.Fprintf(tw, "model cost\t$%.2f to $%.2f\n", p.CostLow, p.CostHigh)
		fmt.Fprintf(tw, "review\t%.1f to %.1f hours\n", p.ReviewLow, p.ReviewHigh)
		for _, note := range p.Notes {
			fmt.Fprintf(tw, "note\t%s\n", note)
		}
		fmt.Fprintln(tw, "This is a plan. No model run starts without a budget that a person approved.")
	} else {
		fmt.Fprintf(tw, "%s: %d items, role %s, goal: %s\n", q.Name, total, q.Role, q.Goal)
		if q.Reason != "" {
			fmt.Fprintf(tw, "%s\n", q.Reason)
		}
		for _, it := range q.Items {
			fmt.Fprintf(tw, "%g\t%s\t%s\t%s\n", it.Priority, it.Target, it.Kind, it.Name)
		}
	}
	if err := tw.Flush(); err != nil {
		return ExitFailure
	}
	return ExitOK
}
