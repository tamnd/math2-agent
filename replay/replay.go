// Package replay checks a formal declaration again, outside the process that elaborated it (spec 07.7, layers 2 to 4).
//
// [Checker.Check] writes a challenge module. The challenge states the declaration again from its recorded statement, with sorry as the proof, and it imports the modules that the module of the declaration imports. The checker exports the challenge and the module of the declaration with leanexport, and runs lake comparator on the two exports. The comparator does three checks. The declaration has the statement of the challenge. The proof uses only the permitted axioms. The Lean kernel accepts the export of the proof. When [Options.Paranoid] is set, each independent kernel of the toolchain must also accept the export: the paranoid checker, lean4lean, nanoda, con-leche and con-ron.
//
// The comparator runs the checkers in a bwrap sandbox, and bwrap needs Linux. [Options.NoSandbox] runs the checkers without the sandbox, for example on macOS. Such a result is not trustworthy, and [Result.Sandboxed] is false.
package replay

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/lean"
)

// Options are the options of a Checker.
type Options struct {
	Paranoid  bool     // replay the proof in each independent kernel of the toolchain, and not only in the Lean kernel
	NoSandbox bool     // run the checkers without the bwrap sandbox. The result is not trustworthy.
	Axioms    []string // the permitted axioms. Empty means lean.StandardAxioms.
	Work      string   // the directory for the temporary files. Empty means the default directory for temporary files.
}

// Target is a declaration to check.
type Target struct {
	Decl        string   // the declaration
	Module      string   // the module that declares Decl
	LevelParams []string // the universe parameters of Decl, in order
	Statement   string   // the statement of Decl. The challenge states it again, thus pp.all output is best.
}

// TargetOf returns the target for an extraction record.
func TargetOf(r lean.Record) Target {
	return Target{Decl: r.Decl, Module: r.Module, LevelParams: r.LevelParams, Statement: r.StatementPPAll}
}

// Result is the result of the check of one declaration.
type Result struct {
	Decl      string   `json:"decl"`
	Passed    bool     `json:"passed"`
	Sandboxed bool     `json:"sandboxed"`        // the checkers ran in the sandbox
	Kernels   []string `json:"kernels"`          // the programs of the kernels that accept the proof
	Reason    string   `json:"reason,omitempty"` // why the check failed
}

// kernels gives the program of each kernel, by the name that lake comparator prints.
var kernels = map[string]string{
	"Lean default":  "leanchecker",
	"Lean paranoid": "leanchecker-paranoid",
	"lean4lean":     "lean4lean",
	"nanoda":        "nanoda_bin",
	"con-leche":     "con-leche",
	"con-ron":       "con-ron",
}

// primitive are the constants that the kernel has built in. lake comparator compares them in the two exports, thus each export must have them. The list is primitiveTargets in Lake/CLI/Check.lean of the toolchain.
var primitive = []string{
	"Nat.add", "Nat.sub", "Nat.mul", "Nat.pow", "Nat.gcd", "Nat.div", "Nat.mod", "Nat.beq", "Nat.ble",
	"Nat.land", "Nat.lor", "Nat.xor", "Nat.shiftLeft", "Nat.shiftRight",
	"String.ofList", "Char.ofNat", "List", "eagerReduce", "Nat", "String", "String.mk", "Char",
	"optParam", "autoParam", "semiOutParam", "outParam",
}

// challenge is the module name of the challenge.
const challenge = "M2Challenge"

// A Checker checks declarations of one Lake project. A Checker is safe for concurrent use.
type Checker struct {
	project  string
	opt      Options
	leanPath string
	srcPath  []string
}

