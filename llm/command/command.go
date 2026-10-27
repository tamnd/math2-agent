// Package command is a [llm.Model] that runs the command-line client of a model subscription: codex for the ChatGPT models of OpenAI, or claude for Claude Code.
//
// A client of this kind is an agent with its own tools, not a model with the tools of the caller. The package therefore puts the request into one prompt: the system prompt, the tools with their schemas, the conversation so far and a reply protocol. The client runs with no tools of its own, or in a read-only sandbox, in an empty directory, and it answers with one JSON object:
//
//	{"text": "...", "calls": [{"name": "lean_check", "input": {"code": "..."}}]}
//
// The package converts the object to the blocks of a [llm.Response]. Each call gets an id from the position of the response in the conversation, thus a replay of the same conversation gives the same ids.
//
// A [Model] has a list of backends. When a backend fails, the next backend gets the request. When the failure is a usage limit or a login problem, the model does not use the backend again. The client runs without the API keys of the environment, thus it uses the subscription and never the credits of an API account.
package command

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tamnd/math2-agent/llm"
)

// The clients that the package can run.
const (
	Codex  = "codex"
	Claude = "claude"
)

// Env is the environment variable that m2 reads for the backends of a [Model], in the form of [Parse]. When it is set, m2 uses the subscriptions and not the Claude API.
const Env = "M2_LLM_COMMAND"

// Backend is one client.
type Backend struct {
	Name  string // Codex or Claude
	Path  string // the executable. Empty means Name on the PATH.
	Model string // the model to ask for. Empty means the model of the request for claude when it is a Claude model, and the default of the client otherwise.
}

// Parse reads a list of backends, separated by commas, in order of preference. Each backend is a name, with an optional model after a colon, for example "codex,claude:claude-opus-5-5".
func Parse(s string) ([]Backend, error) {
	var out []Backend
	for _, f := range strings.Split(s, ",") {
		name, model, _ := strings.Cut(strings.TrimSpace(f), ":")
		if name != Codex && name != Claude {
			return nil, fmt.Errorf("command: the backend %q is not %s or %s", name, Codex, Claude)
		}
		out = append(out, Backend{Name: name, Model: model})
	}
	return out, nil
}

// Model is a [llm.Model] on a list of backends.
type Model struct {
	Backends []Backend
	Timeout  time.Duration // the limit of one run of a client. Zero means 15 minutes.

	mu   sync.Mutex
	down map[string]error // the backends that the model does not use again, with the reason
}

// New returns a model on the backends.
func New(backends ...Backend) *Model { return &Model{Backends: backends} }

// Complete puts the request into a prompt and runs the backends in order until one of them answers.
func (m *Model) Complete(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	prompt, err := Prompt(req)
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, b := range m.Backends {
		m.mu.Lock()
		why := m.down[b.Name]
		m.mu.Unlock()
		if why != nil {
			errs = append(errs, why)
			continue
		}
		resp, err := m.run(ctx, b, req, prompt)
		if err == nil {
			return resp, nil
		}
		err = fmt.Errorf("command: %s: %w", b.Name, err)
		errs = append(errs, err)
		if ctx.Err() != nil {
			break
		}
		if exhausted(err.Error()) {
			m.mu.Lock()
			if m.down == nil {
				m.down = map[string]error{}
			}
			m.down[b.Name] = err
			m.mu.Unlock()
		}
	}
	if len(errs) == 0 {
		return nil, errors.New("command: the model has no backend")
	}
	return nil, errors.Join(errs...)
}

