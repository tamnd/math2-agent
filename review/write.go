package review

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/schema"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/tag"
)

// File returns the path of the review file of the node with the tag, from the tree root: reviews/, the first two characters of the tag, and the tag with .yaml.
func File(tag string) string {
	return "reviews/" + tag[:2] + "/" + tag + ".yaml"
}

// Write adds the review r of the binding b of the packet p to the tree in root. It fills the review as Fill does, gives it the next id of the node, adds it at the end of the review file and adds the id to fidelity_reviews of the binding in the node file. Write returns the review and the paths of the two files, from the tree root. When a step fails, Write puts back the files as they were.
func Write(root string, p Packet, b Binding, r status.Review) (status.Review, []string, error) {
	if !tag.IsTag(p.Tag) {
		return r, nil, fmt.Errorf("review: %s has no tag yet. Review it after m2 mint gives it a tag", p.Tag)
	}
	r, err := Fill(p, b, r)
	if err != nil {
		return r, nil, err
	}
	all, err := status.LoadReviews(root)
	if err != nil {
		return r, nil, err
	}
	r.ID = nextID(p.Tag, all)

	nodes, err := node.Load(root)
	if err != nil {
		return r, nil, err
	}
	var n *node.Node
	for _, m := range nodes {
		if m.ID == p.Tag {
			n = m
		}
	}
	if n == nil {
		return r, nil, fmt.Errorf("review: no node has the tag %s", p.Tag)
	}
	if !addID(n, b.Decl, r.ID) {
		return r, nil, fmt.Errorf("review: %s has no Lean binding of %s", n.Path, b.Decl)
	}
	s, err := schema.Load(root)
	if err != nil {
		return r, nil, err
	}

	rel := File(p.Tag)
	path := filepath.Join(root, filepath.FromSlash(rel))
	old, err := os.ReadFile(path) // #nosec G304 -- the path is in the working tree, which the caller owns.
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return r, nil, err
	}
	data, err := appendReview(old, r)
	if err != nil {
		return r, nil, fmt.Errorf("review: %s: %w", rel, err)
	}
	nodePath := filepath.Join(root, filepath.FromSlash(n.Path))
	oldNode, err := os.ReadFile(nodePath) // #nosec G304 -- the path comes from node.Load.
	if err != nil {
		return r, nil, err
	}
	undo := func() {
		_ = os.WriteFile(nodePath, oldNode, 0o644) // #nosec G306 G703 -- a node file is part of the working tree, and its path comes from node.Load.
		if old == nil {
			_ = os.Remove(path)
		} else {
			_ = os.WriteFile(path, old, 0o644) // #nosec G306 G703 -- a review file is part of the working tree, and its name is a checked tag.
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return r, nil, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil { // #nosec G306 G703 -- a review file is part of the working tree, and its name is a checked tag.
		undo()
		return r, nil, err
	}
	if err := os.WriteFile(nodePath, node.Format(n, s.Order), 0o644); err != nil { // #nosec G306 G703 -- the node file is part of the working tree, and its path comes from node.Load.
		undo()
		return r, nil, err
	}
	if got, err := status.LoadReviews(root); err != nil || got[r.ID].Verdict != r.Verdict {
		undo()
		return r, nil, fmt.Errorf("review: the review file does not read back: %w", err)
	}
	return r, []string{rel, n.Path}, nil
}

// nextID returns the id after the last review of the node with the tag: r-, the tag, - and a number of two digits or more.
func nextID(tag string, all map[string]status.Review) string {
	prefix := "r-" + tag + "-"
	last := 0
	for id := range all {
		if k, err := strconv.Atoi(strings.TrimPrefix(id, prefix)); err == nil && strings.HasPrefix(id, prefix) && k > last {
			last = k
		}
	}
	return fmt.Sprintf("%s%02d", prefix, last+1)
}

// appendReview adds r at the end of the list of a review file and keeps the text before it. A file with no content gets the key reviews.
func appendReview(old []byte, r status.Review) ([]byte, error) {
	item, err := yaml.Marshal([]status.Review{r})
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if len(bytes.TrimSpace(old)) == 0 {
		buf.WriteString("reviews:\n")
	} else {
		var file struct {
			Reviews []status.Review `yaml:"reviews"`
		}
		if err := yaml.Unmarshal(old, &file); err != nil {
			return nil, err
		}
		if len(file.Reviews) == 0 {
			return nil, errors.New("the file has no list of reviews")
		}
		buf.Write(old)
		if !bytes.HasSuffix(old, []byte("\n")) {
			buf.WriteByte('\n')
		}
	}
	for line := range strings.Lines(string(item)) {
		buf.WriteString("  " + line)
	}
	return buf.Bytes(), nil
}

// addID adds the review id to fidelity_reviews of the Lean binding of decl in the front matter of n. It tells if it found the binding.
func addID(n *node.Node, decl, id string) bool {
	if n.Front == nil || len(n.Front.Content) == 0 || n.Front.Content[0].Kind != yaml.MappingNode {
		return false
	}
	front := n.Front.Content[0]
	for i := 0; i+1 < len(front.Content); i += 2 {
		if front.Content[i].Value != "formal" || front.Content[i+1].Kind != yaml.SequenceNode {
			continue
		}
		for _, b := range front.Content[i+1].Content {
			if b.Kind != yaml.MappingNode || value(b, "system") != "lean4" || value(b, "decl") != decl {
				continue
			}
			for j := 0; j+1 < len(b.Content); j += 2 {
				if b.Content[j].Value == "fidelity_reviews" && b.Content[j+1].Kind == yaml.SequenceNode {
					b.Content[j+1].Content = append(b.Content[j+1].Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: id})
					return true
				}
			}
			b.Content = append(b.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "fidelity_reviews"},
				&yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: id}}})
			return true
		}
	}
	return false
}

// value returns the value of a key of a mapping, or empty.
func value(m *yaml.Node, key string) string {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1].Value
		}
	}
	return ""
}
