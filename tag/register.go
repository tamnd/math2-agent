package tag

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"slices"
	"strings"
)

// Entry is one line of registers/tags.reg: the tag, the commit at which it was minted and the kind of the node.
type Entry struct {
	Tag    string
	Commit string
	Kind   string
}

// String returns the entry as one register line.
func (e Entry) String() string { return e.Tag + " " + e.Commit + " " + e.Kind }

// ReadUsed returns the tags in each register file. The first field of each line is a tag. Blank lines and lines that start with # are not records. A file that does not exist has no tags.
func ReadUsed(paths ...string) (map[string]bool, error) {
	used := map[string]bool{}
	for _, path := range paths {
		data, err := os.ReadFile(path) // #nosec G304 -- the caller gives a path in the working tree.
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(bytes.NewReader(data))
		for n := 1; sc.Scan(); n++ {
			fields := strings.Fields(sc.Text())
			if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
				continue
			}
			if !IsTag(fields[0]) {
				return nil, fmt.Errorf("%s:%d: %q is not a tag", path, n, fields[0])
			}
			used[fields[0]] = true
		}
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return used, nil
}

// Append adds entries to the register at path and keeps the records sorted. It does not change or remove a line that exists (rule T01). It fails if a tag is already in the register.
func Append(path string, entries []Entry) error {
	data, err := os.ReadFile(path) // #nosec G304 -- the caller gives a path in the working tree.
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var header, records []string
	for line := range strings.Lines(string(data)) {
		line = strings.TrimRight(line, "\n")
		if line == "" || strings.HasPrefix(line, "#") {
			if len(records) == 0 {
				header = append(header, line)
			}
			continue
		}
		records = append(records, line)
	}
	have := map[string]bool{}
	for _, r := range records {
		have[strings.Fields(r)[0]] = true
	}
	for _, e := range entries {
		if have[e.Tag] {
			return fmt.Errorf("%s: tag %s is already registered", path, e.Tag)
		}
		have[e.Tag] = true
		records = append(records, e.String())
	}
	slices.Sort(records)
	var b strings.Builder
	for _, line := range append(header, records...) {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0o644) // #nosec G306 -- registers are public text in a git tree.
}
