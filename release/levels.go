package release

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/status"
)

// NodeLevels are the levels of one node at a release. An empty level means that the node has no level, for example a definition without evidence or a node without a formal binding.
type NodeLevels struct {
	Evidence string // for example E5
	Fidelity string // for example F2
}

// Levels gives the levels of each live node at a release, by tag. A release keeps them in the file [LevelsFile] next to its manifest, thus the notes of the next release can list the nodes that moved evidence level without a build of the old commit.
type Levels map[string]NodeLevels

// levelsHeader is the first line of a levels file.
const levelsHeader = "# tag evidence fidelity\n"

// LevelsOf returns the levels of the live nodes of a build.
func LevelsOf(st []status.Status) Levels {
	out := Levels{}
	for _, s := range st {
		if !s.Retired {
			out[s.Tag] = NodeLevels{Evidence: s.Evidence.String(), Fidelity: s.Fidelity.String()}
		}
	}
	return out
}

// Marshal returns the levels as text: a header line, then one line for each node in tag order, with the fields separated by one space. A dash is an empty level.
func (l Levels) Marshal() []byte {
	var b bytes.Buffer
	b.WriteString(levelsHeader)
	tags := make([]string, 0, len(l))
	for t := range l {
		tags = append(tags, t)
	}
	slices.Sort(tags)
	for _, t := range tags {
		fmt.Fprintf(&b, "%s %s %s\n", t, dash(l[t].Evidence), dash(l[t].Fidelity))
	}
	return b.Bytes()
}

// ReadLevels reads a levels file that [Levels.Marshal] wrote.
func ReadLevels(path string) (Levels, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the caller names the file.
	if err != nil {
		return nil, err
	}
	out := Levels{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 3 {
			return nil, fmt.Errorf("release: %s:%d: a line has the fields tag, evidence and fidelity", path, n)
		}
		out[f[0]] = NodeLevels{Evidence: undash(f[1]), Fidelity: undash(f[2])}
	}
	return out, sc.Err()
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func undash(s string) string {
	if s == "-" {
		return ""
	}
	return s
}
