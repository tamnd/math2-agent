package tools_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/sandbox"
)

// TestMain runs the test binary as a fake docker CLI when M2_FAKE_DOCKER names a log file.
func TestMain(m *testing.M) {
	if log := os.Getenv("M2_FAKE_DOCKER"); log != "" {
		os.Exit(fakeDocker(log, os.Args[1:]))
	}
	os.Exit(m.Run())
}

// fakeDocker writes the arguments to the log and does what the command in the container asks, with the tree of the mount at /out: lake build passes, lean --json gives one message, and m2extract answers each request with a theorem of type Prop.
func fakeDocker(log string, args []string) int {
	f, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 -- the test names the log.
	if err != nil {
		return 125
	}
	line, _ := json.Marshal(args)
	_, _ = f.Write(append(line, '\n'))
	_ = f.Close()
	i := slices.Index(args, "--")
	if i < 0 || !slices.Contains(args, "none") || len(args) < i+5 {
		return 125
	}
	out := ""
	for j, a := range args[:i] {
		if src, ok := strings.CutPrefix(a, "type=bind,source="); ok && args[j-1] == "--mount" && strings.HasSuffix(src, ",target=/out") {
			out = strings.TrimSuffix(src, ",target=/out")
		}
	}
	cmd := args[i+2:] // lake -d /out ...
	switch {
	case cmd[3] == "build":
		return 0
	case cmd[3] == "env" && cmd[4] == "lean":
		rel := strings.TrimPrefix(cmd[len(cmd)-1], "/out/")
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			fmt.Println(`{"severity":"error","pos":{"line":1,"column":0},"data":"no file"}`)
			return 1
		}
		fmt.Println(`{"severity":"information","pos":{"line":1,"column":0},"data":"checked in the box"}`)
		return 0
	case cmd[3] == "env":
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			var req struct {
				Cmd   string   `json:"cmd"`
				Decls []string `json:"decls"`
			}
			_ = json.Unmarshal(sc.Bytes(), &req)
			switch req.Cmd {
			case "extract":
				var rs []map[string]any
				for _, d := range req.Decls {
					rs = append(rs, map[string]any{"decl": d, "kind": "theorem", "module": "Math2.T", "type": "s 0\n", "axioms": []string{}})
				}
				b, _ := json.Marshal(map[string]any{"records": rs})
				fmt.Println(string(b))
			case "checks":
				fmt.Println(`{"checks":[]}`)
			default:
				fmt.Println(`{}`)
			}
		}
		return 0
	}
	return 127
}

func TestBox(t *testing.T) {
	log := filepath.Join(t.TempDir(), "docker.log")
	t.Setenv("M2_FAKE_DOCKER", log)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ws := tree(t, map[string]string{"lean/Math2/T.lean": "module\n\npublic import Math2.U\n\ntheorem Math2.T.s : True := trivial\n"})
	l := &tools.Lake{Tree: ws.Dir, Exe: "/bin/m2extract", Box: &tools.Box{
		Docker: &sandbox.Docker{Binary: exe},
		Image:  "m2-lean@sha256:" + strings.Repeat("0", 64), Packages: "/var/m2/packages", CPU: 2,
	}}
	ctx := context.Background()
	e, err := l.Check(ctx, "lean/Math2/T.lean")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Messages) != 1 || e.Messages[0].Text != "checked in the box" {
		t.Errorf("the elaboration is %+v", e)
	}
	rs, err := l.Extract(ctx, []string{"Math2.T.s"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].Decl != "Math2.T.s" || !rs[0].Statement() {
		t.Errorf("the records are %+v", rs)
	}

	data, err := os.ReadFile(log) // #nosec G304 -- the log is in the temporary directory of the test.
	if err != nil {
		t.Fatal(err)
	}
	project, _ := filepath.Abs(filepath.Join(ws.Dir, "lean"))
	var commands []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var args []string
		if err := json.Unmarshal([]byte(line), &args); err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(args, " ")
		for _, want := range []string{"run --interactive ", "--network none", "--read-only", "--cpus 2", "type=bind,source=/var/m2/packages,target=/work,readonly", "type=bind,source=" + project + ",target=/out"} {
			if !strings.Contains(joined, want) {
				t.Errorf("the run %s has no %q", joined, want)
			}
		}
		_, cmd, _ := strings.Cut(joined, " -- ")
		commands = append(commands, cmd)
	}
	image := "m2-lean@sha256:" + strings.Repeat("0", 64)
	want := []string{
		image + " lake -d /out build -q Math2.U",
		image + " lake -d /out env lean --json /out/Math2/T.lean",
		image + " lake -d /out build -q",
		image + " lake -d /out env /bin/m2extract",
	}
	if !slices.Equal(commands, want) {
		t.Errorf("the commands are\n%s\nwant\n%s", strings.Join(commands, "\n"), strings.Join(want, "\n"))
	}
}
