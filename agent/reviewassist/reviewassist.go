// Package reviewassist has the goal, the brief and the tool of the review-assist role (spec 10.6 and 12.1).
//
// The target of a review-assist job is a node with a Lean binding in the fidelity queue. The run reads the review packet of the node, which the review package makes from the last build, and acts as the critic of spec 10.4: it looks for each way in which a Lean statement can say something other than the informal statement, and names the class of misformalization of spec 10.5. The tool write_packet writes the packet and the concerns to reports/review/TAG.md, which [Path] gives, for the person who reviews the bindings. The run writes nothing in nodes/ or reviews/. Review-assist never signs, and an agent is never a reviewer (rule V03).
//
// The packet of the file has no reviews, thus a blind re-review (spec 10.8) can use it. [Goal] holds when the changes are inside the field permission set of review-assist, the file of the target has the packet of the build, and each concern names a binding of the packet, a class of [battery.Classes] and a reason. The evaluation of spec 17 measures the recall of the concerns and the length of the packet. [Brief] gives the first message of a run.
package reviewassist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/api"
	"github.com/tamnd/math2-agent/battery"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/review"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/workspace"
)

// Dir is the directory of the review packets, from the tree root.
const Dir = "reports/review"

// Width is the width in columns of the text of a packet in a file.
const Width = 100

// MaxConcerns is the most concerns that a packet has. A long list hides the concerns that matter.
const MaxConcerns = 10

// MaxConcern is the most characters of the reason of one concern.
const MaxConcern = 400

// Path returns the path of the review packet of the node from the tree root, for example reports/review/3C53TD.md.
func Path(tag string) string { return path.Join(Dir, tag+".md") }

// A Concern is one way in which a Lean binding can say something other than its node.
type Concern struct {
	Decl  string // the declaration of the binding, or its adapter
	Class string // the class of misformalization of spec 10.5, for example M-junk
	Text  string // the reason, with the part of the statement that it is about
}

// A File is the file of a review packet in reports/review.
type File struct {
	Tag      string
	By       string // the agent id of the run, for example m2-agent/review-assist@0.1.0
	Packet   string // the packet as text, as review.Packet.WriteText writes it at Width
	Concerns []Concern
}

const (
	fence     = "````"
	noConcern = "The critic found no concern."
)

// concernRe matches the line of a concern in the Markdown of a file.
var concernRe = regexp.MustCompile("^- `([^`]+)` (M-[a-z-]+): (.+)$")

// Markdown returns the file as Markdown.
func (f *File) Markdown() []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# Review packet of %s\n\n", f.Tag)
	fmt.Fprintf(&b, "%s made this packet from the last build, for the person who reviews the Lean bindings of %s. It is not a review. To review, run `m2 review %s`.\n\n", f.By, f.Tag, f.Tag)
	fmt.Fprintf(&b, "## Packet\n\n%stext\n%s\n%s\n\n## Concerns\n\n", fence, strings.TrimRight(f.Packet, "\n"), fence)
	if len(f.Concerns) == 0 {
		b.WriteString(noConcern + "\n")
	}
	for _, c := range f.Concerns {
		fmt.Fprintf(&b, "- `%s` %s: %s\n", c.Decl, c.Class, c.Text)
	}
	return b.Bytes()
}

// ParseFile reads a file of a review packet, as [File.Markdown] writes it.
func ParseFile(data []byte) (*File, error) {
	s := string(data)
	head, rest, ok := strings.Cut(s, "\n\n## Packet\n\n"+fence+"text\n")
	if !ok || !strings.HasPrefix(head, "# Review packet of ") {
		return nil, errors.New("reviewassist: the file is not a review packet")
	}
	f := &File{}
	title, intro, _ := strings.Cut(head, "\n\n")
	f.Tag = strings.TrimPrefix(title, "# Review packet of ")
	f.By, _, _ = strings.Cut(intro, " made this packet")
	packet, concerns, ok := strings.Cut(rest, "\n"+fence+"\n\n## Concerns\n\n")
	if !ok {
		return nil, errors.New("reviewassist: the packet has no end or no concerns")
	}
	f.Packet = packet + "\n"
	if strings.TrimSpace(concerns) == noConcern {
		return f, nil
	}
	for line := range strings.Lines(concerns) {
		m := concernRe.FindStringSubmatch(strings.TrimSuffix(line, "\n"))
		if m == nil {
			return nil, fmt.Errorf("reviewassist: the line %q is not a concern in the form - `DECL` CLASS: REASON", strings.TrimSpace(line))
		}
		f.Concerns = append(f.Concerns, Concern{Decl: m[1], Class: m[2], Text: m[3]})
	}
	return f, nil
}

