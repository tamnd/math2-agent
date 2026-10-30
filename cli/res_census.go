package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tamnd/math2-agent/resource/census"
	resscope "github.com/tamnd/math2-agent/resource/scope"
)

const resCensusUsage = "m2 res census -in DIR [-root DIR] [-env FILE] [-sample N] [-seed N] [-pairs A:B,C:D]"

func runResCensus(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res census", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree. The report goes to "+census.ReportFile+" in it.")
	in := fs.String("in", "", "the directory with one file <catalogue>.jsonl of census records for each catalogue")
	envFile := fs.String("env", "", "a JSON file with the venue register, the Wikidata items of mathematics and the topic threshold of the scope tests")
	sample := fs.Int("sample", 5000, "the size of the sample of in records of each catalogue for the overlap")
	seed := fs.Uint64("seed", 1, "the seed of the samples and of the bootstrap")
	pairs := fs.String("pairs", "", "the pairs of independent catalogues, as A:B separated by commas")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 || *in == "" || *sample < 1 {
		fmt.Fprintln(stderr, "usage: "+resCensusUsage)
		return ExitUsage
	}
	ps, err := parsePairs(*pairs)
	if err != nil {
		fmt.Fprintf(stderr, "m2 res census: %v\n", err)
		return ExitUsage
	}
	var env resscope.Env
	if *envFile != "" {
		data, err := os.ReadFile(*envFile) // #nosec G304 -- the user names the file.
		if err == nil {
			err = json.Unmarshal(data, &env)
		}
		if err != nil {
			fmt.Fprintf(stderr, "m2 res census: %v\n", err)
			return ExitFailure
		}
	}
	files, err := filepath.Glob(filepath.Join(*in, "*.jsonl"))
	if err != nil || len(files) == 0 {
		fmt.Fprintf(stderr, "m2 res census: %s has no file <catalogue>.jsonl\n", *in)
		return ExitFailure
	}
	c := census.New(env, *sample, *seed)
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".jsonl")
		if err := readCensus(c, name, f); err != nil {
			fmt.Fprintf(stderr, "m2 res census: %v\n", err)
			return ExitFailure
		}
	}
	for _, p := range ps {
		for _, n := range p {
			if c.Catalogue(n) == nil {
				fmt.Fprintf(stderr, "m2 res census: the pair names %s, and %s has no file %s.jsonl\n", n, *in, n)
				return ExitUsage
			}
		}
	}
	var buf bytes.Buffer
	if err := c.WriteReport(&buf, ps); err != nil {
		fmt.Fprintf(stderr, "m2 res census: %v\n", err)
		return ExitFailure
	}
	out := filepath.Join(*root, census.ReportFile)
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		fmt.Fprintf(stderr, "m2 res census: %v\n", err)
		return ExitFailure
	}
	if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil { // #nosec G306 -- reports are public text in a git tree.
		fmt.Fprintf(stderr, "m2 res census: %v\n", err)
		return ExitFailure
	}
	for _, cat := range c.Catalogues() {
		fmt.Fprintf(stdout, "%s %d records, %.0f in\n", cat.Name, cat.Records, cat.All["in"])
	}
	fmt.Fprintf(stdout, "wrote %s\n", out)
	return ExitOK
}

func parsePairs(s string) ([][2]string, error) {
	var out [][2]string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		a, b, ok := strings.Cut(p, ":")
		if !ok || a == "" || b == "" || a == b {
			return nil, fmt.Errorf("the pair %q is not of the form A:B with two catalogues", p)
		}
		out = append(out, [2]string{a, b})
	}
	return out, nil
}

func readCensus(c *census.Census, name, path string) error {
	f, err := os.Open(path) // #nosec G304 -- the user names the directory.
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 64<<20)
	for n := 1; sc.Scan(); n++ {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var r census.Record
		dec := json.NewDecoder(bytes.NewReader(sc.Bytes()))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&r); err != nil {
			return fmt.Errorf("%s:%d: %w", path, n, err)
		}
		c.Add(name, r)
	}
	return sc.Err()
}
