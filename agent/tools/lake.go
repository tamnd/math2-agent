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
)

// Lake is the [Lean] environment of the Lake project in the directory lean/ of a work tree of math2.
type Lake struct {
	Tree    string   // the directory of the work tree
	Exe     string   // the M2Extract program, from lean.Install
	Modules []string // the modules that Extract imports. Empty means Math2.
}

func (l *Lake) project() string { return filepath.Join(l.Tree, "lean") }

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
	e, err := lean.Elaborate(ctx, l.project(), rel)
	if err != nil {
		return nil, fmt.Errorf("tools: %w", err)
	}
	e.Seconds += seconds
	return e, nil
}

// Extract builds the library of the tree, then extracts the declarations with M2Extract. A build that fails is an error.
func (l *Lake) Extract(ctx context.Context, decls []string) ([]lean.Record, error) {
	if b := l.build(ctx); len(b.Messages) > 0 {
		return nil, fmt.Errorf("tools: the build of the work tree fails: %s", b.Messages[0].Text)
	}
	s, err := lean.Start(ctx, l.Exe, l.project())
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
	return s.Extract(decls, nil)
}

// build runs lake build on the targets, or on the default targets when there are none. The result has no messages when the build passes, and one error when it fails.
func (l *Lake) build(ctx context.Context, targets ...string) *lean.Elaboration {
	cmd := exec.CommandContext(ctx, "lake", append([]string{"build", "-q"}, targets...)...) // #nosec G204 -- the targets are module names from import lines, and no shell reads them.
	cmd.Dir = l.project()
	start := time.Now()
	out, err := cmd.CombinedOutput()
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
