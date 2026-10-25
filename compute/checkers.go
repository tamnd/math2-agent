package compute

import (
	"path"
	"slices"
	"strings"
)

// Checker is the fixed checker of one certificate kind (spec 08.3 and 08.8). A change to this table needs the review of a person.
type Checker struct {
	Kind   string   // the certificate kind
	Images []string // the images that can check the kind
	Second bool     // the checker must run in an image that is not the image of the computation
	Text   string   // what checks the certificate
}

var checkers = []Checker{
	{"witness", []string{"m2-lean", "m2-python"}, false, "Lean decide or norm_num, or a short checker program"},
	{"witness-table", []string{"m2-lean", "m2-python"}, false, "a Lean or a short Python checker that runs over the table"},
	{"lrat", []string{"m2-sat"}, false, "cake_lpr, a verified checker in CakeML"},
	{"smt-proof", []string{"m2-lean"}, false, "the SMT reconstruction of Lean"},
	{"primality", []string{"m2-lean"}, false, "a Pratt or ECPP certificate checker in Lean"},
	{"interval", []string{"m2-flint", "m2-julia", "m2-lean"}, false, "a second ball arithmetic, or Lean with exact rationals"},
	{"lp-dual", []string{"m2-lp"}, false, "an exact rational check of the dual certificate (VIPR)"},
	{"sos", []string{"m2-sage", "m2-lean"}, false, "an exact check of the polynomial identity"},
	{"group-id", []string{"m2-gap", "m2-sage", "m2-lean"}, true, "a recomputation in a second system, or Lean for small orders"},
}

// Checkers returns the fixed checkers, one for each certificate kind.
func Checkers() []Checker { return slices.Clone(checkers) }

// CheckerFor returns the fixed checker of a certificate kind.
func CheckerFor(kind string) (Checker, bool) {
	i := slices.IndexFunc(checkers, func(c Checker) bool { return c.Kind == kind })
	if i < 0 {
		return Checker{}, false
	}
	return checkers[i], true
}

// Allows reports whether the checker image named by ref can check a certificate of the computation in the image named by engine.
func (c Checker) Allows(ref, engine string) bool {
	name, _, ok := SplitRef(ref)
	if !ok || !slices.Contains(c.Images, name) {
		return false
	}
	other, _, _ := SplitRef(engine)
	return !c.Second || name != other
}

// commands give the program of each image that runs an entry point. The path of the entry point comes after the arguments. A computation has no stdin, thus a program that reads commands after its file stops at the end of the file.
var commands = map[string][]string{
	"m2-pari":   {"gp", "-q", "-f"},
	"m2-gap":    {"gap", "-q", "-b"},
	"m2-python": {"python3", "-I", "-B"},
	"m2-sage":   {"sage"},
	"m2-m2":     {"M2", "--script"},
	"m2-smt":    {"z3"},
	"m2-sat":    {"cadical"},
	"m2-lp":     {"scip", "-f"},
	"m2-lean":   {"lean", "--run"},
}

// Command returns the command that runs the file at entry in the image name. It returns nil when the toolchain does not know how to run a file in that image.
func Command(name, entry string) []string {
	cmd := commands[name]
	if name == "m2-smt" && strings.HasSuffix(entry, ".cvc5.smt2") {
		cmd = []string{"cvc5"}
	}
	if cmd == nil {
		return nil
	}
	return append(slices.Clone(cmd), path.Clean(entry))
}
