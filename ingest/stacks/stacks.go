// Package stacks is the ingest adapter of the Stacks Project, the open source text on algebraic geometry (spec 09.2). It gives one record for each tag in tags/tags whose label is in the LaTeX source.
//
// The Stacks Project is under the GNU Free Documentation License, so the source has the class gfdl (spec 09.3). math2 links to the tags and never copies the text. The adapter gives the LaTeX of each statement and its proof as the text of the record. The licence gate keeps only the fingerprints of the text and drops the text, and rule L02 uses the fingerprints to find Stacks text in nodes/.
//
// The bulk channel is the git archive of the master branch, and the catalogue records its commit. The adapter reads the chapters as text, one line after the other, as the style of the project writes them: a \label on the line after each \begin, \section and \subsection. The text of a statement is its body and the proof after it. The text of a chapter, a section or a subsection is its prose outside the statements and their proofs.
//
// The id of a record is the tag, for example 01H8. The kind is part, chapter, section, subsection, equation, item, or the environment of a statement: lemma, proposition, theorem, definition, remark, remarks, example, exercise or situation. The parts of the book come from chapters.tex, as the book script makes them. The title is the title of a part, a chapter, a section or a subsection, or the name of a statement in square brackets. The adapter does not compute the numbers of the book, such as Lemma 26.4.2. The URL is the page of the tag, and the locator is the file and the line at the commit of the snapshot. The meta of a record has these keys:
//
//	label    the full label, for example schemes-lemma-open-immersion
//	chapter  the title of the chapter, for example Schemes, when the item is in a chapter
//	section  the title of the section of the item, when the item is in a section
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/stacks"
package stacks

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/ingest"
)

// Home is the home of the Stacks Project. The page of a tag is Home/tag/<tag>.
const Home = "https://stacks.math.columbia.edu"

// Repository is the git repository of the Stacks Project.
const Repository = "https://github.com/stacks/stacks-project"

// Archive is the git archive of the master branch of the repository.
const Archive = Repository + "/archive/refs/heads/master.tar.gz"

// Tags is the path of the list of tags in the repository.
const Tags = "tags/tags"

// Chapters is the path of the list of the parts and the chapters of the book in the repository.
const Chapters = "chapters.tex"

// Adapter is the adapter of the Stacks Project.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of the Stacks Project.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "stacks",
		Name:        "The Stacks Project",
		Home:        Home,
		Licence:     "GFDL-1.2-or-later",
		Class:       ingest.GFDL,
		Attribution: "The Stacks Project Authors, The Stacks Project, " + Home + ", GNU Free Documentation License 1.2 or later",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "stacks/1" }

// Dump returns the git archive of the master branch.
func (Adapter) Dump() string { return Archive }

// Fetch gets the git archive at req.URL and sets req.Commit to its commit.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	return ingest.FetchArchive(ctx, req, w)
}

