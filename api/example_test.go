package api_test

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/tamnd/math2-agent/api"
	"github.com/tamnd/math2-agent/api/m2pb"
	"github.com/tamnd/math2-agent/build"
)

// service builds the test tree in a new directory and opens a service on it.
func service() (*api.Service, func()) {
	dir, err := os.MkdirTemp("", "m2api")
	if err != nil {
		panic(err)
	}
	if _, err := build.Run(context.Background(), build.Options{Root: tree, Out: dir}); err != nil {
		panic(err)
	}
	s, err := api.Open(api.Config{Root: tree, Out: dir})
	if err != nil {
		panic(err)
	}
	return s, func() { _ = s.Close(); _ = os.RemoveAll(dir) }
}

func ExampleService_Call() {
	s, done := service()
	defer done()
	r, err := s.Call("path", map[string]any{"from": "DSG7KN", "to": "Q529ES"})
	if err != nil {
		panic(err)
	}
	fmt.Println(r.Result)
	for _, a := range r.About {
		fmt.Println(a.Tag, cmp.Or(a.Evidence, "-"))
	}
	// Output:
	// [DSG7KN GY1XDB E18G47 Q529ES]
	// DSG7KN -
	// GY1XDB -
	// E18G47 -
	// Q529ES E1
}

func ExampleService_Handler() {
	s, done := service()
	defer done()
	srv := httptest.NewServer(s.Handler(api.Options{}))
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL + "/v1/status?ref=nope")
	if err != nil {
		panic(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	fmt.Print(resp.StatusCode, " ", string(body))
	// Output:
	// 404 {"api":"v1","method":"status","error":{"code":"not_found","message":"no node has the alias nope"}}
}

func ExampleService_RegisterGRPC() {
	s, done := service()
	defer done()
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		panic(err)
	}
	srv := grpc.NewServer(grpc.UnaryInterceptor(api.LimitGRPC(api.Options{})))
	s.RegisterGRPC(srv)
	go func() { _ = srv.Serve(ln) }()
	defer srv.Stop()

	conn, err := grpc.NewClient(ln.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}
	defer func() { _ = conn.Close() }()
	r, err := m2pb.NewReadServiceClient(conn).Path(context.Background(), &m2pb.PathRequest{From: "DSG7KN", To: "Q529ES"})
	if err != nil {
		panic(err)
	}
	for _, v := range r.Result.GetListValue().GetValues() {
		fmt.Println(v.GetStringValue())
	}
	_, err = m2pb.NewReadServiceClient(conn).Get(context.Background(), &m2pb.GetRequest{Ref: "ZZZZZZ"})
	fmt.Println(status.Code(err))
	// Output:
	// DSG7KN
	// GY1XDB
	// E18G47
	// Q529ES
	// NotFound
}
