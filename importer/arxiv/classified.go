package arxiv

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// ClassifyDir is the directory of the records of the pass classify (AX7.2) in a math2 tree, relative to its root. The role understand writes them, one shard file for each shard of the mentions.
const ClassifyDir = "records/understand/arxiv/classify"

// Classified gives the item kind of AX1.3 that the pass classify gives an item, by the id of its mention, as ax://paper/2401.00001#R1.
type Classified map[string]string

// ReadClassified reads the records of the pass classify of the math2 tree at root. It keeps a classification only when it is not in the queue classify-review, so a classification with a confidence below 0.7 waits for a person. When an item has two classifications, the later line wins. A tree with no records gives none and no error.
func ReadClassified(root string) (Classified, error) {
	files, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(ClassifyDir), "*.jsonl"))
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
	out := Classified{}
	for _, f := range files {
		if err := readClassified(f, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func readClassified(file string, out Classified) error {
	fh, err := os.Open(file) // #nosec G304 -- the file is in the tree that the caller names.
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer fh.Close()
	sc := bufio.NewScanner(fh)
	sc.Buffer(nil, 1<<20)
	for n := 1; sc.Scan(); n++ {
		var r struct {
			ID    string `json:"id"`
			Pass  string `json:"pass"`
			Kind  string `json:"kind"`
			Queue string `json:"queue"`
		}
		if len(sc.Bytes()) == 0 {
			continue
		}
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return fmt.Errorf("%s:%d: %w", file, n, err)
		}
		if r.ID != "" && r.Kind != "" && r.Queue == "" && (r.Pass == "" || r.Pass == "classify") {
			out[r.ID] = r.Kind
		}
	}
	return sc.Err()
}

// classify gives an object that code leaves of the kind unknown the item kind of its classification.
func (c Classified) classify(o *Object) {
	if o.Item != "unknown" || o.Tag == "" {
		return
	}
	if it, ok := c[MentionID(o.Paper, o.Tag)]; ok {
		o.Item = it
	}
}
