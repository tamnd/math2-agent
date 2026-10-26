package api

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/propose"
)

// Unauthorized is the code of an Error of a write with no token, or with a token that GitHub does not accept.
const Unauthorized = "unauthorized"

// Writes returns the write methods, in the order of spec 11.2. Each one opens a pull request against tamnd/math2, and the handler serves them only when Options.Propose is set.
func Writes() []Method { return slices.Clone(writes) }

var (
	tagRef  = Param{Name: "ref", Type: String, Required: true, Summary: "the tag, an alias or an m2:// URI of the node"}
	text    = Param{Name: "text", Type: String, Required: true, Summary: "the text of the node file: the front matter and the body"}
	message = Param{Name: "message", Type: String, Summary: "a message to the reviewers, for the body of the pull request"}
)

var writes = []Method{
	{
		Name: propose.MethodNode, Summary: "Open a pull request that adds a node. The node gets a provisional id, and the mint workflow gives it a tag when the pull request merges.",
		Params: []Param{text, message},
		change: func(_ *Service, root string, a args) (propose.Change, error) {
			return propose.NewNode(root, []byte(a.str("text")))
		},
	},
	{
		Name: propose.MethodEdit, Summary: "Open a pull request that replaces the text of a node. The text keeps the tag and the kind.",
		Params: []Param{tagRef, text, message},
		change: func(s *Service, root string, a args) (propose.Change, error) {
			t, err := s.tag(a.str("ref"))
			if err != nil {
				return propose.Change{}, err
			}
			return propose.Edit(root, t, []byte(a.str("text")))
		},
	},
	{
		Name: propose.MethodBinding, Summary: "Open a pull request that binds a node to a Lean declaration.",
		Params: []Param{tagRef,
			{Name: "decl", Type: String, Required: true, Summary: "the full name of the Lean declaration"},
			{Name: "match", Type: String, Required: true, Summary: "exact, stronger, weaker, variant or partial"},
			{Name: "library", Type: String, Summary: "the library of the declaration. The default is mathlib."},
			{Name: "adapter", Type: String, Summary: "the adapter in Math2/Meta/Match.lean, for a match that is not exact"},
			message},
		change: func(s *Service, root string, a args) (propose.Change, error) {
			t, err := s.tag(a.str("ref"))
			if err != nil {
				return propose.Change{}, err
			}
			return propose.Bind(root, t, propose.Binding{Library: a.str("library"), Decl: a.str("decl"), Match: a.str("match"), Adapter: a.str("adapter")})
		},
	},
	{
		Name: propose.MethodAlignment, Summary: "Open a pull request that adds a same_as, close_to or aligned edge to a node.",
		Params: []Param{tagRef,
			{Name: "edge", Type: String, Required: true, Summary: "same_as, close_to or aligned"},
			{Name: "id", Type: String, Required: true, Summary: "the id in the external system, or the m2:// URI of the paper result of aligned"},
			{Name: "system", Type: String, Summary: "the external system of same_as and close_to, for example wikidata"},
			{Name: "note", Type: String, Summary: "how the two differ, for close_to"},
			{Name: "match", Type: String, Summary: "the match of aligned: exact, stronger, weaker, variant or partial"},
			message},
		change: func(s *Service, root string, a args) (propose.Change, error) {
			t, err := s.tag(a.str("ref"))
			if err != nil {
				return propose.Change{}, err
			}
			return propose.Align(root, t, propose.Alignment{Edge: a.str("edge"), System: a.str("system"), ID: a.str("id"), Note: a.str("note"), Match: a.str("match")})
		},
	},
}

// write serves one write method with the proposer.
func (s *Service) write(p *propose.Proposer, m Method) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearer(r)
		if !ok {
			fail(w, m.Name, &Error{Code: Unauthorized, Message: "a write needs the header Authorization: Bearer and the GitHub token of a person. The pull request opens under that identity"})
			return
		}
		params, err := body(w, r)
		if err != nil {
			fail(w, m.Name, err)
			return
		}
		a, err := m.args(params)
		if err != nil {
			fail(w, m.Name, err)
			return
		}
		c, err := m.change(s, p.Root, a)
		if err != nil {
			var e *Error
			if !errors.As(err, &e) {
				err = &Error{Code: Invalid, Message: err.Error()}
			}
			fail(w, m.Name, err)
			return
		}
		res, err := p.Open(r.Context(), token, c, a.str("message"))
		var ae *propose.AuditError
		switch {
		case errors.Is(err, propose.ErrUnauthorized):
			fail(w, m.Name, &Error{Code: Unauthorized, Message: err.Error()})
		case errors.Is(err, propose.ErrLimit):
			fail(w, m.Name, &Error{Code: RateLimited, Message: err.Error()})
		case errors.As(err, &ae):
			fail(w, m.Name, &Error{Code: Invalid, Message: err.Error()})
		case err != nil:
			fail(w, m.Name, err)
		default:
			write(w, http.StatusOK, &Response{API: Version, Method: m.Name, Commit: s.commit, Build: s.build, About: s.about([]string{c.Tag}), Result: res})
		}
	}
}

// bearer returns the token of the header Authorization: Bearer TOKEN.
func bearer(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	token = strings.TrimSpace(token)
	return token, ok && strings.EqualFold(scheme, "Bearer") && token != ""
}
