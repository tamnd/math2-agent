// Package search finds nodes of the knowledge graph through four channels and fuses their results (spec 14).
//
// Write builds the index of a graph into one SQLite file, and Open reads it. Index.Search runs the identifier resolver, then the lexical, formula, formal and embedding channels, and fuses their ranks with reciprocal rank fusion:
//
//   - The identifier resolver finds a node from its tag, an alias, an m2:// URI, a Lean declaration or an external id, such as a Wikidata QID or an OEIS A-number. A node that it finds comes first.
//   - The lexical channel is SQLite FTS5 with BM25 over the name, the aliases, the statement and the other sections of each node.
//   - The formula channel parses each LaTeX formula into an operator tree. It indexes the paths from the leaves to the root with anonymous variables, finds candidates with BM25 over the paths and ranks them again by the largest subtree of the query that they have, with a consistent change of the names of the variables.
//   - The formal channel reads Lean types, goals and lists of constants. It matches the head constant of the conclusion, the head constants of the hypotheses and the constants of the type of each extracted declaration. A declaration that no node binds is a result with no node yet.
//   - The embedding channel compares the vector of the query with the vectors of the texts of each node. The index holds the vectors in int8 and searches all of them. A result never holds a vector.
//
// The fusion multiplies the fused score by a prior of the kind and the evidence level of the node, and it folds a retired node into its successors. Each result gives the channels that found it and its rank in each one. Index.Premises ranks Lean declarations for a goal (spec 14.5).
//
// The embedder of the index is align.Hash unless the caller gives another one. A learned model and the record plane come in later issues, and bench/search measures each change.
package search

import (
	"cmp"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	_ "modernc.org/sqlite" // the SQLite driver, in pure Go

	"github.com/tamnd/math2-agent/align"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/uri"
)

// Version is the version of the tables. Open does not read an index of a different version.
const Version = "1"

// The channels. Identifier is the identifier resolver.
const (
	Identifier = "identifier"
	Lexical    = "lexical"
	Formula    = "formula"
	Formal     = "formal"
	Embedding  = "embedding"
	Dependency = "dependency" // the dependency priors of Premises
)

// Channels returns the names of the channels that Search can run, in order.
func Channels() []string { return []string{Identifier, Lexical, Formula, Formal, Embedding} }

// Weights are the default weights of the channels in the fusion. The search benchmark sets them (spec 14.6). The embedding channel has a smaller weight, because align.Hash only finds texts with the same words.
var Weights = map[string]float64{Lexical: 1, Formula: 1, Formal: 1, Embedding: 0.5, Dependency: 1}

// rrfK is the constant of reciprocal rank fusion. A result at rank r in a channel gets weight/(rrfK+r).
const rrfK = 60

// depth is the number of results that each channel gives to the fusion.
const depth = 100

const schema = `
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE docs (id INTEGER PRIMARY KEY, tag TEXT NOT NULL UNIQUE, kind TEXT NOT NULL, name TEXT NOT NULL, retired INTEGER NOT NULL, evidence INTEGER NOT NULL, successors TEXT NOT NULL, uses TEXT NOT NULL);
CREATE TABLE ids (id TEXT NOT NULL, system TEXT NOT NULL, tag TEXT NOT NULL);
CREATE VIRTUAL TABLE lex USING fts5 (name, aliases, statement, more, tokenize = 'unicode61 remove_diacritics 2');
CREATE TABLE formulas (id INTEGER PRIMARY KEY, doc INTEGER NOT NULL, latex TEXT NOT NULL, size INTEGER NOT NULL, subtrees TEXT NOT NULL);
CREATE VIRTUAL TABLE paths USING fts5 (words, tokenize = 'unicode61');
CREATE TABLE decls (decl TEXT PRIMARY KEY, kind TEXT NOT NULL, module TEXT NOT NULL, tags TEXT NOT NULL, statement TEXT NOT NULL, concl TEXT NOT NULL, hyps TEXT NOT NULL, consts TEXT NOT NULL, uses TEXT NOT NULL);
CREATE TABLE vectors (owner TEXT NOT NULL, vec BLOB NOT NULL);
`

