package propose

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Host is where the pull requests go.
type Host interface {
	// User returns the login of the person who has the token. It returns an error that wraps ErrUnauthorized when the host does not accept the token.
	User(ctx context.Context, token string) (string, error)

	// Pulls returns the pull requests against the repository that changed since the time, in any order.
	Pulls(ctx context.Context, token string, since time.Time) ([]Pull, error)

	// Open pushes the files of pr to a new branch and opens a pull request from it, with the token.
	Open(ctx context.Context, token string, pr PullRequest) (Pull, error)
}

// Pull is a pull request of the host.
type Pull struct {
	Number int       `json:"number"`
	URL    string    `json:"url"`
	Branch string    `json:"branch"`          // the head branch
	Opened time.Time `json:"opened"`          // the time when it was opened
	Closed time.Time `json:"closed,omitzero"` // the time when it was closed or merged, or zero when it is open
}

// PullRequest is a pull request to open.
type PullRequest struct {
	User    string            // the login of the person who has the token
	Base    string            // the branch to merge into
	Commit  string            // the commit of the base that the change starts from
	Branch  string            // the new head branch
	Title   string            // the title of the pull request
	Body    string            // the body of the pull request
	Message string            // the message of the commit
	Files   map[string][]byte // the files that the commit adds or replaces, from the tree root
}

// GitHub opens the pull requests on GitHub with the REST API. A person who is not the owner of the repository gets a fork, and the pull request comes from a branch of the fork. The commit gets the files through the git database API, thus GitHub needs no git push.
type GitHub struct {
	Owner string // the owner of the repository, for example tamnd
	Repo  string // the name of the repository, for example math2

	API       string       // the URL of the REST API. Empty means https://api.github.com.
	Client    *http.Client // nil means http.DefaultClient
	UserAgent string       // the User-Agent of each request. Empty means m2 (+https://github.com/tamnd/math2-agent).

	// Wait is the time between two tries of a request on a new fork, which GitHub makes in the background. Zero means 2 seconds. Open tries 15 times.
	Wait time.Duration
}

// User returns the login of the token.
func (g *GitHub) User(ctx context.Context, token string) (string, error) {
	var u struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	}
	if err := g.do(ctx, token, http.MethodGet, "/user", nil, &u); err != nil {
		return "", err
	}
	if u.Login == "" || u.Type == "Bot" {
		return "", fmt.Errorf("%w. The token is of %q, which is not a person", ErrUnauthorized, u.Login)
	}
	return u.Login, nil
}

// maxPages is the number of pages of 100 pull requests that Pulls reads.
const maxPages = 10

// Pulls returns the pull requests of the repository that changed since the time. It reads the newest first, and it stops at the first one that changed before the time.
func (g *GitHub) Pulls(ctx context.Context, token string, since time.Time) ([]Pull, error) {
	var out []Pull
	for page := 1; page <= maxPages; page++ {
		var prs []struct {
			Number  int       `json:"number"`
			URL     string    `json:"html_url"`
			Created time.Time `json:"created_at"`
			Updated time.Time `json:"updated_at"`
			Closed  time.Time `json:"closed_at"`
			Head    struct {
				Ref string `json:"ref"`
			} `json:"head"`
		}
		path := g.repo() + "/pulls?state=all&sort=updated&direction=desc&per_page=100&page=" + strconv.Itoa(page)
		if err := g.do(ctx, token, http.MethodGet, path, nil, &prs); err != nil {
			return nil, err
		}
		for _, pr := range prs {
			if pr.Updated.Before(since) {
				return out, nil
			}
			out = append(out, Pull{Number: pr.Number, URL: pr.URL, Branch: pr.Head.Ref, Opened: pr.Created, Closed: pr.Closed})
		}
		if len(prs) < 100 {
			break
		}
	}
	return out, nil
}

