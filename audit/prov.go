package audit

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/tag"
)

// The rules P01 to P04 read the provenance logs in prov/ (spec 04.8). P02 and P03 also read the summaries of the agent runs. The artefact store has them, and the audit reads its copy in build/runs/. A rule that needs the runs skips when that directory does not exist.

// runsDir is the copy of the run summaries in the tree.
var runsDir = filepath.Join("build", prov.RunsDir)

// logLine is a line of a provenance log with its line number in the file.
type logLine struct {
	n    int
	raw  string
	line prov.Line
	err  error // the line is not JSON
}

// readLog splits a provenance log into its lines. An empty line is skipped.
func readLog(data []byte) []logLine {
	var out []logLine
	for i, raw := range strings.Split(string(data), "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		l := logLine{n: i + 1, raw: raw}
		l.err = json.Unmarshal([]byte(raw), &l.line)
		out = append(out, l)
	}
	return out
}

// p01 checks that each content change in a merge has a provenance line (rule P01). The merge of a pull request is the change from the merge base of Base to the working tree. With no Base, it is the last commit, as on main each merge is one squashed commit. For each node file that the merge changed, the rule compares the fields of the file before and after (prov.Changes). Each changed field needs a new line in the log of the node on that field, on a part of it or on the whole node. A log only grows: the lines that it had before the merge must still be its first lines. Each new line must be valid.
func p01(a *auditor) outcome {
	var o outcome
	if !isRepo(a.root) {
		o.reason = "the tree is not a git repository"
		return o
	}
	from, to, err := a.mergeRange()
	if err != nil {
		o.reason = err.Error()
		return o
	}
	paths, err := a.changedNodes(from, to)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	for _, p := range paths {
		id := trimExt(p)
		before, after := a.nodeAt(from, p), a.nodeAt(to, p)
		changes := prov.Changes(before, after, prov.Line{})
		if len(changes) == 0 {
			continue
		}
		o.checked++
		logPath := prov.Path(id)
		old, now := readLog(a.blob(from, logPath)), readLog(a.blob(to, logPath))
		if len(old) > len(now) || !slices.EqualFunc(old, now[:len(old)], func(x, y logLine) bool { return x.raw == y.raw }) {
			o.add(logPath, 1, 1, "the merge rewrote the provenance log of %s. A log only grows: add a new line to correct a line", id)
			continue
		}
		added := now[len(old):]
		for i := range added {
			l := &added[i]
			if l.err == nil {
				l.err = l.line.Validate()
			}
			if l.err != nil {
				o.add(logPath, l.n, 1, "the provenance line is not valid: %v", l.err)
			}
		}
		at := after
		if at == nil {
			at = before
		}
		for _, c := range changes {
			if !slices.ContainsFunc(added, func(l logLine) bool { return l.err == nil && prov.Covers(l.line.Field, c.Field) }) {
				line, col := fieldPos(at, c.Field)
				if after == nil {
					line, col = 1, 1
				}
				o.add(p, line, col, "the merge changed the field %s of %s, and %s has no new line on it. Run m2 prov record", c.Field, id, logPath)
			}
		}
	}
	return o
}

// mergeRange returns the revisions before and after the merge. An empty to is the working tree, and an empty from is the empty tree.
func (a *auditor) mergeRange() (from, to string, err error) {
	if a.pr {
		mb, err := git(a.root, "merge-base", a.base, "HEAD")
		return strings.TrimSpace(mb), "", err
	}
	if parent, err := git(a.root, "rev-parse", "--verify", "-q", "HEAD^"); err == nil {
		return strings.TrimSpace(parent), "HEAD", nil
	}
	return "", "HEAD", nil
}