// exhausted reports whether the message of a failure is a usage limit or a login problem, which another try does not repair.
func exhausted(msg string) bool {
	msg = strings.ToLower(msg)
	for _, s := range []string{"usage limit", "rate limit", "hit your limit", "limit reached", "quota", "not logged in", "log in", "login", "unauthorized", "authentication", "expired", "status 401", "status 403", "status 429"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// run runs one client on the prompt in an empty directory and reads its reply.
func (m *Model) run(ctx context.Context, b Backend, req *llm.Request, prompt string) (*llm.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, cmp.Or(m.Timeout, 15*time.Minute))
	defer cancel()
	dir, err := os.MkdirTemp("", "m2-command-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	work := filepath.Join(dir, "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		return nil, err
	}
	last := filepath.Join(dir, "last.txt")

	var args []string
	model := b.Model
	effort := req.Effort
	switch b.Name {
	case Codex:
		args = []string{"exec", "--skip-git-repo-check", "--ephemeral", "--ignore-user-config", "--sandbox", "read-only", "-C", work, "--json", "-o", last, "-c", "shell_environment_policy.inherit=none"}
		if model != "" {
			args = append(args, "-m", model)
		}
		if effort == "xhigh" || effort == "max" {
			effort = "high"
		}
		if effort != "" {
			args = append(args, "-c", "model_reasoning_effort="+effort)
		}
		args = append(args, "-")
	case Claude:
		if model == "" && strings.HasPrefix(req.Model, "claude-") {
			model = req.Model
		}
		args = []string{"-p", "--output-format", "json", "--tools", "", "--no-session-persistence", "--strict-mcp-config", "--setting-sources", "", "--system-prompt", system}
		if model != "" {
			args = append(args, "--model", model)
		}
		if effort != "" {
			args = append(args, "--effort", effort)
		}
	default:
		return nil, fmt.Errorf("the backend %q is not %s or %s", b.Name, Codex, Claude)
	}
	cmd := exec.CommandContext(ctx, cmp.Or(b.Path, b.Name), args...) // #nosec G204 -- the backend is codex or claude, from the configuration of the operator.
	cmd.Dir = work
	cmd.Env = environ()
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()

	var text string
	var r *llm.Response
	switch b.Name {
	case Codex:
		u, failure := codexEvents(stdout.Bytes())
		if failure != "" {
			return nil, errors.New(failure)
		}
		if runErr != nil {
			return nil, fmt.Errorf("%w: %s", runErr, tail(stderr.String()))
		}
		data, err := os.ReadFile(last) // #nosec G304 -- the file is in the directory of the run.
		if err != nil {
			return nil, fmt.Errorf("the client wrote no reply: %w", err)
		}
		text = string(data)
		r = &llm.Response{Model: cmp.Or(model, Codex), Usage: u}
	case Claude:
		var out struct {
			IsError        bool   `json:"is_error"`
			Subtype        string `json:"subtype"`
			Result         string `json:"result"`
			APIErrorStatus *int   `json:"api_error_status"`
			Usage          struct {
				Input      int `json:"input_tokens"`
				Output     int `json:"output_tokens"`
				CacheRead  int `json:"cache_read_input_tokens"`
				CacheWrite int `json:"cache_creation_input_tokens"`
			} `json:"usage"`
			ModelUsage map[string]json.RawMessage `json:"modelUsage"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			if runErr != nil {
				return nil, fmt.Errorf("%w: %s", runErr, tail(stderr.String()+stdout.String()))
			}
			return nil, fmt.Errorf("the output is not JSON: %w", err)
		}
		if out.IsError || runErr != nil {
			status := ""
			if out.APIErrorStatus != nil {
				status = fmt.Sprintf(" (status %d)", *out.APIErrorStatus)
			}
			return nil, fmt.Errorf("%s%s: %s", cmp.Or(out.Subtype, "error"), status, tail(out.Result))
		}
		text = out.Result
		models := make([]string, 0, len(out.ModelUsage))
		for k := range out.ModelUsage {
			models = append(models, k)
		}
		slices.Sort(models)
		if len(models) == 1 {
			model = models[0]
		}
		r = &llm.Response{Model: cmp.Or(model, Claude), Usage: llm.Usage{Input: out.Usage.Input, Output: out.Usage.Output, CacheRead: out.Usage.CacheRead, CacheWrite: out.Usage.CacheWrite}}
	}
	r.Content, r.StopReason = Reply(text, len(req.Messages))
	return r, nil
}

// codexEvents reads the events of codex exec --json. It returns the usage of the turn and the message of the first failure, or "".
func codexEvents(data []byte) (llm.Usage, string) {
	var u llm.Usage
	for _, line := range bytes.Split(data, []byte("\n")) {
		var e struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Error   struct {
				Message string `json:"message"`
			} `json:"error"`
			Usage struct {
				Input  int `json:"input_tokens"`
				Cached int `json:"cached_input_tokens"`
				Output int `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(line, &e) != nil {
			continue
		}
		switch e.Type {
		case "error", "turn.failed":
			return u, cmp.Or(e.Message, e.Error.Message, e.Type)
		case "turn.completed":
			cached := min(e.Usage.Cached, e.Usage.Input)
			u = u.Add(llm.Usage{Input: e.Usage.Input - cached, Output: e.Usage.Output, CacheRead: cached})
		}
	}
	return u, ""
}

// environ returns the environment of the process without the keys of the API accounts, thus the client uses its subscription.
func environ() []string {
	var out []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "OPENAI_API_KEY", "CODEX_API_KEY":
			continue
		}
		out = append(out, kv)
	}
	return out
}

