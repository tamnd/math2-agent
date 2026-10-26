package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/tamnd/math2-agent/api"
	"github.com/tamnd/math2-agent/mcp"
	"github.com/tamnd/math2-agent/propose"
)

const serveUsage = "m2 serve [-root DIR] [-out DIR] [-addr A] [-grpc A] [-rate R] [-burst B] [-commit C] [-propose] [-repo OWNER/NAME] [-github URL] [-stdio]"

// interrupt returns the context of the server. The server stops when the context is done. Tests change it.
var interrupt = func() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// listening tells a test the address of the HTTP server, and then the address of the gRPC server when it is on.
var listening = func(string) {}

func runServe(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("m2 serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree")
	out := flags.String("out", "", "the output directory of m2 build. Empty means build/ in the tree.")
	addr := flags.String("addr", "localhost:8080", "the address to listen on")
	grpcAddr := flags.String("grpc", "", "the address of the gRPC server. Empty means no gRPC server.")
	rate := flags.Float64("rate", 10, "the requests per second of each client. A negative rate means no limit.")
	burst := flags.Int("burst", 20, "the requests that a client can send at once")
	commit := flags.String("commit", "", "the commit of the tree, for the responses. Empty means the HEAD of the tree when it is a git repository.")
	write := flags.Bool("propose", false, "serve the write methods over HTTP. Each write opens a pull request on GitHub with the token of the caller. The root must be a git clone of the repository.")
	repo := flags.String("repo", "tamnd/math2", "the GitHub repository of the pull requests of -propose")
	gh := flags.String("github", "", "the URL of the GitHub REST API, for -propose. Empty means https://api.github.com.")
	stdio := flags.Bool("stdio", false, "serve the MCP server on stdin and stdout, not HTTP")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "m2 serve: give no argument, only flags")
		return ExitUsage
	}
	owner, name, ok := strings.Cut(*repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		fmt.Fprintf(stderr, "m2 serve: -repo %q is not OWNER/NAME\n", *repo)
		return ExitUsage
	}
	if *write && *stdio {
		fmt.Fprintln(stderr, "m2 serve: -propose serves the writes over HTTP, and it does not work with -stdio")
		return ExitUsage
	}
	if *write && !gitRoot(*root) {
		fmt.Fprintf(stderr, "m2 serve: -propose needs a git clone of %s as the root, and %s is not the top of a git clone\n", *repo, *root)
		return ExitFailure
	}
	if *commit == "" {
		if b, err := exec.Command("git", "-C", *root, "rev-parse", "HEAD").Output(); err == nil { // #nosec G204 -- the user gives the root, and git gets it as one argument.
			*commit = strings.TrimSpace(string(b))
		}
	}
	s, err := api.Open(api.Config{Root: *root, Out: *out, Commit: *commit})
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(stderr, "m2 serve: %v. Run m2 build first.\n", err)
		return ExitFailure
	}
	if err != nil {
		fmt.Fprintf(stderr, "m2 serve: %v\n", err)
		return ExitFailure
	}
	defer func() { _ = s.Close() }()
	srv := mcp.NewServer(s, version)
	ctx, stop := interrupt()
	defer stop()
	if *stdio {
		if err := srv.Run(ctx, &sdk.StdioTransport{}); err != nil && ctx.Err() == nil {
			fmt.Fprintf(stderr, "m2 serve: %v\n", err)
			return ExitFailure
		}
		return ExitOK
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(stderr, "m2 serve: %v\n", err)
		return ExitFailure
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcp.Handler(srv))
	opt := api.Options{Rate: -1}
	if *write {
		opt.Propose = &propose.Proposer{Root: *root, Host: &propose.GitHub{Owner: owner, Repo: name, API: *gh, UserAgent: "m2/" + version + " (+https://github.com/tamnd/math2-agent)"}}
	}
	mux.Handle("/", s.Handler(opt))
	hs := &http.Server{
		Handler:           api.Limit(mux, api.Options{Rate: *rate, Burst: *burst}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	done := make(chan error, 2)
	go func() { done <- hs.Serve(ln) }()
	fmt.Fprintf(stdout, "m2 serve: the API is at http://%s/%s and the MCP server is at http://%s/mcp\n", ln.Addr(), api.Version, ln.Addr())
	if *write {
		fmt.Fprintf(stdout, "m2 serve: the write methods open pull requests on %s\n", *repo)
	}
	listening(ln.Addr().String())
	if *grpcAddr != "" {
		gl, err := net.Listen("tcp", *grpcAddr)
		if err != nil {
			fmt.Fprintf(stderr, "m2 serve: %v\n", err)
			_ = hs.Close()
			return ExitFailure
		}
		gs := grpc.NewServer(grpc.UnaryInterceptor(api.LimitGRPC(api.Options{Rate: *rate, Burst: *burst})))
		s.RegisterGRPC(gs)
		reflection.Register(gs)
		go func() { done <- gs.Serve(gl) }()
		defer gs.GracefulStop()
		fmt.Fprintf(stdout, "m2 serve: the gRPC service %s is at %s\n", api.ProtoService, gl.Addr())
		listening(gl.Addr().String())
	}
	select {
	case err := <-done:
		fmt.Fprintf(stderr, "m2 serve: %v\n", err)
		return ExitFailure
	case <-ctx.Done():
	}
	shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := hs.Shutdown(shut); err != nil {
		fmt.Fprintf(stderr, "m2 serve: %v\n", err)
		return ExitFailure
	}
	return ExitOK
}

// gitRoot reports whether dir is the top directory of a git clone with a commit.
func gitRoot(dir string) bool {
	top, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output() // #nosec G204 -- the user gives the root, and git gets it as one argument.
	if err != nil {
		return false
	}
	if _, err := exec.Command("git", "-C", dir, "rev-parse", "--verify", "-q", "HEAD").Output(); err != nil { // #nosec G204 -- as above.
		return false
	}
	a, errA := filepath.EvalSymlinks(strings.TrimSpace(string(top)))
	b, errB := filepath.EvalSymlinks(dir)
	if errA != nil || errB != nil {
		return false
	}
	b, err = filepath.Abs(b)
	return err == nil && a == b
}
