package align

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/zeebo/blake3"
	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/schema"
)

// Register is the path of the register of crosswalk decisions in a math2 working tree. Each line is one decision of a person on one edge:
//
//	tag edge system id decision person date record
//
// The id is escaped as a part of a URL path, thus it has no space. The decision is accepted, rejected or confirmed. The record is the hash of the record at the time of the decision, or - when the tree had no records of the system. The lines are in the order of their tags. A line is never changed or removed. A later line on the same edge is the newer decision.
const Register = "registers/crosswalk.reg"

// header is the first line of the register.
const header = "# tag edge system id decision person date record"

// The decisions in the register.
const (
	Accepted  = "accepted"  // a person accepted a proposal, and m2 wrote the edge
	Rejected  = "rejected"  // a person rejected a proposal. The aligner does not propose the edge again.
	Confirmed = "confirmed" // a person confirmed an edge that was in the node already
)

// A Decision is one line of the register.
type Decision struct {
	Tag      string // the node
	Edge     string // SameAs, CloseTo, Aligned or Duplicate
	System   string
	ID       string // the target of the edge, not escaped
	Decision string // Accepted, Rejected or Confirmed
	Person   string // orcid: and an ORCID iD, or github: and a login
	Date     string // YYYY-MM-DD
	Record   string // the hash of the record, or -
}

// String returns the decision as one line of the register.
func (d Decision) String() string {
	return strings.Join([]string{d.Tag, d.Edge, d.System, url.PathEscape(d.ID), d.Decision, d.Person, d.Date, d.Record}, " ")
}

// ReadDecisions returns the decisions in the register of the working tree at root, in the order of the file: by tag, and for each tag in the order of the decisions. A tree with no register has no decisions.
func ReadDecisions(root string) ([]Decision, error) {
	path := filepath.Join(root, filepath.FromSlash(Register))
	data, err := os.ReadFile(path) // #nosec G304 -- the path is the register of the working tree.
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Decision
	n := 0
	for line := range strings.Lines(string(data)) {
		n++
		f := strings.Fields(line)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		if len(f) != 8 {
			return nil, fmt.Errorf("%s:%d: the line has %d fields, and a decision has 8", Register, n, len(f))
		}
		id, err := url.PathUnescape(f[3])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", Register, n, err)
		}
		out = append(out, Decision{f[0], f[1], f[2], id, f[4], f[5], f[6], f[7]})
	}
	return out, nil
}

// appendDecision adds a line to the register. The lines are in the order of their tags, as in the other registers, and the lines of one tag are in the order of the decisions. Thus the new line goes after the last line whose tag is not greater than its tag. appendDecision makes the register with its header when it does not exist.
func appendDecision(root string, d Decision) error {
	path := filepath.Join(root, filepath.FromSlash(Register))
	data, err := os.ReadFile(path) // #nosec G304 -- the path is the register of the working tree.
	if errors.Is(err, fs.ErrNotExist) {
		data = []byte(header + "\n")
	} else if err != nil {
		return err
	}
	lines := slices.Collect(strings.Lines(string(data)))
	at := len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		f := strings.Fields(lines[i])
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		if f[0] <= d.Tag {
			break
		}
		at = i
	}
	if at > 0 && !strings.HasSuffix(lines[at-1], "\n") {
		lines[at-1] += "\n"
	}
	lines = slices.Insert(lines, at, d.String()+"\n")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- the register is part of the working tree, which others read.
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "")), 0o644) // #nosec G306 G703 -- the path is the register of the working tree, which others read.
}

// A Review is what a person gives with a decision.
type Review struct {
	Person string // orcid: and an ORCID iD, or github: and a GitHub login. An agent cannot decide.
	Date   string // YYYY-MM-DD
	Note   string // how the node and the record differ. A close_to edge needs it.
	Match  string // how the paper states the node: exact, stronger, weaker, variant or partial. An aligned edge needs it.
}

