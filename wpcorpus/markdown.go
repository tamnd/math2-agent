package wpcorpus

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The files of the analysis plane are Markdown with a YAML header (WP12.1),
// so that a person can read them on GitHub and a program can parse them.

const frontMatter = "---\n"

// objectsHeader is the YAML header of an objects file.
type objectsHeader struct {
	Page     int      `yaml:"page"`
	Revision int      `yaml:"revision"`
	Title    string   `yaml:"title,omitempty"`
	Source   string   `yaml:"source"`
	Licence  string   `yaml:"licence"`
	Objects  []Object `yaml:"objects"`
}

const licence = "CC BY-SA 4.0 (https://creativecommons.org/licenses/by-sa/4.0/)"

// splitFrontMatter gives the YAML header and the body of a Markdown file.
func splitFrontMatter(data []byte) (head, body []byte, err error) {
	if !bytes.HasPrefix(data, []byte(frontMatter)) {
		return nil, nil, errors.New("the file has no YAML header")
	}
	rest := data[len(frontMatter):]
	i := bytes.Index(rest, []byte("\n"+frontMatter))
	if i < 0 {
		return nil, nil, errors.New("the YAML header does not close")
	}
	return rest[:i+1], rest[i+1+len(frontMatter):], nil
}

func writeFrontMatter(b *bytes.Buffer, v any) error {
	b.WriteString(frontMatter)
	enc := yaml.NewEncoder(b)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	b.WriteString(frontMatter)
	return nil
}

// objectHeading gives the heading of the section of an object in the body.
// The parser finds the section by the ref, so the rest of the heading is only
// for the reader.
func objectHeading(o *Object) string {
	h := "## " + o.Ref + " " + o.Kind
	if o.Name != "" {
		h += ": " + o.Name
	}
	return h
}

// MarshalObjects gives the objects file of a page revision. The header holds
// the fields of each object, and the body holds the text of each object
// under a heading with its ref.
func MarshalObjects(page, rev int, title string, objs []Object) ([]byte, error) {
	var b bytes.Buffer
	h := objectsHeader{Page: page, Revision: rev, Title: title, Source: "https://en.wikipedia.org/w/index.php?oldid=" + strconv.Itoa(rev), Licence: licence, Objects: objs}
	if err := writeFrontMatter(&b, h); err != nil {
		return nil, err
	}
	if title != "" {
		fmt.Fprintf(&b, "\n# %s\n", title)
	}
	for i := range objs {
		// ParseObjects ends a text at the heading of the next object, so a
		// text must not have a line that looks like that heading.
		if i+1 < len(objs) && isHeadingOf("\n"+objs[i].Text, objs[i+1].Ref) {
			return nil, fmt.Errorf("the text of the object %s has a line that starts with the heading of the object %s", objs[i].Ref, objs[i+1].Ref)
		}
		fmt.Fprintf(&b, "\n%s\n", objectHeading(&objs[i]))
		if objs[i].Text != "" {
			b.WriteString("\n" + objs[i].Text + "\n")
		}
	}
	return b.Bytes(), nil
}

func isHeading(line, prefix string) bool {
	return line == prefix || strings.HasPrefix(line, prefix+" ")
}

// isHeadingOf tells if a line of the text is the heading of the object ref.
func isHeadingOf(text, ref string) bool {
	for _, l := range strings.Split(text, "\n") {
		if isHeading(l, "## "+ref) {
			return true
		}
	}
	return false
}

// ParseObjects reads an objects file that MarshalObjects wrote.
func ParseObjects(data []byte) ([]Object, error) {
	head, body, err := splitFrontMatter(data)
	if err != nil {
		return nil, err
	}
	var h objectsHeader
	if err := yaml.Unmarshal(head, &h); err != nil {
		return nil, err
	}
	lines := strings.SplitAfter(string(body), "\n")
	// starts[i] is the line of the heading of object i.
	starts := make([]int, len(h.Objects))
	at := 0
	for i := range h.Objects {
		o := &h.Objects[i]
		o.Page, o.Revision = h.Page, h.Revision
		prefix := "## " + o.Ref
		for ; at < len(lines); at++ {
			if isHeading(strings.TrimSuffix(lines[at], "\n"), prefix) {
				break
			}
		}
		if at == len(lines) {
			return nil, fmt.Errorf("the body has no section for the object %s", o.Ref)
		}
		starts[i] = at
		at++
	}
	for i := range h.Objects {
		end := len(lines)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		// The section is the heading, a blank line, the text and a blank line
		// before the next heading.
		text := strings.Join(lines[min(starts[i]+2, end):end], "")
		if i+1 < len(starts) {
			text = strings.TrimSuffix(text, "\n")
		}
		h.Objects[i].Text = strings.TrimSuffix(text, "\n")
	}
	return h.Objects, nil
}

// MarshalCensus gives the census file of a page revision. The header holds
// the census, and the body is a summary for the reader.
func MarshalCensus(c *Census) ([]byte, error) {
	var b bytes.Buffer
	if err := writeFrontMatter(&b, c); err != nil {
		return nil, err
	}
	fmt.Fprintf(&b, "\n# Census of %s\n\n", c.Title)
	fmt.Fprintf(&b, "The revision %d is of the type %s, and the strength of its cues is %s. It has %d cues, and %d of them have no object and no waiver.\n", c.Revision, c.Type, c.Strength, len(c.Cues), len(c.Gaps()))
	kinds := map[string]int{}
	for _, q := range c.Cues {
		if q.Object != "" {
			kinds[q.Kind]++
		}
	}
	if len(kinds) > 0 {
		names := make([]string, 0, len(kinds))
		for k := range kinds {
			names = append(names, k)
		}
		sort.Strings(names)
		b.WriteString("\n| Kind | Objects |\n|---|---:|\n")
		for _, k := range names {
			fmt.Fprintf(&b, "| %s | %d |\n", k, kinds[k])
		}
	}
	return b.Bytes(), nil
}

// ParseCensus reads a census file that MarshalCensus wrote. It reads only the
// header.
func ParseCensus(data []byte) (*Census, error) {
	head, _, err := splitFrontMatter(data)
	if err != nil {
		return nil, err
	}
	var c Census
	if err := yaml.Unmarshal(head, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
