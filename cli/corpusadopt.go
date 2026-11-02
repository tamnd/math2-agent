package cli

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	wpImport "github.com/tamnd/math2-agent/importer/wikipedia"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/tag"
	"github.com/tamnd/math2-agent/wpsource"
)

const adoptUsage = "m2 corpus adopt -source wikipedia -corpus DIR [-pages FILE] [-hold FILE] [-root DIR] [-day YYYY-MM-DD] [-n]"

// runCorpusAdopt makes nodes from the text of Wikipedia items (corpus wikipedia WP9.2). It writes the nodes to nodes/~/, the rows with the outcomes it set, and the report reports/adopt/wikipedia.md.
func runCorpusAdopt(args []string, stdout, stderr io.Writer) int {
	const name = "m2 corpus adopt"
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	source := fs.String("source", "", "the source. Only wikipedia has adoption.")
	corpus := fs.String("corpus", "", "the checkout of tamnd/wikipedia-math")
	pagesFile := fs.String("pages", "", "a file with one article title on each line, as manifests/wikipedia/pilot.txt. Empty means each article.")
	holdFile := fs.String("hold", "", "a Markdown file whose YAML header has the list hold of the mentions that a person holds, each with mention and reason")
	day := fs.String("day", time.Now().UTC().Format(time.DateOnly), "the date of the run, for the history of the rows")
	dry := fs.Bool("n", false, "write only the report, and no nodes and no rows")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 || *source != wpImport.Source || *corpus == "" {
		fmt.Fprintln(stderr, "usage: "+adoptUsage)
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
	opt := wpImport.AdoptOptions{Corpus: *corpus, Day: *day, Draw: func() (string, error) { return tag.Provisional(nil) }}
	if *pagesFile != "" {
		if opt.Pages, err = readPages(*pagesFile, ms); err != nil {
			return fail(err)
		}
	}
	if *holdFile != "" {
		if opt.Hold, err = readHold(*holdFile); err != nil {
			return fail(err)
		}
	}
	res, err := wpImport.Adopt(rows, ms, adoptNodes(nodes), opt)
	if err != nil {
		return fail(err)
	}
	if !*dry {
		for _, d := range res.Drafts {
			path := filepath.Join(*root, d.Path)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- content directories are public.
				return fail(err)
			}
			if err := os.WriteFile(path, d.Data, 0o644); err != nil { // #nosec G306 -- content files are public text in a git tree.
				return fail(err)
			}
		}
		if res.Linked > 0 {
			if err := inventory.WriteRows(*root, res.Rows); err != nil {
				return fail(err)
			}
		}
	}
	report := filepath.Join(*root, wpImport.AdoptPath)
	if err := os.MkdirAll(filepath.Dir(report), 0o755); err != nil { // #nosec G301 -- report directories are public.
		return fail(err)
	}
	if err := os.WriteFile(report, wpImport.AdoptReport(res, *day), 0o644); err != nil { // #nosec G306 -- reports are public text in a git tree.
		return fail(err)
	}
	n := map[string]int{}
	for _, it := range res.Items {
		n[it.State]++
	}
	fmt.Fprintf(stdout, "%s: %d items, %d adopted, %d drafted, %d waiting, %d rows linked\n", name, len(res.Items), n[wpImport.Adopted], n[wpImport.Drafted], n[wpImport.Waiting], res.Linked)
	return ExitOK
}

// adoptNodes gives what the adopt job reads from each node.
func adoptNodes(nodes []*node.Node) []wpImport.AdoptNode {
	out := make([]wpImport.AdoptNode, 0, len(nodes))
	for _, n := range nodes {
		a := wpImport.AdoptNode{ID: n.ID, Name: n.Get("name"), Aliases: n.List("aliases"), MSC: n.List("msc"), Inventory: n.List("inventory"), Proves: n.Get("proves")}
		_, a.Retired = n.Meta["retired"]
		if srcs, ok := n.Meta["sources"].([]any); ok {
			for _, s := range srcs {
				m, _ := s.(map[string]any)
				if _, ok := m[wpsource.Key]; !ok {
					continue
				}
				if id, ok := m["mention"].(string); ok {
					a.Mentions = append(a.Mentions, id)
				}
			}
		}
		out = append(out, a)
	}
	return out
}

// readPages gives the pages of the titles in the file, by the titles of the article mentions.
func readPages(path string, ms []inventory.Mention) (map[int]bool, error) {
	f, err := os.Open(path) // #nosec G304 -- the user gives the file.
	if err != nil {
		return nil, err
	}
	defer f.Close()
	byTitle := map[string]int{}
	for _, m := range ms {
		if m.Source == wpImport.Source && !strings.Contains(m.ID, "#") {
			byTitle[m.NativeID] = m.Page
		}
	}
	pages := map[int]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		t := strings.TrimSpace(sc.Text())
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		p, ok := byTitle[t]
		if !ok {
			return nil, fmt.Errorf("%s: the article %q has no mention in the tree", path, t)
		}
		pages[p] = true
	}
	return pages, sc.Err()
}

// readHold reads the list hold from the YAML header of a Markdown file.
func readHold(path string) (map[string]string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the user gives the file.
	if err != nil {
		return nil, err
	}
	head, ok := strings.CutPrefix(string(data), "---\n")
	if ok {
		head, _, ok = strings.Cut(head, "\n---\n")
	}
	if !ok {
		return nil, fmt.Errorf("%s: the file has no YAML header", path)
	}
	var h struct {
		Hold []struct {
			Mention string `yaml:"mention"`
			Reason  string `yaml:"reason"`
		} `yaml:"hold"`
	}
	if err := yaml.Unmarshal([]byte(head), &h); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := map[string]string{}
	for _, x := range h.Hold {
		if x.Mention == "" || x.Reason == "" {
			return nil, fmt.Errorf("%s: each item of hold needs a mention and a reason", path)
		}
		out[x.Mention] = x.Reason
	}
	return out, nil
}
