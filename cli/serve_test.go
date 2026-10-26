package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/tamnd/math2-agent/api"
	"github.com/tamnd/math2-agent/api/m2pb"
	"github.com/tamnd/math2-agent/build"
)

func TestServe(t *testing.T) {
	out := t.TempDir()
	if _, err := build.Run(context.Background(), build.Options{Root: valid, Out: out}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	oldInterrupt, oldListening := interrupt, listening
	interrupt = func() (context.Context, context.CancelFunc) { return ctx, cancel }
	addrs := make(chan string, 2)
	listening = func(a string) { addrs <- a }
	defer func() {
		interrupt, listening = oldInterrupt, oldListening
	}()
	codes := make(chan int, 1)
	go func() {
		code, _, stderr := run("serve", "-root", valid, "-out", out, "-addr", "127.0.0.1:0", "-grpc", "127.0.0.1:0", "-commit", "abc")
		if stderr != "" {
			t.Errorf("m2 serve prints: %s", stderr)
		}
		codes <- code
	}()
	addr, grpcAddr := <-addrs, <-addrs
	resp, err := http.Get("http://" + addr + "/v1/get?ref=Q529ES")
	if err != nil {
		t.Fatal(err)
	}
	var r api.Response
	err = json.NewDecoder(resp.Body).Decode(&r)
	_ = resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK || r.Commit != "abc" || len(r.About) != 1 || r.About[0].Tag != "Q529ES" {
		t.Errorf("GET /v1/get gives %d, %+v, %v", resp.StatusCode, r, err)
	}
	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var msg struct {
		Result struct{ Tools []json.RawMessage } `json:"result"`
	}
	err = json.NewDecoder(resp.Body).Decode(&msg)
	_ = resp.Body.Close()
	if err != nil || len(msg.Result.Tools) != len(api.Methods()) {
		t.Errorf("POST /mcp tools/list gives %d, %d tools, %v", resp.StatusCode, len(msg.Result.Tools), err)
	}
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	g, err := m2pb.NewReadServiceClient(conn).Get(context.Background(), &m2pb.GetRequest{Ref: "Q529ES"})
	_ = conn.Close()
	if err != nil || g.Commit != "abc" || len(g.About) != 1 || g.About[0].Tag != "Q529ES" {
		t.Errorf("the gRPC Get gives %v, %v", g, err)
	}
	cancel()
	if code := <-codes; code != ExitOK {
		t.Errorf("m2 serve exits with code %d", code)
	}

	if code, _, stderr := run("serve", "-root", valid, "-out", t.TempDir()); code != ExitFailure || !strings.Contains(stderr, "m2 build") {
		t.Errorf("m2 serve with no build exits with code %d: %s", code, stderr)
	}
	if code, _, _ := run("serve", "x"); code != ExitUsage {
		t.Errorf("m2 serve x exits with code %d", code)
	}
	for _, args := range [][]string{{"-repo", "math2"}, {"-repo", "a/b/c"}, {"-propose", "-stdio"}} {
		if code, _, _ := run(append([]string{"serve"}, args...)...); code != ExitUsage {
			t.Errorf("m2 serve %v exits with code %d", args, code)
		}
	}
	if code, _, stderr := run("serve", "-propose", "-root", valid); code != ExitFailure || !strings.Contains(stderr, "not the top of a git clone") {
		t.Errorf("m2 serve -propose on a tree that is not a git clone exits with code %d: %s", code, stderr)
	}
}
