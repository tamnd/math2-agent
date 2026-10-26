package prompts_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/prompts"
)

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
		if !strings.Contains(r.System, "# Lean conventions of math2") {
			t.Errorf("%s: the system prompt has no style file", r.Name)
		}
		log, err := fs.ReadFile(prompts.FS, e.Name()+"/CHANGELOG.md")
		if err != nil || !strings.Contains(string(log), "\n## "+r.Version+"\n") {
			t.Errorf("%s: the CHANGELOG has no entry for the version %s", r.Name, r.Version)
		}
		for name, text := range map[string]string{"system": r.System} {
			if strings.Contains(text, "—") {
				t.Errorf("%s: the %s prompt has an em dash", r.Name, name)
			}
		}
	}
}
