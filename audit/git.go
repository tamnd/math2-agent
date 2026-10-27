package audit

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// git runs git in the tree at root and returns the standard output.
func git(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...) // #nosec G204 -- the audit gives the arguments, and a revision that starts with - is refused before this call.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// isRepo reports whether root is in a git work tree.
func isRepo(root string) bool {
	out, err := git(root, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// changed returns the shards and the files of the tree at root that changed since the merge base of base and HEAD. The changes in the working tree and the untracked files count.
func changed(root, base string) (map[string]bool, error) {
	if strings.HasPrefix(base, "-") {
		return nil, fmt.Errorf("audit: %q is not a revision", base)
	}
	mb, err := git(root, "merge-base", base, "HEAD")
	if err != nil {
		return nil, err
	}
	diff, err := git(root, "diff", "--name-only", "--relative", "--no-renames", "-z", strings.TrimSpace(mb), "--")
	if err != nil {
		return nil, err
	}
	untracked, err := git(root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for p := range strings.SplitSeq(diff+untracked, "\x00") {
		if p != "" {
			out[shard(p)] = true
		}
	}
	return out, nil
}

// headDate returns the date of the HEAD commit. A rule that measures an age measures it to this date, thus the result for a commit does not change with the day of the audit.
func headDate(root string) (time.Time, error) {
	head, err := git(root, "log", "-1", "--format=%cs")
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.DateOnly, strings.TrimSpace(head))
}

// catFile reads the files of revisions through one git cat-file --batch process. A pull request with many changed files then starts one process and not one for each file.
type catFile struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	out *bufio.Reader
}

func startCatFile(root string) (*catFile, error) {
	cmd := exec.Command("git", "-C", root, "cat-file", "--batch") // #nosec G204 -- the arguments are constant.
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &catFile{cmd: cmd, in: in, out: bufio.NewReader(out)}, nil
}

// read returns the blob of the object name rev:path. ok is false when the object is not there or is not a blob. An error means that the process failed, and the caller must not use it again.
func (c *catFile) read(obj string) (data []byte, ok bool, err error) {
	if _, err := io.WriteString(c.in, obj+"\n"); err != nil {
		return nil, false, err
	}
	head, err := c.out.ReadString('\n')
	if err != nil {
		return nil, false, err
	}
	f := strings.Fields(head)
	if len(f) != 3 {
		return nil, false, nil // missing or ambiguous
	}
	size, err := strconv.Atoi(f[2])
	if err != nil {
		return nil, false, fmt.Errorf("git cat-file: %q: %w", strings.TrimSpace(head), err)
	}
	data = make([]byte, size+1) // the content and a newline
	if _, err := io.ReadFull(c.out, data); err != nil {
		return nil, false, err
	}
	return data[:size], f[1] == "blob", nil
}

func (c *catFile) close() {
	_ = c.in.Close()
	_ = c.cmd.Wait()
}