// Packet returns the packet of the node with the tag from the files of the build as text, with no reviews.
func Packet(in review.Input, tag string) (string, review.Packet, error) {
	p, err := review.Make(in, tag)
	if err != nil {
		return "", p, err
	}
	for i := range p.Bindings {
		p.Bindings[i].Reviews = []status.ReviewState{}
	}
	var b strings.Builder
	if err := p.WriteText(&b, Width); err != nil {
		return "", p, err
	}
	return b.String(), p, nil
}

// Check reports the first problem of the concern for the packet: a binding that the packet does not have, a class that spec 10.5 does not have, or no reason or a reason that is too long.
func Check(p review.Packet, c Concern) error {
	if !slices.ContainsFunc(p.Bindings, func(b review.Binding) bool { return b.Decl == c.Decl || b.Checked == c.Decl }) {
		var decls []string
		for _, b := range p.Bindings {
			decls = append(decls, b.Decl)
		}
		return fmt.Errorf("the packet has no binding of %s. Its bindings are %s", c.Decl, strings.Join(decls, ", "))
	}
	if !slices.Contains(battery.Classes(), c.Class) {
		return fmt.Errorf("%s is not a class of spec 10.5. The classes are %s", c.Class, strings.Join(battery.Classes(), ", "))
	}
	switch t := strings.TrimSpace(c.Text); {
	case t == "":
		return fmt.Errorf("the concern %s of %s has no reason", c.Class, c.Decl)
	case len([]rune(t)) > MaxConcern:
		return fmt.Errorf("the reason of the concern %s of %s has more than %d characters. Make it shorter", c.Class, c.Decl, MaxConcern)
	case strings.Contains(t, "\n"):
		return fmt.Errorf("the reason of the concern %s of %s has more than one line", c.Class, c.Decl)
	}
	return nil
}

// Write returns the tool write_packet, which writes the review packet of the target of the job, with the concerns, to reports/review/TAG.md in the work tree. The packet comes from the build, thus the model gives only the concerns. A new call replaces the concerns. The write goes through write_file, thus the field permission set of the role applies to it.
func Write(ws *workspace.Workspace, in review.Input, by string) agent.Tool {
	write := tools.Write(ws)
	return agent.Func{
		Def: llm.Tool{
			Name:        "write_packet",
			Description: fmt.Sprintf("Write the review packet of the target node with your concerns, for the person who reviews its Lean bindings. Give each concern on its own line in the form DECL CLASS: REASON, for example \"Nat.sub_le M-junk: the statement uses natural subtraction, which is 0 when n < m.\" Give no line when you have no concern. A new call replaces the concerns. Give at most %d concerns.", MaxConcerns),
			Schema: tools.Schema([]api.Param{
				{Name: "concerns", Type: api.String, Summary: "the concerns, one on each line in the form DECL CLASS: REASON, or empty for no concern"},
			}),
		},
		Fn: func(ctx context.Context, job *agent.Job, input json.RawMessage) (agent.Result, error) {
			var args struct{ Concerns string }
			if err := json.Unmarshal(input, &args); err != nil {
				return agent.Result{}, err
			}
			text, p, err := Packet(in, job.Target)
			if err != nil {
				return agent.Result{}, err
			}
			f := &File{Tag: job.Target, By: by, Packet: text}
			for line := range strings.Lines(args.Concerns) {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				head, reason, ok := strings.Cut(line, ":")
				words := strings.Fields(head)
				if !ok || len(words) != 2 {
					return agent.Result{}, fmt.Errorf("the line %q is not in the form DECL CLASS: REASON", line)
				}
				c := Concern{Decl: strings.Trim(words[0], "`"), Class: words[1], Text: strings.TrimSpace(reason)}
				if err := Check(p, c); err != nil {
					return agent.Result{}, err
				}
				f.Concerns = append(f.Concerns, c)
			}
			if len(f.Concerns) > MaxConcerns {
				return agent.Result{}, fmt.Errorf("the packet has %d concerns, and the most is %d. Keep the concerns that can change the verdict", len(f.Concerns), MaxConcerns)
			}
			data, err := json.Marshal(map[string]string{"path": Path(job.Target), "text": string(f.Markdown())})
			if err != nil {
				return agent.Result{}, err
			}
			if _, err := write.Call(ctx, job, data); err != nil {
				return agent.Result{}, err
			}
			return agent.Result{Text: fmt.Sprintf("Wrote the packet of %s with %d concerns to %s.", job.Target, len(f.Concerns), Path(job.Target))}, nil
		},
	}
}

