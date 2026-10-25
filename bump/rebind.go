package bump

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/schema"
)

// Register is the path of the register of the bindings, from the tree root.
const Register = "registers/bindings.reg"

// registerHeader is the first line of a new register of the bindings.
const registerHeader = "# tag system old new cause mathlib author\n"

// Rebind applies the breaks to the tree in root. For a deprecated or a renamed declaration, it writes the new declaration into the formal binding of the node, in the canonical form, and records a rename in registers/bindings.reg. For a refactored declaration, it records a refactor. mathlib is the new Mathlib commit. A rename or a refactor keeps the author of the binding, thus the records have no author. Rebind returns the number of node files that it changed.
func Rebind(root string, bs []Break, mathlib string) (int, error) {
	if mathlib == "" || strings.ContainsAny(mathlib, " \n") {
		return 0, fmt.Errorf("bump: the Mathlib commit %q is not valid", mathlib)
	}
	renames := map[string]map[string]string{} // the new declaration of each old declaration, by tag
	var lines []string
	for _, b := range bs {
		switch {
		case b.Kind.Mechanical():
			if renames[b.Tag] == nil {
				renames[b.Tag] = map[string]string{}
			}
			renames[b.Tag][b.Old] = b.New
			lines = append(lines, strings.Join([]string{b.Tag, "lean4", b.Old, b.New, "rename", mathlib, "-"}, " "))
		case b.Kind == Refactored:
			lines = append(lines, strings.Join([]string{b.Tag, "lean4", b.Old, b.Old, "refactor", mathlib, "-"}, " "))
		}
	}
	if len(lines) == 0 {
		return 0, nil
	}
	for _, l := range lines {
		if strings.Count(l, " ") != 6 {
			return 0, fmt.Errorf("bump: the record %q has a field with a space", l)
		}
	}
	changed := 0
	if len(renames) > 0 {
		s, err := schema.Load(root)
		if err != nil {
			return 0, err
		}
		nodes, err := node.Load(root)
		if err != nil {
			return 0, err
		}
		for _, n := range nodes {
			r := renames[n.ID]
			if r == nil || !rename(n, r) {
				continue
			}
			path := filepath.Join(root, filepath.FromSlash(n.Path))
			if err := os.WriteFile(path, node.Format(n, s.Order), 0o644); err != nil { // #nosec G306 G703 -- the node file is part of the working tree, and its path comes from node.Load.
				return changed, err
			}
			changed++
		}
	}
	return changed, appendRecords(root, lines)
}

// rename writes the new declarations into the Lean bindings of the node. It tells if it changed one.
func rename(n *node.Node, r map[string]string) bool {
	if n.Front == nil || len(n.Front.Content) == 0 || n.Front.Content[0].Kind != yaml.MappingNode {
		return false
	}
	front, changed := n.Front.Content[0], false
	for i := 0; i+1 < len(front.Content); i += 2 {
		if front.Content[i].Value != "formal" || front.Content[i+1].Kind != yaml.SequenceNode {
			continue
		}
		for _, b := range front.Content[i+1].Content {
			if b.Kind != yaml.MappingNode || scalar(b, "system") != "lean4" {
				continue
			}
			for j := 0; j+1 < len(b.Content); j += 2 {
				if v := b.Content[j+1]; b.Content[j].Value == "decl" && r[v.Value] != "" {
					v.Value, changed = r[v.Value], true
				}
			}
		}
	}
	return changed
}

// scalar returns the value of a key of a mapping, or empty.
func scalar(m *yaml.Node, key string) string {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1].Value
		}
	}
	return ""
}

// appendRecords adds the records to the register of the bindings. The register is sorted by tag (spec 06.4). A new record goes after the last record whose tag is not greater, thus the records of one tag stay in the order of time.
func appendRecords(root string, lines []string) error {
	path := filepath.Join(root, filepath.FromSlash(Register))
	data, err := os.ReadFile(path) // #nosec G304 -- the register is in the working tree.
	if errors.Is(err, fs.ErrNotExist) {
		data, err = []byte(registerHeader), os.MkdirAll(filepath.Dir(path), 0o750)
	}
	if err != nil {
		return err
	}
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		data = append(data, '\n')
	}
	all := strings.SplitAfter(string(data), "\n")
	all = all[:len(all)-1] // the empty string after the last newline
	for _, l := range lines {
		tag, _, _ := strings.Cut(l, " ")
		at := len(all)
		for at > 0 {
			prev := strings.TrimSpace(all[at-1])
			t, _, _ := strings.Cut(prev, " ")
			if prev == "" || strings.HasPrefix(prev, "#") || t <= tag {
				break
			}
			at--
		}
		all = append(all[:at], append([]string{l + "\n"}, all[at:]...)...)
	}
	return os.WriteFile(path, []byte(strings.Join(all, "")), 0o644) // #nosec G306 G703 -- the register is part of the working tree.
}
