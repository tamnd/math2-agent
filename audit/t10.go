package audit

import (
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/tag"
)

const registersReadme = "registers/README.md"

// cutoverRE finds the line of registers/README.md that names the cut-over commit.
var cutoverRE = regexp.MustCompile("(?m)^Cut-over commit: `([0-9a-f]{7,40})`")

// t10 checks that no tag of six characters is minted after the cut-over commit (rule T10, corpus spec K3.8). The cut-over commit is the last commit at which the mint workflow made tags of six characters, and registers/README.md names it on a line "Cut-over commit: `SHA`". The commit that added a record to registers/tags.reg comes from git blame. For a record of a tag of six characters, that commit must be the cut-over commit or an ancestor of it. A record that is not committed yet is after the cut-over.
func t10(a *auditor) outcome {
	var o outcome
	if !isRepo(a.root) {
		o.reason = "the tree is not a git repository"
		return o
	}
	readme, err := a.readFile(registersReadme)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	m := cutoverRE.FindSubmatchIndex(readme)
	if m == nil {
		o.reason = registersReadme + " names no cut-over commit"
		return o
	}
	cut := string(readme[m[2]:m[3]])
	if out, _ := git(a.root, "rev-parse", "--is-shallow-repository"); strings.TrimSpace(out) == "true" {
		o.reason = "a shallow clone does not have the history. Fetch the full history"
		return o
	}
	if _, err := git(a.root, "rev-parse", "--verify", "-q", cut+"^{commit}"); err != nil {
		o.add(registersReadme, strings.Count(string(readme[:m[0]]), "\n")+1, 1, "the cut-over commit %s is not in the history", cut)
		return o
	}
	recs, err := readRegister(a, tagsReg)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	var blame map[int]string
	for _, r := range recs {
		t := r.fields[0]
		if !tag.IsLegacy(t) {
			continue
		}
		if blame == nil {
			if blame, err = blameLines(a.root, tagsReg); err != nil {
				o.reason = err.Error()
				return o
			}
		}
		o.checked++
		commit := blame[r.line]
		if commit == "" || strings.Trim(commit, "0") == "" {
			o.add(tagsReg, r.line, 1, "the tag %s has six characters, and its record is not committed yet. A new tag has ten characters", t)
			continue
		}
		if ok, err := isAncestor(a.root, commit, cut); err != nil {
			o.reason = err.Error()
			return o
		} else if !ok {
			o.add(tagsReg, r.line, 1, "the tag %s has six characters, and the commit %s added it after the cut-over commit %s. A new tag has ten characters", t, commit[:12], cut)
		}
	}
	return o
}

// blameLines returns the commit that added each line of the file at rel in the working tree, by line number. A line that is not committed has the commit of zeros.
func blameLines(root, rel string) (map[int]string, error) {
	if _, err := git(root, "ls-files", "--error-unmatch", "--", rel); err != nil {
		return map[int]string{}, nil
	}
	out, err := git(root, "blame", "--porcelain", "--", rel)
	if err != nil {
		return nil, err
	}
	lines := map[int]string{}
	for l := range strings.Lines(out) {
		f := strings.Fields(l)
		if len(f) < 3 || len(f[0]) != 40 || strings.HasPrefix(l, "\t") {
			continue
		}
		if n, err := strconv.Atoi(f[2]); err == nil {
			lines[n] = f[0]
		}
	}
	return lines, nil
}

// isAncestor reports whether the commit a is the commit b or an ancestor of it.
func isAncestor(root, a, b string) (bool, error) {
	_, err := git(root, "merge-base", "--is-ancestor", a, b)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	return err == nil, err
}
