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
	"strings"
	"syscall"
	"time"

	"github.com/tamnd/math2-agent/api"
)

const serveUsage = "m2 serve [-root DIR] [-out DIR] [-addr A] [-rate R] [-burst B] [-commit C]"

// interrupt returns the context of the server. The server stops when the context is done. Tests change it.
var interrupt = func() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// listening tells a test the address of the server.
var listening = func(string) {}

func runServe(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("m2 serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree")
	out := flags.String("out", "", "the output directory of m2 build. Empty means build/ in the tree.")
	addr := flags.String("addr", "localhost:8080", "the address to listen on")
	rate := flags.Float64("rate", 10, "the requests per second of each client. A negative rate means no limit.")
	burst := flags.Int("burst", 20, "the requests that a client can send at once")
	commit := flags.String("commit", "", "the commit of the tree, for the responses. Empty means the HEAD of the tree when it is a git repository.")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "m2 serve: give no argument, only flags")
		return ExitUsage
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
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(stderr, "m2 serve: %v\n", err)
		return ExitFailure
	}
	srv := &http.Server{
		Handler:           s.Handler(api.Options{Rate: *rate, Burst: *burst}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	ctx, stop := interrupt()
	defer stop()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	fmt.Fprintf(stdout, "m2 serve: the API is at http://%s/%s\n", ln.Addr(), api.Version)
	listening(ln.Addr().String())
	select {
	case err := <-done:
		fmt.Fprintf(stderr, "m2 serve: %v\n", err)
		return ExitFailure
	case <-ctx.Done():
	}
	shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shut); err != nil {
		fmt.Fprintf(stderr, "m2 serve: %v\n", err)
		return ExitFailure
	}
	return ExitOK
}
