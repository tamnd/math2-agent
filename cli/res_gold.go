package cli

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/tamnd/math2-agent/resource"
	rescluster "github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/gold"
)

const resGoldUsage = "m2 res gold [-root DIR] sample -set work|preprint|cite [-n N] [-seed S] | label -set S -a KEY -b KEY -value same|different -by P [-note T] | label -set cite -ref REF -value ID|none -by P [-note T] | eval [-out FILE]"

// ResGoldReport is the report of the gold sets of the clustering.
const ResGoldReport = "reports/resources-gold.md"

func runResGold(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res gold", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "usage: "+resGoldUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 res gold: %v\n", err)
		return ExitFailure
	}
	sub, rest := fs.Arg(0), fs.Args()[1:]
	sf := flag.NewFlagSet("m2 res gold "+sub, flag.ContinueOnError)
	sf.SetOutput(stderr)
	set := sf.String("set", "", "the gold set: work, preprint or cite")
	switch sub {
	case "sample":
		n := sf.Int("n", 2000, "the number of new pairs")
		seed := sf.String("seed", "", "the seed of the sample. The same seed gives the same sample")
		if err := sf.Parse(rest); err != nil || (gold.Files[*set] == "" && *set != citeSet) {
			fmt.Fprintln(stderr, "usage: "+resGoldUsage)
			return ExitUsage
		}
		if *set == citeSet {
			return sampleCitations(*root, *n, *seed, stdout, fail)
		}
		recs, err := rescluster.ReadRecords(*root)
		if err != nil {
			return fail(err)
		}
		have, err := gold.Read(*root, *set)
		if err != nil {
			return fail(err)
		}
		ps, err := gold.Sample(recs, *set, *n, *seed, have)
		if err != nil {
			return fail(err)
		}
		if err := gold.Write(*root, *set, append(have, ps...)); err != nil {
			return fail(err)
		}
		fmt.Fprintf(stdout, "added %d pairs to %s, %d pairs in all\n", len(ps), gold.Files[*set], len(have)+len(ps))
		return ExitOK
	case "label":
		a := sf.String("a", "", "the key of the first record, source:native_id")
		b := sf.String("b", "", "the key of the second record")
		value := sf.String("value", "", "same or different, or for cite a resource id or none")
		by := sf.String("by", "", "the person, or judge and its version")
		note := sf.String("note", "", "a note on the label")
		r := sf.String("ref", "", "the reference of G-cite, as <id>/e<n>#<n>")
		err := sf.Parse(rest)
		if err == nil && *set == citeSet && *r != "" && *by != "" {
			return labelCitation(*root, *r, gold.Label{Value: *value, By: *by, At: time.Now().UTC().Format(time.DateOnly), Note: *note}, stdout, fail)
		}
		if err != nil || gold.Files[*set] == "" || *a == "" || *b == "" || *by == "" {
			fmt.Fprintln(stderr, "usage: "+resGoldUsage)
			return ExitUsage
		}
		ps, err := gold.Read(*root, *set)
		if err != nil {
			return fail(err)
		}
		if err := gold.AddLabel(ps, *a, *b, gold.Label{Value: *value, By: *by, At: time.Now().UTC().Format(time.DateOnly), Note: *note}); err != nil {
			return fail(err)
		}
		if err := gold.Write(*root, *set, ps); err != nil {
			return fail(err)
		}
		done, open, disputed := gold.Labelled(ps)
		fmt.Fprintf(stdout, "%s: labelled %d, open %d, waiting for a third label %d\n", *set, done, open, disputed)
		return ExitOK
	case "eval":
		out := sf.String("out", ResGoldReport, "the report, relative to the root")
		if err := sf.Parse(rest); err != nil {
			fmt.Fprintln(stderr, "usage: "+resGoldUsage)
			return ExitUsage
		}
		recs, err := rescluster.ReadRecords(*root)
		if err != nil {
			return fail(err)
		}
		links, err := rescluster.ReadCrosswalks(*root)
		if err != nil {
			return fail(err)
		}
		// The run has no decisions of people: the report measures the passes.
		res, err := rescluster.Run(rescluster.Input{Records: recs, Crosswalks: links, By: "m2 " + version + " " + rescluster.Version, Day: time.Now().UTC().Format(time.DateOnly)})
		if err != nil {
			return fail(err)
		}
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "# The gold sets of the clustering\n\nThe report gives the precision and the recall of the merges of each pass of %s on each gold set (RS10.7). A pair counts when it has its label and the run has its two records. The targets for all passes are a precision of %.3f and a recall of %.2f.\n", rescluster.Version, gold.TargetPrecision, gold.TargetRecall)
		for _, s := range []string{gold.Work, gold.Preprint} {
			ps, err := gold.Read(*root, s)
			if err != nil {
				return fail(err)
			}
			done, open, disputed := gold.Labelled(ps)
			fmt.Fprintf(&buf, "\n## %s\n\nThe file `%s` has %d pairs: %d with a label, %d open, and %d that wait for a third label.\n", s, gold.Files[s], len(ps), done, open, disputed)
			scores, met, n := gold.Eval(res, ps)
			if n == 0 {
				fmt.Fprintf(&buf, "\nNo labelled pair has its two records in the run.\n")
				fmt.Fprintf(stdout, "%s: no labelled pair in the run\n", s)
				continue
			}
			fmt.Fprintf(&buf, "\n%d pairs count.\n\n| Pass | Merged | Wrong | Missed | Precision | Recall |\n|---|---:|---:|---:|---:|---:|\n", n)
			for _, x := range scores {
				fmt.Fprintf(&buf, "| %s | %d | %d | %d | %.4f | %.4f |\n", x.Pass, x.Merged, x.Wrong, x.Missed, x.Precision, x.Recall)
			}
			verdict := "do not meet"
			if met {
				verdict = "meet"
			}
			all := scores[len(scores)-1]
			fmt.Fprintf(&buf, "\nThe merges of all passes %s the targets.\n", verdict)
			fmt.Fprintf(stdout, "%s: %d pairs, precision %.4f, recall %.4f, the targets: %s\n", s, n, all.Precision, all.Recall, verdict)
		}
		if err := evalCitations(*root, &buf, stdout); err != nil {
			return fail(err)
		}
		path := filepath.Join(*root, *out)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return fail(err)
		}
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil { // #nosec G306 -- reports are public text in a git tree.
			return fail(err)
		}
		fmt.Fprintf(stdout, "wrote %s\n", *out)
		return ExitOK
	}
	fmt.Fprintln(stderr, "usage: "+resGoldUsage)
	return ExitUsage
}

