// Package store is the serving store of the knowledge graph (spec 05.7).
//
// Write puts the graph, the status of each node and the aliases into one SQLite file. Open reads the file and makes the adjacency of the graph in memory, in compressed sparse row form. The SQLite tables answer the point lookups and the slice queries. The graph in memory answers the closure, the reverse dependency and the path queries. The store holds only derived data, and m2 build writes it again from the node files.
//
// A Store answers the queries Q1 and Q3 to Q7 of spec 05.6: Resolve, Closure, Dependents, Path and Frontier. The analytical queries over the Parquet files of the build (Q6 at full scale, Q10 and Q11) come with the benchmark of the graph queries.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	_ "modernc.org/sqlite" // the SQLite driver, in pure Go

	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/tag"
	"github.com/tamnd/math2-agent/uri"
)

// Version is the version of the tables. Open does not read a store of a different version.
const Version = "1"

const schema = `
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE nodes (
	tag TEXT PRIMARY KEY, kind TEXT NOT NULL, role TEXT NOT NULL, name TEXT NOT NULL, retired INTEGER NOT NULL, hash TEXT NOT NULL,
	evidence TEXT NOT NULL, fidelity TEXT NOT NULL, status TEXT NOT NULL
);
CREATE TABLE msc (tag TEXT NOT NULL, code TEXT NOT NULL, PRIMARY KEY (tag, code));
CREATE TABLE aliases (alias TEXT PRIMARY KEY, tag TEXT NOT NULL);
CREATE TABLE edges (type TEXT NOT NULL, src TEXT NOT NULL, dst TEXT NOT NULL, via TEXT NOT NULL, system TEXT NOT NULL, label TEXT NOT NULL);
CREATE INDEX edges_src ON edges (type, src);
CREATE INDEX edges_dst ON edges (type, dst);
CREATE INDEX msc_code ON msc (code);
`

// Input is the content of a store.
type Input struct {
	Nodes   []*node.Node      // the nodes, for the MSC codes of each node
	Graph   *graph.Graph      // the graph of the nodes. Nil means that Write builds it.
	Status  []status.Status   // the status of each node
	Aliases map[string]string // the tag of each alias
}

