package cli

import (
	"bytes"
	"strings"
	"testing"
)

func run(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := Run(args, "v0.0.0-test", &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestNoArgumentsIsUsageError(t *testing.T) {
	code, _, stderr := run()
	if code != ExitUsage {
		t.Fatalf("exit code is %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(stderr, "Usage: m2") {
		t.Fatalf("stderr does not show the usage:\n%s", stderr)
	}
}

func TestHelpListsEveryCommand(t *testing.T) {
	code, stdout, _ := run("help")
	if code != ExitOK {
		t.Fatalf("exit code is %d, want %d", code, ExitOK)
	}
	for _, c := range commands {
		if !strings.Contains(stdout, "  "+c.name+" ") {
			t.Errorf("help does not list %q", c.name)
		}
	}
}

func TestVersion(t *testing.T) {
	code, stdout, _ := run("version")
	if code != ExitOK || stdout != "m2 v0.0.0-test\n" {
		t.Fatalf("got code %d and %q", code, stdout)
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, stderr := run("frobnicate")
	if code != ExitUsage {
		t.Fatalf("exit code is %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Fatalf("stderr does not say the command is unknown: %s", stderr)
	}
}

func TestCommandNotWrittenNamesItsMilestone(t *testing.T) {
	for _, c := range commands {
		if c.run != nil {
			continue
		}
		code, stdout, stderr := run(c.name)
		if code != ExitNotWritten {
			t.Errorf("%s: exit code is %d, want %d", c.name, code, ExitNotWritten)
		}
		if stdout != "" {
			t.Errorf("%s: a command that is not written must not write to stdout, got %q", c.name, stdout)
		}
		m := goal(c.milestone)
		if !strings.Contains(stderr, m.ID+", "+m.Title) {
			t.Errorf("%s: stderr does not name %s: %q", c.name, m.ID, stderr)
		}
	}
}

func TestCommandTable(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range commands {
		if seen[c.name] {
			t.Errorf("%s: the name is in the table two times", c.name)
		}
		seen[c.name] = true
		if c.usage == "" || c.summary == "" {
			t.Errorf("%s: usage and summary must not be empty", c.name)
		}
		if !strings.HasPrefix(c.usage, "m2 "+c.name) {
			t.Errorf("%s: the usage must start with \"m2 %s\"", c.name, c.name)
		}
		if c.run == nil && c.milestone == "" {
			t.Errorf("%s: a command that is not written must name a milestone", c.name)
		}
		if c.run != nil && c.milestone != "" {
			t.Errorf("%s: a written command must not name a milestone", c.name)
		}
		if c.milestone != "" && goal(c.milestone).Title == "an unknown milestone" {
			t.Errorf("%s: the milestone %q is not in the plan", c.name, c.milestone)
		}
	}
}

func TestHelpForOneCommand(t *testing.T) {
	code, stdout, _ := run("help", "ingest")
	if code != ExitOK {
		t.Fatalf("exit code is %d, want %d", code, ExitOK)
	}
	if !strings.Contains(stdout, "Usage: m2 ingest") || !strings.Contains(stdout, "M2, the graph meets the world") {
		t.Fatalf("unexpected help:\n%s", stdout)
	}
}
