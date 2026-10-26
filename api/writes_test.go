package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/api"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/propose"
	"github.com/tamnd/math2-agent/tag"
)

// host is a propose.Host in memory, with one person.
type host struct{ opened []propose.PullRequest }

func (h *host) User(_ context.Context, token string) (string, error) {
	if token != "t-ada" {
		return "", fmt.Errorf("%w. GitHub does not accept the token", propose.ErrUnauthorized)
	}
	return "ada", nil
}

func (h *host) Pulls(context.Context, string, time.Time) ([]propose.Pull, error) { return nil, nil }

func (h *host) Open(_ context.Context, _ string, pr propose.PullRequest) (propose.Pull, error) {
	h.opened = append(h.opened, pr)
	return propose.Pull{Number: 7, URL: "https://github.com/tamnd/math2/pull/7", Branch: pr.Branch, Opened: time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)}, nil
}

// writer returns a handler with the write methods on a git copy of the test tree.
func writer(t *testing.T) (http.Handler, *host) {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(tree)); err != nil {
		t.Fatal(err)
	}
	registers(t, root)
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-q", "-m", "the tree"}} {
		cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", args[0], err, out)
		}
	}
	h := &host{}
	return open(t).Handler(api.Options{Rate: -1, Propose: &propose.Proposer{Root: root, Host: h}}), h
}

// registers writes the registers of the nodes of root, which the test tree does not have and the audit of a write needs.
func registers(t *testing.T, root string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, "nodes", "*", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	tags, retired := []string{"# tag commit kind"}, []string{"# tag date reason successors"}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		n, err := node.Parse(p, data)
		if err != nil {
			t.Fatal(err)
		}
		if !tag.IsTag(n.ID) {
			continue
		}
		tags = append(tags, n.ID+" "+strings.Repeat("a", 40)+" "+n.Kind())
		if _, ok := n.Meta["retired"]; ok {
			retired = append(retired, n.ID+" 2026-10-20 duplicate")
		}
	}
	slices.Sort(tags[1:])
	if err := os.MkdirAll(filepath.Join(root, "registers"), 0o750); err != nil {
		t.Fatal(err)
	}
	for name, lines := range map[string][]string{"tags.reg": tags, "retired.reg": retired, "aliases.reg": {"# alias tag from until"}} {
		if err := os.WriteFile(filepath.Join(root, "registers", name), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func post(t *testing.T, h http.Handler, path, token, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s: %v: %s", path, err, w.Body)
	}
	return w.Code, out
}

func TestWrite(t *testing.T) {
	h, hs := writer(t)
	code, out := post(t, h, "/v1/propose_alignment", "t-ada", `{"ref": "Q529ES", "edge": "close_to", "system": "nlab", "id": "Lagrange theorem", "note": "the nLab page states it for a group and a subgroup of finite index", "message": "Found while I read the nLab page."}`)
	if code != http.StatusOK {
		t.Fatalf("the status is %d: %v", code, out)
	}
	res := out["result"].(map[string]any)
	if res["number"] != 7.0 || res["user"] != "github:ada" || out["method"] != "propose_alignment" || len(out["about"].([]any)) != 1 {
		t.Errorf("the answer is %v", out)
	}
	if len(hs.opened) != 1 || !strings.Contains(string(hs.opened[0].Files["nodes/Q5/Q529ES.md"]), "close_to:") || !strings.Contains(hs.opened[0].Body, "> Found while I read") {
		t.Errorf("the pull request is %+v", hs.opened)
	}
}

func TestWriteErrors(t *testing.T) {
	h, hs := writer(t)
	tests := []struct {
		path, token, body string
		code              int
		want              string
	}{
		{"/v1/propose_alignment", "", `{}`, http.StatusUnauthorized, "Authorization: Bearer"},
		{"/v1/propose_alignment", "t-bad", `{"ref": "Q529ES", "edge": "same_as", "system": "wikidata", "id": "Q1"}`, http.StatusUnauthorized, "does not accept the token"},
		{"/v1/propose_alignment", "t-ada", `{"ref": "Q529ES"}`, http.StatusBadRequest, "edge"},
		{"/v1/propose_alignment", "t-ada", `{"ref": "Q529ES", "edge": "uses", "id": "x"}`, http.StatusBadRequest, `"uses" is not`},
		{"/v1/propose_binding", "t-ada", `{"ref": "ZZZZZZ", "decl": "Nat.add", "match": "exact"}`, http.StatusNotFound, "ZZZZZZ"},
		{"/v1/propose_node", "t-ada", `{"text": "---\nkind: concept\nname: Ring\n---\n\n## Summary\n\nA ring is an abelian [[group|ZZZZZZ]] with a second operation.\n"}`, http.StatusBadRequest, "fails the audit"},
		{"/v1/propose_edit", "t-ada", `["text"]`, http.StatusBadRequest, "not a JSON object"},
	}
	for _, tt := range tests {
		code, out := post(t, h, tt.path, tt.token, tt.body)
		if msg := fmt.Sprint(out["error"]); code != tt.code || !strings.Contains(msg, tt.want) {
			t.Errorf("%s %s: the status is %d with %s, want %d and %q", tt.path, tt.body, code, msg, tt.code, tt.want)
		}
	}
	if len(hs.opened) != 0 {
		t.Errorf("a write that failed opened %d pull requests", len(hs.opened))
	}
}

func TestWriteIndex(t *testing.T) {
	h, _ := writer(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1", nil))
	var ix struct {
		Writes []api.Method `json:"writes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ix); err != nil || len(ix.Writes) != len(api.Writes()) || ix.Writes[0].Name != propose.MethodNode {
		t.Errorf("the index has the writes %+v, %v", ix.Writes, err)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodOptions, "/v1/propose_node", nil))
	if w.Code != http.StatusNoContent || !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Errorf("the preflight gave %d with %v", w.Code, w.Header())
	}

	// With no proposer, the API has no write path.
	read := open(t).Handler(api.Options{Rate: -1})
	if code, out := post(t, read, "/v1/propose_node", "t-ada", `{"text": "x"}`); code != http.StatusNotFound {
		t.Errorf("a write with no proposer gave %d: %v", code, out)
	}
}
