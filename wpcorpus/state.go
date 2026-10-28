package wpcorpus

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The files of the scope list in the manifests of the corpus.
var (
	CandidatesPath = filepath.Join(ManifestDir, "candidates.tsv")
	ScopePath      = filepath.Join(ManifestDir, "scope.tsv")
	StoppedPath    = filepath.Join(ManifestDir, "stopped.tsv")
	VerdictsPath   = filepath.Join(ManifestDir, "verdicts.tsv")
)

// The article states of WP1.8 that the corpus has now. The later states
// come with the milestones that make them.
const (
	StateListed  = "listed"
	StateFetched = "fetched"
	StateParsed  = "parsed"
	StateStopped = "stopped"
)

// StopReasons are the reasons of the state stopped (WP1.8).
var StopReasons = map[string]bool{"deleted": true, "merged": true, "not-math": true, "protected-empty": true, "failed": true}

// Stop is one row of manifests/wikipedia/stopped.tsv: an article of the
// scope list that cannot go further.
type Stop struct {
	PageID int64
	Reason string
	Into   int64 // the page id that a merged article goes into, or 0
	Note   string
	Line   int
}

const stoppedHeader = "page_id\treason\tinto\tnote"

// ReadStops reads stopped.tsv of the corpus. A corpus with no such file has
// no stopped article.
func ReadStops(root string) ([]Stop, error) {
	f, err := os.Open(filepath.Join(root, StoppedPath)) // #nosec G304 -- the path is in the corpus that the user names
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return readStops(f)
}

func readStops(r io.Reader) ([]Stop, error) {
	sc := bufio.NewScanner(r)
	var out []Stop
	line := 0
	for sc.Scan() {
		line++
		if line == 1 {
			if sc.Text() != stoppedHeader {
				return nil, fmt.Errorf("line 1: the header is not %q", stoppedHeader)
			}
			continue
		}
		f := strings.Split(sc.Text(), "\t")
		if len(f) != 4 {
			return nil, fmt.Errorf("line %d: want 4 fields, have %d", line, len(f))
		}
		s := Stop{Reason: f[1], Note: f[3], Line: line}
		var err error
		if s.PageID, err = strconv.ParseInt(f[0], 10, 64); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if f[2] != "" {
			if s.Into, err = strconv.ParseInt(f[2], 10, 64); err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
		}
		out = append(out, s)
	}
	return out, sc.Err()
}

// ReadScopeFile reads a candidates file of the corpus, candidates.tsv or
// scope.tsv.
func ReadScopeFile(root, rel string) ([]*Candidate, error) {
	f, err := os.Open(filepath.Join(root, rel)) // #nosec G304 -- the path is in the corpus that the user names
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	cs, err := ReadCandidates(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rel, err)
	}
	return cs, nil
}
