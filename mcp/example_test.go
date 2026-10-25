package mcp_test

import (
	"context"
	"fmt"
	"os"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tamnd/math2-agent/api"
	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/mcp"
)

func ExampleNewServer() {
	out, err := os.MkdirTemp("", "m2mcp")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(out) }()
	if _, err := build.Run(context.Background(), build.Options{Root: tree, Out: out}); err != nil {
		panic(err)
	}
	s, err := api.Open(api.Config{Root: tree, Out: out})
	if err != nil {
		panic(err)
	}
	defer func() { _ = s.Close() }()

	// Connect a client of the MCP SDK to the server in memory.
	ctx := context.Background()
	st, ct := sdk.NewInMemoryTransports()
	ss, err := mcp.NewServer(s, "dev").Connect(ctx, st, nil)
	if err != nil {
		panic(err)
	}
	defer func() { _ = ss.Close() }()
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "client", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		panic(err)
	}
	defer func() { _ = cs.Close() }()

	res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "resolve", Arguments: map[string]any{"ref": "Q180967"}})
	if err != nil {
		panic(err)
	}
	r := res.StructuredContent.(map[string]any)["result"].(map[string]any)
	fmt.Println(r["match"], r["tags"])
	// Output:
	// identifier [Q529ES]
}
