// Package arxivresults is the ingest adapter of the results of arXiv papers (spec 09.5). It reads the content plane of tamnd/arxiv and gives one record for each theorem, lemma, definition, conjecture, problem and example of a paper.
//
// math2 does not extract results again. The extractor of tamnd/arxiv writes each paper as Markdown files under content/en/<YYMM>/<id>/, and it marks each object of the paper with an attribute block at the end of the line where the object starts:
//
//	**Theorem 1** {#thm-1 .statement tag=3KH2 env=theorem}
//	**Proof of [Theorem 1](#thm-1)** {#proof-u1 .proof tag=O3RU}
//
// The adapter reads these blocks. The extent of a result is from its block to the next block that is not an equation, so a display inside a theorem is a part of the theorem. This is the rule of the objects package of tamnd/arxiv-reader, except for the equations. A proof belongs to the result that its heading links to. A proof with no link belongs to the last result before it in the same file, when no section or remark is between them. The dependencies of a result are the other results that its statement or its proofs link to. A link to an equation counts as a link to the result that the equation is in, or to the result of the proof that the equation is in. A link to a section, a figure or a citation is not a dependency, and the adapter does not read references in plain words, such as "by Lemma 3.2" with no link.
//
// The id of a record is the paper and the tag, for example 2312.00752#3KH2, and the locator is ax://paper/2312.00752#3KH2. When a result has no tag yet, the adapter uses its local name, for example thm-1, in place of the tag. The kind is a node kind of spec 04.2: statement, definition, conjecture, problem, example or counterexample. A remark is not a result. The licence of a record is the licence of the version of the paper, and the licence gate keeps the text of the statement only when the class of the licence lets math2 store it. The adapter reads only the mathematics papers, as the arxiv adapter does, and it skips the papers with the access class record, because tamnd/arxiv has no text of these papers. The meta of a record has these keys:
//
//	paper       the arXiv id, without the version
//	version     the version that the content plane has, for example v2
//	env         the environment, for example lemma
//	role        the role of a statement in spec 04.2, for example lemma
//	number      the number that the paper prints, for example 3.2
//	local       the local name of the result in the paper, for example thm-1
//	section     the local name of the section that the result is in
//	file        the content file, for example 03_selective_state_space_models.md
//	path        the source type: render (HTML), source (LaTeX), native (PDF text) or vision (a model)
//	proofs      the ids of the proofs, with a space between them
//	deps        the ids of the results that the result uses, with a space between them
//	msc         the MSC codes of the paper
//	categories  the arXiv categories of the paper
//	derivatives no when the licence forbids derived works, so no unfolding is allowed
//
// A key is not in the meta when it has no value. Unfolding (spec 09.5, step 4) needs a model, and this adapter does not do it. The MSC codes are the codes of the paper. The classifier of step 5 is a later step.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/arxivresults"
package arxivresults

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/arxiv"
)

// Repository is the git repository of the corpus.
const Repository = "https://github.com/tamnd/arxiv"

// Archive is the git archive of the main branch of the repository.
const Archive = Repository + "/archive/refs/heads/main.tar.gz"

// Adapter is the adapter of the results of arXiv papers.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of the results. Each record has the licence of its paper, so the licence of the source is the default licence of arXiv, which lets math2 keep only the record.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "arxiv_results",
		Name:        "The results of arXiv papers, through the content plane of tamnd/arxiv",
		Home:        Repository,
		Licence:     "LicenseRef-arXiv",
		Class:       ingest.RecordOnly,
		Attribution: "The authors of each paper on arXiv, https://arxiv.org, under the licence of the paper",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "arxiv_results/1" }

// Dump returns the git archive of the main branch.
func (Adapter) Dump() string { return Archive }

// Fetch gets the git archive at req.URL and sets req.Commit to its commit.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	return ingest.FetchArchive(ctx, req, w)
}

// Kind returns the node kind and the role of spec 04.2 for an object of the content plane with the environment env and the class class. For example, the environment lemma gives the kind statement and the role lemma. An environment that is not known gives the kind of its class, and a class that is not a result, such as remark or proof, gives an empty kind.
func Kind(env, class string) (kind, role string) {
	switch env = strings.ToLower(env); env {
	case "theorem", "lemma", "proposition", "corollary", "fact", "axiom":
		return "statement", env
	case "claim":
		return "statement", "lemma"
	case "definition", "conjecture", "problem", "example", "counterexample":
		return env, ""
	case "question":
		return "problem", ""
	}
	switch class {
	case "statement":
		return "statement", ""
	case "definition", "problem", "example":
		return class, ""
	}
	return "", ""
}

