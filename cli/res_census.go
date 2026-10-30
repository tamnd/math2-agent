package cli

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource/census"
)

const resCensusUsage = "m2 res census -in DIR [-root DIR] [-env FILE] [-sample N] [-seed N] [-pairs A:B,C:D] [-whole A,B]"

func runResCensus(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res census", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree. The report goes to "+census.ReportFile+" in it.")
	in := fs.String("in", "", "the directory with one file <catalogue>.md of census records for each catalogue")
	envFile := fs.String("env", "", "a JSON file with the venue register, the Wikidata items of mathematics and the topic threshold of the scope tests")
	sample := fs.Int("sample", 5000, "the size of the sample of in records of each catalogue for the overlap")
	seed := fs.Uint64("seed", 1, "the seed of the samples and of the bootstrap")
	pairs := fs.String("pairs", "", "the pairs of independent catalogues, as A:B separated by commas")
	whole := fs.String("whole", "zbmath", "the catalogues of mathematics alone, separated by commas. A lookup of the DOI in one of them counts for an estimate.")
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
	env, err := readEnv(*envFile)
	if err != nil {
		fmt.Fprintf(stderr, "m2 res census: %v\n", err)
		return ExitFailure
	}
	files, err := mdrec.Glob(filepath.Join(*in, "*"+mdrec.Ext))
	if err != nil || len(files) == 0 {
		fmt.Fprintf(stderr, "m2 res census: %s has no file <catalogue>.md\n", *in)
		return ExitFailure
	}
	c := census.New(env, *sample, *seed)
	c.Whole = map[string]bool{}
	for _, n := range strings.Split(*whole, ",") {
		if n = strings.TrimSpace(n); n != "" {
			c.Whole[n] = true
		}
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), mdrec.Ext)
		if err := readCensus(c, name, f); err != nil {
			fmt.Fprintf(stderr, "m2 res census: %v\n", err)
			return ExitFailure
		}
	}
	for _, p := range ps {
		for _, n := range p {
			if c.Catalogue(n) == nil {
				fmt.Fprintf(stderr, "m2 res census: the pair names %s, and %s has no file %s.md\n", n, *in, n)
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
	rs, err := readRecords(path)
	if err != nil {
		return err
	}
	for _, r := range rs {
		c.Add(name, r)
	}
	return nil
}