var (
	orcid = regexp.MustCompile(`^orcid:[0-9]{4}-[0-9]{4}-[0-9]{4}-[0-9]{3}[0-9X]$`)
	login = regexp.MustCompile(`^github:[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)
	date  = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
)

// check reports whether the review names a person and a date.
func (r Review) check() error {
	if !orcid.MatchString(r.Person) && !login.MatchString(r.Person) {
		return fmt.Errorf("align: %q is not a person. Give orcid: and an ORCID iD, or github: and a GitHub login. An agent cannot decide on an edge", r.Person)
	}
	if !date.MatchString(r.Date) {
		return fmt.Errorf("align: the date %q is not YYYY-MM-DD", r.Date)
	}
	return nil
}

// Accept writes the edge of the proposal with the id into the front matter of its node, records the decision in the register and takes the proposal out of the queue. A Duplicate changes no node: the person merges the two nodes or keeps them apart in a later change.
func Accept(root, id string, r Review) (Proposal, error) {
	p, err := find(root, id)
	if err != nil {
		return p, err
	}
	if err := r.check(); err != nil {
		return p, err
	}
	switch p.Edge {
	case CloseTo:
		if strings.TrimSpace(r.Note) == "" {
			return p, errors.New("align: a close_to edge needs a note on how the node and the record differ")
		}
	case Aligned:
		if !slices.Contains([]string{"exact", "stronger", "weaker", "variant", "partial"}, r.Match) {
			return p, fmt.Errorf("align: an aligned edge needs a match of exact, stronger, weaker, variant or partial, and %q is not one", r.Match)
		}
	}
	if p.Edge != Duplicate {
		if err := edit(root, p, r); err != nil {
			return p, err
		}
	}
	return p, decide(root, p, Accepted, r)
}

// Reject records that a person rejected the proposal with the id, and takes it out of the queue. The aligner does not propose an edge from the node to the target again.
func Reject(root, id string, r Review) (Proposal, error) {
	p, err := find(root, id)
	if err != nil {
		return p, err
	}
	if err := r.check(); err != nil {
		return p, err
	}
	return p, decide(root, p, Rejected, r)
}

// Confirm records that a person checked the same_as value of the node in the system. It is for a value that a person or an agent wrote in the node without the queue. Rule A02 then stops reporting it, and rule A03 compares the record with the one at this date.
func Confirm(root, tag, system string, r Review) error {
	if err := r.check(); err != nil {
		return err
	}
	nodes, err := node.Load(root)
	if err != nil {
		return err
	}
	for _, n := range nodes {
		if n.ID != tag {
			continue
		}
		v, ok := SameAsOf(n)[system]
		if !ok || v == "" {
			return fmt.Errorf("align: %s has no same_as value in %s", tag, system)
		}
		return appendDecision(root, Decision{tag, SameAs, system, v, Confirmed, r.Person, r.Date, recordHash(root, system, v)})
	}
	return fmt.Errorf("align: no node has the tag %s", tag)
}

// decide records the decision and takes the proposal out of its queue.
func decide(root string, p Proposal, decision string, r Review) error {
	rec := "-"
	if p.Edge != Duplicate {
		rec = recordHash(root, p.Source, p.Target)
	}
	if err := appendDecision(root, Decision{p.Node, p.Edge, p.System, p.Target, decision, r.Person, r.Date, rec}); err != nil {
		return err
	}
	return remove(root, p)
}

// find returns the proposal with the id.
func find(root, id string) (Proposal, error) {
	ps, err := ReadQueue(root)
	if err != nil {
		return Proposal{}, err
	}
	for _, p := range ps {
		if p.ID == id {
			return p, nil
		}
	}
	return Proposal{}, fmt.Errorf("align: the queue has no proposal %s", id)
}

// RecordHash returns the hash of what the record says: its kind, its title and its text. Rule A03 compares it with the hash in the register.
func RecordHash(r ingest.Record) string {
	sum := blake3.Sum256([]byte(r.Kind + "\x00" + r.Title + "\x00" + r.Text))
	return "blake3:" + hex.EncodeToString(sum[:8])
}

// recordHash returns the hash of the record with the id in the records of the source, or - when the tree has no such record. The id of an aligned edge is a URI, and the record is the one with that locator or URL.
func recordHash(root, source, id string) string {
	recs, _, err := ingest.ReadRecords(root, source)
	if err != nil {
		return "-"
	}
	for _, r := range recs {
		if r.ID == id || r.Locator == id || r.URL == id {
			return RecordHash(r)
		}
	}
	return "-"
}

// SameAsOf returns the same_as values of the node by system. A null value, which says that the system has no entry, is the empty string.
func SameAsOf(n *node.Node) map[string]string {
	out := map[string]string{}
	m, _ := n.Meta[SameAs].(map[string]any)
	for k, v := range m {
		s, _ := v.(string)
		out[k] = s
	}
	return out
}

// edit writes the edge of the proposal into its node file, in the canonical form.
func edit(root string, p Proposal, r Review) error {
	if p.Edge != Aligned {
		if err := crosswalk.Check(p.System, p.Target); err != nil {
			return fmt.Errorf("align: %w", err)
		}
	}
	nodes, err := node.Load(root)
	if err != nil {
		return err
	}
	var n *node.Node
	for _, m := range nodes {
		if m.ID == p.Node {
			n = m
		}
	}
	if n == nil {
		return fmt.Errorf("align: no node has the tag %s", p.Node)
	}
	if n.Front == nil || len(n.Front.Content) == 0 || n.Front.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("align: %s has no front matter", n.Path)
	}
	front := n.Front.Content[0]
	switch p.Edge {
	case SameAs:
		m := value(front, SameAs, yaml.MappingNode)
		if v := value(m, p.System, 0); v != nil {
			if v.Tag != "!!null" && v.Value != p.Target {
				return fmt.Errorf("align: %s has the same_as value %s in %s already", p.Node, v.Value, p.System)
			}
			*v = *str(p.Target)
		} else {
			m.Content = append(m.Content, str(p.System), str(p.Target))
		}
	case CloseTo:
		seq := value(front, CloseTo, yaml.SequenceNode)
		seq.Content = append(seq.Content, mapping("system", p.System, "id", p.Target, "note", strings.TrimSpace(r.Note)))
	case Aligned:
		seq := value(front, Aligned, yaml.SequenceNode)
		seq.Content = append(seq.Content, mapping("record", p.Target, "match", r.Match))
	default:
		return fmt.Errorf("align: the edge %q is not known", p.Edge)
	}
	s, err := schema.Load(root)
	if err != nil {
		return err
	}
	path := filepath.Join(root, filepath.FromSlash(n.Path))
	return os.WriteFile(path, node.Format(n, s.Order), 0o644) // #nosec G306 G703 -- the node file is part of the working tree, and its path comes from node.Load.
}

// value returns the value of the key in the mapping. With a kind, it adds the key with an empty value of that kind when the mapping does not have it, and it replaces a null value.
func value(m *yaml.Node, key string, kind yaml.Kind) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			v := m.Content[i+1]
			if kind != 0 && v.Tag == "!!null" {
				*v = yaml.Node{Kind: kind}
			}
			return v
		}
	}
	if kind == 0 {
		return nil
	}
	v := &yaml.Node{Kind: kind}
	m.Content = append(m.Content, str(key), v)
	return v
}

func str(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s} }

func mapping(kv ...string) *yaml.Node {
	m := &yaml.Node{Kind: yaml.MappingNode}
	for _, s := range kv {
		m.Content = append(m.Content, str(s))
	}
	return m
}
