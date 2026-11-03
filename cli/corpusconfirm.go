package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/agent/confirm"
	"github.com/tamnd/math2-agent/agent/write"
	"github.com/tamnd/math2-agent/node"
)

const confirmUsage = "m2 corpus confirm -source arxiv -arxiv DIR [-root DIR] [-node TAG] [-write] [-json]"

// runCorpusConfirm runs m2 corpus confirm, the source confirmation of corpus K9.5 for the research nodes (corpus arxiv AX9.5). The check model comes from $M2_LLM_CHECK, and it must be of another family than the model that wrote each node (rule X09).
func runCorpusConfirm(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 corpus confirm", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	source := fs.String("source", "", "the source. Only arxiv now.")
	arxiv := fs.String("arxiv", "", "the checkout of tamnd/arxiv, which gives the text of the open items")
	only := fs.String("node", "", "check only this node")
	writeOut := fs.Bool("write", false, "write "+confirm.Path+" in the tree")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 || *source != "arxiv" || *arxiv == "" {
		fmt.Fprintln(stderr, "usage: "+confirmUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 corpus confirm: %v\n", err)
		return ExitFailure
	}
	if os.Getenv(CheckEnv) == "" {
		return fail(errors.New("set " + CheckEnv + " to the check model, a backend with its model of another family than the writer, for example codex:gpt-6-sol"))
	}
	nodes, err := node.Load(*root)
	if err != nil {
		return fail(err)
	}
	ctx := context.Background()
	var rs []confirm.Result
	for _, n := range nodes {
		if *only != "" && n.ID != *only || strings.HasPrefix(n.ID, "~") {
			continue
		}
		if _, retired := n.Meta["retired"]; retired {
			continue
		}
		items, err := write.SourceItems(nil, n, *arxiv)
		if err != nil {
			return fail(err)
		}
		if len(items) == 0 {
			continue
		}
		writer := provModel(*root, n.ID)
		if writer == "" {
			for _, it := range items {
				rs = append(rs, confirm.Result{Node: n.ID, Kind: n.Kind(), Name: n.Name(), Mention: it.ID, Locator: it.Locator, Version: fmt.Sprintf("%sv%d", it.Paper, it.Version), Class: it.Class, Licence: it.Licence, Verdict: confirm.Person, Reason: "no provenance names the model that wrote the node, thus rule X09 cannot choose a check model"})
			}
			continue
		}
		m, id, err := newCheck(writer)
		if err != nil {
			return fail(err)
		}
		got, err := confirm.Check(ctx, m, id, writer, n, items)
		rs = append(rs, got...)
		if err != nil {
			return fail(err)
		}
	}
	day := time.Now().UTC().Format(time.DateOnly)
	if *writeOut {
		p := filepath.Join(*root, confirm.Path)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil { // #nosec G301 -- the report is public.
			return fail(err)
		}
		if err := os.WriteFile(p, confirm.Report(rs, day), 0o644); err != nil { // #nosec G306 -- the report is public.
			return fail(err)
		}
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rs); err != nil {
			return fail(err)
		}
		return ExitOK
	}
	for _, r := range rs {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\n", r.Node, r.Version+" "+r.Locator, r.Model, r.Verdict, r.Reason)
	}
	return ExitOK
}

// provModel gives the model of the last line of the provenance of a node that names one, or "".
func provModel(root, tag string) string {
	if len(tag) < 3 {
		return ""
	}
	f, err := os.Open(filepath.Join(root, "prov", tag[:3], tag+".jsonl")) // #nosec G304 -- the path is in the tree.
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	model := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var l struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(sc.Bytes(), &l) == nil && l.Model != "" {
			model = l.Model
		}
	}
	return model
}
