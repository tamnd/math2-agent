package node

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Order gives the key order of the canonical form. The key of the map is a path, and the value is the keys at that path in order. The top level has the path "". A nested mapping has the keys above it joined with dots. A list does not add to the path. Thus the keys of each item of formal have the path "formal".
type Order map[string][]string

// ParseOrder reads the key order from a node schema. The top level uses the x-m2-order keyword. A nested mapping uses the order of the properties in the schema.
func ParseOrder(schema []byte) (Order, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(schema, &root); err != nil {
		return nil, fmt.Errorf("read the schema: %w", err)
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("the schema is not an object")
	}
	doc := root.Content[0]
	o := Order{}
	top := get(doc, "x-m2-order")
	if top == nil || top.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("the schema has no x-m2-order list")
	}
	for _, k := range top.Content {
		o[""] = append(o[""], k.Value)
	}
	w := orderWalker{doc: doc, order: o}
	w.walk("", doc, 0)
	return o, nil
}

type orderWalker struct {
	doc   *yaml.Node
	order Order
}

// walk adds the property order of the schema s to the path p. It follows $ref, items, then, else and the combining keywords.
func (w orderWalker) walk(p string, s *yaml.Node, depth int) {
	if s == nil || s.Kind != yaml.MappingNode || depth > 16 {
		return
	}
	if ref := get(s, "$ref"); ref != nil {
		if name, ok := strings.CutPrefix(ref.Value, "#/$defs/"); ok {
			w.walk(p, get(get(w.doc, "$defs"), name), depth+1)
		}
	}
	for _, key := range []string{"items", "then", "else"} {
		w.walk(p, get(s, key), depth+1)
	}
	for _, key := range []string{"anyOf", "oneOf", "allOf"} {
		if list := get(s, key); list != nil {
			for _, c := range list.Content {
				w.walk(p, c, depth+1)
			}
		}
	}
	props := get(s, "properties")
	if props == nil {
		return
	}
	for i := 0; i+1 < len(props.Content); i += 2 {
		k := props.Content[i].Value
		if !slices.Contains(w.order[p], k) {
			w.order[p] = append(w.order[p], k)
		}
		sub := k
		if p != "" {
			sub = p + "." + k
		}
		w.walk(sub, props.Content[i+1], depth+1)
	}
}

// get returns the value of key in the mapping m, or nil.
func get(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// Format returns the node file in canonical form:
//
//   - The front matter keys are in the order that o gives. Keys that o does not know come after them, in byte order.
//   - The indent is two spaces. Mappings use block style. A list of scalars uses flow style, for example [20D60, 20Dxx], unless an item has a comment. Other lists use block style.
//   - A scalar has no quotes, unless YAML needs them to keep its type. A null is null.
//   - Comments stay where the YAML parser puts them. Some comment layouts do not have one stable place. For such a file, Format removes all comments, because a second Format must give the same bytes.
//   - The body comes after one empty line. It keeps its bytes, but each line ends with LF, also a line that ends with a CR alone, and the file ends with one newline.
func Format(n *Node, o Order) []byte {
	want := Hash(n)
	cur := format(n, o, false)
	for range 4 {
		m, err := Parse(n.Path, cur)
		if err != nil || Hash(m) != want {
			break
		}
		next := format(m, o, false)
		if bytes.Equal(next, cur) {
			return cur
		}
		cur = next
	}
	return format(n, o, true)
}

// format writes the node in canonical form. When bare is true, it removes the comments.
func format(n *Node, o Order, bare bool) []byte {
	front := canonical(n.Front, o, "", style{bare: bare})
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(encode(front))
	buf.WriteString("---\n")
	if body := strings.Trim(lf(n.Body), "\n"); body != "" {
		buf.WriteString("\n" + body + "\n")
	}
	return buf.Bytes()
}

// lf changes each CR line ending to LF.
func lf(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

// style gives the choices of canonical. When bare is true, canonical removes the comments. When block is true, each list uses block style.
type style struct{ bare, block bool }

// canonical returns a copy of n in canonical form.
func canonical(n *yaml.Node, o Order, p string, st style) *yaml.Node {
	c := *n
	c.Content = nil
	if st.bare {
		c.HeadComment, c.LineComment, c.FootComment = "", "", ""
	}
	switch n.Kind {
	case yaml.DocumentNode:
		c.Content = []*yaml.Node{canonical(n.Content[0], o, p, st)}
	case yaml.MappingNode:
		c.Style = 0
		type pair struct{ k, v *yaml.Node }
		pairs := make([]pair, 0, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			sub := k.Value
			if p != "" {
				sub = p + "." + k.Value
			}
			pairs = append(pairs, pair{canonical(k, o, p, st), canonical(n.Content[i+1], o, sub, st)})
		}
		known := o[p]
		rank := func(k string) int {
			if i := slices.Index(known, k); i >= 0 {
				return i
			}
			return len(known)
		}
		slices.SortStableFunc(pairs, func(a, b pair) int {
			if r := cmp.Compare(rank(a.k.Value), rank(b.k.Value)); r != 0 {
				return r
			}
			return strings.Compare(a.k.Value, b.k.Value)
		})
		for _, kv := range pairs {
			c.Content = append(c.Content, kv.k, kv.v)
		}
	case yaml.SequenceNode:
		flow := !st.block
		for _, item := range n.Content {
			ci := canonical(item, o, p, st)
			if ci.Kind != yaml.ScalarNode || ci.HeadComment != "" || ci.LineComment != "" || ci.FootComment != "" {
				flow = false
			}
			c.Content = append(c.Content, ci)
		}
		c.Style = 0
		if flow {
			c.Style = yaml.FlowStyle
		}
	case yaml.ScalarNode:
		c.Style = 0
		switch c.Tag {
		case "!!null":
			c.Value = "null"
		case "!!bool":
			c.Value = strings.ToLower(c.Value)
		}
	}
	return &c
}

// encode writes a YAML document with a two-space indent.
func encode(doc *yaml.Node) []byte {
	var buf bytes.Buffer
	e := yaml.NewEncoder(&buf)
	e.SetIndent(2)
	if err := e.Encode(doc); err != nil {
		// The document comes from the parser and has no anchors, so the encoder cannot fail on it.
		panic(fmt.Sprintf("node: encode the front matter: %v", err))
	}
	_ = e.Close()
	return buf.Bytes()
}

// Canonical reports whether data is a node file in canonical form.
func Canonical(path string, data []byte, o Order) (bool, error) {
	n, err := Parse(path, data)
	if err != nil {
		return false, err
	}
	return bytes.Equal(Format(n, o), data), nil
}
