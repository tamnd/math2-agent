package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	wpImport "github.com/tamnd/math2-agent/importer/wikipedia"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/wpsource"
)

const parityUsage = "m2 corpus parity -source wikipedia -area NAME [-pages FILE] [-root DIR] [-day YYYY-MM-DD]"

var areaName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// runCorpusParity writes the parity data and the parity report of an area (corpus wikipedia WP9.7).
func runCorpusParity(args []string, stdout, stderr io.Writer) int {
	const name = "m2 corpus parity"
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	source := fs.String("source", "", "the source. Only wikipedia has a parity report.")
	area := fs.String("area", "", "the name of the area, as group-theory. The report is reports/parity/<area>.md.")
	pagesFile := fs.String("pages", "", "a file with one article title on each line. Empty means each article.")
	day := fs.String("day", time.Now().UTC().Format(time.DateOnly), "the date of the report")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 || *source != wpImport.Source || !areaName.MatchString(*area) {
		fmt.Fprintln(stderr, "usage: "+parityUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
	ms, err := inventory.ReadMentions(*root)
	if err != nil {
		return fail(err)
	}
	rows, err := inventory.ReadRows(*root)
	if err != nil {
		return fail(err)
	}
	nodes, err := node.Load(*root)
	if err != nil {
		return fail(err)
	}
	var pages map[int]bool
	if *pagesFile != "" {
		if pages, err = readPages(*pagesFile, ms); err != nil {
			return fail(err)
		}
	}
	items := wpImport.Parity(rows, ms, parityNodes(nodes), pages, *day)
	if err := wpImport.WriteParity(*root, items); err != nil {
		return fail(err)
	}
	report := filepath.Join(*root, wpImport.ParityReportDir, *area+".md")
	if err := os.MkdirAll(filepath.Dir(report), 0o755); err != nil { // #nosec G301 -- report directories are public.
		return fail(err)
	}
	if err := os.WriteFile(report, wpImport.ParityReport(*area, items, *day), 0o644); err != nil { // #nosec G306 -- reports are public text in a git tree.
		return fail(err)
	}
	missing := 0
	for i := range items {
		if items[i].Missing() {
			missing++
		}
	}
	fmt.Fprintf(stdout, "%s: %d items, %d without an outcome, report %s\n", name, len(items), missing, filepath.ToSlash(filepath.Join(wpImport.ParityReportDir, *area+".md")))
	return ExitOK
}

// parityNodes gives the mentions of the wikipedia sources of each live node, with their notes.
func parityNodes(nodes []*node.Node) []wpImport.ParityNode {
	var out []wpImport.ParityNode
	for _, n := range nodes {
		if _, retired := n.Meta["retired"]; retired {
			continue
		}
		srcs, _ := n.Meta["sources"].([]any)
		p := wpImport.ParityNode{ID: n.ID, Mentions: map[string]string{}}
		for _, s := range srcs {
			m, _ := s.(map[string]any)
			if _, ok := m[wpsource.Key]; !ok {
				continue
			}
			id, _ := m["mention"].(string)
			note, _ := m["note"].(string)
			if id != "" {
				p.Mentions[id] = note
			}
		}
		if len(p.Mentions) > 0 {
			out = append(out, p)
		}
	}
	return out
}