const indexes = `
CREATE INDEX ids_id ON ids (id, system);
CREATE INDEX formulas_doc ON formulas (doc);
`

// Input is the content of an index.
type Input struct {
	Nodes    []*node.Node
	Status   []status.Status   // the status of each node
	Aliases  map[string]string // the tag of each alias
	Records  []lean.Record     // the extracted declarations, from extract.json
	Bindings []lean.Binding    // the declarations with the attribute @[m2 "TAG"]
	Embedder align.Embedder    // nil means align.Hash
}

// The sections that state the mathematics of a node, or describe it, in the order that they come.
var mainSections = []string{"Statement", "Definition", "Problem", "Summary", "Description", "Example", "Counterexample", "Proof"}

var wikiLink = regexp.MustCompile(`\[\[([^\]|]*)(?:\|[^\]]*)?\]\]`)

// plain returns Markdown text with each [[text|target]] link changed to its text.
func plain(s string) string { return strings.TrimSpace(wikiLink.ReplaceAllString(s, "$1")) }

// Write writes the index to the file at path. It replaces the file if it exists. Two calls with the same input give the same file.
func Write(path string, in Input) (err error) {
	if in.Embedder == nil {
		in.Embedder = align.Hash{}
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(off)&_pragma=synchronous(off)")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err := tx.Exec(schema); err != nil {
		return err
	}
	w := &writer{tx: tx, stmts: map[string]*sql.Stmt{}}
	defer w.close()
	if err := w.write(in); err != nil {
		return err
	}
	if _, err := tx.Exec(indexes); err != nil {
		return err
	}
	return tx.Commit()
}

type writer struct {
	tx    *sql.Tx
	stmts map[string]*sql.Stmt
}

func (w *writer) exec(query string, args ...any) error {
	st, ok := w.stmts[query]
	if !ok {
		var err error
		if st, err = w.tx.Prepare(query); err != nil {
			return err
		}
		w.stmts[query] = st
	}
	_, err := st.Exec(args...)
	return err
}

func (w *writer) close() {
	for _, st := range w.stmts {
		_ = st.Close()
	}
}

