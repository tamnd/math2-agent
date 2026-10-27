package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/tamnd/math2-agent/pack"
)

const packUsage = "m2 pack [-root DIR] [-out DIR] [-build DIR] [-site URL] [-check] [-json] [-q] <volume>"

// runPack compiles a volume, writes its outputs and its lock file, and runs the pack checks PC1 to PC10 (corpus K11).
func runPack(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 pack", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	out := fs.String("out", "", "the output directory. Empty means build/volumes/<id> in the tree.")
	buildDir := fs.String("build", "", "a build output directory with status.json and writing.json. Empty means that m2 pack computes the levels from the tree.")
	site := fs.String("site", pack.DefaultSite, "the site that the citations of nodes outside the volume link to")
	checkOnly := fs.Bool("check", false, "run the pack checks and write no file")
	asJSON := fs.Bool("json", false, "write the report as JSON")
	quiet := fs.Bool("q", false, "do not write the table of check states")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(stderr, "usage: %s\n", packUsage)
		return ExitUsage
	}
	v, err := pack.Load(*root, fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "m2 pack: %v\n", err)
		return ExitFailure
	}
	d, err := pack.Pack(*root, v, pack.Options{Build: *buildDir, Site: *site})
	if err != nil {
		fmt.Fprintf(stderr, "m2 pack: %v\n", err)
		return ExitFailure
	}
	rep := d.Check()
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintf(stderr, "m2 pack: %v\n", err)
			return ExitFailure
		}
	} else {
		for _, f := range rep.Findings {
			fmt.Fprintln(stdout, f)
		}
		if !*quiet {
			_ = rep.WriteSummary(stderr)
		}
	}
	if *checkOnly {
		if rep.Failed() {
			return ExitFailure
		}
		return ExitOK
	}
	dir := *out
	if dir == "" {
		dir = filepath.Join(*root, "build", pack.Dir, v.ID)
	}
	if err := writePack(d, *root, dir, v.Outputs()); err != nil {
		fmt.Fprintf(stderr, "m2 pack: %v\n", err)
		return ExitFailure
	}
	if !*asJSON {
		fmt.Fprintf(stderr, "m2 pack: wrote %s and %s\n", dir, pack.LockFile(v.ID))
	}
	if rep.Failed() {
		return ExitFailure
	}
	return ExitOK
}

// writePack writes the outputs of the document to dir and the lock file to the tree. The HTML is at the top of dir, and each other format is in a directory with the name of the format.
func writePack(d *pack.Document, root, dir string, formats []string) error {
	for _, f := range formats {
		if !slices.Contains(pack.Backends, f) {
			continue // PC9 reports the format
		}
		files, err := d.Output(f)
		if err != nil {
			return err
		}
		sub := dir
		if f != pack.HTML {
			sub = filepath.Join(dir, f)
		}
		for _, name := range slices.Sorted(maps.Keys(files)) {
			p := filepath.Join(sub, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil { // #nosec G301 -- the volume is public.
				return err
			}
			if err := os.WriteFile(p, files[name], 0o644); err != nil { // #nosec G306 -- the volume is public.
				return err
			}
		}
	}
	lock, err := d.Lock().Marshal()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, filepath.FromSlash(pack.LockFile(d.Volume.ID))), lock, 0o644) // #nosec G306 -- the lock file is part of the working tree.
}
