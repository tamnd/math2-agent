package lean_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tamnd/math2-agent/lean"
)

var update = flag.Bool("update", false, "write the golden files")

var (
	buildOnce sync.Once
	buildErr  error
)

// local builds M2Extract and the fixtures in this directory, and returns the program. The test skips when lake is not on the PATH.
func local(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("lake"); err != nil {
		t.Skip("lake is not on the PATH")
	}
	buildOnce.Do(func() {
		out, err := exec.Command("lake", "build", "m2extract", "Fixture").CombinedOutput()
		if err != nil {
			buildErr = fmt.Errorf("lake build: %w\n%s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	exe, err := filepath.Abs(filepath.Join(".lake", "build", "bin", "m2extract"))
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

func start(t *testing.T, exe, project string, modules ...string) *lean.Session {
	t.Helper()
	s, err := lean.Start(context.Background(), exe, project)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := s.Import(modules...); err != nil {
		t.Fatal(err)
	}
	return s
}

// golden compares got with the file testdata/name, or writes the file when -update is set.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("the records are\n%s\nand %s has\n%s", got, path, want)
	}
}

func summary(rs []lean.Record) string {
	var b strings.Builder
	for _, r := range rs {
		if r.Error != "" {
			fmt.Fprintf(&b, "%s error %q\n", r.Decl, r.Error)
			continue
		}
		fmt.Fprintf(&b, "%s %s %s\n  type %s\n  axioms %v native %d\n  uses_bound %v\n", r.Decl, r.Kind, r.Module, r.TypeHash, r.Axioms, len(r.NativeDecideAxioms), r.UsesBound)
	}
	return b.String()
}

func TestFixture(t *testing.T) {
	s := start(t, local(t), ".", "Fixture")
	v, err := s.Version()
	if err != nil {
		t.Fatal(err)
	}
	toolchain, err := lean.Toolchain(".")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(toolchain, ":v"+v.Lean) {
		t.Errorf("the process runs Lean %s, and lean-toolchain has %s", v.Lean, toolchain)
	}
	decls := []string{"add_comm_a", "add_comm_b", "id_u", "id_v", "inhabited_inst", "inhabited_explicit", "em", "native", "incomplete", "uses_a", "absent"}
	for i, d := range decls {
		decls[i] = "Fixture." + d
	}
	rs, err := s.Extract(decls, []string{"Fixture.add_comm_a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != len(decls) {
		t.Fatalf("Extract gave %d records for %d declarations", len(rs), len(decls))
	}
	by := map[string]lean.Record{}
	for _, r := range rs {
		by[strings.TrimPrefix(r.Decl, "Fixture.")] = r
	}
	same := func(a, b string, want bool) {
		if got := by[a].TypeHash == by[b].TypeHash; got != want {
			t.Errorf("%s and %s: equal type hashes is %v, and it must be %v", a, b, got, want)
		}
	}
	same("add_comm_a", "add_comm_b", true)
	same("id_u", "id_v", true)
	same("inhabited_inst", "inhabited_explicit", false)
	if by["add_comm_a"].ValueHash != by["add_comm_b"].ValueHash || by["add_comm_a"].ValueHash == by["uses_a"].ValueHash {
		t.Error("the value hash of add_comm_a must be equal to that of add_comm_b, and different from that of uses_a")
	}
	if r := by["em"]; !slices.Equal(r.Axioms, []string{"Classical.choice", "Quot.sound", "propext"}) {
		t.Errorf("em has the axioms %v", r.Axioms)
	}
	if r := by["native"]; len(r.NativeDecideAxioms) != 1 || len(r.Axioms) != 0 {
		t.Errorf("native has the axioms %v and the native_decide axioms %v", r.Axioms, r.NativeDecideAxioms)
	}
	if r := by["incomplete"]; !slices.Contains(r.Axioms, "sorryAx") {
		t.Errorf("incomplete has the axioms %v, without sorryAx", r.Axioms)
	}
	if r := by["uses_a"]; !slices.Equal(r.UsesBound, []string{"Fixture.add_comm_a"}) {
		t.Errorf("uses_a has uses_bound %v", r.UsesBound)
	}
	if r := by["absent"]; r.Error == "" {
		t.Error("an unknown declaration has no error")
	}
	if r := by["add_comm_a"]; r.Deprecated != nil {
		t.Errorf("add_comm_a has the deprecation %+v", r.Deprecated)
	}
	old, err := s.Extract([]string{"Fixture.add_comm_old"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if d := old[0].Deprecated; d == nil || *d != (lean.Deprecation{NewName: "Fixture.add_comm_a", Since: "2026-01-01"}) {
		t.Errorf("add_comm_old has the deprecation %+v", d)
	}
	golden(t, "fixture.golden", summary(rs))
}

func TestChecks(t *testing.T) {
	s := start(t, local(t), ".", "Fixture")
	decls := []string{"vacuous", "small", "false_claim", "em", "id_u", "absent"}
	for i, d := range decls {
		decls[i] = "Fixture." + d
	}
	got, err := s.Checks(lean.CheckHeartbeats, decls...)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]lean.Checks{
		"Fixture.vacuous":     {Vacuous: "omega", Trivial: "grind"},
		"Fixture.small":       {Trivial: "rfl"},
		"Fixture.false_claim": {Refuted: "decide"},
		"Fixture.em":          {Trivial: "grind"},
	}
	if len(got) != len(want) {
		t.Errorf("Checks gives %d results, and it must give %d: %v", len(got), len(want), got)
	}
	for d, w := range want {
		if got[d] != w {
			t.Errorf("%s: Checks gives %+v, and it must give %+v", d, got[d], w)
		}
	}
}

func TestImplies(t *testing.T) {
	s := start(t, local(t), ".", "Fixture")
	pairs := [][2]string{
		{"le_add", "le_add_one"},
		{"le_add", "lt_self"},
		{"add_comm_a", "fermat"},
		{"add_comm_a", "add_comm_b"},
		{"id_u", "em"},
		{"absent", "em"},
	}
	for i, p := range pairs {
		pairs[i] = [2]string{"Fixture." + p[0], "Fixture." + p[1]}
	}
	got, err := s.Implies(lean.CheckHeartbeats, pairs...)
	if err != nil {
		t.Fatal(err)
	}
	want := []lean.Implication{
		{Implies: "(intro h; simpa using h)"},
		{Contradicts: "(intro h hb; simp_all)"},
		{},
		{Implies: "(intro h; exact h)"},
		{Error: "not a statement"},
		{Error: "unknown declaration"},
	}
	if len(got) != len(want) {
		t.Fatalf("Implies gives %d results for %d pairs", len(got), len(pairs))
	}
	for i, w := range want {
		w.From, w.To = pairs[i][0], pairs[i][1]
		if got[i] != w {
			t.Errorf("Implies gives %+v, and it must give %+v", got[i], w)
		}
	}
}

func TestLibrary(t *testing.T) {
	s := start(t, local(t), ".", "Fixture")
	all, total, err := s.Library("Fixture", 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if total != len(all) || total != 18 {
		t.Fatalf("Library gives %d records and a total of %d", len(all), total)
	}
	by := map[string]lean.Decl{}
	for i, d := range all {
		if i > 0 && all[i-1].Decl >= d.Decl {
			t.Errorf("%s comes after %s", d.Decl, all[i-1].Decl)
		}
		if d.Error != "" || d.Module != "Fixture" || !strings.HasPrefix(d.TypeHash, "blake3:") {
			t.Errorf("the record of %s is %+v", d.Decl, d)
		}
		by[d.Decl] = d
	}
	a, b := by["Fixture.add_comm_a"], by["Fixture.add_comm_b"]
	if a.Kind != "theorem" || a.TypeHash != b.TypeHash || !strings.Contains(a.Doc, "other binder names") || b.Doc != "" {
		t.Errorf("add_comm_a is %+v and add_comm_b is %+v", a, b)
	}
	if !slices.Contains(by["Fixture.uses_a"].Uses, "Fixture.add_comm_a") {
		t.Errorf("uses_a uses %v", by["Fixture.uses_a"].Uses)
	}
	page, total2, err := s.Library("Fixture", 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if total2 != total || len(page) != 3 || page[0].Decl != all[2].Decl {
		t.Errorf("the page from 2 is %v of %d", page, total2)
	}
	if none, n, err := s.Library("NoSuchRoot", 0, 10); err != nil || n != 0 || len(none) != 0 {
		t.Errorf("an unknown root gives %v, %d, %v", none, n, err)
	}
}

func TestErrors(t *testing.T) {
	exe := local(t)
	s, err := lean.Start(context.Background(), exe, ".")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Extract([]string{"Nat"}, nil); err == nil {
		t.Error("Extract worked before Import")
	}
	if err := s.Import("No.Such.Module"); err == nil {
		t.Error("Import of a module that does not exist worked")
	}
	if err := s.Close(); err != nil {
		t.Error(err)
	}
}

func TestInstall(t *testing.T) {
	if testing.Short() {
		t.Skip("Install builds M2Extract")
	}
	local(t)
	cache := t.TempDir()
	exe, err := lean.Install(context.Background(), ".", cache)
	if err != nil {
		t.Fatal(err)
	}
	again, err := lean.Install(context.Background(), ".", cache)
	if err != nil || again != exe {
		t.Errorf("the second Install gave %q, %v, and the first gave %q", again, err, exe)
	}
	s := start(t, exe, ".", "Init")
	axs, err := s.Axioms("Classical.em", "Nat.add_comm", "No.such")
	if err != nil {
		t.Fatal(err)
	}
	if len(axs) != 2 || len(axs["Nat.add_comm"]) != 0 || !slices.Equal(axs["Classical.em"], lean.StandardAxioms) {
		t.Errorf("the axioms are %v", axs)
	}
}

// TestMath2 checks the seed bindings and 100 sampled Mathlib declarations. It needs the Lake project lean/ of tamnd/math2. Set M2_MATH2_LEAN to its directory.
func TestMath2(t *testing.T) {
	project := os.Getenv("M2_MATH2_LEAN")
	if project == "" {
		t.Skip("M2_MATH2_LEAN is not set")
	}
	s := start(t, local(t), project, "Mathlib", "Math2")
	bs, err := s.Bindings()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(bs, lean.Binding{Decl: "Math2.Match.lagrange", Tag: "3C53TD"}) {
		t.Errorf("the bindings are %v, without the adapter of 3C53TD", bs)
	}
	seed := []string{"Group", "Subgroup.card_subgroup_dvd_card", "Math2.Match.lagrange"}
	rs, err := s.Extract(seed, seed)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "seed.golden", summary(rs))

	sample := lines(t, filepath.Join("testdata", "mathlib-sample.txt"))
	if len(sample) != 100 {
		t.Fatalf("the sample has %d declarations", len(sample))
	}
	got, err := s.Axioms(sample...)
	if err != nil {
		t.Fatal(err)
	}
	want := printAxioms(t, project, sample)
	for _, d := range sample {
		g, w := got[d], want[d]
		slices.Sort(g)
		slices.Sort(w)
		if !slices.Equal(g, w) {
			t.Errorf("%s: Axioms gives %v, and #print axioms gives %v", d, g, w)
		}
	}
}

func lines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(b))
}

// axiomMsg is one message of #print axioms. Lean breaks a long list of axioms over more than one line.
var axiomMsg = regexp.MustCompile(`(?m)^'(.+)' (?:depends on axioms: \[([^\]]*)\]|does not depend on any axioms)`)

// printAxioms runs #print axioms for each declaration in a Lean file in project.
func printAxioms(t *testing.T, project string, decls []string) map[string][]string {
	t.Helper()
	var src strings.Builder
	src.WriteString("import Mathlib\n")
	for _, d := range decls {
		fmt.Fprintf(&src, "#print axioms %s\n", d)
	}
	file := filepath.Join(t.TempDir(), "Axioms.lean")
	if err := os.WriteFile(file, []byte(src.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("lake", "env", "lean", file)
	cmd.Dir = project
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("lean: %v\n%s", err, out)
	}
	res := map[string][]string{}
	for _, m := range axiomMsg.FindAllStringSubmatch(string(out), -1) {
		res[m[1]] = []string{}
		for ax := range strings.SplitSeq(m[2], ",") {
			if ax = strings.TrimSpace(ax); ax != "" {
				res[m[1]] = append(res[m[1]], ax)
			}
		}
	}
	return res
}

func TestInstallDefaultCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("PATH", "")
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Skip(err)
	}
	// lake is not on the PATH, thus Install fails after it makes the cache.
	if _, err := lean.Install(context.Background(), ".", ""); err == nil {
		t.Fatal("Install builds without lake")
	}
	if _, err := os.Stat(filepath.Join(cache, "m2")); err != nil {
		t.Errorf("Install with an empty cache does not use m2 in the user cache directory: %v", err)
	}
}