// front is the front matter of a content file.
type front struct {
	Paper      string   `yaml:"paper"`
	Version    string   `yaml:"version"`
	Categories []string `yaml:"categories"`
	MSCClass   string   `yaml:"msc_class"`
	Access     string   `yaml:"access"`
	Licence    string   `yaml:"licence"`
	Source     string   `yaml:"licence_of_source"`
	Path       string   `yaml:"path"`
	LocalID    string   `yaml:"local_id"`
}

// file is one content file of a paper.
type file struct {
	name  string
	front front
	body  string
}

// contentFile matches the path of an English content file in the archive.
var contentFile = regexp.MustCompile(`^content/en/[0-9]{4}/[^/]+/[^/]+\.md$`)

// Read reads the git archive of tamnd/arxiv and emits one record for each result of each paper.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	papers := map[string][]file{}
	var order []string
	_, err := ingest.WalkArchive(r, func(name string, f io.Reader) error {
		if !contentFile.MatchString(name) {
			return nil
		}
		data, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		cf, err := parseFile(path.Base(name), data)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		dir := path.Dir(name)
		if _, ok := papers[dir]; !ok {
			order = append(order, dir)
		}
		papers[dir] = append(papers[dir], cf)
		return nil
	})
	if err != nil {
		return err
	}
	for _, dir := range order {
		files := papers[dir]
		slices.SortFunc(files, func(a, b file) int { return strings.Compare(a.name, b.name) })
		recs, err := paper(files)
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		for _, rec := range recs {
			if err := emit(rec); err != nil {
				return err
			}
		}
	}
	return nil
}

// parseFile splits a content file into its front matter and its body.
func parseFile(name string, data []byte) (file, error) {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	rest, ok := bytes.CutPrefix(data, []byte("---\n"))
	if !ok {
		return file{}, fmt.Errorf("the file has no front matter")
	}
	head, body, ok := bytes.Cut(rest, []byte("\n---\n"))
	if !ok {
		return file{}, fmt.Errorf("the front matter has no end")
	}
	var f front
	if err := yaml.Unmarshal(head, &f); err != nil {
		return file{}, fmt.Errorf("the front matter: %w", err)
	}
	return file{name: name, front: f, body: string(body)}, nil
}

// block matches an attribute block at the end of a line. It is the pattern of the tags package of tamnd/arxiv-reader.
var block = regexp.MustCompile(`(?m)(^|[^\\\n])\{#([A-Za-z0-9][A-Za-z0-9._-]*)((?:\s+\.[A-Za-z][A-Za-z0-9-]*)*)((?:\s+[A-Za-z_][A-Za-z0-9_-]*=(?:"(?:[^"\\]|\\.)*"|[^\s"}]*))*)\}[ \t]*$`)

// pair matches one key=value pair of an attribute block.
var pair = regexp.MustCompile(`\s+([A-Za-z_][A-Za-z0-9_-]*)=("(?:[^"\\]|\\.)*"|[^\s"}]*)`)

// fragment matches a Markdown link to a fragment of the same paper.
var fragment = regexp.MustCompile(`\]\(#([^)\s]+)\)`)

// number matches the number after the name of an environment in a heading, for example the 3.2 of "Lemma 3.2".
var number = regexp.MustCompile(`^\S+\s+([A-Z]?[0-9][0-9.]*[0-9a-z]?|[A-Z])\b`)

// object is one attribute block of a file, with its extent.
type object struct {
	local, class string
	attrs        map[string]string
	file         int    // the index of the file in the paper
	heading      string // the text on the line of the block, before the block
	text         string // the extent, without the line of the block
	section      string // the local name of the section that the object is in
}

// objects returns the objects of one file, in the order of the file.
func objects(f file, fi int) []object {
	ms := block.FindAllStringSubmatchIndex(f.body, -1)
	out := make([]object, 0, len(ms))
	section := f.front.LocalID
	for _, m := range ms {
		o := object{local: f.body[m[4]:m[5]], file: fi, attrs: map[string]string{}}
		if fs := strings.Fields(f.body[m[6]:m[7]]); len(fs) > 0 {
			o.class = strings.TrimPrefix(fs[0], ".")
		}
		for _, p := range pair.FindAllStringSubmatch(f.body[m[8]:m[9]], -1) {
			if _, ok := o.attrs[p[1]]; !ok {
				o.attrs[p[1]] = strings.Trim(p[2], `"`)
			}
		}
		start := lineStart(f.body, m[3])
		o.heading = strings.TrimSpace(f.body[start:m[3]])
		if o.class == "section" {
			section = o.local
		}
		o.section = section
		out = append(out, o)
	}
	// The extent of an object is from the end of its block to the start of the line of the next block that is not an equation.
	for i := range out {
		from := ms[i][1]
		to := len(f.body)
		for j := i + 1; j < len(ms); j++ {
			if out[j].class != "equation" {
				to = lineStart(f.body, ms[j][3])
				break
			}
		}
		out[i].text = strings.TrimSpace(f.body[from:max(from, to)])
	}
	return out
}

