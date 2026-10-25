// Package sandbox runs a program in a container image with the isolation of spec 08.6: no network, a read-only image, a writable scratch area of bounded size, and limits on the CPU, the memory, the processes, the wall clock and the output.
//
// The computations of math2 and the runs of agents use the same sandbox. [Docker] runs a [Spec] with the docker CLI. When the daemon has the gVisor runtime runsc, [Docker.Detect] selects it, and the program then runs on a user-space kernel. An image is always named by its digest, so a run names exactly the bytes that it ran.
package sandbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The limits of a run when a Spec gives none.
const (
	DefaultMemory  = 1 << 30          // 1 GiB
	DefaultWall    = 10 * time.Minute // spec 08.2
	DefaultScratch = 256 << 20        // 256 MiB
	DefaultOutput  = 16 << 20         // 16 MiB for each of stdout and stderr
	DefaultPids    = 256
)

// The paths in the sandbox.
const (
	WorkDir    = "/work"    // the read-only mount of Spec.Dir
	OutDir     = "/out"     // the writable mount of Spec.Out
	ScratchDir = "/scratch" // the writable scratch area, and the working directory of the program
)

// Spec is one run.
type Spec struct {
	Image   string            // the image by digest: name@sha256:HEX, a registry path with @sha256:HEX, or an image id sha256:HEX
	Command []string          // the program and its arguments, as paths in the image
	Dir     string            // a directory of the host that the program reads at /work, or ""
	Out     string            // a directory of the host that the program writes at /out, or "". The user 65534 must be able to write to it.
	Env     map[string]string // the environment. HOME is /scratch.
	CPU     int               // the number of CPUs. 0 means 1.
	Memory  int64             // the memory in bytes. 0 means DefaultMemory.
	Wall    time.Duration     // the wall clock. 0 means DefaultWall.
	Scratch int64             // the size of /scratch in bytes. 0 means DefaultScratch.
	Output  int64             // the most bytes of stdout, and of stderr, that the result keeps. 0 means DefaultOutput.
}

// Result is the result of one run.
type Result struct {
	ExitCode  int           // the exit code of the program, or -1 when it did not exit
	Stdout    []byte        // at most Spec.Output bytes
	Stderr    []byte        // at most Spec.Output bytes
	Truncated bool          // the program wrote more than Spec.Output bytes to stdout or to stderr
	TimedOut  bool          // the run took longer than Spec.Wall, and the sandbox stopped it
	Duration  time.Duration // the wall time of the run
	Runtime   string        // the OCI runtime: runsc for gVisor, or the default of the daemon
	Arch      string        // the architecture of the machine, as GOARCH gives it, for example amd64 or arm64
	Network   string        // the network of the run. It is always "none".
}

var (
	digestRe = regexp.MustCompile(`^(?:[a-z0-9][a-z0-9._/:-]*@)?sha256:[0-9a-f]{64}$`)
	envRe    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// ValidImage reports whether image names an image by its digest. A tag such as python:3 can change, so it is not valid.
func ValidImage(image string) bool { return digestRe.MatchString(image) }

// Sandbox runs a Spec. Docker is the Sandbox of the toolchain. A test can use another.
type Sandbox interface {
	Run(ctx context.Context, s Spec) (*Result, error)
}

// Docker runs a Spec with the docker CLI.
type Docker struct {
	Binary  string // the docker CLI. "" means docker on the PATH.
	Runtime string // the OCI runtime, for example runsc. "" means the default runtime of the daemon.
}

func (d *Docker) binary() string {
	if d.Binary == "" {
		return "docker"
	}
	return d.Binary
}

// Detect asks the daemon for its runtimes and selects runsc when the daemon has it. It returns an error when the daemon does not answer.
func (d *Docker) Detect(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, d.binary(), "info", "--format", "{{range $k, $v := .Runtimes}}{{$k}} {{end}}").Output() // #nosec G204 -- the binary is the docker CLI that the caller chose.
	if err != nil {
		return fmt.Errorf("sandbox: docker info: %w", err)
	}
	if slices.Contains(strings.Fields(string(out)), "runsc") {
		d.Runtime = "runsc"
	}
	return nil
}

