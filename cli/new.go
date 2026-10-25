package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/schema"
	"github.com/tamnd/math2-agent/tag"
)

func runNew(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 new", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: m2 new [-root DIR] <kind>")
		return ExitUsage
	}
	s, err := schema.Load(*root)
	if err != nil {
		fmt.Fprintf(stderr, "m2 new: %v\n", err)
		return ExitFailure
	}
	kind := fs.Arg(0)
	if !slices.Contains(s.Kinds(), kind) {
		fmt.Fprintf(stderr, "m2 new: %q is not a kind. The kinds are %s.\n", kind, strings.Join(s.Kinds(), ", "))
		return ExitUsage
	}
	path, err := newNode(*root, s, kind)
	if err != nil {
		fmt.Fprintf(stderr, "m2 new: %v\n", err)
		return ExitFailure
	}
	fmt.Fprintln(stdout, path)
	return ExitOK
}

// newNode writes a new node file of the kind to nodes/~/ and returns its path. It draws a new provisional id when a file with the id exists.
func newNode(root string, s *schema.Schema, kind string) (string, error) {
	dir := filepath.Join(root, "nodes", "~")
	if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- content directories are public.
		return "", err
	}
	for range 8 {
		id, err := tag.Provisional(nil)
		if err != nil {
			return "", err
		}
		data, err := s.Template(kind, id)
		if err != nil {
			return "", err
		}
		path := filepath.Join(dir, id+".md")
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) // #nosec G302 G304 -- content files are public text in a git tree.
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := f.Write(data); err != nil {
			_ = f.Close()
			return "", err
		}
		return path, f.Close()
	}
	return "", errors.New("each provisional id that m2 new drew is in use")
}