// Goal returns the goal predicate of a review-assist run on the work tree: the changes are inside the field permission set of review-assist, the file of the packet of the target is in the changes, its packet is the packet of the build, and [Check] accepts each concern.
func Goal(ws *workspace.Workspace, in review.Input, job *agent.Job) agent.Goal {
	return agent.OnChanges(ws, func(_ context.Context, changes []workspace.Change) (bool, string, error) {
		if err := agent.Permit(job.Role, job.Target, changes); err != nil {
			return false, err.Error(), nil
		}
		i := slices.IndexFunc(changes, func(c workspace.Change) bool { return c.Path == Path(job.Target) })
		if i < 0 {
			return false, fmt.Sprintf("the run has not written the packet %s. Write it with write_packet", Path(job.Target)), nil
		}
		f, err := ParseFile(changes[i].New)
		if err != nil {
			return false, err.Error(), nil
		}
		text, p, err := Packet(in, job.Target)
		if err != nil {
			return false, "", err
		}
		if f.Tag != job.Target || f.Packet != text {
			return false, "the packet of the file is not the packet of the build. Write it with write_packet", nil
		}
		if len(f.Concerns) > MaxConcerns {
			return false, fmt.Sprintf("the packet has %d concerns, and the most is %d", len(f.Concerns), MaxConcerns), nil
		}
		for _, c := range f.Concerns {
			if err := Check(p, c); err != nil {
				return false, err.Error(), nil
			}
		}
		return true, "", nil
	})
}

// Brief returns the first message of a review-assist run: the job, the packet of the target from the build in a data block, and the file of the packet that the tree has, if any.
func Brief(ws *workspace.Workspace, in review.Input, job *agent.Job) (string, error) {
	text, p, err := Packet(in, job.Target)
	if err != nil {
		return "", fmt.Errorf("reviewassist: %w", err)
	}
	if len(p.Bindings) == 0 {
		return "", fmt.Errorf("reviewassist: the target %s has no Lean binding", job.Target)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Job %s: prepare the review packet of the target %s, a %s node, for the person who reviews its Lean bindings. The goal of the job: %s.\n\n", job.ID, job.Target, p.Kind, strings.TrimSuffix(job.Goal, "."))
	fmt.Fprintf(&b, "The packet of the last build:\n\n%s\n", tools.Data("m2 review "+job.Target, tools.Licence(tools.NodePath(job.Target)), text))
	old, err := ws.Read(Path(job.Target))
	switch {
	case err == nil:
		fmt.Fprintf(&b, "\nThe tree has a packet of an earlier run. Your packet replaces it:\n\n%s\n", tools.Data(Path(job.Target), tools.Licence(tools.NodePath(job.Target)), string(old)))
	case !errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("reviewassist: %w", err)
	}
	b.WriteString("\nRead each Lean statement against the informal statement, and write the packet with write_packet.\n")
	return b.String(), nil
}
