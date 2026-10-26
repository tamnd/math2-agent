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
	"github.com/tamnd/math2-agent/tag"
)

const corpusUsage = "m2 corpus import [-root DIR] [-commit C] [source...]\n       m2 corpus status [-root DIR] [-json]"

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
	}
	fmt.Fprintln(stderr, "usage: "+corpusUsage)
	return ExitUsage
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
		m, err := ingest.ReadManifest(filepath.Join(*root, ingest.ManifestPath(id)))
		if err != nil {
			fmt.Fprintf(stderr, "m2 corpus import: the import manifest of %s: %v\n", id, err)
			return ExitFailure
		}
		recs, _, err := ingest.ReadRecords(*root, id)
		if err != nil {
			fmt.Fprintf(stderr, "m2 corpus import: %v\n", err)
			return ExitFailure
		}
		ms := inventory.FromRecords(src, m.Snapshot, m.Adapter, recs)
		if err := inventory.WriteMentions(*root, id, ms); err != nil {
			fmt.Fprintf(stderr, "m2 corpus import: %v\n", err)
			return ExitFailure
		}
		fmt.Fprintf(stdout, "%s: %d mentions\n", id, len(ms))
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