// tail returns the last 500 bytes of the text, without the space at its ends.
func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		s = "..." + s[len(s)-500:]
	}
	return s
}

// system is the system prompt of claude. The system prompt of the role is in the prompt, with the rest of the request.
const system = "You are the language model behind an agent program. The program gives you a task, a list of tools and the conversation so far. You cannot run the tools yourself. You answer with one JSON object, and the program runs the tools that you call and sends you the results in the next prompt."

// protocol is the end of each prompt.
const protocol = `# Your reply

Reply with one JSON object and nothing else, with no code fence:

{"text": "what you say to the user, or an empty string", "calls": [{"name": "a tool of the list", "input": {"the": "input, as the schema of the tool says"}}]}

Put the tool calls that you want in "calls", in the order to run them. When the task is done, or when you only answer in words, give an empty list. Do not run commands, read files or use tools of your own: the program runs the tools of the list for you.
`

// Prompt puts the request into one prompt: the system prompt of the role, the tools, the conversation and the reply protocol.
func Prompt(req *llm.Request) (string, error) {
	var b strings.Builder
	if req.System != "" {
		fmt.Fprintf(&b, "# Instructions\n\n%s\n\n", strings.TrimSpace(req.System))
	}
	if len(req.Tools) > 0 {
		b.WriteString("# Tools\n\n")
		for _, t := range req.Tools {
			var schema bytes.Buffer
			if err := json.Compact(&schema, t.Schema); err != nil {
				return "", fmt.Errorf("command: the schema of the tool %s: %w", t.Name, err)
			}
			fmt.Fprintf(&b, "## %s\n\n%s\n\nInput schema: %s\n\n", t.Name, strings.TrimSpace(t.Description), schema.String())
		}
	}
	b.WriteString("# Conversation\n\n")
	names := map[string]string{} // the tool of each call id
	for i, msg := range req.Messages {
		if msg.Role == llm.Assistant {
			r := reply{Text: "", Calls: []call{}}
			for _, blk := range msg.Content {
				switch blk.Type {
				case llm.Text:
					r.Text = strings.TrimSpace(r.Text + "\n\n" + blk.Text)
				case llm.ToolUse:
					names[blk.ID] = blk.Name
					input := blk.Input
					if len(input) == 0 {
						input = json.RawMessage("{}")
					}
					r.Calls = append(r.Calls, call{ID: blk.ID, Name: blk.Name, Input: input})
				}
			}
			data, err := json.Marshal(r)
			if err != nil {
				return "", fmt.Errorf("command: message %d: %w", i, err)
			}
			fmt.Fprintf(&b, "## Your reply\n\n%s\n\n", data)
			continue
		}
		b.WriteString("## Program\n\n")
		for _, blk := range msg.Content {
			switch blk.Type {
			case llm.Text:
				fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(blk.Text))
			case llm.ToolResult:
				status := "result"
				if blk.IsError {
					status = "error"
				}
				fmt.Fprintf(&b, "The %s of the call %s (%s):\n\n%s\n\n", status, blk.ToolUseID, cmp.Or(names[blk.ToolUseID], "a tool that is not known"), strings.TrimSpace(blk.Content))
			}
		}
	}
	b.WriteString(protocol)
	return b.String(), nil
}

// reply is the JSON object of the reply protocol.
type reply struct {
	Text  string `json:"text"`
	Calls []call `json:"calls"`
}

type call struct {
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// Reply reads the reply of a client to a request with n messages. It returns the blocks of the reply and the stop reason. A reply that is not an object of the protocol is a text block. The ids of the calls are "c" and n, then "_" and the position of the call.
func Reply(text string, n int) ([]llm.Block, string) {
	s := strings.TrimSpace(text)
	if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i {
		s = s[i : j+1]
	}
	var r reply
	if json.Unmarshal([]byte(s), &r) != nil {
		return []llm.Block{llm.TextBlock(strings.TrimSpace(text))}, llm.EndTurn
	}
	var blocks []llm.Block
	if t := strings.TrimSpace(r.Text); t != "" {
		blocks = append(blocks, llm.TextBlock(t))
	}
	for i, c := range r.Calls {
		input := c.Input
		if len(bytes.TrimSpace(input)) == 0 || string(input) == "null" {
			input = json.RawMessage("{}")
		}
		blocks = append(blocks, llm.Block{Type: llm.ToolUse, ID: fmt.Sprintf("c%d_%d", n, i+1), Name: c.Name, Input: input})
	}
	if len(r.Calls) > 0 {
		return blocks, llm.ToolCall
	}
	return blocks, llm.EndTurn
}