func (w *writer) write(in Input) error {
	nodes := slices.Clone(in.Nodes)
	slices.SortFunc(nodes, func(a, b *node.Node) int { return strings.Compare(a.ID, b.ID) })
	g := graph.Build(nodes)
	level := map[string]status.Level{}
	for _, s := range in.Status {
		level[s.Tag] = s.Evidence
	}
	aliases := map[string][]string{}
	for _, a := range slices.Sorted(maps.Keys(in.Aliases)) {
		aliases[in.Aliases[a]] = append(aliases[in.Aliases[a]], a)
	}
	successors, uses, bound := map[string][]string{}, map[string][]string{}, map[string][]string{}
	var ids [][3]string
	for _, e := range g.Edges {
		switch e.Type {
		case graph.Successor:
			successors[e.From] = append(successors[e.From], e.To)
		case graph.Uses:
			uses[e.From] = append(uses[e.From], e.To)
		case graph.Formalizes:
			bound[e.From] = append(bound[e.From], e.To)
			ids = append(ids, [3]string{e.From, "lean", e.To})
		case graph.SameAs, graph.Source:
			if e.To != "" {
				ids = append(ids, [3]string{e.To, e.System, e.From})
			}
		}
	}
	for _, b := range in.Bindings {
		bound[b.Decl] = append(bound[b.Decl], b.Tag)
		ids = append(ids, [3]string{b.Decl, "lean", b.Tag})
	}
	for _, n := range g.Nodes {
		ids = append(ids, [3]string{n.Tag, "m2", n.Tag})
	}
	for a, t := range in.Aliases {
		ids = append(ids, [3]string{a, "alias", t})
	}
	slices.SortFunc(ids, func(a, b [3]string) int {
		return cmp.Or(strings.Compare(a[0], b[0]), strings.Compare(a[1], b[1]), strings.Compare(a[2], b[2]))
	})
	ids = slices.Compact(ids)
	for _, id := range ids {
		if err := w.exec(`INSERT INTO ids VALUES (?, ?, ?)`, id[0], id[1], id[2]); err != nil {
			return err
		}
	}

	records := slices.Clone(in.Records)
	slices.SortFunc(records, func(a, b lean.Record) int { return strings.Compare(a.Decl, b.Decl) })
	records = slices.CompactFunc(records, func(a, b lean.Record) bool { return a.Decl == b.Decl })
	statements := map[string][]string{} // the Lean statements of each tag
	for _, r := range records {
		if r.Error != "" {
			continue
		}
		tags := slices.Sorted(slices.Values(bound[r.Decl]))
		tags = slices.Compact(tags)
		for _, t := range tags {
			statements[t] = append(statements[t], r.StatementPP)
		}
		sh := typeShape(r.Type, r.UsesConstantsInStatement)
		if err := w.exec(`INSERT INTO decls VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, r.Decl, r.Kind, r.Module, strings.Join(tags, " "), r.StatementPP, sh.concl, strings.Join(sh.hyps, " "), strings.Join(sh.consts, " "), strings.Join(r.UsesDecls, " ")); err != nil {
			return err
		}
		if r.StatementPP != "" {
			if err := w.exec(`INSERT INTO vectors VALUES (?, ?)`, "d:"+r.Decl, quantize(in.Embedder.Embed(r.StatementPP)).bytes()); err != nil {
				return err
			}
		}
	}

	files := map[string]*node.Node{}
	for _, n := range nodes {
		files[n.ID] = n
	}
	for i, gn := range g.Nodes {
		n := files[gn.Tag]
		var main, more, expl []string
		if n != nil {
			for _, s := range check.Sections(n) {
				text := plain(s.Text)
				switch {
				case s.Name == "Explanations":
					expl = append(expl, text)
					more = append(more, text)
				case slices.Contains(mainSections, s.Name) && len(main) == 0:
					main = append(main, text)
				default:
					more = append(more, text)
				}
			}
		}
		id := i + 1
		if err := w.exec(`INSERT INTO docs VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, gn.Tag, gn.Kind, gn.Name, gn.Retired, int(level[gn.Tag]), strings.Join(successors[gn.Tag], " "), strings.Join(uses[gn.Tag], " ")); err != nil {
			return err
		}
		statement := strings.Join(main, "\n\n")
		if err := w.exec(`INSERT INTO lex (rowid, name, aliases, statement, more) VALUES (?, ?, ?, ?, ?)`, id, gn.Name, strings.Join(aliases[gn.Tag], " "), statement, strings.Join(more, "\n\n")); err != nil {
			return err
		}
		texts := []string{strings.TrimSpace(gn.Name + "\n" + statement)}
		texts = append(texts, statements[gn.Tag]...)
		texts = append(texts, expl...)
		for _, t := range texts {
			if strings.TrimSpace(t) == "" {
				continue
			}
			if err := w.exec(`INSERT INTO vectors VALUES (?, ?)`, "n:"+gn.Tag, quantize(in.Embedder.Embed(t)).bytes()); err != nil {
				return err
			}
		}
		var spans []string
		for _, t := range append(main, more...) {
			spans = append(spans, mathSpans(t)...)
		}
		slices.Sort(spans)
		for _, latex := range slices.Compact(spans) {
			f, ok := indexFormula(latex)
			if !ok {
				continue
			}
			var sub []string
			for _, k := range slices.Sorted(maps.Keys(f.subtrees)) {
				sub = append(sub, fmt.Sprintf("%s:%d", k, f.subtrees[k]))
			}
			res, err := w.tx.Exec(`INSERT INTO formulas (doc, latex, size, subtrees) VALUES (?, ?, ?, ?)`, id, latex, f.size, strings.Join(sub, " "))
			if err != nil {
				return err
			}
			fid, err := res.LastInsertId()
			if err != nil {
				return err
			}
			if err := w.exec(`INSERT INTO paths (rowid, words) VALUES (?, ?)`, fid, f.words); err != nil {
				return err
			}
		}
	}
	for _, kv := range [][2]string{{"version", Version}, {"embedder", in.Embedder.Name()}, {"dim", strconv.Itoa(len(in.Embedder.Embed("dim")))}} {
		if err := w.exec(`INSERT INTO meta VALUES (?, ?)`, kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}

// doc is a node in the index.
type doc struct {
	id         int64
	tag, kind  string
	name       string
	retired    bool
	evidence   status.Level
	successors []string
	uses       []string
}

// decl is a declaration in the index.
type decl struct {
	name, kind string
	tags       []string
	statement  string
	shape      shape
	uses       []string
}

type owned struct {
	owner string
	v     vector
}

// Index is an open index. Its methods are safe for concurrent use.
type Index struct {
	db       *sql.DB
	embedder align.Embedder
	docs     []doc // in the order of their ids, from 1
	byTag    map[string]int
	decls    []decl
	byDecl   map[string]int
	consts   map[string]bool // the constants of the types of the declarations, and their last parts
	nodeVecs []owned
	declVecs []owned
}

// Open opens the index in the file at path. The embedder must be the one that wrote the index. Nil means align.Hash.
func Open(path string, e align.Embedder) (*Index, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	if e == nil {
		e = align.Hash{}
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	ix := &Index{db: db, embedder: e, byTag: map[string]int{}, byDecl: map[string]int{}, consts: map[string]bool{}}
	if err := ix.load(); err != nil {
		return nil, errors.Join(fmt.Errorf("search: %s: %w", path, err), db.Close())
	}
	return ix, nil
}

func (ix *Index) load() error {
	meta := map[string]string{}
	err := each(ix.db, `SELECT key, value FROM meta`, func(r *sql.Rows) error {
		var k, v string
		err := r.Scan(&k, &v)
		meta[k] = v
		return err
	})
	if err != nil {
		return err
	}
	if meta["version"] != Version {
		return fmt.Errorf("the index has version %q, and this m2 reads version %s. Run m2 build again", meta["version"], Version)
	}
	if meta["embedder"] != ix.embedder.Name() {
		return fmt.Errorf("the index has the vectors of %s, not of %s", meta["embedder"], ix.embedder.Name())
	}
	if dim := strconv.Itoa(len(ix.embedder.Embed("dim"))); meta["dim"] != dim {
		return fmt.Errorf("the index has vectors of length %s, and the embedder gives vectors of length %s", meta["dim"], dim)
	}
	err = each(ix.db, `SELECT id, tag, kind, name, retired, evidence, successors, uses FROM docs ORDER BY id`, func(r *sql.Rows) error {
		var d doc
		var succ, uses string
		var ev int
		if err := r.Scan(&d.id, &d.tag, &d.kind, &d.name, &d.retired, &ev, &succ, &uses); err != nil {
			return err
		}
		d.evidence = status.Level(ev) // #nosec G115 -- Write stores a Level.
		d.successors, d.uses = strings.Fields(succ), strings.Fields(uses)
		ix.byTag[d.tag] = len(ix.docs)
		ix.docs = append(ix.docs, d)
		return nil
	})
	if err != nil {
		return err
	}
	err = each(ix.db, `SELECT decl, kind, tags, statement, concl, hyps, consts, uses FROM decls ORDER BY decl`, func(r *sql.Rows) error {
		var d decl
		var tags, hyps, consts, uses string
		if err := r.Scan(&d.name, &d.kind, &tags, &d.statement, &d.shape.concl, &hyps, &consts, &uses); err != nil {
			return err
		}
		d.tags, d.shape.hyps, d.shape.consts, d.uses = strings.Fields(tags), strings.Fields(hyps), strings.Fields(consts), strings.Fields(uses)
		for _, c := range append(append([]string{d.shape.concl}, d.shape.hyps...), d.shape.consts...) {
			for c != "" {
				ix.consts[c] = true
				_, c, _ = strings.Cut(c, ".")
			}
		}
		ix.byDecl[d.name] = len(ix.decls)
		ix.decls = append(ix.decls, d)
		return nil
	})
	if err != nil {
		return err
	}
	return each(ix.db, `SELECT owner, vec FROM vectors ORDER BY rowid`, func(r *sql.Rows) error {
		var o owned
		var b []byte
		if err := r.Scan(&o.owner, &b); err != nil {
			return err
		}
		v, err := readVector(b)
		if err != nil {
			return err
		}
		o.v = v
		if strings.HasPrefix(o.owner, "n:") {
			o.owner = o.owner[2:]
			ix.nodeVecs = append(ix.nodeVecs, o)
		} else {
			o.owner = strings.TrimPrefix(o.owner, "d:")
			ix.declVecs = append(ix.declVecs, o)
		}
		return nil
	})
}

// Close closes the index.
func (ix *Index) Close() error { return ix.db.Close() }

func each(db *sql.DB, query string, fn func(*sql.Rows) error, args ...any) error {
	r, err := db.Query(query, args...)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	for r.Next() {
		if err := fn(r); err != nil {
			return err
		}
	}
	return r.Err()
}

// Query is a search.
type Query struct {
	Text     string
	K        int                // the number of results. Zero means 10.
	Channels []string           // the channels to run. Empty means all of them.
	Kinds    []string           // only nodes of these kinds. Empty means all kinds, and the declarations with no node.
	Open     bool               // rank the open problems and the weak evidence first, as the open facet of the workspace does
	Weights  map[string]float64 // the weights of the channels. A channel that is not in the map has its weight in Weights.
}

// Result is one result of a search: a node, or a Lean declaration that no node binds.
type Result struct {
	Tag      string   `json:"tag,omitempty"`  // the node, or "" for a declaration with no node yet
	Decl     string   `json:"decl,omitempty"` // the declaration that the formal channel found, or ""
	Kind     string   `json:"kind,omitempty"`
	Name     string   `json:"name"`
	Evidence string   `json:"evidence,omitempty"`
	Score    float64  `json:"score"`
	Channels []Hit    `json:"channels"`         // the channels that found the result, in the order of Channels
	Folded   []string `json:"folded,omitempty"` // the retired nodes that the result stands for
}

// Hit is the place of a result in one channel.
type Hit struct {
	Channel string  `json:"channel"`
	Rank    int     `json:"rank"` // from 1
	Score   float64 `json:"score"`
}

// cand is a result of one channel. The key is the tag of a node, or "lean:" and the name of a declaration with no node.
type cand struct {
	key   string
	score float64
	decl  string
}

// Search runs the query and returns the results in order, best first.
func (ix *Index) Search(q Query) ([]Result, error) {
	if q.K <= 0 {
		q.K = 10
	}
	text := strings.TrimSpace(q.Text)
	if text == "" {
		return nil, errors.New("search: the query is empty")
	}
	for _, c := range q.Channels {
		if !slices.Contains(Channels(), c) {
			return nil, fmt.Errorf("search: %q is not a channel. The channels are %s", c, strings.Join(Channels(), ", "))
		}
	}
	on := func(c string) bool { return len(q.Channels) == 0 || slices.Contains(q.Channels, c) }
	lists := map[string][]cand{}
	var err error
	if on(Identifier) {
		if lists[Identifier], err = ix.identifier(text); err != nil {
			return nil, err
		}
	}
	if on(Lexical) {
		if lists[Lexical], err = ix.lexical(text); err != nil {
			return nil, err
		}
	}
	if on(Formula) {
		if lists[Formula], err = ix.formula(text); err != nil {
			return nil, err
		}
	}
	if on(Formal) && isLean(text) {
		lists[Formal] = ix.formal(parseLean(text))
	}
	// An identifier has no meaning that the embedding channel can match, thus a query that resolves as an identifier does not run it.
	if on(Embedding) && len(lists[Identifier]) == 0 {
		lists[Embedding] = ix.embedding(text)
	}
	return ix.fuse(q, lists), nil
}

// identifier resolves the query as one identifier.
func (ix *Index) identifier(text string) ([]cand, error) {
	if strings.ContainsAny(text, " \t\n") {
		return nil, nil
	}
	var where, system string
	where = text
	if strings.HasPrefix(text, uri.Scheme) {
		u, err := uri.Parse(text)
		if err != nil {
			return nil, err
		}
		switch {
		case u.IsRow():
			return nil, nil // a row is not a node
		case !u.IsAlias():
			where = u.Tag
		case u.Namespace == "lean":
			where, system = u.Name, "lean"
		case u.Namespace == "ext":
			system, where, _ = strings.Cut(u.Name, "/")
		default:
			where = u.Name
		}
	}
	var out []cand
	seen := map[string]bool{}
	for _, id := range []string{where, strings.ToLower(where)} {
		err := each(ix.db, `SELECT tag FROM ids WHERE id = ? AND (? = '' OR system = ?) ORDER BY tag`, func(r *sql.Rows) error {
			var t string
			if err := r.Scan(&t); err != nil {
				return err
			}
			if !seen[t] {
				seen[t] = true
				out = append(out, cand{key: t, score: 1})
			}
			return nil
		}, id, system, system)
		if err != nil || len(out) > 0 {
			return out, err
		}
	}
	if i, ok := ix.byDecl[where]; ok && len(ix.decls[i].tags) == 0 {
		out = append(out, cand{key: "lean:" + where, score: 1, decl: where})
	}
	return out, nil
}

var command = regexp.MustCompile(`\\[a-zA-Z]+`)

// words returns the words of the text in lower case, for a query of FTS5. It leaves out the LaTeX commands and the words of one letter, such as the variables of a formula and the article a.
func words(text string) []string {
	ws := strings.FieldsFunc(strings.ToLower(command.ReplaceAllString(text, " ")), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	ws = slices.DeleteFunc(ws, func(w string) bool { r, n := utf8.DecodeRuneInString(w); return n == len(w) && unicode.IsLetter(r) })
	slices.Sort(ws)
	ws = slices.Compact(ws)
	if len(ws) > 32 {
		ws = ws[:32]
	}
	return ws
}

func (ix *Index) lexical(text string) ([]cand, error) {
	ws := words(text)
	if len(ws) == 0 {
		return nil, nil
	}
	for i, w := range ws {
		ws[i] = `"` + w + `"`
	}
	var out []cand
	err := each(ix.db, `SELECT d.tag, bm25(lex, 10.0, 5.0, 2.0, 1.0) AS s FROM lex JOIN docs d ON d.id = lex.rowid WHERE lex MATCH ? ORDER BY s, d.tag LIMIT ?`, func(r *sql.Rows) error {
		var c cand
		err := r.Scan(&c.key, &c.score)
		c.score = -c.score
		out = append(out, c)
		return err
	}, strings.Join(ws, " OR "), depth)
	return out, err
}

// formulas returns the formulas of a query: its math spans, or the whole query when it looks like LaTeX and has no span.
func formulas(text string) []string {
	if spans := mathSpans(text); len(spans) > 0 {
		return spans
	}
	if strings.ContainsAny(text, `\^_=+<>|`) {
		return []string{text}
	}
	return nil
}

func (ix *Index) formula(text string) ([]cand, error) {
	best := map[string]float64{}
	for _, latex := range formulas(text) {
		qf, ok := indexFormula(latex)
		if !ok {
			continue
		}
		ws := strings.Fields(qf.words)
		slices.Sort(ws)
		ws = slices.Compact(ws)
		err := each(ix.db, `SELECT d.tag, f.size, f.subtrees, bm25(paths) AS s FROM paths JOIN formulas f ON f.id = paths.rowid JOIN docs d ON d.id = f.doc WHERE paths MATCH ? ORDER BY s, f.id LIMIT ?`, func(r *sql.Rows) error {
			var t, sub string
			var c formula
			var bm float64
			if err := r.Scan(&t, &c.size, &sub, &bm); err != nil {
				return err
			}
			c.subtrees = map[string]int{}
			for _, f := range strings.Fields(sub) {
				k, n, _ := strings.Cut(f, ":")
				var size int
				_, _ = fmt.Sscan(n, &size)
				c.subtrees[k] = size
			}
			// The structural score ranks first, and a formula with no common subtree is not a result. BM25 breaks ties, and it is less than 0.01 after the change.
			st := structural(qf, c)
			if st == 0 {
				return nil
			}
			s := st + 0.01/(1-bm)
			if s > best[t] {
				best[t] = s
			}
			return nil
		}, strings.Join(ws, " OR "), depth*4)
		if err != nil {
			return nil, err
		}
	}
	return ranked(best), nil
}

// rrf returns the term of reciprocal rank fusion for a result of a channel with the weight w, at the rank, with the score, where top is the score of rank 1. The score scales the term from one half to one. Thus an exact match far above the next result keeps a margin that the priors do not take away.
func rrf(w float64, rank int, score, top float64) float64 {
	f := 1.0
	if top > 0 {
		f = 0.5 + 0.5*max(0, min(score/top, 1))
	}
	return f * w / float64(rrfK+rank)
}

// ranked returns the keys of the scores, best first.
func ranked(scores map[string]float64) []cand {
	out := make([]cand, 0, len(scores))
	for k, s := range scores {
		out = append(out, cand{key: k, score: s})
	}
	slices.SortFunc(out, func(a, b cand) int { return cmp.Or(cmp.Compare(b.score, a.score), strings.Compare(a.key, b.key)) })
	if len(out) > depth {
		out = out[:depth]
	}
	return out
}

func (ix *Index) formal(q leanQuery) []cand {
	resolved := map[string]bool{}
	for _, c := range q.consts {
		resolved[c] = ix.consts[c]
	}
	type scored struct {
		d *decl
		s float64
	}
	var hits []scored
	for i := range ix.decls {
		d := &ix.decls[i]
		if s := q.score(d.shape, resolved); s > 0 {
			hits = append(hits, scored{d, s})
		}
	}
	// A better fit comes first. Of two equal fits, the declaration with fewer constants is more specific.
	slices.SortFunc(hits, func(a, b scored) int {
		return cmp.Or(cmp.Compare(b.s, a.s), cmp.Compare(len(a.d.shape.consts), len(b.d.shape.consts)), strings.Compare(a.d.name, b.d.name))
	})
	var out []cand
	for _, h := range hits {
		if len(h.d.tags) == 0 {
			out = append(out, cand{key: "lean:" + h.d.name, score: h.s, decl: h.d.name})
		}
		for _, t := range h.d.tags {
			out = append(out, cand{key: t, score: h.s, decl: h.d.name})
		}
		if len(out) >= depth {
			break
		}
	}
	return out
}

func (ix *Index) embedding(text string) []cand {
	q := quantize(ix.embedder.Embed(text))
	best := map[string]float64{}
	for _, o := range ix.nodeVecs {
		if s := dot(q, o.v); s > 0 && s > best[o.owner] {
			best[o.owner] = s
		}
	}
	return ranked(best)
}

// kindPrior is the prior of a kind for a query in words: statements and concepts rank above proofs (spec 14.6). The priors are near 1, because they only order results of near ranks. The ranks of reciprocal rank fusion are near too: the scores of rank 1 and rank 10 differ by 13%.
func kindPrior(kind string, open bool) float64 {
	switch kind {
	case "statement", "concept", "definition":
		return 1
	case "conjecture", "problem":
		if open {
			return 1.02
		}
		return 0.98
	case "topic", "object":
		return 0.99
	case "proof", "computation":
		return 0.95
	}
	return 0.97
}

// evidencePrior is the prior of an evidence level: E5, E3 and E2 rank above E1. The open facet turns it around, thus open and claimed nodes rank first.
func evidencePrior(l status.Level, open bool) float64 {
	weak := l == status.Open || l == status.Claimed
	switch {
	case open && weak:
		return 1
	case open:
		return 0.98
	case l >= status.Certified:
		return 1
	case l >= status.Standard:
		return 0.99
	}
	return 0.98
}

// fuse joins the lists of the channels with reciprocal rank fusion, applies the priors and folds the retired nodes.
func (ix *Index) fuse(q Query, lists map[string][]cand) []Result {
	type acc struct {
		r     Result
		score float64
		id    bool
	}
	accs := map[string]*acc{}
	get := func(key string) *acc {
		a, ok := accs[key]
		if ok {
			return a
		}
		a = &acc{}
		if d, ok := strings.CutPrefix(key, "lean:"); ok {
			a.r = Result{Decl: d, Name: d}
		} else if i, ok := ix.byTag[key]; ok {
			d := ix.docs[i]
			a.r = Result{Tag: d.tag, Kind: d.kind, Name: d.name, Evidence: d.evidence.String()}
		}
		accs[key] = a
		return a
	}
	for _, ch := range Channels() {
		w := Weights[ch]
		if x, ok := q.Weights[ch]; ok {
			w = x
		}
		var top float64
		if len(lists[ch]) > 0 {
			top = lists[ch][0].score
		}
		for i, c := range lists[ch] {
			a := get(c.key)
			a.r.Channels = append(a.r.Channels, Hit{Channel: ch, Rank: i + 1, Score: c.score})
			if c.decl != "" && a.r.Decl == "" {
				a.r.Decl = c.decl
			}
			if ch == Identifier {
				// A node that the identifier names comes before each result of the fusion, whose score is less than 1.
				a.id = true
				a.score++
				continue
			}
			a.score += rrf(w, i+1, c.score, top)
		}
	}
	// A retired node folds into its successors that are live, and a successor keeps the better score of the two. A retired node with no live successor stays only when the identifier resolver found it.
	for _, key := range slices.Sorted(maps.Keys(accs)) {
		a := accs[key]
		i, ok := ix.byTag[key]
		if !ok || !ix.docs[i].retired {
			continue
		}
		var live []string
		for _, s := range ix.docs[i].successors {
			if j, ok := ix.byTag[s]; ok && !ix.docs[j].retired {
				live = append(live, s)
			}
		}
		if len(live) == 0 {
			if !a.id {
				delete(accs, key)
			}
			continue
		}
		delete(accs, key)
		for _, s := range live {
			b := get(s)
			b.score = max(b.score, a.score)
			b.id = b.id || a.id
			b.r.Folded = append(b.r.Folded, key)
			for _, h := range a.r.Channels {
				if !slices.ContainsFunc(b.r.Channels, func(x Hit) bool { return x.Channel == h.Channel }) {
					b.r.Channels = append(b.r.Channels, h)
				}
			}
		}
	}
	var out []*acc
	for key, a := range accs {
		if len(q.Kinds) > 0 && !slices.Contains(q.Kinds, a.r.Kind) {
			continue
		}
		if i, ok := ix.byTag[key]; ok {
			d := ix.docs[i]
			a.score *= kindPrior(d.kind, q.Open) * evidencePrior(d.evidence, q.Open)
		} else {
			a.score *= kindPrior("statement", q.Open) * evidencePrior(status.None, q.Open)
		}
		a.r.Score = a.score
		order := func(h Hit) int { return slices.Index(Channels(), h.Channel) }
		slices.SortFunc(a.r.Channels, func(x, y Hit) int { return order(x) - order(y) })
		slices.Sort(a.r.Folded)
		out = append(out, a)
	}
	slices.SortFunc(out, func(a, b *acc) int {
		return cmp.Or(cmp.Compare(b.score, a.score), strings.Compare(a.r.Tag+a.r.Decl, b.r.Tag+b.r.Decl))
	})
	if len(out) > q.K {
		out = out[:q.K]
	}
	res := make([]Result, len(out))
	for i, a := range out {
		res[i] = a.r
	}
	return res
}
