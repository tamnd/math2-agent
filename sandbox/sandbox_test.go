package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

const testImage = "m2-sage@sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestValidImage(t *testing.T) {
	for image, want := range map[string]bool{
		testImage: true,
		"ghcr.io/tamnd/m2-lean@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef": true,
		"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef":                       true,
		"python:3":            false,
		"m2-sage":             false,
		"m2-sage@sha256:0123": false,
		"m2-sage@sha256:0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef": false,
		"": false,
	} {
		if got := ValidImage(image); got != want {
			t.Errorf("ValidImage(%q) = %v, want %v", image, got, want)
		}
	}
}

func TestArgs(t *testing.T) {
	d := &Docker{Runtime: "runsc"}
	args, err := d.Args(Spec{
		Image: testImage, Command: []string{"/bin/sage", "run.sage"}, Dir: "/data/in", Out: "/data/out",
		Env: map[string]string{"SEED": "7", "LANG": "C"}, CPU: 2, Memory: 1 << 20, Scratch: 1 << 10,
	}, "m2-x")
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(args, " ")
	for _, want := range []string{
		"run --rm --name m2-x ", "--network none", "--read-only", "--cap-drop ALL", "--security-opt no-new-privileges",
		"--cpus 2", "--memory 1048576", "--memory-swap 1048576", "--user 65534:65534", "--runtime runsc",
		"--tmpfs /scratch:rw,exec,size=1024,mode=1777", "--mount type=bind,source=/data/in,target=/work,readonly", "--mount type=bind,source=/data/out,target=/out --env",
		"--env LANG=C --env SEED=7 -- " + testImage + " /bin/sage run.sage",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("the arguments have no %q:\n%s", want, line)
		}
	}
	if args[len(args)-3] != testImage {
		t.Errorf("the image is not the last argument before the command: %v", args)
	}
	if args, _ := (&Docker{}).Args(Spec{Image: testImage, Command: []string{"/x"}}, "n"); slices.Contains(args, "--runtime") || slices.Contains(args, "--mount") {
		t.Errorf("the arguments have a runtime or a mount that the spec does not give: %v", args)
	}
}

func TestArgsErrors(t *testing.T) {
	for name, s := range map[string]Spec{
		"tag":        {Image: "python:3", Command: []string{"/x"}},
		"no command": {Image: testImage},
		"relative":   {Image: testImage, Command: []string{"/x"}, Dir: "in"},
		"comma":      {Image: testImage, Command: []string{"/x"}, Dir: "/a,b"},
		"out":        {Image: testImage, Command: []string{"/x"}, Out: "out"},
		"env":        {Image: testImage, Command: []string{"/x"}, Env: map[string]string{"A B": "1"}},
		"home":       {Image: testImage, Command: []string{"/x"}, Env: map[string]string{"HOME": "/"}},
	} {
		if _, err := (&Docker{}).Args(s, "n"); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestCapped(t *testing.T) {
	c := &capped{max: 5}
	for _, s := range []string{"abc", "def", "g"} {
		if n, err := c.Write([]byte(s)); n != len(s) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", s, n, err)
		}
	}
	if got := c.buf.String(); got != "abcde" || !c.over {
		t.Errorf("capped has %q and over %v, want abcde and true", got, c.over)
	}
}

// TestDocker runs the probe in a real sandbox. It needs a docker daemon. Set M2_DOCKER=1 to make the test fail when there is none, and M2_DOCKER_RUNTIME to choose the runtime.
func TestDocker(t *testing.T) {
	if testing.Short() {
		t.Skip("the test runs containers")
	}
	ctx := context.Background()
	d := &Docker{Runtime: os.Getenv("M2_DOCKER_RUNTIME")}
	if err := exec.CommandContext(ctx, d.binary(), "info").Run(); err != nil {
		if os.Getenv("M2_DOCKER") != "" {
			t.Fatalf("docker info: %v", err)
		}
		t.Skip("there is no docker daemon")
	}
	image := probeImage(t)

	in := t.TempDir()
	if err := os.WriteFile(filepath.Join(in, "input.txt"), []byte("42\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := os.Chmod(out, 0o777); err != nil { // #nosec G302 -- the user 65534 in the sandbox writes here.
		t.Fatal(err)
	}
	res, err := d.Run(ctx, Spec{Image: image, Command: []string{"/probe", "net"}, Dir: in, Out: out, Wall: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("runtime %s, arch %s, %s:\n%s%s", res.Runtime, res.Arch, res.Duration, res.Stdout, res.Stderr)
	want := "dial failed\nudp failed\nresolve failed\nwrite-root failed\nwrite-work failed\nwrite-scratch ok\nwrite-out ok\nread-work ok\nuid 65534\n"
	if string(res.Stdout) != want || res.ExitCode != 0 {
		t.Errorf("the probe gave exit code %d and\n%s\nwant exit code 0 and\n%s", res.ExitCode, res.Stdout, want)
	}
	if b, err := os.ReadFile(filepath.Join(out, "probe")); err != nil || string(b) != "out\n" {
		t.Errorf("the probe did not write /out/probe: %q, %v", b, err)
	}
	if res.Network != "none" || res.Arch != runtime.GOARCH {
		t.Errorf("the result has network %q and arch %q", res.Network, res.Arch)
	}

	res, err = d.Run(ctx, Spec{Image: image, Command: []string{"/probe", "exit", "3"}})
	if err != nil || res.ExitCode != 3 || res.TimedOut {
		t.Errorf("exit 3: %+v, %v", res, err)
	}
	res, err = d.Run(ctx, Spec{Image: image, Command: []string{"/probe", "spam"}, Output: 1000})
	if err != nil || len(res.Stdout) != 1000 || !res.Truncated {
		t.Errorf("spam: %d bytes, truncated %v, %v", len(res.Stdout), res.Truncated, err)
	}
	res, err = d.Run(ctx, Spec{Image: image, Command: []string{"/probe", "sleep"}, Wall: 3 * time.Second})
	if err != nil || !res.TimedOut || res.ExitCode != -1 || res.Duration > 40*time.Second {
		t.Errorf("sleep: %+v, %v", res, err)
	}
	if res, err := d.Run(ctx, Spec{Image: image, Command: []string{"/no-such-file"}}); err == nil && res.ExitCode == 0 {
		t.Errorf("a command that is not in the image ran: %+v", res)
	}
}

// probeImage builds testdata/probe as a static binary in an image FROM scratch, and returns the id of the image. The build pulls nothing.
func probeImage(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	build := exec.Command("go", "build", "-trimpath", "-o", filepath.Join(dir, "probe"), "./testdata/probe")
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+runtime.GOARCH)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\nCOPY probe /probe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("docker", "build", "-q", dir).Output()
	if err != nil {
		t.Fatalf("docker build: %v", err)
	}
	id := strings.TrimSpace(string(out))
	if !ValidImage(id) {
		t.Fatalf("docker build gave the id %q", id)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rmi", "-f", id).Run() })
	return id
}
