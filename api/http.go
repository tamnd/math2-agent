package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/tamnd/math2-agent/propose"
)

// Options are the options of Handler, Limit and LimitGRPC.
type Options struct {
	Rate  float64 // the requests per second of each client. Zero means 10. A negative rate means no limit.
	Burst int     // the requests that a client can send at once. Zero means 20.

	// Propose turns on the write methods of Writes, as POST /v1/{method}. Each write opens a pull request with the proposer. Nil means that the handler serves only the read methods.
	Propose *propose.Proposer

	// Client returns the key of the client of a request, for the rate limit. Nil means the IP address of the peer. Behind a proxy, give a function that reads the address that the proxy sets. LimitGRPC does not use it, and the key of a gRPC client is the IP address of the peer.
	Client func(*http.Request) string
}

// maxBody is the largest request body that the handler reads.
const maxBody = 1 << 20

// Handler serves the methods over HTTP with JSON:
//
//	GET  /v1                 the methods and their parameters
//	GET  /v1/{method}?p=v    a call with the parameters in the query
//	POST /v1/{method}        a call with the parameters in a JSON object
//	GET  /healthz            ok
//
// A call gives a Response with the status 200, or an Error in {"error": ...} with the status of its code. Each client has the rate limit of Limit.
//
// When opt.Propose is set, the handler also serves each write method of Writes as POST /v1/{method}, with the header Authorization: Bearer and the GitHub token of a person. The result of a write is the propose.Result of its pull request. A write with no token, or with a token that GitHub does not accept, gets the status 401. A write that fails the audit gets 400 with the findings, and a write over the cap of its method gets 429.
func (s *Service) Handler(opt Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	ix := index{API: Version, Methods: methods}
	if opt.Propose != nil {
		ix.Writes = writes
		for _, m := range writes {
			mux.HandleFunc("POST /"+Version+"/"+m.Name, s.write(opt.Propose, m))
			mux.HandleFunc("OPTIONS /"+Version+"/"+m.Name, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Access-Control-Allow-Methods", "POST")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.WriteHeader(http.StatusNoContent)
			})
		}
	}
	mux.HandleFunc("GET /"+Version, func(w http.ResponseWriter, _ *http.Request) {
		write(w, http.StatusOK, ix)
	})
	call := func(w http.ResponseWriter, r *http.Request) {
		params, err := body(w, r)
		if err != nil {
			fail(w, r.PathValue("method"), err)
			return
		}
		resp, err := s.Call(r.PathValue("method"), params)
		if err != nil {
			fail(w, r.PathValue("method"), err)
			return
		}
		write(w, http.StatusOK, resp)
	}
	mux.HandleFunc("GET /"+Version+"/{method}", call)
	mux.HandleFunc("POST /"+Version+"/{method}", call)
	return Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		mux.ServeHTTP(w, r)
	}), opt)
}

// Limit gives each client of h the rate limit of opt. A client over the limit gets the status 429 with a Retry-After header and an Error with the code RateLimited. m2 serve puts the API and the MCP server behind one limit.
func Limit(h http.Handler, opt Options) http.Handler {
	lim := newLimiter(opt)
	if lim == nil {
		return h
	}
	if opt.Client == nil {
		opt.Client = peer
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wait, ok := lim.allow(opt.Client(r), time.Now()); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
			fail(w, "", &Error{Code: RateLimited, Message: "too many requests. Wait and try again"})
			return
		}
		h.ServeHTTP(w, r)
	})
}

// body returns the parameters of a call: the JSON object of the body of a POST, or the query of a GET.
func body(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	params := map[string]any{}
	if r.Method == http.MethodPost {
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
		dec.UseNumber()
		if err := dec.Decode(&params); err != nil {
			return nil, invalid("the body is not a JSON object of parameters: %v", err)
		}
		return params, nil
	}
	for name, vs := range r.URL.Query() {
		if len(vs) != 1 {
			return nil, invalid("the parameter %s has %d values, and it must have one", name, len(vs))
		}
		params[name] = vs[0]
	}
	return params, nil
}

// index is the answer of GET /v1.
type index struct {
	API     string   `json:"api"`
	Methods []Method `json:"methods"`
	Writes  []Method `json:"writes,omitempty"` // the write methods, when the handler serves them
}

// failure is the body of an error.
type failure struct {
	API    string `json:"api"`
	Method string `json:"method,omitempty"`
	Error  *Error `json:"error"`
}

var httpCodes = map[string]int{Invalid: http.StatusBadRequest, Unauthorized: http.StatusUnauthorized, NotFound: http.StatusNotFound, RateLimited: http.StatusTooManyRequests, InternalError: http.StatusInternalServerError}

func fail(w http.ResponseWriter, method string, err error) {
	var e *Error
	if !errors.As(err, &e) {
		e = &Error{Code: InternalError, Message: err.Error()}
	}
	write(w, httpCodes[e.Code], failure{API: Version, Method: method, Error: e})
}

func write(w http.ResponseWriter, code int, v any) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		code = http.StatusInternalServerError
		b.Reset()
		_ = json.NewEncoder(&b).Encode(failure{API: Version, Error: &Error{Code: InternalError, Message: err.Error()}})
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(b.Bytes())
}

func peer(r *http.Request) string { return host(r.RemoteAddr) }

// host returns the host of an address, or the address when it has no port.
func host(addr string) string {
	h, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return h
}

// limiter is a token bucket for each client.
type limiter struct {
	rate, burst float64
	mu          sync.Mutex
	clients     map[string]*bucket
}

// newLimiter returns the limiter of opt, or nil when the rate is negative.
func newLimiter(opt Options) *limiter {
	if opt.Rate < 0 {
		return nil
	}
	if opt.Rate == 0 {
		opt.Rate = 10
	}
	if opt.Burst <= 0 {
		opt.Burst = 20
	}
	return &limiter{rate: opt.Rate, burst: float64(opt.Burst), clients: map[string]*bucket{}}
}

type bucket struct {
	tokens float64
	last   time.Time
}

// maxClients is the number of buckets after which the limiter drops the full ones.
const maxClients = 10000

// allow takes a token of the client. When the bucket is empty, it returns the time until the next token.
func (l *limiter) allow(client string, now time.Time) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.clients[client]
	if !ok {
		if len(l.clients) >= maxClients {
			l.drop(now)
		}
		b = &bucket{tokens: l.burst, last: now}
		l.clients[client] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens < 1 {
		return time.Duration((1 - b.tokens) / l.rate * float64(time.Second)), false
	}
	b.tokens--
	return 0, true
}

// drop removes the buckets that are full again, because they are the same as new buckets.
func (l *limiter) drop(now time.Time) {
	for c, b := range l.clients {
		if b.tokens+now.Sub(b.last).Seconds()*l.rate >= l.burst {
			delete(l.clients, c)
		}
	}
}