// citeSet is the name of G-cite in the flag -set.
const citeSet = "cite"

func sampleCitations(root string, n int, seed string, stdout io.Writer, fail func(error) int) int {
	rs, err := resource.ReadResources(root)
	if err != nil {
		return fail(err)
	}
	langs := map[string]string{}
	for i := range rs {
		for _, e := range rs[i].Expressions {
			langs[rs[i].ID+"/"+e.Key] = e.Language
		}
	}
	edges, err := resource.ReadCites(root)
	if err != nil {
		return fail(err)
	}
	have, err := gold.ReadCitations(root)
	if err != nil {
		return fail(err)
	}
	cs := gold.SampleCitations(edges, func(from string) string { return langs[from] }, n, seed, have)
	if err := gold.WriteCitations(root, append(have, cs...)); err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "added %d references to %s, %d references in all\n", len(cs), gold.CitationsFile, len(have)+len(cs))
	return ExitOK
}

func labelCitation(root, ref string, l gold.Label, stdout io.Writer, fail func(error) int) int {
	cs, err := gold.ReadCitations(root)
	if err != nil {
		return fail(err)
	}
	if err := gold.AddCiteLabel(cs, ref, l); err != nil {
		return fail(err)
	}
	if err := gold.WriteCitations(root, cs); err != nil {
		return fail(err)
	}
	done := 0
	for i := range cs {
		if cs[i].Value() != "" {
			done++
		}
	}
	fmt.Fprintf(stdout, "%s: labelled %d of %d\n", citeSet, done, len(cs))
	return ExitOK
}

// evalCitations adds the section of G-cite to the report: the precision and the recall of the cites edges on the labelled references.
func evalCitations(root string, buf *bytes.Buffer, stdout io.Writer) error {
	cs, err := gold.ReadCitations(root)
	if err != nil {
		return err
	}
	edges, err := resource.ReadCites(root)
	if err != nil {
		return err
	}
	s := gold.EvalCitations(edges, cs)
	fmt.Fprintf(buf, "\n## cite\n\nThe file `%s` has %d references, and %d have a label. The score measures the cites edges of the catalogue against the labels. The spec gives no target for the matcher.\n", gold.CitationsFile, len(cs), s.Labelled)
	if s.Labelled == 0 {
		fmt.Fprintf(stdout, "%s: no labelled reference\n", citeSet)
		return nil
	}
	fmt.Fprintf(buf, "\n| Labelled | Matched | Wrong | Cited | Found | Precision | Recall |\n|---:|---:|---:|---:|---:|---:|---:|\n| %d | %d | %d | %d | %d | %.4f | %.4f |\n", s.Labelled, s.Matched, s.Wrong, s.Cited, s.Found, s.Precision, s.Recall)
	fmt.Fprintf(stdout, "%s: %d references, precision %.4f, recall %.4f\n", citeSet, s.Labelled, s.Precision, s.Recall)
	return nil
}
