package compute

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/zeebo/blake3"

	"github.com/tamnd/math2-agent/sandbox"
)

// The files of a run, in build/compute/TAG/ARCH/.
const (
	RunFile         = "run.json"
	StdoutFile      = "stdout"
	StderrFile      = "stderr"
	CertificateFile = "certificate" // the program writes its certificate to /out/certificate
)

// Run is what one run of a computation gave on one architecture. Runner.Run writes it to build/compute/TAG/ARCH/run.json.
type Run struct {
	Tag         string  `json:"tag"`
	Image       string  `json:"image"`  // the image of the record
	Digest      string  `json:"digest"` // the digest that ran, for the architecture of the run
	Entry       string  `json:"entry"`
	Arch        string  `json:"arch"`
	Runtime     string  `json:"runtime"`
	Network     string  `json:"network"`
	ExitCode    int     `json:"exit_code"`
	TimedOut    bool    `json:"timed_out,omitempty"`
	Truncated   bool    `json:"truncated,omitempty"`
	Seconds     float64 `json:"seconds"`
	Output      string  `json:"output"`                // blake3:HEX of the standard output
	Certificate string  `json:"certificate,omitempty"` // blake3:HEX of the certificate file

	// The fields that Verify sets.
	Verified bool   `json:"verified,omitempty"` // the output and the certificate are the ones of the record, and the checker accepts the certificate
	Problem  string `json:"problem,omitempty"`  // why the run is not verified
	Check    *Check `json:"check,omitempty"`    // the run of the checker
}

// Check is the run of a certificate checker.
type Check struct {
	Image    string  `json:"image"`
	Digest   string  `json:"digest"`
	Entry    string  `json:"entry"`
	ExitCode int     `json:"exit_code"`
	TimedOut bool    `json:"timed_out,omitempty"`
	Seconds  float64 `json:"seconds"`
	Accepted bool    `json:"accepted"`
}

// Runner runs the computations of a tree.
type Runner struct {
	Root    string          // the math2 working tree
	Out     string          // the output directory. Empty means build/compute in the tree.
	Images  *Images         // the image manifest of the tree
	Sandbox sandbox.Sandbox // the sandbox, usually a *sandbox.Docker
	Arch    string          // the architecture of the sandbox. Empty means the architecture of this machine.
}

func (r *Runner) arch() string {
	if r.Arch == "" {
		return runtime.GOARCH
	}
	return r.Arch
}

// Dir returns the directory of the runs of a tag on the architecture of the runner.
func (r *Runner) Dir(tag string) string {
	out := r.Out
	if out == "" {
		out = filepath.Join(r.Root, "build", "compute")
	}
	return filepath.Join(out, tag, r.arch())
}

// image returns the local reference of the image that ref names, for the architecture of the runner: the image id sha256:HEX, as docker load gives it.
func (r *Runner) image(ref string) (name, digest string, err error) {
	img, _, ok := r.Images.Find(ref)
	if !ok {
		return "", "", fmt.Errorf("the image %s is not in %s", ref, ImagesFile)
	}
	digest = img.Digests[r.arch()]
	if digest == "" {
		return "", "", fmt.Errorf("%s has no digest for %s", img.Name, r.arch())
	}
	return img.Name, digest, nil
}

