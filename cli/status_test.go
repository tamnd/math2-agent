package cli

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/status"
)

const valid = "../testdata/nodes/valid"

// TestStatusAgreesWithBuild checks that m2 status gives the status of the build for each node.
func TestStatusAgreesWithBuild(t *testing.T) {
	out := t.TempDir()
	if _, err := build.Run(context.Background(), build.Options{Root: valid, Out: out}); err != nil {
		t.Fatal(err)
	}
	all, err := build.ReadStatus(out)
	if err != nil || len(all) == 0 {
		t.Fatalf("ReadStatus gives %d statuses, %v", len(all), err)
	}
	for _, want := range all {
		code, stdout, stderr := run("status", "-root", valid, "-out", out, "-json", want.Tag)
		if code != ExitOK {
			t.Fatalf("m2 status %s exits with code %d: %s", want.Tag, code, stderr)
		}
		var got status.Status
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("m2 status %s gives %+v, and the build gives %+v", want.Tag, got, want)
		}
		code, stdout, _ = run("status", "-root", valid, "-out", out, want.Tag)
		if code != ExitOK || !strings.HasPrefix(stdout, want.Tag+" ") || !strings.Contains(stdout, "evidence") {
			t.Errorf("m2 status %s exits with code %d and prints:\n%s", want.Tag, code, stdout)
		}
		if want.StatementHash != "" && !strings.Contains(stdout, "statement hash  "+want.StatementHash+"\n") {
			t.Errorf("m2 status %s does not print the statement hash %s:\n%s", want.Tag, want.StatementHash, stdout)
		}
	}
}

func TestStatusResolve(t *testing.T) {
	out := t.TempDir()
	if _, err := build.Run(context.Background(), build.Options{Root: valid, Out: out}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		ref  string
		code int
		want string
	}{
		{"Q529ES", ExitOK, "Q529ES "},
		{"lagrange-theorem-groups", ExitOK, "Q529ES "},
		{"m2://Q529ES", ExitOK, "Q529ES "},
		{"m2://statement/lagrange-theorem-groups", ExitOK, "Q529ES "},
		{"m2://concept/lagrange-theorem-groups", ExitFailure, "is a statement, not a concept"},
		{"m2://Q529ES@2027.03", ExitFailure, "only the last build"},
		{"m2://msc/20D60", ExitFailure, "cannot resolve an alias in the namespace msc"},
		{"ZZZZZZ", ExitFailure, "no node has the tag ZZZZZZ"},
		{"no-such-alias", ExitFailure, "no node has the alias no-such-alias"},
		{"Not a tag", ExitFailure, "is not a tag, an alias or an m2:// URI"},
	}
	for _, tt := range tests {
		code, stdout, stderr := run("status", "-root", valid, "-out", out, tt.ref)
		if code != tt.code || !strings.Contains(stdout+stderr, tt.want) {
			t.Errorf("m2 status %s: code %d, output %q %q, want code %d and %q", tt.ref, code, stdout, stderr, tt.code, tt.want)
		}
	}
}

func TestStatusWithoutBuild(t *testing.T) {
	code, _, stderr := run("status", "-root", valid, "-out", t.TempDir(), "Q529ES")
	if code != ExitFailure || !strings.Contains(stderr, "Run m2 build first") {
		t.Errorf("code %d, stderr %q", code, stderr)
	}
	if code, _, _ := run("status", "-root", valid); code != ExitUsage {
		t.Errorf("no argument gives the code %d, want %d", code, ExitUsage)
	}
}

// TestStatusStatementHash checks that the status of a statement has the hash that a review records, and that a node without a statement section has none.
func TestStatusStatementHash(t *testing.T) {
	out := t.TempDir()
	if _, err := build.Run(context.Background(), build.Options{Root: valid, Out: out}); err != nil {
		t.Fatal(err)
	}
	all, err := build.ReadStatus(out)
	if err != nil {
		t.Fatal(err)
	}
	var with int
	for _, s := range all {
		if s.StatementHash != "" {
			with++
			if !strings.HasPrefix(s.StatementHash, "blake3:") {
				t.Errorf("%s has the statement hash %q", s.Tag, s.StatementHash)
			}
		}
		if s.Tag == "Q529ES" && s.StatementHash == "" {
			t.Errorf("the statement Q529ES has no statement hash")
		}
	}
	if with == len(all) {
		t.Errorf("each of the %d nodes has a statement hash", len(all))
	}
}
