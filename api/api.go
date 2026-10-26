// Package api is the read surface of math2 (spec 11.2), and the write surface of spec 11.1 over HTTP.
//
// A Service answers the read methods from the derived data of one build: the serving store, the search index and the node files of the tree that the build read. Methods returns the methods with their parameters. Service.Call runs one method, and Service.Handler serves each method over HTTP with JSON. The MCP server of the mcp package uses the same methods, so the encodings cannot drift apart.
//
// A write is a pull request against tamnd/math2 (spec 11.1). Writes returns the write methods. When Options.Propose is set, Service.Handler serves them, and each write opens a pull request through package propose with the GitHub token of the caller. The audit of the change is the gate, and the API has no other write path. The gRPC service and the MCP server serve only the read methods.
//
// Each response says what it is about (spec 11.1). It has the version of the API, the commit of the tree, the output hash of the build, and the content hash and the evidence level of each node in the result. The API serves the status that the build computed and no other status.
package api

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/search"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/store"
	"github.com/tamnd/math2-agent/tag"
)

// Version is the version of the API. It is the first part of the path of each HTTP endpoint. A change that is not compatible gives a new version.
const Version = "v1"

// Config tells Open where the data of a build is.
type Config struct {
	Root   string // the math2 working tree
	Out    string // the output directory of m2 build. Empty means build/ in Root.
	Commit string // the commit of the tree, for the responses. Empty means that the responses give no commit.
}

// Service answers the read methods from the data of one build. Its methods are safe for concurrent use.
type Service struct {
	root   string
	commit string
	build  string
	store  *store.Store
	index  *search.Index
}

// Open opens the serving store and the search index that m2 build wrote. It returns an error that wraps fs.ErrNotExist when the build has not made them.
func Open(c Config) (*Service, error) {
	out := c.Out
	if out == "" {
		out = filepath.Join(c.Root, "build")
	}
	m, err := build.ReadManifest(out)
	if err != nil {
		return nil, fmt.Errorf("api: %w", err)
	}
	st, err := store.Open(filepath.Join(out, build.StoreFile))
	if err != nil {
		return nil, fmt.Errorf("api: %w", err)
	}
	ix, err := search.Open(filepath.Join(out, build.SearchFile), nil)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("api: %w", err), st.Close())
	}
	return &Service{root: c.Root, commit: c.Commit, build: m.Output, store: st, index: ix}, nil
}

// Close closes the store and the index.
func (s *Service) Close() error {
	return errors.Join(s.store.Close(), s.index.Close())
}

// Response is the answer to one call.
type Response struct {
	API    string  `json:"api"`
	Method string  `json:"method"`
	Commit string  `json:"commit,omitempty"` // the commit of the tree that the build read
	Build  string  `json:"build"`            // the output hash of the build
	About  []About `json:"about"`            // each node in the result, in the order of the result
	Result any     `json:"result"`
}

// About is a node that a response is about, as the build saw it.
type About struct {
	Tag      string `json:"tag"`
	Hash     string `json:"hash"`               // the content hash of the node file
	Evidence string `json:"evidence,omitempty"` // the evidence level, or "" when the node has none
}

// The codes of an Error.
const (
	Invalid       = "invalid"        // a parameter is missing or wrong
	NotFound      = "not_found"      // the method, the node or the result does not exist
	RateLimited   = "rate_limited"   // the client sent too many requests
	InternalError = "internal_error" // the server failed
)

// Error is the error of a call.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error returns the message.
func (e *Error) Error() string { return e.Message }

func invalid(format string, a ...any) error {
	return &Error{Code: Invalid, Message: fmt.Sprintf(format, a...)}
}

func notFound(err error) error {
	var e *Error
	if errors.As(err, &e) {
		return err
	}
	return &Error{Code: NotFound, Message: err.Error()}
}

// Call runs the method with the parameters. A parameter value is a string, a bool, a number, or a string that holds a bool or a number. The error is an *Error.
func (s *Service) Call(method string, params map[string]any) (*Response, error) {
	i := slices.IndexFunc(methods, func(m Method) bool { return m.Name == method })
	if i < 0 {
		return nil, &Error{Code: NotFound, Message: fmt.Sprintf("%q is not a method. The methods are %s", method, strings.Join(names(), ", "))}
	}
	m := methods[i]
	a, err := m.args(params)
	if err != nil {
		return nil, err
	}
	result, tags, err := m.run(s, a)
	if err != nil {
		var e *Error
		if errors.As(err, &e) {
			return nil, err
		}
		return nil, &Error{Code: InternalError, Message: err.Error()}
	}
	return &Response{API: Version, Method: m.Name, Commit: s.commit, Build: s.build, About: s.about(tags), Result: result}, nil
}

// about returns the facts of each node, once, in order.
func (s *Service) about(tags []string) []About {
	out := []About{}
	seen := map[string]bool{}
	for _, t := range tags {
		n, ok := s.store.Node(t)
		if t == "" || seen[t] || !ok {
			continue
		}
		seen[t] = true
		a := About{Tag: t, Hash: n.Hash}
		if st, ok := s.store.Status(t); ok && st.Evidence != status.None {
			a.Evidence = st.Evidence.String()
		}
		out = append(out, a)
	}
	return out
}

// tag resolves ref to the tag of one node.
func (s *Service) tag(ref string) (string, error) {
	tags, err := s.store.Resolve(ref)
	if err != nil {
		return "", notFound(err)
	}
	if len(tags) > 1 {
		return "", invalid("%s names %d nodes: %s. Give one tag", ref, len(tags), strings.Join(tags, ", "))
	}
	return tags[0], nil
}

// file reads the node file with the tag. It returns an error when the file is not the file that the build read.
func (s *Service) file(id string) (*node.Node, error) {
	if !tag.IsProvisional(id) && len(id) < tag.ShardLen {
		return nil, fmt.Errorf("the tag %s is too short", id)
	}
	path := filepath.Join("nodes", tag.Shard(id), id+".md")
	data, err := os.ReadFile(filepath.Join(s.root, path)) // #nosec G304 -- the tag comes from the store, thus the path stays in the tree.
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("the file of %s is not in the tree. Run m2 build again", id)
	}
	if err != nil {
		return nil, err
	}
	n, err := node.Parse(filepath.ToSlash(path), data)
	if err != nil {
		return nil, err
	}
	if g, _ := s.store.Node(id); node.Hash(n) != g.Hash {
		return nil, fmt.Errorf("the file of %s changed after the build. Run m2 build again", id)
	}
	return n, nil
}