// Run runs the computation of rec on the architecture of the runner, and writes run.json, stdout, stderr and the certificate to Dir(rec.Tag). The run reads compute/TAG/ at /work. It gets the inputs as JSON in the variable M2_INPUTS, and the seed in M2_SEED. A run that fails or runs out of time gives a Run with no error. An error means that the computation could not run.
func (r *Runner) Run(ctx context.Context, rec *Record) (*Run, error) {
	name, digest, err := r.image(rec.Image)
	if err != nil {
		return nil, fmt.Errorf("compute: %s: %w", rec.Tag, err)
	}
	cmd := Command(name, sandbox.WorkDir+"/"+rec.Entry)
	if cmd == nil {
		return nil, fmt.Errorf("compute: %s: the toolchain cannot run a file in %s", rec.Tag, name)
	}
	if _, err := os.Stat(filepath.Join(r.Root, filepath.FromSlash(rec.EntryPath()))); err != nil {
		return nil, fmt.Errorf("compute: %s: %w", rec.Tag, err)
	}
	dir := r.Dir(rec.Tag)
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(out, 0o750); err != nil {
		return nil, err
	}
	// The program runs as the user 65534, and writes its certificate here.
	if err := os.Chmod(out, 0o777); err != nil { // #nosec G302 -- the sandbox user writes to this directory, which is in the build output.
		return nil, err
	}
	inputs, err := json.Marshal(rec.Inputs)
	if err != nil {
		return nil, fmt.Errorf("compute: %s: the inputs: %w", rec.Tag, err)
	}
	env := map[string]string{"M2_INPUTS": string(inputs)}
	if rec.Seed != nil {
		env["M2_SEED"] = strconv.FormatInt(*rec.Seed, 10)
	}
	code, err := filepath.Abs(filepath.Join(r.Root, filepath.FromSlash(rec.Dir())))
	if err != nil {
		return nil, err
	}
	absOut, err := filepath.Abs(out)
	if err != nil {
		return nil, err
	}
	res, err := r.Sandbox.Run(ctx, sandbox.Spec{
		Image: digest, Command: cmd, Dir: code, Out: absOut, Env: env,
		CPU: rec.Resources.CPU, Memory: rec.Resources.Memory, Wall: rec.Resources.Wall,
	})
	if err != nil {
		return nil, fmt.Errorf("compute: %s: %w", rec.Tag, err)
	}
	run := &Run{
		Tag: rec.Tag, Image: rec.Image, Digest: digest, Entry: rec.Entry,
		Arch: r.arch(), Runtime: res.Runtime, Network: res.Network,
		ExitCode: res.ExitCode, TimedOut: res.TimedOut, Truncated: res.Truncated,
		Seconds: res.Duration.Seconds(), Output: Hash(res.Stdout),
	}
	if err := os.WriteFile(filepath.Join(dir, StdoutFile), res.Stdout, 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, StderrFile), res.Stderr, 0o600); err != nil {
		return nil, err
	}
	if h, err := hashFile(filepath.Join(out, CertificateFile)); err == nil {
		run.Certificate = h
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return run, writeRun(dir, run)
}

// Verify runs the computation of rec, and checks the run against the record: the run ends with the exit code 0 in time, its output has the hash of the record, and, when the record has a certificate, the certificate has the hash of the record and the fixed checker of its kind accepts it. The checker reads compute/TAG/ and the certificate at /work. Verify writes the result to run.json.
func (r *Runner) Verify(ctx context.Context, rec *Record) (*Run, error) {
	run, err := r.Run(ctx, rec)
	if err != nil {
		return nil, err
	}
	run.Problem = r.problem(rec, run)
	if run.Problem == "" && rec.Certificate != nil {
		if run.Check, err = r.check(ctx, rec); err != nil {
			return nil, err
		}
		if !run.Check.Accepted {
			run.Problem = fmt.Sprintf("the checker %s exits with the code %d", rec.Certificate.CheckerEntry, run.Check.ExitCode)
		}
	}
	run.Verified = run.Problem == ""
	return run, writeRun(r.Dir(rec.Tag), run)
}

func (r *Runner) problem(rec *Record, run *Run) string {
	switch {
	case run.TimedOut:
		return "the run takes longer than its wall time"
	case run.ExitCode != 0:
		return fmt.Sprintf("the run exits with the code %d", run.ExitCode)
	case run.Truncated:
		return "the output is longer than the limit of the sandbox"
	case rec.Output == nil || rec.Output.Hash == "":
		return fmt.Sprintf("the record has no output hash. The run gives %s", run.Output)
	case rec.Output.Hash != run.Output:
		return fmt.Sprintf("the output hash is %s, and the record has %s", run.Output, rec.Output.Hash)
	case rec.Certificate == nil:
		return ""
	case run.Certificate == "":
		return "the run writes no certificate to /out/certificate"
	case run.Certificate != rec.Certificate.Artefact:
		return fmt.Sprintf("the certificate hash is %s, and the record has %s", run.Certificate, rec.Certificate.Artefact)
	}
	c, ok := CheckerFor(rec.Certificate.Kind)
	switch {
	case !ok:
		return fmt.Sprintf("%q is not a certificate kind", rec.Certificate.Kind)
	case !c.Allows(rec.Certificate.Checker, rec.Image):
		return fmt.Sprintf("the checker %s is not the fixed checker of %s", rec.Certificate.Checker, c.Kind)
	case rec.Certificate.CheckerEntry == "":
		return "the certificate has no checker entry"
	}
	return ""
}