// Read reads a git archive of the repository and emits one record for each tag. Read fails when the archive has no list of tags, on a tag that does not pass crosswalk.Check, on a tag or a label that the list has two times, on a label that the LaTeX source does not have and on a list with no tags.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	items := map[string]*item{}
	var list []byte
	_, err := ingest.WalkArchive(r, func(name string, f io.Reader) error {
		switch {
		case name == Tags:
			b, err := io.ReadAll(f)
			list = b
			return err
		case name == Chapters:
			b, err := io.ReadAll(f)
			if err != nil {
				return err
			}
			for _, it := range parts(b) {
				items[it.label] = it
			}
		case path.Dir(name) == "." && path.Ext(name) == ".tex":
			its, err := scan(name, f)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			for _, it := range its {
				if first, dup := items[it.label]; dup {
					return fmt.Errorf("%s and %s both have the label %s", first.locator, it.locator, it.label)
				}
				items[it.label] = it
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if list == nil {
		return fmt.Errorf("the archive has no %s", Tags)
	}
	tags, err := readTags(list)
	if err != nil {
		return err
	}
	for _, t := range tags {
		it, ok := items[t.label]
		if !ok {
			return fmt.Errorf("%s: line %d: the LaTeX source has no label %s", Tags, t.line, t.label)
		}
		meta := map[string]string{"label": it.label}
		if it.chapter != "" {
			meta["chapter"] = it.chapter
		}
		if it.section != "" {
			meta["section"] = it.section
		}
		rec := ingest.Record{
			ID:      t.tag,
			Kind:    it.kind,
			Title:   it.title,
			URL:     Home + "/tag/" + t.tag,
			Locator: it.locator,
			Text:    strings.TrimSpace(it.text.String()),
			Meta:    meta,
		}
		if err := emit(rec); err != nil {
			return err
		}
	}
	return nil
}

// tag is one line of the list of tags.
type tag struct {
	tag, label string
	line       int
}

// readTags reads the list of tags. Each line is a tag and a full label with a comma between them. A line that starts with # is a comment.
func readTags(b []byte) ([]tag, error) {
	var out []tag
	seenTag, seenLabel := map[string]bool{}, map[string]bool{}
	for i, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		id, label, ok := strings.Cut(l, ",")
		if !ok || label == "" {
			return nil, fmt.Errorf("%s: line %d is not a tag and a label", Tags, i+1)
		}
		if err := crosswalk.Check("stacks", id); err != nil {
			return nil, fmt.Errorf("%s: line %d: %w", Tags, i+1, err)
		}
		if seenTag[id] || seenLabel[label] {
			return nil, fmt.Errorf("%s: line %d: the tag %s or the label %s is in the list two times", Tags, i+1, id, label)
		}
		seenTag[id], seenLabel[label] = true, true
		out = append(out, tag{tag: id, label: label, line: i + 1})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s has no tags", Tags)
	}
	return out, nil
}

// parts gives the parts of the book in the list of chapters. A line with no backslash is the title of a part, and the book script makes the label book-part-<title> from it, with the title in lower case and a dash between the words.
func parts(b []byte) []*item {
	var out []*item
	for i, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.Contains(l, `\`) {
			continue
		}
		label := "book-part-" + strings.Join(strings.Fields(strings.ToLower(l)), "-")
		out = append(out, &item{label: label, kind: "part", title: l, locator: Chapters + ":" + strconv.Itoa(i+1)})
	}
	return out
}

// item is a labelled part of a chapter.
type item struct {
	label   string // the full label
	kind    string
	title   string
	chapter string
	section string
	locator string
	text    strings.Builder
}

// statements are the environments of the project that have a label and can have a proof after them.
var statements = map[string]bool{
	"lemma": true, "proposition": true, "theorem": true, "definition": true, "remark": true,
	"remarks": true, "example": true, "exercise": true, "situation": true,
}

// open is an environment that the scanner is in.
type open struct {
	env  string
	name string // the name in square brackets
	line int
	it   *item // nil until the label of the environment
}

// scan reads one chapter and gives its labelled items. The text of a statement is its body and the proof after it. The text of a section is its prose outside the statements and their proofs.
func scan(name string, r io.Reader) ([]*item, error) {
	chapter := strings.TrimSuffix(name, ".tex")
	var (
		items    []*item
		title    string // the title of the chapter
		section  string // the title of the current section
		heading  *open  // a \section or \subsection that waits for its label
		prose    *item  // the item that gets the prose: the chapter, the section or the subsection
		stack    []*open
		proofFor *item // the last statement, which gets the next proof
		proof    int   // the depth of proof environments
	)
	add := func(label string, line int, kind, t string) *item {
		it := &item{label: chapter + "-" + label, kind: kind, title: t, chapter: title, locator: name + ":" + strconv.Itoa(line)}
		if kind != "chapter" && kind != "section" {
			it.section = section
		}
		items = append(items, it)
		return it
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "%") {
			continue
		}
		if v, ok := command(t, `\title`); ok && title == "" {
			title = v
			continue
		}
		if v, ok := command(t, `\section`); ok {
			section = v
			heading = &open{env: "section", name: v, line: n}
			proofFor = nil
			continue
		}
		if v, ok := command(t, `\subsection`); ok {
			heading = &open{env: "subsection", name: v, line: n}
			proofFor = nil
			continue
		}
		if env, rest, ok := begin(t); ok {
			switch {
			case env == "proof" && (proof > 0 || proofFor != nil):
				proof++
			case statements[env] || env == "equation":
				o := &open{env: env, line: n}
				if strings.HasPrefix(rest, "[") {
					o.name = strings.TrimSuffix(strings.TrimPrefix(rest, "["), "]")
				}
				stack = append(stack, o)
				proofFor = nil
			}
		}
		if label, ok := command(t, `\label`); ok || strings.Contains(t, `\label{`) {
			if !ok {
				label = labelIn(t)
			}
			kind, _, _ := strings.Cut(label, "-")
			switch {
			case label == "section-phantom":
				prose = add(label, n, "chapter", title)
			case heading != nil && kind == heading.env:
				prose = add(label, n, kind, heading.name)
				heading = nil
			case len(stack) > 0 && stack[len(stack)-1].it == nil:
				o := stack[len(stack)-1]
				o.it = add(label, o.line, o.env, o.name)
			default:
				add(label, n, kind, "")
			}
		}
		// The line is text of each open statement, of the proof, or of the prose. The labels and the lines that begin or end a statement or a proof are markup and not text.
		if !markup(t) {
			for _, o := range stack {
				if o.it != nil {
					o.it.text.WriteString(line + "\n")
				}
			}
			switch {
			case proof > 0 && proofFor != nil:
				proofFor.text.WriteString(line + "\n")
			case len(stack) == 0 && proof == 0 && prose != nil:
				prose.text.WriteString(line + "\n")
			}
		}
		if env, ok := end(t); ok {
			switch {
			case env == "proof" && proof > 0:
				proof--
				if proof == 0 {
					proofFor = nil
				}
			case len(stack) > 0 && stack[len(stack)-1].env == env:
				o := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if o.it == nil {
					return nil, fmt.Errorf("line %d: the %s of line %d has no label", n, env, o.line)
				}
				if statements[env] && len(stack) == 0 {
					proofFor = o.it
				}
			}
		}
		if t != "" && !strings.HasPrefix(t, `\begin{proof}`) && proof == 0 && len(stack) == 0 && !strings.HasPrefix(t, `\end{`) {
			proofFor = nil
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(stack) > 0 {
		return nil, errors.New("the file ends in the " + stack[len(stack)-1].env + " of line " + strconv.Itoa(stack[len(stack)-1].line))
	}
	return items, nil
}

// command returns the argument of a line that is one command with one argument, such as \section{Introduction}.
func command(t, name string) (string, bool) {
	rest, ok := strings.CutPrefix(t, name+"{")
	if !ok || !strings.HasSuffix(rest, "}") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimSuffix(rest, "}")), true
}

// markup reports whether a line is only a label, or begins or ends a statement, an equation or a proof.
func markup(t string) bool {
	if _, ok := command(t, `\label`); ok {
		return true
	}
	env, rest, ok := begin(t)
	if !ok {
		env, ok = end(t)
	} else if rest != "" && !strings.HasPrefix(rest, "[") {
		return false
	}
	return ok && (statements[env] || env == "equation" || env == "proof")
}

// labelIn returns the argument of the first \label in a line.
func labelIn(t string) string {
	_, rest, _ := strings.Cut(t, `\label{`)
	label, _, _ := strings.Cut(rest, "}")
	return label
}

// begin returns the environment of a line that starts with \begin, and the rest of the line.
func begin(t string) (env, rest string, ok bool) {
	rest, ok = strings.CutPrefix(t, `\begin{`)
	if !ok {
		return "", "", false
	}
	env, rest, ok = strings.Cut(rest, "}")
	return env, strings.TrimSpace(rest), ok
}

// end returns the environment of a line that is \end{...}.
func end(t string) (string, bool) {
	rest, ok := strings.CutPrefix(t, `\end{`)
	if !ok {
		return "", false
	}
	env, _, ok := strings.Cut(rest, "}")
	return env, ok
}
