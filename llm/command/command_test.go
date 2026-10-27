package command

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/llm"
)

// TestMain runs the test binary as a fake client when FAKE_CLIENT is set. The fake reads the prompt, appends a line to the file FAKE_LOG and answers as codex or claude.
//
// The modes in FAKE_CODEX and FAKE_CLAUDE are: ok, which answers with the reply in FAKE_REPLY; limit, which reports a usage limit; and crash, which exits with no output.
func TestMain(m *testing.M) {
	if os.Getenv("FAKE_CLIENT") != "" {
		os.Exit(fake())
	}
	os.Exit(m.Run())
}

func fake() int {
	args := os.Args[1:]
	prompt, _ := io.ReadAll(os.Stdin)
	name, mode := Claude, os.Getenv("FAKE_CLAUDE")
	if len(args) > 0 && args[0] == "exec" {
		name, mode = Codex, os.Getenv("FAKE_CODEX")
	}
	wd, _ := os.Getwd()
	entries, _ := os.ReadDir(wd)
	line, _ := json.Marshal(map[string]any{"name": name, "args": args, "prompt": string(prompt), "key": os.Getenv("ANTHROPIC_API_KEY") + os.Getenv("OPENAI_API_KEY"), "files": len(entries)})
	f, err := os.OpenFile(os.Getenv("FAKE_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return 3
	}
	_, _ = f.Write(append(line, '\n'))
	_ = f.Close()
	reply := os.Getenv("FAKE_REPLY")
	switch {
	case mode == "crash":
		return 2
	case name == Codex && mode == "limit":
		fmt.Println(`{"type":"thread.started","thread_id":"t1"}`)
		fmt.Println(`{"type":"error","message":"You've hit your usage limit. Try again at 4:09 PM."}`)
		fmt.Println(`{"type":"turn.failed","error":{"message":"You've hit your usage limit. Try again at 4:09 PM."}}`)
		return 1
	case name == Codex:
		last := args[slices.Index(args, "-o")+1]
		if os.WriteFile(last, []byte(reply), 0o600) != nil {
			return 3
		}
		fmt.Println(`{"type":"thread.started","thread_id":"t1"}`)
		fmt.Println(`{"type":"turn.started"}`)
		fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":1200,"cached_input_tokens":1000,"output_tokens":30}}`)
	case mode == "limit":
		fmt.Println(`{"type":"result","subtype":"success","is_error":true,"api_error_status":429,"result":"You've hit your limit · resets 5pm"}`)
		return 1
	default:
		out, _ := json.Marshal(map[string]any{"type": "result", "subtype": "success", "is_error": false, "result": reply, "usage": map[string]int{"input_tokens": 2, "output_tokens": 16, "cache_read_input_tokens": 100, "cache_creation_input_tokens": 2415}, "modelUsage": map[string]any{"claude-sonnet-5-5": map[string]int{"inputTokens": 2}}})
		fmt.Println(string(out))
	}
	return 0
}

type logLine struct {
	Name   string
	Args   []string
	Prompt string
	Key    string
	Files  int
}

// fakeModel returns a model on codex and claude that runs the test binary as the fake client, with the modes, and the file of its log.
func fakeModel(t *testing.T, codex, claude, reply string) (*Model, string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "log")
	t.Setenv("FAKE_CLIENT", "1")
	t.Setenv("FAKE_LOG", log)
	t.Setenv("FAKE_CODEX", codex)
	t.Setenv("FAKE_CLAUDE", claude)
	t.Setenv("FAKE_REPLY", reply)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	return New(Backend{Name: Codex, Path: exe}, Backend{Name: Claude, Path: exe}), log
}

func readLog(t *testing.T, file string) []logLine {
	t.Helper()
	data, err := os.ReadFile(file) // #nosec G304 -- the file is in the test directory.
	if err != nil {
		t.Fatal(err)
	}
	var out []logLine
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var l logLine
		if err := json.Unmarshal([]byte(line), &l); err != nil {
			t.Fatal(err)
		}
		out = append(out, l)
	}
	return out
}

// request is a conversation with a tool call and its result.
func request() *llm.Request {
	return &llm.Request{
		Model:     "claude-sonnet-5-5",
		System:    "You prove lemmas.",
		MaxTokens: 1000,
		Effort:    "xhigh",
		Tools:     []llm.Tool{{Name: "lean_check", Description: "Check Lean code.", Schema: json.RawMessage(`{"type": "object", "properties": {"code": {"type": "string"}}}`)}},
		Messages: []llm.Message{
			{Role: llm.User, Content: []llm.Block{llm.TextBlock("Prove 0 + n = n.")}},
			{Role: llm.Assistant, Content: []llm.Block{llm.TextBlock("I check a case."), {Type: llm.ToolUse, ID: "c1_1", Name: "lean_check", Input: json.RawMessage(`{"code":"example : 0 + 0 = 0 := rfl"}`)}}},
			{Role: llm.User, Content: []llm.Block{llm.ResultBlock("c1_1", "unknown tactic", true)}},
		},
	}
}

func TestPrompt(t *testing.T) {
	p, err := Prompt(request())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Instructions\n\nYou prove lemmas.\n\n# Tools\n\n## lean_check\n\nCheck Lean code.\n\nInput schema: {\"type\":\"object\",\"properties\":{\"code\":{\"type\":\"string\"}}}\n\n# Conversation\n\n## Program\n\nProve 0 + n = n.\n\n",
		`## Your reply` + "\n\n" + `{"text":"I check a case.","calls":[{"id":"c1_1","name":"lean_check","input":{"code":"example : 0 + 0 = 0 := rfl"}}]}`,
		"The error of the call c1_1 (lean_check):\n\nunknown tactic\n\n# Your reply",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("the prompt has no %q:\n%s", want, p)
		}
	}
	bad := request()
	bad.Tools[0].Schema = json.RawMessage("{")
	if _, err := Prompt(bad); err == nil {
		t.Error("a tool with a schema that is not JSON gives a prompt")
	}
}

