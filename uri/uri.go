// Package uri parses and prints m2:// URIs (spec 04.4).
//
// A URI names a node by its tag. It can add a release, a commit or a section:
//
//	m2://7K3QX2            the node at the latest release
//	m2://7K3QX2@2026.11    the node at release 2026.11
//	m2://7K3QX2@c4f1e9a    the node at a git commit
//	m2://7K3QX2#proof      a section of the node
//
// An alias URI names a node through a namespace, for example m2://concept/compact-space or m2://lean/Subgroup.card_subgroup_dvd_card. An alias resolves to a tag. Never store an alias as a reference (rule T04).
package uri

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/tamnd/math2-agent/tag"
)

// Scheme is the URI scheme of math2.
const Scheme = "m2://"

var (
	releaseRE = regexp.MustCompile(`^[0-9]{4}\.(0[1-9]|1[0-2])(\.[1-9][0-9]*)?$`)
	commitRE  = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	sectionRE = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	slugRE    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

// Kinds are the 11 node kinds. Each kind is also an alias namespace.
var Kinds = []string{"concept", "definition", "statement", "conjecture", "problem", "example", "counterexample", "object", "proof", "computation", "topic"}

// Namespaces are the alias namespaces that are not kinds.
var Namespaces = []string{"msc", "lean", "release", "ext"}

// URI is a parsed m2:// URI. Exactly one of Tag and Namespace is set.
type URI struct {
	Tag       string // the tag, for a node URI
	Release   string // the release after @, for example 2026.11
	Commit    string // the commit after @
	Section   string // the section after #, for example proof
	Namespace string // the alias namespace, for example concept or lean
	Name      string // the alias name in the namespace
}

// ErrNotM2 means that the string does not start with m2://.
var ErrNotM2 = errors.New("uri: not an m2:// URI")

// Parse parses s as an m2:// URI.
func Parse(s string) (URI, error) {
	rest, ok := strings.CutPrefix(s, Scheme)
	if !ok {
		return URI{}, ErrNotM2
	}
	if ns, name, ok := strings.Cut(rest, "/"); ok {
		return parseAlias(s, ns, name)
	}
	var u URI
	rest, u.Section, _ = strings.Cut(rest, "#")
	rest, at, hasAt := strings.Cut(rest, "@")
	if !tag.IsTag(rest) {
		return URI{}, fmt.Errorf("uri: %q: %q is not a tag", s, rest)
	}
	u.Tag = rest
	switch {
	case !hasAt:
	case releaseRE.MatchString(at):
		u.Release = at
	case commitRE.MatchString(at):
		u.Commit = at
	default:
		return URI{}, fmt.Errorf("uri: %q: %q is not a release or a commit", s, at)
	}
	if strings.Contains(s, "#") && !sectionRE.MatchString(u.Section) {
		return URI{}, fmt.Errorf("uri: %q: %q is not a section name", s, u.Section)
	}
	return u, nil
}

func parseAlias(s, ns, name string) (URI, error) {
	if name == "" {
		return URI{}, fmt.Errorf("uri: %q: the alias has no name", s)
	}
	switch {
	case isKind(ns):
		if !tag.IsTag(name) && !slugRE.MatchString(name) {
			return URI{}, fmt.Errorf("uri: %q: %q is not a tag or a slug", s, name)
		}
	case ns == "release":
		if !releaseRE.MatchString(name) {
			return URI{}, fmt.Errorf("uri: %q: %q is not a release", s, name)
		}
	case ns == "ext":
		system, id, ok := strings.Cut(name, "/")
		if !ok || system == "" || id == "" {
			return URI{}, fmt.Errorf("uri: %q: write m2://ext/<system>/<id>", s)
		}
	case ns == "msc" || ns == "lean":
	default:
		return URI{}, fmt.Errorf("uri: %q: %q is not an alias namespace", s, ns)
	}
	return URI{Namespace: ns, Name: name}, nil
}

func isKind(s string) bool {
	for _, k := range Kinds {
		if k == s {
			return true
		}
	}
	return false
}

// IsAlias reports whether u is an alias URI.
func (u URI) IsAlias() bool { return u.Namespace != "" }

// String prints u in the form that Parse reads.
func (u URI) String() string {
	if u.IsAlias() {
		return Scheme + u.Namespace + "/" + u.Name
	}
	s := Scheme + u.Tag
	switch {
	case u.Release != "":
		s += "@" + u.Release
	case u.Commit != "":
		s += "@" + u.Commit
	}
	if u.Section != "" {
		s += "#" + u.Section
	}
	return s
}
