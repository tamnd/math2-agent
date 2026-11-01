package cli

import (
	"strings"
	"testing"
)

func TestNewCheck(t *testing.T) {
	t.Setenv(CheckEnv, "")
	if m, _, err := newCheck("claude-haiku-5-5"); m != nil || err != nil {
		t.Errorf("with no %s the check is %v, %v", CheckEnv, m, err)
	}
	t.Setenv(CheckEnv, "codex:gpt-6-sol")
	if m, id, err := newCheck("claude-haiku-5-5"); m == nil || id != "gpt-6-sol" || err != nil {
		t.Errorf("codex:gpt-6-sol gives %v, %q, %v", m, id, err)
	}
	for env, want := range map[string]string{
		"claude:claude-opus-5-5": "another family",
		"codex":                  "one backend with its model",
		"codex:gpt-0":            "no entry",
	} {
		t.Setenv(CheckEnv, env)
		if _, _, err := newCheck("claude-haiku-5-5"); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s gives %v", env, err)
		}
	}
}