// Open makes the commit of pr on a new branch and opens the pull request. The branch is in the repository when the person is its owner, and in the fork of the person otherwise.
func (g *GitHub) Open(ctx context.Context, token string, pr PullRequest) (Pull, error) {
	head := g.repo()
	if !strings.EqualFold(pr.User, g.Owner) {
		var fork struct {
			FullName string `json:"full_name"`
		}
		if err := g.do(ctx, token, http.MethodPost, g.repo()+"/forks", map[string]any{"default_branch_only": true}, &fork); err != nil {
			return Pull{}, err
		}
		if fork.FullName == "" {
			return Pull{}, fmt.Errorf("propose: GitHub made a fork with no name")
		}
		head = "/repos/" + fork.FullName
		// A fork that is behind gets the new commits. A fork that cannot merge them keeps its branch, and the new branch starts from the commit of the base anyway.
		_ = g.retry(ctx, token, http.MethodPost, head+"/merge-upstream", map[string]any{"branch": pr.Base}, nil, http.StatusNotFound)
	}

	var tree []map[string]any
	for path, data := range pr.Files {
		if !utf8.Valid(data) {
			return Pull{}, fmt.Errorf("propose: %s is not UTF-8 text", path)
		}
		tree = append(tree, map[string]any{"path": path, "mode": "100644", "type": "blob", "content": string(data)})
	}
	var sha struct {
		SHA string `json:"sha"`
	}
	if err := g.retry(ctx, token, http.MethodPost, head+"/git/trees", map[string]any{"base_tree": pr.Commit, "tree": tree}, &sha, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity); err != nil {
		return Pull{}, err
	}
	if err := g.do(ctx, token, http.MethodPost, head+"/git/commits", map[string]any{"message": pr.Message, "tree": sha.SHA, "parents": []string{pr.Commit}}, &sha); err != nil {
		return Pull{}, err
	}
	if err := g.do(ctx, token, http.MethodPost, head+"/git/refs", map[string]any{"ref": "refs/heads/" + pr.Branch, "sha": sha.SHA}, nil); err != nil {
		return Pull{}, err
	}
	owner := g.Owner
	if head != g.repo() {
		owner = strings.Split(strings.TrimPrefix(head, "/repos/"), "/")[0]
	}
	var made struct {
		Number  int       `json:"number"`
		URL     string    `json:"html_url"`
		Created time.Time `json:"created_at"`
	}
	in := map[string]any{"title": pr.Title, "body": pr.Body, "head": owner + ":" + pr.Branch, "base": pr.Base, "maintainer_can_modify": true}
	if err := g.do(ctx, token, http.MethodPost, g.repo()+"/pulls", in, &made); err != nil {
		return Pull{}, err
	}
	return Pull{Number: made.Number, URL: made.URL, Branch: pr.Branch, Opened: made.Created}, nil
}

func (g *GitHub) repo() string {
	return "/repos/" + url.PathEscape(g.Owner) + "/" + url.PathEscape(g.Repo)
}

// retry sends the request until GitHub has the fork. A new fork gives the status codes for a short time.
func (g *GitHub) retry(ctx context.Context, token, method, path string, in, out any, codes ...int) error {
	var err error
	for range 15 {
		err = g.do(ctx, token, method, path, in, out)
		var he *httpError
		if err == nil || !errors.As(err, &he) || !slices.Contains(codes, he.code) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cmp.Or(g.Wait, 2*time.Second)):
		}
	}
	return err
}

// httpError is an answer of GitHub with a status that is not 2xx.
type httpError struct {
	code    int
	method  string
	path    string
	message string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("propose: GitHub answered %s %s with %d: %s", e.method, e.path, e.code, e.message)
}

// maxAnswer is the largest answer of GitHub that do reads.
const maxAnswer = 8 << 20

// do sends one request with the token and decodes the JSON answer into out, when out is not nil.
func (g *GitHub) do(ctx context.Context, token, method, path string, in, out any) error {
	var rd io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(cmp.Or(g.API, "https://api.github.com"), "/")+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", cmp.Or(g.UserAgent, "m2 (+https://github.com/tamnd/math2-agent)"))
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := g.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req) // #nosec G704 -- the URL is the GitHub API that the server was given.
	if err != nil {
		return fmt.Errorf("propose: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswer))
	if err != nil {
		return fmt.Errorf("propose: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("%w. GitHub does not accept the token", ErrUnauthorized)
	}
	if resp.StatusCode/100 != 2 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &e)
		return &httpError{code: resp.StatusCode, method: method, path: strings.SplitN(path, "?", 2)[0], message: cmp.Or(e.Message, http.StatusText(resp.StatusCode))}
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("propose: the answer of GitHub to %s %s is not the JSON that m2 knows: %w", method, path, err)
	}
	return nil
}
