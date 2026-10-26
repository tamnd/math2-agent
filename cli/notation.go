package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/tamnd/math2-agent/notation"
)

const notationUsage = `m2 notation build [-root DIR] [-check]
       m2 notation seed [-root DIR]`

// runNotation writes the macro files from the notation register, or makes a first register from the macro file (corpus K10.4).
func runNotation(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "build" && args[0] != "seed") {
		fmt.Fprintln(stderr, "usage: "+notationUsage)
		return ExitUsage
	}
	sub := args[0]
	flags := flag.NewFlagSet("m2 notation "+sub, flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree")
	checkOnly := false
	if sub == "build" {
		flags.BoolVar(&checkOnly, "check", false, "change no file. Write the path of each macro file that is not the output of the register, and exit with code 1 if there is one.")
	}
	if err := flags.Parse(args[1:]); err != nil {
		return ExitUsage
	}
	if flags.NArg() > 0 {
		fmt.Fprintln(stderr, "usage: "+notationUsage)
		return ExitUsage
	}
	var err error
	code := ExitOK
	if sub == "build" {
		code, err = notationBuild(*root, checkOnly, stdout)
	} else {
		err = notationSeed(*root, stdout)
	}
	if err != nil {
		fmt.Fprintf(stderr, "m2 notation %s: %v\n", sub, err)
		return ExitFailure
	}
	return code
}

// notationBuild writes macros/m2.sty and macros/m2.katex.json from the register, and writes the path of each file that it changes. With checkOnly, it changes no file and returns ExitFailure when a file is not the output of the register.
func notationBuild(root string, checkOnly bool, stdout io.Writer) (int, error) {
	r, err := notation.Load(root)
	if err != nil {
		return ExitFailure, err
	}
	outputs := r.Outputs()
	names := make([]string, 0, len(outputs))
	for name := range outputs {
		names = append(names, name)
	}
	slices.Sort(names)
	code := ExitOK
	for _, name := range names {
		path := filepath.Join(root, filepath.FromSlash(name))
		old, err := os.ReadFile(path) // #nosec G304 -- the path is in the working tree, which the caller owns.
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return ExitFailure, err
		}
		if bytes.Equal(old, outputs[name]) {
			continue
		}
		if checkOnly {
			code = ExitFailure
		} else {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- the macro files are public text in a git tree.
				return ExitFailure, err
			}
			if err := os.WriteFile(path, outputs[name], 0o644); err != nil { // #nosec G306 -- the macro files are public text in a git tree.
				return ExitFailure, err
			}
		}
		fmt.Fprintln(stdout, name)
	}
	return code, nil
}

// notationSeed writes notation/global.yaml with an entry for each macro of macros/m2.sty. It does not change a register that is there.
func notationSeed(root string, stdout io.Writer) error {
	if _, err := notation.Load(root); err == nil {
		return errors.New("the tree has a notation register. Seed makes only a first register")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	sty, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(notation.StyFile))) // #nosec G304 -- the path is in the working tree, which the caller owns.
	if err != nil {
		return err
	}
	entries, err := notation.Seed(sty)
	if err != nil {
		return fmt.Errorf("%s: %w", notation.StyFile, err)
	}
	data, err := notation.Marshal(entries)
	if err != nil {
		return err
	}
	name := notation.Dir + "/" + notation.Global + ".yaml"
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- the register is public text in a git tree.
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil { // #nosec G306 G703 -- the user names the root, and the register is public text in a git tree.
		return err
	}
	fmt.Fprintln(stdout, name)
	return nil
}