// changedNodes returns the node files in scope that differ between the revisions from and to.
func (a *auditor) changedNodes(from, to string) ([]string, error) {
	all, err := a.changedFiles(from, to, "nodes")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range all {
		if strings.HasSuffix(p, ".md") && strings.Count(p, "/") == 2 && a.inScope(p) {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// changedFiles returns the files below the directories that differ between the revisions from and to, sorted. No directory means the whole tree.
func (a *auditor) changedFiles(from, to string, dirs ...string) ([]string, error) {
	spec := append([]string{"--"}, dirs...)
	var out string
	var err error
	switch {
	case from == "":
		out, err = git(a.root, append([]string{"ls-tree", "-r", "-z", "--name-only", to}, spec...)...)
	case to == "":
		out, err = git(a.root, append([]string{"diff", "--name-only", "--no-renames", "-z", from}, spec...)...)
		if err == nil {
			var untracked string
			untracked, err = git(a.root, append([]string{"ls-files", "--others", "--exclude-standard", "-z"}, spec...)...)
			out += untracked
		}
	default:
		out, err = git(a.root, append([]string{"diff", "--name-only", "--no-renames", "-z", from, to}, spec...)...)
	}
	if err != nil {
		return nil, err
	}
	var paths []string
	for p := range strings.SplitSeq(out, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	return slices.Compact(paths), nil
}

// blob returns the file at the path in the revision rev, or in the working tree when rev is empty. A file that is not there gives no data.
func (a *auditor) blob(rev, path string) []byte {
	if rev == "" {
		data, _ := a.readFile(path)
		return data
	}
	out, err := git(a.root, "show", rev+":"+path)
	if err != nil {
		return nil
	}
	return []byte(out)
}

// nodeAt returns the node file at the path in the revision rev, or nil when it is not there or does not parse.
func (a *auditor) nodeAt(rev, path string) *node.Node {
	data := a.blob(rev, path)
	if data == nil {
		return nil
	}
	n, err := node.Parse(path, data)
	if err != nil {
		return nil
	}
	return n
}

// fieldPos returns the position of a field in the node file: the key of the front matter, or the heading of the section or of the explanation.
func fieldPos(n *node.Node, field string) (line, col int) {
	rest, ok := strings.CutPrefix(field, "body.")
	if !ok {
		key, _, _ := strings.Cut(field, ".")
		key, _, _ = strings.Cut(key, "[")
		return keyPos(n, key)
	}
	section, level, _ := strings.Cut(rest, ".")
	for _, s := range check.Sections(n) {
		if strings.ReplaceAll(strings.ToLower(s.Name), " ", "_") != section {
			continue
		}
		for _, h := range s.Headings {
			if level != "" && h.Level == 3 && strings.HasPrefix(h.Name, level) {
				return h.Line, 1
			}
		}
		return s.Line, 1
	}
	return n.BodyLine, 1
}

// agentLines returns the lines of agents in the provenance logs of the nodes in scope, with the path of each log.
func (a *auditor) agentLines() (map[string][]logLine, error) {
	out := map[string][]logLine{}
	for _, n := range a.scopeNodes() {
		p := prov.Path(n.ID)
		data, err := a.readFile(p)
		if err != nil {
			return nil, err
		}
		for _, l := range readLog(data) {
			if l.err == nil && l.line.Agent != "" {
				out[p] = append(out[p], l)
			}
		}
	}
	return out, nil
}

// runs reads the run summaries in build/runs/. It gives the reason to skip when the directory does not exist.
func (a *auditor) runs() (map[string]*prov.Run, string) {
	runs, err := prov.ReadRuns(filepath.Join(a.root, runsDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, filepath.ToSlash(runsDir) + " does not exist, thus the audit cannot see the run records of the artefact store"
	}
	if err != nil {
		return nil, err.Error()
	}
	return runs, ""
}

// p02 checks that each line of an agent names a run that is in the artefact store, and that the run is of that agent and that model (rule P02).
func p02(a *auditor) outcome {
	var o outcome
	logs, err := a.agentLines()
	if err != nil {
		o.reason = err.Error()
		return o
	}
	if len(logs) == 0 {
		return o
	}
	runs, reason := a.runs()
	if reason != "" {
		o.reason = reason
		return o
	}
	for _, p := range slices.Sorted(maps.Keys(logs)) {
		for _, l := range logs[p] {
			o.checked++
			r := runs[l.line.Run]
			switch {
			case l.line.Run == "":
				o.add(p, l.n, 1, "the line of %s on %s names no run", l.line.Agent, l.line.Field)
			case r == nil:
				o.add(p, l.n, 1, "the line of %s on %s names the run %s, and the artefact store has no record of it", l.line.Agent, l.line.Field, l.line.Run)
			case r.Agent != l.line.Agent:
				o.add(p, l.n, 1, "the line names the agent %s, and the record of the run %s names %s", l.line.Agent, l.line.Run, r.Agent)
			case l.line.Model != "" && !slices.Contains(r.Models, l.line.Model):
				o.add(p, l.n, 1, "the line names the model %s, and the run %s used only %s", l.line.Model, l.line.Run, strings.Join(r.Models, ", "))
			}
		}
	}
	return o
}

// p03 checks that each agent run that a line in scope names acted on behalf of a person, a project or a schedule (rule P03). An agent is not an author: it is a tool that acts for someone, and that someone is responsible.
func p03(a *auditor) outcome {
	var o outcome
	logs, err := a.agentLines()
	if err != nil {
		o.reason = err.Error()
		return o
	}
	ids := map[string]bool{}
	for _, ls := range logs {
		for _, l := range ls {
			if l.line.Run != "" {
				ids[l.line.Run] = true
			}
		}
	}
	if len(ids) == 0 {
		return o
	}
	runs, reason := a.runs()
	if reason != "" {
		o.reason = reason
		return o
	}
	for _, id := range slices.Sorted(maps.Keys(ids)) {
		r := runs[id]
		if r == nil {
			continue // P02 reports it
		}
		o.checked++
		p := filepath.ToSlash(filepath.Join(runsDir, id+".json"))
		switch {
		case r.OnBehalfOf == "":
			o.add(p, jsonKeyLine(a, p, "on_behalf_of"), 1, "the run %s of %s names no person, project or schedule on whose behalf it acted", id, r.Agent)
		case !prov.IsResponsible(r.OnBehalfOf):
			o.add(p, jsonKeyLine(a, p, "on_behalf_of"), 1, "the run %s acted on behalf of %q. Name a person (orcid: or github:), a project (project:) or a schedule (schedule:)", id, r.OnBehalfOf)
		}
	}
	return o
}

// jsonKeyLine returns the line of the key in a JSON file of the tree, or 1.
func jsonKeyLine(a *auditor, path, key string) int {
	data, _ := a.readFile(path)
	if i := bytes.Index(data, []byte(`"`+key+`"`)); i >= 0 {
		return bytes.Count(data[:i], []byte("\n")) + 1
	}
	return 1
}

// explanationLevel matches the heading of an explanation that says a person wrote it, and gives the level.
var explanationLevel = regexp.MustCompile(`^(L[1-4]) human$`)

// p04 finds the explanations that are marked human, and whose text is from an agent that no person confirmed (rule P04). The workspace shows the marker of the heading, thus such an explanation shows AI text as the text of a person. Explanations are agent until a person confirms them (spec 12.6). A node with no provenance log is not checked here: P01 reports a change with no line.
func p04(a *auditor) outcome {
	var o outcome
	for _, n := range a.scopeNodes() {
		var marked []check.Heading
		for _, s := range check.Sections(n) {
			if s.Name != "Explanations" {
				continue
			}
			for _, h := range s.Headings {
				if h.Level == 3 && explanationLevel.MatchString(h.Name) {
					marked = append(marked, h)
				}
			}
		}
		if len(marked) == 0 || !tag.IsTag(n.ID) && !tag.IsProvisional(n.ID) {
			continue
		}
		log, err := prov.Read(a.root, n.ID)
		if err != nil {
			o.reason = err.Error()
			return o
		}
		if log == nil {
			continue
		}
		for _, h := range marked {
			o.checked++
			level := explanationLevel.FindStringSubmatch(h.Name)[1]
			l, ok := prov.Author(log, "body.explanations."+level)
			if ok && l.Agent != "" && l.Human == "" {
				o.add(n.Path, h.Line, 1, "the explanation %s is marked human, and its text is from %s in the run %s, which no person confirmed. Mark it \"### %s agent\", or confirm it with m2 prov confirm", level, l.Agent, l.Run, level)
			}
		}
	}
	return o
}