// check runs the checker of the certificate of rec on a copy of compute/TAG/ with the certificate.
func (r *Runner) check(ctx context.Context, rec *Record) (*Check, error) {
	name, digest, err := r.image(rec.Certificate.Checker)
	if err != nil {
		return nil, fmt.Errorf("compute: %s: the checker: %w", rec.Tag, err)
	}
	cmd := Command(name, sandbox.WorkDir+"/"+rec.Certificate.CheckerEntry)
	if cmd == nil {
		return nil, fmt.Errorf("compute: %s: the toolchain cannot run a file in %s", rec.Tag, name)
	}
	dir := filepath.Join(r.Dir(rec.Tag), "check")
	if err := os.CopyFS(dir, os.DirFS(filepath.Join(r.Root, filepath.FromSlash(rec.Dir())))); err != nil {
		return nil, err
	}
	if err := copyFile(filepath.Join(dir, CertificateFile), filepath.Join(r.Dir(rec.Tag), "out", CertificateFile)); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	res, err := r.Sandbox.Run(ctx, sandbox.Spec{Image: digest, Command: cmd, Dir: abs, Wall: rec.Resources.Wall})
	if err != nil {
		return nil, fmt.Errorf("compute: %s: the checker: %w", rec.Tag, err)
	}
	if err := os.WriteFile(filepath.Join(r.Dir(rec.Tag), "check.log"), append(res.Stdout, res.Stderr...), 0o600); err != nil {
		return nil, err
	}
	return &Check{
		Image: rec.Certificate.Checker, Digest: digest, Entry: rec.Certificate.CheckerEntry,
		ExitCode: res.ExitCode, TimedOut: res.TimedOut, Seconds: res.Duration.Seconds(),
		Accepted: res.ExitCode == 0 && !res.TimedOut,
	}, nil
}

// Hash returns the hash of data as blake3:HEX, the form of the output hash of a record.
func Hash(data []byte) string {
	sum := blake3.Sum256(data)
	return "blake3:" + hex.EncodeToString(sum[:])
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p) // #nosec G304 -- a file of the build output.
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := blake3.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "blake3:" + hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(dst, src string) error {
	data, err := os.ReadFile(src) // #nosec G304 -- a file of the build output.
	if err != nil {
		return err
	}
	// The checker runs as the user 65534 and reads the copy.
	return os.WriteFile(dst, data, 0o644) // #nosec G306 G703 -- a file of the build output that the sandbox user reads.
}

func writeRun(dir string, run *Run) error {
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, RunFile), append(data, '\n'), 0o600)
}

// ReadRuns reads each run.json in the output directory dir, usually build/compute. The result maps a tag to its runs by architecture. A directory that does not exist gives no runs and no error.
func ReadRuns(dir string) (map[string]map[string]*Run, error) {
	out := map[string]map[string]*Run{}
	files, err := filepath.Glob(filepath.Join(dir, "*", "*", RunFile))
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		data, err := os.ReadFile(f) // #nosec G304 -- a file of the build output.
		if err != nil {
			return nil, err
		}
		var run Run
		if err := json.Unmarshal(data, &run); err != nil {
			return nil, fmt.Errorf("compute: %s: %w", f, err)
		}
		if out[run.Tag] == nil {
			out[run.Tag] = map[string]*Run{}
		}
		out[run.Tag][run.Arch] = &run
	}
	return out, nil
}