// New returns a Checker for the Lake project in project. The project must have built the modules of the declarations to check. New needs lake on the PATH.
func New(ctx context.Context, project string, opt Options) (*Checker, error) {
	if len(opt.Axioms) == 0 {
		opt.Axioms = lean.StandardAxioms
	}
	c := &Checker{project: project, opt: opt}
	var err error
	if c.leanPath, err = c.env(ctx, "LEAN_PATH"); err != nil {
		return nil, err
	}
	src, err := c.env(ctx, "LEAN_SRC_PATH")
	if err != nil {
		return nil, err
	}
	c.srcPath = filepath.SplitList(src)
	return c, nil
}

func (c *Checker) env(ctx context.Context, name string) (string, error) {
	out, err := c.run(ctx, nil, "lake", "env", "printenv", name)
	return strings.TrimSpace(out), err
}

// Check checks one declaration. A declaration that fails a check gives a Result with Passed false and a Reason. An error means that the check did not run.
func (c *Checker) Check(ctx context.Context, t Target) (Result, error) {
	res := Result{Decl: t.Decl, Sandboxed: !c.opt.NoSandbox, Kernels: []string{}}
	if t.Decl == "" || t.Module == "" || strings.TrimSpace(t.Statement) == "" {
		return res, fmt.Errorf("replay: the target %q has no declaration, module or statement", t.Decl)
	}
	imports, err := c.imports(t.Module)
	if err != nil {
		return res, err
	}
	dir, err := os.MkdirTemp(c.opt.Work, "m2-replay-")
	if err != nil {
		return res, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	if err := os.WriteFile(filepath.Join(dir, challenge+".lean"), []byte(source(t, imports)), 0o600); err != nil {
		return res, err
	}
	path := []string{"LEAN_PATH=" + dir + string(filepath.ListSeparator) + c.leanPath}
	if out, err := c.run(ctx, path, "lean", "--root="+dir, "-o", filepath.Join(dir, challenge+".olean"), filepath.Join(dir, challenge+".lean")); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return res, err
		}
		res.Reason = "the recorded statement does not elaborate again: " + firstError(out)
		return res, nil
	}

	targets := slices.Concat([]string{t.Decl}, c.opt.Axioms, primitive)
	if slices.Contains(c.opt.Axioms, "Quot.sound") {
		targets = append(targets, "Quot", "Quot.mk", "Quot.lift", "Quot.ind")
	}
	challengeExport := filepath.Join(dir, "challenge.ndjson")
	solutionExport := filepath.Join(dir, "solution.ndjson")
	if err := c.export(ctx, path, challenge, targets, challengeExport); err != nil {
		return res, err
	}
	if err := c.export(ctx, nil, t.Module, targets, solutionExport); err != nil {
		return res, err
	}

	config, err := json.Marshal(map[string]any{
		"challenge_module": challenge,
		"solution_module":  t.Module,
		"theorem_names":    []string{t.Decl},
		"permitted_axioms": c.opt.Axioms,
	})
	if err != nil {
		return res, err
	}
	configFile := filepath.Join(dir, "comparator.json")
	if err := os.WriteFile(configFile, config, 0o600); err != nil {
		return res, err
	}
	args := []string{"comparator", "--config", configFile, "--challenge-from-export", challengeExport, "--solution-from-export", solutionExport}
	if c.opt.Paranoid {
		args = append(args, "--paranoid")
	}
	if c.opt.NoSandbox {
		args = append(args, "--inadvisably-no-sandbox")
	}
	out, err := c.run(ctx, nil, "lake", args...)
	for line := range strings.Lines(out) {
		name, ok := strings.CutSuffix(strings.TrimSpace(line), " kernel accepts the solution")
		if p, known := kernels[name]; ok && known {
			res.Kernels = append(res.Kernels, p)
		}
	}
	slices.Sort(res.Kernels)
	var exit *exec.ExitError
	switch {
	case err == nil && strings.Contains(out, "Your solution is okay!"):
		res.Passed = true
	case err == nil:
		return res, fmt.Errorf("replay: lake comparator exited with 0 and did not accept %s:\n%s", t.Decl, tail(out))
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		res.Reason = "lake comparator rejects the declaration: " + firstError(out)
	default:
		return res, fmt.Errorf("replay: lake comparator did not run: %w\n%s", err, tail(out))
	}
	return res, nil
}

