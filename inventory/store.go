package inventory

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// WriteMentions writes the mentions of one source to records/mentions/<source>/ in the tree at root. It replaces each file of the source, and it removes the shards that have no mention now. Each mention must be of the source.
func WriteMentions(root, source string, ms []Mention) error {
	shards := map[string][]Mention{}
	for _, m := range ms {
		if m.Source != source {
			return fmt.Errorf("inventory: the mention %s is of %s, not of %s", m.ID, m.Source, source)
		}
		k := MentionShard(m.ID)
		shards[k] = append(shards[k], m)
	}
	dir := filepath.Join(root, MentionsDir, source)
	files := map[string][]byte{}
	for k, part := range shards {
		sortMentions(part)
		data, err := encodeLines(part)
		if err != nil {
			return err
		}
		files[k+".jsonl"] = data
	}
	return replaceDir(dir, files)
}

// ReadMentions reads the mentions of each source from records/mentions/ in the tree at root, in the order of their identifiers. A tree with no mentions gives no error.
func ReadMentions(root string) ([]Mention, error) {
	var out []Mention
	err := readLines(filepath.Join(root, MentionsDir), func(path string, line []byte) error {
		var m Mention
		if err := json.Unmarshal(line, &m); err != nil {
			return err
		}
		out = append(out, m)
		return nil
	})
	sortMentions(out)
	return out, err
}

// WriteRows writes the rows to records/inventory/ in the tree at root. It replaces each file, and it removes the shards that have no row now.
func WriteRows(root string, rows []Row) error {
	shards := map[string][]Row{}
	for _, r := range rows {
		k := RowShard(r.ID)
		shards[k] = append(shards[k], r)
	}
	files := map[string][]byte{}
	for k, part := range shards {
		sortRows(part)
		data, err := encodeLines(part)
		if err != nil {
			return err
		}
		files[k+".jsonl"] = data
	}
	return replaceDir(filepath.Join(root, RowsDir), files)
}

// ReadRows reads the rows from records/inventory/ in the tree at root, in the order of their identifiers. A tree with no rows gives no error.
func ReadRows(root string) ([]Row, error) {
	var out []Row
	err := readLines(filepath.Join(root, RowsDir), func(path string, line []byte) error {
		var r Row
		if err := json.Unmarshal(line, &r); err != nil {
			return err
		}
		out = append(out, r)
		return nil
	})
	sortRows(out)
	return out, err
}

// ReadRegister reads the row identifiers of registers/inventory/ in the tree at root.
func ReadRegister(root string) (map[string]bool, error) {
	out := map[string]bool{}
	err := walkFiles(filepath.Join(root, RegisterDir), ".reg", func(path string, data []byte) error {
		for n, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(line)
			if len(f) == 0 || strings.HasPrefix(f[0], "#") {
				continue
			}
			if len(f) != 2 {
				return fmt.Errorf("%s:%d: want an identifier and a commit", path, n+1)
			}
			out[f[0]] = true
		}
		return nil
	})
	return out, err
}

// AppendRegister adds the row identifiers to registers/inventory/ in the tree at root, each with the commit that adds it. A register never loses a line, so AppendRegister fails on an identifier that the register has.
func AppendRegister(root string, ids []string, commit string) error {
	have, err := ReadRegister(root)
	if err != nil {
		return err
	}
	shards := map[string][]string{}
	for _, id := range ids {
		if have[id] {
			return fmt.Errorf("inventory: the register has %s", id)
		}
		have[id] = true
		k := RowShard(id)
		shards[k] = append(shards[k], id+" "+commit)
	}
	for k, lines := range shards {
		path := filepath.Join(root, RegisterDir, k+".reg")
		old, err := os.ReadFile(path) // #nosec G304 -- the caller owns the tree.
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if len(old) == 0 {
			old = []byte("# id commit\n")
		}
		all := append(strings.Split(strings.TrimSuffix(string(old), "\n"), "\n"), lines...)
		head, body := all[:1], all[1:]
		slices.Sort(body)
		if err := writeFile(path, []byte(strings.Join(append(head, body...), "\n")+"\n")); err != nil {
			return err
		}
	}
	return nil
}

// encodeLines writes each value as one JSON line.
func encodeLines[T any](vs []T) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, v := range vs {
		if err := enc.Encode(v); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// readLines calls fn for each line of each .jsonl file below dir.
func readLines(dir string, fn func(path string, line []byte) error) error {
	return walkFiles(dir, ".jsonl", func(path string, data []byte) error {
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(nil, 64<<20)
		for n := 1; sc.Scan(); n++ {
			if err := fn(path, sc.Bytes()); err != nil {
				return fmt.Errorf("%s:%d: %w", path, n, err)
			}
		}
		return sc.Err()
	})
}

// walkFiles calls fn for each file below dir with the extension, in lexical order. A missing dir gives no error.
func walkFiles(dir, ext string, fn func(path string, data []byte) error) error {
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ext {
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G304 -- the caller owns the tree.
		if err != nil {
			return err
		}
		return fn(path, data)
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// replaceDir makes dir hold the files and nothing else. It writes only the files that changed.
func replaceDir(dir string, files map[string][]byte) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	old, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range old {
		if _, keep := files[e.Name()]; !keep && !e.IsDir() {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	for name, data := range files {
		path := filepath.Join(dir, name)
		if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, data) { // #nosec G304 -- the caller owns the tree.
			continue
		}
		if err := writeFile(path, data); err != nil {
			return err
		}
	}
	return nil
}

// writeFile writes data to path through a temporary file, so that a reader never sees a part of the file.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil { // #nosec G302 -- the files of the tree are public.
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