func TestReply(t *testing.T) {
	for _, c := range []struct {
		text   string
		blocks []llm.Block
		stop   string
	}{
		{`{"text": "Done.", "calls": []}`, []llm.Block{llm.TextBlock("Done.")}, llm.EndTurn},
		{"```json\n{\"text\": \"\", \"calls\": [{\"name\": \"lean_check\", \"input\": {\"code\": \"x\"}}, {\"name\": \"report_stuck\"}]}\n```", []llm.Block{
			{Type: llm.ToolUse, ID: "c3_1", Name: "lean_check", Input: json.RawMessage(`{"code": "x"}`)},
			{Type: llm.ToolUse, ID: "c3_2", Name: "report_stuck", Input: json.RawMessage(`{}`)},
		}, llm.ToolCall},
		{"I cannot answer in JSON.", []llm.Block{llm.TextBlock("I cannot answer in JSON.")}, llm.EndTurn},
		{`{"text": "", "calls": []}`, nil, llm.EndTurn},
	} {
		blocks, stop := Reply(c.text, 3)
		if !reflect.DeepEqual(blocks, c.blocks) || stop != c.stop {
			t.Errorf("Reply(%q) = %+v, %s", c.text, blocks, stop)
		}
	}
}

func TestComplete(t *testing.T) {
	m, log := fakeModel(t, "ok", "ok", `{"text": "", "calls": [{"name": "lean_check", "input": {"code": "theorem t (n : Nat) : 0 + n = n := by simp"}}]}`)
	resp, err := m.Complete(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	want := &llm.Response{
		Model:      Codex,
		Content:    []llm.Block{{Type: llm.ToolUse, ID: "c3_1", Name: "lean_check", Input: json.RawMessage(`{"code": "theorem t (n : Nat) : 0 + n = n := by simp"}`)}},
		StopReason: llm.ToolCall,
		Usage:      llm.Usage{Input: 200, Output: 30, CacheRead: 1000},
	}
	if !reflect.DeepEqual(resp, want) {
		t.Errorf("the response is %+v", resp)
	}
	l := readLog(t, log)
	if len(l) != 1 || l[0].Name != Codex || l[0].Key != "" || l[0].Files != 0 {
		t.Fatalf("the runs are %+v", l)
	}
	args := strings.Join(l[0].Args, " ")
	for _, want := range []string{"exec ", "--sandbox read-only", "--ignore-user-config", "-c model_reasoning_effort=high", " -"} {
		if !strings.Contains(args, want) {
			t.Errorf("the arguments of codex have no %q: %s", want, args)
		}
	}
	if !strings.HasPrefix(l[0].Prompt, "# Instructions\n\nYou prove lemmas.") {
		t.Errorf("the prompt is %q", l[0].Prompt)
	}
}

func TestFallback(t *testing.T) {
	m, log := fakeModel(t, "limit", "ok", `{"text": "The proof is done.", "calls": []}`)
	for range 2 {
		resp, err := m.Complete(context.Background(), request())
		if err != nil {
			t.Fatal(err)
		}
		if resp.Model != "claude-sonnet-5-5" || resp.Text() != "The proof is done." || resp.StopReason != llm.EndTurn || resp.Usage != (llm.Usage{Input: 2, Output: 16, CacheRead: 100, CacheWrite: 2415}) {
			t.Errorf("the response is %+v", resp)
		}
	}
	// codex is not run again after its usage limit.
	l := readLog(t, log)
	var names []string
	for _, x := range l {
		names = append(names, x.Name)
	}
	if !slices.Equal(names, []string{Codex, Claude, Claude}) {
		t.Fatalf("the runs are %v", names)
	}
	args := strings.Join(l[1].Args, " ")
	for _, want := range []string{"-p --output-format json --tools  --no-session-persistence", "--model claude-sonnet-5-5", "--effort xhigh"} {
		if !strings.Contains(args, want) {
			t.Errorf("the arguments of claude have no %q: %s", want, args)
		}
	}
	if l[1].Key != "" {
		t.Error("claude ran with the API key")
	}
}

func TestFailure(t *testing.T) {
	m, log := fakeModel(t, "crash", "limit", "")
	_, err := m.Complete(context.Background(), request())
	if err == nil || !strings.Contains(err.Error(), "command: codex: exit status 2") || !strings.Contains(err.Error(), "command: claude: success (status 429): You've hit your limit") {
		t.Fatalf("the error is %v", err)
	}
	// A crash is not a limit, thus codex runs again. claude hit its limit, thus it does not.
	_, err = m.Complete(context.Background(), request())
	if err == nil {
		t.Fatal("the second request has no error")
	}
	var names []string
	for _, x := range readLog(t, log) {
		names = append(names, x.Name)
	}
	if !slices.Equal(names, []string{Codex, Claude, Codex}) {
		t.Errorf("the runs are %v", names)
	}
	if _, err := m.Complete(context.Background(), &llm.Request{}); err == nil {
		t.Error("a request with no model gives no error")
	}
}

func TestParse(t *testing.T) {
	b, err := Parse("codex, claude:claude-opus-5-5")
	if err != nil || !reflect.DeepEqual(b, []Backend{{Name: Codex}, {Name: Claude, Model: "claude-opus-5-5"}}) {
		t.Errorf("Parse = %+v, %v", b, err)
	}
	if _, err := Parse("codex,opencode"); err == nil {
		t.Error("Parse accepts opencode")
	}
}