// lineStart returns the start of the line that has the byte at i.
func lineStart(s string, i int) int {
	return strings.LastIndexByte(s[:i], '\n') + 1
}

// links returns the local names that a text links to, in order and without repeats.
func links(text string) []string {
	var out []string
	for _, m := range fragment.FindAllStringSubmatch(text, -1) {
		if !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}

// result is one result of a paper while the adapter reads it.
type result struct {
	object
	id, kind, role string
	proofs         []*object
}

// paper reads the files of one paper and returns the records of its results.
func paper(files []file) ([]ingest.Record, error) {
	if len(files) == 0 {
		return nil, nil
	}
	head := files[0].front
	for _, f := range files[1:] {
		if f.front.Paper != head.Paper || f.front.Version != head.Version {
			return nil, fmt.Errorf("%s is %s %s and %s is %s %s", files[0].name, head.Paper, head.Version, f.name, f.front.Paper, f.front.Version)
		}
	}
	if head.Paper == "" {
		return nil, fmt.Errorf("%s names no paper", files[0].name)
	}
	if head.Access == "record" || !arxiv.Math(head.Categories) {
		return nil, nil
	}
	var all []object
	for i, f := range files {
		all = append(all, objects(f, i)...)
	}
	// owner maps the local name of each result, and of each equation in the extent of a result or of its proof, to the result. last is the result that a proof with no link belongs to, and cur is the result whose extent the loop is in.
	var results []*result
	owner := map[string]*result{}
	var last, cur *result
	for i := range all {
		o := &all[i]
		if i > 0 && o.file != all[i-1].file {
			last, cur = nil, nil
		}
		switch kind, role := Kind(o.attrs["env"], o.class); {
		case kind != "":
			id := o.attrs["tag"]
			if id == "" {
				id = o.local
			}
			res := &result{object: *o, id: head.Paper + "#" + id, kind: kind, role: role}
			results = append(results, res)
			owner[o.local] = res
			last, cur = res, res
		case o.class == "proof":
			cur = last
			if ls := links(o.heading); len(ls) > 0 && owner[ls[0]] != nil {
				cur = owner[ls[0]]
			}
			if cur != nil {
				cur.proofs = append(cur.proofs, o)
			}
		case o.class == "equation":
			if cur != nil {
				owner[o.local] = cur
			}
		case o.class == "section" || o.class == "remark":
			last, cur = nil, nil
		default:
			cur = nil
		}
	}
	licence := arxiv.Licence(head.Source)
	if head.Source == "" && head.Licence != "" {
		licence = head.Licence
	}
	var out []ingest.Record
	for _, res := range results {
		out = append(out, record(head, files[res.file].name, licence, res, owner))
	}
	return out, nil
}

// record makes the ingest record of one result.
func record(head front, name, licence string, res *result, owner map[string]*result) ingest.Record {
	var deps, proofs []string
	add := func(text string) {
		for _, l := range links(text) {
			if to := owner[l]; to != nil && to != res && !slices.Contains(deps, to.id) {
				deps = append(deps, to.id)
			}
		}
	}
	add(res.text)
	for _, p := range res.proofs {
		id := p.attrs["tag"]
		if id == "" {
			id = p.local
		}
		proofs = append(proofs, head.Paper+"#"+id)
		add(p.text)
	}
	meta := map[string]string{}
	set := func(k, v string) {
		if v = strings.TrimSpace(v); v != "" {
			meta[k] = v
		}
	}
	title := strings.Join(strings.Fields(strings.ReplaceAll(res.heading, "*", "")), " ")
	set("paper", head.Paper)
	set("version", head.Version)
	set("env", res.attrs["env"])
	set("role", res.role)
	if m := number.FindStringSubmatch(title); m != nil {
		set("number", m[1])
	}
	set("local", res.local)
	set("section", res.section)
	set("file", name)
	set("path", head.Path)
	set("proofs", strings.Join(proofs, " "))
	set("deps", strings.Join(deps, " "))
	set("msc", strings.Join(arxiv.MSC(head.MSCClass), " "))
	set("categories", strings.Join(head.Categories, " "))
	if strings.Contains(licence, "-ND-") {
		set("derivatives", "no")
	}
	_, tag, _ := strings.Cut(res.id, "#")
	return ingest.Record{
		ID:      res.id,
		Kind:    res.kind,
		Title:   title,
		URL:     "https://arxiv.org/abs/" + head.Paper + head.Version,
		Locator: "ax://paper/" + head.Paper + "#" + tag,
		Licence: licence,
		Text:    res.text,
		Meta:    meta,
	}
}
