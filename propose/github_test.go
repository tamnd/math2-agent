package propose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// gitHub is a fake of the REST API of GitHub. It records each request.
type gitHub struct {
	t        *testing.T
	mu       sync.Mutex
	calls    []string
	bodies   map[string]map[string]any
	notReady int // the requests on the git trees of a fork that get 404 before the fork is ready
	pulls    []map[string]any
}

func (g *gitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer t-ada" && r.Header.Get("Authorization") != "Bearer t-owner" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
		return
	}
	if ua := r.Header.Get("User-Agent"); !strings.HasPrefix(ua, "m2") {
		g.t.Errorf("the User-Agent is %q", ua)
	}
	call := r.Method + " " + r.URL.Path
	g.calls = append(g.calls, call)
	var in map[string]any
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			g.t.Errorf("%s: %v", call, err)
		}
		g.bodies[call] = in
	}
	reply := func(code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	switch call {
	case "GET /user":
		login := "ada"
		if r.Header.Get("Authorization") == "Bearer t-owner" {
			login = "tamnd"
		}
		reply(200, map[string]any{"login": login, "type": "User"})
	case "GET /repos/tamnd/math2/pulls":
		page := r.URL.Query().Get("page")
		var out []map[string]any
		for i, p := range g.pulls {
			if fmt.Sprint(i/100+1) == page {
				out = append(out, p)
			}
		}
		reply(200, out)
	case "POST /repos/tamnd/math2/forks":
		reply(202, map[string]any{"full_name": "ada/math2-1"})
	case "POST /repos/ada/math2-1/merge-upstream":
		reply(409, map[string]any{"message": "conflict"})
	case "POST /repos/ada/math2-1/git/trees", "POST /repos/tamnd/math2/git/trees":
		if g.notReady > 0 {
			g.notReady--
			reply(404, map[string]any{"message": "Not Found"})
			return
		}
		reply(201, map[string]any{"sha": "tree1"})
	case "POST /repos/ada/math2-1/git/commits", "POST /repos/tamnd/math2/git/commits":
		reply(201, map[string]any{"sha": "commit1"})
	case "POST /repos/ada/math2-1/git/refs", "POST /repos/tamnd/math2/git/refs":
		reply(201, map[string]any{"ref": in["ref"]})
	case "POST /repos/tamnd/math2/pulls":
		reply(201, map[string]any{"number": 42, "html_url": "https://github.com/tamnd/math2/pull/42", "created_at": "2026-11-02T10:00:00Z"})
	default:
		reply(404, map[string]any{"message": "Not Found"})
	}
}

func newGitHub(t *testing.T) (*GitHub, *gitHub) {
	f := &gitHub{t: t, bodies: map[string]map[string]any{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return &GitHub{Owner: "tamnd", Repo: "math2", API: srv.URL, Client: srv.Client(), Wait: time.Millisecond}, f
}

var request = PullRequest{User: "ada", Base: "main", Commit: "base1", Branch: "m2-api/propose_node/0123abcd-00", Title: "Add the concept Monoid", Body: "body", Message: "Add the concept Monoid", Files: map[string][]byte{"nodes/~/~0123abcd.md": []byte("---\n---\n")}}

func TestGitHubOpenFork(t *testing.T) {
	g, f := newGitHub(t)
	f.notReady = 2
	pr, err := g.Open(context.Background(), "t-ada", request)
	if err != nil {
		t.Fatal(err)
	}
	if pr.Number != 42 || pr.Branch != request.Branch || !pr.Opened.Equal(now) {
		t.Errorf("the pull request is %+v", pr)
	}
	want := []string{"POST /repos/tamnd/math2/forks", "POST /repos/ada/math2-1/merge-upstream", "POST /repos/ada/math2-1/git/trees", "POST /repos/ada/math2-1/git/trees", "POST /repos/ada/math2-1/git/trees", "POST /repos/ada/math2-1/git/commits", "POST /repos/ada/math2-1/git/refs", "POST /repos/tamnd/math2/pulls"}
	if strings.Join(f.calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("the calls are\n%s", strings.Join(f.calls, "\n"))
	}
	tree := f.bodies["POST /repos/ada/math2-1/git/trees"]
	if tree["base_tree"] != "base1" || len(tree["tree"].([]any)) != 1 {
		t.Errorf("the tree is %v", tree)
	}
	if c := f.bodies["POST /repos/ada/math2-1/git/commits"]; c["tree"] != "tree1" || c["parents"].([]any)[0] != "base1" {
		t.Errorf("the commit is %v", c)
	}
	if r := f.bodies["POST /repos/ada/math2-1/git/refs"]; r["ref"] != "refs/heads/"+request.Branch || r["sha"] != "commit1" {
		t.Errorf("the ref is %v", r)
	}
	if p := f.bodies["POST /repos/tamnd/math2/pulls"]; p["head"] != "ada:"+request.Branch || p["base"] != "main" || p["title"] != request.Title {
		t.Errorf("the pull request is %v", p)
	}
}

func TestGitHubOpenOwner(t *testing.T) {
	g, f := newGitHub(t)
	r := request
	r.User = "tamnd"
	if _, err := g.Open(context.Background(), "t-owner", r); err != nil {
		t.Fatal(err)
	}
	if f.calls[0] != "POST /repos/tamnd/math2/git/trees" || f.bodies["POST /repos/tamnd/math2/pulls"]["head"] != "tamnd:"+r.Branch {
		t.Errorf("the calls are %v", f.calls)
	}
}

func TestGitHubUser(t *testing.T) {
	g, _ := newGitHub(t)
	login, err := g.User(context.Background(), "t-ada")
	if err != nil || login != "ada" {
		t.Errorf("the login is %q, %v", login, err)
	}
	if _, err := g.User(context.Background(), "t-bad"); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("a bad token: %v", err)
	}
}

func TestGitHubPulls(t *testing.T) {
	g, f := newGitHub(t)
	start := now.Add(-time.Hour)
	for i := range 150 {
		f.pulls = append(f.pulls, map[string]any{
			"number": 1000 - i, "html_url": "u", "head": map[string]any{"ref": "m2-api/propose_edit/x"},
			"created_at": start.Add(-time.Duration(i) * time.Hour), "updated_at": start.Add(-time.Duration(i) * time.Hour), "closed_at": nil,
		})
	}
	pulls, err := g.Pulls(context.Background(), "t-ada", start.Add(-119*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(pulls) != 120 || pulls[0].Number != 1000 || !pulls[0].Closed.IsZero() || pulls[119].Branch != "m2-api/propose_edit/x" {
		t.Errorf("Pulls gave %d pull requests, from %+v", len(pulls), pulls[0])
	}
	if got := strings.Count(strings.Join(f.calls, "\n"), "GET /repos/tamnd/math2/pulls"); got != 2 {
		t.Errorf("Pulls read %d pages", got)
	}
}

func TestGitHubErrors(t *testing.T) {
	g, _ := newGitHub(t)
	g.Repo = "other"
	_, err := g.Open(context.Background(), "t-owner", PullRequest{User: "tamnd", Files: map[string][]byte{"a": {0xff}}})
	if err == nil || !strings.Contains(err.Error(), "not UTF-8") {
		t.Errorf("a file that is not text: %v", err)
	}
	if _, err := g.Pulls(context.Background(), "t-ada", now); err == nil || !strings.Contains(err.Error(), "404: Not Found") {
		t.Errorf("a repository that does not exist: %v", err)
	}
}
