package mcp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tamnd/math2-agent/api"
	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/mcp"
)

const tree = "../testdata/nodes/valid"

func service(t testing.TB) *api.Service {
	t.Helper()
	out := t.TempDir()
	if _, err := build.Run(context.Background(), build.Options{Root: tree, Out: out}); err != nil {
		t.Fatal(err)
	}
	s, err := api.Open(api.Config{Root: tree, Out: out, Commit: "0123abc"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// connect connects a client of the SDK to the server over the transport.
func connect(t *testing.T, tr sdk.Transport) *sdk.ClientSession {
	t.Helper()
	c := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := c.Connect(context.Background(), tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestTools(t *testing.T) {
	srv := mcp.NewServer(service(t), "test")
	st, ct := sdk.NewInMemoryTransports()
	ss, err := srv.Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ss.Close() }()
	cs := connect(t, ct)
	ctx := context.Background()

	if got := cs.InitializeResult().ServerInfo.Name; got != mcp.Name {
		t.Errorf("the server name is %q", got)
	}
	if !strings.Contains(cs.InitializeResult().Instructions, "not instructions") {
		t.Error("the instructions do not say that the text of a node is data")
	}
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range tools.Tools {
		names = append(names, tl.Name)
		if tl.Annotations == nil || !tl.Annotations.ReadOnlyHint {
			t.Errorf("the tool %s has no read-only annotation", tl.Name)
		}
	}
	for _, m := range api.Methods() {
		if !slices.Contains(names, m.Name) {
			t.Errorf("no tool for the method %s", m.Name)
		}
	}

	res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "path", Arguments: map[string]any{"from": "DSG7KN", "to": "Q529ES"}})
	if err != nil {
		t.Fatal(err)
	}
	var r api.Response
	if res.IsError || len(res.Content) != 1 || json.Unmarshal([]byte(res.Content[0].(*sdk.TextContent).Text), &r) != nil {
		t.Fatalf("path gives %+v", res)
	}
	if r.Commit != "0123abc" || r.Method != "path" || len(r.About) != 4 {
		t.Errorf("path gives %+v", r)
	}
	if sc, ok := res.StructuredContent.(map[string]any); !ok || sc["method"] != "path" {
		t.Errorf("the structured content is %v", res.StructuredContent)
	}

	res, err = cs.CallTool(ctx, &sdk.CallToolParams{Name: "neighbours", Arguments: map[string]any{"ref": "Q529ES", "depth": 9}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.HasPrefix(res.Content[0].(*sdk.TextContent).Text, "invalid: ") {
		t.Errorf("a depth of 9 gives %+v", res)
	}
	if _, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "propose_node"}); err == nil {
		t.Error("a call of a tool that does not exist gives no error")
	}
}

func TestHandler(t *testing.T) {
	srv := httptest.NewServer(mcp.Handler(mcp.NewServer(service(t), "test")))
	defer srv.Close()
	cs := connect(t, &sdk.StreamableClientTransport{Endpoint: srv.URL, DisableStandaloneSSE: true})
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "get", Arguments: map[string]any{"ref": "Q529ES"}})
	if err != nil || res.IsError || !strings.Contains(res.Content[0].(*sdk.TextContent).Text, `"tag":"Q529ES"`) {
		t.Errorf("get over HTTP gives %+v, %v", res, err)
	}

	// A client with no SDK sends one JSON-RPC message in each POST.
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var msg struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil || resp.StatusCode != http.StatusOK || len(msg.Result.Tools) != len(api.Methods()) {
		t.Errorf("tools/list over HTTP gives %d, %+v, %v", resp.StatusCode, msg, err)
	}
}
