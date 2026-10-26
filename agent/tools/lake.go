package tools

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/sandbox"
)

// Lake is the [Lean] environment of the Lake project in the directory lean/ of a work tree of math2.
type Lake struct {
	Tree    string   // the directory of the work tree
	Exe     string   // the M2Extract program, from lean.Install. With a Box, it is the path of the program in the image.
	Modules []string // the modules that Extract imports. Empty means Math2.
	Box     *Box     // the sandbox of the Lean programs. Nil means that they run on the host.
}

// Box is the sandbox of the Lean programs of a [Lake] (spec 11.7). Each run of lake, of Lean and of M2Extract is a container of the image, with no network. The directory lean/ of the work tree is writable at /out, and the Lake packages of the tree, such as Mathlib, are read-only at /work. Thus lean/.lake/packages in the work tree must be a link to /work, and the user 65534 must be able to write in lean/ of the work tree.
type Box struct {
	Docker   *sandbox.Docker
	Image    string // the image by digest. It has lake, the Lean toolchain of the tree and M2Extract.
	Packages string // the directory of the host with the Lake packages of the tree, an absolute path
	CPU      int    // the number of CPUs. 0 means 1.
	Memory   int64  // the memory in bytes. 0 means sandbox.DefaultMemory.
}

func (l *Lake) project() string { return filepath.Join(l.Tree, "lean") }

// lake returns the command that runs lake with the arguments in the project, on the host or in the sandbox.
func (l *Lake) lake(ctx context.Context, args ...string) (*exec.Cmd, error) {
	if l.Box == nil {
		cmd := exec.CommandContext(ctx, "lake", args...) // #nosec G204 -- the arguments are module names from import lines and paths of the tree, and no shell reads them.
		cmd.Dir = l.project()
		return cmd, nil
	}
	project, err := filepath.Abs(l.project())
	if err != nil {
		return nil, err
	}
	return l.Box.Docker.Command(ctx, sandbox.Spec{
		Image:   l.Box.Image,
		Command: append([]string{"lake", "-d", sandbox.OutDir}, args...),
		Dir:     l.Box.Packages,
		Out:     project,
		CPU:     l.Box.CPU,
		Memory:  l.Box.Memory,
	})
}

// Check builds the modules of Math2 that the file imports, then elaborates the file. A build that fails gives an Elaboration with one error that has the end of the output of Lake.
func (l *Lake) Check(ctx context.Context, file string) (*lean.Elaboration, error) {
	rel, ok := strings.CutPrefix(file, "lean/")
	if !ok {
		return nil, fmt.Errorf("tools: %s is not in lean/", file)
	}
	data, err := os.ReadFile(filepath.Join(l.project(), filepath.FromSlash(rel)))
	if err != nil {
		return nil, fmt.Errorf("tools: %w", err)
	}
	var seconds float64
	if mods := Imports(data); len(mods) > 0 {
		b := l.build(ctx, mods...)
		if len(b.Messages) > 0 {
			return b, nil
		}
		seconds = b.Seconds
	}
	var e *lean.Elaboration
	if l.Box == nil {
		e, err = lean.Elaborate(ctx, l.project(), rel)
	} else {
		var cmd *exec.Cmd
		if cmd, err = l.lake(ctx, "env", "lean", "--json", sandbox.OutDir+"/"+rel); err == nil {
			e, err = lean.ElaborateWith(ctx, cmd, rel)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("tools: %w", err)
	}
	e.Seconds += seconds
	return e, nil
}

// Extract builds the library of the tree, then extracts the declarations with M2Extract. With checks, it runs the degenerate-statement checks on each statement, with lean.CheckHeartbeats. A build that fails is an error.
func (l *Lake) Extract(ctx context.Context, decls []string, checks bool) ([]lean.Record, error) {
	if b := l.build(ctx); len(b.Messages) > 0 {
		return nil, fmt.Errorf("tools: the build of the work tree fails: %s", b.Messages[0].Text)
	}
	cmd, err := l.lake(ctx, "env", l.Exe)
	if err != nil {
		return nil, fmt.Errorf("tools: %w", err)
	}
	s, err := lean.StartWith(cmd)
	if err != nil {
		return nil, fmt.Errorf("tools: %w", err)
	}
	defer func() { _ = s.Close() }()
	mods := l.Modules
	if len(mods) == 0 {
		mods = []string{"Math2"}
	}
	if err := s.Import(mods...); err != nil {
		return nil, fmt.Errorf("tools: %w", err)
	}
	rs, err := s.Extract(decls, nil)
	if err != nil || !checks {
		return rs, err
	}
	var statements []string
	for _, r := range rs {
		if r.Statement() {
			statements = append(statements, r.Decl)
		}
	}
	if len(statements) == 0 {
		return rs, nil
	}
	cs, err := s.Checks(lean.CheckHeartbeats, statements...)
	if err != nil {
		return nil, fmt.Errorf("tools: %w", err)
	}
	for i := range rs {
		if c, ok := cs[rs[i].Decl]; ok {
			rs[i].Checks = &c
		}
	}
	return rs, nil
}

// build runs lake build on the targets, or on the default targets when there are none. The result has no messages when the build passes, and one error when it fails.
func (l *Lake) build(ctx context.Context, targets ...string) *lean.Elaboration {
	start := time.Now()
	cmd, err := l.lake(ctx, append([]string{"build", "-q"}, targets...)...)
	var out []byte
	if err == nil {
		out, err = cmd.CombinedOutput()
	}
	if err == nil {
		return &lean.Elaboration{Messages: []lean.Message{}, Seconds: time.Since(start).Seconds()}
	}
	text := strings.TrimSpace(string(out))
	if len(text) > 4000 {
		text = "…" + text[len(text)-4000:]
	}
	return &lean.Elaboration{
		Messages: []lean.Message{{Severity: "error", Line: 1, Text: fmt.Sprintf("lake build %s: %v\n%s", strings.Join(targets, " "), err, text)}},
		Seconds:  time.Since(start).Seconds(),
	}
}

// Imports returns the modules of Math2 that the Lean text imports, in order and with no repeats.
func Imports(text []byte) []string {
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(text))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) > 0 && (f[0] == "public" || f[0] == "private") {
			f = f[1:]
		}
		if len(f) < 2 || f[0] != "import" {
			continue
		}
		for _, m := range f[1:] {
			if strings.HasPrefix(m, "Math2.") && !slices.Contains(out, m) {
				out = append(out, m)
			}
		}
	}
	return out
}
