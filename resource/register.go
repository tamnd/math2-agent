package resource

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Entry is one line of a register of resource ids: the id, the commit that added it and the kind of the work (RS3.1).
type Entry struct {
	ID     string
	Commit string
	Kind   string
}

// String returns the entry as one register line.
func (e Entry) String() string { return e.ID + " " + e.Commit + " " + e.Kind }

// RegisterLine is one entry of a register with its place in the tree.
type RegisterLine struct {
	Entry
	File string // relative to the root, with forward slashes
	Line int
}

// ReadRegister reads the entries of registers/resources/ in the tree at root, in the order of the files and their lines. A tree with no register gives no entry and no error. A line that does not have three fields is an error.
func ReadRegister(root string) ([]RegisterLine, error) {
	dir := filepath.Join(root, RegisterDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []RegisterLine
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".reg" {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(RegisterDir, e.Name()))
		data, err := os.ReadFile(filepath.Join(dir, e.Name())) // #nosec G304 -- the caller owns the tree.
		if err != nil {
			return nil, err
		}
		for n, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(line)
			if len(f) == 0 || strings.HasPrefix(f[0], "#") {
				continue
			}
			if len(f) != 3 {
				return nil, fmt.Errorf("%s:%d: want an id, a commit and a kind", rel, n+1)
			}
			out = append(out, RegisterLine{Entry{f[0], f[1], f[2]}, rel, n + 1})
		}
	}
	return out, nil
}

// Registered returns the set of the ids of registers/resources/ in the tree at root.
func Registered(root string) (map[string]bool, error) {
	ls, err := ReadRegister(root)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(ls))
	for _, l := range ls {
		out[l.ID] = true
	}
	return out, nil
}

// AppendRegister adds the entries to registers/resources/ in the tree at root. A register never loses or changes a line (K4.8), so AppendRegister fails on an id that the register has.
func AppendRegister(root string, es []Entry) error {
	have, err := Registered(root)
	if err != nil {
		return err
	}
	shards := map[string][]string{}
	for _, e := range es {
		if have[e.ID] {
			return fmt.Errorf("resource: the register has %s", e.ID)
		}
		if e.Commit == "" || e.Kind == "" || strings.ContainsAny(e.Commit+e.Kind, " \t\n") {
			return fmt.Errorf("resource: the entry of %s needs a commit and a kind with no space", e.ID)
		}
		have[e.ID] = true
		shards[Shard(e.ID)] = append(shards[Shard(e.ID)], e.String())
	}
	for k, add := range shards {
		path := filepath.Join(root, RegisterDir, k+".reg")
		old, err := os.ReadFile(path) // #nosec G304 -- the caller owns the tree.
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if len(old) == 0 {
			old = []byte("# id commit kind\n")
		}
		all := append(strings.Split(strings.TrimSuffix(string(old), "\n"), "\n"), add...)
		head, body := all[:1], all[1:]
		slices.Sort(body)
		if err := writeFile(path, []byte(strings.Join(append(head, body...), "\n")+"\n")); err != nil {
			return err
		}
	}
	return nil
}
