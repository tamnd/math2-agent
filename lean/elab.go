package lean

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Message is one message of Lean about a file.
type Message struct {
	Severity  string `json:"severity"`       // error, warning or information
	Kind      string `json:"kind,omitempty"` // for example hasSorry for a declaration that uses sorry
	Line      int    `json:"line"`           // the line of the start, from 1
	Column    int    `json:"column"`         // the column of the start, from 0
	EndLine   int    `json:"end_line,omitempty"`
	EndColumn int    `json:"end_column,omitempty"`
	Text      string `json:"text"`
}

// Elaboration is the result of the elaboration of one Lean file.
type Elaboration struct {
	Messages []Message `json:"messages"`
	Seconds  float64   `json:"seconds"` // the wall time of Lean
}

// Errors returns the messages with the severity error.
func (e *Elaboration) Errors() []Message {
	var out []Message
	for _, m := range e.Messages {
		if m.Severity == "error" {
			out = append(out, m)
		}
	}
	return out
}

// Sorry reports whether a declaration of the file uses sorry.
func (e *Elaboration) Sorry() bool {
	for _, m := range e.Messages {
		if m.Kind == "hasSorry" {
			return true
		}
	}
	return false
}

// Elaborate elaborates the Lean file with "lake env lean --json" in the Lake project directory project, thus the file can import the modules of the project and of its dependencies. The file is a path relative to project, or an absolute path. A file that does not elaborate gives an Elaboration with errors, not an error. The error is for a run of Lean that gave no messages that Elaborate can read, for example when ctx is done. Elaborate needs lake on the PATH.
func Elaborate(ctx context.Context, project, file string) (*Elaboration, error) {
	cmd := exec.CommandContext(ctx, "lake", "env", "lean", "--json", file) // #nosec G204 -- the caller names the file, and no shell reads the arguments.
	cmd.Dir = project
	return ElaborateWith(ctx, cmd, file)
}

// ElaborateWith runs cmd, a command that runs "lean --json" on the file, and reads its messages as Elaborate does. Thus a caller can run Lean in a sandbox. The command must not have its stdout or its stderr set, and ctx must be the context of the command.
func ElaborateWith(ctx context.Context, cmd *exec.Cmd, file string) (*Elaboration, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	err := cmd.Run()
	e := &Elaboration{Messages: []Message{}, Seconds: time.Since(start).Seconds()}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("lean: elaborate %s: %w", file, ctx.Err())
	}
	sc := bufio.NewScanner(&stdout)
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var m struct {
			Severity string `json:"severity"`
			Kind     string `json:"kind"`
			Data     string `json:"data"`
			Pos      struct{ Line, Column int }
			EndPos   *struct{ Line, Column int }
		}
		if json.Unmarshal(line, &m) != nil || m.Severity == "" {
			return nil, fmt.Errorf("lean: elaborate %s: the output %q is not a message", file, line)
		}
		msg := Message{Severity: m.Severity, Kind: m.Kind, Line: m.Pos.Line, Column: m.Pos.Column, Text: m.Data}
		if msg.Kind == "[anonymous]" {
			msg.Kind = ""
		}
		if m.EndPos != nil {
			msg.EndLine, msg.EndColumn = m.EndPos.Line, m.EndPos.Column
		}
		e.Messages = append(e.Messages, msg)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("lean: elaborate %s: %w", file, err)
	}
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit) && len(e.Errors()) > 0:
	default:
		return nil, fmt.Errorf("lean: elaborate %s: %w: %s", file, err, strings.TrimSpace(stderr.String()))
	}
	return e, nil
}
