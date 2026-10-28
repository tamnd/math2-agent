package wpcorpus

import (
	"regexp"
	"strings"
)

// The article types of WP1.5.
const (
	TypeConcept   = "concept"
	TypeStatement = "statement"
	TypeObject    = "object"
	TypeMethod    = "method"
	TypeProblem   = "problem"
	TypeField     = "field"
	TypeList      = "list"
	TypeBiography = "biography"
)

// Types are the article types of WP1.5, in the order of the spec.
var Types = []string{TypeConcept, TypeStatement, TypeObject, TypeMethod, TypeProblem, TypeField, TypeList, TypeBiography}

var (
	personWord  = regexp.MustCompile(`\b(mathematicians?|statisticians?|logicians?|physicists?|astronomers?|economists?|philosophers?|engineers?|computer scientists?|scientists?|cryptographers?|professors?|academics?|educators?|geometers?|polymaths?|actuar(y|ies)|chess players?|puzzlers?)\b`)
	fieldShort  = regexp.MustCompile(`^(an? )?(main |major )?(branch|field|area|subfield|discipline|study|theory)\b.*\b(of|in|within)\b`)
	methodWord  = regexp.MustCompile(`\b(algorithms?|methods?|techniques?|procedures?|sort|search|iterations?|sieve)\b`)
	problemWord = regexp.MustCompile(`\b(problems?|puzzles?)\b`)
	objectShort = regexp.MustCompile(`\b(constant|sporadic( simple)? group|polyhedron|polytope|tessellation|tiling|(prime|irrational|transcendental|perfect) number|integer sequence)\b`)
)

// TypeOf gives the first article type of WP1.5 from the title, the short
// description and the categories. The rules look only at these facts, so
// the type can be wrong. The model pass type of WP07 replaces it.
func TypeOf(title, short string, categories []string) string {
	t := strings.ToLower(title)
	for _, p := range []string{"list of ", "lists of ", "glossary of ", "outline of ", "timeline of ", "index of "} {
		if strings.HasPrefix(t, p) {
			return TypeList
		}
	}
	s := strings.ToLower(short)
	base := strings.ToLower(baseTitle(title))
	for _, c := range categories {
		c = strings.ToLower(c)
		if strings.HasSuffix(c, " births") || strings.HasSuffix(c, " deaths") || c == "living people" {
			return TypeBiography
		}
	}
	if personWord.MatchString(s) && !strings.Contains(s, "theorem") {
		return TypeBiography
	}
	unsolved := false
	for _, c := range categories {
		if strings.HasPrefix(strings.ToLower(c), "unsolved problems in ") {
			unsolved = true
		}
	}
	switch {
	case problemWord.MatchString(base) || unsolved:
		return TypeProblem
	case resultWord.MatchString(base):
		return TypeStatement
	case methodWord.MatchString(base) || strings.HasPrefix(s, "algorithm") || strings.HasPrefix(s, "method"):
		return TypeMethod
	case fieldShort.MatchString(s):
		return TypeField
	case objectShort.MatchString(s):
		return TypeObject
	}
	return TypeConcept
}

// typeOf gives the type of an article in the census report: the type of
// the metadata when it is a type of WP1.5, or the type from the title.
func typeOf(a *CensusArticle) string {
	if a.Meta != nil {
		for _, t := range Types {
			if a.Meta.Type == t {
				return t
			}
		}
		return TypeOf(a.Title, a.Meta.Short, a.Meta.Categories)
	}
	return TypeOf(a.Title, "", nil)
}