// Args returns the arguments of docker for the run of s in the container name. It returns an error when s is not valid.
func (d *Docker) Args(s Spec, name string) ([]string, error) {
	if !ValidImage(s.Image) {
		return nil, fmt.Errorf("sandbox: the image %q has no digest", s.Image)
	}
	if len(s.Command) == 0 {
		return nil, errors.New("sandbox: the spec has no command")
	}
	for _, dir := range []string{s.Dir, s.Out} {
		if dir != "" && (strings.ContainsAny(dir, ",:") || !strings.HasPrefix(dir, "/")) {
			return nil, fmt.Errorf("sandbox: the directory %q is not an absolute path without a comma or a colon", dir)
		}
	}
	cpu := max(s.CPU, 1)
	mem := cmpOr(s.Memory, DefaultMemory)
	scratch := cmpOr(s.Scratch, DefaultScratch)
	args := []string{
		"run", "--rm", "--name", name,
		"--network", "none",
		"--read-only",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", strconv.Itoa(DefaultPids),
		"--cpus", strconv.Itoa(cpu),
		"--memory", strconv.FormatInt(mem, 10),
		"--memory-swap", strconv.FormatInt(mem, 10),
		"--user", "65534:65534",
		"--tmpfs", ScratchDir + ":rw,exec,size=" + strconv.FormatInt(scratch, 10) + ",mode=1777",
		"--tmpfs", "/tmp:rw,size=" + strconv.FormatInt(scratch, 10) + ",mode=1777",
		"--workdir", ScratchDir,
		"--env", "HOME=" + ScratchDir,
	}
	if d.Runtime != "" {
		args = append(args, "--runtime", d.Runtime)
	}
	if s.Dir != "" {
		args = append(args, "--mount", "type=bind,source="+s.Dir+",target="+WorkDir+",readonly")
	}
	if s.Out != "" {
		args = append(args, "--mount", "type=bind,source="+s.Out+",target="+OutDir)
	}
	keys := make([]string, 0, len(s.Env))
	for k := range s.Env {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if !envRe.MatchString(k) || k == "HOME" {
			return nil, fmt.Errorf("sandbox: %q is not a valid name for the environment", k)
		}
		args = append(args, "--env", k+"="+s.Env[k])
	}
	args = append(args, "--", s.Image)
	return append(args, s.Command...), nil
}

// Run runs s and waits for it to end. An error means that the sandbox could not run the program. A program that fails, runs out of memory or runs out of time gives a Result and no error.
func (d *Docker) Run(ctx context.Context, s Spec) (*Result, error) {
	name := "m2-" + randomHex(8)
	args, err := d.Args(s, name)
	if err != nil {
		return nil, err
	}
	wall := s.Wall
	if wall <= 0 {
		wall = DefaultWall
	}
	limit := cmpOr(s.Output, DefaultOutput)
	stdout, stderr := &capped{max: limit}, &capped{max: limit}
	rctx, cancel := context.WithTimeout(ctx, wall)
	defer cancel()
	cmd := exec.CommandContext(rctx, d.binary(), args...) // #nosec G204 -- Args checks each value that comes from the spec.
	cmd.Stdout, cmd.Stderr = stdout, stderr
	// When the time runs out, stop the container, and not only the CLI that waits for it.
	cmd.Cancel = func() error {
		kctx, kcancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer kcancel()
		_ = exec.CommandContext(kctx, d.binary(), "kill", name).Run() // #nosec G204 -- the name is random hex.
		return cmd.Process.Kill()
	}
	cmd.WaitDelay = 30 * time.Second
	start := time.Now()
	err = cmd.Run()
	res := &Result{
		Stdout: stdout.buf.Bytes(), Stderr: stderr.buf.Bytes(),
		Truncated: stdout.over || stderr.over, Duration: time.Since(start),
		Runtime: d.Runtime, Arch: runtime.GOARCH, Network: "none",
	}
	if res.Runtime == "" {
		res.Runtime = "default"
	}
	var exit *exec.ExitError
	switch {
	case err != nil && errors.Is(rctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil:
		res.TimedOut, res.ExitCode = true, -1
	case err != nil && ctx.Err() != nil:
		return nil, ctx.Err()
	case errors.As(err, &exit):
		res.ExitCode = exit.ExitCode()
		// docker run exits with 125 when it cannot start the container, and with 126 or 127 when the command cannot run. So a program must not use the exit code 125.
		if res.ExitCode == 125 {
			return nil, fmt.Errorf("sandbox: docker could not run the container: %s", bytes.TrimSpace(res.Stderr))
		}
	case err != nil:
		return nil, fmt.Errorf("sandbox: %w", err)
	}
	return res, nil
}

// capped keeps the first max bytes that it gets, and counts the rest.
type capped struct {
	buf  bytes.Buffer
	max  int64
	over bool
}

func (c *capped) Write(p []byte) (int, error) {
	if room := c.max - int64(c.buf.Len()); room < int64(len(p)) {
		c.over = true
		c.buf.Write(p[:max(room, 0)])
		return len(p), nil
	}
	return c.buf.Write(p)
}

func cmpOr(v, def int64) int64 {
	if v <= 0 {
		return def
	}
	return v
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
