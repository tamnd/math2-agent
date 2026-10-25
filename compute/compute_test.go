package compute

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/sandbox"
)

// The digests of the tests are not digests of real images.
var (
	amd = "sha256:" + strings.Repeat("a", 64)
	arm = "sha256:" + strings.Repeat("b", 64)
	chk = "sha256:" + strings.Repeat("c", 64)
)

func parse(t *testing.T, src string) *node.Node {
	t.Helper()
	n, err := node.Parse("nodes/T1/T1TEST.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestFixtures(t *testing.T) {
	nodes, err := node.Load(filepath.Join("..", "testdata", "nodes", "valid"))
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, n := range nodes {
		if n.Kind() != "computation" {
			continue
		}
		seen++
		if _, err := FromNode(n); err != nil {
			t.Error(err)
		}
	}
	if seen == 0 {
		t.Fatal("the fixtures have no computation")
	}
}

func TestFromNode(t *testing.T) {
	r, err := FromNode(parse(t, `---
tag: T1TEST
kind: computation
name: A test
claim: It works.
role: experiment
engine:
  image: m2-python@`+amd+`
  entry: compute/T1TEST/main.py
  inputs: {limit: 100}
  seed: 7
  resources: {cpu: 2, mem: 512M, wall: 90s}
output:
  hash: blake3:`+strings.Repeat("0", 64)+`
  summary: fine
certificate:
  kind: witness
  artefact: blake3:`+strings.Repeat("1", 64)+`
  checker: m2-lean@`+chk+`
  checker_entry: Check.lean
---
`))
	if err != nil {
		t.Fatal(err)
	}
	if r.Entry != "main.py" || r.EntryPath() != "compute/T1TEST/main.py" || r.Role != "experiment" || r.Claim != "It works." {
		t.Errorf("record %+v", r)
	}
	if r.Seed == nil || *r.Seed != 7 || r.Inputs["limit"] != 100 {
		t.Errorf("seed %v, inputs %v", r.Seed, r.Inputs)
	}
	if r.Resources != (Resources{CPU: 2, Memory: 512 << 20, Wall: 90 * time.Second}) {
		t.Errorf("resources %+v", r.Resources)
	}
	if r.Output.Summary != "fine" || r.Certificate.Kind != "witness" || r.Certificate.CheckerEntry != "Check.lean" {
		t.Errorf("output %+v, certificate %+v", r.Output, r.Certificate)
	}
}

func TestFromNodeErrors(t *testing.T) {
	head := "---\ntag: T1TEST\nkind: computation\nclaim: x\n"
	for name, front := range map[string]string{
		"kind":      "---\ntag: T1TEST\nkind: statement\n",
		"no engine": head,
		"tag":       head + "engine: {image: python:3, entry: a.py}\n",
		"entry":     head + "engine: {image: m2-python@" + amd + ", entry: ../a.py}\n",
		"absolute":  head + "engine: {image: m2-python@" + amd + ", entry: /a.py}\n",
		"mem":       head + "engine: {image: m2-python@" + amd + ", entry: a.py, resources: {mem: lots}}\n",
		"wall":      head + "engine: {image: m2-python@" + amd + ", entry: a.py, resources: {wall: 3}}\n",
		"inputs":    head + "engine: {image: m2-python@" + amd + ", entry: a.py, inputs: [1]}\n",
		"checker":   head + "engine: {image: m2-python@" + amd + ", entry: a.py}\ncertificate: {kind: witness, checker_entry: ../x}\n",
	} {
		if _, err := FromNode(parse(t, front+"---\n")); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestParseBytes(t *testing.T) {
	for s, want := range map[string]int64{"512": 512, "1K": 1024, "512M": 512 << 20, "4G": 4 << 30, "1GiB": 1 << 30, "2T": 2 << 40} {
		if got, err := ParseBytes(s); err != nil || got != want {
			t.Errorf("ParseBytes(%q) = %d, %v, want %d", s, got, err, want)
		}
	}
	for _, s := range []string{"", "lots", "-1G", "0", "1P", "99999999999T"} {
		if _, err := ParseBytes(s); err == nil {
			t.Errorf("ParseBytes(%q) gives no error", s)
		}
	}
}

const manifest = `images:
  - name: m2-python
    contents: Python
    amd64: %AMD%
    arm64: %ARM%
  - name: m2-lean
    amd64: %CHK%
    arm64: %CHK%
  - name: m2-gap
`

func images(t *testing.T) *Images {
	t.Helper()
	im, err := ParseImages([]byte(strings.NewReplacer("%AMD%", amd, "%ARM%", arm, "%CHK%", chk).Replace(manifest)))
	if err != nil {
		t.Fatal(err)
	}
	return im
}

func TestImages(t *testing.T) {
	im := images(t)
	if img, arch, ok := im.Find("m2-python@" + arm); !ok || img.Name != "m2-python" || arch != "arm64" || img.Contents != "Python" {
		t.Errorf("Find gives %v %s %v", img, arch, ok)
	}
	for _, ref := range []string{"m2-python@" + chk, "m2-gap@" + amd, "m2-sage@" + amd, "m2-python"} {
		if _, _, ok := im.Find(ref); ok {
			t.Errorf("Find(%q) finds an image", ref)
		}
	}
	if im.Named("m2-gap") == nil || im.Named("m2-sage") != nil {
		t.Error("Named")
	}
	for name, data := range map[string]string{
		"twice":  "images:\n  - name: a\n  - name: a\n",
		"arch":   "images:\n  - name: a\n    riscv: " + amd + "\n",
		"digest": "images:\n  - name: a\n    amd64: sha256:12\n",
		"name":   "images:\n  - name: A\n",
		"yaml":   "images: [",
	} {
		if _, err := ParseImages([]byte(data)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestCheckers(t *testing.T) {
	kinds := []string{"witness", "witness-table", "lrat", "smt-proof", "primality", "interval", "lp-dual", "sos", "group-id"}
	if len(Checkers()) != len(kinds) {
		t.Errorf("there are %d checkers, and spec 08.3 has %d kinds", len(Checkers()), len(kinds))
	}
	for _, k := range kinds {
		if _, ok := CheckerFor(k); !ok {
			t.Errorf("no checker for %s", k)
		}
	}
	if _, ok := CheckerFor("magic"); ok {
		t.Error("a checker for magic")
	}
	w, _ := CheckerFor("witness")
	if !w.Allows("m2-lean@"+chk, "m2-python@"+amd) || w.Allows("m2-sage@"+chk, "m2-python@"+amd) || w.Allows("m2-lean", "") {
		t.Error("witness")
	}
	g, _ := CheckerFor("group-id")
	if g.Allows("m2-gap@"+chk, "m2-gap@"+amd) || !g.Allows("m2-sage@"+chk, "m2-gap@"+amd) {
		t.Error("a group-id certificate must be checked in a second system")
	}
}

func TestCommand(t *testing.T) {
	if got := strings.Join(Command("m2-python", "/work/main.py"), " "); got != "python3 -I -B /work/main.py" {
		t.Errorf("python: %s", got)
	}
	if got := strings.Join(Command("m2-smt", "/work/q.cvc5.smt2"), " "); got != "cvc5 /work/q.cvc5.smt2" {
		t.Errorf("cvc5: %s", got)
	}
	if Command("m2-magma", "/work/a.m") != nil {
		t.Error("magma has a command")
	}
}

// fake is a sandbox that runs no program. It writes the entry file to stdout. When the entry file has the word cert, it writes "cert" to /out/certificate. A checker accepts the certificate when the file is "cert".
type fake struct{ specs []sandbox.Spec }

func (f *fake) Run(_ context.Context, s sandbox.Spec) (*sandbox.Result, error) {
	f.specs = append(f.specs, s)
	file := filepath.Join(s.Dir, strings.TrimPrefix(s.Command[len(s.Command)-1], sandbox.WorkDir+"/"))
	data, err := os.ReadFile(file)
	if err != nil {
		return &sandbox.Result{ExitCode: 2, Stderr: []byte(err.Error())}, nil
	}
	res := &sandbox.Result{Stdout: data, Runtime: "fake", Network: "none", Duration: time.Second}
	if strings.Contains(string(data), "check") {
		cert, _ := os.ReadFile(filepath.Join(s.Dir, CertificateFile))
		if string(cert) != "cert" {
			res.ExitCode = 1
		}
		return res, nil
	}
	if strings.Contains(string(data), "cert") {
		if err := os.WriteFile(filepath.Join(s.Out, CertificateFile), []byte("cert"), 0o600); err != nil {
			return nil, err
		}
	}
	if strings.Contains(string(data), "fail") {
		res.ExitCode = 3
	}
	return res, nil
}

func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, data := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func record(t *testing.T, extra string) *Record {
	t.Helper()
	r, err := FromNode(parse(t, "---\ntag: T1TEST\nkind: computation\nclaim: x\nengine:\n  image: m2-python@"+amd+"\n  entry: main.py\n  inputs: {n: 3}\n  seed: 5\n"+extra+"---\n"))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRunVerify(t *testing.T) {
	src := "print('cert')\n"
	root := tree(t, map[string]string{"compute/T1TEST/main.py": src, "compute/T1TEST/Check.lean": "check\n"})
	f := &fake{}
	r := &Runner{Root: root, Images: images(t), Sandbox: f, Arch: "arm64"}
	out := Hash([]byte(src))

	run, err := r.Run(context.Background(), record(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if run.Digest != arm || run.Arch != "arm64" || run.Output != out || run.Certificate != Hash([]byte("cert")) || run.Network != "none" {
		t.Errorf("run %+v", run)
	}
	s := f.specs[0]
	if s.Image != arm || s.Env["M2_INPUTS"] != `{"n":3}` || s.Env["M2_SEED"] != "5" || strings.Join(s.Command, " ") != "python3 -I -B /work/main.py" {
		t.Errorf("spec %+v", s)
	}
	runs, err := ReadRuns(filepath.Join(root, "build", "compute"))
	if err != nil || runs["T1TEST"]["arm64"] == nil || runs["T1TEST"]["arm64"].Output != out {
		t.Errorf("ReadRuns: %v, %v", runs, err)
	}

	cert := "certificate:\n  kind: witness\n  artefact: " + Hash([]byte("cert")) + "\n  checker: m2-lean@" + chk + "\n  checker_entry: Check.lean\n"
	for name, c := range map[string]struct {
		extra, problem string
	}{
		"no hash":  {"", "the record has no output hash"},
		"wrong":    {"output: {hash: " + Hash(nil) + "}\n", "the output hash is"},
		"e1":       {"output: {hash: " + out + "}\n", ""},
		"e4":       {"output: {hash: " + out + "}\n" + cert, ""},
		"checker":  {"output: {hash: " + out + "}\n" + strings.Replace(cert, "m2-lean@", "m2-sage@", 1), "is not the fixed checker"},
		"kind":     {"output: {hash: " + out + "}\n" + strings.Replace(cert, "witness", "magic", 1), "is not a certificate kind"},
		"artefact": {"output: {hash: " + out + "}\n" + strings.Replace(cert, Hash([]byte("cert")), Hash(nil), 1), "the certificate hash is"},
		"no entry": {"output: {hash: " + out + "}\n" + strings.Replace(cert, "  checker_entry: Check.lean\n", "", 1), "no checker entry"},
	} {
		run, err := r.Verify(context.Background(), record(t, c.extra))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !strings.Contains(run.Problem, c.problem) || run.Verified != (c.problem == "") {
			t.Errorf("%s: verified %v, problem %q, want %q", name, run.Verified, run.Problem, c.problem)
		}
		if name == "e4" && (run.Check == nil || !run.Check.Accepted || run.Check.Digest != chk) {
			t.Errorf("e4: check %+v", run.Check)
		}
	}
}

func TestRunErrors(t *testing.T) {
	root := tree(t, map[string]string{"compute/T1TEST/main.py": "fail\n"})
	r := &Runner{Root: root, Images: images(t), Sandbox: &fake{}, Arch: "amd64"}
	run, err := r.Verify(context.Background(), record(t, ""))
	if err != nil || run.ExitCode != 3 || run.Verified || !strings.Contains(run.Problem, "exits with the code 3") {
		t.Errorf("a failed run: %+v, %v", run, err)
	}
	bad := record(t, "")
	bad.Image = "m2-sage@" + amd
	if _, err := r.Run(context.Background(), bad); err == nil || !strings.Contains(err.Error(), "is not in manifests/images.yaml") {
		t.Errorf("an image that is not in the manifest: %v", err)
	}
	bad.Image, bad.Entry = "m2-python@"+amd, "none.py"
	if _, err := r.Run(context.Background(), bad); err == nil {
		t.Error("an entry that does not exist runs")
	}
	gap := &Runner{Root: root, Images: images(t), Sandbox: &fake{}, Arch: "amd64"}
	gap.Images.Images[2].Digests = map[string]string{"arm64": arm}
	bad.Image, bad.Entry = "m2-gap@"+arm, "main.py"
	if _, err := gap.Run(context.Background(), bad); err == nil || !strings.Contains(err.Error(), "no digest for amd64") {
		t.Errorf("an image with no digest for the architecture: %v", err)
	}
}
