package wpcorpus

import (
	"bytes"
	"fmt"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
)

// RunsDir is the directory of the run records of the corpus.
var RunsDir = filepath.Join("reports", "runs")

// RunRecord is the record of one run of a command of m2 wp (WP12.6): the
// command, the versions, the dump, the articles that it read and the
// result.
type RunRecord struct {
	Command   string   `yaml:"command"`
	Args      []string `yaml:"args,omitempty"`
	M2        string   `yaml:"m2"`
	Wiki      string   `yaml:"wiki,omitempty"`
	Dump      string   `yaml:"dump,omitempty"`
	Started   string   `yaml:"started"`
	Finished  string   `yaml:"finished"`
	Articles  int      `yaml:"articles"`
	Pages     int      `yaml:"pages"`
	Stopped   int      `yaml:"stopped,omitempty"`
	Failed    int      `yaml:"failed"`
	Requests  int      `yaml:"requests,omitempty"`
	PerMinute float64  `yaml:"requests_per_minute,omitempty"`
	Failures  []string `yaml:"failures,omitempty"`
}

// WikiVersion gives the version of the module tamnd/wikipedia-cli in the
// build, or "".
func WikiVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, d := range bi.Deps {
		if d.Path == "github.com/tamnd/wikipedia-cli" {
			return d.Version
		}
	}
	return ""
}

// RunPath gives the path of a run record, by the start time and the
// command.
func RunPath(started time.Time, command string) string {
	name := strings.ReplaceAll(strings.TrimPrefix(command, "m2 "), " ", "-")
	return filepath.Join(RunsDir, started.UTC().Format("20060102-150405")+"-"+name+".md")
}

// WriteRunRecord writes a run record of the corpus, and gives its path.
func WriteRunRecord(root string, r *RunRecord) (string, error) {
	started, err := time.Parse(time.RFC3339, r.Started)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := writeFrontMatter(&b, r); err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "\n# Run of %s\n\n", r.Command)
	fmt.Fprintf(&b, "The run started at %s and finished at %s. It read %d articles, wrote %d pages and had %d failures.", r.Started, r.Finished, r.Articles, r.Pages, r.Failed)
	if r.Requests > 0 {
		fmt.Fprintf(&b, " It made %d requests, at %.1f requests a minute.", r.Requests, r.PerMinute)
	}
	b.WriteString("\n")
	rel := RunPath(started, r.Command)
	return rel, writeFile(filepath.Join(root, rel), b.Bytes())
}

// ParsoidDir is the work area of the Parsoid HTML in the corpus. It is not
// committed (WP2.5).
var ParsoidDir = filepath.Join("work", "parsoid")

// ParsoidPath gives the path of the Parsoid HTML of a page revision.
func ParsoidPath(page, rev int) string {
	return filepath.Join(ParsoidDir, Shard(page), fmt.Sprintf("%d-r%d.html", page, rev))
}