// Write writes the store to the file at path. It replaces the file if it exists.
func Write(path string, in Input) (err error) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	db, err := sql.Open("sqlite", path)
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
	g := in.Graph
	if g == nil {
		g = graph.Build(in.Nodes)
	}
	byTag := map[string]status.Status{}
	for _, s := range in.Status {
		byTag[s.Tag] = s
	}
	exec := func(query string, rows func(stmt *sql.Stmt) error) error {
		stmt, err := tx.Prepare(query)
		if err != nil {
			return err
		}
		return errors.Join(rows(stmt), stmt.Close())
	}
	err = errors.Join(
		exec(`INSERT INTO meta VALUES (?, ?)`, func(st *sql.Stmt) error {
			_, err1 := st.Exec("version", Version)
			_, err2 := st.Exec("graph", g.Hash())
			return errors.Join(err1, err2)
		}),
		exec(`INSERT INTO nodes VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, func(st *sql.Stmt) error {
			for _, n := range g.Nodes {
				s, ok := byTag[n.Tag]
				if !ok {
					s.Fidelity = status.Unbound
				}
				data, err := json.Marshal(s)
				if err != nil {
					return err
				}
				if _, err := st.Exec(n.Tag, n.Kind, n.Role, n.Name, n.Retired, n.Hash, s.Evidence.String(), s.Fidelity.String(), string(data)); err != nil {
					return err
				}
			}
			return nil
		}),
		exec(`INSERT OR IGNORE INTO msc VALUES (?, ?)`, func(st *sql.Stmt) error {
			for _, n := range in.Nodes {
				for _, code := range n.List("msc") {
					if _, err := st.Exec(n.ID, code); err != nil {
						return err
					}
				}
			}
			return nil
		}),
		exec(`INSERT INTO aliases VALUES (?, ?)`, func(st *sql.Stmt) error {
			for _, a := range sortedKeys(in.Aliases) {
				if _, err := st.Exec(a, in.Aliases[a]); err != nil {
					return err
				}
			}
			return nil
		}),
		exec(`INSERT INTO edges VALUES (?, ?, ?, ?, ?, ?)`, func(st *sql.Stmt) error {
			for _, e := range g.Edges {
				if _, err := st.Exec(e.Type, e.From, e.To, e.Via, e.System, e.Label); err != nil {
					return err
				}
			}
			return nil
		}),
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Store is an open store. Its methods are safe for concurrent use.
type Store struct {
	db     *sql.DB
	graph  *graph.Graph
	status map[string]status.Status
}

// Open opens the store in the file at path, and reads the graph into memory.
func Open(path string) (*Store, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	s, err := load(db)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("store: %s: %w", path, err), db.Close())
	}
	return s, nil
}

func load(db *sql.DB) (*Store, error) {
	var v string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key = 'version'`).Scan(&v); err != nil {
		return nil, err
	}
	if v != Version {
		return nil, fmt.Errorf("the store has version %s, and this m2 reads version %s. Run m2 build again", v, Version)
	}
	s := &Store{db: db, status: map[string]status.Status{}}
	var nodes []graph.Node
	err := rows(db, `SELECT tag, kind, role, name, retired, hash, status FROM nodes`, func(r *sql.Rows) error {
		var n graph.Node
		var data string
		if err := r.Scan(&n.Tag, &n.Kind, &n.Role, &n.Name, &n.Retired, &n.Hash, &data); err != nil {
			return err
		}
		var st status.Status
		if err := json.Unmarshal([]byte(data), &st); err != nil {
			return err
		}
		nodes = append(nodes, n)
		if st.Tag != "" {
			s.status[n.Tag] = st
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var edges []graph.Edge
	err = rows(db, `SELECT type, src, dst, via, system, label FROM edges`, func(r *sql.Rows) error {
		var e graph.Edge
		if err := r.Scan(&e.Type, &e.From, &e.To, &e.Via, &e.System, &e.Label); err != nil {
			return err
		}
		edges = append(edges, e)
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.graph = graph.New(nodes, edges)
	return s, nil
}

// Close closes the store.
func (s *Store) Close() error { return s.db.Close() }

// Graph returns the graph of the store.
func (s *Store) Graph() *graph.Graph { return s.graph }

// Status returns the status of the node with the tag.
func (s *Store) Status(tag string) (status.Status, bool) {
	st, ok := s.status[tag]
	return st, ok
}

// Resolve returns the tags of the nodes that ref names (query Q1). ref is a tag, a provisional id, an alias or an m2:// URI. The URI can name a node, an alias of a kind, a Lean declaration or an external id: m2://7K3QX2, m2://concept/group, m2://lean/Subgroup.card_subgroup_dvd_card or m2://ext/wikidata/Q83478. Only a Lean declaration or an external id can name more than one node. The tags are in byte order.
func (s *Store) Resolve(ref string) ([]string, error) {
	switch {
	case strings.HasPrefix(ref, uri.Scheme):
		u, err := uri.Parse(ref)
		if err != nil {
			return nil, err
		}
		if u.Release != "" || u.Commit != "" {
			return nil, fmt.Errorf("%s names a release or a commit, and the store holds only the last build", ref)
		}
		switch {
		case !u.IsAlias():
			return s.node(u.Tag)
		case u.Namespace == "lean":
			return s.tags(ref, `SELECT DISTINCT dst FROM edges WHERE type = ? AND src = ? ORDER BY dst`, graph.Formalizes, u.Name)
		case u.Namespace == "ext":
			system, id, _ := strings.Cut(u.Name, "/")
			return s.tags(ref, `SELECT DISTINCT src FROM edges WHERE type = ? AND system = ? AND dst = ? ORDER BY src`, graph.SameAs, system, id)
		case slices.Contains(uri.Kinds, u.Namespace):
			var tags []string
			if tag.IsTag(u.Name) {
				tags, err = s.node(u.Name)
			} else {
				tags, err = s.alias(u.Name)
			}
			if err != nil {
				return nil, err
			}
			if n, _ := s.graph.Node(tags[0]); n.Kind != u.Namespace {
				return nil, fmt.Errorf("%s names %s, and %s is a %s, not a %s", ref, n.Tag, n.Tag, n.Kind, u.Namespace)
			}
			return tags, nil
		}
		return nil, fmt.Errorf("%s names a %s view, not a node", ref, u.Namespace)
	case tag.IsTag(ref) || tag.IsProvisional(ref):
		return s.node(ref)
	case tag.IsAlias(ref):
		return s.alias(ref)
	}
	return nil, fmt.Errorf("%q is not a tag, an alias or an m2:// URI", ref)
}

func (s *Store) node(t string) ([]string, error) {
	if _, ok := s.graph.Node(t); !ok {
		return nil, fmt.Errorf("no node has the tag %s in the store. Check the tag, or run m2 build again", t)
	}
	return []string{t}, nil
}

func (s *Store) alias(name string) ([]string, error) {
	return s.tags("the alias "+name, `SELECT tag FROM aliases WHERE alias = ?`, name)
}

// tags runs a query that gives tags. It is an error when the query gives none.
func (s *Store) tags(what, query string, args ...any) ([]string, error) {
	var out []string
	err := rows(s.db, query, func(r *sql.Rows) error {
		var t string
		err := r.Scan(&t)
		out = append(out, t)
		return err
	}, args...)
	if err == nil && len(out) == 0 {
		err = fmt.Errorf("no node has %s", strings.TrimPrefix(what, uri.Scheme))
	}
	return out, err
}

// rows runs a query and calls fn for each row.
func rows(db *sql.DB, query string, fn func(*sql.Rows) error, args ...any) error {
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

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
