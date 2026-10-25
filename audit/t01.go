package audit

import (
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/tag"
)

// t01 checks that no tag disappears (rule T01). Each record that was ever in registers/tags.reg must be in the file of the working tree, with no change. Each tagged node file that was ever in nodes/ must be in the working tree at the same path. A retired node keeps its file.
func t01(a *auditor) outcome {
	var o outcome
	if !isRepo(a.root) {
		o.reason = "the tree is not a git repository"
		return o
	}
	if out, err := git(a.root, "rev-parse", "--is-shallow-repository"); err != nil || strings.TrimSpace(out) == "true" {
		o.reason = "the git clone is shallow, and T01 needs the full history"
		return o
	}
	if _, err := git(a.root, "rev-parse", "--verify", "-q", "HEAD"); err != nil {
		o.reason = "the repository has no commits"
		return o
	}

	const reg = "registers/tags.reg"
	log, err := git(a.root, "log", "--relative", "-p", "-U0", "--no-renames", "--format=", "--", reg)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	data, err := a.readFile(reg)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	now := map[string]bool{}
	for line := range strings.Lines(string(data)) {
		now[strings.TrimRight(line, "\n")] = true
	}
	seen := map[string]bool{}
	for line := range strings.Lines(log) {
		line = strings.TrimRight(line, "\n")
		rec, ok := strings.CutPrefix(line, "+")
		if !ok || strings.HasPrefix(line, "+++") {
			continue
		}
		fields := strings.Fields(rec)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") || seen[rec] {
			continue
		}
		seen[rec] = true
		o.checked++
		if !now[rec] {
			o.add(reg, 0, 0, "the record %q was in the register and is not now. A record never changes and is never removed", rec)
		}
	}

	files, err := git(a.root, "log", "--relative", "--name-only", "--no-renames", "--diff-filter=A", "--format=", "--", "nodes/")
	if err != nil {
		o.reason = err.Error()
		return o
	}
	have := map[string]bool{}
	for _, f := range a.all {
		have[f.Path] = true
	}
	var paths []string
	for p := range strings.Lines(files) {
		p = strings.TrimSpace(p)
		id, ok := strings.CutSuffix(p[strings.LastIndexByte(p, '/')+1:], ".md")
		if ok && tag.IsTag(id) && shard(p) == "nodes/"+tag.Shard(id) && !slices.Contains(paths, p) {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	for _, p := range paths {
		o.checked++
		if !have[p] {
			o.add(p, 0, 0, "the node file was in the history and is not in the tree. To remove a node, retire it and keep the file")
		}
	}
	return o
}
