package pack

import (
	"bytes"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/graph"
)

// Lock is the lock file of a volume, volumes/<id>.lock (corpus K11.1). It pins each node of an edition to a release and to its content, so that an edition builds the same output again.
type Lock struct {
	Volume      string            `yaml:"volume"`
	Release     string            `yaml:"release"`
	Nodes       []LockNode        `yaml:"nodes"`
	Choose      map[string]string `yaml:"choose,omitempty"`      // the definition of each concept with two or more, as Pack chose it
	Conventions map[string]string `yaml:"conventions,omitempty"` // the option of each convention that the volume depends on
	Notation    map[string]string `yaml:"notation,omitempty"`    // the variant of each register entry with a variant in the volume
}

// LockNode is one node of the edition.
type LockNode struct {
	Node   string `yaml:"node"`             // TAG@release
	Hash   string `yaml:"hash"`             // the hash of the node file
	Number string `yaml:"number,omitempty"` // the number in the volume. A proof has none
	Label  string `yaml:"label"`
}

// LockFile returns the path of the lock file of the volume with the id, as a slash path in the working tree.
func LockFile(id string) string { return Dir + "/" + id + ".lock" }

const lockHeader = "# The lock file of a math2 volume. m2 pack writes it. Do not edit it.\n"

// Lock returns the lock of the document.
func (d *Document) Lock() *Lock {
	l := &Lock{Volume: d.Volume.ID, Release: d.Release}
	for _, it := range d.Items() {
		gn, _ := d.tree.graph.Node(it.Tag)
		l.Nodes = append(l.Nodes, LockNode{Node: it.Tag + "@" + d.Release, Hash: gn.Hash, Number: it.Number, Label: it.Label})
	}
	add := func(m *map[string]string, k, v string) {
		if *m == nil {
			*m = map[string]string{}
		}
		(*m)[k] = v
	}
	for c, def := range d.chosen() {
		add(&l.Choose, c, def)
	}
	for _, c := range d.Conventions {
		add(&l.Conventions, c.ID, c.Option)
	}
	for _, n := range d.Notation {
		if n.Variant != "" {
			add(&l.Notation, n.ID, n.Variant)
		}
	}
	return l
}

// chosen returns the definition that the volume shows for each concept with two or more definitions.
func (d *Document) chosen() map[string]string {
	out := map[string]string{}
	for _, it := range d.Items() {
		if it.Concept != "" && len(d.definitionsOf(it.Concept)) > 1 {
			out[it.Concept] = it.Tag
		}
	}
	return out
}

func (d *Document) definitionsOf(concept string) []string {
	var out []string
	for _, e := range d.tree.graph.In(graph.Defines, concept) {
		out = append(out, e.From)
	}
	return out
}

// Marshal returns the lock file. It is the same each time for the same lock.
func (l *Lock) Marshal() ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(lockHeader)
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(l); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// ReadLock reads the lock file of the volume with the id. It returns nil and no error when the volume has no lock file.
func ReadLock(root, id string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(LockFile(id)))) // #nosec G304 -- the path is in the working tree.
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}
