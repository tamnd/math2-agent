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
)

// Options are the options of Handler and Limit.
type Options struct {
	Rate  float64 // the requests per second of each client. Zero means 10. A negative rate means no limit.
	Burst int     // the requests that a client can send at once. Zero means 20.

	// Client returns the key of the client of a request, for the rate limit. Nil means the IP address of the peer. Behind a proxy, give a function that reads the address that the proxy sets.
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
func (s *Service) Handler(opt Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /"+Version, func(w http.ResponseWriter, _ *http.Request) {
		write(w, http.StatusOK, index{API: Version, Methods: methods})
	})
	call := func(w http.ResponseWriter, r *http.Request) {
		params := map[string]any{}
		if r.Method == http.MethodPost {
			dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
			dec.UseNumber()
			if err := dec.Decode(&params); err != nil {
				fail(w, r.PathValue("method"), invalid("the body is not a JSON object of parameters: %v", err))
				return
			}
		} else {
			for name, vs := range r.URL.Query() {
				if len(vs) != 1 {
					fail(w, r.PathValue("method"), invalid("the parameter %s has %d values, and it must have one", name, len(vs)))
					return
				}
				params[name] = vs[0]
			}
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
	if opt.Rate < 0 {
		return h
	}
	if opt.Rate == 0 {
		opt.Rate = 10
	}
	if opt.Burst <= 0 {
		opt.Burst = 20
	}
	if opt.Client == nil {
		opt.Client = peer
	}
	lim := &limiter{rate: opt.Rate, burst: float64(opt.Burst), clients: map[string]*bucket{}}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wait, ok := lim.allow(opt.Client(r), time.Now()); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
			fail(w, "", &Error{Code: RateLimited, Message: "too many requests. Wait and try again"})
			return
		}
		h.ServeHTTP(w, r)
	})
}

// index is the answer of GET /v1.
type index struct {
	API     string   `json:"api"`
	Methods []Method `json:"methods"`
}

// failure is the body of an error.
type failure struct {
	API    string `json:"api"`
	Method string `json:"method,omitempty"`
	Error  *Error `json:"error"`
}

var codes = map[string]int{Invalid: http.StatusBadRequest, NotFound: http.StatusNotFound, RateLimited: http.StatusTooManyRequests, InternalError: http.StatusInternalServerError}

func fail(w http.ResponseWriter, method string, err error) {
	var e *Error
	if !errors.As(err, &e) {
		e = &Error{Code: InternalError, Message: err.Error()}
	}
	write(w, codes[e.Code], failure{API: Version, Method: method, Error: e})
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

func peer(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// limiter is a token bucket for each client.
type limiter struct {
	rate, burst float64
	mu          sync.Mutex
	clients     map[string]*bucket
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
