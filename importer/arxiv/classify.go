package arxiv

import (
	"regexp"
	"slices"
	"strings"
)

// envItems gives the item kind of AX3.3 for a title or an environment name, in the normalized form of envKey. It is the part of manifests/environments.yaml of tamnd/arxiv-reader that a paper also writes as an environment name, as lem or thm, so that an object that an old version of ax left of the kind unknown gets its kind from code and not from the pass classify (AX7.2).
var envItems = map[string]string{}

func init() {
	for item, keys := range map[string][]string{
		"statement/theorem":     {"theorem", "thm", "theo", "thrm", "mainthm", "maintheorem", "namedthm", "thmx", "thma", "satz", "mainstatement"},
		"statement/lemma":       {"lemma", "lem", "lemm", "lema", "sublemma", "keylemma"},
		"statement/proposition": {"proposition", "prop", "prp", "propo"},
		"statement/corollary":   {"corollary", "cor", "coro", "corol"},
		"statement/claim":       {"claim", "clm"},
		"statement/fact":        {"fact", "observation", "obs"},
		"statement":             {"statement", "stat", "sta", "stmt"},
		"definition":            {"definition", "defn", "def", "dfn", "defi", "dfntn"},
		"example":               {"example", "ex", "exa", "exam", "exmp", "eg"},
		"counterexample":        {"counterexample"},
		"conjecture":            {"conjecture", "conj"},
		"problem":               {"problem", "question", "prob", "ques", "qn"},
		"exercise":              {"exercise", "exer", "exo"},
		"context/hypothesis":    {"assumption", "assum", "assumabbr", "assump", "hypothesis", "hyp", "setup", "setting", "context", "choice"},
		"remark":                {"remark", "rem", "rmk", "rk", "note", "caveat", "warning"},
		"other/section":         {"conclusion", "conclusions", "summary", "discussion", "ack", "acks", "acknowledgement", "acknowledgements", "acknowledgment", "acknowledgments", "funding", "appendices", "appendix", "todo"},
		"proof":                 {"proof", "proofof", "pf", "prf"},
	} {
		for _, k := range keys {
			envItems[k] = item
		}
	}
}

var (
	// nameMacro is a title that a macro of the paper prints, as \protect\theoremname or \lemmaname of babel.
	nameMacro = regexp.MustCompile(`^\\(?:protect\s*)?\\?([A-Za-z]+?)name\b`)
	nonLetter = regexp.MustCompile(`[^a-z]+`)
)

// envKey is the normalized form of a title or an environment name: lower case, letters only, and a trailing s removed when the rest is a key, as "Theorems" or "thm*".
func envKey(s string) string {
	s = strings.TrimSpace(s)
	if m := nameMacro.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	k := nonLetter.ReplaceAllString(strings.ToLower(s), "")
	if _, ok := envItems[k]; !ok && strings.HasSuffix(k, "s") {
		if _, ok := envItems[k[:len(k)-1]]; ok {
			return k[:len(k)-1]
		}
	}
	return k
}

// ItemOf returns the item kind of an object. It is the item of the record, unless ax left it unknown. Then the title decides, as in AX3.3, and else the name of the environment. An object whose title and environment say nothing stays unknown, for the pass classify. The rule is a rule of code, with no model, so the same records give the same kinds.
func ItemOf(o *Object) string {
	if o.Item != "" && o.Item != "unknown" {
		return o.Item
	}
	if it, ok := envItems[envKey(o.Title)]; ok {
		return it
	}
	if it, ok := envItems[envKey(o.Env)]; ok {
		return it
	}
	return o.Item
}

// categoryMSC gives the MSC2020 areas of the arXiv math categories, after the descriptions of the categories on arXiv. A category with more than one area gives each of them, except math.CA, which names eight areas and gives only its main one, 26: a paper of math.CA in another area says so with its own MSC codes.
var categoryMSC = map[string][]string{
	"math.AC": {"13"}, "math.AG": {"14"}, "math.AP": {"35"}, "math.AT": {"55"},
	"math.CA": {"26"}, "math.CO": {"05"}, "math.CT": {"18"},
	"math.CV": {"30", "32"}, "math.DG": {"53"}, "math.DS": {"37"}, "math.FA": {"46"}, "math.GM": {"00"},
	"math.GN": {"54"}, "math.GR": {"20"}, "math.GT": {"57"}, "math.HO": {"01"}, "math.IT": {"94"},
	"math.KT": {"19"}, "math.LO": {"03"}, "math.MG": {"51", "52"}, "math.MP": {"81"}, "math-ph": {"81"},
	"math.NA": {"65"}, "math.NT": {"11"}, "math.OA": {"46", "47"}, "math.OC": {"49", "90", "93"},
	"math.PR": {"60"}, "math.QA": {"17", "81"}, "math.RA": {"16"}, "math.RT": {"20", "22"},
	"math.SG": {"53"}, "math.SP": {"35", "47", "58"}, "math.ST": {"62"}, "stat.TH": {"62"},
}

// CategoryMSC returns the MSC areas of the categories of a paper, as codes of the form 14-XX, in the order of the categories and without repeats. The importer uses them when the msc-class field gives no code, so that the mention is in a block (AX8.1).
func CategoryMSC(cats []string) []string {
	var out []string
	for _, c := range cats {
		for _, a := range categoryMSC[c] {
			if code := a + "-XX"; !slices.Contains(out, code) {
				out = append(out, code)
			}
		}
	}
	return out
}