// source returns the challenge module for t.
func source(t Target, imports []string) string {
	var b strings.Builder
	for _, m := range imports {
		fmt.Fprintf(&b, "import %s\n", m)
	}
	b.WriteString("\n-- The challenge of m2. It states the declaration again from its record, without the proof. A name that is not known is an error, and not a new variable.\n")
	b.WriteString("set_option autoImplicit false\n\n")
	name := t.Decl
	if len(t.LevelParams) > 0 {
		name += ".{" + strings.Join(t.LevelParams, ", ") + "}"
	}
	fmt.Fprintf(&b, "theorem %s :\n%s :=\n  sorry\n", name, t.Statement)
	return b.String()
}

// imports returns the modules that module imports. It reads the header of the source of the module in the source path of the project. A module of the core library has no source in the source path. Each module imports Init, thus the challenge of an Init module imports nothing.
func (c *Checker) imports(module string) ([]string, error) {
	rel := filepath.Join(strings.Split(module, ".")...) + ".lean"
	for _, dir := range c.srcPath {
		f, err := os.Open(filepath.Join(dir, rel)) // #nosec G304 -- the path is in the source path of the project.
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		return header(f)
	}
	if module == "Init" || strings.HasPrefix(module, "Init.") {
		return nil, nil
	}
	return nil, fmt.Errorf("replay: the source of the module %s is not in the source path of the project", module)
}

// header returns the imports in the header of a Lean source file. The header has an optional module keyword, an optional prelude keyword and the imports. An import has the modifiers public, meta and all.
func header(r io.Reader) ([]string, error) {
	var out []string
	s := bufio.NewScanner(r)
	block := false
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if block {
			if _, after, ok := strings.Cut(line, "-/"); ok {
				block = false
				line = strings.TrimSpace(after)
			} else {
				continue
			}
		}
		if i := strings.Index(line, "--"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if strings.HasPrefix(line, "/-") && !strings.HasPrefix(line, "/-!") && !strings.HasPrefix(line, "/--") {
			if !strings.Contains(line[2:], "-/") {
				block = true
			}
			continue
		}
		words := strings.Fields(line)
		if len(words) == 0 {
			continue
		}
		if words[0] == "module" || words[0] == "prelude" {
			continue
		}
		for len(words) > 0 && (words[0] == "public" || words[0] == "meta") {
			words = words[1:]
		}
		if len(words) == 0 || words[0] != "import" {
			break
		}
		words = words[1:]
		if len(words) > 0 && words[0] == "all" {
			words = words[1:]
		}
		out = append(out, words...)
	}
	return out, s.Err()
}

// export writes the export of the closure of decls in module to file.
func (c *Checker) export(ctx context.Context, env []string, module string, decls []string, file string) error {
	f, err := os.Create(file) // #nosec G304 -- the file is in the temporary directory of the check.
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "lake", slices.Concat([]string{"env", "leanexport", module, "--"}, decls)...) // #nosec G204 -- no shell reads the arguments.
	cmd.Dir = c.project
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = f
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err = cmd.Run()
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("replay: leanexport %s: %w\n%s", module, err, tail(stderr.String()))
	}
	return nil
}

// run runs a program in the project with the extra environment env, and returns the standard output and the standard error together.
func (c *Checker) run(ctx context.Context, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 -- the programs are fixed in this package, and no shell reads the arguments.
	cmd.Dir = c.project
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// firstError returns the first line of out that reports an error, or the last line of out.
func firstError(out string) string {
	var last string
	for line := range strings.Lines(out) {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "error") {
			return line
		}
		if line != "" {
			last = line
		}
	}
	return last
}

// tail returns the last 20 lines of out.
func tail(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
	}
	return strings.Join(lines, "\n")
}
