package prompts_test

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/zeebo/blake3"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/prompts"
)

var update = flag.Bool("update", false, "write roles.sum with the version and the digest of each role")

func TestRoles(t *testing.T) {
	entries, err := fs.ReadDir(prompts.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("FS has no role")
	}
	for _, e := range entries {
		r, err := agent.LoadRole(prompts.FS, e.Name())
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(r.Tools, "lean_check") && !strings.Contains(r.System, "# Lean conventions of math2") {
			t.Errorf("%s: the system prompt has no Lean style file", r.Name)
		}
		log, err := fs.ReadFile(prompts.FS, e.Name()+"/CHANGELOG.md")
		if err != nil || !strings.Contains(string(log), "\n## "+r.Version+"\n") {
			t.Errorf("%s: the CHANGELOG has no entry for the version %s", r.Name, r.Version)
		}
		prompts := map[string]string{"system": r.System}
		for name, text := range r.Checks {
			prompts["check "+name] = text
		}
		for name, text := range r.Passes {
			prompts["pass "+name] = text
		}
		for name, text := range prompts {
			if strings.Contains(text, "—") {
				t.Errorf("%s: the %s prompt has an em dash", r.Name, name)
			}
		}
	}
}

// digest returns the hash of the files of the role directory: the path and the content of each file, in the order of the paths.
func digest(t *testing.T, role string) string {
	t.Helper()
	h := blake3.New()
	err := fs.WalkDir(prompts.FS, role, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(prompts.FS, p)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", p, len(data))
		_, _ = h.Write(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return "blake3:" + hex.EncodeToString(h.Sum(nil))
}

// less reports whether the semver a is before b.
func less(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := range min(len(pa), len(pb)) {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			return x < y
		}
	}
	return false
}

// TestVersions checks that a change to a file of a role bumps the version of the role (spec 12.8). roles.sum has the version and the digest of the files of each role. When the files change, the version must go up, and go test -update writes the new line.
func TestVersions(t *testing.T) {
	data, err := os.ReadFile("roles.sum")
	if err != nil && !*update {
		t.Fatal(err)
	}
	type sum struct{ version, digest string }
	old := map[string]sum{}
	for line := range strings.Lines(string(data)) {
		f := strings.Fields(line)
		if len(f) != 3 {
			t.Fatalf("roles.sum: the line %q is not a role, a version and a digest", strings.TrimSpace(line))
		}
		old[f[0]] = sum{f[1], f[2]}
	}
	entries, err := fs.ReadDir(prompts.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, e := range entries {
		r, err := agent.LoadRole(prompts.FS, e.Name())
		if err != nil {
			t.Fatal(err)
		}
		now := sum{r.Version, digest(t, r.Name)}
		lines = append(lines, r.Name+" "+now.version+" "+now.digest+"\n")
		was, ok := old[r.Name]
		switch {
		case !ok:
			if !*update {
				t.Errorf("roles.sum has no line for %s. Run go test ./prompts -update", r.Name)
			}
		case was == now:
		case was.version == now.version:
			t.Errorf("the files of %s changed, and the version is still %s. Bump the version in role.yaml and tell why in CHANGELOG.md", r.Name, now.version)
		case !less(was.version, now.version):
			t.Errorf("the version of %s goes from %s to %s, and a new version must be greater", r.Name, was.version, now.version)
		case was.digest == now.digest:
			t.Errorf("the version of %s changed, and its files did not", r.Name)
		case !*update:
			t.Errorf("%s is at the version %s now. Run go test ./prompts -update to write it in roles.sum", r.Name, now.version)
		}
	}
	slices.Sort(lines)
	if *update {
		if err := os.WriteFile("roles.sum", []byte(strings.Join(lines, "")), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
